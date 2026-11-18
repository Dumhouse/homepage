package discovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Dumhouse/homepage/internal/inventory"
)

type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSettings) Get(_ context.Context, k, def string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[k]; ok {
		return v, nil
	}
	return def, nil
}

func (s *memSettings) Set(_ context.Context, _, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

type fakeUpserter struct {
	mu   sync.Mutex
	seen map[string][]inventory.PortInfo
}

func (f *fakeUpserter) UpsertDiscovery(_ context.Context, address, _ string, ports []inventory.PortInfo) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	nh := 0
	if _, ok := f.seen[address]; !ok {
		nh = 1
	}
	f.seen[address] = ports
	return nh, len(ports), nil
}

func TestParseSubnetsAndPorts(t *testing.T) {
	got := ParseSubnets("192.168.1.0/24, 10.0.0.7/24;bogus\n192.168.1.0/24 172.16.0.0/30")
	if want := []string{"192.168.1.0/24", "10.0.0.0/24", "172.16.0.0/30"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("subnets %v", got)
	}
	if got := ParsePorts("443, 22 80;8000-8002,0,70000,9-3,x"); !reflect.DeepEqual(got, []int{22, 80, 443, 8000, 8001, 8002}) {
		t.Fatalf("ports %v", got)
	}
	if got := ParsePorts("1-1000"); len(got) != 0 {
		t.Fatalf("a range wider than MaxPorts is refused: %d ports", len(got))
	}
	many := ""
	for p := 1000; p < 1100; p++ {
		many += strconv.Itoa(p) + ","
	}
	if got := ParsePorts(many); len(got) != MaxPorts {
		t.Fatalf("capped at %d, got %d", MaxPorts, len(got))
	}
}

func TestPortURL(t *testing.T) {
	cases := []struct {
		port           int
		service, title string
		want           string
	}{
		{443, "", "", "https://192.0.2.1:443/"},
		{8006, "proxmox", "", "https://192.0.2.1:8006/"},
		{9999, "", "My App", "http://192.0.2.1:9999/"},
		{8080, "http-alt", "", "http://192.0.2.1:8080/"},
		{22, "ssh", "", ""},
	}
	for _, c := range cases {
		if got := PortURL("192.0.2.1", c.port, c.service, c.title); got != c.want {
			t.Errorf("PortURL(%d,%q,%q) = %q", c.port, c.service, c.title, got)
		}
	}
	if !bytesLess("10.0.0.2", "10.0.0.10") || bytesLess("10.0.0.10", "10.0.0.2") || !bytesLess("a", "b") {
		t.Fatal("addresses sort numerically")
	}
}

func TestRunnerScansAndReports(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<title>Fixture</title>"))
	}))
	defer web.Close()
	_, portStr, _ := net.SplitHostPort(web.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	inv := &fakeUpserter{seen: map[string][]inventory.PortInfo{}}
	set := &memSettings{m: map[string]string{KeyPorts: portStr}}
	r := NewRunner(inv, set)
	var mu sync.Mutex
	var announced []string
	r.OnNew = func(_ context.Context, address, _ string, _ []inventory.PortInfo) {
		mu.Lock()
		announced = append(announced, address)
		mu.Unlock()
	}
	if st := r.Status(); st.Running || st.Percent != 0 {
		t.Fatalf("idle status %+v", st)
	}
	if got := r.Ports(context.Background()); !reflect.DeepEqual(got, []int{port}) {
		t.Fatalf("ports from settings: %v", got)
	}
	if err := r.Start(nil, "t"); err == nil {
		t.Fatal("no subnets must fail")
	}
	if err := r.Start([]string{"10.0.0.0/16"}, "t"); err == nil {
		t.Fatal("a /16 is too large")
	}
	if err := r.Start([]string{"nope"}, "t"); err == nil {
		t.Fatal("bad CIDR")
	}
	if err := r.Start([]string{"127.0.0.1/32"}, "t"); err != nil {
		t.Fatal(err)
	}
	if err := r.Start([]string{"127.0.0.1/32"}, "t"); err != ErrBusy {
		// The scan may already be over on a fast machine; only a different error is wrong.
		if err != nil {
			t.Fatalf("second start: %v", err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.Status().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	st := r.Status()
	if st.Running || st.Percent != 100 || st.Phase != "done" || st.Found != 1 || st.Error != "" {
		t.Fatalf("final status %+v", st)
	}
	if last, _ := set.Get(context.Background(), KeyLastAt, ""); last == "" {
		t.Fatal("the run should record when it finished")
	}
	inv.mu.Lock()
	ports := inv.seen["127.0.0.1"]
	inv.mu.Unlock()
	if len(ports) != 1 || ports[0].Port != port || ports[0].Title != "Fixture" {
		t.Fatalf("stored ports %+v", ports)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(announced) == 0 || announced[0] != "127.0.0.1" {
		t.Fatalf("OnNew %v", announced)
	}
}

func TestStatusPercent(t *testing.T) {
	r := NewRunner(&fakeUpserter{seen: map[string][]inventory.PortInfo{}}, &memSettings{m: map[string]string{}})
	r.st = Status{Running: true, Count: 2, Index: 2, Phase: "identify", Done: 1, Total: 2}
	if p := r.Status().Percent; p != 90 { // subnet 1 done + 80 % of subnet 2 = 1.8 of 2
		t.Fatalf("percent %d", p)
	}
	r.st = Status{Running: true, Count: 1, Index: 1, Phase: "ports", Done: 10, Total: 10}
	if p := r.Status().Percent; p != 60 {
		t.Fatalf("ports phase caps at 60 %%: %d", p)
	}
}

func TestMaybeRunSchedule(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	set := &memSettings{m: map[string]string{KeyEveryH: "0", KeySubnets: "127.0.0.1/32", KeyPorts: "1"}}
	r := NewRunner(&fakeUpserter{seen: map[string][]inventory.PortInfo{}}, set)
	r.maybeRun(ctx, now)
	if r.Status().Running || !r.Status().StartedAt.IsZero() {
		t.Fatal("every=0 means off")
	}
	set.m[KeyEveryH] = "24"
	set.m[KeyLastAt] = now.Add(-time.Hour).Format("2006-01-02 15:04:05")
	r.maybeRun(ctx, now)
	if !r.Status().StartedAt.IsZero() {
		t.Fatal("scanned an hour ago: not due")
	}
	set.m[KeyLastAt] = now.Add(-25 * time.Hour).Format("2006-01-02 15:04:05")
	r.maybeRun(ctx, now)
	if r.Status().StartedAt.IsZero() {
		t.Fatal("due: the scheduler should start a scan")
	}
	for r.Status().Running {
		time.Sleep(10 * time.Millisecond)
	}
	if got := r.Subnets(ctx); !reflect.DeepEqual(got, []string{"127.0.0.1/32"}) {
		t.Fatalf("subnets %v", got)
	}
}
