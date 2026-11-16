package web_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/auth"
	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/web"
)

func TestChangesetsAPIAndUI(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(d, auth.None{})
	if err != nil {
		t.Fatal(err)
	}
	code, created := do(t, srv, "POST", "/api/v1/changesets", `{
		"title":"Proxmox import","source":"plugin:proxmox",
		"ops":[{"action":"create","entity":"host","ref":"api-host","data":{"kind":"lxc"}}]}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, created)
	}
	id := strconv.Itoa(int(created["id"].(float64)))
	code, body := page(t, srv, "/changesets")
	if code != 200 || !strings.Contains(body, "Proxmox import") || !strings.Contains(body, "api-host") {
		t.Fatalf("changes page: %d", code)
	}
	if code, _ := postForm(t, srv, "/changesets/"+id+"/apply", nil); code != 303 {
		t.Fatalf("apply: %d", code)
	}
	if _, body = page(t, srv, "/hosts"); !strings.Contains(body, "api-host") {
		t.Error("applied host missing")
	}
	if code, list := doList(t, srv, "/api/v1/changesets?status=applied"); code != 200 || len(list) != 1 {
		t.Errorf("applied list: %d %v", code, list)
	}
	if code, _ := postForm(t, srv, "/changesets/"+id+"/undo", nil); code != 303 {
		t.Fatalf("undo: %d", code)
	}
	if _, body = page(t, srv, "/hosts"); strings.Contains(body, "api-host") {
		t.Error("undone host should be hidden")
	}

	code, rejected := do(t, srv, "POST", "/api/v1/changesets", `{"title":"Nope","ops":[{"action":"create","entity":"host","ref":"x"}]}`)
	if code != 201 {
		t.Fatalf("create reject set: %d", code)
	}
	rid := strconv.Itoa(int(rejected["id"].(float64)))
	if code, _ := postForm(t, srv, "/changesets/"+rid+"/reject", nil); code != 303 {
		t.Errorf("reject: %d", code)
	}
	code, body = page(t, srv, "/changesets")
	if code != 200 || !strings.Contains(body, "rejected") {
		t.Errorf("rejected status missing: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/changesets/"+rid+"/apply", ""); code != 422 {
		t.Errorf("rejected apply should be 422, got %d", code)
	}

	_ = httptest.NewRequest(http.MethodGet, "/", nil)
}
