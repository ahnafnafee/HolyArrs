package sources

import (
	"cmp"
	"context"
	"crypto/rand"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const nsfwResultsLimit = 100

type nsfwRelease struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Source       string    `json:"source"`
	Size         int64     `json:"size"`
	Seeders      *int      `json:"seeders"`
	Peers        *int      `json:"peers"`
	Published    time.Time `json:"published,omitzero"`
	DetailsURL   string    `json:"detailsUrl,omitempty"`
	HasThumbnail bool      `json:"hasThumbnail"`
	CanDownload  bool      `json:"canDownload"`
}

type nsfwHit struct {
	source domain.Source
	item   Item
}

type nsfwSearch struct {
	expires time.Time
	hits    map[string]nsfwHit
}

type nsfwSource struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type nsfwSearchAnswer struct {
	Releases []nsfwRelease `json:"releases"`
	Sources  []nsfwSource  `json:"sources"`
}

// nsfwAccess keeps search results and related requests behind the same checks.
func nsfwAccess(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if !parts.PrivateReleaseSearchEnabled() {
		webserver.Fail(w, http.StatusNotFound, "no such route")
		return false
	}
	if !webserver.RequireStrong(w, r) {
		return false
	}
	if caller, ok := webserver.From(r.Context()); ok && caller.MaxRating != "" {
		webserver.Fail(w, http.StatusForbidden, "this account cannot access NSFW releases")
		return false
	}
	return true
}

func (h *Handlers) searchNSFW(w http.ResponseWriter, r *http.Request) {
	if !nsfwAccess(w, r) {
		return
	}
	var in struct {
		Query string `json:"query"`
	}
	if !webserver.Decode(w, r, &in, "send a release query") {
		return
	}
	if len([]rune(in.Query)) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "release query is too long")
		return
	}
	q := textQuery(Query{Title: in.Query})
	if q == "" {
		webserver.Fail(w, http.StatusBadRequest, "enter a release query")
		return
	}
	srcs, err := database.Select[domain.Source](r.Context(), h.Store.Read(), "sources",
		"state != 'disabled' AND protocol = 'torrent' AND format != 'rss' AND EXISTS (SELECT 1 FROM json_each(tags) WHERE value = 'nsfw') ORDER BY priority, id")
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	srcs = slices.DeleteFunc(srcs, func(src domain.Source) bool {
		return !src.UseFor.HandSearch || (!handlesTV(src) && !handlesMovies(src))
	})
	out := nsfwSearchAnswer{Releases: []nsfwRelease{}, Sources: make([]nsfwSource, len(srcs))}
	results := make([][]nsfwHit, len(srcs))
	words := nameWords(q)
	now := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i, src := range srcs {
		out.Sources[i] = nsfwSource{Name: src.Name, Status: "unavailable"}
		if pausedNow(src, now) {
			out.Sources[i].Status = "paused"
			continue
		}
		wg.Go(func() {
			cl, err := h.search().client(ctx, src)
			if err != nil {
				return
			}
			categories := append(slices.Clone(src.Categories["movie"]), src.Categories["tv"]...)
			slices.Sort(categories)
			categories = slices.Compact(categories)
			if len(categories) == 0 {
				categories = []int{6000}
			}
			items, err := cl.Search(ctx, Query{Title: q, Categories: categories, TV: !handlesMovies(src)})
			if err != nil {
				return
			}
			out.Sources[i].Status = "answered"
			for _, item := range items {
				if item.Protocol == "torrent" && namesAll(item.Title, words) {
					results[i] = append(results[i], nsfwHit{src, item})
				}
			}
		})
	}
	wg.Wait()
	hits := slices.Concat(results...)
	slices.SortStableFunc(hits, func(a, b nsfwHit) int {
		return cmp.Compare(knownSeeders(b.item.Seeders), knownSeeders(a.item.Seeders))
	})
	search := nsfwSearch{expires: time.Now().Add(20 * time.Minute), hits: make(map[string]nsfwHit)}
	for _, hit := range hits[:min(len(hits), nsfwResultsLimit)] {
		id := rand.Text()
		search.hits[id] = hit
		item := hit.item
		var details string
		if u := nsfwWebURL(item.DetailsURL); u != nil {
			// Feed URLs may contain access tokens. Only share the public page path.
			u.RawQuery, u.Fragment, u.ForceQuery = "", "", false
			if srcURL, err := url.Parse(hit.source.URL); err == nil && srcURL.Hostname() != u.Hostname() {
				details = u.String()
			}
		}
		out.Releases = append(out.Releases, nsfwRelease{
			ID: id, Name: item.Title, Source: hit.source.Name, Size: item.Size, Seeders: item.Seeders,
			Peers: item.Peers, Published: item.Published, DetailsURL: details,
			HasThumbnail: nsfwWebURL(item.CoverURL) != nil || xxxtorReleasePage(item) != nil, CanDownload: hit.source.DownloaderID != nil,
		})
	}
	h.keepNSFWSearch(webserver.RequestActor(r).ID, search)
	webserver.WriteJSON(w, http.StatusOK, out)
}

func nsfwWebURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return nil
	}
	return u
}

// Retain only the latest search per account. Tokens keep Source links on the server.
func (h *Handlers) keepNSFWSearch(userID int64, search nsfwSearch) {
	h.nsfwMu.Lock()
	defer h.nsfwMu.Unlock()
	if h.nsfwSearches == nil {
		h.nsfwSearches = make(map[int64]nsfwSearch)
	}
	now := time.Now()
	maps.DeleteFunc(h.nsfwSearches, func(_ int64, old nsfwSearch) bool { return now.After(old.expires) })
	delete(h.nsfwSearches, userID)
	if len(h.nsfwSearches) >= 32 {
		oldest := slices.MinFunc(slices.Collect(maps.Keys(h.nsfwSearches)), func(a, b int64) int {
			return h.nsfwSearches[a].expires.Compare(h.nsfwSearches[b].expires)
		})
		delete(h.nsfwSearches, oldest)
	}
	h.nsfwSearches[userID] = search
}

func (h *Handlers) nsfwResult(r *http.Request) (nsfwHit, bool) {
	h.nsfwMu.Lock()
	defer h.nsfwMu.Unlock()
	search := h.nsfwSearches[webserver.RequestActor(r).ID]
	hit, ok := search.hits[r.PathValue("token")]
	return hit, ok && time.Now().Before(search.expires)
}

func knownSeeders(n *int) int {
	if n == nil {
		return -1
	}
	return *n
}
