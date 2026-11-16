package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dawidof/home-services/internal/inventory"
)

func TestPatchHost(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	loc, _ := s.CreateLocation(ctx, "test", inventory.Location{Name: "Lab"})
	h, err := s.CreateHost(ctx, "test", inventory.Host{
		Name: "acme", Description: "keep me", LocationID: &loc.ID, Tags: []string{"prod"},
		Addresses: []inventory.Address{{Address: "10.0.0.5"}, {Kind: "tailscale", Address: "100.64.0.1"}},
		SSH:       []inventory.SSHAccess{{User: "root"}, {User: "deploy", Port: 2222}},
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "acme-prod"
	got, err := s.PatchHost(ctx, "test", h.ID, inventory.HostPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "acme-prod" || got.Description != "keep me" || len(got.Addresses) != 2 || len(got.SSH) != 2 {
		t.Fatalf("omitted fields must be preserved: %+v", got)
	}
	if got.Location == nil || got.Location.ID != loc.ID {
		t.Errorf("location dropped by patch: %+v", got.Location)
	}

	// add an address, update one, remove one SSH login
	ops := []inventory.AddressOp{
		{Kind: "public", Address: "203.0.113.7"},
		{ID: got.Addresses[0].ID, IsPrimary: true},
	}
	sshOps := []inventory.SSHAccessOp{{ID: got.SSH[0].ID, Remove: true}}
	tags := []string{"prod", "rails"}
	got, err = s.PatchHost(ctx, "test", h.ID, inventory.HostPatch{Addresses: &ops, SSH: &sshOps, Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Addresses) != 3 || len(got.SSH) != 1 || got.SSH[0].User != "deploy" {
		t.Fatalf("address/ssh patch: %+v %+v", got.Addresses, got.SSH)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags not patched: %+v", got.Tags)
	}

	// clear the location with 0
	zero := int64(0)
	got, err = s.PatchHost(ctx, "test", h.ID, inventory.HostPatch{LocationID: &zero})
	if err != nil || got.LocationID != nil || got.Location != nil {
		t.Fatalf("clear location: %v %+v", err, got.Location)
	}

	// invalid values are rejected and leave the host alone
	bad := "toaster"
	if _, err := s.PatchHost(ctx, "test", h.ID, inventory.HostPatch{Kind: &bad}); !errors.Is(err, inventory.ErrInvalid) {
		t.Errorf("bad kind: %v", err)
	}
	if after, _ := s.GetHost(ctx, h.ID); after.Kind != "vm" {
		t.Errorf("failed patch must not write: %+v", after)
	}
	if _, err := s.PatchHost(ctx, "test", 999, inventory.HostPatch{}); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("unknown host: %v", err)
	}
	if got, err := s.PatchHost(ctx, "test", h.ID, inventory.HostPatch{}); err != nil || len(got.Addresses) != 3 {
		t.Errorf("empty patch should be a no-op: %v %+v", err, got)
	}
}
