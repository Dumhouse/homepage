// Package demo seeds a small fictional inventory ("acme") for trials, screenshots and tests.
// Addresses use 192.0.2.0/24 (RFC 5737 TEST-NET-1), which is never routed, so health checks
// started by the demo can't reach anything on a real network.
package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/dawidof/home-services/internal/inventory"
)

// Seed fills an empty store. It refuses to touch a database that already has hosts or projects.
func Seed(ctx context.Context, inv *inventory.Store) error {
	stats, err := inv.Stats(ctx)
	if err != nil {
		return err
	}
	if stats.Hosts > 0 || stats.Projects > 0 {
		return fmt.Errorf("database already has %d hosts and %d projects — use an empty data directory for the demo",
			stats.Hosts, stats.Projects)
	}
	loc, err := inv.CreateLocation(ctx, "demo", inventory.Location{
		Name: "Proxmox demo", Kind: "proxmox", URL: "https://192.0.2.250:8006",
	})
	if err != nil {
		return err
	}
	edge, err := inv.CreateHost(ctx, "demo", inventory.Host{
		Name: "edge", Kind: "lxc", OS: "Debian 13", LocationID: &loc.ID,
		Tags: []string{"edge", "public"}, Addresses: []inventory.Address{{Address: "192.0.2.10"}},
		SSH: []inventory.SSHAccess{{User: "deploy", Vault: "https://vault.example.com/item/edge"}},
	})
	if err != nil {
		return err
	}
	app, err := inv.CreateHost(ctx, "demo", inventory.Host{
		Name: "app-1", Kind: "lxc", OS: "Debian 13", LocationID: &loc.ID,
		Tags: []string{"app"}, Addresses: []inventory.Address{{Address: "192.0.2.20"}},
		SSH: []inventory.SSHAccess{{User: "deploy"}},
	})
	if err != nil {
		return err
	}
	for _, svc := range []inventory.Service{
		{Name: "Caddy", Type: "caddy", Port: 2019, Protocol: "http", Domain: "edge.example.com"},
		{Name: "ntfy", Type: "ntfy", Port: 8080, Protocol: "http", Domain: "ntfy.example.com"},
	} {
		if _, err := inv.AddService(ctx, "demo", edge.ID, svc); err != nil {
			return err
		}
	}
	web, err := inv.AddService(ctx, "demo", app.ID, inventory.Service{
		Name: "web", Type: "rails", Port: 3000, Protocol: "http", Domain: "acme.example.com",
	})
	if err != nil {
		return err
	}
	if _, err := inv.AddService(ctx, "demo", app.ID, inventory.Service{Name: "postgres", Type: "postgres", Port: 5432}); err != nil {
		return err
	}

	proj, err := inv.CreateProject(ctx, "demo", inventory.Project{
		Name: "Acme", Group: "Apps", Description: "Demo project: Rails app + Postgres, deployed from Forgejo.",
		Runbook: "## Deploy\n\n1. Push to `main`.\n2. Forgejo Actions builds and deploys to `app-1`.\n3. Check `/up`.",
	})
	if err != nil {
		return err
	}
	prod, err := inv.AddProjectEnv(ctx, "demo", proj.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com", Sort: 1})
	if err != nil {
		return err
	}
	if _, err := inv.AddProjectEnv(ctx, "demo", proj.ID, inventory.ProjectEnv{Name: "staging", BaseURL: "https://staging.acme.example.com", Sort: 2}); err != nil {
		return err
	}
	if _, err := inv.AddProjectLink(ctx, "demo", proj.ID, inventory.ProjectLink{Kind: "repo", URL: "https://git.example.com/acme/acme"}); err != nil {
		return err
	}
	for _, link := range []inventory.AppLink{
		{Label: "App", Path: "/", Kind: "page"},
		{Label: "Admin", Path: "/admin", Kind: "admin"},
		{Label: "Sidekiq", Path: "/sidekiq", Kind: "jobs"},
		{Label: "Health", Path: "/up", Kind: "health", HealthCheck: true},
		{Label: "MCP", Path: "/mcp", Kind: "mcp"},
	} {
		if _, err := inv.AddAppLink(ctx, "demo", proj.ID, link); err != nil {
			return err
		}
	}
	acc, err := inv.CreateAccess(ctx, "demo", inventory.Access{
		TargetEntity: "project", TargetID: proj.ID, Label: "Admin", Method: "github_sso",
		Account: "ops@example.com", TwoFA: "totp", VaultItemURL: "https://vault.example.com/item/acme-admin",
	})
	if err != nil {
		return err
	}
	if _, err := inv.CreateAccess(ctx, "demo", inventory.Access{
		TargetEntity: "host", TargetID: app.ID, Label: "SSH", Method: "ssh_key",
		Account: "deploy", VaultItemURL: "https://vault.example.com/item/app-ssh",
	}); err != nil {
		return err
	}
	if err := inv.LinkProjectHost(ctx, "demo", proj.ID, app.ID, prod.Name); err != nil {
		return err
	}
	if err := inv.LinkProjectService(ctx, "demo", proj.ID, web.ID, prod.Name); err != nil {
		return err
	}
	if _, err := inv.CreateConnection(ctx, "demo", inventory.Connection{
		ProjectID: proj.ID, TargetKind: "url", URL: "https://git.example.com", Role: "code",
		Detail: "Forgejo repo + Actions", ImpactNote: "no deploys", AccessID: &acc.ID,
	}); err != nil {
		return err
	}
	expiry := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	if _, err := inv.CreateDomain(ctx, "demo", inventory.Domain{
		Name: "example.com", Registrar: "Demo Registrar", Account: "ops@example.com",
		ExpiresAt: expiry, AutoRenew: true, PriceCents: 1299, Currency: "EUR",
		PaymentLabel: "Card ··42", Nameservers: "ns1.example.com\nns2.example.com", TransferLock: true,
	}); err != nil {
		return err
	}
	if _, err := inv.CreateCost(ctx, "demo", inventory.Cost{
		Label: "App LXC", AmountCents: 451, Currency: "EUR", Cycle: "monthly",
		ProjectID: &proj.ID, AutoRenew: true, PaymentLabel: "Card ··42",
	}); err != nil {
		return err
	}
	return seedLab(ctx, inv, loc.ID, proj.ID, edge.ID)
}

// seedLab adds the rest of the example lab: a Proxmox node, NAS, Home Assistant,
// monitoring, a VPS, more projects, domains, costs and a few unregistered
// devices in the Discovery inbox. Everything is fictional.
func seedLab(ctx context.Context, inv *inventory.Store, pveLoc, acmeID, edgeID int64) error {
	vpsLoc, err := inv.CreateLocation(ctx, "demo", inventory.Location{Name: "Hetzner", Kind: "vps", URL: "https://console.hetzner.cloud"})
	if err != nil {
		return err
	}
	rack, err := inv.CreateLocation(ctx, "demo", inventory.Location{Name: "Home network", Kind: "bare-metal", Notes: "Devices outside Proxmox"})
	if err != nil {
		return err
	}
	type svc = inventory.Service
	type hostDef struct {
		h    inventory.Host
		svcs []svc
	}
	lan := func(ip string) []inventory.Address {
		return []inventory.Address{{Kind: "lan", Address: ip, IsPrimary: true}}
	}
	defs := []hostDef{
		{inventory.Host{Name: "pve", Kind: "bare", OS: "Proxmox VE 9", Hostname: "pve", LocationID: &pveLoc, Tags: []string{"hypervisor"},
			Description: "Single Proxmox node running most of the lab.", Addresses: lan("192.0.2.5"), SSH: []inventory.SSHAccess{{User: "root"}}},
			[]svc{{Name: "Proxmox VE", Type: "proxmox", Port: 8006, Protocol: "https"}}},
		{inventory.Host{Name: "git", Kind: "lxc", OS: "Debian 13", LocationID: &pveLoc, Tags: []string{"git", "ci"}, Addresses: lan("192.0.2.12"),
			SSH: []inventory.SSHAccess{{User: "deploy"}}},
			[]svc{{Name: "Forgejo", Type: "forgejo", Port: 3000, Protocol: "http", Domain: "git.example.com"}, {Name: "Forgejo Runner", Type: "forgejo", Port: 0, Protocol: "tcp"}}},
		{inventory.Host{Name: "monitor", Kind: "lxc", OS: "Debian 13", LocationID: &pveLoc, Tags: []string{"monitoring"}, Addresses: lan("192.0.2.15")},
			[]svc{{Name: "Grafana", Type: "grafana", Port: 3000, Protocol: "http"}, {Name: "Prometheus", Type: "prometheus", Port: 9090, Protocol: "http"}, {Name: "Gatus", Type: "gatus", Port: 8080, Protocol: "http", Domain: "status.example.com"}}},
		{inventory.Host{Name: "app-2", Kind: "vm", OS: "Ubuntu 24.04", LocationID: &pveLoc, Tags: []string{"app", "staging"}, Addresses: lan("192.0.2.21"),
			SSH: []inventory.SSHAccess{{User: "deploy"}}},
			[]svc{{Name: "web (staging)", Type: "rails", Port: 3000, Protocol: "http", Domain: "staging.acme.example.com"}}},
		{inventory.Host{Name: "nas", Kind: "bare", OS: "OpenMediaVault 7", LocationID: &rack.ID, Tags: []string{"storage"}, Addresses: lan("192.0.2.30"),
			Description: "4 × 8 TB, SMB and NFS shares, nightly backups."},
			[]svc{{Name: "OpenMediaVault", Type: "openmediavault", Port: 80, Protocol: "http"}, {Name: "Immich", Type: "immich", Port: 2283, Protocol: "http", Domain: "photos.example.com"}, {Name: "qBittorrent", Type: "qbittorrent", Port: 8080, Protocol: "http"}}},
		{inventory.Host{Name: "homeassistant", Kind: "vm", OS: "Home Assistant OS", LocationID: &pveLoc, Tags: []string{"smart-home"}, Addresses: lan("192.0.2.40")},
			[]svc{{Name: "Home Assistant", Type: "home-assistant", Port: 8123, Protocol: "http"}}},
		{inventory.Host{Name: "router", Kind: "bare", OS: "RouterOS 7", LocationID: &rack.ID, Tags: []string{"network"}, Addresses: lan("192.0.2.1")},
			[]svc{{Name: "WebFig", Type: "mikrotik", Port: 80, Protocol: "http"}}},
		{inventory.Host{Name: "vps-1", Kind: "vps", OS: "Debian 13", LocationID: &vpsLoc.ID, Tags: []string{"public"}, Addresses: []inventory.Address{{Kind: "public", Address: "192.0.2.200", IsPrimary: true}},
			SSH: []inventory.SSHAccess{{User: "deploy"}}},
			[]svc{{Name: "Uptime page", Type: "gatus", Port: 443, Protocol: "https", Domain: "uptime.example.org"}}},
		{inventory.Host{Name: "old-registry", Kind: "vm", OS: "Alpine", LocationID: &pveLoc, Lifecycle: "deprecated", Tags: []string{"registry"}, Addresses: lan("192.0.2.99"),
			Description: "Replaced by Forgejo packages; kept until the images are moved."}, nil},
	}
	ids := map[string]int64{}
	for _, d := range defs {
		h, err := inv.CreateHost(ctx, "demo", d.h)
		if err != nil {
			return err
		}
		ids[d.h.Name] = h.ID
		for _, v := range d.svcs {
			if _, err := inv.AddService(ctx, "demo", h.ID, v); err != nil {
				return err
			}
		}
	}

	photos, err := inv.CreateProject(ctx, "demo", inventory.Project{Name: "Photo vault", Group: "Home",
		Description: "Family photos: Immich on the NAS with nightly backups to a VPS."})
	if err != nil {
		return err
	}
	if _, err := inv.AddProjectEnv(ctx, "demo", photos.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://photos.example.com", Sort: 1}); err != nil {
		return err
	}
	platform, err := inv.CreateProject(ctx, "demo", inventory.Project{Name: "Home lab platform", Group: "Infra",
		Description: "Shared infrastructure: hypervisor, edge proxy, git, monitoring and storage."})
	if err != nil {
		return err
	}
	for _, n := range []string{"pve", "git", "monitor", "nas", "router"} {
		if err := inv.LinkProjectHost(ctx, "demo", platform.ID, ids[n], ""); err != nil {
			return err
		}
	}
	if err := inv.LinkProjectHost(ctx, "demo", photos.ID, ids["nas"], "production"); err != nil {
		return err
	}
	if err := inv.LinkProjectHost(ctx, "demo", acmeID, ids["app-2"], "staging"); err != nil {
		return err
	}

	soon := time.Now().AddDate(0, 0, 21).Format("2006-01-02")
	later := time.Now().AddDate(0, 7, 0).Format("2006-01-02")
	for _, d := range []inventory.Domain{
		{Name: "acme.dev", Registrar: "Cloudflare", Account: "ops@example.com", ExpiresAt: soon, AutoRenew: false, PriceCents: 1099, Currency: "USD"},
		{Name: "example.org", Registrar: "Porkbun", Account: "me@example.org", ExpiresAt: later, AutoRenew: true, PriceCents: 899, Currency: "EUR"},
	} {
		if _, err := inv.CreateDomain(ctx, "demo", d); err != nil {
			return err
		}
	}
	for _, c := range []inventory.Cost{
		{Label: "Hetzner CX22 (vps-1)", AmountCents: 459, Currency: "EUR", Cycle: "monthly", AutoRenew: true, PaymentLabel: "Card ··42", ProjectID: &photos.ID},
		{Label: "Electricity — rack", AmountCents: 1840, Currency: "EUR", Cycle: "monthly", ProjectID: &platform.ID},
		{Label: "Backblaze B2 backups", AmountCents: 340, Currency: "USD", Cycle: "monthly", AutoRenew: true, ProjectID: &platform.ID},
		{Label: "Proxmox subscription", AmountCents: 11500, Currency: "EUR", Cycle: "yearly", NextRenewal: later, AutoRenew: true, ProjectID: &platform.ID},
	} {
		if _, err := inv.CreateCost(ctx, "demo", c); err != nil {
			return err
		}
	}

	for _, d := range []struct {
		ip, name string
		ports    []inventory.PortInfo
	}{
		{"192.0.2.150", "printer", []inventory.PortInfo{{Port: 80, Service: "Web UI", Title: "Klipper · Mainsail"}}},
		{"192.0.2.77", "", []inventory.PortInfo{{Port: 22, Service: "SSH"}, {Port: 8096, Service: "Jellyfin", Title: "Jellyfin"}}},
		{"192.0.2.88", "cam-garage", []inventory.PortInfo{{Port: 554, Service: "RTSP"}}},
	} {
		if _, _, err := inv.UpsertDiscovery(ctx, d.ip, d.name, d.ports); err != nil {
			return err
		}
	}
	_ = edgeID
	return nil
}
