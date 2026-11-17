package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/discovery"
	"github.com/Dumhouse/homepage/internal/fx"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/go-chi/chi/v5"
)

// ---- settings sections (display, scanning, notifications, favicons) ----

type notifyRow struct {
	Name, Value, Hint, Href string
	On                      bool
}

type settingsPage struct {
	Layout
	Section        string
	DateFormat     string
	ClockFmt       string
	DateFormats    []struct{ Key, Label string }
	Sample         string
	HomeView       string
	TileClick      string
	Subnets        []string
	ScanPorts      string
	EveryH         string
	LastScan       string
	Notify         []notifyRow
	FaviconFetch   string
	FaviconPrivate string
	IconCount      int
	ServiceCount   int
	Currency       string
	Currencies     []string
	Rates          []rateRow
	RatesDate      string
	ManualRates    string
	AutoRates      string
}

type rateRow struct {
	Code   string
	PerOne string // value of 1 unit in the default currency
	Manual bool
}

var sectionTitles = map[string]string{
	"display": "Display", "currency": "Currency", "scanning": "Scanning", "notifications": "Notifications", "favicons": "Favicons",
}

func maskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "set"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

func (p *pages) settingsSection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sec := chi.URLParam(r, "section")
	if sec == "" {
		sec = "display"
	}
	title := sectionTitles[sec]
	l := p.shell(r, title, "settings", crumbs(Crumb{Label: "Settings", Href: "/settings"}, Crumb{Label: title}))
	l.Flash = flash(r)
	st := style()
	page := settingsPage{Layout: l, Section: sec, DateFormat: st.Date, ClockFmt: st.Clock, DateFormats: DateFormats,
		Sample:         fmtDayTime(time.Now()),
		HomeView:       settingOr(r, p.set, "home_view", "launcher"),
		TileClick:      settingOr(r, p.set, "tile_click", "site"),
		Subnets:        p.scan.Subnets(ctx),
		ScanPorts:      joinInts(p.scan.Ports(ctx)),
		FaviconFetch:   settingOr(r, p.set, "favicon_fetch", "on"),
		FaviconPrivate: settingOr(r, p.set, "favicon_private", "off"),
	}
	page.EveryH, _ = p.set.Get(ctx, discovery.KeyEveryH, "24")
	page.LastScan, _ = p.set.Get(ctx, discovery.KeyLastAt, "")
	if sec == "currency" {
		page.Currency = p.defaultCurrency(ctx)
		t := fx.Load(ctx, p.set)
		page.Currencies, page.RatesDate = currencyChoices(t, page.Currency), t.Date
		page.ManualRates, _ = p.set.Get(ctx, fx.KeyManual, "")
		page.AutoRates, _ = p.set.Get(ctx, fx.KeyAuto, "on")
		for _, c := range t.Codes() {
			if c == page.Currency {
				continue
			}
			v, _ := t.Convert(100_00, c, page.Currency)
			page.Rates = append(page.Rates, rateRow{Code: c, PerOne: strconv.FormatFloat(float64(v)/10000, 'f', 4, 64), Manual: t.Manual[c]})
		}
	}
	if sec == "favicons" {
		page.ServiceCount, page.IconCount, _ = p.inv.ServiceIconStats(ctx)
	}
	env := func(k string) string { return strings.TrimSpace(os.Getenv(k)) }
	ntfy, hook := env("HS_NTFY_URL"), env("HS_WEBHOOK_URL")
	page.Notify = []notifyRow{}
	pluginOn := false
	if p.intg != nil {
		if states, err := p.intg.List(ctx); err == nil {
			for _, st := range states {
				if st.Plugin.Kind != integrations.KindNotify {
					continue
				}
				u := st.Config["url"]
				pluginOn = pluginOn || u != ""
				page.Notify = append(page.Notify, notifyRow{Name: st.Name, On: u != "", Value: ternary(u != "", maskURL(u), "not set up"),
					Hint: "plugin — set up in Integrations", Href: "/settings/integrations?p=" + st.ID})
			}
		}
	}
	if ntfy != "" {
		page.Notify = append(page.Notify, notifyRow{Name: "ntfy (environment)", On: true, Value: maskURL(ntfy), Hint: "HS_NTFY_URL"})
	}
	if hook != "" {
		page.Notify = append(page.Notify, notifyRow{Name: "Webhook (environment)", On: true, Value: maskURL(hook), Hint: "HS_WEBHOOK_URL"})
	}
	any := pluginOn || ntfy != "" || hook != ""
	page.Notify = append(page.Notify, []notifyRow{
		{Name: "Wait before alerting", On: true, Value: ternary(env("HS_ALERTS_MIN_DOWN") != "", env("HS_ALERTS_MIN_DOWN")+" s", "120 s (default)"), Hint: "HS_ALERTS_MIN_DOWN — ignore blips shorter than this"},
		{Name: "Digest and expiry reminders", On: env("HS_DIGEST") != "off" && any, Value: ternary(env("HS_DIGEST") == "off", "off", ternary(any, "on", "needs a notifier")), Hint: "HS_DIGEST=off to disable · HS_DIGEST_DAYS for expiry warnings"},
	}...)
	p.ui.render(w, 200, "settings", page)
}

func (p *pages) saveDisplay(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx, actor := r.Context(), actorOf(r)
	if v := r.FormValue("date_format"); v != "" {
		if _, ok := dateLayouts[v]; !ok {
			backWithError(w, r, "/settings/display", fmt.Errorf("%w: unknown date format", inventory.ErrInvalid))
			return
		}
		_ = p.set.Set(ctx, actor, "date_format", v)
	}
	if v := r.FormValue("clock"); v == "24" || v == "12" {
		_ = p.set.Set(ctx, actor, "clock", v)
	}
	if v := r.FormValue("home_view"); v == "tiles" || v == "launcher" {
		_ = p.set.Set(ctx, actor, "home_view", v)
	}
	if v := r.FormValue("tile_click"); v == "details" || v == "site" {
		_ = p.set.Set(ctx, actor, "tile_click", v)
	}
	loadStyle(ctx, p.set.Get)
	http.Redirect(w, r, "/settings/display?ok=saved", http.StatusSeeOther)
}

// refetchFavicons queues an icon fetch for every web service (public domain or LAN address).
func (p *pages) refetchFavicons(w http.ResponseWriter, r *http.Request) {
	services, err := p.inv.ServicesWithDomain(r.Context())
	if err != nil {
		p.fail(w, r, err)
		return
	}
	n := 0
	for i := range services {
		p.icons.Queue(p.inv, &services[i])
		n++
	}
	http.Redirect(w, r, "/settings/favicons?ok=icons-queued", http.StatusSeeOther)
}

// shortErr trims a Go HTTP error to its cause for tooltips:
// `Get "http://x": dial tcp: lookup x: no such host` -> `lookup x: no such host`.
func shortErr(e string) string {
	if i := strings.LastIndex(e, "\": "); i >= 0 {
		e = e[i+3:]
	}
	e = strings.TrimPrefix(e, "dial tcp: ")
	if len(e) > 70 {
		e = e[:67] + "…"
	}
	return e
}
