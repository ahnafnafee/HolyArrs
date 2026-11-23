package downloads

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// QueueNSFW sends a standalone release to its Source's assigned qBittorrent.
// The explicit category keeps these jobs outside title imports.
func (m *Manager) QueueNSFW(ctx context.Context, src domain.Source, item sources.Item) (string, error) {
	const category = "nsfw"
	if m == nil || src.DownloaderID == nil {
		return "", errors.New("assign a qBittorrent downloader to this source")
	}
	var downloader domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", *src.DownloaderID, &downloader); err != nil ||
		downloader.Removed || downloader.State == "disabled" || downloader.Adapter != "qbittorrent" ||
		downloader.Protocol != "torrent" || !downloader.Writable || ReadOnlyByEnv(downloader) {
		return "", errors.New("assign an active, writable qBittorrent downloader to this source")
	}
	prepareErr := errors.New("the source did not provide a usable torrent")
	link, err := grabLinkAllowed("qbittorrent", item.Link, src.AllowLocal)
	if err != nil {
		return "", prepareErr
	}
	batch := AddBatch{Category: category}
	if !strings.HasPrefix(link, "magnet:") {
		var rt http.RoundTripper
		if src.ProxyURL != "" {
			if m.SourceTransport == nil {
				return "", prepareErr
			}
			if rt, err = m.SourceTransport(ctx, src); err != nil {
				return "", prepareErr
			}
		}
		var header http.Header
		if m.SourceHeader != nil {
			h, err := m.SourceHeader(ctx, src)
			if err != nil {
				return "", prepareErr
			}
			header = sourceHeaderFor(src.URL, link, h)
		}
		file, magnet, err := fetchGrabLinkHeader(ctx, link, addFetchMax, rt, sourceLocalHost(src.URL, src.AllowLocal), header)
		if err != nil {
			return "", prepareErr
		}
		if magnet == "" {
			file, err = safeTorrent(bytes.TrimRight(file, " \t\r\n"))
			if err != nil {
				return "", prepareErr
			}
			hash, err := torrentFileHash(file)
			if err != nil || item.InfoHash != "" && !strings.EqualFold(hash, item.InfoHash) {
				return "", prepareErr
			}
			batch.Files = []TorrentUpload{{Name: item.Title + ".torrent", Data: file}}
		}
		link = magnet
	}
	if link != "" {
		clean, _, err := safeMagnet(link, false)
		if err != nil || item.InfoHash != "" && !strings.EqualFold(linkHash(clean), item.InfoHash) {
			return "", prepareErr
		}
		batch.Links = []string{clean}
	}

	var out AddOutcome
	setupErr := errors.New("configure a separate save folder for the nsfw category in qBittorrent")
	var actionErr error
	err = m.ControlEngine(ctx, downloader, func(e Engine) error {
		q, ok := e.(interface {
			TorrentAdder
			TorrentFolders
		})
		if !ok {
			return ErrNotSupported
		}
		folders, err := q.SaveBase(ctx)
		if err != nil {
			return err
		}
		folder := folders.Categories[category]
		normalize := func(s string) string { return path.Clean(strings.ReplaceAll(s, "\\", "/")) }
		if folder == "" || strings.EqualFold(normalize(folder), normalize(folders.Default)) {
			actionErr = setupErr
			return actionErr
		}
		batch.SavePath = folder
		out, err = q.AddTorrents(ctx, batch)
		return err
	})
	switch {
	case actionErr != nil:
		return "", actionErr
	case err != nil || out.Failed > 0:
		return "", errors.New("qBittorrent did not confirm the release. Check qBittorrent before retrying")
	case out.Duplicates > 0:
		return "duplicate", nil
	case out.Pending > 0:
		return "pending", nil
	case out.Added > 0:
		return "added", nil
	default:
		return "", errors.New("qBittorrent did not confirm the release. Check qBittorrent before retrying")
	}
}
