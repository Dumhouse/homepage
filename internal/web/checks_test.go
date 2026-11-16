package web_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/inventory"
	"github.com/dawidof/home-services/internal/web"
)

func TestChecksAPIAndUI(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	ctx := context.Background()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer healthy.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer broken.Close()
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(healthy.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	h, err := inv.CreateHost(ctx, "test", inventory.Host{Name: "local", Addresses: []inventory.Address{{Address: host}}})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "web", Port: port, Protocol: "http"})
	if err != nil {
		t.Fatal(err)
	}
	manualTarget, err := inv.AddService(ctx, "test", h.ID, inventory.Service{Name: "manual", Notes: "no port, no auto check"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checks.New(d).Reconcile(ctx, inv); err != nil {
		t.Fatal(err)
	}

	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}

	// run the auto service check from the host page
	code, loc := redirect(t, srv, "/services/"+strconv.Itoa(int(svc.ID))+"/check", nil)
	if code != 303 || !strings.Contains(loc, "/hosts/") {
		t.Fatalf("check now: %d %s", code, loc)
	}
	code, body := page(t, srv, "/hosts/"+strconv.Itoa(int(h.ID)))
	if code != 200 || !strings.Contains(body, "dot ok") {
		t.Errorf("host page should show the up check: %d", code)
	}

	// API: create a manual HTTP check against a broken server, run it, see it as a problem
	code, created := do(t, srv, "POST", "/api/v1/checks", `{
		"target_entity":"service","target_id":`+strconv.Itoa(int(manualTarget.ID))+`,
		"kind":"http","params":{"url":"`+broken.URL+`"}}`)
	if code != 201 {
		t.Fatalf("create check: %d %v", code, created)
	}
	checkID := strconv.Itoa(int(created["id"].(float64)))
	code, ran := do(t, srv, "POST", "/api/v1/checks/"+checkID+"/run", "")
	if code != 200 || ran["ok"] != false || ran["state"] != "failing" {
		t.Fatalf("run check: %d %v", code, ran)
	}
	code, list := doList(t, srv, "/api/v1/problems")
	if code != 200 || len(list) != 1 {
		t.Errorf("problems: %d %v", code, list)
	}
	if code, list := doList(t, srv, "/api/v1/checks?target_entity=service&target_id="+strconv.Itoa(int(manualTarget.ID))); code != 200 || len(list) != 1 {
		t.Errorf("list checks: %d %v", code, list)
	}
	code, body = page(t, srv, "/problems")
	if code != 200 || !strings.Contains(body, "manual") || !strings.Contains(body, "Check now") || !strings.Contains(body, "1 h") {
		t.Errorf("problems page: %d", code)
	}

	// silence alerts for the service via a maintenance window
	if code, _ := redirect(t, srv, "/maintenance", url.Values{
		"target_entity": {"service"}, "target_id": {strconv.Itoa(int(manualTarget.ID))},
		"duration_min": {"60"}, "reason": {"upgrade"},
	}); code != 303 {
		t.Fatalf("add maintenance: %d", code)
	}
	code, body = page(t, srv, "/maintenance")
	if code != 200 || !strings.Contains(body, "upgrade") || !strings.Contains(body, "active") {
		t.Errorf("maintenance page: %d", code)
	}
	var winID int64
	if err := d.QueryRow(`SELECT id FROM maintenance_windows ORDER BY id DESC LIMIT 1`).Scan(&winID); err != nil {
		t.Fatal(err)
	}
	if code, _ := redirect(t, srv, "/maintenance/"+strconv.FormatInt(winID, 10)+"/delete", nil); code != 303 {
		t.Errorf("delete maintenance: %d", code)
	}
	if _, body = page(t, srv, "/maintenance"); !strings.Contains(body, "No windows") {
		t.Error("maintenance window should be gone")
	}
	// pause the check from the problems page
	if code, _ := redirect(t, srv, "/checks/"+checkID+"/toggle", url.Values{"back": {"/problems"}}); code != 303 {
		t.Errorf("toggle check: %d", code)
	}
	code, body = page(t, srv, "/problems")
	if code != 200 || !strings.Contains(body, "All clear") {
		t.Errorf("paused check should clear problems: %d", code)
	}
	// resume so the delete check below sees the original state
	if code, _ := redirect(t, srv, "/checks/"+checkID+"/toggle", url.Values{"back": {"/problems"}}); code != 303 {
		t.Errorf("resume check: %d", code)
	}
	code, body = page(t, srv, "/")
	if code != 200 || !strings.Contains(body, "need attention") {
		t.Errorf("home should show the attention bar: %d", code)
	}
	if code, _ := do(t, srv, "DELETE", "/api/v1/checks/"+checkID, ""); code != 204 {
		t.Errorf("delete check: %d", code)
	}
	if code, list := doList(t, srv, "/api/v1/problems"); code != 200 || len(list) != 0 {
		t.Errorf("problems after delete: %d %v", code, list)
	}
}
