package integrations

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proxmox reads nodes, VMs and containers and proposes host records.
// Read-only: only GET requests are made.
func Proxmox() *Plugin {
	return &Plugin{
		ID:          "proxmox",
		Category:    CatServers,
		Name:        "Proxmox",
		Description: "Read nodes, VMs and LXC containers, and propose host records.",
		Fields: []Field{
			{Key: "api_url", Label: "API URL", Kind: "url", Required: true, Default: "https://192.168.1.250:8006"},
			{Key: "token_id", Label: "Token id", Kind: "text", Required: true, Help: "user@realm!tokenid"},
			{Key: "token_secret", Label: "Token secret", Kind: "password", Secret: true, Required: true},
			{Key: "ignore_tls", Label: "Ignore TLS errors", Kind: "bool", Default: "1", Help: "Proxmox usually has a self-signed certificate."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var out struct {
				Data any `json:"data"`
			}
			return pveGet(ctx, cfg, "/version", &out)
		},
		Sync: proxmoxSync,
	}
}

// errNoPermission explains the most common Proxmox token problem: a token
// with "Privilege Separation" has no rights of its own, so lists come back
// empty instead of failing.
var errNoPermission = errors.New("the token can log in but sees no VMs or containers. In Proxmox: Datacenter → Permissions → Add → API Token Permission, path /, role PVEAuditor (read-only) — or recreate the token without Privilege Separation")

func proxmoxSync(ctx context.Context, cfg Config) ([]Op, error) {
	var nodes struct {
		Data []struct {
			Node   string `json:"node"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := pveGet(ctx, cfg, "/nodes", &nodes); err != nil {
		return nil, err
	}
	if len(nodes.Data) == 0 {
		if pveHasPermissions(ctx, cfg) {
			return []Op{}, nil
		}
		return nil, errNoPermission
	}
	// node addresses from the cluster status (works on single nodes too)
	nodeIP := map[string]string{}
	var cs struct {
		Data []struct {
			Type string `json:"type"`
			Name string `json:"name"`
			IP   string `json:"ip"`
		} `json:"data"`
	}
	if pveGet(ctx, cfg, "/cluster/status", &cs) == nil {
		for _, e := range cs.Data {
			if e.Type == "node" {
				nodeIP[e.Name] = e.IP
			}
		}
	}
	ops := []Op{}
	guests := 0
	for _, n := range nodes.Data {
		ops = append(ops, Op{
			Action: "create", Entity: "host", Ref: n.Node,
			Data: map[string]string{"kind": "bare", "location": n.Node, "address": nodeIP[n.Node],
				"os": "Proxmox VE", "status": n.Status},
			Note: "Proxmox node",
		})
		for _, kind := range []string{"lxc", "qemu"} {
			var list struct {
				Data []struct {
					VMID    int64  `json:"vmid"`
					Name    string `json:"name"`
					Status  string `json:"status"`
					MaxMem  int64  `json:"maxmem"`
					MaxDisk int64  `json:"maxdisk"`
					Tags    string `json:"tags"`
				} `json:"data"`
			}
			if err := pveGet(ctx, cfg, "/nodes/"+url.PathEscape(n.Node)+"/"+kind, &list); err != nil {
				continue
			}
			hostKind := "vm"
			if kind == "lxc" {
				hostKind = "lxc"
			}
			for _, vm := range list.Data {
				if vm.VMID == 0 {
					continue
				}
				guests++
				name := strings.TrimSpace(vm.Name)
				if name == "" {
					name = fmt.Sprintf("%s-%d", kind, vm.VMID)
				}
				addr := pveGuestIP(ctx, cfg, n.Node, kind, vm.VMID, vm.Status == "running")
				ops = append(ops, Op{
					Action: "create", Entity: "host", Ref: name,
					Data: map[string]string{
						"kind": hostKind, "proxmox_vmid": strconv.FormatInt(vm.VMID, 10),
						"location": n.Node, "status": vm.Status, "address": addr,
						"tags":      strings.ReplaceAll(vm.Tags, ";", ","),
						"memory_mb": strconv.FormatInt(vm.MaxMem/(1024*1024), 10),
						"disk_gb":   strconv.FormatInt(vm.MaxDisk/(1024*1024*1024), 10),
					},
					Note: fmt.Sprintf("Proxmox %s %d on %s (%s)", strings.ToUpper(kind), vm.VMID, n.Node, vm.Status),
				})
			}
		}
	}
	if guests == 0 && !pveHasPermissions(ctx, cfg) {
		return ops, errNoPermission
	}
	return ops, nil
}

// pveHasPermissions checks whether the token can see anything at all, so an
// empty node isn't mistaken for a privilege-separated token.
func pveHasPermissions(ctx context.Context, cfg Config) bool {
	var p struct {
		Data map[string]any `json:"data"`
	}
	if err := pveGet(ctx, cfg, "/access/permissions", &p); err != nil {
		return false
	}
	return len(p.Data) > 0
}

// pveGuestIP finds a guest's IPv4: the static ip= of an LXC's net0, else
// what the QEMU guest agent reports (only when running).
func pveGuestIP(ctx context.Context, cfg Config, node, kind string, vmid int64, running bool) string {
	base := "/nodes/" + url.PathEscape(node) + "/" + kind + "/" + strconv.FormatInt(vmid, 10)
	if kind == "lxc" {
		var c struct {
			Data map[string]any `json:"data"`
		}
		if pveGet(ctx, cfg, base+"/config", &c) == nil {
			for k, v := range c.Data {
				s, _ := v.(string)
				if !strings.HasPrefix(k, "net") {
					continue
				}
				for _, part := range strings.Split(s, ",") {
					if ip, ok := strings.CutPrefix(part, "ip="); ok && ip != "dhcp" && ip != "manual" {
						return strings.Split(ip, "/")[0]
					}
				}
			}
		}
		if !running {
			return ""
		}
		var ifs struct {
			Data []struct {
				Name string `json:"name"`
				Inet string `json:"inet"`
			} `json:"data"`
		}
		if pveGet(ctx, cfg, base+"/interfaces", &ifs) == nil {
			for _, i := range ifs.Data {
				if i.Name != "lo" && i.Inet != "" {
					return strings.Split(i.Inet, "/")[0]
				}
			}
		}
		return ""
	}
	if !running {
		return ""
	}
	var ag struct {
		Data struct {
			Result []struct {
				Name string `json:"name"`
				IPs  []struct {
					Type string `json:"ip-address-type"`
					Addr string `json:"ip-address"`
				} `json:"ip-addresses"`
			} `json:"result"`
		} `json:"data"`
	}
	if pveGet(ctx, cfg, base+"/agent/network-get-interfaces", &ag) == nil {
		for _, i := range ag.Data.Result {
			if i.Name == "lo" {
				continue
			}
			for _, ip := range i.IPs {
				if ip.Type == "ipv4" && !strings.HasPrefix(ip.Addr, "127.") {
					return ip.Addr
				}
			}
		}
	}
	return ""
}

func pveGet(ctx context.Context, cfg Config, path string, out any) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["api_url"]), "/")
	if base == "" {
		return fmt.Errorf("%w: api_url is required", ErrInvalid)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api2/json"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "PVEAPIToken="+cfg["token_id"]+"="+cfg["token_secret"])
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: truthy(cfg["ignore_tls"])}}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("proxmox: HTTP 401 — check the token id (user@realm!name) and secret")
		}
		return fmt.Errorf("proxmox: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func truthy(s string) bool {
	return s == "1" || s == "true" || s == "on" || s == "yes"
}
