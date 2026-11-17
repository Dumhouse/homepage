package web_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/web"
)

// Adding a subnet must keep the ones already configured; removing drops one.
func TestSubnetsAddKeepsExisting(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := postForm(t, srv, "/discovery/subnets", url.Values{"add": {"192.168.2.0/24"}}); code != 303 {
		t.Fatalf("add: %d", code)
	}
	_, body := page(t, srv, "/discovery")
	if !strings.Contains(body, "192.168.1.0/24") || !strings.Contains(body, "192.168.2.0/24") {
		t.Fatal("both subnets should be listed after adding the second")
	}
	if code, _ := postForm(t, srv, "/discovery/subnets", url.Values{"add": {"not-a-net"}}); code != 303 {
		t.Fatalf("bad add should redirect with an error: %d", code)
	}
	if code, _ := postForm(t, srv, "/discovery/subnets", url.Values{"remove": {"192.168.1.0/24"}}); code != 303 {
		t.Fatalf("remove: %d", code)
	}
	_, body = page(t, srv, "/discovery")
	if strings.Contains(body, ">192.168.1.0/24<") || !strings.Contains(body, ">192.168.2.0/24<") {
		t.Fatal("remove should drop only that subnet")
	}
	if code, _ := postForm(t, srv, "/discovery/subnets", url.Values{"list": {"10.0.0.0/24"}, "every_h": {"9999"}}); code != 303 {
		t.Fatalf("save: %d", code)
	}
}

// The date format setting changes how dates render and what the pickers read.
func TestDisplayDateFormat(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := postForm(t, srv, "/settings/display", url.Values{"date_format": {"eu"}, "clock": {"24"}}); code != 303 {
		t.Fatalf("save display: %d", code)
	}
	_, body := page(t, srv, "/settings/display")
	if !strings.Contains(body, `data-datefmt="eu"`) || !strings.Contains(body, `value="eu" selected`) {
		t.Fatal("EU format should be active and selected")
	}
	if code, _ := postForm(t, srv, "/settings/display", url.Values{"date_format": {"klingon"}}); code != 303 {
		t.Fatalf("unknown format should redirect with an error: %d", code)
	}
	_, body = page(t, srv, "/settings/display")
	if !strings.Contains(body, `data-datefmt="eu"`) {
		t.Fatal("an unknown format must not change the setting")
	}
	postForm(t, srv, "/settings/display", url.Values{"date_format": {"long"}})
}
