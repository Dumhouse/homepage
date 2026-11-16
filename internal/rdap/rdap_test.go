package rdap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLookupParsesRegistrarAndExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/example.com" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/rdap+json")
		_, _ = w.Write([]byte(`{
			"status": ["client transfer prohibited"],
			"events": [
				{"eventAction": "registration", "eventDate": "2020-01-02T00:00:00Z"},
				{"eventAction": "expiration", "eventDate": "2027-01-02T10:30:00Z"}
			],
			"entities": [
				{"roles": ["registrant"], "vcardArray": ["vcard", [["fn", {}, "text", "Someone"]]]},
				{"roles": ["registrar"], "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "Demo Registrar"]]]}
			]
		}`))
	}))
	defer srv.Close()
	old := Endpoint
	Endpoint = srv.URL + "/"
	defer func() { Endpoint = old }()

	info, err := Lookup(context.Background(), " Example.COM. ")
	if err != nil {
		t.Fatal(err)
	}
	if info.Registrar != "Demo Registrar" || info.ExpiresAt != "2027-01-02" {
		t.Fatalf("info: %+v", info)
	}
	if len(info.Status) != 1 || info.Status[0] != "client transfer prohibited" {
		t.Errorf("status: %+v", info.Status)
	}
}

func TestLookupErrors(t *testing.T) {
	if _, err := Lookup(context.Background(), "nodot"); err == nil {
		t.Error("a name without a dot should be refused")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	old := Endpoint
	Endpoint = srv.URL + "/"
	defer func() { Endpoint = old }()
	_, err := Lookup(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "no record") {
		t.Fatalf("404 should be explained, got %v", err)
	}
}

func TestVCardFN(t *testing.T) {
	if got := vcardFN([]byte(`[["version",{},"text","4.0"],["fn",{},"text","Registrar Ltd."]]`)); got != "Registrar Ltd." {
		t.Errorf("fn = %q", got)
	}
	if got := vcardFN([]byte(`not json`)); got != "" {
		t.Errorf("invalid vcard = %q", got)
	}
}
