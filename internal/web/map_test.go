package web_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/demo"
	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestDependencyGraph(t *testing.T) {
	_, inv := newTestServer(t)
	ctx := context.Background()
	loc, err := inv.CreateLocation(ctx, "t", inventory.Location{Name: "Rack A", Kind: "other"})
	if err != nil {
		t.Fatal(err)
	}
	h, _ := inv.CreateHost(ctx, "t", inventory.Host{Name: "pve-1", Kind: "bare", LocationID: &loc.ID})
	gone, _ := inv.CreateHost(ctx, "t", inventory.Host{Name: "old", Kind: "vm", Lifecycle: "decommissioned"})
	pg, err := inv.AddService(ctx, "t", h.ID, inventory.Service{Name: "postgres", Port: 5432})
	if err != nil {
		t.Fatal(err)
	}
	p, err := inv.CreateProject(ctx, "t", inventory.Project{Name: "Shop", Slug: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.CreateConnection(ctx, "t", inventory.Connection{ProjectID: p.ID, TargetKind: "service", TargetID: &pg.ID, Role: "storage"}); err != nil {
		t.Fatal(err)
	}
	g, err := inv.DependencyGraph(ctx)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, n := range g.Nodes {
		keys[n.Key] = true
	}
	if keys["h:"+itoa(gone.ID)] {
		t.Fatal("decommissioned host must not be on the map")
	}
	want := []inventory.GraphEdge{
		{From: "p:" + itoa(p.ID), To: "s:" + itoa(pg.ID), Role: "storage"},
		{From: "s:" + itoa(pg.ID), To: "h:" + itoa(h.ID)},
		{From: "h:" + itoa(h.ID), To: "l:" + itoa(loc.ID)},
	}
	for _, w := range want {
		found := false
		for _, e := range g.Edges {
			if e == w {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing edge %+v in %+v", w, g.Edges)
		}
	}
	// Location down → host, service and project are all affected.
	impact := g.Impact("l:" + itoa(loc.ID))
	if strings.Join(impact, ",") != "h:"+itoa(h.ID)+",s:"+itoa(pg.ID)+",p:"+itoa(p.ID) {
		t.Fatalf("impact = %v", impact)
	}
	if len(g.Impact("p:"+itoa(p.ID))) != 0 {
		t.Fatal("nothing depends on a project")
	}
}

func TestMapPage(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	code, body := page(t, srv, "/map")
	if code != 200 || !strings.Contains(body, "No dependencies yet") {
		t.Fatalf("empty map: %d", code)
	}
	if err := demo.Seed(ctx, inv); err != nil {
		t.Fatal(err)
	}
	code, body = page(t, srv, "/map")
	if code != 200 || !strings.Contains(body, `id="depmap"`) || !strings.Contains(body, `data-deps="`) {
		t.Fatalf("map: %d", code)
	}
	if !strings.Contains(body, `href="/map"`) {
		t.Fatal("sidebar should link to the map")
	}
	g, _ := inv.DependencyGraph(ctx)
	// Pick the host with the widest impact and focus it.
	best, n := "", 0
	for _, node := range g.Nodes {
		if node.Kind == "host" {
			if k := len(g.Impact(node.Key)); k > n {
				best, n = node.Key, k
			}
		}
	}
	if best == "" {
		t.Fatal("demo should have a host something depends on")
	}
	code, body = page(t, srv, "/map?focus="+best)
	if code != 200 || !strings.Contains(body, "goes down") || !strings.Contains(body, "map-node host focus") {
		t.Fatalf("focus: %d", code)
	}
	if strings.Contains(body, "Nothing in the inventory depends on it.") {
		t.Fatal("focused host has dependants")
	}
	code, body = page(t, srv, "/map?all=1")
	if code != 200 || !strings.Contains(body, "Only linked") {
		t.Fatalf("all: %d", code)
	}
}
