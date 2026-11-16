package web

import (
	"net/http"

	"github.com/dawidof/home-services/internal/power"

	"github.com/go-chi/chi/v5"

	"github.com/dawidof/home-services/internal/changesets"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/discovery"
	"github.com/dawidof/home-services/internal/icons"
	"github.com/dawidof/home-services/internal/integrations"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/settings"
	"github.com/dawidof/home-services/internal/tokens"
)

// pages serves the server-rendered UI. It is a thin adapter over the stores,
// exactly like the JSON API and MCP.
type pages struct {
	inv   *inventory.Store
	tok   *tokens.Store
	set   *settings.Store
	chk   *checks.Scheduler
	icons *icons.Fetcher
	scan  *discovery.Runner
	intg  *integrations.Manager
	facts FactsCollector
	cs    *changesets.Store
	pow   *power.Store
	ui    *ui
}

func newPages(inv *inventory.Store, tok *tokens.Store, set *settings.Store, chk *checks.Scheduler, ic *icons.Fetcher, intg *integrations.Manager, factsC FactsCollector, cs *changesets.Store) (*pages, error) {
	u, err := newUI()
	if err != nil {
		return nil, err
	}
	return &pages{inv: inv, tok: tok, set: set, chk: chk, icons: ic, intg: intg, facts: factsC, cs: cs, ui: u}, nil
}

func (p *pages) routes(r chi.Router) {
	r.Get("/", p.home)
	r.Post("/prefs", p.saveHomePrefs)
	r.Get("/hosts", p.hosts)
	r.Get("/locations", p.locations)
	r.Get("/locations/new", p.locationForm)
	r.Get("/locations/{id}/edit", p.locationForm)
	r.Post("/locations", p.locationSave)
	r.Post("/locations/{id}", p.locationSave)
	r.Post("/locations/{id}/delete", p.locationDelete)
	r.Post("/locations/{id}/restore", p.locationRestore)
	r.Get("/services", p.services)
	r.Get("/hosts/new", p.hostForm)
	r.Get("/hosts/{id}", p.host)
	r.Get("/hosts/{id}/edit", p.hostForm)
	r.Post("/hosts", p.hostSave)
	r.Post("/hosts/{id}", p.hostSave)
	r.Post("/hosts/{id}/delete", p.hostDelete)
	r.Post("/hosts/{id}/restore", p.hostRestore)
	r.Post("/hosts/{id}/services", p.serviceAdd)
	r.Post("/hosts/{id}/access", p.accessAdd)
	r.Post("/services/{id}/delete", p.serviceDelete)
	r.Post("/services/{id}/check", p.serviceCheckNow)
	r.Get("/services/{id}/edit", p.serviceForm)
	r.Post("/services/{id}", p.serviceSave)
	r.Get("/access/{id}/edit", p.accessForm)
	r.Post("/access/{id}", p.accessSave)
	r.Get("/app-links/{id}/edit", p.appLinkForm)
	r.Post("/app-links/{id}", p.appLinkSave)
	r.Post("/hosts/{id}/check", p.hostCheckNow)
	r.Post("/hosts/{id}/facts", p.hostFacts)
	r.Post("/hosts/{id}/facts/edit", p.hostFactsEdit)
	r.Post("/hosts/{id}/power", p.hostPower)
	r.Get("/hosts/{id}/context.md", p.hostContext)
	r.Post("/checks/{id}/run", p.checkRun)
	r.Post("/app-links/{id}/delete", p.appLinkDelete)
	r.Post("/access/{id}/delete", p.accessDelete)
	r.Get("/projects", p.projects)
	r.Get("/projects/new", p.projectForm)
	r.Get("/projects/{slug}", p.project)
	r.Get("/projects/{slug}/edit", p.projectForm)
	r.Post("/projects", p.projectSave)
	r.Post("/projects/{slug}", p.projectSave)
	r.Post("/projects/{slug}/delete", p.projectDelete)
	r.Post("/projects/{id}/restore", p.projectRestore)
	r.Post("/projects/{slug}/envs", p.envAdd)
	r.Post("/projects/{slug}/app-links", p.appLinkAdd)
	r.Post("/projects/{slug}/access", p.accessAdd)
	r.Get("/settings/mcp", p.settingsMCP)
	r.Post("/settings/mcp/tokens", p.createToken)
	r.Post("/settings/mcp/tokens/revoke", p.revokeToken)
	r.Post("/settings", p.savePrefs)
	r.Post("/settings/favicons", p.saveFavicons)
	r.Post("/settings/favicons/refetch", p.refetchFavicons)
	r.Get("/settings", p.settingsSection)
	r.Get("/settings/{section:display|currency|scanning|notifications|favicons}", p.settingsSection)
	r.Post("/settings/currency", p.saveCurrency)
	r.Post("/settings/currency/refresh", p.refreshRates)
	r.Post("/settings/display", p.saveDisplay)
	r.Get("/changesets", p.changesetsPage)
	r.Post("/changesets/{id}/apply", p.changesetApply)
	r.Post("/changesets/{id}/reject", p.changesetReject)
	r.Post("/changesets/{id}/undo", p.changesetUndo)
	r.Get("/settings/integrations", p.integrationsPage)
	r.Post("/settings/integrations/{id}", p.integrationSave)
	r.Post("/settings/integrations/{id}/test", p.integrationTest)
	r.Post("/settings/integrations/{id}/sync", p.integrationSync)
	r.Post("/settings/integrations/{id}/propose", p.integrationPropose)
	r.Post("/settings/integrations/{id}/reset", p.integrationReset)
	r.Get("/discovery", p.discoveryPage)
	r.Post("/discovery/scan", p.discoveryScan)
	r.Get("/discovery/status", p.discoveryStatus)
	r.Post("/discovery/subnets", p.discoverySubnets)
	r.Post("/discovery/{id}/register", p.discoveryRegister)
	r.Post("/discovery/{id}/ignore", p.discoveryIgnore)
	r.Get("/calendar", p.calendar)
	r.Get("/costs", p.costs)
	r.Get("/costs.csv", p.costsCSV)
	r.Post("/costs", p.costAdd)
	r.Post("/costs/{id}/delete", p.costDelete)
	r.Post("/costs/{id}/restore", p.costRestore)
	r.Get("/domains", p.domains)
	r.Get("/domains/new", p.domainForm)
	r.Get("/domains/{id}", p.domain)
	r.Get("/domains/{id}/edit", p.domainForm)
	r.Post("/domains", p.domainSave)
	r.Post("/domains/{id}", p.domainSave)
	r.Post("/domains/{id}/delete", p.domainDelete)
	r.Post("/domains/{id}/lookup", p.domainLookup)
	r.Post("/domains/{id}/restore", p.domainRestore)
	r.Post("/domains/{id}/records", p.domainRecordAdd)
	r.Post("/records/{id}/delete", p.domainRecordDelete)
	r.Get("/problems", p.problems)
	r.Get("/maintenance", p.maintenance)
	r.Post("/maintenance", p.maintenanceAdd)
	r.Post("/maintenance/{id}/delete", p.maintenanceDelete)
	r.Post("/checks/{id}/toggle", p.checkToggle)
	r.Get("/export/ssh-config", p.exportSSH)
	r.Get("/export/inventory.json", p.exportJSON)
}

// shell builds the layout data every page shares.
func (p *pages) shell(r *http.Request, title, page string, cr []Crumb) Layout {
	ctx := r.Context()
	loadStyle(ctx, p.set.Get)
	errMsg := formError(r)
	stats, _ := p.inv.Stats(ctx)
	sidebar, err := p.set.Get(ctx, "sidebar", "open")
	if err != nil || (sidebar != "open" && sidebar != "collapsed") {
		sidebar = "open"
	}
	activeTokens := 0
	if all, err := p.tok.List(ctx); err == nil {
		for _, t := range all {
			if t.RevokedAt == nil {
				activeTokens++
			}
		}
	}
	problems, _ := p.chk.Store().ListProblems(ctx, 100)
	discovered, _ := p.inv.ListDiscovered(ctx, "new")
	pending := pendingChanges(ctx, p)
	// Same order and look as the design mockups; items without a real page yet are left out.
	nav := []NavItem{
		{Key: "dashboard", Label: "Dashboard", Icon: "grid", Href: "/"},
		{Key: "projects", Label: "Projects", Icon: "folder", Href: "/projects", Count: stats.Projects},
		{Key: "hosts", Label: "Hosts", Icon: "server", Href: "/hosts", Count: stats.Hosts},
		{Key: "locations", Label: "Locations", Icon: "globe", Href: "/locations", Count: stats.Locations},
		{Key: "services", Label: "Services", Icon: "activity", Href: "/services", Count: stats.Services},
		{Key: "domains", Label: "Domains", Icon: "globe", Href: "/domains", Count: stats.Domains, CountClass: expiringClass(stats.Expiring)},
		{Key: "costs", Label: "Costs", Icon: "coin", Href: "/costs"},
		{Key: "discovery", Label: "Discovery", Icon: "radar", Href: "/discovery", Count: len(discovered), CountClass: "warn"},
		{Key: "problems", Label: "Alerts", Icon: "bell", Href: "/problems", Count: len(problems), CountClass: "bad"},
	}
	ops := []NavItem{
		{Key: "changes", Label: "AI changes", Icon: "bot", Href: "/changesets", Count: pending, CountClass: "warn"},
		{Key: "calendar", Label: "Calendar", Icon: "clock", Href: "/calendar", Count: stats.Expiring, CountClass: expiringClass(stats.Expiring)},
		{Key: "maintenance", Label: "Maintenance", Icon: "clock", Href: "/maintenance"},
		{Key: "settings", Label: "Settings", Icon: "sliders", Href: "/settings"},
	}
	var notices []Notice
	for i, pr := range problems {
		if i == 4 {
			break
		}
		cls := "warn"
		if pr.State == "down" {
			cls = "bad"
		}
		notices = append(notices, Notice{Icon: kindIcon(pr.TargetEntity), Class: cls,
			Title: p.chk.Label(ctx, pr.TargetEntity, pr.TargetID) + " " + pr.State, Sub: pr.LastError,
			Href: "/problems", Btn: "Open"})
	}
	if stats.Expiring > 0 {
		notices = append(notices, Notice{Icon: "globe", Class: "warn", Title: plural(stats.Expiring, "domain") + " expiring soon",
			Sub: "within 30 days", Href: "/calendar", Btn: "View"})
	}
	if pending > 0 {
		notices = append(notices, Notice{Icon: "bot", Class: "info", Group: "s", Title: "AI proposed " + plural(pending, "change set"),
			Sub: "waiting for your review", Href: "/changesets", Btn: "Review"})
	}
	if len(discovered) > 0 {
		notices = append(notices, Notice{Icon: "radar", Class: "info", Group: "s", Title: plural(len(discovered), "new device") + " on the network",
			Sub: "not in the inventory yet", Href: "/discovery", Btn: "See"})
	}
	return Layout{
		Title: title, Page: page, Sidebar: sidebar, HostLabel: r.Host, Version: Version, Error: errMsg,
		Nav: nav, Ops: ops, Crumbs: cr, Stats: stats, TokenCount: activeTokens,
		Notices: notices, Alerts: len(problems) + stats.Expiring,
	}
}
