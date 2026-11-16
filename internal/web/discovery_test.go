package web_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/discovery"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/settings"
	"github.com/dawidof/home-services/internal/web"
)

func TestDiscoveryPageAndAPI(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	if _, _, err := inv.UpsertDiscovery(ctx, "10.0.0.9", "acme", []inventory.PortInfo{
		{Port: 3000, Service: "app", Title: "Acme admin"},
	}); err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	code, body := page(t, srv, "/discovery")
	if code != 200 || !strings.Contains(body, "10.0.0.9") || !strings.Contains(body, `href="http://10.0.0.9:3000/"`) || !strings.Contains(body, "Acme admin") {
		t.Fatalf("discovery page: %d", code)
	}
	hosts, _ := inv.ListDiscovered(ctx, "new")
	if len(hosts) != 1 {
		t.Fatalf("seed: %+v", hosts)
	}
	if code, loc := redirect(t, srv, "/discovery/"+strconv.FormatInt(hosts[0].ID, 10)+"/register", map[string][]string{"name": {"acme-discovered"}}); code != 303 || !strings.Contains(loc, "/hosts/") {
		t.Fatalf("register: %d %s", code, loc)
	}
	_, body = page(t, srv, "/hosts/1")
	if !strings.Contains(body, "acme-discovered") || !strings.Contains(body, "3000") {
		t.Error("registered host/services missing")
	}

	// API scan against a local listener
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
	port := ln.Addr().(*net.TCPAddr).Port
	if err := settings.New(d).Set(ctx, "test", discovery.KeyPorts, strconv.Itoa(port)); err != nil {
		t.Fatal(err)
	}
	code, res := do(t, srv, "POST", "/api/v1/discovery/scan", `{"subnet":"127.0.0.1/32"}`)
	if code != 202 || res["running"] != true {
		t.Fatalf("scan start: %d %v", code, res)
	}
	done := false
	for i := 0; i < 80; i++ {
		_, st := do(t, srv, "GET", "/api/v1/discovery/status", "")
		if st["running"] == false {
			done = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !done {
		t.Fatal("scan did not finish")
	}
	code, list := doList(t, srv, "/api/v1/discovery?status=new")
	if code != 200 || len(list) == 0 {
		t.Fatalf("list discovery: %d %v", code, list)
	}
	var id string
	for _, item := range list {
		m := item.(map[string]any)
		if m["address"] == "127.0.0.1" {
			id = strconv.Itoa(int(m["id"].(float64)))
		}
	}
	if id == "" {
		t.Fatalf("loopback not discovered: %v", list)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/discovery/"+id+"/ignore", ""); code != 204 {
		t.Errorf("ignore: %d", code)
	}
	if code, list := doList(t, srv, "/api/v1/discovery?status=new"); code != 200 {
		t.Errorf("list after ignore: %d %v", code, list)
	}
}
