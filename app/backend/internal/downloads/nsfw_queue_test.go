package downloads

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

func TestQueueNSFWRequiresSeparateSaveFolder(t *testing.T) {
	ctx := context.Background()
	w, m := setup(t)
	f := &manageFake{answers: map[string]string{
		"app/preferences":     `{"save_path":"/downloads/library"}`,
		"torrents/categories": `{}`,
		"torrents/info":       `[]`,
		"torrents/add":        "Ok.",
	}}
	target := f.start(t)
	d := &domain.Downloader{Name: "qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: target, State: "ok", AfterImport: "leave", AllowLocal: true, Writable: true}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	src := domain.Source{DownloaderID: &d.ID}
	item := sources.Item{Link: "magnet:?xt=urn:btih:" + hashA + "&tr=http://127.0.0.1:80/announce"}
	if _, err := m.QueueNSFW(ctx, src, item); err == nil || f.count("torrents/add") != 0 {
		t.Fatal("release queued with no category save folder")
	}
	f.mu.Lock()
	f.answers["torrents/categories"] = `{"nsfw":{"savePath":"/downloads/separate"}}`
	f.mu.Unlock()
	if result, err := m.QueueNSFW(ctx, src, item); err != nil || result != "added" {
		t.Fatalf("queue returned %s, %v", result, err)
	}
	got := f.last("torrents/add")
	if !strings.Contains(got, "category=nsfw") || !strings.Contains(got, "savepath=/downloads/separate") || strings.Contains(got, "127.0.0.1") {
		t.Fatalf("unexpected qBittorrent request: %s", got)
	}
}
