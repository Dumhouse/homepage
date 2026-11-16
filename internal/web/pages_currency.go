package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dawidof/home-services/internal/fx"
	"github.com/dawidof/home-services/internal/inventory"
)

// ---- currency ----

// defaultCurrency is the currency totals are shown in (Settings → Currency).
func (p *pages) defaultCurrency(ctx context.Context) string {
	v, _ := p.set.Get(ctx, fx.KeyDefault, "EUR")
	if len(v) != 3 {
		return "EUR"
	}
	return strings.ToUpper(v)
}

// currencyChoices is the default currency first, then the common ones, then
// every other currency with a rate.
func currencyChoices(t fx.Table, def string) []string {
	out := []string{def}
	for _, c := range append([]string{"EUR", "USD", "GBP", "PLN", "UAH", "CHF", "CZK", "SEK"}, t.Codes()...) {
		if !contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func (p *pages) saveCurrency(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	ctx, actor := r.Context(), actorOf(r)
	if c := strings.ToUpper(strings.TrimSpace(r.FormValue("currency"))); c != "" {
		if len(c) != 3 {
			backWithError(w, r, "/settings/currency", fmt.Errorf("%w: currency must be a 3-letter code like EUR", inventory.ErrInvalid))
			return
		}
		_ = p.set.Set(ctx, actor, fx.KeyDefault, c)
	}
	if r.Form.Has("manual") {
		raw := r.FormValue("manual")
		if _, err := fx.ParseManual(raw); err != nil {
			backWithError(w, r, "/settings/currency", fmt.Errorf("%w: %v", inventory.ErrInvalid, err))
			return
		}
		_ = p.set.Set(ctx, actor, fx.KeyManual, strings.TrimSpace(raw))
	}
	if v := r.FormValue("auto"); v == "on" || v == "off" {
		_ = p.set.Set(ctx, actor, fx.KeyAuto, v)
	}
	http.Redirect(w, r, "/settings/currency?ok=saved", http.StatusSeeOther)
}

func (p *pages) refreshRates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if _, err := fx.Refresh(ctx, p.set, actorOf(r)); err != nil {
		backWithError(w, r, "/settings/currency", err)
		return
	}
	http.Redirect(w, r, "/settings/currency?ok=rates", http.StatusSeeOther)
}
