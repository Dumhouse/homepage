package integrations_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/integrations"
)

func TestFileSecrets(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "secrets.enc")
	f, err := integrations.NewFileSecrets(path, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Set(ctx, "caddy.token", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Get(ctx, "caddy.token"); v != "s3cret" {
		t.Errorf("get: %q", v)
	}
	// reload with the same key
	g, err := integrations.NewFileSecrets(path, "correct horse battery staple")
	if err != nil || g == nil {
		t.Fatal(err)
	}
	if v, _ := g.Get(ctx, "caddy.token"); v != "s3cret" {
		t.Errorf("persisted: %q", v)
	}
	if _, err := integrations.NewFileSecrets(path, "wrong key"); err == nil {
		t.Error("wrong master key should fail")
	}
	if err := g.Set(ctx, "caddy.token", ""); err != nil {
		t.Fatal(err)
	}
	if v, _ := g.Get(ctx, "caddy.token"); v != "" {
		t.Errorf("delete failed: %q", v)
	}
	if _, err := integrations.NewFileSecrets(path, ""); !errors.Is(err, integrations.ErrNoMasterKey) {
		t.Errorf("empty key: %v", err)
	}
}

func TestManager(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	called := ""
	p := &integrations.Plugin{
		ID: "fake", Name: "Fake", Description: "test plugin",
		Fields: []integrations.Field{
			{Key: "url", Label: "URL", Kind: "url", Required: true},
			{Key: "token", Label: "Token", Kind: "password", Secret: true},
		},
		Test: func(ctx context.Context, c integrations.Config) error {
			if c["token"] != "tkn" || c["url"] == "" {
				return errors.New("bad config")
			}
			return nil
		},
		Sync: func(ctx context.Context, c integrations.Config) ([]integrations.Op, error) {
			called = c["url"]
			return []integrations.Op{{Action: "create", Entity: "domain", Ref: "example.com"}}, nil
		},
	}
	m.Register(p)

	list, err := m.List(ctx)
	if err != nil || len(list) != 1 || list[0].Configured() {
		t.Fatalf("new integration should not be configured: %v %+v", err, list)
	}
	if err := m.Save(ctx, "ui", "fake", map[string]string{"url": "http://x", "token": "tkn"}); err != nil {
		t.Fatal(err)
	}
	list, err = m.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %+v", err, list)
	}
	st := list[0]
	if st.Config["url"] != "http://x" || !st.HasSecret["token"] || !st.Configured() {
		t.Errorf("state: %+v", st)
	}
	if _, ok := st.Config["token"]; ok {
		t.Error("secret leaked into config")
	}
	if err := m.Test(ctx, "fake"); err != nil {
		t.Fatalf("test: %v", err)
	}
	ops, err := m.Sync(ctx, "fake")
	if err != nil || len(ops) != 1 || called != "http://x" {
		t.Fatalf("sync: %v %+v %q", err, ops, called)
	}
	list, _ = m.List(ctx)
	if len(list[0].Ops) != 1 || list[0].LastSyncAt == "" || list[0].LastTestAt == "" {
		t.Errorf("recorded state: %+v", list[0])
	}
	if err := m.Save(ctx, "ui", "fake", map[string]string{"url": "", "token": "tkn"}); !errors.Is(err, integrations.ErrInvalid) {
		t.Errorf("missing required: %v", err)
	}
	if err := m.Save(ctx, "ui", "nope", nil); !errors.Is(err, integrations.ErrInvalid) {
		t.Errorf("unknown plugin: %v", err)
	}
	if err := m.Save(ctx, "ui", "fake", map[string]string{"url": "http://x"}); err != nil {
		t.Errorf("blank secret should keep it: %v", err)
	}
	if err := m.Test(ctx, "fake"); err != nil {
		t.Errorf("secret should persist: %v", err)
	}
}

func TestProxmoxEmptyNodeWithPermissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/json/nodes":
			w.Write([]byte(`{"data":[{"node":"pve","status":"online"}]}`))
		case "/api2/json/cluster/status":
			w.Write([]byte(`{"data":[{"type":"node","name":"pve","ip":"192.168.1.250"}]}`))
		case "/api2/json/access/permissions":
			w.Write([]byte(`{"data":{"/":{"Sys.Audit":1}}}`))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()
	ops, err := integrations.Proxmox().Sync(context.Background(), integrations.Config{"api_url": srv.URL, "token_id": "a@pam!b", "token_secret": "c"})
	if err != nil {
		t.Fatalf("empty but permitted node should not error: %v", err)
	}
	if len(ops) != 1 || ops[0].Ref != "pve" {
		t.Errorf("ops: %+v", ops)
	}
}

func TestCaddyPlugin(t *testing.T) {
	cfg := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"routes": []any{
							map[string]any{
								"match": []any{map[string]any{"host": []any{"app.example.com", "*.wild.example.com"}}},
								"handle": []any{
									map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": "10.0.0.5:3000"}}},
								},
							},
						},
					},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(cfg)
	}))
	defer srv.Close()
	p := integrations.Caddy()
	ctx := context.Background()
	if err := p.Test(ctx, integrations.Config{"admin_url": srv.URL}); err != nil {
		t.Fatalf("test: %v", err)
	}
	ops, err := p.Sync(ctx, integrations.Config{"admin_url": srv.URL})
	if err != nil || len(ops) != 1 {
		t.Fatalf("sync: %v %+v", err, ops)
	}
	if ops[0].Entity != "service" || ops[0].Action != "link" || ops[0].Data["domain"] != "app.example.com" ||
		ops[0].Data["address"] != "10.0.0.5" || ops[0].Data["port"] != "3000" {
		t.Errorf("op: %+v", ops[0])
	}
	if err := p.Test(ctx, integrations.Config{"admin_url": srv.URL + "/nope"}); err == nil {
		t.Error("bad admin url should fail")
	}
}

func TestProxmoxPlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api2/json/version":
			w.Write([]byte(`{"data":{"version":"8.2"}}`))
		case "/api2/json/nodes":
			w.Write([]byte(`{"data":[{"node":"pve","status":"online"}]}`))
		case "/api2/json/cluster/status":
			w.Write([]byte(`{"data":[{"type":"cluster","name":"lab"},{"type":"node","name":"pve","ip":"192.168.1.250"}]}`))
		case "/api2/json/nodes/pve/lxc/101/config":
			w.Write([]byte(`{"data":{"hostname":"acme","net0":"name=eth0,bridge=vmbr0,ip=192.168.1.240/24,gw=192.168.1.1"}}`))
		case "/api2/json/nodes/pve/lxc":
			w.Write([]byte(`{"data":[{"vmid":101,"name":"acme","status":"running","maxmem":1073741824,"maxdisk":8589934592,"cpus":2}]}`))
		case "/api2/json/nodes/pve/qemu":
			w.Write([]byte(`{"data":[{"vmid":200,"name":"build","status":"stopped","maxmem":2147483648,"maxdisk":17179869184,"cpus":4}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := integrations.Proxmox()
	cfg := integrations.Config{"api_url": srv.URL, "token_id": "root@pam!hs", "token_secret": "s3cret", "ignore_tls": "1"}
	ctx := context.Background()
	if err := p.Test(ctx, cfg); err != nil {
		t.Fatalf("test: %v", err)
	}
	ops, err := p.Sync(ctx, cfg)
	if err != nil || len(ops) != 3 {
		t.Fatalf("sync: %v %+v", err, ops)
	}
	if ops[0].Ref != "pve" || ops[0].Data["kind"] != "bare" || ops[0].Data["address"] != "192.168.1.250" {
		t.Errorf("node op: %+v", ops[0])
	}
	if ops[1].Data["kind"] != "lxc" || ops[1].Data["proxmox_vmid"] != "101" || ops[1].Data["address"] != "192.168.1.240" {
		t.Errorf("lxc op: %+v", ops[1])
	}
	if ops[2].Data["kind"] != "vm" || ops[2].Data["status"] != "stopped" {
		t.Errorf("qemu op: %+v", ops[2])
	}
}

// A privilege-separated token without a role sees no guests: say why.
func TestProxmoxTokenWithoutPermissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes":
			w.Write([]byte(`{"data":[]}`))
		default:
			w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()
	_, err := integrations.Proxmox().Sync(context.Background(), integrations.Config{"api_url": srv.URL, "token_id": "a@pam!b", "token_secret": "c"})
	if err == nil || !strings.Contains(err.Error(), "PVEAuditor") {
		t.Fatalf("want a permissions hint, got %v", err)
	}
}

func TestCloudflarePlugin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tkn" {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/user/tokens/verify":
			w.Write([]byte(`{"success":true,"result":{"status":"active"}}`))
		case "/zones":
			w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com","status":"active","name_servers":["ns1.cf.com","ns2.cf.com"]}]}`))
		case "/zones/z1/dns_records":
			w.Write([]byte(`{"success":true,"result":[
				{"name":"app.example.com","type":"A","content":"10.0.0.5","proxied":true},
				{"name":"example.com","type":"MX","content":"mail.example.com","proxied":false}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := integrations.Cloudflare()
	cfg := integrations.Config{"api_base": srv.URL, "api_token": "tkn"}
	ctx := context.Background()
	if err := p.Test(ctx, cfg); err != nil {
		t.Fatalf("test: %v", err)
	}
	ops, err := p.Sync(ctx, cfg)
	if err != nil || len(ops) != 3 {
		t.Fatalf("sync: %v %+v", err, ops)
	}
	if ops[0].Entity != "domain" || ops[0].Ref != "example.com" || ops[0].Data["zone_id"] != "z1" {
		t.Errorf("domain op: %+v", ops[0])
	}
	if ops[1].Entity != "dns_record" || ops[1].Data["domain"] != "example.com" || ops[1].Data["proxied"] != "true" {
		t.Errorf("record op: %+v", ops[1])
	}
	bad := integrations.Config{"api_base": srv.URL, "api_token": "wrong"}
	if err := p.Test(ctx, bad); err == nil {
		t.Error("bad token should fail")
	}
}

func dockerSSHServer(t *testing.T) (string, []byte) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(block)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				sconn, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer sconn.Close()
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					if nc.ChannelType() != "session" {
						_ = nc.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					ch, chReqs, err := nc.Accept()
					if err != nil {
						continue
					}
					go func(ch ssh.Channel, chReqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range chReqs {
							if req.Type != "exec" {
								_ = req.Reply(false, nil)
								continue
							}
							var p struct{ Command string }
							_ = ssh.Unmarshal(req.Payload, &p)
							_ = req.Reply(true, nil)
							out := "24.0.0\n"
							if strings.Contains(p.Command, "docker ps") {
								out = `{"Names":"web","Image":"nginx:latest","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:8080->80/tcp"}` + "\n"
							}
							_, _ = ch.Write([]byte(out))
							_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							return
						}
					}(ch, chReqs)
				}
			}(c)
		}
	}()
	return ln.Addr().String(), keyPEM
}

func TestDockerPlugin(t *testing.T) {
	addr, key := dockerSSHServer(t)
	p := integrations.Docker()
	cfg := integrations.Config{"targets": "deploy@" + addr, "key": string(key)}
	ctx := context.Background()
	if err := p.Test(ctx, cfg); err != nil {
		t.Fatalf("test: %v", err)
	}
	ops, err := p.Sync(ctx, cfg)
	if err != nil || len(ops) != 1 {
		t.Fatalf("sync: %v %+v", err, ops)
	}
	op := ops[0]
	if op.Data["host"] != "127.0.0.1" || op.Data["port"] != "8080" || op.Data["protocol"] != "http" ||
		op.Data["name"] != "web" || op.Data["type"] != "nginx:latest" {
		t.Errorf("op: %+v", op)
	}
	if _, err := p.Sync(ctx, integrations.Config{"targets": "deploy@" + addr}); err == nil {
		t.Error("missing key should fail")
	}
}

// A second copy of a plugin keeps its own settings and secrets.
func TestInstances(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	m := integrations.NewManager(d, integrations.NewMemorySecrets())
	m.Register(&integrations.Plugin{ID: "fake", Name: "Fake",
		Fields: []integrations.Field{{Key: "url", Label: "URL", Kind: "url", Required: true}, {Key: "token", Label: "Token", Kind: "password", Secret: true}},
		Test: func(ctx context.Context, c integrations.Config) error {
			if c["token"] == "" {
				return errors.New("no token")
			}
			return nil
		}})
	ctx := context.Background()
	if err := m.Save(ctx, "t", "fake", map[string]string{"url": "http://a", "token": "A"}); err != nil {
		t.Fatal(err)
	}
	id, err := m.AddInstance(ctx, "t", "fake", "lab")
	if err != nil || id != "fake~2" {
		t.Fatalf("%q %v", id, err)
	}
	if err := m.Test(ctx, id); err == nil {
		t.Fatal("the copy must not inherit the first one's token")
	}
	if err := m.Save(ctx, "t", id, map[string]string{"url": "http://b", "token": "B", "_label": "lab"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Test(ctx, id); err != nil {
		t.Fatal(err)
	}
	states, _ := m.List(ctx)
	if len(states) != 2 || states[0].Name != "Fake" || states[1].Name != "Fake · lab" || states[1].Config["url"] != "http://b" || states[0].Config["url"] != "http://a" {
		t.Fatalf("%+v", states)
	}
	if _, err := m.AddInstance(ctx, "t", "nope", ""); err == nil {
		t.Fatal("unknown plugin")
	}
	if err := m.Reset(ctx, "t", id); err != nil {
		t.Fatal(err)
	}
	if states, _ := m.List(ctx); len(states) != 1 {
		t.Fatalf("copy should be gone: %+v", states)
	}
}
