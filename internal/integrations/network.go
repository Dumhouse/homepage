package integrations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Routers, DNS filters and reverse proxies know which name has which
// address and which domain goes where:
//
//   - DHCP leases and local DNS fill in the LAN address of hosts that have
//     none yet (never overriding one you entered); unknown clients are skipped
//   - the router/controller itself and its network devices become hosts
//   - proxy routes become service links (domain → host:port), like Caddy

// leaseOp proposes name → ip for a documented host without a LAN address.
func leaseOp(name, ip, source string) (Op, bool) {
	name = strings.TrimSpace(name)
	if name == "" || name == "*" || ip == "" || strings.Contains(ip, ":") {
		return Op{}, false
	}
	return Op{Action: "link", Entity: "address", Ref: name, Note: source + ": " + name + " is " + ip,
		Data: map[string]string{"host": name, "address": ip, "kind": "lan", "optional": "1", "only_if_empty": "1"}}, true
}

func leaseOps(pairs [][2]string, source string) []Op {
	ops, seen := []Op{}, map[string]bool{}
	for _, p := range pairs {
		if op, ok := leaseOp(p[0], p[1], source); ok && !seen[strings.ToLower(p[0])] {
			seen[strings.ToLower(p[0])] = true
			ops = append(ops, op)
		}
	}
	return ops
}

func linkOp(domain, address, port, source string) Op {
	return Op{Action: "link", Entity: "service", Ref: domain, Note: source + ": " + domain + " → " + address + ":" + port,
		Data: map[string]string{"domain": domain, "address": address, "port": port, "server": source}}
}

func basicAuth(user, pass string) map[string]string {
	return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))}
}

var insecureField = Field{Key: "insecure", Label: "Accept self-signed certificate", Kind: "bool", Default: "1"}

// ---- UniFi ----

func UniFi() *Plugin {
	return &Plugin{
		ID: "unifi", Name: "UniFi Network", Category: CatNetwork,
		Description: "Import UniFi gateways, switches and access points as hosts, and fill in the LAN address of documented hosts from the client list.",
		Fields: []Field{
			{Key: "url", Label: "Console URL", Kind: "url", Required: true, Help: "Your UniFi OS console (UDM, Cloud Key, UCG), e.g. https://192.168.1.1"},
			{Key: "token", Label: "API key", Kind: "password", Secret: true, Required: true, Help: "UniFi Network → Settings → Control Plane → Integrations → Create API key (Network 9.0+)."},
			insecureField,
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := unifiSites(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			sites, err := unifiSites(ctx, cfg)
			if err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, s := range sites {
				var devs struct {
					Data []struct {
						Name      string `json:"name"`
						Model     string `json:"model"`
						IPAddress string `json:"ipAddress"`
						State     string `json:"state"`
					} `json:"data"`
				}
				if err := unifiGet(ctx, cfg, "/sites/"+s.ID+"/devices?limit=200", &devs); err != nil {
					return ops, err
				}
				for _, d := range devs.Data {
					op := hostOp(d.Name, d.IPAddress, "bare", "UniFi "+d.Model, "UniFi "+d.Model+" in site "+s.Name, "network,unifi")
					if d.State != "" && d.State != "ONLINE" {
						op.Data["status"] = "stopped"
					}
					ops = append(ops, op)
				}
				var clients struct {
					Data []struct {
						Name      string `json:"name"`
						IPAddress string `json:"ipAddress"`
					} `json:"data"`
				}
				if err := unifiGet(ctx, cfg, "/sites/"+s.ID+"/clients?limit=1000", &clients); err == nil {
					pairs := [][2]string{}
					for _, c := range clients.Data {
						pairs = append(pairs, [2]string{c.Name, c.IPAddress})
					}
					ops = append(ops, leaseOps(pairs, "UniFi")...)
				}
			}
			return ops, nil
		},
	}
}

type unifiSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func unifiSites(ctx context.Context, cfg Config) ([]unifiSite, error) {
	var res struct {
		Data []unifiSite `json:"data"`
	}
	err := unifiGet(ctx, cfg, "/sites", &res)
	return res.Data, err
}

func unifiGet(ctx context.Context, cfg Config, path string, out any) error {
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/proxy/network/integration/v1" + path, What: "UniFi", Insecure: truthy(cfg["insecure"]),
		Header: map[string]string{"X-API-KEY": strings.TrimSpace(cfg["token"])}}, out)
}

// ---- OPNsense ----

func OPNsense() *Plugin {
	return &Plugin{
		ID: "opnsense", Name: "OPNsense", Category: CatNetwork,
		Description: "Import the firewall as a host and fill in LAN addresses of documented hosts from its DHCP leases (ISC DHCP, Kea or Dnsmasq).",
		Fields: []Field{
			{Key: "url", Label: "OPNsense URL", Kind: "url", Required: true, Help: "e.g. https://192.168.1.1"},
			{Key: "key", Label: "API key", Kind: "text", Required: true, Help: "System → Access → Users → (a user with read-only privileges) → API keys → +. The download holds key and secret."},
			{Key: "secret", Label: "API secret", Kind: "password", Secret: true, Required: true},
			insecureField,
		},
		Test: func(ctx context.Context, cfg Config) error {
			var st map[string]any
			return opnGet(ctx, cfg, "/api/core/firmware/status", &st)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var fw struct {
				Product struct {
					Name    string `json:"product_name"`
					Version string `json:"product_version"`
				} `json:"product"`
			}
			if err := opnGet(ctx, cfg, "/api/core/firmware/status", &fw); err != nil {
				return nil, err
			}
			addr := urlHost(cfg["url"])
			osName := strings.TrimSpace(fw.Product.Name + " " + fw.Product.Version)
			if osName == "" {
				osName = "OPNsense"
			}
			ops := []Op{hostOp("opnsense", addr, "bare", osName, "Firewall / router", "network,firewall")}
			pairs := [][2]string{}
			for _, path := range []string{"/api/dhcpv4/leases/searchLease", "/api/kea/leases4/search", "/api/dnsmasq/leases/search"} {
				var res struct {
					Rows []struct {
						Address  string `json:"address"`
						Hostname string `json:"hostname"`
					} `json:"rows"`
				}
				if opnGet(ctx, cfg, path, &res) == nil {
					for _, r := range res.Rows {
						pairs = append(pairs, [2]string{r.Hostname, r.Address})
					}
				}
			}
			return append(ops, leaseOps(pairs, "OPNsense DHCP")...), nil
		},
	}
}

func opnGet(ctx context.Context, cfg Config, path string, out any) error {
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + path, What: "OPNsense", Insecure: truthy(cfg["insecure"]),
		Header: basicAuth(strings.TrimSpace(cfg["key"]), cfg["secret"])}, out)
}

// ---- MikroTik ----

func MikroTik() *Plugin {
	return &Plugin{
		ID: "mikrotik", Name: "MikroTik RouterOS", Category: CatNetwork,
		Description: "Import the router (board, RouterOS version, uptime) and fill in LAN addresses of documented hosts from its DHCP leases. RouterOS 7 REST API.",
		Fields: []Field{
			{Key: "url", Label: "Router URL", Kind: "url", Required: true, Help: "e.g. https://192.168.88.1 — enable the www-ssl (or www) service under IP → Services."},
			{Key: "user", Label: "User", Kind: "text", Required: true, Help: "A user in the read group."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
			insecureField,
		},
		Test: func(ctx context.Context, cfg Config) error {
			var r map[string]any
			return mtGet(ctx, cfg, "/system/resource", &r)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var res struct {
				Version string `json:"version"`
				Uptime  string `json:"uptime"`
				Board   string `json:"board-name"`
			}
			if err := mtGet(ctx, cfg, "/system/resource", &res); err != nil {
				return nil, err
			}
			var id struct {
				Name string `json:"name"`
			}
			_ = mtGet(ctx, cfg, "/system/identity", &id)
			name := orStr(id.Name, "mikrotik")
			addr := urlHost(cfg["url"])
			osName := "RouterOS " + res.Version
			ops := []Op{hostOp(name, addr, "bare", osName, "MikroTik "+res.Board, "network,mikrotik"),
				factsOp(name, addr, map[string]string{"os": osName, "hostname": id.Name, "uptime": res.Uptime})}
			var leases []struct {
				Address  string `json:"address"`
				HostName string `json:"host-name"`
				Comment  string `json:"comment"`
			}
			if mtGet(ctx, cfg, "/ip/dhcp-server/lease", &leases) == nil {
				pairs := [][2]string{}
				for _, l := range leases {
					pairs = append(pairs, [2]string{orStr(l.HostName, l.Comment), l.Address})
				}
				ops = append(ops, leaseOps(pairs, "MikroTik DHCP")...)
			}
			return ops, nil
		},
	}
}

func mtGet(ctx context.Context, cfg Config, path string, out any) error {
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/rest" + path, What: "MikroTik", Insecure: truthy(cfg["insecure"]),
		Header: basicAuth(cfg["user"], cfg["password"])}, out)
}

func orStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ---- OpenWrt ----

func OpenWrt() *Plugin {
	return &Plugin{
		ID: "openwrt", Name: "OpenWrt", Category: CatNetwork,
		Description: "Import the router (model, OpenWrt release, uptime) and fill in LAN addresses of documented hosts from its DHCP leases, over ubus.",
		Fields: []Field{
			{Key: "url", Label: "Router URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.1 — LuCI must be installed (it provides /ubus and luci-rpc)."},
			{Key: "user", Label: "User", Kind: "text", Required: true, Default: "root"},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := ubusLogin(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			sid, err := ubusLogin(ctx, cfg)
			if err != nil {
				return nil, err
			}
			var board struct {
				Hostname string `json:"hostname"`
				Model    string `json:"model"`
				Release  struct {
					Description string `json:"description"`
				} `json:"release"`
			}
			_ = ubusCall(ctx, cfg, sid, "system", "board", &board)
			var info struct {
				Uptime float64 `json:"uptime"`
			}
			_ = ubusCall(ctx, cfg, sid, "system", "info", &info)
			name := orStr(board.Hostname, "openwrt")
			addr := urlHost(cfg["url"])
			ops := []Op{hostOp(name, addr, "bare", board.Release.Description, board.Model, "network,openwrt"),
				factsOp(name, addr, map[string]string{"os": board.Release.Description, "hostname": board.Hostname, "uptime": humanUptime(info.Uptime)})}
			var leases struct {
				Leases []struct {
					Hostname string `json:"hostname"`
					IP       string `json:"ipaddr"`
				} `json:"dhcp_leases"`
			}
			if ubusCall(ctx, cfg, sid, "luci-rpc", "getDHCPLeases", &leases) == nil {
				pairs := [][2]string{}
				for _, l := range leases.Leases {
					pairs = append(pairs, [2]string{l.Hostname, l.IP})
				}
				ops = append(ops, leaseOps(pairs, "OpenWrt DHCP")...)
			}
			return ops, nil
		},
	}
}

func ubusRaw(ctx context.Context, cfg Config, params []any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "call", "params": params})
	var res struct {
		Result []json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := getJSON(ctx, req{Method: "POST", URL: baseURL(cfg["url"]) + "/ubus", What: "OpenWrt", Body: bytes.NewReader(body),
		Header: map[string]string{"Content-Type": "application/json"}}, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, fmt.Errorf("OpenWrt: %s", res.Error.Message)
	}
	if len(res.Result) == 0 {
		return nil, fmt.Errorf("OpenWrt: empty reply")
	}
	var code int
	_ = json.Unmarshal(res.Result[0], &code)
	if code != 0 {
		return nil, fmt.Errorf("OpenWrt: ubus %v failed with code %d (6 = permission denied: check the user's rpcd ACL)", params[1:3], code)
	}
	if len(res.Result) < 2 {
		return json.RawMessage("{}"), nil
	}
	return res.Result[1], nil
}

func ubusLogin(ctx context.Context, cfg Config) (string, error) {
	raw, err := ubusRaw(ctx, cfg, []any{"00000000000000000000000000000000", "session", "login",
		map[string]string{"username": cfg["user"], "password": cfg["password"]}})
	if err != nil {
		if strings.Contains(err.Error(), "code 6") {
			return "", fmt.Errorf("OpenWrt refused the user or password")
		}
		return "", err
	}
	var s struct {
		SID string `json:"ubus_rpc_session"`
	}
	_ = json.Unmarshal(raw, &s)
	return s.SID, nil
}

func ubusCall(ctx context.Context, cfg Config, sid, obj, method string, out any) error {
	raw, err := ubusRaw(ctx, cfg, []any{sid, obj, method, map[string]any{}})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// ---- Pi-hole ----

func PiHole() *Plugin {
	return &Plugin{
		ID: "pihole", Name: "Pi-hole", Category: CatNetwork,
		Description: "Read Pi-hole's local DNS records and DHCP leases (v6 API) to fill in LAN addresses of documented hosts.",
		Fields: []Field{
			{Key: "url", Label: "Pi-hole URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.2"},
			{Key: "password", Label: "App password", Kind: "password", Secret: true, Required: true, Help: "Settings → Web interface / API → Configure app password. Works with 2-factor login on."},
			insecureField,
		},
		Test: func(ctx context.Context, cfg Config) error {
			sid, err := piholeLogin(ctx, cfg)
			if err == nil {
				piholeLogout(ctx, cfg, sid)
			}
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			sid, err := piholeLogin(ctx, cfg)
			if err != nil {
				return nil, err
			}
			defer piholeLogout(ctx, cfg, sid)
			h := map[string]string{"X-FTL-SID": sid}
			base := baseURL(cfg["url"])
			pairs := [][2]string{}
			var dns struct {
				Config struct {
					DNS struct {
						Hosts []string `json:"hosts"`
					} `json:"dns"`
				} `json:"config"`
			}
			if err := getJSON(ctx, req{URL: base + "/api/config/dns/hosts", What: "Pi-hole", Header: h, Insecure: truthy(cfg["insecure"])}, &dns); err != nil {
				return nil, err
			}
			for _, line := range dns.Config.DNS.Hosts {
				if f := strings.Fields(line); len(f) >= 2 {
					pairs = append(pairs, [2]string{f[1], f[0]})
				}
			}
			var leases struct {
				Leases []struct {
					IP   string `json:"ip"`
					Name string `json:"name"`
				} `json:"leases"`
			}
			if getJSON(ctx, req{URL: base + "/api/dhcp/leases", What: "Pi-hole", Header: h, Insecure: truthy(cfg["insecure"])}, &leases) == nil {
				for _, l := range leases.Leases {
					pairs = append(pairs, [2]string{l.Name, l.IP})
				}
			}
			return leaseOps(pairs, "Pi-hole"), nil
		},
	}
}

func piholeLogin(ctx context.Context, cfg Config) (string, error) {
	body, _ := json.Marshal(map[string]string{"password": cfg["password"]})
	var res struct {
		Session struct {
			Valid   bool   `json:"valid"`
			SID     string `json:"sid"`
			Message string `json:"message"`
		} `json:"session"`
	}
	if err := getJSON(ctx, req{Method: "POST", URL: baseURL(cfg["url"]) + "/api/auth", What: "Pi-hole", Body: bytes.NewReader(body),
		Insecure: truthy(cfg["insecure"]), Header: map[string]string{"Content-Type": "application/json"}}, &res); err != nil {
		if strings.Contains(err.Error(), "404") {
			return "", errors.New("this Pi-hole has no /api/auth — it needs Pi-hole v6 or newer")
		}
		return "", err
	}
	if !res.Session.Valid {
		return "", fmt.Errorf("the Pi-hole password was refused (%s)", res.Session.Message)
	}
	return res.Session.SID, nil
}

func piholeLogout(ctx context.Context, cfg Config, sid string) {
	_ = getJSON(ctx, req{Method: "DELETE", URL: baseURL(cfg["url"]) + "/api/auth", What: "Pi-hole", Insecure: truthy(cfg["insecure"]),
		Header: map[string]string{"X-FTL-SID": sid}}, nil)
}

// ---- AdGuard Home ----

func AdGuard() *Plugin {
	return &Plugin{
		ID: "adguard", Name: "AdGuard Home", Category: CatNetwork,
		Description: "Read AdGuard Home's DNS rewrites and DHCP leases to fill in LAN addresses of documented hosts.",
		Fields: []Field{
			{Key: "url", Label: "AdGuard Home URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.2:3000"},
			{Key: "user", Label: "User", Kind: "text", Required: true},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var st map[string]any
			return agGet(ctx, cfg, "/control/status", &st)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var rewrites []struct {
				Domain string `json:"domain"`
				Answer string `json:"answer"`
			}
			if err := agGet(ctx, cfg, "/control/rewrite/list", &rewrites); err != nil {
				return nil, err
			}
			pairs := [][2]string{}
			for _, r := range rewrites {
				if !strings.HasPrefix(r.Domain, "*") {
					pairs = append(pairs, [2]string{r.Domain, r.Answer})
				}
			}
			var dhcp struct {
				Leases []struct {
					IP       string `json:"ip"`
					Hostname string `json:"hostname"`
				} `json:"leases"`
				Static []struct {
					IP       string `json:"ip"`
					Hostname string `json:"hostname"`
				} `json:"static_leases"`
			}
			if agGet(ctx, cfg, "/control/dhcp/status", &dhcp) == nil {
				for _, l := range append(dhcp.Static, dhcp.Leases...) {
					pairs = append(pairs, [2]string{l.Hostname, l.IP})
				}
			}
			return leaseOps(pairs, "AdGuard Home"), nil
		},
	}
}

func agGet(ctx context.Context, cfg Config, path string, out any) error {
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + path, What: "AdGuard Home", Header: basicAuth(cfg["user"], cfg["password"])}, out)
}

// ---- Nginx Proxy Manager ----

func NginxProxyManager() *Plugin {
	return &Plugin{
		ID: "npm", Name: "Nginx Proxy Manager", Category: CatNetwork,
		Description: "Import proxy hosts as service links: each domain is attached to the service at its forward host and port.",
		Fields: []Field{
			{Key: "url", Label: "NPM URL", Kind: "url", Required: true, Help: "The admin UI, e.g. http://192.168.1.5:81"},
			{Key: "email", Label: "User e-mail", Kind: "text", Required: true},
			{Key: "password", Label: "Password", Kind: "password", Secret: true, Required: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			_, err := npmToken(ctx, cfg)
			return err
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			tok, err := npmToken(ctx, cfg)
			if err != nil {
				return nil, err
			}
			var hosts []struct {
				Domains []string `json:"domain_names"`
				Host    string   `json:"forward_host"`
				Port    int      `json:"forward_port"`
				Enabled any      `json:"enabled"`
			}
			if err := getJSON(ctx, req{URL: baseURL(cfg["url"]) + "/api/nginx/proxy-hosts", What: "Nginx Proxy Manager",
				Header: map[string]string{"Authorization": "Bearer " + tok}}, &hosts); err != nil {
				return nil, err
			}
			ops := []Op{}
			for _, h := range hosts {
				if fmt.Sprint(h.Enabled) == "false" || fmt.Sprint(h.Enabled) == "0" {
					continue
				}
				for _, d := range h.Domains {
					ops = append(ops, linkOp(d, h.Host, fmt.Sprint(h.Port), "Nginx Proxy Manager"))
				}
			}
			return ops, nil
		},
	}
}

func npmToken(ctx context.Context, cfg Config) (string, error) {
	body, _ := json.Marshal(map[string]string{"identity": strings.TrimSpace(cfg["email"]), "secret": cfg["password"]})
	var res struct {
		Token string `json:"token"`
	}
	err := getJSON(ctx, req{Method: "POST", URL: baseURL(cfg["url"]) + "/api/tokens", What: "Nginx Proxy Manager", Body: bytes.NewReader(body),
		Header: map[string]string{"Content-Type": "application/json"}}, &res)
	return res.Token, err
}

// ---- Traefik ----

var hostRule = regexp.MustCompile(`Host\(([^)]*)\)`)

func Traefik() *Plugin {
	return &Plugin{
		ID: "traefik", Name: "Traefik", Category: CatNetwork,
		Description: "Import HTTP routers as service links: each Host(`…`) rule is attached to the service behind it.",
		Fields: []Field{
			{Key: "url", Label: "Traefik API URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.5:8080 — needs api.insecure=true, or put the dashboard router's URL and credentials here."},
			{Key: "user", Label: "User", Kind: "text", Help: "Only if the API sits behind basic auth."},
			{Key: "password", Label: "Password", Kind: "password", Secret: true},
		},
		Test: func(ctx context.Context, cfg Config) error {
			var v map[string]any
			return trGet(ctx, cfg, "/api/version", &v)
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			var routers []struct {
				Name     string `json:"name"`
				Rule     string `json:"rule"`
				Service  string `json:"service"`
				Provider string `json:"provider"`
				Status   string `json:"status"`
			}
			if err := trGet(ctx, cfg, "/api/http/routers", &routers); err != nil {
				return nil, err
			}
			var services []struct {
				Name string `json:"name"`
				LB   struct {
					Servers []struct {
						URL string `json:"url"`
					} `json:"servers"`
				} `json:"loadBalancer"`
			}
			if err := trGet(ctx, cfg, "/api/http/services", &services); err != nil {
				return nil, err
			}
			backend := map[string]string{}
			for _, s := range services {
				if len(s.LB.Servers) > 0 {
					backend[s.Name] = s.LB.Servers[0].URL
				}
			}
			ops := []Op{}
			for _, r := range routers {
				if r.Status == "disabled" || r.Provider == "internal" {
					continue
				}
				svc := r.Service
				if !strings.Contains(svc, "@") {
					svc += "@" + r.Provider
				}
				u, err := url.Parse(backend[svc])
				if err != nil || u.Hostname() == "" {
					continue
				}
				port := u.Port()
				if port == "" {
					port = map[string]string{"https": "443"}[u.Scheme]
					if port == "" {
						port = "80"
					}
				}
				for _, m := range hostRule.FindAllStringSubmatch(r.Rule, -1) {
					for _, d := range strings.Split(m[1], ",") {
						if d = strings.Trim(strings.TrimSpace(d), "`\""); d != "" {
							ops = append(ops, linkOp(d, u.Hostname(), port, "Traefik"))
						}
					}
				}
			}
			return ops, nil
		},
	}
}

func trGet(ctx context.Context, cfg Config, path string, out any) error {
	h := map[string]string{}
	if cfg["user"] != "" {
		h = basicAuth(cfg["user"], cfg["password"])
	}
	return getJSON(ctx, req{URL: baseURL(cfg["url"]) + path, What: "Traefik", Header: h}, out)
}
