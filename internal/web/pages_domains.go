package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dawidof/home-services/internal/fx"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/rdap"
	"github.com/go-chi/chi/v5"
)

// ---- domains ----

type domainsPage struct {
	Layout
	Domains   []domainRow
	Providers []providerCard
	Records   []recordRow
	Unmatched int
	RecordN   int
	DNSFilter string
	Upcoming  []upcomingItem
	YearCents int64
	YearCur   string
	Expiring  int
	Unrenewed *domainRow // first domain that will lapse soon (auto-renew off)
}

type domainRow struct {
	inventory.Domain
	RecordCount int
	Pretty      string
}

type providerCard struct {
	Name    string
	Domains int
	Records int
	Source  string
}

type recordRow struct {
	inventory.DomainRecord
	FQDN     string
	DomainID int64
	Host     *inventory.Host
	HostAddr string
}

// upcomingItem is one row of a "Coming up" list (mockup: .exp-list).
type upcomingItem struct {
	MonthHead  string // set on the first item of each month
	Mon, Day   string
	Title, Sub string
	Badge      string
	BadgeClass string
	Warn       bool
	Href       string
}

func prettyDate(d string) string {
	return day(d)
}

// upcoming lists domain expiries and cost renewals within the next days.
func (p *pages) upcoming(ctx context.Context, days int) []upcomingItem {
	type ev struct {
		date string
		it   upcomingItem
	}
	var evs []ev
	if list, err := p.inv.ListDomains(ctx); err == nil {
		for _, d := range list {
			if d.ExpiresAt == "" || d.DaysLeft > days {
				continue
			}
			it := upcomingItem{Title: d.Name, Sub: "Domain", Href: "/domains/" + strconv.FormatInt(d.ID, 10)}
			if d.Registrar != "" {
				it.Sub += " · " + d.Registrar
			}
			if d.PriceCents > 0 {
				it.Sub += " · " + money(d.PriceCents, d.Currency)
			}
			switch {
			case d.DaysLeft < 0:
				it.Badge, it.BadgeClass, it.Warn = "expired", "bad", true
			case !d.AutoRenew && d.DaysLeft <= 60:
				it.Badge, it.BadgeClass, it.Warn = "decide", "warn", true
				it.Sub += " · auto-renew off"
			case d.AutoRenew:
				it.Badge, it.BadgeClass = "auto-renew", "ok"
			default:
				it.Badge = "manual"
			}
			evs = append(evs, ev{d.ExpiresAt, it})
		}
	}
	if list, err := p.inv.ListCosts(ctx); err == nil {
		for _, c := range list {
			if c.NextRenewal == "" || c.DaysLeft > days || c.DaysLeft < 0 {
				continue
			}
			it := upcomingItem{Title: c.Label, Sub: strings.ReplaceAll(c.Cycle, "_", "-") + " · " + money(c.AmountCents, c.Currency), Href: "/costs"}
			if c.AutoRenew {
				it.Badge = "auto-renew"
			} else if c.DaysLeft <= 30 {
				it.Badge, it.BadgeClass, it.Warn = "soon", "warn", true
			} else {
				it.Badge = "manual"
			}
			evs = append(evs, ev{c.NextRenewal, it})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].date < evs[j].date })
	out := []upcomingItem{}
	lastMonth := ""
	for _, e := range evs {
		t, err := time.Parse("2006-01-02", e.date)
		if err != nil {
			continue
		}
		it := e.it
		it.Mon, it.Day = t.Format("Jan"), strconv.Itoa(t.Day())
		if m := t.Format("January 2006"); m != lastMonth {
			it.MonthHead, lastMonth = m, m
		}
		out = append(out, it)
	}
	return out
}

func (p *pages) domains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := p.shell(r, "Domains", "domains", crumbs(Crumb{Label: "Domains"}))
	l.Undo = undoFromQuery(r, "/domains/")
	list, err := p.inv.ListDomains(ctx)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	hosts, _ := p.inv.ListHosts(ctx)
	hostByID := map[int64]*inventory.Host{}
	for i := range hosts {
		hostByID[hosts[i].ID] = &hosts[i]
	}
	page := domainsPage{Layout: l, Expiring: l.Stats.Expiring, DNSFilter: r.URL.Query().Get("dns"), YearCur: p.defaultCurrency(ctx)}
	rates := fx.Load(ctx, p.set)
	provIdx := map[string]int{}
	for _, d := range list {
		full, err := p.inv.GetDomain(ctx, d.ID)
		if err != nil {
			continue
		}
		full.DaysLeft = d.DaysLeft
		row := domainRow{Domain: *full, RecordCount: len(full.Records), Pretty: prettyDate(full.ExpiresAt)}
		page.Domains = append(page.Domains, row)
		if !full.AutoRenew && full.ExpiresAt != "" && full.DaysLeft <= 60 && page.Unrenewed == nil {
			rr := row
			page.Unrenewed = &rr
		}
		if v, ok := rates.Convert(full.PriceCents, full.Currency, page.YearCur); ok {
			page.YearCents += v
		}
		prov := full.Registrar
		if prov == "" {
			prov = "Added by hand"
		}
		i, ok := provIdx[prov]
		if !ok {
			i = len(page.Providers)
			provIdx[prov] = i
			page.Providers = append(page.Providers, providerCard{Name: prov, Source: full.Source})
		}
		page.Providers[i].Domains++
		page.Providers[i].Records += len(full.Records)
		for _, rec := range full.Records {
			page.RecordN++
			rr := recordRow{DomainRecord: rec, DomainID: full.ID, FQDN: rec.Name}
			if rec.Name == "" || rec.Name == "@" {
				rr.FQDN = full.Name
			} else if !strings.HasSuffix(rec.Name, full.Name) {
				rr.FQDN = rec.Name + "." + full.Name
			}
			if rec.MatchedHost != nil {
				if h := hostByID[*rec.MatchedHost]; h != nil {
					rr.Host = h
					for _, a := range h.Addresses {
						if a.IsPrimary || rr.HostAddr == "" {
							rr.HostAddr = a.Address
						}
					}
				}
			}
			if rr.Host == nil {
				page.Unmatched++
				page.Records = append(page.Records, rr)
				continue
			}
			if page.DNSFilter != "unmatched" {
				page.Records = append(page.Records, rr)
			}
		}
	}
	page.Upcoming = p.upcoming(ctx, 183)
	p.ui.render(w, 200, "domains", page)
}

type domainPage struct {
	Layout
	Domain    *inventory.Domain
	Hosts     []inventory.Host
	FormError string
	Records   []recordRow
	UsedBy    int
	Stale     int
	Pretty    string
	PrevYear  string
	Pct       int
	NSList    []string // recorded nameservers, one per entry
	NS        *nsCheck // live lookup, only when asked (?ns=check)
}

type nsCheck struct {
	Live    []string
	Match   bool
	Missing []string // recorded but not live
	Extra   []string // live but not recorded
	Err     string
}

func splitNS(s string) []string {
	out := []string{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == ';' }) {
		if f = strings.TrimSuffix(strings.TrimSpace(f), "."); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// lookupNS asks public DNS which nameservers the domain delegates to and
// compares them with what was recorded.
func lookupNS(ctx context.Context, name string, recorded []string) *nsCheck {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	res := &nsCheck{}
	ns, err := net.DefaultResolver.LookupNS(ctx, name)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	live := map[string]bool{}
	for _, n := range ns {
		h := strings.ToLower(strings.TrimSuffix(n.Host, "."))
		res.Live = append(res.Live, h)
		live[h] = true
	}
	sort.Strings(res.Live)
	rec := map[string]bool{}
	for _, r := range recorded {
		rec[r] = true
		if !live[r] {
			res.Missing = append(res.Missing, r)
		}
	}
	for _, l := range res.Live {
		if !rec[l] {
			res.Extra = append(res.Extra, l)
		}
	}
	res.Match = len(recorded) > 0 && len(res.Missing) == 0 && len(res.Extra) == 0
	return res
}

// recordRows resolves each record's full name and matched host.
func recordRows(d *inventory.Domain, hosts []inventory.Host) []recordRow {
	byID := map[int64]*inventory.Host{}
	for i := range hosts {
		byID[hosts[i].ID] = &hosts[i]
	}
	out := []recordRow{}
	for _, rec := range d.Records {
		rr := recordRow{DomainRecord: rec, DomainID: d.ID, FQDN: rec.Name}
		if rec.Name == "" || rec.Name == "@" {
			rr.FQDN = d.Name
		} else if !strings.HasSuffix(rec.Name, d.Name) {
			rr.FQDN = rec.Name + "." + d.Name
		}
		if rec.MatchedHost != nil {
			if h := byID[*rec.MatchedHost]; h != nil {
				rr.Host = h
				for _, a := range h.Addresses {
					if a.IsPrimary || rr.HostAddr == "" {
						rr.HostAddr = a.Address
					}
				}
			}
		}
		out = append(out, rr)
	}
	return out
}

func (p *pages) domain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := p.inv.GetDomain(ctx, id)
	if errors.Is(err, inventory.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l := p.shell(r, d.Name, "domains", crumbs(
		Crumb{Label: "Domains", Href: "/domains"}, Crumb{Label: d.Name},
	))
	l.Flash = flash(r)
	hosts, _ := p.inv.ListHosts(ctx)
	page := domainPage{Layout: l, Domain: d, Hosts: hosts, FormError: formError(r), Records: recordRows(d, hosts)}
	page.NSList = splitNS(d.Nameservers)
	if r.URL.Query().Get("ns") == "check" && len(page.NSList) > 0 {
		page.NS = lookupNS(ctx, d.Name, page.NSList)
	}
	used := map[int64]bool{}
	for _, rr := range page.Records {
		if rr.Host != nil {
			used[rr.Host.ID] = true
		}
		if rr.Stale {
			page.Stale++
		}
	}
	page.UsedBy = len(used)
	if t, err := time.Parse("2006-01-02", d.ExpiresAt); err == nil {
		page.Pretty = fmtDay(t)
		page.PrevYear = fmtDay(t.AddDate(-1, 0, 0))
		page.Pct = min(max(100-d.DaysLeft*100/365, 0), 100)
	}
	p.ui.render(w, 200, "domain", page)
}

type domainFormPage struct {
	Layout
	IsNew  bool
	Action string
	Domain *inventory.Domain
	Error  string
}

func (p *pages) domainForm(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	isNew := id == 0
	d := &inventory.Domain{Currency: p.defaultCurrency(r.Context()), Source: "manual"}
	if !isNew {
		var err error
		d, err = p.inv.GetDomain(r.Context(), id)
		if errors.Is(err, inventory.ErrNotFound) {
			p.notFound(w, r)
			return
		}
		if err != nil {
			p.fail(w, r, err)
			return
		}
	}
	l := p.shell(r, "Domain", "domains", crumbs(Crumb{Label: "Domains", Href: "/domains"}, Crumb{Label: "Form"}))
	action := "/domains"
	if isNew {
		l.Crumbs = crumbs(Crumb{Label: "Domains", Href: "/domains"}, Crumb{Label: "New domain"})
	} else {
		l.Crumbs = crumbs(Crumb{Label: "Domains", Href: "/domains"}, Crumb{Label: d.Name, Href: "/domains/" + itoa(d.ID)}, Crumb{Label: "Edit"})
		action = "/domains/" + itoa(d.ID)
	}
	p.ui.render(w, 200, "domain_form", domainFormPage{Layout: l, IsNew: isNew, Action: action, Domain: d})
}

func (p *pages) domainSave(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	price := 0.0
	if v := strings.TrimSpace(r.FormValue("price")); v != "" {
		price, _ = strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	}
	d := inventory.Domain{
		ID: id, Name: r.FormValue("name"), Registrar: r.FormValue("registrar"),
		Account: r.FormValue("account"), ExpiresAt: r.FormValue("expires_at"),
		AutoRenew: r.FormValue("auto_renew") != "", PriceCents: int64(price*100 + 0.5),
		Currency: r.FormValue("currency"), Notes: r.FormValue("notes"), Source: "manual",
		PaidAt:       strings.TrimSpace(r.FormValue("paid_at")),
		Nameservers:  strings.TrimSpace(r.FormValue("nameservers")),
		DNSSEC:       r.FormValue("dnssec") != "",
		TransferLock: r.FormValue("transfer_lock") != "",
	}
	if v := strings.TrimSpace(r.FormValue("charged")); v != "" {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && f >= 0 {
			d.ChargedCents = int64(f*100 + 0.5)
		}
	}
	// The payment label is only set by integrations; keep it and the source.
	// Forms without the DNS section (older pages, API-shaped posts) keep the
	// imported nameservers/DNSSEC/transfer lock instead of clearing them.
	if id != 0 {
		if old, err := p.inv.GetDomain(r.Context(), id); err == nil {
			d.PaymentLabel, d.Source = old.PaymentLabel, old.Source
			if r.FormValue("dns") == "" {
				d.Nameservers, d.DNSSEC, d.TransferLock = old.Nameservers, old.DNSSEC, old.TransferLock
			}
		}
	}
	lookupNote := ""
	if r.FormValue("lookup") != "" && (d.Registrar == "" || d.ExpiresAt == "") {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		info, err := rdap.Lookup(ctx, d.Name)
		cancel()
		if err != nil {
			lookupNote = err.Error()
		} else {
			if d.Registrar == "" {
				d.Registrar = info.Registrar
			}
			if d.ExpiresAt == "" {
				d.ExpiresAt = info.ExpiresAt
			}
		}
	}
	note := p.lockDomainRate(r, &d)
	var err error
	if id == 0 {
		_, err = p.inv.CreateDomain(r.Context(), actorOf(r), d)
	} else {
		_, err = p.inv.UpdateDomain(r.Context(), actorOf(r), id, d)
	}
	if err != nil {
		l := p.shell(r, "Domain", "domains", crumbs(Crumb{Label: "Domains", Href: "/domains"}, Crumb{Label: "Form"}))
		p.ui.render(w, 422, "domain_form", domainFormPage{
			Layout: l, IsNew: id == 0, Action: r.URL.Path, Domain: &d, Error: err.Error(),
		})
		return
	}
	if lookupNote != "" {
		backWithError(w, r, ternary(id == 0, "/domains", "/domains/"+itoa(id)), errors.New("saved, but the registration lookup failed: "+lookupNote))
		return
	}
	if id == 0 {
		if note != "" {
			http.Redirect(w, r, "/domains?ok=msg&msg="+url.QueryEscape("Domain added. "+note), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/domains?ok=created", http.StatusSeeOther)
		return
	}
	if note != "" {
		http.Redirect(w, r, "/domains/"+itoa(id)+"?ok=msg&msg="+url.QueryEscape("Saved. "+note), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/domains/"+itoa(id)+"?ok=saved", http.StatusSeeOther)
}

// lockDomainRate mirrors lockRate for domain renewals: a bank charge wins,
// else the ECB rate of the paid day; only when the currency differs.
func (p *pages) lockDomainRate(r *http.Request, d *inventory.Domain) string {
	def := p.defaultCurrency(r.Context())
	cur := strings.ToUpper(strings.TrimSpace(d.Currency))
	if cur == "" || cur == def {
		return ""
	}
	if d.ChargedCents > 0 {
		d.ChargedCurrency, d.ChargedNote = def, "as charged"
		return ""
	}
	if d.PaidAt == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	t, day, err := fx.TableOn(ctx, p.set, d.PaidAt)
	if err != nil {
		return "Couldn't get the rate of " + d.PaidAt + " (" + err.Error() + "), so today's rate is used for it."
	}
	v, ok := t.Convert(d.PriceCents, cur, def)
	if !ok {
		return "No " + cur + " rate for " + d.PaidAt + " — add a manual rate in Settings → Currency."
	}
	d.ChargedCents, d.ChargedCurrency, d.ChargedNote = v, def, "ECB rate of "+day
	return ""
}

// domainLookup refreshes registrar and expiry from RDAP (public registration data).
func (p *pages) domainLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := p.inv.GetDomain(ctx, id)
	if err != nil {
		p.notFound(w, r)
		return
	}
	lctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	info, err := rdap.Lookup(lctx, d.Name)
	cancel()
	if err != nil {
		backWithError(w, r, "/domains/"+itoa(id), err)
		return
	}
	if info.Registrar == "" && info.ExpiresAt == "" {
		backWithError(w, r, "/domains/"+itoa(id), errors.New("the registry does not publish a registrar or expiry date for this domain"))
		return
	}
	if info.Registrar != "" {
		d.Registrar = info.Registrar
	}
	if info.ExpiresAt != "" {
		d.ExpiresAt = info.ExpiresAt
	}
	if _, err := p.inv.UpdateDomain(ctx, actorOf(r), id, *d); err != nil {
		backWithError(w, r, "/domains/"+itoa(id), err)
		return
	}
	http.Redirect(w, r, "/domains/"+itoa(id)+"?ok=looked-up", http.StatusSeeOther)
}

func (p *pages) domainDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := p.inv.GetDomain(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteDomain(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/domains?ok=deleted&id="+itoa(id)+"&name="+url.QueryEscape(d.Name), http.StatusSeeOther)
}

func (p *pages) domainRestore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.RestoreDomain(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/domains?ok=restored", http.StatusSeeOther)
}

func (p *pages) domainRecordAdd(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	rec := inventory.DomainRecord{
		Name: r.FormValue("name"), Type: r.FormValue("type"), Content: r.FormValue("content"),
		Proxied: r.FormValue("proxied") != "", Stale: r.FormValue("stale") != "",
	}
	if v := intField(r, "matched_host_id"); v > 0 {
		hid := int64(v)
		rec.MatchedHost = &hid
	}
	if _, err := p.inv.AddDomainRecord(r.Context(), actorOf(r), id, rec); err != nil {
		backWithError(w, r, "/domains/"+itoa(id), err)
		return
	}
	http.Redirect(w, r, "/domains/"+itoa(id)+"?ok=record", http.StatusSeeOther)
}

func (p *pages) domainRecordDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	domainID := int64(intField(r, "domain_id"))
	if err := p.inv.DeleteDomainRecord(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/domains/"+itoa(domainID)+"?ok=record-deleted", http.StatusSeeOther)
}
