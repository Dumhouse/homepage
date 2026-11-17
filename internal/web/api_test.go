package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/web"
)

func do(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m
}

func doList(t *testing.T, h http.Handler, path string) (int, []any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var v []any
	json.Unmarshal(rec.Body.Bytes(), &v)
	return rec.Code, v
}

func TestAPIHostCRUD(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})

	code, m := do(t, h, "POST", "/api/v1/hosts", `{"name":"acme","addresses":[{"address":"192.168.1.240"}],"ssh":[{"user":"deploy","port":2222}]}`)
	if code != 201 || m["name"] != "acme" {
		t.Fatalf("create: %d %v", code, m)
	}
	id := int(m["id"].(float64))
	p := "/api/v1/hosts/" + strconv.Itoa(id)

	if code, _ = do(t, h, "POST", p+"/services", `{"name":"web","port":3000,"protocol":"http"}`); code != 201 {
		t.Fatalf("add service: %d", code)
	}
	if code, m = do(t, h, "GET", p, ""); code != 200 || len(m["services"].([]any)) != 1 {
		t.Fatalf("get: %d %v", code, m)
	}
	if code, _ = do(t, h, "POST", "/api/v1/hosts", `{"name":""}`); code != 422 {
		t.Errorf("invalid should be 422, got %d", code)
	}
	if code, _ = do(t, h, "POST", "/api/v1/hosts", `{"name":"x","bogus":1}`); code != 400 {
		t.Errorf("unknown field should be 400, got %d", code)
	}
	if code, _ = do(t, h, "DELETE", p, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ = do(t, h, "GET", p, ""); code != 404 {
		t.Errorf("deleted should be 404, got %d", code)
	}
	if code, _ = do(t, h, "POST", p+"/restore", ""); code != 204 {
		t.Errorf("restore: %d", code)
	}
	if code, _ = do(t, h, "GET", p, ""); code != 200 {
		t.Errorf("restored should be 200, got %d", code)
	}
}

func TestAPIProjectDetailsAndLocations(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})

	code, loc := do(t, h, "POST", "/api/v1/locations", `{"name":"Proxmox .250","kind":"proxmox"}`)
	if code != 201 || loc["name"] != "Proxmox .250" {
		t.Fatalf("create location: %d %v", code, loc)
	}
	locID := strconv.Itoa(int(loc["id"].(float64)))

	code, host := do(t, h, "POST", "/api/v1/hosts", `{"name":"acme","tags":["backend"],"location_id":`+locID+`}`)
	if code != 201 || host["location"].(map[string]any)["name"] != "Proxmox .250" {
		t.Fatalf("create host: %d %v", code, host)
	}
	if code, list := doList(t, h, "/api/v1/tags"); code != 200 || len(list) != 1 || list[0].(map[string]any)["name"] != "backend" {
		t.Errorf("tags: %d %v", code, list)
	}
	if code, list := doList(t, h, "/api/v1/locations"); code != 200 || list[0].(map[string]any)["host_count"].(float64) != 1 {
		t.Errorf("locations: %d %v", code, list)
	}

	code, proj := do(t, h, "POST", "/api/v1/projects", `{"name":"Acme","group":"Clients"}`)
	if code != 201 || proj["group"] != "Clients" {
		t.Fatalf("create project: %d %v", code, proj)
	}
	pid := strconv.Itoa(int(proj["id"].(float64)))
	if code, list := doList(t, h, "/api/v1/groups"); code != 200 || len(list) != 1 {
		t.Errorf("groups: %d %v", code, list)
	}

	if code, _ := do(t, h, "POST", "/api/v1/projects/"+pid+"/links", `{"kind":"repo","url":"https://git.example.com/acme"}`); code != 201 {
		t.Fatalf("add link: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/v1/projects/"+pid+"/envs", `{"name":"production","base_url":"https://acme.example.com"}`); code != 201 {
		t.Fatalf("add env: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/v1/projects/"+pid+"/app-links", `{"label":"Admin","path":"/admin","kind":"admin"}`); code != 201 {
		t.Fatalf("add app link: %d", code)
	}
	code, acc := do(t, h, "POST", "/api/v1/access", `{"target_entity":"project","target_id":`+pid+`,"label":"Admin","method":"password","vault_item_url":"https://vault.example.com/i/1"}`)
	if code != 201 || acc["label"] != "Admin" {
		t.Fatalf("create access: %d %v", code, acc)
	}
	if code, _ := do(t, h, "POST", "/api/v1/projects/"+pid+"/hosts", `{"host_id":`+strconv.Itoa(int(host["id"].(float64)))+`,"env":"production"}`); code != 204 {
		t.Fatalf("link host: %d", code)
	}

	code, got := do(t, h, "GET", "/api/v1/projects/"+pid, "")
	if code != 200 {
		t.Fatalf("get project: %d %v", code, got)
	}
	envs := got["envs"].([]any)
	if len(envs) != 1 || len(got["links"].([]any)) != 1 || len(got["hosts"].([]any)) != 1 || len(got["access"].([]any)) != 1 {
		t.Fatalf("project children: %v", got)
	}
	link := envs[0].(map[string]any)["links"].([]any)[0].(map[string]any)
	if link["url"] != "https://acme.example.com/admin" {
		t.Errorf("resolved url: %v", link["url"])
	}

	// deleting a location in use is a 422, not a 500
	if code, m := do(t, h, "DELETE", "/api/v1/locations/"+locID, ""); code != 422 || !strings.Contains(m["error"].(string), "move them first") {
		t.Errorf("delete in-use location: %d %v", code, m)
	}
	// strict JSON still applies to the new endpoints
	if code, _ := do(t, h, "POST", "/api/v1/locations", `{"name":"x","bogus":1}`); code != 400 {
		t.Errorf("unknown field should be 400, got %d", code)
	}
}

func TestAPISearchPortAndAudit(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})

	code, host := do(t, h, "POST", "/api/v1/hosts", `{"name":"acme","addresses":[{"address":"10.0.0.5"}],"services":[{"name":"x","port":1}]}`)
	if code != 201 {
		t.Fatalf("create host: %d %v", code, host)
	}
	hid := strconv.Itoa(int(host["id"].(float64)))
	if code, _ = do(t, h, "POST", "/api/v1/hosts/"+hid+"/services", `{"name":"web","port":3000,"protocol":"http"}`); code != 201 {
		t.Fatalf("add service: %d", code)
	}
	if code, m := do(t, h, "GET", "/api/v1/hosts/"+hid+"/free-port?near=3000", ""); code != 200 || m["port"].(float64) != 3001 {
		t.Errorf("free port: %d %v", code, m)
	}
	if code, list := doList(t, h, "/api/v1/search?q=acme"); code != 200 || len(list) == 0 || list[0].(map[string]any)["kind"] != "host" {
		t.Errorf("search: %d %v", code, list)
	}
	if code, list := doList(t, h, "/api/v1/audit?entity=host&id="+hid); code != 200 || len(list) != 1 {
		t.Errorf("audit: %d %v", code, list)
	}
	if code, list := doList(t, h, "/api/v1/audit?limit=1"); code != 200 || len(list) != 1 {
		t.Errorf("audit limit: %d %v", code, list)
	}
	// a second host on the same address gets warnings, not a 422
	code, second := do(t, h, "POST", "/api/v1/hosts", `{"name":"other","addresses":[{"address":"10.0.0.5"}]}`)
	if code != 201 {
		t.Fatalf("second host: %d %v", code, second)
	}
	w, _ := second["warnings"].([]any)
	if len(w) != 1 || !strings.Contains(w[0].(string), "also used by acme") {
		t.Errorf("warnings: %v", second["warnings"])
	}
}

func TestAPIPatchHost(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})

	code, host := do(t, h, "POST", "/api/v1/hosts", `{"name":"acme","description":"keep","addresses":[{"address":"10.0.0.5"}],"ssh":[{"user":"root"}]}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, host)
	}
	hid := strconv.Itoa(int(host["id"].(float64)))

	code, patched := do(t, h, "PATCH", "/api/v1/hosts/"+hid, `{"name":"acme-prod"}`)
	if code != 200 || patched["name"] != "acme-prod" || patched["description"] != "keep" {
		t.Fatalf("patch: %d %v", code, patched)
	}
	if len(patched["addresses"].([]any)) != 1 || len(patched["ssh"].([]any)) != 1 {
		t.Errorf("children must survive a partial patch: %v", patched)
	}
	code, patched = do(t, h, "PATCH", "/api/v1/hosts/"+hid, `{"addresses":[{"kind":"tailscale","address":"100.64.0.1"}]}`)
	if code != 200 || len(patched["addresses"].([]any)) != 2 {
		t.Errorf("add address: %d %v", code, patched)
	}
	if code, _ := do(t, h, "PATCH", "/api/v1/hosts/"+hid, `{"kind":"toaster"}`); code != 422 {
		t.Errorf("bad patch should be 422, got %d", code)
	}
	if code, _ := do(t, h, "PATCH", "/api/v1/hosts/999", `{"name":"x"}`); code != 404 {
		t.Errorf("unknown host should be 404, got %d", code)
	}
}

func TestAPIExportImport(t *testing.T) {
	d1, _ := db.Open(t.TempDir() + "/a.db")
	h1, _ := web.New(d1, auth.None{})
	if code, _ := do(t, h1, "POST", "/api/v1/hosts", `{"name":"acme","addresses":[{"address":"10.0.0.5"}]}`); code != 201 {
		t.Fatal("seed host")
	}
	code, proj := do(t, h1, "POST", "/api/v1/projects", `{"name":"Acme"}`)
	if code != 201 {
		t.Fatal("seed project")
	}
	pid := strconv.Itoa(int(proj["id"].(float64)))
	if code, _ := do(t, h1, "POST", "/api/v1/projects/"+pid+"/envs", `{"name":"production","base_url":"https://acme.example.com"}`); code != 201 {
		t.Fatal("seed env")
	}
	if code, _ := do(t, h1, "POST", "/api/v1/projects/"+pid+"/app-links", `{"label":"Admin","path":"/admin"}`); code != 201 {
		t.Fatal("seed app link")
	}

	req := httptest.NewRequest("GET", "/api/v1/export", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h1.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"acme"`) {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	doc := rec.Body.String()

	d2, _ := db.Open(t.TempDir() + "/b.db")
	h2, _ := web.New(d2, auth.None{})
	code, summary := do(t, h2, "POST", "/api/v1/import", doc)
	if code != 200 || summary["dry_run"] != true {
		t.Fatalf("dry run: %d %v", code, summary)
	}
	if n := summary["create"].(map[string]any)["hosts"].(float64); n != 1 {
		t.Errorf("dry-run should plan one host: %v", summary)
	}
	code, summary = do(t, h2, "POST", "/api/v1/import?apply=1&mode=replace", doc)
	if code != 200 || summary["dry_run"] != false {
		t.Fatalf("apply: %d %v", code, summary)
	}
	code, list := doList(t, h2, "/api/v1/projects")
	if code != 200 || len(list) != 1 {
		t.Fatalf("imported projects: %d %v", code, list)
	}
	slug := list[0].(map[string]any)["slug"].(string)
	code, got := do(t, h2, "GET", "/api/v1/projects/1", "")
	if code != 200 || got["slug"] != slug {
		t.Fatalf("imported project detail: %d %v", code, got)
	}
	if links := got["envs"].([]any)[0].(map[string]any)["links"].([]any); len(links) != 1 {
		t.Errorf("env links after import: %v", links)
	}
}

func TestAPIDomains(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})

	code, dom := do(t, h, "POST", "/api/v1/domains", `{"name":"example.com","registrar":"OVH","expires_at":"2027-01-01","price_cents":1299,"currency":"EUR","auto_renew":true}`)
	if code != 201 || dom["name"] != "example.com" || dom["price_cents"].(float64) != 1299 {
		t.Fatalf("create domain: %d %v", code, dom)
	}
	id := strconv.Itoa(int(dom["id"].(float64)))
	if code, _ := do(t, h, "POST", "/api/v1/domains/"+id+"/records", `{"name":"www","type":"A","content":"10.0.0.5"}`); code != 201 {
		t.Fatalf("add record: %d", code)
	}
	code, got := do(t, h, "GET", "/api/v1/domains/"+id, "")
	if code != 200 || len(got["records"].([]any)) != 1 {
		t.Fatalf("get domain: %d %v", code, got)
	}
	if code, _ := do(t, h, "PUT", "/api/v1/domains/"+id, `{"name":"example.com","auto_renew":false}`); code != 200 {
		t.Fatalf("update domain: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/v1/domains", `{"name":"nope"}`); code != 422 {
		t.Errorf("invalid domain should be 422, got %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/api/v1/domains/"+id, ""); code != 204 {
		t.Errorf("delete domain: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/v1/domains/"+id+"/restore", ""); code != 204 {
		t.Errorf("restore domain: %d", code)
	}
	if code, _ := do(t, h, "GET", "/api/v1/domains", ""); code != 200 {
		t.Errorf("list domains: %d", code)
	}
}
