package inventory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/inventory"
)

func TestSSHConfig(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	_, err := s.CreateHost(ctx, "test", inventory.Host{
		Name:      "Acme Prod",
		Addresses: []inventory.Address{{Kind: "tailscale", Address: "100.64.0.1"}, {Address: "10.0.0.5"}},
		SSH:       []inventory.SSHAccess{{User: "deploy", Port: 2222}, {User: "root"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHost(ctx, "test", inventory.Host{Name: "no-ssh"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.SSHConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Host acme-prod\n", "HostName 100.64.0.1", "User deploy", "Port 2222", "Host acme-prod-root", "User root"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("missing %q in:\n%s", want, cfg)
		}
	}
	if strings.Contains(cfg, "no-ssh") {
		t.Error("hosts without SSH logins should be omitted")
	}
	if cfg, _ := s.SSHConfig(ctx); strings.Count(cfg, "Host ") != 2 {
		t.Errorf("want two blocks, got:\n%s", cfg)
	}
}
