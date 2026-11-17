package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/tokens"
	"github.com/Dumhouse/homepage/internal/web"
)

func call(t *testing.T, h http.Handler, method, path, body, token string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func TestTokenAuth(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})
	ctx := context.Background()
	ts := tokens.New(d)
	reader, err := ts.Create(ctx, "reader", "read")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := ts.Create(ctx, "writer", "write")
	if err != nil {
		t.Fatal(err)
	}
	planner, err := ts.Create(ctx, "planner", "propose")
	if err != nil {
		t.Fatal(err)
	}

	if code, _ := call(t, h, "GET", "/api/v1/hosts", "", ""); code != 401 {
		t.Errorf("no token should be 401, got %d", code)
	}
	if code, _ := call(t, h, "GET", "/api/v1/hosts", "", "hs_000000000000_00000000000000000000000000000000"); code != 401 {
		t.Errorf("bad token should be 401, got %d", code)
	}
	if code, _ := call(t, h, "GET", "/api/v1/hosts", "", reader.Plaintext); code != 200 {
		t.Errorf("read token GET: %d", code)
	}
	if code, _ := call(t, h, "POST", "/api/v1/hosts", `{"name":"x"}`, reader.Plaintext); code != 403 {
		t.Errorf("read token POST should be 403, got %d", code)
	}
	if code, _ := call(t, h, "POST", "/api/v1/hosts", `{"name":"x"}`, writer.Plaintext); code != 201 {
		t.Errorf("write token POST: %d", code)
	}
	if code, _ := call(t, h, "GET", "/api/v1/hosts", "", planner.Plaintext); code != 200 {
		t.Errorf("propose token GET: %d", code)
	}
	if code, _ := call(t, h, "POST", "/api/v1/hosts", `{"name":"x"}`, planner.Plaintext); code != 403 {
		t.Errorf("propose token direct write should be 403, got %d", code)
	}
	if code, _ := call(t, h, "POST", "/api/v1/changesets",
		`{"title":"Planner","ops":[{"action":"create","entity":"host","ref":"planned"}]}`, planner.Plaintext); code != 201 {
		t.Errorf("propose token should queue a changeset, got %d", code)
	}

	code, body := call(t, h, "GET", "/api/v1/audit?entity=host", "", writer.Plaintext)
	if code != 200 {
		t.Fatalf("audit: %d %s", code, body)
	}
	var entries []map[string]any
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0]["actor"] != "api:writer" {
		t.Errorf("audit actor should be api:writer: %v", entries)
	}

	// health and the prototype stay open; /mcp needs a token and is now the real server
	if code, _ := call(t, h, "GET", "/healthz", "", ""); code != 200 {
		t.Errorf("healthz: %d", code)
	}
	if code, _ := call(t, h, "POST", "/mcp", "", ""); code != 401 {
		t.Errorf("mcp without token: %d", code)
	}
	if code, _ := call(t, h, "POST", "/mcp", "", writer.Plaintext); code == 401 || code == 403 {
		t.Errorf("mcp with a write token should reach the server, got %d", code)
	}
}
