package inventory_test

import (
	"context"
	"testing"

	"github.com/dawidof/home-services/internal/inventory"
)

func seedInventory(t *testing.T, ctx context.Context, s *inventory.Store) {
	t.Helper()
	loc, err := s.CreateLocation(ctx, "seed", inventory.Location{Name: "Proxmox .250", Kind: "proxmox", URL: "https://192.168.1.250"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.CreateHost(ctx, "seed", inventory.Host{
		Name: "acme-prod", Hostname: "acme.example.com", LocationID: &loc.ID, Tags: []string{"prod"},
		Addresses: []inventory.Address{{Address: "192.168.1.240"}},
		SSH:       []inventory.SSHAccess{{User: "deploy", Port: 2222}},
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.AddService(ctx, "seed", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccess(ctx, "seed", inventory.Access{
		TargetEntity: "host", TargetID: h.ID, Label: "SSH", Method: "ssh_key",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "seed", inventory.Project{Name: "Acme", Group: "Clients", Runbook: "# Run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProjectLink(ctx, "seed", p.ID, inventory.ProjectLink{Kind: "repo", URL: "https://git.example.com/acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProjectEnv(ctx, "seed", p.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com"}); err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccess(ctx, "seed", inventory.Access{
		TargetEntity: "project", TargetID: p.ID, Label: "Admin", Method: "password", VaultItemURL: "https://vault.example.com/i/1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAppLink(ctx, "seed", p.ID, inventory.AppLink{Label: "Admin", Path: "/admin", Kind: "admin", AccessID: &acc.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkProjectHost(ctx, "seed", p.ID, h.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkProjectService(ctx, "seed", p.ID, v.ID, "production"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConnection(ctx, "seed", inventory.Connection{
		ProjectID: p.ID, TargetKind: "service", TargetID: &v.ID, Role: "code", ImpactNote: "app is down",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	src, dst := newStore(t), newStore(t)
	seedInventory(t, ctx, src)

	export, err := src.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Hosts) != 1 || len(export.Projects) != 1 || len(export.Locations) != 1 {
		t.Fatalf("export: %+v", export)
	}
	if len(export.Hosts[0].Services) != 1 || len(export.Hosts[0].Access) != 1 || len(export.Projects[0].Connections) != 1 {
		t.Fatalf("export should embed children: %+v %+v", export.Hosts[0], export.Projects[0])
	}

	// dry-run merge makes no changes and reports three creates
	sum, err := dst.Import(ctx, "test", export, "merge", true)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.DryRun || sum.Create.Hosts != 1 || sum.Create.Projects != 1 || sum.Create.Locations != 1 {
		t.Fatalf("dry run summary: %+v", sum)
	}
	if list, _ := dst.ListHosts(ctx); len(list) != 0 {
		t.Fatal("dry-run must not write")
	}

	sum, err = dst.Import(ctx, "test", export, "merge", false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Create.Hosts != 1 || sum.Create.Projects != 1 {
		t.Fatalf("merge summary: %+v", sum)
	}
	hosts, _ := dst.ListHosts(ctx)
	if len(hosts) != 1 || hosts[0].Name != "acme-prod" {
		t.Fatalf("imported host: %+v", hosts)
	}
	host, err := dst.GetHost(ctx, hosts[0].ID)
	if err != nil || host.Location == nil || len(host.Services) != 1 || len(host.Access) != 1 {
		t.Fatalf("imported host detail: %v %+v", err, host)
	}
	projects, _ := dst.ListProjects(ctx)
	if len(projects) != 1 || projects[0].Group != "Clients" {
		t.Fatalf("imported project: %+v", projects)
	}
	got, err := dst.GetProject(ctx, projects[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Links) != 1 || len(got.Envs) != 1 || len(got.AppLinks) != 1 || len(got.Hosts) != 1 || len(got.Services) != 1 || len(got.Connections) != 1 || len(got.Access) != 1 {
		t.Fatalf("project children after import: %+v", got)
	}
	if got.Hosts[0].HostID != host.ID || got.Services[0].ServiceID != host.Services[0].ID {
		t.Errorf("project links should point at the imported rows: %+v %+v", got.Hosts, got.Services)
	}
	if got.Connections[0].TargetID == nil || *got.Connections[0].TargetID != host.Services[0].ID {
		t.Errorf("connection target should be remapped: %+v", got.Connections[0])
	}
	if got.AppLinks[0].AccessID == nil || *got.AppLinks[0].AccessID != got.Access[0].ID {
		t.Errorf("app link access should be remapped: %+v %+v", got.AppLinks[0], got.Access)
	}
	link := got.Envs[0].Links[0]
	if link.URL != "https://acme.example.com/admin" {
		t.Errorf("env link should resolve: %+v", link)
	}

	// merging the same document again is idempotent (updates, no duplicates)
	sum, err = dst.Import(ctx, "test", export, "merge", false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Update.Hosts != 1 || sum.Update.Projects != 1 || sum.Create.Hosts != 0 {
		t.Fatalf("second merge: %+v", sum)
	}
	got, _ = dst.GetProject(ctx, projects[0].ID)
	if len(got.Links) != 1 || len(got.AppLinks) != 1 || len(got.Hosts) != 1 || len(got.Services) != 1 ||
		len(got.Connections) != 1 || len(got.Access) != 1 || len(got.Envs) != 1 {
		t.Fatalf("second merge duplicated children: %+v", got)
	}
	if h2, _ := dst.GetHost(ctx, host.ID); len(h2.Services) != 1 || len(h2.Access) != 1 {
		t.Fatalf("second merge duplicated host children: %+v", h2)
	}

	// replace: dry-run counts what would be deleted; apply leaves exactly one of each
	sum, err = dst.Import(ctx, "test", export, "replace", true)
	if err != nil || sum.Delete.Hosts != 1 || sum.Create.Hosts != 1 {
		t.Fatalf("replace dry run: %v %+v", err, sum)
	}
	if _, err := dst.Import(ctx, "test", export, "replace", false); err != nil {
		t.Fatal(err)
	}
	if list, _ := dst.ListHosts(ctx); len(list) != 1 {
		t.Errorf("replace should leave one clean host: %+v", list)
	} else if h3, _ := dst.GetHost(ctx, list[0].ID); len(h3.Services) != 1 {
		t.Errorf("replace duplicated services: %+v", h3)
	}
	if projects, _ := dst.ListProjects(ctx); len(projects) != 1 {
		t.Errorf("replace should leave one project: %+v", projects)
	}
	if sum, err := dst.Import(ctx, "test", export, "nope", true); err == nil {
		t.Errorf("bad mode should fail, got %+v", sum)
	}
}
