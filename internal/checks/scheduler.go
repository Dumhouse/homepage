package checks

import (
	"context"
	"database/sql"
	"strconv"
	"sync"
	"time"

	"github.com/dawidof/home-services/internal/alerts"
	"github.com/dawidof/home-services/internal/inventory"
)

// Scheduler runs due checks on a ticker, reconciles auto checks every minute and
// prunes history hourly. Start it once and stop it with the returned function.
type Scheduler struct {
	runner  *Runner
	store   *Store
	inv     *inventory.Store
	Every   time.Duration
	Workers int
}

func NewScheduler(db *sql.DB, inv *inventory.Store) *Scheduler {
	store := New(db)
	runner := NewRunner(store)
	runner.SetLabel(func(ctx context.Context, entity string, id int64) string {
		switch entity {
		case "service":
			if v, err := inv.GetService(ctx, id); err == nil {
				if h, err := inv.GetHost(ctx, v.HostID); err == nil {
					return h.Name + "/" + v.Name
				}
				return v.Name
			}
		case "host":
			if h, err := inv.GetHost(ctx, id); err == nil {
				return h.Name
			}
		case "ssh_access":
			if x, hostID, err := inv.GetSSHAccess(ctx, id); err == nil {
				if h, err := inv.GetHost(ctx, hostID); err == nil {
					return "ssh " + x.User + " on " + h.Name
				}
				return "ssh " + x.User
			}
		}
		return entity + " #" + strconv.FormatInt(id, 10)
	})
	runner.SetSuppress(func(ctx context.Context, entity string, id int64) bool {
		on, err := store.InMaintenance(ctx, entity, id)
		return err == nil && on
	})
	runner.SetSSHState(func(ctx context.Context, id int64) string {
		if x, _, err := inv.GetSSHAccess(ctx, id); err == nil {
			return x.Status
		}
		return ""
	})
	return &Scheduler{
		runner: runner, store: store, inv: inv,
		Every: 5 * time.Second, Workers: 8,
	}
}

// SetNotifier enables alert delivery for check transitions.
func (s *Scheduler) SetNotifier(n alerts.Notifier) { s.runner.SetNotifier(n) }

// Start begins the background loop. The returned stop function waits for it.
func (s *Scheduler) Start(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(s.Every)
		maint := time.NewTicker(time.Minute)
		defer tick.Stop()
		defer maint.Stop()
		_, _ = s.store.Reconcile(ctx, s.inv)
		_, _ = s.Tick(ctx)
		rounds := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				_, _ = s.Tick(ctx)
			case <-maint.C:
				rounds++
				if rounds%60 == 0 {
					_ = s.store.Prune(ctx)
				}
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

// Reconcile refreshes auto checks from the inventory.
func (s *Scheduler) Reconcile(ctx context.Context) (int, error) {
	return s.store.Reconcile(ctx, s.inv)
}

// Tick reconciles auto checks, then runs every due check with a small worker
// pool and returns how many ran.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	_, _ = s.store.Reconcile(ctx, s.inv)
	due, err := s.store.Due(ctx, 100)
	if err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}
	workers := s.Workers
	if workers <= 0 {
		workers = 8
	}
	if workers > len(due) {
		workers = len(due)
	}
	jobs := make(chan Check)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				_, _ = s.runner.RunCheck(ctx, c)
			}
		}()
	}
	for _, c := range due {
		jobs <- c
	}
	close(jobs)
	wg.Wait()
	return len(due), nil
}

// RunNow is what the UI/MCP "check now" button calls.
func (s *Scheduler) RunNow(ctx context.Context, id int64) (Result, error) {
	return s.runner.RunNow(ctx, id)
}

func (s *Scheduler) RunTarget(ctx context.Context, entity string, id int64) (Result, error) {
	return s.runner.RunTarget(ctx, entity, id)
}

func (s *Scheduler) CheckSSH(ctx context.Context, hostID int64) (Result, error) {
	return s.runner.CheckSSH(ctx, hostID, s.inv)
}

func (s *Scheduler) Store() *Store { return s.store }

// Label resolves a human name for a check target (used in alerts and digests).
func (s *Scheduler) Label(ctx context.Context, entity string, id int64) string {
	if s.runner.label == nil {
		return entity + " #" + strconv.FormatInt(id, 10)
	}
	return s.runner.label(ctx, entity, id)
}

// CheckHost runs every check of a host (its services, SSH logins and the host
// itself) in parallel and returns how many ran. It is bounded by ctx.
func (s *Scheduler) CheckHost(ctx context.Context, hostID int64) (int, error) {
	_, _ = s.store.Reconcile(ctx, s.inv)
	h, err := s.inv.GetHost(ctx, hostID)
	if err != nil {
		return 0, err
	}
	var list []Check
	add := func(entity string, id int64) {
		if cs, err := s.store.List(ctx, entity, id); err == nil {
			for _, c := range cs {
				if c.Enabled {
					list = append(list, c)
				}
			}
		}
	}
	add("host", h.ID)
	for _, v := range h.Services {
		add("service", v.ID)
	}
	for _, x := range h.SSH {
		add("ssh_access", x.ID)
	}
	var wg sync.WaitGroup
	for _, c := range list {
		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			_, _ = s.runner.RunCheck(ctx, c)
		}(c)
	}
	wg.Wait()
	return len(list), ctx.Err()
}
