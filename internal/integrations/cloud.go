package integrations

import (
	"context"
	"crypto/sha1" //nolint:gosec // OVH's request signature is defined as SHA-1
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Cloud and VPS providers: each server becomes a "vps" host with its public
// address, plus its monthly price as a cost when the API tells it.

const CatCloud = "Cloud & VPS"

// HetznerAPI is the Hetzner Cloud API base (a variable for tests).
var HetznerAPI = "https://api.hetzner.cloud/v1"

func Hetzner() *Plugin {
	return &Plugin{
		ID: "hetzner", Name: "Hetzner Cloud", Category: CatCloud,
		Description: "Import cloud servers with their public and private addresses, type and location, and propose each server's monthly price as a cost.",
		Fields: []Field{
			{Key: "token", Label: "API token", Kind: "password", Secret: true, Required: true, Help: "Cloud console → your project → Security → API tokens → Generate, permission Read. One token per project."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := hetznerServers(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			servers, err := hetznerServers(ctx, cfg)
			if err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, s := range servers {
				priv := ""
				if len(s.PrivateNet) > 0 {
					priv = s.PrivateNet[0].IP
				}
				summary := fmt.Sprintf("Hetzner %s (%g vCPU, %g GB RAM, %d GB disk) in %s",
					s.ServerType.Name, s.ServerType.Cores, s.ServerType.Memory, s.ServerType.Disk, s.Datacenter.Location.Name)
				tags := []string{"hetzner"}
				for k, v := range s.Labels {
					tags = append(tags, k+"="+v)
				}
				ops = append(ops, Op{Action: "create", Entity: "host", Ref: s.Name, Note: summary, Data: map[string]string{
					"kind": "vps", "location": "Hetzner " + s.Datacenter.Location.Name, "address": s.PublicNet.IPv4.IP, "address_kind": "public",
					"address2": priv, "address2_kind": "lan", "os": s.Image.Description, "status": s.Status,
					"summary": summary, "tags": strings.Join(tags, ","),
				}})
				for _, p := range s.ServerType.Prices {
					if p.Location != s.Datacenter.Location.Name {
						continue
					}
					if v, err := strconv.ParseFloat(p.PriceMonthly.Gross, 64); err == nil && v > 0 {
						ops = append(ops, costOp(s.Name, "Hetzner", "hetzner", int64(math.Round(v*100)), "EUR", s.ServerType.Name))
					}
				}
			}
			return ops, nil
		},
	}
}

type hetznerServer struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
	PublicNet struct {
		IPv4 struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
	PrivateNet []struct {
		IP string `json:"ip"`
	} `json:"private_net"`
	ServerType struct {
		Name   string  `json:"name"`
		Cores  float64 `json:"cores"`
		Memory float64 `json:"memory"`
		Disk   int     `json:"disk"`
		Prices []struct {
			Location     string `json:"location"`
			PriceMonthly struct {
				Gross string `json:"gross"`
			} `json:"price_monthly"`
		} `json:"prices"`
	} `json:"server_type"`
	Datacenter struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"datacenter"`
	Image struct {
		Description string `json:"description"`
	} `json:"image"`
}

func hetznerServers(ctx context.Context, cfg Config) ([]hetznerServer, error) {
	all := []hetznerServer{}
	for page := 1; page < 50; page++ {
		var res struct {
			Servers []hetznerServer `json:"servers"`
			Meta    struct {
				Pagination struct {
					NextPage *int `json:"next_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		err := getJSON(ctx, req{URL: HetznerAPI + "/servers?per_page=50&page=" + strconv.Itoa(page), What: "Hetzner",
			Header: map[string]string{"Authorization": "Bearer " + strings.TrimSpace(cfg["token"])}}, &res)
		if err != nil {
			return nil, err
		}
		all = append(all, res.Servers...)
		if res.Meta.Pagination.NextPage == nil {
			break
		}
	}
	return all, nil
}

// DigitalOceanAPI is the DigitalOcean API base (a variable for tests).
var DigitalOceanAPI = "https://api.digitalocean.com/v2"

func DigitalOcean() *Plugin {
	return &Plugin{
		ID: "digitalocean", Name: "DigitalOcean", Category: CatCloud,
		Description: "Import droplets with their public and private addresses, size and region, and propose each droplet's monthly price as a cost.",
		Fields: []Field{
			{Key: "token", Label: "API token", Kind: "password", Secret: true, Required: true, Help: "Control panel → API → Generate New Token, scope Read only (or custom scope droplet:read)."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := doDroplets(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			drops, err := doDroplets(ctx, cfg)
			if err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, d := range drops {
				pub, priv := "", ""
				for _, n := range d.Networks.V4 {
					switch n.Type {
					case "public":
						pub = n.IP
					case "private":
						priv = n.IP
					}
				}
				summary := fmt.Sprintf("DigitalOcean %s (%d vCPU, %d MB RAM, %d GB disk) in %s", d.SizeSlug, d.VCPUs, d.Memory, d.Disk, d.Region.Slug)
				ops = append(ops, Op{Action: "create", Entity: "host", Ref: d.Name, Note: summary, Data: map[string]string{
					"kind": "vps", "location": "DigitalOcean " + d.Region.Slug, "address": pub, "address_kind": "public",
					"address2": priv, "address2_kind": "lan", "os": strings.TrimSpace(d.Image.Distribution + " " + d.Image.Name),
					"status": d.Status, "summary": summary, "tags": strings.Join(append([]string{"digitalocean"}, d.Tags...), ","),
				}})
				if d.Size.PriceMonthly > 0 {
					ops = append(ops, costOp(d.Name, "DigitalOcean", "digitalocean", int64(math.Round(d.Size.PriceMonthly*100)), "USD", d.SizeSlug))
				}
			}
			return ops, nil
		},
	}
}

type doDroplet struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Memory   int      `json:"memory"`
	VCPUs    int      `json:"vcpus"`
	Disk     int      `json:"disk"`
	SizeSlug string   `json:"size_slug"`
	Tags     []string `json:"tags"`
	Size     struct {
		PriceMonthly float64 `json:"price_monthly"`
	} `json:"size"`
	Region struct {
		Slug string `json:"slug"`
	} `json:"region"`
	Image struct {
		Distribution string `json:"distribution"`
		Name         string `json:"name"`
	} `json:"image"`
	Networks struct {
		V4 []struct {
			IP   string `json:"ip_address"`
			Type string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
}

func doDroplets(ctx context.Context, cfg Config) ([]doDroplet, error) {
	all := []doDroplet{}
	next := DigitalOceanAPI + "/droplets?per_page=200"
	for i := 0; next != "" && i < 20; i++ {
		var res struct {
			Droplets []doDroplet `json:"droplets"`
			Links    struct {
				Pages struct {
					Next string `json:"next"`
				} `json:"pages"`
			} `json:"links"`
		}
		if err := getJSON(ctx, req{URL: next, What: "DigitalOcean",
			Header: map[string]string{"Authorization": "Bearer " + strings.TrimSpace(cfg["token"])}}, &res); err != nil {
			return nil, err
		}
		all = append(all, res.Droplets...)
		next = res.Links.Pages.Next
	}
	return all, nil
}

// OVHEndpoints maps the OVH region to its API base.
var OVHEndpoints = map[string]string{
	"ovh-eu": "https://eu.api.ovh.com/1.0",
	"ovh-ca": "https://ca.api.ovh.com/1.0",
	"ovh-us": "https://api.us.ovhcloud.com/1.0",
}

func OVH() *Plugin {
	return &Plugin{
		ID: "ovh", Name: "OVHcloud", Category: CatCloud,
		Description: "Import VPS and dedicated servers with their addresses, model and zone.",
		Fields: []Field{
			{Key: "endpoint", Label: "Region", Kind: "text", Required: true, Default: "ovh-eu", Help: "ovh-eu, ovh-ca or ovh-us — or a full API URL."},
			{Key: "app_key", Label: "Application key", Kind: "text", Required: true, Help: "Create at https://eu.api.ovh.com/createToken with rights GET /vps, GET /vps/*, GET /dedicated/server, GET /dedicated/server/*."},
			{Key: "app_secret", Label: "Application secret", Kind: "password", Secret: true, Required: true},
			{Key: "consumer_key", Label: "Consumer key", Kind: "password", Secret: true, Required: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var names []string
			return ovhGet(ctx, cfg, "/vps", &names)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			ops := []Op{}
			var vps []string
			if err := ovhGet(ctx, cfg, "/vps", &vps); err != nil {
				return nil, err
			}
			for _, name := range vps {
				var v struct {
					DisplayName string `json:"displayName"`
					State       string `json:"state"`
					Zone        string `json:"zone"`
					Model       struct {
						Name   string `json:"name"`
						Memory int    `json:"memory"`
						Disk   int    `json:"disk"`
						VCore  int    `json:"vcore"`
					} `json:"model"`
				}
				if err := ovhGet(ctx, cfg, "/vps/"+url.PathEscape(name), &v); err != nil {
					return ops, err
				}
				var ips []string
				_ = ovhGet(ctx, cfg, "/vps/"+url.PathEscape(name)+"/ips", &ips)
				label := v.DisplayName
				if label == "" {
					label = name
				}
				summary := fmt.Sprintf("OVH VPS %s (%d vCore, %d MB RAM, %d GB disk) in %s · %s", v.Model.Name, v.Model.VCore, v.Model.Memory, v.Model.Disk, v.Zone, name)
				ops = append(ops, Op{Action: "create", Entity: "host", Ref: label, Note: summary, Data: map[string]string{
					"kind": "vps", "location": "OVH " + v.Zone, "address": firstIPv4(ips), "address_kind": "public",
					"status": map[string]string{"stopped": "stopped"}[v.State], "summary": summary, "tags": "ovh",
				}})
			}
			var dedicated []string
			if err := ovhGet(ctx, cfg, "/dedicated/server", &dedicated); err == nil {
				for _, name := range dedicated {
					var d struct {
						Reverse    string `json:"reverse"`
						IP         string `json:"ip"`
						Datacenter string `json:"datacenter"`
						State      string `json:"state"`
						OS         string `json:"os"`
					}
					if err := ovhGet(ctx, cfg, "/dedicated/server/"+url.PathEscape(name), &d); err != nil {
						continue
					}
					summary := "OVH dedicated server " + name + " in " + d.Datacenter
					ops = append(ops, Op{Action: "create", Entity: "host", Ref: name, Note: summary, Data: map[string]string{
						"kind": "bare", "location": "OVH " + d.Datacenter, "address": d.IP, "address_kind": "public",
						"os": d.OS, "summary": summary, "tags": "ovh",
					}})
				}
			}
			return ops, nil
		},
	}
}

// ovhGet signs a GET as OVH requires: "$1$" + sha1(secret+consumer+method+url+body+timestamp).
func ovhGet(ctx context.Context, cfg Config, path string, out any) error {
	base := OVHEndpoints[strings.TrimSpace(cfg["endpoint"])]
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(cfg["endpoint"]), "/")
	}
	if !strings.HasPrefix(base, "http") {
		return errors.New("OVH region must be ovh-eu, ovh-ca, ovh-us or an https:// API URL")
	}
	full := base + path
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := sha1.Sum([]byte(cfg["app_secret"] + "+" + cfg["consumer_key"] + "+GET+" + full + "++" + ts)) //nolint:gosec
	return getJSON(ctx, req{URL: full, What: "OVH", Header: map[string]string{
		"X-Ovh-Application": strings.TrimSpace(cfg["app_key"]), "X-Ovh-Consumer": strings.TrimSpace(cfg["consumer_key"]),
		"X-Ovh-Timestamp": ts, "X-Ovh-Signature": "$1$" + hex.EncodeToString(h[:]),
	}}, out)
}

func firstIPv4(ips []string) string {
	for _, ip := range ips {
		ip = strings.SplitN(ip, "/", 2)[0]
		if strings.Count(ip, ".") == 3 {
			return ip
		}
	}
	return ""
}

// costOp proposes a server's monthly price as a cost tied to the host.
func costOp(host, provider, source string, cents int64, currency, plan string) Op {
	return Op{Action: "create", Entity: "cost", Ref: host, Note: fmt.Sprintf("%s %s: %.2f %s / month", provider, plan, float64(cents)/100, currency),
		Data: map[string]string{"label": host, "host": host, "amount_cents": strconv.FormatInt(cents, 10), "currency": currency,
			"cycle": "monthly", "provider": provider, "notes": provider + " " + plan + " (list price from the API)", "source": "plugin:" + source}}
}
