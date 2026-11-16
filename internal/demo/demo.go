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
		{Name: "Forgejo", Type: "forgejo", Port: 3000, Protocol: "http", Domain: "git.example.com"},
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
	return nil
}
