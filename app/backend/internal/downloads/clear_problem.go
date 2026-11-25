package downloads

import (
	"context"
	"database/sql"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

var errNotProblem = errors.New("this download is no longer failed or waiting for review")

// ClearProblem dismisses a problem in HolyArrs and resolves the Slots still
// tied to it. An older failed attempt must not change its replacement.
func (m *Manager) ClearProblem(ctx context.Context, id int64, search bool, by domain.Actor) error {
	reason := "download problem cleared"
	var d domain.Download
	moved := map[int64]mediafiles.SlotState{}
	changed := false
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		clear(moved)
		changed = false
		if err := database.Get(ctx, tx, "downloads", id, &d); err != nil {
			return err
		}
		if d.State == "cleaned" && d.StateReason != nil && *d.StateReason == reason {
			return nil
		}
		failed := d.State == "failed" || d.State == "vanished"
		if !failed && !finishedState(d.State) {
			return errNotProblem
		}
		if running, err := mediafiles.ImportRunning(ctx, tx, id); err != nil {
			return err
		} else if running {
			return ErrImporting
		}
		for _, sid := range d.SlotIDs {
			state, download, file, err := mediafiles.SlotDownloadTx(tx, sid)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if !download.Valid || download.Int64 != id {
				continue
			}
			switch mediafiles.SlotState(state) {
			case mediafiles.SlotNeedsADecision, mediafiles.SlotRejected:
			case mediafiles.SlotFailed, mediafiles.SlotWanted, mediafiles.SlotBetterWanted:
				if !failed {
					continue
				}
			default:
				continue
			}
			to := mediafiles.SlotWanted
			if file.Valid {
				to = mediafiles.SlotBetterWanted
			}
			if !search {
				var kind string
				if err := tx.QueryRow(`SELECT t.kind FROM slots s JOIN items i ON i.id = s.item_id
					JOIN titles t ON t.id = i.title_id WHERE s.id = ?`, sid).Scan(&kind); err != nil {
					return err
				}
				if err := stopLookingTx(tx, sid, kind); err != nil {
					return err
				}
				if err := mediafiles.ClearSlotDownloadTx(tx, sid); err != nil {
					return err
				}
			}
			if mediafiles.SlotState(state) != to {
				if err := mediafiles.MoveSlotTx(tx, sid, to, reason, by); err != nil {
					return err
				}
			}
			moved[sid] = to
		}
		if !failed && len(moved) == 0 {
			return errNotProblem
		}
		now := m.now()
		d.State, d.StateReason, d.CleanedAt = "cleaned", &reason, &now
		d.Speed, d.UpSpeed, d.Seeds, d.Peers, d.ETA = 0, 0, 0, 0, nil
		changed = true
		return database.Update(tx, "downloads", &d, by)
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	m.publish("download", d)
	m.eventWith(ctx, &d, "download_removed", "Cleared a download problem", map[string]any{"search_again": search})
	for sid, to := range moved {
		m.publish("slot", map[string]any{"id": sid, "state": to, "reason": reason})
		if search && m.OnFailed != nil {
			m.OnFailed(ctx, sid)
		}
	}
	return nil
}
