package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

func TestLocations(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	home, err := s.CreateLocation(ctx, "test", inventory.Location{Name: "Proxmox .250", Kind: "proxmox", URL: "https://192.168.1.250"})
	if err != nil {
		t.Fatal(err)
	}
	ovh, err := s.CreateLocation(ctx, "test", inventory.Location{Name: "OVH"})
	if err != nil {
		t.Fatal(err)
	}
	if ovh.Kind != "other" {
		t.Errorf("kind should default: %+v", ovh)
	}
	h, err := s.CreateHost(ctx, "test", inventory.Host{Name: "acme", LocationID: &home.ID})
	if err != nil {
		t.Fatal(err)
	}
	if h.Location == nil || h.Location.Name != "Proxmox .250" {
		t.Fatalf("host should embed its location: %+v", h.Location)
	}
	got, err := s.GetLocation(ctx, home.ID)
	if err != nil || got.HostCount != 1 {
		t.Fatalf("host count: %v %+v", err, got)
	}
	if err := s.DeleteLocation(ctx, "test", home.ID); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("deleting a location in use should be invalid, got %v", err)
	}
	h.LocationID = &ovh.ID
	if _, err := s.UpdateHost(ctx, "test", h.ID, *h); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLocation(ctx, "test", home.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetLocation(ctx, home.ID); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("deleted location should be hidden, got %v", err)
	}
	if err := s.RestoreLocation(ctx, "test", home.ID); err != nil {
		t.Fatal(err)
	}

	// a host cannot point at a missing location
	bogus := int64(999)
	if _, err := s.CreateHost(ctx, "test", inventory.Host{Name: "x", LocationID: &bogus}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("unknown location should be invalid, got %v", err)
	}
	if _, err := s.CreateLocation(ctx, "test", inventory.Location{Name: "x", Kind: "toaster"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad kind should be invalid, got %v", err)
	}
	if _, err := s.CreateLocation(ctx, "test", inventory.Location{Name: "x", URL: "ftp://nope"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad url should be invalid, got %v", err)
	}
}

func TestTagsAndGroups(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, err := s.CreateHost(ctx, "test", inventory.Host{Name: "acme", Tags: []string{"Backend", " infra ", "backend", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Tags) != 2 || h.Tags[0] != "backend" || h.Tags[1] != "infra" {
		t.Errorf("tags should be normalized and deduped: %+v", h.Tags)
	}
	v, err := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "postgres", Tags: []string{"backend", "db"}})
	if err != nil || len(v.Tags) != 2 {
		t.Fatalf("service tags: %v %+v", err, v)
	}
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	usage := map[string]int{}
	for _, tag := range tags {
		usage[tag.Name] = tag.Usage
	}
	if usage["backend"] != 2 || usage["db"] != 1 || usage["infra"] != 1 {
		t.Errorf("usage counts wrong: %+v", tags)
	}

	p, err := s.CreateProject(ctx, "test", inventory.Project{Name: "Acme", Group: "Clients"})
	if err != nil || p.Group != "Clients" || p.GroupID == nil {
		t.Fatalf("group should be created by name: %v %+v", err, p)
	}
	groups, err := s.ListGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Name != "Clients" {
		t.Fatalf("groups: %v %+v", err, groups)
	}
	p.Group = ""
	p.GroupID = &groups[0].ID
	up, err := s.UpdateProject(ctx, "test", p.ID, *p)
	if err != nil || up.Group != "Clients" {
		t.Fatalf("group by id: %v %+v", err, up)
	}
	bad := int64(999)
	p.GroupID = &bad
	if _, err := s.UpdateProject(ctx, "test", p.ID, *p); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("unknown group should be invalid, got %v", err)
	}
}

func TestProjectChildren(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	h, _ := s.CreateHost(ctx, "test", inventory.Host{Name: "acme-prod"})
	v, _ := s.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http"})
	p, err := s.CreateProject(ctx, "test", inventory.Project{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddProjectLink(ctx, "test", p.ID, inventory.ProjectLink{Kind: "repo", URL: "https://git.example.com/acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProjectLink(ctx, "test", p.ID, inventory.ProjectLink{Kind: "other", URL: "nope"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("relative project link should be invalid, got %v", err)
	}

	prod, err := s.AddProjectEnv(ctx, "test", p.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com/", Sort: 1})
	if err != nil {
		t.Fatal(err)
	}
	staging, err := s.AddProjectEnv(ctx, "test", p.ID, inventory.ProjectEnv{Name: "staging", BaseURL: "https://staging.acme.example.com", Sort: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProjectEnv(ctx, "test", p.ID, inventory.ProjectEnv{Name: "production"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("duplicate env should be invalid, got %v", err)
	}
	if _, err := s.AddProjectEnv(ctx, "test", p.ID, inventory.ProjectEnv{Name: "x", BaseURL: "acme.example.com"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad base url should be invalid, got %v", err)
	}

	if _, err := s.AddAppLink(ctx, "test", p.ID, inventory.AppLink{Label: "Admin", Path: "admin", Kind: "admin", Sort: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAppLink(ctx, "test", p.ID, inventory.AppLink{Label: "Mailpit", Path: "/mailpit", Kind: "other", OnlyEnv: "staging"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAppLink(ctx, "test", p.ID, inventory.AppLink{Label: "External", OverrideURL: "https://status.example.com", Kind: "health"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAppLink(ctx, "test", p.ID, inventory.AppLink{Label: "nope", Kind: "page"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("link without path/url should be invalid, got %v", err)
	}

	access, err := s.CreateAccess(ctx, "test", inventory.Access{
		TargetEntity: "project", TargetID: p.ID, Label: "Admin", Method: "password",
		VaultItemURL: "https://vault.example.com/item/1", TwoFA: "totp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccess(ctx, "test", inventory.Access{TargetEntity: "project", TargetID: p.ID, Label: "x", VaultItemURL: "javascript:alert(1)"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("non-http vault url should be invalid, got %v", err)
	}
	if _, err := s.CreateAccess(ctx, "test", inventory.Access{TargetEntity: "project", TargetID: 999, Label: "x"}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("unknown target should be invalid, got %v", err)
	}

	if err := s.LinkProjectHost(ctx, "test", p.ID, h.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkProjectService(ctx, "test", p.ID, v.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkProjectHost(ctx, "test", p.ID, 999, "production"); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("unknown host should be not found, got %v", err)
	}

	if _, err := s.CreateConnection(ctx, "test", inventory.Connection{
		ProjectID: p.ID, TargetKind: "url", URL: "https://forgejo.example.com", Role: "code",
		Detail: "code + CI", ImpactNote: "no deploys",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConnection(ctx, "test", inventory.Connection{
		ProjectID: p.ID, TargetKind: "service", TargetID: &v.ID, Role: "secrets", AccessID: &access.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConnection(ctx, "test", inventory.Connection{
		ProjectID: p.ID, TargetKind: "service", Role: "code",
	}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("connection without target should be invalid, got %v", err)
	}

	got, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Links) != 1 || len(got.Envs) != 2 || len(got.AppLinks) != 3 {
		t.Fatalf("children: %+v", got)
	}
	if len(got.Hosts) != 1 || got.Hosts[0].Name != "acme-prod" || len(got.Services) != 1 {
		t.Fatalf("hosts/services: %+v %+v", got.Hosts, got.Services)
	}
	if len(got.Access) != 1 || got.Access[0].Label != "Admin" {
		t.Fatalf("access: %+v", got.Access)
	}
	if len(got.Connections) != 2 {
		t.Fatalf("connections: %+v", got.Connections)
	}

	prodGot, stagingGot := got.Envs[0], got.Envs[1]
	if prodGot.Name != "production" {
		t.Fatalf("env order: %+v", got.Envs)
	}
	if len(prodGot.Links) != 2 {
		t.Errorf("production should have 2 links (not staging-only): %+v", prodGot.Links)
	}
	if len(stagingGot.Links) != 3 {
		t.Errorf("staging should have 3 links: %+v", stagingGot.Links)
	}
	for _, l := range prodGot.Links {
		switch l.Label {
		case "Admin":
			if l.URL != "https://acme.example.com/admin" {
				t.Errorf("joined url: %q", l.URL)
			}
		case "External":
			if l.URL != "https://status.example.com" {
				t.Errorf("override url: %q", l.URL)
			}
		}
	}
	if len(prodGot.Hosts) != 1 || prodGot.Hosts[0].ID != h.ID || len(prodGot.Services) != 1 {
		t.Errorf("env hosts/services: %+v %+v", prodGot.Hosts, prodGot.Services)
	}
	if len(stagingGot.Hosts) != 0 {
		t.Errorf("staging should not list a production host: %+v", stagingGot.Hosts)
	}

	host, err := s.GetHost(ctx, h.ID)
	if err != nil || len(host.Projects) != 1 || host.Projects[0].Slug != "acme" {
		t.Errorf("host should list its project: %v %+v", err, host.Projects)
	}

	used, err := s.UsedBy(ctx, "service", v.ID)
	if err != nil || len(used) != 1 || used[0].Role != "secrets" {
		t.Errorf("used-by: %v %+v", err, used)
	}

	if err := s.UnlinkProjectHost(ctx, "test", p.ID, h.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if err := s.UnlinkProjectHost(ctx, "test", p.ID, h.ID, "production"); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("unlinking twice should be not found, got %v", err)
	}
	if err := s.DeleteAppLink(ctx, "test", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccess(ctx, "test", access.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProjectEnv(ctx, "test", staging.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteConnection(ctx, "test", got.Connections[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProjectLink(ctx, "test", got.Links[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetProject(ctx, p.ID)
	if len(got.Links) != 0 || len(got.Envs) != 1 || len(got.AppLinks) != 2 || len(got.Access) != 0 || len(got.Connections) != 1 {
		t.Errorf("soft deleted children still listed: %+v", got)
	}
	_ = prod
}
