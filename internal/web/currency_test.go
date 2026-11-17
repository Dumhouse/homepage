package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/fx"
	"github.com/Dumhouse/homepage/internal/inventory"
)

// Costs in other currencies are converted into the default currency for
// totals; a currency with no rate is left out and named.
func TestCostsConvertToDefaultCurrency(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
<Cube><Cube time="2026-11-13"><Cube currency="USD" rate="2.0"/><Cube currency="PLN" rate="4.0"/></Cube></Cube></gesmes:Envelope>`))
	}))
	defer fake.Close()
	old := fx.Endpoint
	fx.Endpoint = fake.URL
	defer func() { fx.Endpoint = old }()

	srv, inv := newTestServer(t)
	ctx := context.Background()
	for _, c := range []inventory.Cost{
		{Label: "vps", AmountCents: 1000, Currency: "EUR", Cycle: "monthly"},
		{Label: "s3", AmountCents: 1000, Currency: "USD", Cycle: "monthly"}, // = 5 EUR
		{Label: "odd", AmountCents: 1000, Currency: "UAH", Cycle: "monthly"},
	} {
		if _, err := inv.CreateCost(ctx, "test", c); err != nil {
			t.Fatal(err)
		}
	}
	if code, loc := redirect(t, srv, "/settings/currency/refresh", nil); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("refresh: %d %s", code, loc)
	}
	_, body := page(t, srv, "/costs")
	if !strings.Contains(body, "15.00 EUR") || !strings.Contains(body, "No rate for UAH") {
		t.Fatal("total should be 10 EUR + 5 EUR, UAH named as missing")
	}
	// manual rate for UAH, default currency PLN: (10 + 5 + 10*0.025) EUR * 4
	if code, loc := redirect(t, srv, "/settings/currency", url.Values{"currency": {"PLN"}, "manual": {"UAH=0.025"}}); code != 303 || strings.Contains(loc, "ok=err") {
		t.Fatalf("save: %d %s", code, loc)
	}
	_, body = page(t, srv, "/costs")
	if !strings.Contains(body, "totals in PLN") || strings.Contains(body, "No rate for") || !strings.Contains(body, "61.00 PLN") {
		t.Fatal("totals should be in PLN with UAH converted by the manual rate")
	}
	if code, loc := redirect(t, srv, "/settings/currency", url.Values{"manual": {"UAH:abc"}}); code != 303 || !strings.Contains(loc, "ok=err") {
		t.Fatalf("bad manual rate should be refused: %s", loc)
	}
}
