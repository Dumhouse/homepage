package discovery_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dawidof/home-services/internal/discovery"
)

func TestScanLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Forgejo</title></head></html>`))
	}))
	defer srv.Close()
	httpPort := srv.Listener.Addr().(*net.TCPAddr).Port
	port1 := ln.Addr().(*net.TCPAddr).Port

	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	s := discovery.New()
	s.Workers = 4
	s.Timeout = 200 * time.Millisecond
	s.Ports = []int{port1, httpPort, closedPort}
	s.WebPorts = map[int]bool{httpPort: true}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := s.Scan(ctx, "127.0.0.1/32")
	if err != nil || len(res) != 1 {
		t.Fatalf("scan: %v %+v", err, res)
	}
	if len(res[0].Ports) != 2 {
		t.Fatalf("open ports: %+v", res[0].Ports)
	}
	found := false
	for _, p := range res[0].Ports {
		if p.Port == httpPort && p.Title == "Forgejo" && p.Service == "forgejo" {
			found = true
		}
	}
	if !found {
		t.Errorf("http title missing: %+v", res[0].Ports)
	}
	if _, err := s.Scan(ctx, "nope"); err == nil {
		t.Error("bad subnet should fail")
	}
	if _, err := s.Scan(ctx, "10.0.0.0/8"); err == nil {
		t.Error("huge subnet should be refused")
	}
}
