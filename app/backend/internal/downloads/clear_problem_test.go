package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestClearProblemKeepsFilesAndStopsLooking(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	if err := m.ClearProblem(ctx, id, false, luis); !errors.Is(err, errNotProblem) {
		t.Fatalf("active download clear: %v", err)
	}
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE downloads SET state = 'failed' WHERE id = ?", id); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'needs_a_decision' WHERE id = ?", w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	queue := jobs.New(m.Store, jobs.Options{})
	subject := &database.Subject{Type: "slot", ID: w.slotID}
	auto, _, err := queue.Enqueue(ctx, jobs.KindSearchSlot, subject, nil, "auto", database.System)
	if err != nil {
		t.Fatal(err)
	}
	manual, _, err := queue.Enqueue(ctx, jobs.KindSearchSlot, subject, map[string]any{"by_hand": true}, "manual", luis)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ClearProblem(ctx, id, false, luis); err != nil {
		t.Fatal(err)
	}
	for jobID, state := range map[int64]string{auto: "cancelled", manual: "queued"} {
		var job jobs.Job
		if err := database.Get(ctx, m.Store.Read(), "jobs", jobID, &job); err != nil || job.State != state {
			t.Fatalf("job %d: %s, want %s (%v)", jobID, job.State, state, err)
		}
	}
	if err := m.ClearProblem(ctx, id, false, luis); err != nil {
		t.Fatalf("repeat clear: %v", err)
	}
	var keep bool
	if err := m.Store.Read().QueryRow("SELECT keep_up_to_date FROM versions WHERE id = (SELECT version_id FROM slots WHERE id = ?)", w.slotID).Scan(&keep); err != nil {
		t.Fatal(err)
	}
	if d, s := w.download(t, id), w.slot(t); d.State != "cleaned" || s.State != mediafiles.SlotWanted || s.ActiveDownloadID != nil || keep {
		t.Fatalf("download %s, slot %s, attached %v, keep up to date %v", d.State, s.State, s.ActiveDownloadID != nil, keep)
	}
	if len(w.failedSlots) != 0 || len(w.fake.deleted) != 0 {
		t.Fatal("clear searched again or removed files")
	}
}

func TestClearProblemLeavesNewerAttemptAlone(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	newer := w.download(t, id)
	newer.ID = 0
	newer.ExternalID += "-new"
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE downloads SET state = 'failed' WHERE id = ?", id); err != nil {
			return err
		}
		next, err := database.Insert(tx, "downloads", &newer, luis)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", next, w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.ClearProblem(ctx, id, true, luis); err != nil {
		t.Fatal(err)
	}
	s := w.slot(t)
	if s.ActiveDownloadID == nil || *s.ActiveDownloadID == id || s.State != mediafiles.SlotDownloading || len(w.failedSlots) != 0 {
		t.Fatalf("changed replacement: %+v; searches: %v", s, w.failedSlots)
	}
}

func TestClearProblemCanSearchAgainAfterImportReview(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := grabbed(t, w, m)
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE downloads SET state = 'finished' WHERE id = ?", id); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'needs_a_decision' WHERE id = ?", w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.ClearProblem(ctx, id, true, luis); err != nil {
		t.Fatal(err)
	}
	d, s := w.download(t, id), w.slot(t)
	if d.State != "cleaned" || s.State != mediafiles.SlotWanted || len(w.failedSlots) != 1 || w.failedSlots[0] != w.slotID {
		t.Fatalf("download %s, slot %s, searches %v", d.State, s.State, w.failedSlots)
	}
	if len(w.fake.deleted) != 0 {
		t.Fatal("clear called the client")
	}
}
