package discovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestMurmur3Vectors(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int32
	}{
		{"", 0},
		{"foo", -156908512},
		{"hello", 613153351},
	} {
		if got := int32(murmur3([]byte(tc.in), 0)); got != tc.want {
			t.Errorf("murmur3(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFaviconHashMatchesEncodebytes(t *testing.T) {
	data := make([]byte, 0, 512)
	for i := 0; i < 2; i++ {
		for b := 0; b < 256; b++ {
			data = append(data, byte(b))
		}
	}
	if got := faviconHash(data); got != -1173581353 {
		t.Fatalf("faviconHash(512-byte vector) = %d", got)
	}
}

func TestFaviconServiceKnownHashes(t *testing.T) {
	for sum, want := range map[int32]string{
		2123863676: "grafana",
		712297537:  "portainer",
		167837543:  "nextcloud",
	} {
		if got := faviconService(sum); got != want {
			t.Errorf("faviconService(%d) = %q, want %q", sum, got, want)
		}
	}
}

func TestFaviconFingerprintFetchesAndMatches(t *testing.T) {
	icon := []byte("not a real icon, but enough bytes to hash")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/favicon.ico" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(icon)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portStr)
	old := faviconSums
	faviconSums = map[int32]string{faviconHash(icon): "test-service"}
	defer func() { faviconSums = old }()
	if got := faviconFingerprint(context.Background(), "http", host, port); got != "test-service" {
		t.Fatalf("favicon fingerprint = %q", got)
	}
}
