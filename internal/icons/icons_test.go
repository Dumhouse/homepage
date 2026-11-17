package icons_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/icons"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/settings"
)

func TestFetchCachesAndFallsBack(t *testing.T) {
	ctx := context.Background()
	requests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`<html><head><link rel="shortcut icon" href="/logo.png"></head></html>`))
	})
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 'P', 'N', 'G', 0, 0})
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>no icon</html>`))
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write([]byte{0, 0, 1, 0})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	f := icons.New(dir, nil)
	if _, err := f.Fetch(ctx, srv.URL, false); !errors.Is(err, icons.ErrPrivate) {
		t.Fatalf("private host should be refused, got %v", err)
	}
	path, err := f.Fetch(ctx, srv.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, "/icons/") || !strings.HasSuffix(path, ".png") {
		t.Fatalf("path: %q", path)
	}
	if _, err := os.Stat(filepath.Join(dir, strings.TrimPrefix(path, "/icons/"))); err != nil {
		t.Errorf("icon file: %v", err)
	}
	before := requests
	if again, _ := f.Fetch(ctx, srv.URL, true); again != path {
		t.Errorf("cache miss: %q vs %q", again, path)
	}
	if requests != before {
		t.Errorf("cache should avoid requests: %d → %d", before, requests)
	}
	fresh := icons.New(t.TempDir(), nil)
	if fallback, err := fresh.Fetch(ctx, srv.URL+"/plain", true); err != nil || !strings.HasSuffix(fallback, ".ico") {
		t.Errorf("favicon.ico fallback: %q %v", fallback, err)
	}
}

func TestFetchRejectsNonImageAndLarge(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Write([]byte(`<html></html>`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("nope"))
	}))
	defer srv.Close()
	f := icons.New(t.TempDir(), nil)
	if _, err := f.Fetch(ctx, srv.URL+"/page", true); err == nil {
		t.Error("html favicon should fail")
	}

	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Write([]byte(`<html></html>`))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(make([]byte, f.MaxBytes+1))
	}))
	defer big.Close()
	if _, err := f.Fetch(ctx, big.URL, true); !errors.Is(err, icons.ErrTooLarge) {
		t.Errorf("oversized icon: %v", err)
	}
}

func TestForService(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	set := settings.New(d)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte{0, 0, 1, 0})
			return
		}
		w.Write([]byte(`<html><head><link rel="icon" href="/favicon.ico"></head></html>`))
	}))
	defer srv.Close()
	hostName, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	h, _ := inv.CreateHost(ctx, "test", inventory.Host{Name: "acme", Addresses: []inventory.Address{{Address: hostName}}})
	portNum, _ := strconv.Atoi(port)
	svc, _ := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: portNum, Protocol: "http"})
	svc.Domain = hostName + ":" + port

	f := icons.New(t.TempDir(), set)
	if _, err := f.ForService(ctx, inv, svc); !errors.Is(err, icons.ErrPrivate) {
		t.Fatalf("private by default: %v", err)
	}
	if err := set.Set(ctx, "ui", "favicon_private", "on"); err != nil {
		t.Fatal(err)
	}
	icon, err := f.ForService(ctx, inv, svc)
	if err != nil || icon == "" {
		t.Fatalf("fetch: %v %q", err, icon)
	}
	got, _ := inv.GetService(ctx, svc.ID)
	if got.Icon != icon {
		t.Errorf("service icon not saved: %+v", got)
	}
	if err := set.Set(ctx, "ui", "favicon_fetch", "off"); err != nil {
		t.Fatal(err)
	}
	svc2 := *got
	if _, err := f.ForService(ctx, inv, &svc2); !errors.Is(err, icons.ErrDisabled) {
		t.Errorf("opt-out: %v", err)
	}
}
