package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

var echoRemote = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(r.RemoteAddr))
})

func serveThrough(trusted []*net.IPNet, remote string, headers map[string]string) string {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	realIP(trusted)(echoRemote).ServeHTTP(rec, req)
	return rec.Body.String()
}

func TestRealIPIgnoresSpoofedHeaderWithoutTrustedProxy(t *testing.T) {
	got := serveThrough(nil, "198.51.100.10:1234", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if got != "198.51.100.10:1234" {
		t.Fatalf("untrusted header changed RemoteAddr: %q", got)
	}
}

func TestRealIPUsesHeaderOnlyFromTrustedProxy(t *testing.T) {
	trusted := parseTrustedProxies("192.0.2.0/24")
	got := serveThrough(trusted, "192.0.2.5:1234", map[string]string{"X-Forwarded-For": "203.0.113.9, 192.0.2.5"})
	if got != "203.0.113.9" {
		t.Fatalf("trusted proxy XFF: got %q", got)
	}
	got = serveThrough(trusted, "198.51.100.10:1234", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if got != "198.51.100.10:1234" {
		t.Fatalf("untrusted peer was allowed to spoof: %q", got)
	}
}

func TestRealIPPrefersCloudflareAndRealIP(t *testing.T) {
	trusted := parseTrustedProxies("127.0.0.1")
	got := serveThrough(trusted, "127.0.0.1:9999", map[string]string{
		"CF-Connecting-IP": "203.0.113.7", "X-Forwarded-For": "198.51.100.1",
	})
	if got != "203.0.113.7" {
		t.Fatalf("CF-Connecting-IP: got %q", got)
	}
	got = serveThrough(trusted, "127.0.0.1:9999", map[string]string{"X-Real-IP": "203.0.113.8"})
	if got != "203.0.113.8" {
		t.Fatalf("X-Real-IP: got %q", got)
	}
}

func TestParseTrustedProxiesIgnoresGarbage(t *testing.T) {
	got := parseTrustedProxies("192.0.2.0/24, not-an-ip, 2001:db8::1, ,10.0.0.5")
	if len(got) != 3 {
		t.Fatalf("parsed %d entries: %+v", len(got), got)
	}
}
