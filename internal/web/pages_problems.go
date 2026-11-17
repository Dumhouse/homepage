package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/dawidof/home-services/internal/checks"
	"github.com/go-chi/chi/v5"
)

// ---- problems ----

type problemRow struct {
	checks.Problem
	Title string
	Href  string
	Icon  string
	Since string
}

// Severity is the mockup's high/medium badge.
func (r problemRow) Severity() string {
	if r.State == "down" {
		return "high"
	}
	return "medium"
}

type incident struct {
	Title, Href, What string
	Start             string
	Duration          string
	Ongoing           bool
	Note              string
}

type problemsPage struct {
	Layout
	Rows      []problemRow
	Incidents []incident
	Days      int
	Windows   []checks.MaintenanceWindow
}

// targetTitle resolves a check target to a display name and page link.
func (p *pages) targetTitle(ctx context.Context, entity string, id int64) (string, string) {
	switch entity {
	case "host":
		if h, err := p.inv.GetHost(ctx, id); err == nil {
			return h.Name, "/hosts/" + itoa(h.ID)
		}
	case "service":
		if v, err := p.inv.GetService(ctx, id); err == nil {
			name := v.Name
			if h, err := p.inv.GetHost(ctx, v.HostID); err == nil {
				name += " · " + h.Name
			}
			return name, "/hosts/" + itoa(v.HostID)
		}
	case "ssh_access":
		if x, hostID, err := p.inv.GetSSHAccess(ctx, id); err == nil {
			name := "ssh " + x.User + " :" + itoa(int64(x.Port))
			if h, err := p.inv.GetHost(ctx, hostID); err == nil {
				name = "SSH · " + h.Name
			}
			return name, "/hosts/" + itoa(hostID)
		}
	case "app_link":
		if a, err := p.inv.GetAppLink(ctx, id); err == nil {
			if pr, err := p.inv.GetProject(ctx, a.ProjectID); err == nil {
				return a.Label + " · " + pr.Name, "/projects/" + pr.Slug
			}
			return a.Label, "/projects"
		}
	}
	return entity + " #" + itoa(id), "/problems"
}

func checkIcon(kind string) string {
	switch kind {
	case "ssh":
		return "key"
	case "http", "https":
		return "activity"
	case "tcp":
		return "network"
	case "tls":
		return "lock"
	}
	return "activity"
}

// shortWhen formats a DB timestamp as "today 10:12" or "Nov 12, 14:03".
func shortWhen(ts string, now time.Time) string {
	t, err := time.ParseInLocation(checks.TimeLayout, ts, time.UTC)
	if err != nil {
		return ts
	}
	t = t.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return "today " + fmtClock(t)
	}
	return fmtDayTime(t)
}

func shortDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + " s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " min"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + " days"
}

func (p *pages) problems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Alerts", "problems", crumbs(Crumb{Label: "Alerts"}))
	list, err := p.chk.Store().ListProblems(ctx, 100)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	list = append(p.healthProblems(ctx), list...)
	now := time.Now()
	rows := []problemRow{}
	for _, pr := range list {
		row := problemRow{Problem: pr, Icon: checkIcon(pr.Kind)}
		row.Title, row.Href = p.targetTitle(ctx, pr.TargetEntity, pr.TargetID)
		if pr.LastRunAt != "" {
			row.Since = shortWhen(pr.LastRunAt, now)
		}
		rows = append(rows, row)
	}
	// Incidents: runs of failed results per check over the chosen window (1, 7 or 30 days).
	days := 7
	if d, _ := strconv.Atoi(r.URL.Query().Get("days")); d == 1 || d == 30 {
		days = d
	}
	var incs []incident
	cutoff := now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
	if all, err := p.chk.Store().List(ctx, "", 0); err == nil {
		for _, c := range all {
			res, err := p.chk.Store().Transitions(ctx, c.ID, cutoff) // oldest first
			if err != nil || len(res) == 0 {
				continue
			}
			title, href := "", ""
			var cur *incident
			var curStart time.Time
			for i := 0; i < len(res); i++ {
				at, err := time.ParseInLocation(checks.TimeLayout, res[i].At, time.UTC)
				if err != nil {
					continue
				}
				if !res[i].OK && cur == nil {
					if title == "" {
						title, href = p.targetTitle(ctx, c.TargetEntity, c.TargetID)
					}
					cur = &incident{Title: title, Href: href, What: c.Kind, Start: res[i].At, Note: res[i].Message}
					curStart = at
				} else if res[i].OK && cur != nil {
					cur.Duration = shortDur(at.Sub(curStart))
					incs = append(incs, *cur)
					cur = nil
				}
			}
			if cur != nil {
				cur.Ongoing = true
				incs = append(incs, *cur)
			}
		}
	}
	sort.Slice(incs, func(i, j int) bool { return incs[i].Start > incs[j].Start })
	if len(incs) > 50 {
		incs = incs[:50]
	}
	for i := range incs {
		incs[i].Start = shortWhen(incs[i].Start, now)
	}
	windows, _ := p.chk.Store().ListMaintenance(ctx)
	l.Flash = flash(r)
	p.ui.render(w, 200, "problems", problemsPage{Layout: l, Rows: rows, Incidents: incs, Windows: windows, Days: days})
}

type maintenancePage struct {
	Layout
	Windows      []checks.MaintenanceWindow
	DefaultStart string
	DefaultEnd   string
}

func (p *pages) maintenance(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "Maintenance", "maintenance", crumbs(Crumb{Label: "Maintenance"}))
	list, err := p.chk.Store().ListMaintenance(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	l.Flash = flash(r)
	p.ui.render(w, 200, "maintenance", maintenancePage{
		Layout: l, Windows: list,
		DefaultStart: now.Format("2006-01-02T15:04"),
		DefaultEnd:   now.Add(2 * time.Hour).Format("2006-01-02T15:04"),
	})
}

func (p *pages) maintenanceAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	start, end, reason := r.FormValue("starts_at"), r.FormValue("ends_at"), r.FormValue("reason")
	if mins := intField(r, "duration_min"); mins > 0 {
		now := time.Now().UTC()
		start = now.Format(checks.TimeLayout)
		end = now.Add(time.Duration(mins) * time.Minute).Format(checks.TimeLayout)
	}
	win := checks.MaintenanceWindow{
		TargetEntity: r.FormValue("target_entity"),
		TargetID:     int64(intField(r, "target_id")),
		StartsAt:     start, EndsAt: end, Reason: reason,
	}
	if _, err := p.chk.Store().AddMaintenance(r.Context(), actorOf(r), win); err != nil {
		backWithError(w, r, safeBack(r.FormValue("back"), "/maintenance"), err)
		return
	}
	http.Redirect(w, r, withQuery(safeBack(r.FormValue("back"), "/maintenance"), "ok=maintenance"), http.StatusSeeOther)
}

func (p *pages) maintenanceDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.chk.Store().DeleteMaintenance(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/maintenance?ok=maintenance-deleted", http.StatusSeeOther)
}

func (p *pages) checkToggle(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	c, err := p.chk.Store().Get(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.chk.Store().SetEnabled(r.Context(), actorOf(r), id, !c.Enabled); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, safeBack(r.FormValue("back"), "/problems"), http.StatusSeeOther)
}

// healthProblems turns host warnings from facts (a failing disk, a degraded
// pool, a UPS on battery, collected by SSH or an integration) into problems,
// so they show in Needs a look next to failing checks. Snoozed hosts are
// left out.
func (p *pages) healthProblems(ctx context.Context) []checks.Problem {
	hosts, err := p.inv.ListHosts(ctx)
	if err != nil {
		return nil
	}
	out := []checks.Problem{}
	for _, h := range hosts {
		f, err := p.inv.GetHostFacts(ctx, h.ID)
		if err != nil || f == nil || f.Error == "" {
			continue
		}
		if in, _ := p.chk.Store().InMaintenance(ctx, "host", h.ID); in {
			continue
		}
		out = append(out, checks.Problem{TargetEntity: "host", TargetID: h.ID, Title: h.Name, Kind: "health",
			State: "warning", LastError: f.Error, LastRunAt: f.CollectedAt})
	}
	return out
}
