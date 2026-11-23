package downloads

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrNotOurs means a change was asked of another app's item on a Downloader
// set to show only HolyArrs's own items.
var ErrNotOurs = errors.New("this item belongs to another app on the same Downloader")

// ownCategory reports whether category is one of cats. A Downloader with no
// category, "" or "*" has every item as its own.
func ownCategory(cats []string, category string) bool {
	for _, c := range cats {
		if c == "" || c == "*" {
			return true
		}
	}
	return hasCategory(cats, category)
}

// hiddenItem reports whether j is hidden on d: d shows only its own items and
// j is in another app's category.
func hiddenItem(d domain.Downloader, j Job) bool {
	return d.OwnOnly && !ownCategory(d.Categories(), j.Category)
}

// hideClientItem is the Downloads view's category boundary. Standalone
// search downloads keep their separate folder and require settings access.
func hideClientItem(ctx context.Context, d domain.Downloader, j Job) bool {
	if isStandaloneSearchCategory(j.Category) {
		return !maySeeStandaloneSearch(ctx)
	}
	return hiddenItem(d, j)
}

func (a *Web) clientSnapshotItem(downloaderID int64, hash string) (Job, bool) {
	if a.DLSync == nil {
		return Job{}, false
	}
	j, ok := a.DLSync.Store.Current(downloaderID).Items[strings.ToLower(hash)]
	return j, ok
}

// notOurs checks the reader's snapshot before changing client items.
func (a *Web) notOurs(ctx context.Context, downloaderID int64, hashes []string) bool {
	if a.DLSync == nil || len(hashes) == 0 {
		return false
	}
	var d domain.Downloader
	if database.Get(ctx, a.read(), "downloaders", downloaderID, &d) != nil {
		return false
	}
	items := a.DLSync.Store.Current(downloaderID).Items
	for _, hash := range hashes {
		if j, ok := items[strings.ToLower(hash)]; ok && hideClientItem(ctx, d, j) {
			return true
		}
	}
	return false
}

// everyOwn starts or stops the torrents in q's categories. Without any
// category known it does nothing rather than touch every torrent.
func (q *Qbittorrent) everyOwn(ctx context.Context, start bool) error {
	var hashes []string
	for _, c := range q.cats {
		if c == "" {
			continue
		}
		jobs, err := q.List(ctx, c)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if h, err := qbitHash(j.ID); err == nil && !slices.Contains(hashes, h) {
				hashes = append(hashes, h)
			}
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	return q.qbitStartStop(ctx, strings.Join(hashes, "|"), start)
}
