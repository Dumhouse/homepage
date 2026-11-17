package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/fx"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- costs ----

type costRow struct {
	ID             int64
	Kind           string // monthly | yearly | one_time | estimate | domain
	Label          string
	Detail         string
	Project        string
	Payment        string
	AmountCents    int64
	Currency       string
	NextRenewal    string
	AutoRenew      bool
	Explicit       bool
	DaysLeft       int
	MonthlyCents   int64 // in the default currency
	YearCents      int64 // in the default currency
	ConvCents      int64 // AmountCents in the default currency
	Foreign        bool  // Currency differs from the default
	NoRate         bool  // no exchange rate: left out of totals
	LockedCents    int64
	LockedCurrency string
	RateNote       string // "ECB rate of …", "as charged" or "today's rate"
	Provider       string // registrar for domains
	Account        string
	Pretty         string
}

// Free reports a recurring entry with no amount (a free plan).
func (c costRow) Free() bool { return c.AmountCents == 0 && c.Kind != "one_time" }

type costBar struct {
	Label string
	Href  string
	Cents int64
	Pct   int
	Color string
	Warn  bool
}

type costsPage struct {
	Layout
	Rows         []costRow
	Recurring    []costRow
	Hardware     []costRow
	ByKind       []costBar
	ByProject    []costBar
	FreeNames    []string
	Decide       int
	Filter       string // "" | domain | server | other
	PerYear      bool
	Projects     []inventory.Project
	MonthlyCents int64
	YearlyCents  int64
	OneTimeCents int64
	Next30Cents  int64
	Next30Count  int
	Currency     string
	Currencies   []string
	NoRate       []string
	RatesDate    string
}

func (p *pages) costRows(ctx context.Context) ([]costRow, costsPage, error) {
	var page costsPage
	page.Currency = p.defaultCurrency(ctx)
	rates := fx.Load(ctx, p.set)
	page.Currencies, page.RatesDate = currencyChoices(rates, page.Currency), rates.Date
	// conv turns an amount into the default currency; ok=false when no rate.
	conv := func(row *costRow) int64 {
		row.Foreign = !strings.EqualFold(row.Currency, page.Currency) && row.Currency != ""
		from, cents := row.Currency, row.AmountCents
		if row.LockedCents > 0 && row.LockedCurrency != "" { // value fixed on the day it was paid
			from, cents = row.LockedCurrency, row.LockedCents
		} else if row.Foreign {
			row.RateNote = "today's rate"
		}
		v, ok := rates.Convert(cents, from, page.Currency)
		if !ok {
			row.NoRate = true
			if !contains(page.NoRate, row.Currency) {
				page.NoRate = append(page.NoRate, row.Currency)
			}
			return 0
		}
		row.ConvCents = v
		return v
	}
	costs, err := p.inv.ListCosts(ctx)
	if err != nil {
		return nil, page, err
	}
	projects, err := p.inv.ListProjects(ctx)
	if err != nil {
		return nil, page, err
	}
	page.Projects = projects
	projectNames := map[int64]string{}
	for _, pr := range projects {
		projectNames[pr.ID] = pr.Name
	}
	rows := []costRow{}
	monthly, yearly, oneTime := int64(0), int64(0), int64(0)
	for _, c := range costs {
		row := costRow{
			ID: c.ID, Kind: c.Cycle, Label: c.Label, Detail: c.TargetEntity, AmountCents: c.AmountCents,
			Currency: c.Currency, NextRenewal: c.NextRenewal, AutoRenew: c.AutoRenew, Explicit: true,
			DaysLeft: c.DaysLeft, Payment: c.PaymentLabel,
			LockedCents: c.ChargedCents, LockedCurrency: c.ChargedCurrency, RateNote: c.ChargedNote,
		}
		if c.ProjectID != nil {
			row.Project = projectNames[*c.ProjectID]
		}
		amt := conv(&row)
		switch c.Cycle {
		case "monthly", "estimate":
			row.MonthlyCents, row.YearCents = amt, amt*12
		case "yearly":
			row.MonthlyCents, row.YearCents = amt/12, amt
		case "one_time":
			oneTime += amt
		}
		monthly += row.MonthlyCents
		yearly += row.YearCents
		rows = append(rows, row)
	}
	domains, err := p.inv.ListDomains(ctx)
	if err != nil {
		return nil, page, err
	}
	for _, d := range domains {
		if d.PriceCents <= 0 {
			continue
		}
		row := costRow{
			ID: d.ID, Kind: "domain", Label: d.Name, Detail: "domain renewal", AmountCents: d.PriceCents,
			Currency: d.Currency, NextRenewal: d.ExpiresAt, AutoRenew: d.AutoRenew, DaysLeft: d.DaysLeft,
			Payment: d.PaymentLabel, Provider: d.Registrar, Account: d.Account,
			LockedCents: d.ChargedCents, LockedCurrency: d.ChargedCurrency, RateNote: d.ChargedNote,
		}
		amt := conv(&row)
		row.MonthlyCents, row.YearCents = amt/12, amt
		monthly += row.MonthlyCents
		yearly += row.YearCents
		rows = append(rows, row)
	}
	// Electricity: each host's average draw at the configured price.
	if price := p.powerPrice(ctx); price > 0 {
		readings, _ := p.pow.List(ctx)
		hosts, _ := p.inv.ListHosts(ctx)
		for _, h := range hosts {
			pr, ok := readings[h.ID]
			if !ok || pr.Watts() <= 0 {
				continue
			}
			cents := int64(pr.MonthlyKWh()*price*100 + 0.5)
			how := "typed in"
			if pr.Live() {
				how = "measured, " + map[string]string{"ha": "Home Assistant", "shelly": "Shelly", "tasmota": "Tasmota"}[pr.Source]
			}
			row := costRow{
				ID: h.ID, Kind: "power", Label: h.Name, Detail: fmt.Sprintf("%.0f W avg · %.0f kWh/mo · %s", pr.Watts(), pr.MonthlyKWh(), how),
				AmountCents: cents, Currency: page.Currency, ConvCents: cents, MonthlyCents: cents, YearCents: cents * 12,
			}
			if full, err := p.inv.GetHost(ctx, h.ID); err == nil && len(full.Projects) > 0 {
				row.Project = full.Projects[0].Name
			}
			monthly += row.MonthlyCents
			yearly += row.YearCents
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].NextRenewal == "" {
			return false
		}
		if rows[j].NextRenewal == "" {
			return true
		}
		return rows[i].NextRenewal < rows[j].NextRenewal
	})
	next30, next30Count := int64(0), 0
	for _, r := range rows {
		if r.NextRenewal != "" && r.DaysLeft >= 0 && r.DaysLeft <= 30 {
			next30 += r.ConvCents
			next30Count++
		}
	}
	page.Rows, page.MonthlyCents, page.YearlyCents, page.OneTimeCents = rows, monthly, yearly, oneTime
	page.Next30Cents, page.Next30Count = next30, next30Count
	return rows, page, nil
}

func (p *pages) costs(w http.ResponseWriter, r *http.Request) {
	l := p.shell(r, "Costs", "costs", crumbs(Crumb{Label: "Costs"}))
	l.Undo = undoFromQuery(r, "/costs/")
	_, data, err := p.costRows(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	l.Flash = flash(r)
	data.Layout = l
	q := r.URL.Query()
	data.Filter, data.PerYear = q.Get("f"), q.Get("per") == "year"
	kinds := map[string]int64{}
	projects := map[string]int64{}
	for i := range data.Rows {
		row := &data.Rows[i]
		if row.NextRenewal != "" {
			row.Pretty = prettyDate(row.NextRenewal)
		}
		if row.Kind == "one_time" {
			data.Hardware = append(data.Hardware, *row)
			continue
		}
		if row.Free() {
			data.FreeNames = append(data.FreeNames, row.Label)
		}
		if row.NextRenewal != "" && row.DaysLeft >= 0 && row.DaysLeft <= 30 && !row.AutoRenew {
			data.Decide++
		}
		year := row.YearCents
		k := map[string]string{"domain": "Domains", "estimate": "Estimates", "power": "Electricity"}[row.Kind]
		if k == "" {
			k = "Servers & plans"
		}
		kinds[k] += year
		pr := row.Project
		if pr == "" {
			pr = "No project"
		}
		projects[pr] += year
		switch data.Filter {
		case "domain":
			if row.Kind != "domain" {
				continue
			}
		case "server":
			if row.Kind != "monthly" && row.Kind != "yearly" {
				continue
			}
		case "power":
			if row.Kind != "power" {
				continue
			}
		case "other":
			if row.Kind != "estimate" {
				continue
			}
		}
		data.Recurring = append(data.Recurring, *row)
	}
	bars := func(m map[string]int64, colors []string) []costBar {
		total := int64(0)
		for _, v := range m {
			total += v
		}
		out := []costBar{}
		for k, v := range m {
			if v <= 0 {
				continue
			}
			out = append(out, costBar{Label: k, Cents: v, Pct: int(v * 100 / max(total, 1))})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Cents > out[j].Cents })
		for i := range out {
			out[i].Color = colors[i%len(colors)]
		}
		return out
	}
	data.ByKind = bars(kinds, []string{"#5b8def", "#e5a000", "#f38020", "#16a34a"})
	data.ByProject = bars(projects, []string{"#5b9cff", "#7c3aed", "#0ea5a4", "#d97706"})
	for i := range data.ByProject {
		if data.ByProject[i].Label == "No project" {
			data.ByProject[i].Warn = true
		}
		for _, pr := range data.Projects {
			if pr.Name == data.ByProject[i].Label {
				data.ByProject[i].Href = "/projects/" + pr.Slug
			}
		}
	}
	p.ui.render(w, 200, "costs", data)
}

func (p *pages) costsCSV(w http.ResponseWriter, r *http.Request) {
	rows, _, err := p.costRows(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	var b strings.Builder
	b.WriteString("label,kind,project,cycle,amount,currency,next_renewal,auto_renew,payment,detail\n")
	for _, row := range rows {
		cycle := row.Kind
		if row.Kind == "domain" {
			cycle = "yearly"
		}
		fmt.Fprintf(&b, "%q,%q,%q,%q,%.2f,%q,%q,%t,%q,%q\n",
			row.Label, row.Kind, row.Project, cycle, float64(row.AmountCents)/100, row.Currency,
			row.NextRenewal, row.AutoRenew, row.Payment, row.Detail)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="homeservices-costs.csv"`)
	w.Write([]byte(b.String()))
}

func (p *pages) costAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	amount := 0.0
	if v := strings.TrimSpace(r.FormValue("amount")); v != "" {
		amount, _ = strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	}
	c := inventory.Cost{
		Label: r.FormValue("label"), AmountCents: int64(amount*100 + 0.5), Currency: r.FormValue("currency"),
		Cycle: r.FormValue("cycle"), NextRenewal: r.FormValue("next_renewal"),
		AutoRenew: r.FormValue("auto_renew") != "", PaymentLabel: r.FormValue("payment_label"),
		Notes: r.FormValue("notes"), TargetEntity: r.FormValue("target_entity"),
	}
	if v := intField(r, "project_id"); v > 0 {
		id := int64(v)
		c.ProjectID = &id
	}
	c.PaidAt = strings.TrimSpace(r.FormValue("paid_at"))
	note := p.lockRate(r, &c)
	if _, err := p.inv.CreateCost(r.Context(), actorOf(r), c); err != nil {
		backWithError(w, r, "/costs", err)
		return
	}
	if note != "" {
		http.Redirect(w, r, "/costs?ok=msg&msg="+url.QueryEscape("Cost added. "+note), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/costs?ok=cost", http.StatusSeeOther)
}

// lockRate fixes what a cost in another currency was worth in the default
// currency when it was paid: the amount your bank charged if given, else the
// ECB rate of the payment day. One-time purchases default to today as the
// payment day; recurring costs are locked only when a date is given (each
// future renewal is paid at its own day's rate, so totals use today's).
func (p *pages) lockRate(r *http.Request, c *inventory.Cost) string {
	def := p.defaultCurrency(r.Context())
	cur := strings.ToUpper(strings.TrimSpace(c.Currency))
	if cur == "" || cur == def {
		return ""
	}
	if v := strings.TrimSpace(r.FormValue("charged")); v != "" {
		if f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil && f > 0 {
			c.ChargedCents, c.ChargedCurrency, c.ChargedNote = int64(f*100+0.5), def, "as charged"
			return ""
		}
	}
	if c.PaidAt == "" && c.Cycle == "one_time" {
		c.PaidAt = time.Now().Format("2006-01-02")
	}
	if c.PaidAt == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	t, day, err := fx.TableOn(ctx, p.set, c.PaidAt)
	if err != nil {
		return "Couldn't get the rate of " + c.PaidAt + " (" + err.Error() + "), so today's rate is used for it."
	}
	v, ok := t.Convert(c.AmountCents, cur, def)
	if !ok {
		return "No " + cur + " rate for " + c.PaidAt + " — add a manual rate in Settings → Currency."
	}
	c.ChargedCents, c.ChargedCurrency, c.ChargedNote = v, def, "ECB rate of "+day
	return ""
}

func (p *pages) costRestore(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := p.inv.RestoreCost(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/costs?ok=restored", http.StatusSeeOther)
}

func (p *pages) costDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	c, err := p.inv.GetCost(r.Context(), id)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	if err := p.inv.DeleteCost(r.Context(), actorOf(r), id); err != nil {
		p.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/costs?ok=deleted&id="+itoa(id)+"&name="+url.QueryEscape(c.Label), http.StatusSeeOther)
}
