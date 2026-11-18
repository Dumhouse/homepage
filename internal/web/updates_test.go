package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
)

func TestUpdatesSettings(t *testing.T) {
	t.Setenv("HS_UPDATE_CHECK", "")
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://example.com/releases/v9.0.0"}`))
	}))
	defer fake.Close()
	oldEP, oldV := updateEndpoint, Version
	updateEndpoint, Version = fake.URL, "v0.2.0"
	defer func() { updateEndpoint, Version = oldEP, oldV }()

	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	h, err := New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if method == "POST" {
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := do("GET", "/settings/updates", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Never checked") || !strings.Contains(rec.Body.String(), "You are running v0.2.0") {
		t.Fatalf("updates page: %d", rec.Code)
	}
	// Check while off: refused politely, nothing fetched.
	rec = do("POST", "/settings/updates", url.Values{"check": {"1"}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, url.QueryEscape("Turn the update check on first.")) {
		t.Fatalf("check while off went to %q", loc)
	}
	rec = do("POST", "/settings/updates", url.Values{"update_check": {"on"}, "check": {"1"}})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, url.QueryEscape("v9.0.0 is available.")) {
		t.Fatalf("check went to %q", loc)
	}
	body := do("GET", "/", nil).Body.String()
	if !strings.Contains(body, "v9.0.0 available") || !strings.Contains(body, "Home Services v9.0.0 is available") {
		t.Fatal("sidebar and bell should announce the new release")
	}
	if rec := do("POST", "/settings/updates", url.Values{"update_check": {"maybe"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad value: %d", rec.Code)
	}
	do("POST", "/settings/updates", url.Values{"update_check": {"off"}})
	if strings.Contains(do("GET", "/", nil).Body.String(), "v9.0.0 available") {
		t.Fatal("switched off: no announcement")
	}
}
