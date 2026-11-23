package sources

import (
	"context"
	"errors"
	htmltext "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

var nsfwArtworkClient = &http.Client{
	Timeout: 6 * time.Second,
	Transport: &http.Transport{
		DialContext:     outbound.PolicyDialer(outbound.PublicOnly).DialContext,
		MaxIdleConns:    12,
		IdleConnTimeout: 30 * time.Second,
	},
	CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 4 || nsfwWebURL(r.URL.String()) == nil {
			return errors.New("artwork redirect refused")
		}
		return nil
	},
}

// The first cell pair in this release table holds the cover. Other images on
// the page include ads and screenshots, so never fall back to a generic img.
var xxxtorCover = regexp.MustCompile(`(?is)<table\s+id="details"\s*>\s*<tr>\s*<td>\s*</td>\s*<td>\s*<img\b[^>]*?\bsrc="([^"]+)"`)

func xxxtorReleasePage(item Item) *url.URL {
	page := nsfwWebURL(item.DetailsURL)
	if page == nil || page.Hostname() != "xxxtor.com" || !strings.HasPrefix(page.Path, "/torrent/") || page.RawQuery != "" {
		return nil
	}
	return page
}

// Lazy image requests from the list share this limit for release page lookups.
var nsfwCoverLookups = make(chan struct{}, 4)

func nsfwCoverFromPage(r *http.Request, page *url.URL) *url.URL {
	select {
	case nsfwCoverLookups <- struct{}{}:
		defer func() { <-nsfwCoverLookups }()
	case <-r.Context().Done():
		return nil
	}
	body := nsfwFetchArtwork(r, page, 768<<10)
	match := xxxtorCover.FindSubmatch(body)
	if len(match) != 2 {
		return nil
	}
	return nsfwWebURL(htmltext.UnescapeString(string(match[1])))
}

func nsfwFetchArtwork(r *http.Request, u *url.URL, limit int64) []byte {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := nsfwArtworkClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil
	}
	return body
}

func (h *Handlers) nsfwThumbnail(w http.ResponseWriter, r *http.Request) {
	if !nsfwAccess(w, r) {
		return
	}
	hit, ok := h.nsfwResult(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	cover := nsfwWebURL(hit.item.CoverURL)
	if page := xxxtorReleasePage(hit.item); cover == nil && page != nil {
		cover = nsfwCoverFromPage(r, page)
	}
	if cover == nil {
		http.NotFound(w, r)
		return
	}
	body := nsfwFetchArtwork(r, cover, 4<<20)
	switch contentType := http.DetectContentType(body); contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(body)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handlers) downloadNSFW(w http.ResponseWriter, r *http.Request) {
	if !nsfwAccess(w, r) {
		return
	}
	hit, ok := h.nsfwResult(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "search again to refresh this release")
		return
	}
	var src domain.Source
	if err := database.Get(r.Context(), h.Store.Read(), "sources", hit.source.ID, &src); err != nil ||
		src.State == "disabled" || pausedNow(src, time.Now()) || src.Protocol != "torrent" || !src.UseFor.HandSearch ||
		!slices.Contains(src.Tags, "nsfw") || src.URL != hit.source.URL {
		webserver.Fail(w, http.StatusConflict, "search again to refresh this release's source")
		return
	}
	if h.QueueNSFW == nil || src.DownloaderID == nil {
		webserver.Fail(w, http.StatusConflict, "assign a qBittorrent downloader to this source")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	status, err := h.QueueNSFW(ctx, src, hit.item)
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]string{"status": status})
}
