package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dawidof/home-services/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- check now ----

func (p *pages) serviceCheckNow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	svc, err := p.inv.GetService(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if _, err := p.chk.Reconcile(r.Context()); err != nil {
		p.fail(w, r, err)
		return
	}
	if _, err := p.chk.RunTarget(r.Context(), "service", id); err != nil {
		backWithError(w, r, "/hosts/"+itoa(svc.HostID), err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(svc.HostID)+"?ok=checked", http.StatusSeeOther)
}

func (p *pages) hostFacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if p.facts == nil {
		backWithError(w, r, "/hosts/"+itoa(id), errors.New("SSH facts are not configured (set HS_SSH_KEY)"))
		return
	}
	h, err := p.inv.GetHost(ctx, id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if len(h.Addresses) == 0 {
		backWithError(w, r, "/hosts/"+itoa(id), errors.New("host has no address"))
		return
	}
	f, err := p.facts.Collect(ctx, h.Addresses[0].Address)
	if err != nil {
		backWithError(w, r, "/hosts/"+itoa(id), err)
		return
	}
	f.HostID = h.ID
	if err := p.inv.SaveHostFacts(ctx, actorOf(r), *f); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(id)+"?ok=facts", http.StatusSeeOther)
}

// hostCheckNow runs every check of the host at once (services, public URLs,
// SSH). The page submits it with fetch, so the UI stays usable meanwhile.
func (p *pages) hostCheckNow(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	n, err := p.chk.CheckHost(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil && n == 0 {
		backWithError(w, r, "/hosts/"+itoa(id), err)
		return
	}
	if n == 0 {
		backWithError(w, r, "/hosts/"+itoa(id), errors.New("nothing to check yet — add a service with a port or domain, or an SSH login"))
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(id)+"?ok=checked", http.StatusSeeOther)
}

func (p *pages) checkRun(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := p.chk.RunNow(r.Context(), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back"), "/"), http.StatusSeeOther)
}

func (p *pages) savePrefs(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	key, value := r.FormValue("key"), r.FormValue("value")
	if key != "sidebar" || (value != "open" && value != "collapsed") {
		http.Error(w, "unknown setting", http.StatusBadRequest)
		return
	}
	if err := p.set.Set(r.Context(), actorOf(r), key, value); err != nil {
		p.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// undoFromQuery turns ?ok=deleted&id=N&name=X into a restore toast.
func undoFromQuery(r *http.Request, base string) *Undo {
	q := r.URL.Query()
	if q.Get("ok") != "deleted" || q.Get("id") == "" {
		return nil
	}
	name := q.Get("name")
	if name == "" {
		name = "Item"
	}
	return &Undo{Path: base + q.Get("id") + "/restore", Name: name}
}

func flash(r *http.Request) string {
	switch r.URL.Query().Get("ok") {
	case "msg":
		return r.URL.Query().Get("msg")
	case "revoked":
		return "Token revoked."
	case "created":
		return "Created."
	case "saved":
		return "Saved."
	case "restored":
		return "Restored."
	case "service":
		return "Service added."
	case "service-deleted":
		return "Service removed."
	case "access":
		return "Access entry added."
	case "env":
		return "Environment added."
	case "link":
		return "App link added."
	case "checked":
		return "Check finished."
	case "facts":
		return "SSH facts collected."
	case "rates":
		return "Exchange rates updated."
	case "facts-saved":
		return "Facts saved."
	case "looked-up":
		return "Registrar and expiry updated from public registration data."
	case "favicons":
		return "Favicon settings saved."
	case "cost":
		return "Cost added."
	case "scan":
		return "Scan started — it runs in the background; new hosts appear below."
	case "subnets":
		return "Scan settings saved."
	case "scan-done":
		return "Scan finished."
	case "icons-queued":
		return "Fetching icons in the background — reload in a few seconds."
	case "proposed":
		msg := "Changeset proposed — review it below."
		if w := r.URL.Query().Get("msg"); w != "" {
			msg += " Note: " + w
		}
		return msg
	case "applied":
		return "Changes applied."
	case "rejected":
		return "Changeset rejected."
	case "undone":
		return "Changes undone."
	case "integration":
		return "Integration saved."
	case "integration-tested":
		return "Connection OK."
	case "integration-synced":
		return "Sync finished — review the proposed ops below."
	case "record":
		return "DNS record added."
	case "record-deleted":
		return "DNS record removed."
	case "maintenance":
		return "Maintenance window saved."
	case "maintenance-deleted":
		return "Maintenance window removed."
	default:
		return ""
	}
}

// withQuery appends a query pair to a local path.
func withQuery(path, kv string) string {
	if strings.Contains(path, "?") {
		return path + "&" + kv
	}
	return path + "?" + kv
}
