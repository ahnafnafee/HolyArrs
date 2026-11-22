package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// OtherShowFolder reports a secondary copy in another library folder with
// no independent monitoring or 4K preference.
func OtherShowFolder(main, other Version) bool {
	return main.IsMain && !main.FourK && !other.IsMain && !other.KeepUpToDate &&
		!other.FourK && other.LibraryFolderID != main.LibraryFolderID
}

// ShowOtherFolderFileSQL finds a present episode copy in a secondary, unmonitored
// folder. The surrounding query aliases the primary candidate slot as s.
const ShowOtherFolderFileSQL = `(SELECT f.id
	FROM versions primary_version
	JOIN slots other ON other.item_id = s.item_id AND other.version_id != s.version_id
	JOIN versions v2 ON v2.id = other.version_id AND v2.title_id = primary_version.title_id
	JOIN files f ON f.id = other.file_id AND f.state = 'present'
	WHERE primary_version.id = s.version_id AND primary_version.is_main = 1 AND primary_version.four_k = 0
	AND v2.is_main = 0 AND v2.keep_up_to_date = 0 AND v2.four_k = 0
	AND v2.library_folder_id != primary_version.library_folder_id AND other.state != 'removed'
	ORDER BY v2.id LIMIT 1)`

// ShowEpisodeHasFileSQL counts a primary file or a secondary folder's copy.
const ShowEpisodeHasFileSQL = `COALESCE(s.file_id, ` + ShowOtherFolderFileSQL + `) IS NOT NULL`

// ShowEpisodesInOtherFolder returns the primary slots covered by another folder.
func ShowEpisodesInOtherFolder(ctx context.Context, q database.Lister, versionID int64) (map[int64]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id FROM slots s WHERE s.version_id = ? AND s.file_id IS NULL AND `+ShowOtherFolderFileSQL+` IS NOT NULL`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	covered := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		covered[id] = true
	}
	return covered, rows.Err()
}

// ShowEpisodeInOtherFolder checks one slot before an automatic search.
func ShowEpisodeInOtherFolder(ctx context.Context, q database.Querier, slotID int64) (bool, error) {
	var covered bool
	err := q.QueryRowContext(ctx, `SELECT `+ShowOtherFolderFileSQL+` IS NOT NULL FROM slots s WHERE s.id = ?`, slotID).Scan(&covered)
	return covered, err
}
