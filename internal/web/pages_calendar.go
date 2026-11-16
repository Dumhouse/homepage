package web

import (
	"net/http"
	"sort"

	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/inventory"
)

// ---- expiry calendar ----

type calendarPage struct {
	Layout
	Domains []inventory.Domain
	Windows []checks.MaintenanceWindow
	Certs   []certRow
}

type certRow struct {
	Label  string
	Days   int
	Issuer string
}

func (p *pages) calendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Calendar", "calendar", crumbs(Crumb{Label: "Calendar"}))
	domains, err := p.inv.ListDomains(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	sort.SliceStable(domains, func(i, j int) bool {
		if domains[i].ExpiresAt == "" {
			return false
		}
		if domains[j].ExpiresAt == "" {
			return true
		}
		return domains[i].ExpiresAt < domains[j].ExpiresAt
	})
	windows, _ := p.chk.Store().ListMaintenance(ctx)
	certs := []certRow{}
	if list, err := p.chk.Store().Certificates(ctx, 120); err == nil {
		for _, c := range list {
			certs = append(certs, certRow{
				Label: p.chk.Label(ctx, c.TargetEntity, c.TargetID), Days: c.CertDays, Issuer: c.CertIssuer,
			})
		}
	}
	sort.SliceStable(certs, func(i, j int) bool { return certs[i].Days < certs[j].Days })
	p.ui.render(w, 200, "calendar", calendarPage{Layout: l, Domains: domains, Windows: windows, Certs: certs})
}
