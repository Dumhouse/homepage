package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFingerprintService(t *testing.T) {
	for _, tc := range []struct {
		title, banner string
		port          int
		want          string
	}{
		{"Proxmox Virtual Environment", "", 8006, "proxmox"},
		{"Grafana", "", 3000, "grafana"},
		{"", "SSH-2.0-OpenSSH_9.6", 22, "ssh"},
		{"", "220 mail.example ESMTP", 25, ""},
		{"hello", "", 12345, ""},
	} {
		if got := fingerprintService(tc.port, tc.title, tc.banner); got != tc.want {
			t.Errorf("fingerprintService(%d, %q, %q) = %q, want %q", tc.port, tc.title, tc.banner, got, tc.want)
		}
	}
}

func TestProbeAtMatchesMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"database":"ok","version":"11.2"}`))
	}))
	defer srv.Close()
	rule := probeRule{Service: "grafana", Path: "/api/health", Marker: `"database"`}
	if got := probeAt(context.Background(), srv.URL, rule); got != "grafana" {
		t.Fatalf("probe = %q", got)
	}
	rule.Marker = `"not-there"`
	if got := probeAt(context.Background(), srv.URL, rule); got != "" {
		t.Errorf("missing marker should not match, got %q", got)
	}
}

func TestProbeAtKeepsAuthErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"API running."}`))
	}))
	defer srv.Close()
	rule := probeRule{Service: "home-assistant", Path: "/api/", Marker: "API running"}
	if got := probeAt(context.Background(), srv.URL, rule); got != "home-assistant" {
		t.Fatalf("401 body should still identify an API, got %q", got)
	}
}

func TestProbeAtIgnoresServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"database":"broken"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	rule := probeRule{Service: "grafana", Path: "/api/health", Marker: `"database"`}
	if got := probeAt(context.Background(), srv.URL, rule); got != "" {
		t.Fatalf("500 should not identify a service, got %q", got)
	}
}
