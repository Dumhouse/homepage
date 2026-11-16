package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/demo"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

var hrefRe = regexp.MustCompile(`href="(/[^"#]*)`)

// TestCrawlAllPages seeds the demo inventory and follows every local link from the home page.
// Every HTML page must answer 200 and render completely — this catches template errors such as a
// missing field, which used to produce a half-rendered page with status 200.
func TestCrawlAllPages(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := demo.Seed(context.Background(), inventory.New(d)); err != nil {
		t.Fatal(err)
	}
	h, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	queue := []string{"/", "/hosts/new", "/projects/new", "/domains/new"}
	for len(queue) > 0 && len(seen) < 300 {
		p := strings.ReplaceAll(queue[0], "&amp;", "&")
		queue = queue[1:]
		if seen[p] || skipCrawl(p) {
			continue
		}
		seen[p] = true
		code, body := page(t, h, p)
		if code != http.StatusOK {
			t.Errorf("GET %s = %d", p, code)
			continue
		}
		if !strings.Contains(body, "<html") {
			continue // CSV, JSON, Markdown, ssh config
		}
		if !strings.Contains(body, "</html>") {
			t.Errorf("GET %s: page is cut off (template error?)", p)
		}
		for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
			queue = append(queue, m[1])
		}
	}
	if len(seen) < 30 {
		t.Fatalf("crawl only reached %d pages — demo seed or links changed?", len(seen))
	}
	for _, must := range []string{"/hosts/1/edit", "/app-links/1/edit", "/access/1/edit", "/services/1/edit"} {
		if !seen[must] {
			t.Errorf("crawl never reached %s", must)
		}
	}
}

func skipCrawl(p string) bool {
	for _, pre := range []string{"/assets/", "/static/", "/design", "/icons/", "/favicon", "/logout"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	// links that change saved preferences are covered by their own tests
	return strings.Contains(p, "view=") || strings.Contains(p, "tile=")
}

// TestCrossSiteFormsRefused: with HS_AUTH=none any page on the internet could otherwise post the
// app's forms from the user's browser.
func TestCrossSiteFormsRefused(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	h, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	post := func(site, origin string) int {
		req := httptest.NewRequest("POST", "/hosts", strings.NewReader("name=x&kind=vm"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := post("cross-site", "https://evil.example"); c != http.StatusForbidden {
		t.Errorf("cross-site form POST = %d, want 403", c)
	}
	if c := post("", "https://evil.example"); c != http.StatusForbidden {
		t.Errorf("foreign Origin form POST = %d, want 403", c)
	}
	if c := post("same-origin", ""); c != http.StatusSeeOther {
		t.Errorf("same-origin form POST = %d, want 303", c)
	}
}

// TestAccessAddRedirectStaysLocal: a failed "add login" used to redirect to any URL in "back".
func TestAccessAddRedirectStaysLocal(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	h, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/hosts/1/access", strings.NewReader("method=bogus&back=https://evil.example/"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") {
		t.Fatalf("redirected to %q", loc)
	}
}
