package subtitles

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

func TestReleaseSearchQueuesSubtitlesForItsVideoFiles(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	q := jobs.New(f.st, jobs.Options{})
	s := &Service{Store: f.st}
	s.Register(q)
	var keptPart, unkeptPart, movieSlot int64
	if err := f.st.Write(ctx, func(tx *sql.Tx) error {
		stamp := database.FormatTime(now())
		for n, part := range []*int64{&keptPart, &unkeptPart} {
			err := tx.QueryRow(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name, keep_up_to_date)
				VALUES (?, ?, ?, ?, ?, 'season', ?, 'Season', ?) RETURNING id`, stamp, sysActor, stamp, sysActor, f.show, n+1, n == 0).Scan(part)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE items SET part_id = CASE id WHEN ? THEN ? ELSE ? END WHERE title_id = ?", f.ep1, keptPart, unkeptPart, f.show); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE titles SET keep_up_to_date = 1 WHERE id = ?", f.show); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO subtitle_looks (file_id, looked_at, wish) VALUES (?, ?, '[]')", f.ep1File, stamp); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, origin, state, next_try_at)
			VALUES (?, ?, ?, ?, ?, 'en', 'wish', 'fetched', 'wanted', ?)`,
			stamp, sysActor, stamp, sysActor, f.ep1File, database.FormatTime(now().Add(time.Hour))); err != nil {
			return err
		}
		return tx.QueryRow("SELECT id FROM slots WHERE item_id = ?", f.movieItem).Scan(&movieSlot)
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		scope ReleaseSearchScope
		want  []int64
	}{
		{"movie slot", ReleaseSearchScope{SlotID: movieSlot}, []int64{f.movieFile}},
		{"selected seasons", ReleaseSearchScope{TitleID: f.show, KeptOnly: true}, []int64{f.ep1File}},
		{"explicit season", ReleaseSearchScope{PartID: unkeptPart, VersionID: f.showVer}, []int64{f.ep2File}},
		{"other version", ReleaseSearchScope{PartID: unkeptPart, VersionID: f.show4K}, nil},
		{"scheduled backoff", ReleaseSearchScope{PartID: keptPart, VersionID: f.showVer, DueOnly: true}, nil},
		{"movie bulk search", ReleaseSearchScope{Kind: "movie", DueOnly: true}, []int64{f.movieFile}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := s.releaseSearchFiles(ctx, c.scope)
			if err != nil || !slices.Equal(got, c.want) {
				t.Fatalf("files = %v, %v; want %v", got, err, c.want)
			}
		})
	}
	s.AfterReleaseSearch(ctx, ReleaseSearchScope{TitleID: f.show, KeptOnly: true})
	s.AfterReleaseSearch(ctx, ReleaseSearchScope{PartID: keptPart, VersionID: f.showVer})
	var count, fileID int64
	if err := f.st.Read().QueryRow("SELECT count(*), COALESCE(max(json_extract(subject, '$.id')), 0) FROM jobs WHERE kind = ?", KindSearch).Scan(&count, &fileID); err != nil {
		t.Fatal(err)
	}
	if count != 1 || fileID != f.ep1File {
		t.Fatalf("queued %d subtitle jobs, last file %d; want one for %d", count, fileID, f.ep1File)
	}
}
