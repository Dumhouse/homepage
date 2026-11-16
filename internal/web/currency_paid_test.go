package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/fx"
)

// A purchase in another currency keeps the value it had on the day it was
// paid; a later change in today's rate doesn't move it. What the bank
// charged wins over any rate.
func TestCostKeepsRateOfPaymentDay(t *testing.T) {
	today := "2.0"
	daily := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="x"><Cube><Cube time="2026-11-13"><Cube currency="USD" rate="` + today + `"/></Cube></Cube></gesmes:Envelope>`))
	}))
	defer daily.Close()
	hist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="x"><Cube><Cube time="2026-11-06"><Cube currency="USD" rate="1.25"/></Cube></Cube></gesmes:Envelope>`))
	}))
	defer hist.Close()
	oldD, oldH := fx.Endpoint, fx.HistEndpoint
	fx.Endpoint, fx.HistEndpoint = daily.URL, hist.URL+"/hist90.xml"
	defer func() { fx.Endpoint, fx.HistEndpoint = oldD, oldH }()

	srv, inv := newTestServer(t)
	if code, loc := redirect(t, srv, "/settings/currency/refresh", nil); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("refresh: %s", loc)
	}
	// 100 USD paid on Saturday 2026-11-07 → Friday's rate 1.25 → 80 EUR.
	if code, loc := redirect(t, srv, "/costs", url.Values{"label": {"NUC"}, "amount": {"100"}, "currency": {"USD"},
		"cycle": {"one_time"}, "paid_at": {"2026-11-07"}}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("add: %s", loc)
	}
	// 50 USD, bank took 47.10 EUR.
	if code, loc := redirect(t, srv, "/costs", url.Values{"label": {"Disk"}, "amount": {"50"}, "currency": {"USD"},
		"cycle": {"one_time"}, "charged": {"47,10"}}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("add: %s", loc)
	}
	costs, _ := inv.ListCosts(context.Background())
	for _, c := range costs {
		switch c.Label {
		case "NUC":
			if c.ChargedCents != 8000 || c.ChargedNote != "ECB rate of 2026-11-06" {
				t.Errorf("NUC locked at %d (%s), want 8000 at Friday's rate", c.ChargedCents, c.ChargedNote)
			}
		case "Disk":
			if c.ChargedCents != 4710 || c.ChargedNote != "as charged" {
				t.Errorf("Disk: %d %s", c.ChargedCents, c.ChargedNote)
			}
		}
	}
	// Today's rate moves; the purchases don't.
	today = "4.0"
	redirect(t, srv, "/settings/currency/refresh", nil)
	_, body := page(t, srv, "/costs")
	if !strings.Contains(body, "80.00 EUR") || !strings.Contains(body, "47.10 EUR") || !strings.Contains(body, "ECB rate of 2026-11-06") {
		t.Fatal("locked values should show regardless of today's rate")
	}
}

func TestDomainPaidValue(t *testing.T) {
	srv, inv := newTestServer(t)
	ctx := context.Background()
	if code, loc := redirect(t, srv, "/domains", url.Values{
		"name": {"paid.example"}, "expires_at": {"2026-12-01"},
		"price": {"100"}, "currency": {"USD"}, "charged": {"90"},
	}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("save domain: %d %s", code, loc)
	}
	list, _ := inv.ListDomains(ctx)
	if len(list) != 1 || list[0].ChargedCents != 9000 || list[0].ChargedNote != "as charged" {
		t.Fatalf("domain lock: %+v", list)
	}
	_, body := page(t, srv, "/costs")
	if !strings.Contains(body, "90.00 EUR") || !strings.Contains(body, "100.00 USD") {
		t.Error("costs should use the locked domain value and keep the original price")
	}
}
