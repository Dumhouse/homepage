package web

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
)

// syncResult is what one integration sync produced.
type syncResult struct {
	Found     int                   // items the plugin reported
	Facts     int                   // hosts whose facts were updated directly
	Missing   []string              // machines with facts but no host yet
	Changeset *changesets.ChangeSet // nil when nothing new
	Warning   error                 // partial failure: some items still came back
}

// syncToChangeset runs one integration sync. Facts are written directly;
// everything new for the inventory becomes one changeset to review.
func syncToChangeset(ctx context.Context, im *integrations.Manager, cs *changesets.Store, id, actor string) (syncResult, error) {
	ops, err := im.Sync(ctx, id)
	if err != nil && len(ops) == 0 {
		return syncResult{}, err
	}
	res := syncResult{Found: len(ops), Warning: err}
	list := make([]changesets.Op, 0, len(ops))
	for _, op := range ops {
		list = append(list, changesets.Op{Action: op.Action, Entity: op.Entity, Ref: op.Ref, Data: op.Data, Note: op.Note})
	}
	res.Facts, res.Missing, list = cs.ApplyFacts(ctx, "plugin:"+id, list)
	fresh := cs.Fresh(ctx, list)
	if len(fresh) == 0 {
		return res, nil
	}
	name := id
	for _, pl := range im.Plugins() {
		if pl.ID == id {
			name = pl.Name
		}
	}
	title := name + " sync: " + plural(len(fresh), "change")
	if err != nil {
		title += " (partial)"
	}
	c, cerr := cs.Create(ctx, actor, title, "plugin:"+id, fresh)
	if cerr != nil {
		return res, cerr
	}
	res.Changeset = c
	return res, nil
}

// AutoSync syncs every integration whose auto-sync setting is on, when its
// period has passed. It checks every ten minutes and never stacks proposals:
// while a changeset from an integration waits for review, that integration
// is skipped. The returned function stops the loop.
func AutoSync(ctx context.Context, d *sql.DB, im *integrations.Manager) func() {
	ctx, cancel := context.WithCancel(ctx)
	cs := changesets.New(d, inventory.New(d))
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			autoSyncOnce(ctx, im, cs, time.Now().UTC())
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return cancel
}

// autoSyncOnce returns the IDs it synced (for tests and logs).
func autoSyncOnce(ctx context.Context, im *integrations.Manager, cs *changesets.Store, now time.Time) []string {
	states, err := im.List(ctx)
	if err != nil {
		return nil
	}
	var done []string
	for _, s := range states {
		if !integrations.SyncDue(s, now) || cs.PendingFrom(ctx, "plugin:"+s.ID) {
			continue
		}
		sctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		res, err := syncToChangeset(sctx, im, cs, s.ID, "system:auto-sync")
		cancel()
		done = append(done, s.ID)
		switch {
		case err != nil:
			log.Printf("auto-sync %s: %v", s.ID, err)
		case res.Changeset != nil:
			log.Printf("auto-sync %s: proposed changeset #%d (%s)", s.ID, res.Changeset.ID, res.Changeset.Title)
		default:
			msg := "nothing new"
			if res.Warning != nil {
				msg += "; warning: " + strings.TrimSpace(res.Warning.Error())
			}
			log.Printf("auto-sync %s: %s", s.ID, msg)
		}
	}
	return done
}
