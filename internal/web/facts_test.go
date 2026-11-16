package web_test

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

type fakeFacts struct{}

func (fakeFacts) Collect(ctx context.Context, address string) (*inventory.HostFacts, error) {
	if address != "10.0.0.5" {
		return nil, context.Canceled
	}
	return &inventory.HostFacts{
		OS: "Debian GNU/Linux 13", Kernel: "6.12.0", Hostname: "acme", Uptime: "up 3 days",
		DiskUsedPct: 42, FailedUnits: 1, RebootRequired: "no", PasswordAuth: "no",
		RootLogin: "prohibit-password", KeyFingerprints: "SHA256:abc deploy",
	}, nil
}

func TestFactsCollection(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()
	h, err := inv.CreateHost(ctx, "test", inventory.Host{Name: "acme", Addresses: []inventory.Address{{Address: "10.0.0.5"}}})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{}, web.WithFacts(fakeFacts{}))
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/hosts/"+strconv.FormatInt(h.ID, 10)+"/facts", url.Values{}); code != 303 {
		t.Fatalf("collect facts: %d", code)
	}
	code, body := page(t, srv, "/hosts/"+strconv.FormatInt(h.ID, 10))
	if code != 200 || !strings.Contains(body, "Debian GNU/Linux 13") || !strings.Contains(body, "42%") || !strings.Contains(body, "SHA256:abc") {
		t.Errorf("facts not rendered: %d", code)
	}
	code, res := do(t, srv, "POST", "/api/v1/hosts/"+strconv.FormatInt(h.ID, 10)+"/facts", "")
	if code != 200 || res["os"] != "Debian GNU/Linux 13" {
		t.Errorf("facts API: %d %v", code, res)
	}
}
