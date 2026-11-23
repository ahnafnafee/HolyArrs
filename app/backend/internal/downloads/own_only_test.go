package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// With "Show only HolyArrs items" on, another app's torrents on the same
// qBittorrent are hidden and every single or bulk change to them is refused;
// with it off they show and can be changed as before.
func TestOwnOnlyHidesAndRefusesOtherApps(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, outsideTorrents(3, 0)).URL)
	feedStore(t, x.dl)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	theirs := fmt.Sprintf("%040x", 1)
	item := fmt.Sprintf("%s/api/v1/downloaders/%d/items/%s/", x.srv.URL, id, theirs)
	bulk := fmt.Sprintf("%s/api/v1/downloaders/%d/bulk", x.srv.URL, id)
	body := `{"action":"pause","hashes":["` + theirs + `"]}`

	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); v.OutsideTotal != 3 {
		t.Fatalf("switch off: outside %d", v.OutsideTotal)
	}
	do(t, "POST", item+"pause", "", 200)
	do(t, "POST", bulk, body, 200)

	exec41(t, x.s, "UPDATE downloaders SET own_only = 1")
	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); v.OutsideTotal != 0 || len(v.Downloads) != 1 {
		t.Fatalf("switch on: outside %d, rows %d", v.OutsideTotal, len(v.Downloads))
	}
	for _, act := range []string{"pause", "resume", "recheck", "reannounce", "top"} {
		do(t, "POST", item+act, "", 403)
	}
	do(t, "POST", item+"remove", `{"delete_files":false}`, 403)
	do(t, "POST", bulk, body, 403)
	do(t, "POST", bulk, `{"action":"remove","hashes":["`+theirs+`"]}`, 403)
	do(t, "POST", fmt.Sprintf("%s/api/v1/downloaders/%d/speed-limits", x.srv.URL, id), `{"hashes":["`+theirs+`"],"down":0,"up":0}`, 403)
}

func TestStandaloneSearchDownloadVisibility(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, "UPDATE downloaders SET own_only = 1, category = 'holyarrs'")
	var id int64
	if err := x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	x.dl.DLSync = &Sync{Store: &Store{}, DB: x.s}
	private := Job{ID: fmt.Sprintf("%040x", 2), Name: "Private release", Category: standaloneSearchCategory, State: StateQueued}
	other := Job{ID: fmt.Sprintf("%040x", 3), Name: "Other app", Category: "other", State: StateQueued}
	x.dl.DLSync.Store.Update(id, 1, []Job{private, other}, time.Now())
	for _, test := range []struct {
		name    string
		caller  webserver.Caller
		allowed bool
	}{
		{"owner", webserver.Caller{Via: "session", Role: webserver.RoleOwner}, true},
		{"manager", webserver.Caller{Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}}, false},
		{"rating-limited owner", webserver.Caller{Via: "session", Role: webserver.RoleOwner, MaxRating: "PG"}, false},
		{"network sign-in", webserver.Caller{Via: "local_network", Role: webserver.RoleOwner}, false},
		{"read key", webserver.Caller{Via: "api_key", Role: webserver.RoleOwner, Scopes: []string{webserver.ScopeRead}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := webserver.ContextWith(context.Background(), test.caller)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/view", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			x.dl.DownloadsView(w, r)
			var response struct{ Downloads []DownloadRow }
			if err := json.Unmarshal(w.Body.Bytes(), &response); w.Code != http.StatusOK || err != nil {
				t.Fatalf("Downloads view returned %d: %v", w.Code, err)
			}
			found := false
			for _, row := range response.Downloads {
				if row.Hash == other.ID {
					t.Fatal("Downloads view included an unrelated category")
				}
				found = found || row.Hash == private.ID
			}
			if found != test.allowed || x.dl.notOurs(ctx, id, []string{private.ID}) == test.allowed {
				t.Fatalf("standalone download visibility or controls differ from access permission")
			}
		})
	}
	if outcome, _, err := x.dl.MatchLinkOnly(context.Background(), id, private.ID, private.Name, private.Category); outcome != matchOtherApp || err != nil {
		t.Fatalf("standalone download entered title matching: %q %v", outcome, err)
	}
}
