package web

import (
	"net/http"
	"strings"

	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/inventory"
)

// ---- services ----

type serviceRow struct {
	inventory.ServiceWithHost
	State string
	Bars  []checks.Bar
}

type servicesPage struct {
	Layout
	Rows  []serviceRow
	Q     string
	Total int
}

func (p *pages) services(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Services", "services", crumbs(Crumb{Label: "Services"}))
	all, err := p.inv.AllServices(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	states := map[int64]string{}
	if list, err := p.chk.Store().List(ctx, "", 0); err == nil {
		for _, c := range list {
			if c.TargetEntity != "service" {
				continue
			}
			if betterState(c.State, states[c.TargetID]) {
				states[c.TargetID] = c.State
			}
		}
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	rows := []serviceRow{}
	for _, v := range all {
		hay := strings.ToLower(v.Name + " " + v.Type + " " + v.Domain + " " + v.HostName + " " + v.Address +
			" " + strings.Join(v.Tags, " "))
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		rows = append(rows, serviceRow{ServiceWithHost: v, State: states[v.ID]})
	}
	p.ui.render(w, 200, "services", servicesPage{Layout: l, Rows: rows, Q: r.URL.Query().Get("q"), Total: len(all)})
}
