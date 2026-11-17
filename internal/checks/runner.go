package checks

import (
	"context"
	"fmt"
	"log"

	"github.com/Dumhouse/homepage/internal/alerts"
	"github.com/Dumhouse/homepage/internal/inventory"
)

// Runner executes probes and persists results. It also keeps ssh_access.status
// in sync with SSH probes.
type Runner struct {
	store    *Store
	prober   *Prober
	notifier alerts.Notifier
	label    func(context.Context, string, int64) string
	sshState func(context.Context, int64) string
	suppress func(context.Context, string, int64) bool
}

func NewRunner(store *Store) *Runner {
	return &Runner{store: store, prober: NewProber()}
}

// SetNotifier enables transition alerts (down / back up).
func (r *Runner) SetNotifier(n alerts.Notifier) { r.notifier = n }

// SetLabel supplies human names for alert messages.
func (r *Runner) SetLabel(fn func(context.Context, string, int64) string) { r.label = fn }

// SetSSHState lets the runner compare an SSH probe with the stored status, so
// "password auth became allowed" is announced once.
func (r *Runner) SetSSHState(fn func(context.Context, int64) string) { r.sshState = fn }

// SetSuppress lets maintenance windows silence alerts.
func (r *Runner) SetSuppress(fn func(context.Context, string, int64) bool) { r.suppress = fn }

func (r *Runner) suppressed(ctx context.Context, c Check) bool {
	return r.suppress != nil && r.suppress(ctx, c.TargetEntity, c.TargetID)
}

// RunCheck probes one check, records the result and updates derived state.
func (r *Runner) RunCheck(ctx context.Context, c Check) (Result, error) {
	prev := c.State
	beforeSSH := ""
	if c.TargetEntity == "ssh_access" && r.sshState != nil {
		beforeSSH = r.sshState(ctx, c.TargetID)
	}
	res := r.prober.Run(ctx, c)
	if err := r.store.Record(ctx, c, res); err != nil {
		return res, err
	}
	if c.TargetEntity == "ssh_access" && res.SSHStatus != "" {
		_ = r.store.SetSSHStatus(ctx, c.TargetID, res.SSHStatus)
		if res.SSHStatus == "password_allowed" && beforeSSH != "password_allowed" && !r.suppressed(ctx, c) {
			r.notify(ctx, c, res, "warning")
		}
	}
	r.notifyTransition(ctx, c, res, prev)
	return res, nil
}

// notifyTransition only fires on meaningful edges: entering down, and entering
// up again after a real outage. The "recovering" step is intentionally quiet.
func (r *Runner) notifyTransition(ctx context.Context, c Check, res Result, prev string) {
	if r.notifier == nil || r.suppressed(ctx, c) {
		return
	}
	next, _ := nextState(prev, res.OK, c.FailCount)
	state := ""
	switch {
	case next == "down" && prev != "down":
		state = "down"
	case next == "up" && (prev == "down" || prev == "recovering"):
		state = "up"
	default:
		return
	}
	r.notify(ctx, c, res, state)
}

func (r *Runner) notify(ctx context.Context, c Check, res Result, state string) {
	if r.notifier == nil {
		return
	}
	prev := c.State
	label := c.TargetEntity + " #" + itoa64(c.TargetID)
	if r.label != nil {
		label = r.label(ctx, c.TargetEntity, c.TargetID)
	}
	msg := fmt.Sprintf("%s: %s", label, res.Message)
	if state == "warning" && res.SSHStatus != "" {
		msg = fmt.Sprintf("%s: SSH password authentication is allowed — switch to keys", label)
	}
	if err := r.notifier.Send(ctx, alerts.Event{
		CheckID: c.ID, Entity: c.TargetEntity, TargetID: c.TargetID, Label: label,
		Kind: c.Kind, State: state, Previous: prev, Message: msg,
	}); err != nil {
		log.Printf("alert (%s %s): %v", c.TargetEntity, label, err)
	}
}

func itoa64(n int64) string { return fmt.Sprintf("%d", n) }

// RunNow executes one check regardless of its interval.
func (r *Runner) RunNow(ctx context.Context, id int64) (Result, error) {
	c, err := r.store.Get(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return r.RunCheck(ctx, *c)
}

// RunTarget runs (or creates) the check for a target and returns the result.
func (r *Runner) RunTarget(ctx context.Context, entity string, id int64) (Result, error) {
	list, err := r.store.List(ctx, entity, id)
	if err != nil {
		return Result{}, err
	}
	if len(list) == 0 {
		return Result{}, ErrNotFound
	}
	return r.RunCheck(ctx, list[0])
}

// CheckSSH probes a host's SSH login on demand without persisting? It does
// persist, so the host badge and history stay current.
func (r *Runner) CheckSSH(ctx context.Context, hostID int64, inv *inventory.Store) (Result, error) {
	h, err := inv.GetHost(ctx, hostID)
	if err != nil {
		return Result{}, err
	}
	if len(h.SSH) == 0 || len(h.Addresses) == 0 {
		return Result{}, ErrNotFound
	}
	x := h.SSH[0]
	params := map[string]any{"address": h.Addresses[0].Address, "port": x.Port, "user": x.User}
	if _, err := r.store.upsertAuto(ctx, "ssh_access", x.ID, "ssh", 3600, params); err != nil {
		return Result{}, err
	}
	list, err := r.store.List(ctx, "ssh_access", x.ID)
	if err != nil || len(list) == 0 {
		return Result{}, ErrNotFound
	}
	return r.RunCheck(ctx, list[0])
}
