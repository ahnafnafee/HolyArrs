package subtitles

import (
	"context"
	"log"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ReleaseSearchScope identifies the copies involved in a media search.
// A scope without a title, part or slot is a library-wide search.
type ReleaseSearchScope struct {
	TitleID, PartID, SlotID, VersionID int64
	Kind                               string
	KeptOnly, DueOnly                  bool
}

// AfterReleaseSearch queues independent subtitle jobs for existing video
// files. A subtitle queue failure must not interrupt the release search.
func (s *Service) AfterReleaseSearch(ctx context.Context, scope ReleaseSearchScope) int {
	ids, err := s.releaseSearchFiles(ctx, scope)
	if err != nil {
		log.Printf("subtitles: select files for release search: %v", err)
		return 0
	}
	queued := 0
	for _, id := range ids {
		if err := s.Queue(ctx, id, nil); err != nil {
			log.Printf("subtitles: queue search for file %d: %v", id, err)
			break
		}
		queued++
	}
	return queued
}

func (s *Service) releaseSearchFiles(ctx context.Context, scope ReleaseSearchScope) ([]int64, error) {
	q := `SELECT DISTINCT f.id FROM items i
		JOIN titles t ON t.id = i.title_id JOIN slots sl ON sl.item_id = i.id JOIN files f ON f.id = sl.file_id
		JOIN versions v ON v.id = f.version_id JOIN library_folders lf ON lf.id = v.library_folder_id
		WHERE t.state != 'removed' AND i.dropped_at IS NULL AND i.kind IN ('movie', 'episode')
		AND f.role = 'main' AND f.state = 'present' AND lf.look_only = 0`
	var args []any
	if scope.TitleID != 0 {
		q += " AND i.title_id = ?"
		args = append(args, scope.TitleID)
	}
	if scope.PartID != 0 {
		q += " AND i.part_id = ?"
		args = append(args, scope.PartID)
	}
	if scope.SlotID != 0 {
		q += " AND sl.id = ?"
		args = append(args, scope.SlotID)
	}
	if scope.VersionID != 0 {
		q += " AND v.id = ?"
		args = append(args, scope.VersionID)
	}
	if scope.Kind != "" {
		q += " AND t.kind = ?"
		args = append(args, scope.Kind)
	}
	if scope.KeptOnly {
		q += ` AND t.keep_up_to_date = 1 AND i.keep_up_to_date = 1
			AND (i.part_id IS NULL OR EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1))`
	}
	if scope.DueOnly {
		q += ` AND (NOT EXISTS (SELECT 1 FROM subtitle_looks l WHERE l.file_id = f.id)
			OR EXISTS (SELECT 1 FROM subtitles sub WHERE sub.file_id = f.id AND sub.place = 'wish'
				AND sub.state IN ('wanted', 'none_good_enough') AND (sub.next_try_at IS NULL OR sub.next_try_at <= ?)))`
		args = append(args, database.FormatTime(now()))
	}
	db := s.Store.Read()
	q += " ORDER BY f.id"
	if scope.TitleID == 0 && scope.PartID == 0 && scope.SlotID == 0 {
		q += " LIMIT ?"
		args = append(args, ReadSettings(ctx, db).Batch)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
