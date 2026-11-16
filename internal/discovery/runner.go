package discovery

import (
	"context"
	"errors"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dawidof/home-services/internal/inventory"
)

// Setting keys used by the runner.
const (
	KeySubnets = "scan_subnets" // comma-separated CIDRs
	KeyLastAt  = "scan_last_at" // UTC "2006-01-02 15:04:05"
	KeyEveryH  = "scan_every_h" // hours between background scans, 0 = off
	KeyPorts   = "scan_ports"   // comma-separated TCP ports; empty = DefaultPorts
	DefaultNet = "192.168.1.0/24"
)

// ErrBusy is returned when a scan is already running.
var ErrBusy = errors.New("a scan is already running")

// Upserter stores scan results (inventory.Store).
type Upserter interface {
	UpsertDiscovery(ctx context.Context, address, hostname string, ports []inventory.PortInfo) (int, int, error)
}

// Settings is the small part of settings.Store the runner needs.
type Settings interface {
	Get(ctx context.Context, key, def string) (string, error)
	Set(ctx context.Context, actor, key, value string) error
}

// Status is a snapshot of the current or last scan, for the progress bar.
type Status struct {
	Running    bool      `json:"running"`
	Subnet     string    `json:"subnet,omitempty"`
	Phase      string    `json:"phase,omitempty"` // ports | identify | done
	Done       int       `json:"done"`
	Total      int       `json:"total"`
	Percent    int       `json:"percent"`
	Index      int       `json:"index"` // 1-based subnet being scanned
	Count      int       `json:"count"` // subnets in this run
	Found      int       `json:"found"` // hosts with open ports so far
	NewHosts   int       `json:"new_hosts"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Runner runs scans in the background, one at a time, and on a schedule.
type Runner struct {
	inv     Upserter
	set     Settings
	Scanner func() *Scanner // overridable in tests
	// OnNew, when set, is called for each address seen for the first time.
	OnNew func(ctx context.Context, address, hostname string, ports []inventory.PortInfo)

	mu sync.Mutex
	st Status
}

func NewRunner(inv Upserter, set Settings) *Runner {
	return &Runner{inv: inv, set: set, Scanner: New}
}

// ParseSubnets splits a comma/space/newline separated list and keeps valid,
// de-duplicated CIDRs in order.
func ParseSubnets(s string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == ';' }) {
		_, n, err := net.ParseCIDR(strings.TrimSpace(f))
		if err != nil {
			continue
		}
		c := n.String()
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Subnets returns the configured subnets.
func (r *Runner) Subnets(ctx context.Context) []string {
	v, _ := r.set.Get(ctx, KeySubnets, DefaultNet)
	return ParseSubnets(v)
}

// Ports returns the TCP ports a scan checks (the setting, else DefaultPorts).
func (r *Runner) Ports(ctx context.Context) []int {
	v, _ := r.set.Get(ctx, KeyPorts, "")
	if p := ParsePorts(v); len(p) > 0 {
		return p
	}
	return DefaultPorts
}

// MaxPorts bounds how many ports one scan checks (each costs a probe per address).
const MaxPorts = 64

// ParsePorts reads "22, 80 443,8000-8003" into sorted, unique ports 1–65535.
// Ranges are allowed; the result is capped at MaxPorts.
func ParsePorts(s string) []int {
	seen := map[int]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == ';' }) {
		lo, hi := f, f
		if a, b, ok := strings.Cut(f, "-"); ok {
			lo, hi = a, b
		}
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b, err2 := strconv.Atoi(strings.TrimSpace(hi))
		if err1 != nil || err2 != nil || a < 1 || b > 65535 || a > b || b-a > MaxPorts {
			continue
		}
		for p := a; p <= b; p++ {
			seen[p] = true
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	if len(out) > MaxPorts {
		out = out[:MaxPorts]
	}
	return out
}

// Status returns a copy of the current status.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.st
	// One bar for the whole run: each subnet is an equal share, split into
	// the port sweep (first 60 %) and identifying what answered (last 40 %).
	if st.Running && st.Count > 0 {
		frac := 0.0
		if st.Total > 0 {
			frac = float64(st.Done) / float64(st.Total)
		}
		step := 0.6 * frac
		if st.Phase == "identify" {
			step = 0.6 + 0.4*frac
		}
		st.Percent = int((float64(max(st.Index-1, 0)) + step) / float64(st.Count) * 100)
		st.Percent = min(st.Percent, 99)
	} else if !st.Running && !st.FinishedAt.IsZero() {
		st.Percent = 100
	}
	return st
}

// Start scans the given subnets in a goroutine. It returns ErrBusy while a
// scan runs.
func (r *Runner) Start(subnets []string, actor string) error {
	return r.StartWith(subnets, nil, actor)
}

// StartWith runs a scan with an optional port override (API callers).
func (r *Runner) StartWith(subnets []string, ports []int, actor string) error {
	if len(subnets) == 0 {
		return errors.New("no subnets to scan")
	}
	for _, s := range subnets {
		if _, n, err := net.ParseCIDR(s); err != nil {
			return err
		} else if ones, bits := n.Mask.Size(); bits-ones > 10 {
			return errors.New(s + " is too large: scan /22 or smaller")
		}
	}
	r.mu.Lock()
	if r.st.Running {
		r.mu.Unlock()
		return ErrBusy
	}
	r.st = Status{Running: true, Count: len(subnets), StartedAt: time.Now(), Phase: "ports"}
	r.mu.Unlock()
	go r.run(subnets, ports, actor)
	return nil
}

func (r *Runner) run(subnets []string, override []int, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var errs []string
	for i, subnet := range subnets {
		r.mu.Lock()
		r.st.Subnet, r.st.Index, r.st.Done, r.st.Total, r.st.Phase = subnet, i+1, 0, 0, "ports"
		r.mu.Unlock()
		sc := r.Scanner()
		ports := override
		if len(ports) == 0 {
			ports = r.Ports(ctx)
		}
		if len(ports) > 0 {
			sc.Ports = ports
			if sc.WebPorts == nil {
				sc.WebPorts = map[int]bool{}
			}
			for _, p := range ports { // unknown ports: try for a web page title too
				if _, known := portNames[p]; !known {
					sc.WebPorts[p] = true
				}
			}
		}
		sc.Progress = func(done, total int) {
			r.mu.Lock()
			r.st.Done, r.st.Total = done, total
			r.mu.Unlock()
		}
		sc.Identified = func(done, total int) {
			r.mu.Lock()
			r.st.Phase, r.st.Done, r.st.Total = "identify", done, total
			r.mu.Unlock()
		}
		results, err := sc.Scan(ctx, subnet)
		if err != nil {
			errs = append(errs, subnet+": "+err.Error())
			continue
		}
		for _, res := range results {
			nh, _, err := r.inv.UpsertDiscovery(ctx, res.Address, res.Hostname, res.Ports)
			if err != nil {
				errs = append(errs, res.Address+": "+err.Error())
				continue
			}
			r.mu.Lock()
			r.st.Found++
			r.st.NewHosts += nh
			r.mu.Unlock()
			if nh > 0 && r.OnNew != nil {
				r.OnNew(ctx, res.Address, res.Hostname, res.Ports)
			}
		}
	}
	_ = r.set.Set(context.Background(), actor, KeyLastAt, time.Now().UTC().Format("2006-01-02 15:04:05"))
	r.mu.Lock()
	r.st.Running, r.st.Phase, r.st.FinishedAt = false, "done", time.Now()
	r.st.Done = r.st.Total
	r.st.Error = strings.Join(errs, "; ")
	r.mu.Unlock()
	if len(errs) > 0 {
		log.Printf("discovery: %s", strings.Join(errs, "; "))
	}
}

// Loop starts scheduled scans every scan_every_h hours (default 24, 0 = off).
// It returns a stop function.
func (r *Runner) Loop(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.maybeRun(ctx, time.Now())
			}
		}
	}()
	return cancel
}

func (r *Runner) maybeRun(ctx context.Context, now time.Time) {
	every, _ := r.set.Get(ctx, KeyEveryH, "24")
	h, err := strconv.Atoi(strings.TrimSpace(every))
	if err != nil || h <= 0 {
		return
	}
	last, _ := r.set.Get(ctx, KeyLastAt, "")
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", last, time.UTC); err == nil && now.Sub(t) < time.Duration(h)*time.Hour {
		return
	}
	if err := r.Start(r.Subnets(ctx), "scheduler"); err != nil && !errors.Is(err, ErrBusy) {
		log.Printf("discovery: scheduled scan: %v", err)
	}
}
