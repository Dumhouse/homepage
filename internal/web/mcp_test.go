package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/tokens"
	"github.com/Dumhouse/homepage/internal/web"
)

type headerRT struct{ token string }

func (h headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+h.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPOverHTTP(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := web.New(d, auth.None{})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ts := tokens.New(d)
	writeTok, err := ts.Create(context.Background(), "claude-code", "write")
	if err != nil {
		t.Fatal(err)
	}
	readTok, err := ts.Create(context.Background(), "reader", "read")
	if err != nil {
		t.Fatal(err)
	}
	proposeTok, err := ts.Create(context.Background(), "planner", "propose")
	if err != nil {
		t.Fatal(err)
	}

	connect := func(tok string) (*sdk.ClientSession, error) {
		c := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil)
		tr := &sdk.StreamableClientTransport{
			Endpoint: srv.URL + "/mcp", DisableStandaloneSSE: true,
			HTTPClient: &http.Client{Transport: headerRT{token: tok}},
		}
		return c.Connect(context.Background(), tr, nil)
	}

	if _, err := connect(""); err == nil {
		t.Error("connecting without a token should fail")
	} else if !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("expected 401 Unauthorized, got %v", err)
	}

	cs, err := connect(writeTok.Plaintext)
	if err != nil {
		t.Fatalf("connect with write token: %v", err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) < 15 {
		t.Fatalf("list tools: %v (%d)", err, len(tools.Tools))
	}
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_host", Arguments: map[string]any{"name": "over-http"},
	})
	if err != nil || res.IsError {
		t.Fatalf("create_host over HTTP: %v %v", err, res.Content)
	}
	entries, _ := storeAudit(t, d)
	if len(entries) == 0 || entries[0] != "mcp:claude-code" {
		t.Errorf("audit actor: %v", entries)
	}

	prop, err := connect(proposeTok.Plaintext)
	if err != nil {
		t.Fatalf("connect with propose token: %v", err)
	}
	defer prop.Close()
	propTools, err := prop.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hasPropose, hasCreate := false, false
	for _, tool := range propTools.Tools {
		hasPropose = hasPropose || tool.Name == "propose_changes"
		hasCreate = hasCreate || tool.Name == "create_host"
	}
	if !hasPropose || hasCreate {
		t.Errorf("propose token tools: propose=%v create_host=%v", hasPropose, hasCreate)
	}
	if res, err := prop.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "propose_changes", Arguments: map[string]any{
			"title": "Planner", "ops": []map[string]any{{"action": "create", "entity": "host", "ref": "planned"}},
		},
	}); err != nil || res.IsError {
		t.Fatalf("propose with propose token: %v %v", err, res.Content)
	}

	ro, err := connect(readTok.Plaintext)
	if err != nil {
		t.Fatalf("connect with read token: %v", err)
	}
	defer ro.Close()
	// A read token gets a server without write tools: they are neither listed nor callable.
	roTools, err := ro.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range roTools.Tools {
		if tl.Name == "create_host" {
			t.Error("read token should not see create_host")
		}
	}
	if _, err := ro.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_host", Arguments: map[string]any{"name": "nope"},
	}); err == nil {
		t.Error("read token should not be able to call create_host")
	}

	code, body := call(t, h, "GET", "/inventory.md", "", readTok.Plaintext)
	if code != 200 || !strings.Contains(string(body), "Home Services inventory") {
		t.Errorf("inventory.md: %d %s", code, body)
	}
	if code, _ := call(t, h, "GET", "/inventory.md", "", ""); code != 401 {
		t.Errorf("inventory.md without token: %d", code)
	}
}

// storeAudit returns actors of host audit rows, newest first.
func storeAudit(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	rows, err := d.Query(`SELECT actor FROM audit_log WHERE entity='host' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
