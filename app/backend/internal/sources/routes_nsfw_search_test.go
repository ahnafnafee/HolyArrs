package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestNSFWSearchUsesTaggedSourcesAndOmitsLinks(t *testing.T) {
	t.Setenv("HOLYARRS_PRIVATE_RELEASE_SEARCH", "1")
	regular := &liveFeed{}
	regular.set(duneFeed)
	f := seed(t, regular.url(t))
	tagged := &liveFeed{}
	tagged.set(strings.Replace(duneFeed, "</title>", `</title><comments>https://source.example/release?apikey=private</comments><torznab:attr name="coverurl" value="https://images.example/cover.jpg" />`, 1))
	id := addSource(t, f.s, "Tagged", tagged.url(t), 25, "enabled")
	addSource(t, f.s, "Paused", regular.url(t), 25, "paused")
	exec(t, f, `UPDATE sources SET tags = '["nsfw"]', categories = '{"movie":[6000]}' WHERE id = ? OR name = 'Paused'`, id)
	searcher := &Searcher{Store: f.s}
	h := &Handlers{Store: f.s, Search: func() *Searcher { return searcher }}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/nsfw-search", strings.NewReader(`{"query":"Dune 1080p"}`))
	req.Header.Set("Content-Type", "application/json")
	reply := httptest.NewRecorder()
	h.searchNSFW(reply, req)

	var got nsfwSearchAnswer
	if err := json.Unmarshal(reply.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if reply.Code != http.StatusOK || len(got.Releases) != 1 || got.Releases[0].Source != "Tagged" {
		t.Fatalf("search returned %d: %s", reply.Code, reply.Body.String())
	}
	release := got.Releases[0]
	if release.ID == "" || !release.HasThumbnail || release.DetailsURL != "https://source.example/release" {
		t.Fatalf("unexpected release metadata: %+v", release)
	}
	download := func(caller webserver.Caller) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/sources/nsfw-search/token/download", nil)
		r.SetPathValue("token", release.ID)
		r = r.WithContext(webserver.ContextWith(r.Context(), caller))
		w := httptest.NewRecorder()
		h.downloadNSFW(w, r)
		return w
	}
	h.QueueNSFW = func(_ context.Context, src domain.Source, item Item) (string, error) {
		if src.ID != id || item.Link == "" || item.Title != release.Name {
			t.Fatal("queue did not receive the selected feed result")
		}
		return "added", nil
	}
	if w := download(webserver.Caller{UserID: 2, Via: "session"}); w.Code != http.StatusNotFound {
		t.Fatalf("another account can retrieve the result: %d", w.Code)
	}
	if w := download(webserver.Caller{UserID: 1, Via: "session", MaxRating: "PG"}); w.Code != http.StatusForbidden {
		t.Fatalf("rating-limited account can retrieve the result: %d", w.Code)
	}
	if w := download(webserver.Caller{UserID: 1, Via: "session"}); w.Code != http.StatusConflict {
		t.Fatalf("unassigned source did not fail: %d", w.Code)
	}
	exec(t, f, `UPDATE sources SET downloader_id = 1 WHERE id = ?`, id)
	if w := download(webserver.Caller{UserID: 1, Via: "session"}); w.Code != http.StatusOK {
		t.Fatalf("selected release did not reach the queue: %d %s", w.Code, w.Body.String())
	}
	if feedHits(tagged) != 1 || feedHits(regular) != 0 {
		t.Fatalf("tagged source hits %d, untagged or paused source hits %d", feedHits(tagged), feedHits(regular))
	}
	if len(got.Sources) != 2 || got.Sources[0].Status != "answered" || got.Sources[1].Status != "paused" {
		t.Fatalf("unexpected source status: %+v", got.Sources)
	}
	if strings.Contains(reply.Body.String(), ".torrent") || strings.Contains(reply.Body.String(), "private") || reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("search exposed download links or permitted caching: %s", reply.Body.String())
	}
}

func TestNSFWSearchRefusesRatingLimitedAccount(t *testing.T) {
	t.Setenv("HOLYARRS_PRIVATE_RELEASE_SEARCH", "1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/nsfw-search", strings.NewReader(`{"query":"Dune"}`))
	req = req.WithContext(webserver.ContextWith(req.Context(), webserver.Caller{Via: "session", MaxRating: "PG"}))
	reply := httptest.NewRecorder()
	(&Handlers{}).searchNSFW(reply, req)
	if reply.Code != http.StatusForbidden {
		t.Fatalf("got HTTP %d, want 403", reply.Code)
	}
}

func TestPrivateReleaseRoutesDisabledByDefault(t *testing.T) {
	t.Setenv("HOLYARRS_PRIVATE_RELEASE_SEARCH", "")
	h := &Handlers{}
	for _, f := range []http.HandlerFunc{h.searchNSFW, h.nsfwThumbnail, h.downloadNSFW} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Via: "session"}))
		f(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("disabled route returned HTTP %d, want 404", w.Code)
		}
	}
}

func TestNSFWReleasePageCover(t *testing.T) {
	page := `<img src="https://ads.example/ad.jpg"><table id="details">
<tr><td></td><td><img border=0 src="https://images.example/cover.jpg">`
	match := xxxtorCover.FindStringSubmatch(page)
	if len(match) != 2 || match[1] != "https://images.example/cover.jpg" {
		t.Fatalf("could not pick the release cover: %v", match)
	}
	if xxxtorCover.MatchString(`<table id="details"><tr><td>Info</td><td><img src="https://ads.example/ad.jpg">`) {
		t.Fatal("unrelated image selected as a release cover")
	}
}
