package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/power"
	"github.com/go-chi/chi/v5"
)

// ---- hosts ----

type hostRow struct {
	inventory.Host
	ServiceCount int
	ServicesUp   int
	ServiceDot   string // ok | warn | bad | "" (no checks)
	ProjectCount int
	SSHCopy      string // ssh command for the copy button
	SSHURL       template.URL
}

// hostChip is one filter chip on the hosts page.
type hostChip struct {
	Label, Href, Initial, Color string
	Count                       int
	On                          bool
	Warn                        bool
}

type hostsPage struct {
	Layout
	Rows      []hostRow
	Q         string
	Total     int
	LocChips  []hostChip
	LifeChips []hostChip
}

func (p *pages) hosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	qs := r.URL.Query()
	q := strings.ToLower(strings.TrimSpace(qs.Get("q")))
	loc, life, sshFix := qs.Get("loc"), qs.Get("lc"), qs.Get("ssh") == "fix"
	if life == "" && !sshFix {
		life = "active"
	}
	l := p.shell(r, "Hosts", "hosts", crumbs(Crumb{Label: "Hosts"}))
	l.Undo = undoFromQuery(r, "/hosts/")
	all, err := p.inv.ListHosts(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	states := map[int64][]string{}
	if list, err := p.chk.Store().List(ctx, "service", 0); err == nil {
		for _, c := range list {
			if c.Enabled {
				states[c.TargetID] = append(states[c.TargetID], c.State)
			}
		}
	}
	link := func(loc, life string, fix bool) string {
		v := url.Values{}
		if loc != "" {
			v.Set("loc", loc)
		}
		if life != "" && life != "active" {
			v.Set("lc", life)
		}
		if fix {
			v.Set("ssh", "fix")
		}
		if q != "" {
			v.Set("q", qs.Get("q"))
		}
		if len(v) == 0 {
			return "/hosts"
		}
		return "/hosts?" + v.Encode()
	}
	locCount, locOrder, locSeen := map[string]int{}, []string{}, map[string]bool{}
	lifeCount := map[string]int{}
	needFix := 0
	rows := []hostRow{}
	for _, h := range all {
		full, err := p.inv.GetHost(ctx, h.ID)
		if err != nil {
			continue
		}
		if q != "" && !hostMatches(full, q) {
			continue
		}
		locName := ""
		if full.Location != nil {
			locName = full.Location.Name
		}
		if locName != "" && !locSeen[locName] {
			locSeen[locName] = true
			locOrder = append(locOrder, locName)
		}
		lc := full.Lifecycle
		if lc == "" {
			lc = "active"
		}
		fix := false
		for _, s := range full.SSH {
			if s.Status == "password_allowed" {
				fix = true
			}
		}
		if loc == "" || loc == locName {
			lifeCount[lc]++
			if fix {
				needFix++
			}
		}
		if life == "" || life == lc {
			locCount[locName]++
		}
		if (loc != "" && loc != locName) || (life != "" && life != lc) || (sshFix && !fix) {
			continue
		}
		row := hostRow{Host: *full, ServiceCount: len(full.Services), ProjectCount: len(full.Projects)}
		worst := ""
		for _, v := range full.Services {
			ok := true
			for _, st := range states[v.ID] {
				if betterState(st, worst) {
					worst = st
				}
				if st != "up" {
					ok = false
				}
			}
			if ok {
				row.ServicesUp++
			}
		}
		row.ServiceDot = map[string]string{"up": "ok", "failing": "warn", "down": "bad"}[worst]
		if len(full.SSH) > 0 && len(full.Addresses) > 0 {
			s := full.SSH[0]
			row.SSHCopy = "ssh " + s.User + "@" + full.Addresses[0].Address
			row.SSHURL = sshURL(s.User, full.Addresses[0].Address, s.Port)
			if s.Port != 0 && s.Port != 22 {
				row.SSHCopy += " -p " + strconv.Itoa(s.Port)
			}
		}
		rows = append(rows, row)
	}
	total := 0
	for _, n := range locCount {
		total += n
	}
	locChips := []hostChip{{Label: "All", Href: link("", life, sshFix), Count: total, On: loc == ""}}
	for _, name := range locOrder {
		locChips = append(locChips, hostChip{Label: name, Href: link(name, life, sshFix), Count: locCount[name],
			On: loc == name, Initial: initial(name), Color: colorFor(name)})
	}
	lifeChips := []hostChip{}
	for _, lc := range []string{"active", "planned", "deprecated", "decommissioned"} {
		if lifeCount[lc] == 0 && lc != "active" {
			continue
		}
		lifeChips = append(lifeChips, hostChip{Label: strings.ToUpper(lc[:1]) + lc[1:], Href: link(loc, lc, false),
			Count: lifeCount[lc], On: life == lc && !sshFix})
	}
	if needFix > 0 {
		lifeChips = append(lifeChips, hostChip{Label: "Needs SSH fix", Href: link(loc, "", true), Count: needFix, On: sshFix, Warn: true})
	}
	p.ui.render(w, 200, "hosts", hostsPage{Layout: l, Rows: rows, Q: qs.Get("q"), Total: len(all),
		LocChips: locChips, LifeChips: lifeChips})
}

func hostMatches(h *inventory.Host, q string) bool {
	hay := strings.ToLower(h.Name + " " + h.Hostname + " " + h.Description + " " + strings.Join(h.Tags, " "))
	for _, a := range h.Addresses {
		hay += " " + strings.ToLower(a.Address)
	}
	return strings.Contains(hay, q)
}

type hostPage struct {
	Layout
	Host         *inventory.Host
	Audit        []inventory.AuditEntry
	ServiceCheck map[int64]*checks.Check
	SSHCheck     map[int64]*checks.Check
	ServiceBars  map[int64][]checks.Bar
	SSHBars      map[int64][]checks.Bar
	SSHPassword  bool
	FirstSSH     *inventory.SSHAccess
	HostChecks   []checks.Check
	FormError    string
	// header + security card, same as the design mockup
	SiteURL, PublicURL string
	SSHCmd             string
	SSHURL             template.URL // ssh:// is not a "safe" scheme for html/template
	SvcUp, SvcDown     int
	FactsSSH           bool
	FactsUser          string
	HostState          string // ok | warn | bad | ""
	Score              string // a | b | c
	ScoreText          string
	Security           []secItem
	Power              *power.Reading
	PowerCostCents     int64 // per month in the default currency, 0 without a price
	Currency           string
	HAReady            bool
}

// secItem is one line of the host Security card.
type secItem struct {
	OK   bool
	Text string
	Tip  string
}

func (p *pages) host(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h, err := p.inv.GetHost(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l := p.shell(r, h.Name, "hosts", crumbs(
		Crumb{Label: "Hosts", Href: "/hosts"},
		Crumb{Label: h.Name},
	))
	l.Flash = flash(r)
	audit, _ := p.inv.ListAudit(ctx, "host", id, 25)
	serviceChecks := map[int64]*checks.Check{}
	for _, v := range h.Services {
		if list, err := p.chk.Store().List(ctx, "service", v.ID); err == nil && len(list) > 0 {
			serviceChecks[v.ID] = &list[0]
		}
	}
	sshChecks := map[int64]*checks.Check{}
	for _, x := range h.SSH {
		if list, err := p.chk.Store().List(ctx, "ssh_access", x.ID); err == nil && len(list) > 0 {
			sshChecks[x.ID] = &list[0]
		}
	}
	serviceBars := map[int64][]checks.Bar{}
	for _, v := range h.Services {
		serviceBars[v.ID], _ = p.chk.Store().AvailabilityFor(ctx, "service", v.ID)
	}
	sshBars := map[int64][]checks.Bar{}
	passwordAuth := false
	for _, x := range h.SSH {
		sshBars[x.ID], _ = p.chk.Store().AvailabilityFor(ctx, "ssh_access", x.ID)
		if x.Status == "password_allowed" {
			passwordAuth = true
		}
	}
	hostChecks, _ := p.chk.Store().List(ctx, "host", id)
	var firstSSH *inventory.SSHAccess
	if len(h.SSH) > 0 {
		firstSSH = &h.SSH[0]
	}
	page := hostPage{
		Layout: l, Host: h, Audit: audit, ServiceCheck: serviceChecks, SSHCheck: sshChecks,
		ServiceBars: serviceBars, SSHBars: sshBars, SSHPassword: passwordAuth, FirstSSH: firstSSH,
		HostChecks: hostChecks, FormError: formError(r),
	}
	worst := ""
	for _, v := range h.Services {
		if c := serviceChecks[v.ID]; c != nil {
			if c.State == "up" {
				page.SvcUp++
			} else {
				page.SvcDown++
			}
			if betterState(c.State, worst) {
				worst = c.State
			}
		}
		if page.SiteURL == "" {
			if u := serviceURL(h, &v); u != "" {
				page.SiteURL = u
				if v.Domain != "" {
					page.PublicURL = u
				}
			}
		}
	}
	for _, c := range hostChecks {
		if betterState(c.State, worst) {
			worst = c.State
		}
	}
	page.HostState = map[string]string{"up": "ok", "failing": "warn", "down": "bad"}[worst]
	if firstSSH != nil && len(h.Addresses) > 0 {
		page.SSHCmd = "ssh " + firstSSH.User + "@" + h.Addresses[0].Address
		page.SSHURL = sshURL(firstSSH.User, h.Addresses[0].Address, firstSSH.Port)
		if firstSSH.Port != 0 && firstSSH.Port != 22 {
			page.SSHCmd += " -p " + strconv.Itoa(firstSSH.Port)
		}
	}
	// Security card: built only from what was actually checked.
	issues := 0
	add := func(ok bool, text, tip string) {
		page.Security = append(page.Security, secItem{OK: ok, Text: text, Tip: tip})
		if !ok {
			issues++
		}
	}
	keyOnly, checked := 0, 0
	for _, x := range h.SSH {
		if x.Status == "key_only" || x.Status == "password_allowed" {
			checked++
		}
		if x.Status == "key_only" {
			keyOnly++
		}
	}
	switch {
	case len(h.SSH) == 0:
	case passwordAuth:
		add(false, "The SSH server still accepts passwords", "Anyone who can reach the port can try to guess a password. Switch to keys — see SSH access.")
	case checked == len(h.SSH):
		add(true, "Key-only SSH on all users", "The server refuses password logins, so nobody can guess their way in.")
	case h.SSH[0].Status == "unreachable":
		add(false, "SSH did not answer", "The port is closed or filtered from here. Check the address and port.")
	default:
		add(false, "SSH not checked yet", "Press Check now to ask the server which login methods it offers.")
	}
	if f := h.Facts; f != nil {
		if f.RootLogin != "" {
			add(f.RootLogin != "yes", "Root login: "+f.RootLogin, "")
		}
		if f.FailedUnits > 0 {
			add(false, plural(f.FailedUnits, "failed systemd unit"), "")
		}
		if f.RebootRequired == "yes" {
			add(false, "Reboot needed", "")
		}
		if f.DiskUsedPct >= 85 {
			add(false, "Disk "+strconv.Itoa(f.DiskUsedPct)+"% full", "")
		}
	}
	public := false
	for _, a := range h.Addresses {
		if a.Kind == "public" {
			public = true
		}
	}
	add(!public, ternary(public, "Has a public address", "Not exposed to the internet"), "")
	page.Score, page.ScoreText = "a", "Good"
	switch {
	case issues >= 2:
		page.Score, page.ScoreText = "c", "Needs work, "+plural(issues, "issue")
	case issues == 1:
		page.Score, page.ScoreText = "b", "Good, 1 issue"
	}
	page.FactsSSH = p.facts != nil
	page.FactsUser = os.Getenv("HS_SSH_FACTS_USER")
	if page.FactsUser == "" {
		page.FactsUser = "inventory"
	}
	page.Power, _ = p.pow.Get(ctx, id)
	page.Currency = p.defaultCurrency(ctx)
	if p.intg != nil {
		pc := PowerConfig(ctx, p.intg)
		page.HAReady = pc.HAURL != "" && pc.HAToken != ""
		if page.Power != nil && pc.PricePerKWh > 0 {
			page.PowerCostCents = int64(page.Power.MonthlyKWh()*pc.PricePerKWh*100 + 0.5)
		}
	}
	p.ui.render(w, 200, "host", page)
}

// sshURL builds an ssh:// link (with the port when it isn't 22). The user and
// address come from the inventory, so they are escaped before being trusted.
func sshURL(user, addr string, port int) template.URL {
	host := addr
	if strings.Contains(addr, ":") { // IPv6
		host = "[" + addr + "]"
	}
	if port != 0 && port != 22 {
		host += ":" + strconv.Itoa(port)
	}
	u := url.URL{Scheme: "ssh", User: url.User(user), Host: host}
	return template.URL(u.String())
}

// hostContext is the Markdown an AI agent gets for one host (same as the MCP
// resource), shown in the "What an AI agent sees" drawer.
func (p *pages) hostContext(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	md, err := p.inv.HostMarkdown(r.Context(), id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(md))
}

// hostFactsEdit saves facts typed in by hand.
// powerPrice is the configured price per kWh, 0 when electricity isn't set up.
func (p *pages) powerPrice(ctx context.Context) float64 {
	if p.intg == nil {
		return 0
	}
	return PowerConfig(ctx, p.intg).PricePerKWh
}

// PowerConfig reads the Electricity integration's settings for the poller.
func PowerConfig(ctx context.Context, m *integrations.Manager) power.Config {
	cfg, err := m.Config(ctx, "power")
	if err != nil {
		return power.Config{}
	}
	price, _ := integrations.ParsePrice(cfg["price_kwh"])
	return power.Config{PricePerKWh: price, HAURL: cfg["ha_url"], HAToken: cfg["ha_token"]}
}

func (p *pages) hostPower(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := p.inv.GetHost(ctx, id); err != nil {
		p.notFound(w, r)
		return
	}
	back := "/hosts/" + itoa(id)
	if r.FormValue("source") == "" {
		_ = p.pow.Set(ctx, id, "", "", 0)
		http.Redirect(w, r, back+"?ok=msg&msg="+url.QueryEscape("Power figure removed.")+"#facts", http.StatusSeeOther)
		return
	}
	src := r.FormValue("source")
	watts, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.FormValue("manual_w")), ",", "."), 64)
	ref := r.FormValue("ref_" + src)
	if err := p.pow.Set(ctx, id, src, ref, watts); err != nil {
		backWithError(w, r, back, err)
		return
	}
	msg := "Power saved."
	if src != power.Manual && src != "" {
		cfg := power.Config{}
		if p.intg != nil {
			cfg = PowerConfig(ctx, p.intg)
		}
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		v, err := power.Read(rctx, cfg, src, ref)
		cancel()
		_ = p.pow.Record(ctx, id, v, err)
		if err != nil {
			msg = "Saved, but the first reading failed: " + err.Error()
		} else {
			msg = fmt.Sprintf("Saved — reading now %.1f W. It's read every 5 minutes.", v)
		}
	}
	http.Redirect(w, r, back+"?ok=msg&msg="+url.QueryEscape(msg)+"#facts", http.StatusSeeOther)
}

func (p *pages) hostFactsEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := p.inv.GetHost(ctx, id); err != nil {
		p.notFound(w, r)
		return
	}
	num := func(k string, hi int) int {
		n, _ := strconv.Atoi(strings.TrimSpace(r.FormValue(k)))
		return min(max(n, 0), hi)
	}
	pick := func(k string, allowed ...string) string {
		v := r.FormValue(k)
		if contains(allowed, v) {
			return v
		}
		return allowed[0]
	}
	f := inventory.HostFacts{
		HostID: id, OS: strings.TrimSpace(r.FormValue("os")), Kernel: strings.TrimSpace(r.FormValue("kernel")),
		Uptime: strings.TrimSpace(r.FormValue("uptime")), DiskUsedPct: num("disk_used_pct", 100),
		FailedUnits: num("failed_units", 10000), RebootRequired: pick("reboot_required", "no", "yes"),
		PasswordAuth: pick("password_auth", "unknown", "no", "yes"), RootLogin: strings.TrimSpace(r.FormValue("root_login")),
	}
	if old, err := p.inv.GetHostFacts(ctx, id); err == nil && old != nil {
		f.Hostname, f.KeyFingerprints = old.Hostname, old.KeyFingerprints
	}
	if err := p.inv.SaveHostFacts(ctx, actorOf(r), f); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/hosts/"+itoa(id)+"?ok=facts-saved#facts", http.StatusSeeOther)
}
