package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/settings"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.2", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.2.0", "v0.3.0", false},
		{"v1.0.0", "v0.99.9", true},
		{"v0.2.1", "v0.2.0-4-gabcdef", true},
		{"v0.2.0", "v0.2.0-4-gabcdef", false},
		{"v0.2.0", "v0.2.0-4-gabcdef-dirty", false},
		{"v0.2.0", "dev", false},
		{"v0.2.0", "abc1234", false},
		{"garbage", "v0.1.0", false},
		{"v0.2", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func testStore(t *testing.T) *settings.Store {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return settings.New(d)
}

func TestCheckAndAvailable(t *testing.T) {
	ctx := context.Background()
	set := testStore(t)
	t.Setenv("HS_UPDATE_CHECK", "")
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Write([]byte(`{"tag_name":"v0.9.0","html_url":"https://example.com/r/v0.9.0","body":"x"}`))
	}))
	defer srv.Close()

	if Enabled(ctx, set) {
		t.Fatal("the update check must be off by default")
	}
	if !due(ctx, set, time.Now()) {
		t.Fatal("never checked means due")
	}
	r, err := Check(ctx, set, srv.Client(), srv.URL)
	if err != nil || r.Tag != "v0.9.0" || ua != "homeservices-update-check" {
		t.Fatalf("check: %+v %v ua=%q", r, err, ua)
	}
	if due(ctx, set, time.Now()) {
		t.Fatal("just checked")
	}
	if tag, _ := Available(ctx, set, "v0.2.0"); tag != "" {
		t.Fatal("check is off: nothing to announce")
	}
	_ = set.Set(ctx, "t", KeyEnabled, "on")
	tag, u := Available(ctx, set, "v0.2.0")
	if tag != "v0.9.0" || u != "https://example.com/r/v0.9.0" {
		t.Fatalf("available: %q %q", tag, u)
	}
	if tag, _ := Available(ctx, set, "v0.9.0"); tag != "" {
		t.Fatal("up to date")
	}
	t.Setenv("HS_UPDATE_CHECK", "off")
	if Enabled(ctx, set) {
		t.Fatal("HS_UPDATE_CHECK=off wins over the setting")
	}
	t.Setenv("HS_UPDATE_CHECK", "on")
	_ = set.Set(ctx, "t", KeyEnabled, "off")
	if !Enabled(ctx, set) {
		t.Fatal("HS_UPDATE_CHECK=on wins over the setting")
	}
}

func TestCheckErrors(t *testing.T) {
	ctx := context.Background()
	set := testStore(t)
	for _, body := range []struct {
		code int
		text string
	}{{404, `{}`}, {200, `{"tag_name":""}`}, {200, `not json`}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(body.code)
			w.Write([]byte(body.text))
		}))
		if _, err := Check(ctx, set, srv.Client(), srv.URL); err == nil {
			t.Fatalf("%d %q should fail", body.code, body.text)
		}
		srv.Close()
		if msg, _ := set.Get(ctx, KeyError, ""); msg == "" {
			t.Fatal("the error should be stored for the settings page")
		}
	}
}
