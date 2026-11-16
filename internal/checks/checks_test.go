package checks_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/dawidof/home-services/internal/alerts"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
)

func newStore(t *testing.T) *checks.Store {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return checks.New(d)
}

func TestTCPAndHTTPProbes(t *testing.T) {
	ctx, p := context.Background(), checks.NewProber()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				close(done)
				return
			}
			c.Close()
		}
	}()
	res := p.Run(ctx, checks.Check{Kind: "tcp", Params: map[string]any{"address": addr, "port": port}})
	if !res.OK || res.LatencyMS < 0 {
		t.Fatalf("tcp up: %+v", res)
	}
	ln.Close()
	<-done
	if res := p.Run(ctx, checks.Check{Kind: "tcp", Params: map[string]any{"address": addr, "port": port}}); res.OK {
		t.Errorf("closed port should fail: %+v", res)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte("hello acme"))
	}))
	defer srv.Close()
	for _, c := range []struct {
		params map[string]any
		ok     bool
	}{
		{map[string]any{"url": srv.URL}, true},
		{map[string]any{"url": srv.URL + "/bad"}, false},
		{map[string]any{"url": srv.URL, "keyword": "acme"}, true},
		{map[string]any{"url": srv.URL, "keyword": "missing"}, false},
		{map[string]any{"url": srv.URL + "/bad", "expect_status": "500"}, true},
	} {
		res := p.Run(ctx, checks.Check{Kind: "http", Params: c.params})
		if res.OK != c.ok {
			t.Errorf("http %v: got %+v want ok=%v", c.params, res, c.ok)
		}
	}
}

func TestTLSAndDNSProbes(t *testing.T) {
	ctx, p := context.Background(), checks.NewProber()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	if res := p.Run(ctx, checks.Check{Kind: "tls", Params: map[string]any{"address": srv.Listener.Addr().String()}}); !res.OK {
		t.Errorf("tls: %+v", res)
	}
	if res := p.Run(ctx, checks.Check{Kind: "tls", Params: map[string]any{"address": srv.Listener.Addr().String(), "warn_days": 100000}}); res.OK {
		t.Errorf("tls warn window should fail: %+v", res)
	}
	if res := p.Run(ctx, checks.Check{Kind: "tls", Params: map[string]any{"address": "127.0.0.1:1"}}); res.OK {
		t.Errorf("tls unreachable should fail: %+v", res)
	}
	if res := p.Run(ctx, checks.Check{Kind: "dns", Params: map[string]any{"name": "localhost", "expect_ip": "127.0.0.1"}}); !res.OK {
		t.Errorf("dns: %+v", res)
	}
	if res := p.Run(ctx, checks.Check{Kind: "dns", Params: map[string]any{"name": "localhost", "expect_ip": "10.255.255.1"}}); res.OK {
		t.Errorf("dns mismatch should fail: %+v", res)
	}
}

func sshTestServer(t *testing.T, password, key bool) (string, func()) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{}
	cfg.AddHostKey(signer)
	if password {
		cfg.PasswordCallback = func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, errors.New("denied")
		}
	}
	if key {
		cfg.PublicKeyCallback = func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, errors.New("denied")
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _, _, _ = ssh.NewServerConn(c, cfg)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestSSHProbe(t *testing.T) {
	ctx, p := context.Background(), checks.NewProber()
	addr, closeSrv := sshTestServer(t, true, true)
	defer closeSrv()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	res := p.Run(ctx, checks.Check{Kind: "ssh", Params: map[string]any{"address": host, "port": port, "user": "probe"}})
	if res.SSHStatus != "password_allowed" || !res.OK {
		t.Errorf("password server: %+v", res)
	}

	keyAddr, closeKey := sshTestServer(t, false, true)
	defer closeKey()
	kh, kp, _ := net.SplitHostPort(keyAddr)
	kport, _ := strconv.Atoi(kp)
	res = p.Run(ctx, checks.Check{Kind: "ssh", Params: map[string]any{"address": kh, "port": kport, "user": "probe"}})
	if res.SSHStatus != "key_only" || !res.OK {
		t.Errorf("key-only server: %+v", res)
	}

	res = p.Run(ctx, checks.Check{Kind: "ssh", Params: map[string]any{"address": "127.0.0.1", "port": 1}})
	if res.SSHStatus != "unreachable" || res.OK {
		t.Errorf("unreachable server: %+v", res)
	}
}

func TestStateMachineAndRecord(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c, err := s.Create(ctx, "test", checks.Check{TargetEntity: "service", TargetID: 1, Kind: "http", Params: map[string]any{"url": "http://x"}})
	if err != nil {
		t.Fatal(err)
	}
	fail := checks.Result{Message: "boom"}
	ok := checks.Result{OK: true, LatencyMS: 12, Message: "fine"}
	for i, want := range []string{"failing", "failing", "down"} {
		if err := s.Record(ctx, *c, fail); err != nil {
			t.Fatal(err)
		}
		c, _ = s.Get(ctx, c.ID)
		if c.State != want || c.FailCount != i+1 {
			t.Errorf("failure %d: %+v want %s", i+1, c, want)
		}
	}
	if err := s.Record(ctx, *c, ok); err != nil {
		t.Fatal(err)
	}
	c, _ = s.Get(ctx, c.ID)
	if c.State != "recovering" {
		t.Errorf("first success after down = recovering: %+v", c)
	}
	if err := s.Record(ctx, *c, ok); err != nil {
		t.Fatal(err)
	}
	c, _ = s.Get(ctx, c.ID)
	if c.State != "up" || c.LastOKAt == nil || c.LastError != "" {
		t.Errorf("second success = up: %+v", c)
	}
	recent, err := s.Recent(ctx, c.ID, 10)
	if err != nil || len(recent) != 5 {
		t.Errorf("history: %v %d", err, len(recent))
	}
	problems, _ := s.ListProblems(ctx, 10)
	if len(problems) != 0 {
		t.Errorf("up check should not be a problem: %+v", problems)
	}
}

func TestReconcileAndScheduler(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	inv := inventory.New(d)
	cstore := checks.New(d)

	h, err := inv.CreateHost(ctx, "test", inventory.Host{
		Name: "acme", Addresses: []inventory.Address{{Address: "10.0.0.5"}},
		SSH: []inventory.SSHAccess{{User: "deploy"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: 3000, Protocol: "http"})
	if err != nil {
		t.Fatal(err)
	}
	proj, _ := inv.CreateProject(ctx, "test", inventory.Project{Name: "Acme"})
	env, _ := inv.AddProjectEnv(ctx, "test", proj.ID, inventory.ProjectEnv{Name: "production", BaseURL: "https://acme.example.com"})
	link, err := inv.AddAppLink(ctx, "test", proj.ID, inventory.AppLink{Label: "Admin", Path: "/admin", HealthCheck: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = env

	n, err := cstore.Reconcile(ctx, inv)
	if err != nil || n != 3 {
		t.Fatalf("reconcile: %v changed %d", err, n)
	}
	if n, err := cstore.Reconcile(ctx, inv); err != nil || n != 0 {
		t.Errorf("second reconcile should be a no-op: %v %d", err, n)
	}
	svcChecks, _ := cstore.List(ctx, "service", svc.ID)
	if len(svcChecks) != 1 || svcChecks[0].Kind != "http" {
		t.Fatalf("service check: %+v", svcChecks)
	}
	if svcChecks[0].Params["url"] != "http://10.0.0.5:3000" {
		t.Errorf("service url: %+v", svcChecks[0].Params)
	}
	sshChecks, _ := cstore.List(ctx, "ssh_access", h.SSH[0].ID)
	if len(sshChecks) != 1 || sshChecks[0].Kind != "ssh" {
		t.Fatalf("ssh check: %+v", sshChecks)
	}
	linkChecks, _ := cstore.List(ctx, "app_link", link.ID)
	if len(linkChecks) != 1 || linkChecks[0].Params["url"] != "https://acme.example.com/admin" {
		t.Fatalf("app link check: %+v", linkChecks)
	}

	// changing the port refreshes the auto params
	svc.Port = 3001
	if _, err := inv.UpdateService(ctx, "test", svc.ID, *svc); err != nil {
		t.Fatal(err)
	}
	if n, _ := cstore.Reconcile(ctx, inv); n != 1 {
		t.Errorf("port change should update one check, got %d", n)
	}

	// scheduler runs due checks against a local HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	if _, err := cstore.Create(ctx, "test", checks.Check{
		TargetEntity: "service", TargetID: 999, Kind: "http", Params: map[string]any{"url": srv.URL},
	}); err != nil {
		t.Fatal(err)
	}
	sched := checks.NewScheduler(d, inv)
	sched.Workers = 2
	// keep the auto checks out of this tick; their targets are not reachable in tests
	if _, err := d.Exec(`UPDATE checks SET last_run_at=datetime('now') WHERE auto=1`); err != nil {
		t.Fatal(err)
	}
	if n, err := sched.Tick(ctx); err != nil || n < 1 {
		t.Fatalf("tick: %v %d", err, n)
	}
	list, _ := cstore.List(ctx, "service", 999)
	if len(list) != 1 || list[0].State != "up" {
		t.Errorf("scheduler result: %+v", list)
	}

	// deleting the service removes its auto check
	if err := inv.DeleteService(ctx, "test", svc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cstore.Reconcile(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if list, _ := cstore.List(ctx, "service", svc.ID); len(list) != 0 {
		t.Errorf("auto check should be removed: %+v", list)
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := checks.New(d)
	c, err := s.Create(ctx, "test", checks.Check{TargetEntity: "host", TargetID: 1, Kind: "tcp", Params: map[string]any{"address": "127.0.0.1", "port": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO check_results(check_id,ok,latency_ms,message,at) VALUES(?,1,12,'old',datetime('now','-31 days'))`, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	var results, okCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM check_results WHERE check_id=?`, c.ID).Scan(&results); err != nil || results != 0 {
		t.Errorf("old raw result should be pruned: %v %d", err, results)
	}
	if err := d.QueryRow(`SELECT ok_count FROM check_rollups WHERE check_id=?`, c.ID).Scan(&okCount); err != nil || okCount != 1 {
		t.Errorf("rollup should keep the count: %v %d", err, okCount)
	}
}

func TestAvailability(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := checks.New(d)
	c, err := s.Create(ctx, "test", checks.Check{TargetEntity: "host", TargetID: 1, Kind: "tcp", Params: map[string]any{"address": "127.0.0.1", "port": 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		at string
		ok int
	}{{"-10 minutes", 1}, {"-20 minutes", 0}, {"-25 hours", 1}} {
		if _, err := d.Exec(`INSERT INTO check_results(check_id,ok,latency_ms,message,at) VALUES(?,?,1,'x',datetime('now',?))`, c.ID, row.ok, row.at); err != nil {
			t.Fatal(err)
		}
	}
	bars, err := s.Availability(ctx, c.ID, 4, 24)
	if err != nil || len(bars) != 4 {
		t.Fatalf("availability: %v %+v", err, bars)
	}
	last := bars[3]
	if last.Class != "bad" || last.Ratio != 0.5 {
		t.Errorf("last bucket should be half-failed and bad: %+v", last)
	}
	for i := 0; i < 3; i++ {
		if bars[i].Class != "empty" {
			t.Errorf("bucket %d should be empty: %+v", i, bars[i])
		}
	}
	empty, _ := s.Availability(ctx, 0, 2, 24)
	if len(empty) != 2 || empty[0].Class != "empty" {
		t.Errorf("no check should give empty bars: %+v", empty)
	}
}

type captureNotifier struct{ events []string }

func (c *captureNotifier) Name() string { return "capture" }
func (c *captureNotifier) Send(ctx context.Context, e alerts.Event) error {
	c.events = append(c.events, e.State+":"+e.Label)
	return nil
}

func TestRunnerAlertsOnTransitions(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	store := checks.New(d)
	broken := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, err := store.Create(ctx, "test", checks.Check{
		TargetEntity: "service", TargetID: 7, Kind: "http", Params: map[string]any{"url": srv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := checks.NewRunner(store)
	n := &captureNotifier{}
	runner.SetNotifier(n)
	runner.SetLabel(func(context.Context, string, int64) string { return "acme/web" })

	for i := 0; i < 3; i++ {
		if _, err := runner.RunNow(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.events) != 1 || n.events[0] != "down:acme/web" {
		t.Fatalf("expected one down alert, got %+v", n.events)
	}
	broken = false
	for i := 0; i < 2; i++ {
		if _, err := runner.RunNow(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.events) != 2 || n.events[1] != "up:acme/web" {
		t.Fatalf("expected recovery alert, got %+v", n.events)
	}
}

func TestCheckCounts(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	upSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer upSrv.Close()
	runner := checks.NewRunner(s)
	up, _ := s.Create(ctx, "test", checks.Check{TargetEntity: "service", TargetID: 1, Kind: "http", Params: map[string]any{"url": upSrv.URL}})
	if _, err := runner.RunNow(ctx, up.ID); err != nil {
		t.Fatal(err)
	}
	down, _ := s.Create(ctx, "test", checks.Check{TargetEntity: "service", TargetID: 2, Kind: "tcp", Params: map[string]any{"address": "127.0.0.1", "port": 1}})
	for i := 0; i < 3; i++ {
		if _, err := runner.RunNow(ctx, down.ID); err != nil {
			t.Fatal(err)
		}
	}
	paused, _ := s.Create(ctx, "test", checks.Check{TargetEntity: "host", TargetID: 3, Kind: "tcp", Params: map[string]any{"address": "127.0.0.1", "port": 1}})
	if err := s.SetEnabled(ctx, "test", paused.ID, false); err != nil {
		t.Fatal(err)
	}
	cc, err := s.CheckCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cc.Up != 1 || cc.Down != 1 || cc.Disabled != 1 || cc.Unknown != 0 {
		t.Errorf("counts: %+v", cc)
	}
}

func TestMaintenanceWindows(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC()
	win, err := s.AddMaintenance(ctx, "test", checks.MaintenanceWindow{
		TargetEntity: "service", TargetID: 1,
		StartsAt: now.Add(-time.Hour).Format(checks.TimeLayout),
		EndsAt:   now.Add(time.Hour).Format(checks.TimeLayout),
		Reason:   "upgrade",
	})
	if err != nil || !win.Active {
		t.Fatalf("add window: %v %+v", err, win)
	}
	for _, tc := range []struct {
		entity string
		id     int64
		want   bool
	}{
		{"service", 1, true},
		{"service", 2, false},
		{"host", 1, false},
	} {
		got, err := s.InMaintenance(ctx, tc.entity, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("%s #%d: got %v want %v (%v)", tc.entity, tc.id, got, tc.want, err)
		}
	}
	if _, err := s.AddMaintenance(ctx, "test", checks.MaintenanceWindow{
		TargetEntity: "service", TargetID: 9,
		StartsAt: now.Add(time.Hour).Format(checks.TimeLayout),
		EndsAt:   now.Add(2 * time.Hour).Format(checks.TimeLayout),
	}); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.InMaintenance(ctx, "service", 9); on {
		t.Error("upcoming window should not silence yet")
	}
	if _, err := s.AddMaintenance(ctx, "test", checks.MaintenanceWindow{
		TargetEntity: "service", StartsAt: now.Add(time.Hour).Format(checks.TimeLayout),
		EndsAt: now.Format(checks.TimeLayout),
	}); !errors.Is(err, checks.ErrInvalid) {
		t.Errorf("reversed window: %v", err)
	}
	if _, err := s.AddMaintenance(ctx, "test", checks.MaintenanceWindow{TargetEntity: "toaster", EndsAt: now.Add(time.Hour).Format(checks.TimeLayout)}); !errors.Is(err, checks.ErrInvalid) {
		t.Errorf("bad entity: %v", err)
	}
	// a global window silences everything
	g, err := s.AddMaintenance(ctx, "test", checks.MaintenanceWindow{
		EndsAt: now.Add(30 * time.Minute).Format(checks.TimeLayout), Reason: "everything",
	})
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := s.InMaintenance(ctx, "host", 42); !on {
		t.Error("global window should silence hosts")
	}
	if list, _ := s.ListMaintenance(ctx); len(list) != 3 {
		t.Errorf("list windows: %+v", list)
	}
	if err := s.DeleteMaintenance(ctx, "test", g.ID); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.InMaintenance(ctx, "host", 42); on {
		t.Error("deleted global window still silences")
	}
	if err := s.DeleteMaintenance(ctx, "test", g.ID); !errors.Is(err, checks.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestRunnerSuppressionDuringMaintenance(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	store := checks.New(d)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	runner := checks.NewRunner(store)
	n := &captureNotifier{}
	runner.SetNotifier(n)
	on := true
	runner.SetSuppress(func(context.Context, string, int64) bool { return on })

	c, _ := store.Create(ctx, "test", checks.Check{TargetEntity: "service", TargetID: 1, Kind: "http", Params: map[string]any{"url": srv.URL}})
	for i := 0; i < 3; i++ {
		if _, err := runner.RunNow(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.events) != 0 {
		t.Fatalf("alerts should be suppressed: %+v", n.events)
	}
	on = false
	c2, _ := store.Create(ctx, "test", checks.Check{TargetEntity: "service", TargetID: 2, Kind: "http", Params: map[string]any{"url": srv.URL}})
	for i := 0; i < 3; i++ {
		if _, err := runner.RunNow(ctx, c2.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.events) != 1 {
		t.Fatalf("alerts should resume: %+v", n.events)
	}
}
