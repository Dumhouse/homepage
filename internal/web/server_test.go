package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/web"
)

func TestHealthzAndPrototype(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	h, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{"/healthz": 200, "/design/dashboard.html": 404, "/assets/app.css": 200, "/nope": 404, "/dashboard.html": 404} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s: got %d want %d", path, rec.Code, want)
		}
	}
}
