package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/request"
)

func proxyServe(t *testing.T, p auth.Provider, path, peer, user string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var actor string
	h := p.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor = request.Actor(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = peer
	if user != "" {
		req.Header.Set("Remote-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, actor
}

func TestProxyNeedsTrustedProxies(t *testing.T) {
	if _, err := auth.New(auth.Config{Mode: "proxy"}); err == nil {
		t.Fatal("proxy mode without HS_TRUSTED_PROXIES must fail")
	}
	if _, err := auth.New(auth.Config{Mode: "proxy", TrustedProxies: "not-an-ip"}); err == nil {
		t.Fatal("proxy mode with only invalid proxies must fail")
	}
}

func TestProxyTrustsHeaderOnlyFromProxy(t *testing.T) {
	p, err := auth.New(auth.Config{Mode: "proxy", TrustedProxies: "10.0.0.5, 192.168.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "proxy" {
		t.Fatalf("name = %q", p.Name())
	}
	// From the proxy, with a user: allowed, and the user becomes the audit actor.
	rec, actor := proxyServe(t, p, "/hosts", "10.0.0.5:4444", "alice")
	if rec.Code != http.StatusOK || actor != "ui:alice" {
		t.Fatalf("trusted proxy: code %d actor %q", rec.Code, actor)
	}
	rec, _ = proxyServe(t, p, "/", "192.168.1.77:1", "bob")
	if rec.Code != http.StatusOK {
		t.Fatalf("CIDR proxy: %d", rec.Code)
	}
	// Anyone else sending the header is refused.
	if rec, _ := proxyServe(t, p, "/hosts", "10.0.0.9:4444", "alice"); rec.Code != http.StatusForbidden {
		t.Fatalf("spoofed header: %d", rec.Code)
	}
	// The proxy without a user header is refused too.
	if rec, _ := proxyServe(t, p, "/hosts", "10.0.0.5:4444", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("no user: %d", rec.Code)
	}
	// Health and token-protected routes stay reachable.
	for _, path := range []string{"/healthz", "/mcp", "/api/v1/hosts"} {
		if rec, _ := proxyServe(t, p, path, "203.0.113.1:1", ""); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	sp := p.(auth.SessionProvider)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	r.RemoteAddr = "10.0.0.9:1"
	r.Header.Set("Remote-User", "alice")
	if sp.ValidSession(r) {
		t.Fatal("ValidSession must not trust an untrusted peer")
	}
}

func TestProxyAllowList(t *testing.T) {
	p, err := auth.New(auth.Config{Mode: "proxy", TrustedProxies: "10.0.0.5", ProxyUsers: "Alice, carol"})
	if err != nil {
		t.Fatal(err)
	}
	if rec, _ := proxyServe(t, p, "/", "10.0.0.5:1", "alice"); rec.Code != http.StatusOK {
		t.Fatalf("allowed user (case-insensitive): %d", rec.Code)
	}
	if rec, _ := proxyServe(t, p, "/", "10.0.0.5:1", "mallory"); rec.Code != http.StatusForbidden {
		t.Fatalf("user not on the list: %d", rec.Code)
	}
}

func TestProxyCustomHeader(t *testing.T) {
	p, err := auth.New(auth.Config{Mode: "proxy", TrustedProxies: "::1", ProxyHeader: "X-Forwarded-User"})
	if err != nil {
		t.Fatal(err)
	}
	h := p.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:9"
	req.Header.Set("X-Forwarded-User", "dana")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("custom header: %d", rec.Code)
	}
}
