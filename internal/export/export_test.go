package export_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/export"
	"github.com/Dumhouse/homepage/internal/inventory"
)

func hosts() []inventory.Host {
	return []inventory.Host{
		{ID: 1, Name: "NAS box", Kind: "bare", Tags: []string{"storage", "proposed"}, Location: &inventory.LocationRef{Name: "Rack"},
			Addresses: []inventory.Address{{Kind: "tailscale", Address: "100.64.0.5"}, {Kind: "lan", Address: "192.168.1.20", IsPrimary: true}},
			SSH:       []inventory.SSHAccess{{User: "admin", Port: 2222, Vault: "https://vault/secret"}},
			Services: []inventory.Service{{Name: "Grafana", Type: "grafana", Port: 3000, Protocol: "http"},
				{Name: "Jellyfin", Domain: "jf.example.com", Port: 8096}, {Name: "postgres", Port: 5432, Protocol: "tcp"}, {Name: "node", Port: 9100, Protocol: "http"}}},
		{ID: 2, Name: "old", Kind: "vm", Lifecycle: "decommissioned", Addresses: []inventory.Address{{Kind: "lan", Address: "192.168.1.99"}}},
		{ID: 3, Name: "vps", Kind: "vps", Addresses: []inventory.Address{{Kind: "public", Address: "203.0.113.5"}}},
	}
}

func TestHomepage(t *testing.T) {
	b, _ := export.Render("homepage.yaml", hosts(), export.Options{})
	s := string(b)
	for _, want := range []string{`- "Rack":`, `- "Grafana":`, `href: "http://192.168.1.20:3000"`, `icon: "grafana.png"`, `href: "https://jf.example.com"`, `description: "on NAS box"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in\n%s", want, s)
		}
	}
	if strings.Contains(s, "postgres") || strings.Contains(s, "vault") {
		t.Errorf("non-web service or secret leaked:\n%s", s)
	}
}

func TestAnsible(t *testing.T) {
	b, _ := export.Render("ansible.yaml", hosts(), export.Options{})
	s := string(b)
	for _, want := range []string{"nas_box:\n      ansible_host: \"192.168.1.20\"\n      ansible_user: \"admin\"\n      ansible_port: 2222", "loc_rack:", "tag_storage:", "kind_vps:"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, "old") || strings.Contains(s, "tag_proposed") || strings.Contains(s, "vault") {
		t.Errorf("unexpected content:\n%s", s)
	}
}

func TestPrometheusAndNetBox(t *testing.T) {
	b, _ := export.Render("prometheus.json", hosts(), export.Options{})
	var groups []struct {
		Targets []string          `json:"targets"`
		Labels  map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(b, &groups); err != nil || len(groups) != 1 || groups[0].Targets[0] != "192.168.1.20:9100" || groups[0].Labels["location"] != "Rack" {
		t.Fatalf("%s", b)
	}
	b, _ = export.Render("prometheus.json", hosts(), export.Options{All: true, Port: 9273})
	if !strings.Contains(string(b), "203.0.113.5:9273") {
		t.Fatalf("all: %s", b)
	}
	b, _ = export.Render("netbox-ips.csv", hosts(), export.Options{})
	if s := string(b); !strings.HasPrefix(s, "address,status,dns_name,description\n") || !strings.Contains(s, "192.168.1.20/32,active,,NAS box (lan)") || strings.Contains(s, "192.168.1.99") {
		t.Fatalf("%s", s)
	}
}
