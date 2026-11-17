package integrations_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/integrations"
)

func basic(user, pass string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		u, p, ok := r.BasicAuth()
		return ok && u == user && p == pass
	}
}

func leaseFor(ops []integrations.Op, name string) string {
	if op := find(ops, "address", name); op != nil && op.Data["only_if_empty"] == "1" && op.Data["optional"] == "1" {
		return op.Data["address"]
	}
	return ""
}

func TestUniFi(t *testing.T) {
	srv := serve(t, map[string]string{
		"/proxy/network/integration/v1/sites":            `{"data":[{"id":"s1","name":"Default"}]}`,
		"/proxy/network/integration/v1/sites/s1/devices": `{"data":[{"name":"UDM","model":"UDM Pro","ipAddress":"192.168.1.1","state":"ONLINE"}]}`,
		"/proxy/network/integration/v1/sites/s1/clients": `{"data":[{"name":"nas","ipAddress":"192.168.1.20"},{"name":"","ipAddress":"192.168.1.99"}]}`,
	}, func(r *http.Request) bool { return r.Header.Get("X-API-KEY") == "k" })
	ops, err := integrations.UniFi().Sync(context.Background(), integrations.Config{"url": srv.URL, "token": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if h := find(ops, "host", "UDM"); h == nil || h.Data["address"] != "192.168.1.1" {
		t.Fatalf("%+v", ops)
	}
	if leaseFor(ops, "nas") != "192.168.1.20" || len(ops) != 2 {
		t.Fatalf("leases %+v", ops)
	}
}

func TestOPNsense(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/core/firmware/status":      `{"product":{"product_name":"OPNsense","product_version":"25.1"}}`,
		"/api/kea/leases4/search":        `{"rows":[{"address":"192.168.1.30","hostname":"pve"}]}`,
		"/api/dhcpv4/leases/searchLease": `{"rows":[{"address":"192.168.1.31","hostname":"printer"}]}`,
	}, basic("key", "sec"))
	ops, err := integrations.OPNsense().Sync(context.Background(), integrations.Config{"url": srv.URL, "key": "key", "secret": "sec"})
	if err != nil {
		t.Fatal(err)
	}
	if find(ops, "host", "opnsense").Data["os"] != "OPNsense 25.1" || leaseFor(ops, "pve") != "192.168.1.30" || leaseFor(ops, "printer") == "" {
		t.Fatalf("%+v", ops)
	}
}

func TestMikroTik(t *testing.T) {
	srv := serve(t, map[string]string{
		"/rest/system/resource":      `{"version":"7.16 (stable)","uptime":"3w2d","board-name":"hAP ax3"}`,
		"/rest/system/identity":      `{"name":"gw"}`,
		"/rest/ip/dhcp-server/lease": `[{"address":"192.168.88.10","host-name":"nas"},{"address":"192.168.88.11","comment":"cam"}]`,
	}, basic("ro", "pw"))
	ops, err := integrations.MikroTik().Sync(context.Background(), integrations.Config{"url": srv.URL, "user": "ro", "password": "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "gw"); f == nil || f.Data["uptime"] != "3w2d" {
		t.Fatalf("%+v", ops)
	}
	if leaseFor(ops, "cam") != "192.168.88.11" {
		t.Fatal("comment should name the lease")
	}
}

func TestOpenWrt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Params []any `json:"params"`
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		call := body.Params[1].(string) + "." + body.Params[2].(string)
		switch call {
		case "session.login":
			if body.Params[3].(map[string]any)["password"] != "pw" {
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[6]}`))
				return
			}
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[0,{"ubus_rpc_session":"SID"}]}`))
		case "system.board":
			w.Write([]byte(`{"result":[0,{"hostname":"OpenWrt","model":"GL.iNet MT6000","release":{"description":"OpenWrt 24.10.0"}}]}`))
		case "system.info":
			w.Write([]byte(`{"result":[0,{"uptime":200000}]}`))
		case "luci-rpc.getDHCPLeases":
			w.Write([]byte(`{"result":[0,{"dhcp_leases":[{"hostname":"nas","ipaddr":"192.168.8.20"}]}]}`))
		}
	}))
	defer srv.Close()
	ops, err := integrations.OpenWrt().Sync(context.Background(), integrations.Config{"url": srv.URL, "user": "root", "password": "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if h := find(ops, "host", "OpenWrt"); h == nil || h.Data["os"] != "OpenWrt 24.10.0" || leaseFor(ops, "nas") != "192.168.8.20" {
		t.Fatalf("%+v", ops)
	}
	if err := integrations.OpenWrt().Test(context.Background(), integrations.Config{"url": srv.URL, "user": "root", "password": "no"}); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("bad password: %v", err)
	}
}

func TestPiHole(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/auth":             `{"session":{"valid":true,"sid":"S"}}`,
		"/api/config/dns/hosts": `{"config":{"dns":{"hosts":["192.168.1.20 nas.lan","192.168.1.21 jellyfin.lan"]}}}`,
		"/api/dhcp/leases":      `{"leases":[{"ip":"192.168.1.50","name":"pve"}]}`,
	}, func(r *http.Request) bool { return r.URL.Path == "/api/auth" || r.Header.Get("X-FTL-SID") == "S" })
	ops, err := integrations.PiHole().Sync(context.Background(), integrations.Config{"url": srv.URL, "password": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if leaseFor(ops, "nas.lan") != "192.168.1.20" || leaseFor(ops, "pve") != "192.168.1.50" {
		t.Fatalf("%+v", ops)
	}
}

func TestAdGuard(t *testing.T) {
	srv := serve(t, map[string]string{
		"/control/rewrite/list": `[{"domain":"nas.home","answer":"192.168.1.20"},{"domain":"*.home","answer":"192.168.1.5"}]`,
		"/control/dhcp/status":  `{"leases":[],"static_leases":[{"ip":"192.168.1.40","hostname":"pve"}]}`,
	}, basic("a", "b"))
	ops, err := integrations.AdGuard().Sync(context.Background(), integrations.Config{"url": srv.URL, "user": "a", "password": "b"})
	if err != nil || len(ops) != 2 || leaseFor(ops, "pve") != "192.168.1.40" {
		t.Fatalf("%v %+v", err, ops)
	}
}

func TestNginxProxyManager(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/tokens":            `{"token":"T"}`,
		"/api/nginx/proxy-hosts": `[{"domain_names":["jf.example.com","media.example.com"],"forward_host":"192.168.1.21","forward_port":8096,"enabled":1},{"domain_names":["old.example.com"],"forward_host":"x","forward_port":1,"enabled":0}]`,
	}, func(r *http.Request) bool {
		return r.URL.Path == "/api/tokens" || r.Header.Get("Authorization") == "Bearer T"
	})
	ops, err := integrations.NginxProxyManager().Sync(context.Background(), integrations.Config{"url": srv.URL, "email": "e", "password": "p"})
	if err != nil {
		t.Fatal(err)
	}
	if l := find(ops, "service", "jf.example.com"); len(ops) != 2 || l == nil || l.Action != "link" || l.Data["port"] != "8096" {
		t.Fatalf("%+v", ops)
	}
}

func TestTraefik(t *testing.T) {
	srv := serve(t, map[string]string{
		"/api/http/routers":  "[{\"name\":\"grafana@docker\",\"rule\":\"Host(`grafana.example.com`) || Host(`g.example.com`)\",\"service\":\"grafana\",\"provider\":\"docker\",\"status\":\"enabled\"},{\"name\":\"api@internal\",\"rule\":\"PathPrefix(`/api`)\",\"service\":\"api@internal\",\"provider\":\"internal\"}]",
		"/api/http/services": `[{"name":"grafana@docker","loadBalancer":{"servers":[{"url":"http://192.168.1.30:3000"}]}}]`,
	}, nil)
	ops, err := integrations.Traefik().Sync(context.Background(), integrations.Config{"url": srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if l := find(ops, "service", "g.example.com"); len(ops) != 2 || l == nil || l.Data["address"] != "192.168.1.30" || l.Data["port"] != "3000" {
		t.Fatalf("%+v", ops)
	}
}

func TestScrutiny(t *testing.T) {
	srv := serve(t, map[string]string{"/api/summary": `{"data":{"summary":{
		"w1":{"device":{"device_name":"sda","model_name":"WDC WD40","host_id":"nas","device_status":1},"smart":{"temp":41,"power_on_hours":30000}},
		"w2":{"device":{"device_name":"sdb","model_name":"WDC WD40","host_id":"nas","device_status":0},"smart":{"temp":38}},
		"w3":{"device":{"device_name":"nvme0","host_id":"","device_status":0},"smart":{}},
		"w4":{"device":{"device_name":"sdc","host_id":"pve","device_status":0},"smart":{}}}}}`}, nil)
	ops, err := integrations.Scrutiny().Sync(context.Background(), integrations.Config{"url": srv.URL, "default_host": "mini"})
	if err != nil {
		t.Fatal(err)
	}
	if f := find(ops, "facts", "nas"); f == nil || !strings.Contains(f.Data["error"], "sda (WDC WD40) fails SMART") {
		t.Fatalf("nas %+v", f)
	}
	if f := find(ops, "facts", "pve"); f == nil || f.Data["error"] != "" {
		t.Fatalf("healthy host should clear its warning: %+v", f)
	} else if _, ok := f.Data["error"]; !ok {
		t.Fatal("error key must be present to clear")
	}
	if find(ops, "facts", "mini") == nil {
		t.Fatal("disks without host id go to the default host")
	}
}

func TestOpenMediaVault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Service, Method string
			Params          map[string]any
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		switch body.Service + "." + body.Method {
		case "Session.login":
			if body.Params["password"] != "pw" {
				w.Write([]byte(`{"response":{"authenticated":false,"errmsg":"bad"},"error":null}`))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "X-OPENMEDIAVAULT-SESSIONID", Value: "S1"})
			w.Write([]byte(`{"response":{"authenticated":true},"error":null}`))
		case "System.getInformation":
			if !strings.Contains(r.Header.Get("Cookie"), "S1") {
				w.Write([]byte(`{"response":null,"error":{"code":5001,"message":"Session not authenticated."}}`))
				return
			}
			w.Write([]byte(`{"response":{"hostname":"omv","version":"7.4.1","kernel":"Linux 6.1","uptime":90000,"memTotal":8589934592},"error":null}`))
		case "FileSystemMgmt.enumerateMountedFilesystems":
			w.Write([]byte(`{"response":[{"devicefile":"/dev/sda1","label":"data","percentage":96,"size":"1000","used":"960"}],"error":null}`))
		case "Smart.getList":
			w.Write([]byte(`{"response":{"total":1,"data":[{"devicefile":"/dev/sdb","model":"WDC","overallstatus":"BAD_STATUS"}]},"error":null}`))
		}
	}))
	defer srv.Close()
	cfg := integrations.Config{"url": srv.URL, "user": "admin", "password": "pw"}
	ops, err := integrations.OpenMediaVault().Sync(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	f := find(ops, "facts", "omv")
	if f == nil || f.Data["os"] != "OpenMediaVault 7.4.1" || f.Data["disk_used_pct"] != "96" ||
		!strings.Contains(f.Data["error"], "data is 96% full") || !strings.Contains(f.Data["error"], "/dev/sdb (WDC) SMART BAD_STATUS") {
		t.Fatalf("%+v", f)
	}
	if err := integrations.OpenMediaVault().Test(context.Background(), integrations.Config{"url": srv.URL, "user": "admin", "password": "no"}); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("bad password: %v", err)
	}
}
