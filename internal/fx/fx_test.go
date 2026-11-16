package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mapSettings map[string]string

func (m mapSettings) Get(_ context.Context, k, def string) (string, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	return def, nil
}
func (m mapSettings) Set(_ context.Context, _, k, v string) error { m[k] = v; return nil }

// A Saturday payment uses Friday's fixing; manual rates fill unknown currencies.
func TestTableOnUsesLastBusinessDay(t *testing.T) {
	fri := time.Now().AddDate(0, 0, -10)
	for fri.Weekday() != time.Friday {
		fri = fri.AddDate(0, 0, -1)
	}
	sat := fri.AddDate(0, 0, 1).Format("2006-01-02")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<gesmes:Envelope xmlns:gesmes="x"><Cube>
<Cube time="` + fri.Format("2006-01-02") + `"><Cube currency="USD" rate="1.10"/></Cube>
<Cube time="` + fri.AddDate(0, 0, -1).Format("2006-01-02") + `"><Cube currency="USD" rate="1.20"/></Cube>
</Cube></gesmes:Envelope>`))
	}))
	defer srv.Close()
	old := HistEndpoint
	HistEndpoint = srv.URL + "/hist.xml"
	defer func() { HistEndpoint = old }()

	tab, day, err := TableOn(context.Background(), mapSettings{KeyManual: "UAH=0.02"}, sat)
	if err != nil {
		t.Fatal(err)
	}
	if day != fri.Format("2006-01-02") {
		t.Errorf("day = %s, want Friday %s", day, fri.Format("2006-01-02"))
	}
	if v, ok := tab.Convert(1100, "USD", "EUR"); !ok || v != 1000 {
		t.Errorf("11 USD = %d EUR cents, want 1000", v)
	}
	if v, ok := tab.Convert(10000, "UAH", "EUR"); !ok || v != 200 {
		t.Errorf("100 UAH = %d EUR cents, want 200", v)
	}

	// Manual rates win over ECB: 1 USD = 0.50 EUR means 1 USD = 50 cents.
	manual, _, err := TableOn(context.Background(), mapSettings{KeyManual: "USD=0.5"}, sat)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := manual.Convert(100, "USD", "EUR"); !ok || v != 50 {
		t.Errorf("manual USD rate should win: %d", v)
	}
}
