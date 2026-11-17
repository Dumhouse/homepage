package changesets

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// Ops beyond plain creates, used by the cloud, VPN, metrics and storage
// plugins:
//
//   - address/link: add an address (e.g. a Tailscale IP) to an existing host
//   - cost/create:  a monthly cost for a host (e.g. a cloud server's price)
//   - facts:        an observation (OS, disk, uptime) — written straight away
//     by ApplyFacts, like SSH facts, never queued for review

// hostLoose finds a host by name (case-insensitive, also the short name of an
// FQDN) or by any of its addresses.
func (s *Store) hostLoose(ctx context.Context, name string, addresses ...string) int64 {
	hosts, _ := s.inv.ListHosts(ctx)
	name = strings.ToLower(strings.TrimSpace(name))
	short := strings.SplitN(name, ".", 2)[0]
	for _, h := range hosts {
		for _, a := range h.Addresses {
			for _, want := range addresses {
				if want != "" && a.Address == want {
					return h.ID
				}
			}
		}
	}
	if name == "" {
		return 0
	}
	for _, h := range hosts {
		n := strings.ToLower(h.Name)
		if n == name || n == short || strings.SplitN(n, ".", 2)[0] == short {
			return h.ID
		}
	}
	return 0
}

// linkAddress adds data["address"] (kind data["kind"]) to the host named
// data["host"] or holding data["match"]. Nothing is created, so Undo has
// nothing to delete.
func (s *Store) linkAddress(ctx context.Context, actor string, data map[string]string) (int64, error) {
	id := s.hostLoose(ctx, data["host"], data["match"])
	if id == 0 {
		return 0, fmt.Errorf("no host named %q in the inventory — add it first, then apply again", data["host"])
	}
	h, err := s.inv.GetHost(ctx, id)
	if err != nil {
		return 0, err
	}
	for _, a := range h.Addresses {
		if a.Address == data["address"] {
			return 0, nil
		}
		if data["only_if_empty"] != "" && a.Kind == orDefault(data["kind"], "other") {
			return 0, nil // a DHCP lease never overrides an address you entered
		}
	}
	h.Addresses = append(h.Addresses, inventory.Address{Kind: orDefault(data["kind"], "other"), Address: data["address"], IsPrimary: len(h.Addresses) == 0})
	_, err = s.inv.UpdateHost(ctx, actor, id, *h)
	return 0, err
}

// createCost adds a recurring cost tied to a host (looked up by name, so a
// host created earlier in the same changeset is found).
func (s *Store) createCost(ctx context.Context, actor string, op Op) (int64, error) {
	data := op.Data
	cents, err := strconv.ParseInt(data["amount_cents"], 10, 64)
	if err != nil || cents < 0 {
		return 0, fmt.Errorf("bad amount %q", data["amount_cents"])
	}
	c := inventory.Cost{
		Label: orDefault(data["label"], op.Ref), AmountCents: cents, Currency: orDefault(data["currency"], "EUR"),
		Cycle: orDefault(data["cycle"], "monthly"), PaymentLabel: data["provider"], Notes: data["notes"],
		Source: orDefault(data["source"], "plugin"), AutoRenew: true,
	}
	if id := s.hostLoose(ctx, data["host"]); id != 0 {
		c.TargetEntity, c.TargetID = "host", id
	}
	created, err := s.inv.CreateCost(ctx, actor, c)
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

// costExists reports a cost with the same label from the same source.
func (s *Store) costExists(ctx context.Context, label, source string) bool {
	costs, _ := s.inv.ListCosts(ctx)
	for _, c := range costs {
		if c.Label == label && c.Source == source {
			return true
		}
	}
	return false
}

// ApplyFacts writes the "facts" ops onto the hosts they describe and returns
// the other ops. Facts of hosts not in the inventory yet are named in
// missing and returned in rest. Fields an op leaves empty keep their old value.
func (s *Store) ApplyFacts(ctx context.Context, actor string, ops []Op) (updated int, missing []string, rest []Op) {
	for _, op := range ops {
		if op.Entity != "facts" {
			rest = append(rest, op)
			continue
		}
		d := op.Data
		id := s.hostLoose(ctx, orDefault(d["host"], op.Ref), d["address"], d["address2"])
		if id == 0 {
			// Not documented yet: keep it next to the proposed host so the
			// facts land when that host is approved.
			missing = append(missing, orDefault(d["host"], op.Ref))
			rest = append(rest, op)
			continue
		}
		f := inventory.HostFacts{HostID: id, PasswordAuth: "unknown", RebootRequired: "no"}
		if old, err := s.inv.GetHostFacts(ctx, id); err == nil && old != nil {
			f = *old
			f.HostID = id
		}
		set := func(dst *string, key string) {
			if v := strings.TrimSpace(d[key]); v != "" {
				*dst = v
			}
		}
		set(&f.OS, "os")
		set(&f.Kernel, "kernel")
		set(&f.Hostname, "hostname")
		set(&f.Uptime, "uptime")
		set(&f.RebootRequired, "reboot_required")
		if v, err := strconv.Atoi(d["disk_used_pct"]); err == nil && v >= 0 && v <= 100 {
			f.DiskUsedPct = v
		}
		if v, ok := d["error"]; ok {
			f.Error = v // a problem the source reports now ("" clears it), e.g. a degraded pool
		}
		if err := s.inv.SaveHostFacts(ctx, actor, f); err == nil {
			updated++
		}
	}
	return updated, missing, rest
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
