package auth_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("secret")) })

func TestNoneLetsEveryoneIn(t *testing.T) {
	p, _ := auth.New(auth.Config{})
	rec := httptest.NewRecorder()
	p.Wrap(ok).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Body.String() != "secret" {
		t.Fatal("none should pass through")
	}
}

func TestPasswordFlow(t *testing.T) {
	hash, _ := auth.HashPassword("hunter2")
	p, err := auth.New(auth.Config{Mode: "password", PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	h := p.Wrap(ok)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 303 {
		t.Fatalf("anonymous should redirect, got %d", rec.Code)
	}
	for _, open := range []string{"/healthz", "/mcp"} {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", open, nil))
		if rec.Code != 200 {
			t.Errorf("%s should stay open, got %d", open, rec.Code)
		}
	}

	bad := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {"nope"}}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("wrong password must not set a session")
	}

	good := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {"hunter2"}}.Encode()))
	good.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, good)
	cs := rec.Result().Cookies()
	if len(cs) != 1 {
		t.Fatal("right password should set a session cookie")
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cs[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "secret" {
		t.Fatal("valid session should pass")
	}
	cs[0].Value = "1.tampered"
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cs[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 303 {
		t.Fatal("forged cookie must be rejected")
	}
}

func TestPasswordNeedsHash(t *testing.T) {
	if _, err := auth.New(auth.Config{Mode: "password"}); err == nil {
		t.Fatal("expected error without hash")
	}
	if _, err := auth.New(auth.Config{Mode: "bogus"}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestLoginRateLimit(t *testing.T) {
	hash, _ := auth.HashPassword("hunter2")
	p, err := auth.New(auth.Config{Mode: "password", PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	h := p.Wrap(ok)
	post := func(pw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {pw}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := 0; i < 5; i++ {
		if rec := post("nope"); rec.Code != 200 {
			t.Fatalf("attempt %d: %d", i+1, rec.Code)
		}
	}
	rec := post("nope")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt should be 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "300" {
		t.Errorf("Retry-After = %q, want 300", got)
	}
	if rec := post("hunter2"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("correct password should stay locked out, got %d", rec.Code)
	}
}
