package integrations_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/integrations"
)

// serve answers path → body; anything else is a 404.
func serve(t *testing.T, routes map[string]string, check func(*http.Request) bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil && !check(r) {
			w.WriteHeader(401)
			return
		}
		key := r.URL.Path
		if q := r.URL.Query().Get("query"); q != "" {
			key += "?" + q
		}
		if c := r.URL.Query().Get("chart"); c != "" {
			key += "?" + c
		}
		if body, ok := routes[key]; ok {
			w.Write([]byte(body))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func find(ops []integrations.Op, entity, ref string) *integrations.Op {
	for i := range ops {
		if ops[i].Entity == entity && ops[i].Ref == ref {
			return &ops[i]
		}
	}
	return nil
}

func bearer(tok string) func(*http.Request) bool {
	return func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+tok }
}

func TestHetzner(t *testing.T) {
	srv := serve(t, map[string]string{"/servers": `{"servers":[{"name":"web-1","status":"running","labels":{"env":"prod"},
		"public_net":{"ipv4":{"ip":"203.0.113.5"}},"private_net":[{"ip":"10.0.0.2"}],
		"server_type":{"name":"cx22","cores":2,"memory":4,"disk":40,"prices":[{"location":"fsn1","price_monthly":{"gross":"4.5100000000"}},{"location":"hel1","price_monthly":{"gross":"9"}}]},
		"datacenter":{"location":{"name":"fsn1"}},"image":{"description":"Debian 12"}}],"meta":{"pagination":{"next_page":null}}}`}, bearer("tok"))
	old := integrations.HetznerAPI
	integrations.HetznerAPI = srv.URL
	defer func() { integrations.HetznerAPI = old }()
	ops, err := integrations.Hetzner().Sync(context.Background(), integrations.Config{"token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	h := find(ops, "host", "web-1")
	if h == nil || h.Data["kind"] != "vps" || h.Data["address"] != "203.0.113.5" || h.Data["address_kind"] != "public" || h.Data["address2"] != "10.0.0.2" {
		t.Fatalf("host: %+v", ops)
	}
	if c := find(ops, "cost", "web-1"); c == nil || c.Data["amount_cents"] != "451" || c.Data["currency"] != "EUR" {
		t.Fatalf("cost: %+v", c)
	}
	if _, err := integrations.Hetzner().Sync(context.Background(), integrations.Config{"token": "bad"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad token: %v", err)
	}
}

func TestDigitalOcean(t *testing.T) {
	srv := serve(t, map[string]string{"/droplets": `{"droplets":[{"name":"db","status":"active","memory":2048,"vcpus":1,"disk":50,"size_slug":"s-1vcpu-2gb",
		"size":{"price_monthly":12},"region":{"slug":"ams3"},"image":{"distribution":"Ubuntu","name":"24.04"},"tags":["prod"],
		"networks":{"v4":[{"ip_address":"10.110.0.2","type":"private"},{"ip_address":"198.51.100.7","type":"public"}]}}],"links":{}}`}, bearer("t"))
	old := integrations.DigitalOceanAPI
	integrations.DigitalOceanAPI = srv.URL
	defer func() { integrations.DigitalOceanAPI = old }()
	ops, err := integrations.DigitalOcean().Sync(context.Background(), integrations.Config{"token": "t"})
	if err != nil {
		t.Fatal(err)
	}
	if h := find(ops, "host", "db"); h == nil || h.Data["address"] != "198.51.100.7" || h.Data["os"] != "Ubuntu 24.04" {
		t.Fatalf("%+v", ops)
	}
	if c := find(ops, "cost", "db"); c == nil || c.Data["amount_cents"] != "1200" || c.Data["currency"] != "USD" {
		t.Fatalf("cost %+v", c)
	}
}

func TestOVHSignsRequests(t *testing.T) {
	srv := serve(t, map[string]string{
		"/vps":                         `["vps-abc.vps.ovh.net"]`,
		"/vps/vps-abc.vps.ovh.net":     `{"displayName":"mail","state":"running","zone":"Region OpenStack: os-gra7","model":{"name":"vps-value-1-2-40","memory":2048,"disk":40,"vcore":1}}`,
		"/vps/vps-abc.vps.ovh.net/ips": `["2001:db8::1","51.0.0.9"]`,
		"/dedicated/server":            `[]`,
	}, func(r *http.Request) bool {
		return strings.HasPrefix(r.Header.Get("X-Ovh-Signature"), "$1$") && r.Header.Get("X-Ovh-Application") == "ak"
	})
	integrations.OVHEndpoints["test"] = srv.URL
	ops, err := integrations.OVH().Sync(context.Background(), integrations.Config{"endpoint": "test", "app_key": "ak", "app_secret": "as", "consumer_key": "ck"})
	if err != nil {
		t.Fatal(err)
	}
	if h := find(ops, "host", "mail"); h == nil || h.Data["address"] != "51.0.0.9" {
		t.Fatalf("%+v", ops)
	}
}

func TestTailscaleAddsAddressesToKnownHosts(t *testing.T) {
	srv := serve(t, map[string]string{"/tailnet/-/devices": `{"devices":[
		{"hostname":"nas","name":"nas.tail1.ts.net","addresses":["100.64.0.5","fd7a::5"],"os":"linux","tags":["tag:server"]},
		{"hostname":"pixel","name":"pixel.tail1.ts.net","addresses":["100.64.0.9"],"os":"android"}]}`}, bearer("tskey"))
	old := integrations.TailscaleAPI
	integrations.TailscaleAPI = srv.URL
	defer func() { integrations.TailscaleAPI = old }()
	ops, err := integrations.Tailscale().Sync(context.Background(), integrations.Config{"token": "tskey", "tailnet": "-"})
	if err != nil {
		t.Fatal(err)
	}
	a := find(ops, "address", "nas")
	if len(ops) != 2 || a == nil || a.Data["address"] != "100.64.0.5" || a.Data["kind"] != "tailscale" || a.Data["optional"] != "1" {
		t.Fatalf("%+v", ops)
	}
	ops, _ = integrations.Tailscale().Sync(context.Background(), integrations.Config{"token": "tskey", "add_missing": "1"})
	if h := find(ops, "host", "pixel"); h == nil || h.Data["address_kind"] != "tailscale" {
		t.Fatalf("add missing: %+v", ops)
	}
}

func TestHeadscale(t *testing.T) {
	srv := serve(t, map[string]string{"/api/v1/node": `{"nodes":[{"name":"x","givenName":"proxy","ipAddresses":["100.64.0.2"],"online":true}]}`}, bearer("k"))
	ops, err := integrations.Headscale().Sync(context.Background(), integrations.Config{"url": srv.URL, "token": "k"})
	if err != nil || find(ops, "address", "proxy") == nil {
		t.Fatalf("%v %+v", err, ops)
	}
}

func TestPrometheusFacts(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/v1/query?node_uname_info":                 `{"status":"success","data":{"result":[{"metric":{"instance":"192.168.1.240:9100","job":"node","nodename":"webapp","release":"6.8.12","sysname":"Linux"},"value":[0,"1"]}]}}`,
		"/api/v1/query?node_os_info":                    `{"status":"success","data":{"result":[{"metric":{"instance":"192.168.1.240:9100","pretty_name":"Debian GNU/Linux 12"},"value":[0,"1"]}]}}`,
		"/api/v1/query?time() - node_boot_time_seconds": `{"status":"success","data":{"result":[{"metric":{"instance":"192.168.1.240:9100"},"value":[0,"864000"]}]}}`,
		`/api/v1/query?100 - 100 * node_filesystem_avail_bytes{mountpoint="/",fstype!="rootfs"} / node_filesystem_size_bytes{mountpoint="/",fstype!="rootfs"}`: `{"status":"success","data":{"result":[{"metric":{"instance":"192.168.1.240:9100"},"value":[0,"41.6"]}]}}`,
		"/api/v1/query?node_reboot_required": `{"status":"success","data":{"result":[]}}`,
	}, nil)
	ops, err := integrations.Prometheus().Sync(context.Background(), integrations.Config{"url": srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	f := find(ops, "facts", "webapp")
	if f == nil || f.Data["address"] != "192.168.1.240" || f.Data["os"] != "Debian GNU/Linux 12" || f.Data["kernel"] != "6.8.12" ||
		f.Data["uptime"] != "10 days" || f.Data["disk_used_pct"] != "42" {
		t.Fatalf("facts: %+v", f)
	}
	if find(ops, "host", "webapp") == nil {
		t.Fatal("host op missing")
	}
}

func TestNetdataParentAndChildren(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/v1/info":                           `{"os_name":"Debian GNU/Linux","os_version":"12","kernel_version":"6.1","mirrored_hosts":["parent","child1"]}`,
		"/host/child1/api/v1/info":               `{"os_name":"Ubuntu","os_version":"24.04","kernel_version":"6.8"}`,
		"/api/v1/data?system.uptime":             `{"labels":["time","uptime"],"data":[[1,7200]]}`,
		"/api/v1/data?disk_space./":              `{"labels":["time","avail","used","reserved for root"],"data":[[1,60,30,10]]}`,
		"/host/child1/api/v1/data?system.uptime": `{"data":[[1,300000]]}`,
	}, nil)
	ops, err := integrations.Netdata().Sync(context.Background(), integrations.Config{"urls": srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]
	if f := find(ops, "facts", "parent"); f == nil || f.Data["disk_used_pct"] != "30" || f.Data["uptime"] != "2 h" || f.Data["address"] != host {
		t.Fatalf("parent: %+v", f)
	}
	if f := find(ops, "facts", "child1"); f == nil || f.Data["os"] != "Ubuntu 24.04" || f.Data["uptime"] != "3 days" {
		t.Fatalf("child: %+v", f)
	}
}

func TestBeszel(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/collections/users/auth-with-password": `{"token":"pbtok"}`,
		"/api/collections/systems/records":          `{"items":[{"name":"nas","host":"192.168.1.20","status":"down","info":{"h":"nas","k":"6.6","u":90000,"dp":71.2,"m":"N100","c":4}}]}`,
	}, func(r *http.Request) bool {
		return strings.HasSuffix(r.URL.Path, "auth-with-password") || r.Header.Get("Authorization") == "pbtok"
	})
	ops, err := integrations.Beszel().Sync(context.Background(), integrations.Config{"url": srv.URL, "email": "a@b", "password": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "nas"); f == nil || f.Data["disk_used_pct"] != "71" || !strings.Contains(f.Data["error"], "down") {
		t.Fatalf("%+v", f)
	}
}

func TestGlancesFallsBackToV3(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/3/system": `{"hostname":"pi","os_name":"Linux","os_version":"6.6.31","linux_distro":"Raspbian 12"}`,
		"/api/3/uptime": `"21 days, 4:01:02"`,
		"/api/3/fs":     `[{"mnt_point":"/boot","percent":10},{"mnt_point":"/","percent":55.4}]`,
	}, nil)
	ops, err := integrations.Glances().Sync(context.Background(), integrations.Config{"urls": srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "pi"); f == nil || f.Data["uptime"] != "21 days" || f.Data["disk_used_pct"] != "55" || f.Data["os"] != "Raspbian 12" {
		t.Fatalf("%+v", f)
	}
}

func TestTrueNAS(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/v2.0/system/info": `{"version":"TrueNAS-SCALE-24.10","hostname":"vault","physmem":34359738368,"uptime_seconds":500000}`,
		"/api/v2.0/pool":        `[{"name":"tank","status":"DEGRADED","healthy":false},{"name":"fast","status":"ONLINE","healthy":true}]`,
		"/api/v2.0/service":     `[{"service":"cifs","state":"RUNNING"},{"service":"nfs","state":"STOPPED"}]`,
	}, bearer("k"))
	ops, err := integrations.TrueNAS().Sync(context.Background(), integrations.Config{"url": srv.URL, "token": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "vault"); f == nil || !strings.Contains(f.Data["error"], "tank is DEGRADED") {
		t.Fatalf("facts %+v", f)
	}
	if s := find(ops, "service", "SMB"); s == nil || s.Data["port"] != "445" || s.Data["host"] != "vault" {
		t.Fatalf("smb %+v", ops)
	}
	if find(ops, "service", "NFS") != nil {
		t.Fatal("stopped NFS should not be proposed")
	}
}

func TestSynology(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("api") + "/" + r.Form.Get("method") {
		case "SYNO.API.Auth/login":
			if r.Form.Get("passwd") != "pw" {
				w.Write([]byte(`{"success":false,"error":{"code":400}}`))
				return
			}
			w.Write([]byte(`{"success":true,"data":{"sid":"S"}}`))
		case "SYNO.DSM.Info/getinfo":
			w.Write([]byte(`{"success":true,"data":{"model":"DS920+","ram":4096,"version_string":"DSM 7.2.1","uptime":200000}}`))
		case "SYNO.Storage.CGI.Storage/load_info":
			w.Write([]byte(`{"success":true,"data":{"volumes":[{"id":"volume_1","status":"normal","size":{"total":"1000000000000","used":"640000000000"}}]}}`))
		default:
			w.Write([]byte(`{"success":true,"data":{}}`))
		}
	}))
	defer srv.Close()
	ops, err := integrations.Synology().Sync(context.Background(), integrations.Config{"url": srv.URL, "user": "u", "password": "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "synology-ds920+"); f == nil || f.Data["disk_used_pct"] != "64" {
		t.Fatalf("%+v", ops)
	}
	if err := integrations.Synology().Test(context.Background(), integrations.Config{"url": srv.URL, "user": "u", "password": "no"}); err == nil || !strings.Contains(err.Error(), "wrong user or password") {
		t.Fatalf("bad login: %v", err)
	}
}

func TestNUT(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				rd := bufio.NewScanner(c)
				for rd.Scan() {
					switch rd.Text() {
					case "LIST UPS":
						c.Write([]byte("BEGIN LIST UPS\nUPS eaton \"Rack UPS\"\nEND LIST UPS\n"))
					case "LIST VAR eaton":
						c.Write([]byte("BEGIN LIST VAR eaton\nVAR eaton device.mfr \"EATON\"\nVAR eaton device.model \"5E 1100i\"\nVAR eaton ups.status \"OB DISCHRG\"\nVAR eaton battery.charge \"87\"\nVAR eaton ups.load \"23\"\nVAR eaton battery.runtime \"1260\"\nEND LIST VAR eaton\n"))
					}
				}
			}(c)
		}
	}()
	ops, err := integrations.NUT().Sync(context.Background(), integrations.Config{"address": ln.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	h := find(ops, "host", "ups-eaton")
	if h == nil || !strings.Contains(h.Data["summary"], "battery 87%") || !strings.Contains(h.Data["summary"], "21 min") {
		t.Fatalf("%+v", ops)
	}
	if f := find(ops, "facts", "ups-eaton"); f == nil || f.Data["error"] != "on battery" {
		t.Fatalf("facts %+v", f)
	}
}

func TestRedfish(t *testing.T) {
	srv := serve(t, map[string]string{
		"/redfish/v1/Systems":         `{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`,
		"/redfish/v1/Systems/1":       `{"Manufacturer":"Dell Inc.","Model":"PowerEdge R730","HostName":"pve2","PowerState":"On","Status":{"Health":"Warning"},"MemorySummary":{"TotalSystemMemoryGiB":128},"ProcessorSummary":{"Count":2,"Model":"Xeon E5-2680"}}`,
		"/redfish/v1/Chassis":         `{"Members":[{"@odata.id":"/redfish/v1/Chassis/1"}]}`,
		"/redfish/v1/Chassis/1/Power": `{"PowerControl":[{"PowerConsumedWatts":184}]}`,
	}, func(r *http.Request) bool {
		u, p, ok := r.BasicAuth()
		return ok && u == "ro" && p == "pw"
	})
	ops, err := integrations.Redfish().Sync(context.Background(), integrations.Config{"url": srv.URL, "user": "ro", "password": "pw"})
	if err != nil {
		t.Fatal(err)
	}
	h := find(ops, "host", "pve2")
	if h == nil || !strings.Contains(h.Data["summary"], "184 W") || h.Data["address2_kind"] != "other" {
		t.Fatalf("%+v", ops)
	}
	if f := find(ops, "facts", "pve2"); f == nil || !strings.Contains(f.Data["error"], "Warning") {
		t.Fatalf("facts %+v", f)
	}
}

func TestConnectionRefusedIsExplained(t *testing.T) {
	_, err := integrations.Prometheus().Sync(context.Background(), integrations.Config{"url": "http://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "connection refused") || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("%v", err)
	}
}
