package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/go-chi/chi/v5"
)

// ---- changesets ----

func pendingChanges(ctx context.Context, p *pages) int {
	if p.cs == nil {
		return 0
	}
	list, err := p.cs.List(ctx, "pending")
	if err != nil {
		return 0
	}
	return len(list)
}

type changesPage struct {
	Layout
	Pending []changesets.ChangeSet
	Recent  []changesets.ChangeSet
}

func (p *pages) changesetsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "AI changes", "changes", crumbs(Crumb{Label: "AI changes"}))
	l.Flash = flash(r)
	pending, err := p.cs.List(ctx, "pending")
	if err != nil {
		p.fail(w, r, err)
		return
	}
	recent, _ := p.cs.List(ctx, "")
	kept := recent[:0]
	for _, c := range recent {
		if c.Status != "pending" {
			kept = append(kept, c)
		}
	}
	if len(kept) > 10 {
		kept = kept[:10]
	}
	p.ui.render(w, 200, "changesets", changesPage{Layout: l, Pending: pending, Recent: kept})
}

func (p *pages) changesetApply(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := p.cs.Apply(r.Context(), actorOf(r), id); err != nil {
		backWithError(w, r, "/changesets", err)
		return
	}
	http.Redirect(w, r, "/changesets?ok=applied", http.StatusSeeOther)
}

func (p *pages) changesetReject(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.cs.Reject(r.Context(), actorOf(r), id); err != nil {
		backWithError(w, r, "/changesets", err)
		return
	}
	http.Redirect(w, r, "/changesets?ok=rejected", http.StatusSeeOther)
}

func (p *pages) changesetUndo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.cs.Undo(r.Context(), actorOf(r), id); err != nil {
		backWithError(w, r, "/changesets", err)
		return
	}
	http.Redirect(w, r, "/changesets?ok=undone", http.StatusSeeOther)
}
