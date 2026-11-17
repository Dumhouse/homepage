package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/inventory"
	hsmcp "github.com/Dumhouse/homepage/internal/mcp"
	"github.com/Dumhouse/homepage/internal/request"
)

func connect(t *testing.T, ctx context.Context, inv *inventory.Store, chk *checks.Scheduler, cs *changesets.Store) *sdk.ClientSession {
	t.Helper()
	server := hsmcp.New(inv, chk, cs, nil)
	client := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func structured(t *testing.T, res *sdk.CallToolResult, v any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("structured content %s: %v", b, err)
	}
}

func TestMCPTools(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	chk := checks.NewScheduler(d, inv)
	cstore := changesets.New(d, inv)
	ctx := request.WithScope(request.WithActor(context.Background(), "mcp:test"), "write")
	cs := connect(t, ctx, inv, chk, cstore)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"search", "list_hosts", "get_host", "list_projects", "create_host",
		"update_host", "add_service", "create_project", "add_app_link", "add_access", "restore",
		"list_checks", "list_problems", "run_check", "check_service", "check_ssh",
		"propose_changes", "list_changesets", "get_changeset", "suggest_plugins",
		"detect", "request_connection"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}

	res := call(t, cs, "create_host", map[string]any{
		"name": "acme-prod", "location": "Proxmox .250",
		"addresses": []map[string]any{{"address": "10.0.0.5"}},
		"ssh":       []map[string]any{{"user": "root"}},
	})
	if res.IsError {
		t.Fatalf("create_host failed: %s", contentText(res))
	}
	var host inventory.Host
	structured(t, res, &host)
	if host.ID == 0 || host.Location == nil || host.Location.Name != "Proxmox .250" {
		t.Fatalf("created host: %+v", host)
	}

	res = call(t, cs, "update_host", map[string]any{"id": host.ID, "description": "patched by agent"})
	if res.IsError {
		t.Fatalf("update_host: %v", res.Content)
	}
	res = call(t, cs, "get_host", map[string]any{"id": host.ID})
	structured(t, res, &host)
	if host.Description != "patched by agent" || len(host.Addresses) != 1 || len(host.SSH) != 1 {
		t.Fatalf("patch should keep children: %+v", host)
	}

	res = call(t, cs, "add_service", map[string]any{"host_id": host.ID, "name": "web", "port": 3000, "protocol": "http"})
	if res.IsError {
		t.Fatalf("add_service: %v", res.Content)
	}
	res = call(t, cs, "suggest_free_port", map[string]any{"host_id": host.ID, "near": 3000})
	var port struct {
		Port int `json:"port"`
	}
	structured(t, res, &port)
	if port.Port != 3001 {
		t.Errorf("free port: %+v", port)
	}

	res = call(t, cs, "create_project", map[string]any{"name": "Acme", "group": "Clients"})
	var proj inventory.Project
	structured(t, res, &proj)
	if proj.Slug != "acme" || proj.Group != "Clients" {
		t.Fatalf("project: %+v", proj)
	}
	res = call(t, cs, "add_project_env", map[string]any{"project_id": proj.ID, "name": "production", "base_url": "https://acme.example.com"})
	if res.IsError {
		t.Fatalf("add_project_env: %v", res.Content)
	}
	res = call(t, cs, "add_app_link", map[string]any{"project_id": proj.ID, "label": "Admin", "path": "/admin", "kind": "admin"})
	if res.IsError {
		t.Fatalf("add_app_link: %v", res.Content)
	}
	res = call(t, cs, "add_access", map[string]any{
		"target_entity": "project", "target_id": proj.ID, "label": "Admin",
		"method": "password", "vault_item_url": "https://vault.example.com/i/1",
	})
	if res.IsError {
		t.Fatalf("add_access: %v", res.Content)
	}
	res = call(t, cs, "get_project", map[string]any{"id": proj.ID})
	structured(t, res, &proj)
	if len(proj.Envs) != 1 || len(proj.Envs[0].Links) != 1 || proj.Envs[0].Links[0].URL != "https://acme.example.com/admin" {
		t.Fatalf("resolved app link: %+v", proj.Envs)
	}

	rr, err := cs.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "homeservices://host/" + fmt.Sprint(host.ID)})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "# acme-prod") {
		t.Fatalf("host resource: %v %+v", err, rr)
	}
	rr, err = cs.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "homeservices://project/acme"})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "## Environments") {
		t.Fatalf("project resource: %v %+v", err, rr)
	}
	rr, err = cs.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "homeservices://inventory"})
	if err != nil || len(rr.Contents) == 0 || !strings.Contains(rr.Contents[0].Text, "Home Services inventory") {
		t.Fatalf("inventory resource: %v %+v", err, rr)
	}

	res = call(t, cs, "propose_changes", map[string]any{
		"title": "Agent proposal",
		"ops":   []map[string]any{{"action": "create", "entity": "host", "ref": "agent-host"}},
	})
	if res.IsError {
		t.Fatalf("propose_changes: %v", res.Content)
	}
	res = call(t, cs, "list_changesets", map[string]any{})
	if res.IsError {
		t.Fatalf("list_changesets: %v", res.Content)
	}

	res = call(t, cs, "suggest_plugins", map[string]any{})
	if res.IsError {
		t.Fatalf("suggest_plugins: %v", res.Content)
	}

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><title>Detect me</title></html>`))
	}))
	defer probe.Close()
	res = call(t, cs, "detect", map[string]any{"target": probe.URL})
	if res.IsError {
		t.Fatalf("detect: %v", res.Content)
	}
	res = call(t, cs, "request_connection", map[string]any{
		"project": "acme", "target_kind": "url", "url": "https://git.example.com", "role": "code",
	})
	if res.IsError {
		t.Fatalf("request_connection: %v", res.Content)
	}
	if res.IsError {
		t.Fatalf("suggest_plugins: %v", res.Content)
	}

	res = call(t, cs, "list_problems", map[string]any{})
	if res.IsError {
		t.Fatalf("list_problems: %v", res.Content)
	}

	res = call(t, cs, "search", map[string]any{"q": "acme sidekiq"})
	if res.IsError {
		t.Fatalf("search: %v", res.Content)
	}
	// "sidekiq" does not exist: no hits but still a valid answer
	res = call(t, cs, "search", map[string]any{"q": "acme"})
	if res.IsError {
		t.Fatalf("search: %v", res.Content)
	}

	entries, err := inv.ListAudit(ctx, "host", host.ID, 10)
	if err != nil || len(entries) == 0 || entries[0].Actor != "mcp:test" {
		t.Errorf("audit actor: %v %+v", err, entries)
	}

	if err := inv.DeleteHost(ctx, "mcp:test", host.ID); err != nil {
		t.Fatal(err)
	}
	res = call(t, cs, "restore", map[string]any{"entity": "host", "id": host.ID})
	if res.IsError {
		t.Fatalf("restore: %v", res.Content)
	}
}

func TestMCPReadOnlyTokenCannotWrite(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	chk := checks.NewScheduler(d, inv)
	ctx := request.WithScope(request.WithActor(context.Background(), "mcp:reader"), "read")
	cs := connect(t, ctx, inv, chk, changesets.New(d, inv))

	res := call(t, cs, "create_host", map[string]any{"name": "nope"})
	if !res.IsError {
		t.Fatalf("read scope should not write: %+v", res.Content)
	}
	if len(res.Content) == 0 || !strings.Contains(contentText(res), "read-only") {
		t.Errorf("error should mention read-only: %v", contentText(res))
	}
	if list, _ := inv.ListHosts(context.Background()); len(list) != 0 {
		t.Error("read-only call wrote a host")
	}
	if res := call(t, cs, "list_hosts", map[string]any{}); res.IsError {
		t.Errorf("read tool should work: %v", res.Content)
	}
}

func contentText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestMCPReadOnlyServerHidesWriteTools(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	inv := inventory.New(d)
	chk := checks.NewScheduler(d, inv)
	server := hsmcp.NewReadOnly(inv, chk, changesets.New(d, inv), nil)
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools.Tools {
		names[tl.Name] = true
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("read-only server lists write tool %s", tl.Name)
		}
	}
	if !names["search"] || !names["get_host"] || names["create_host"] || names["delete_host"] {
		t.Fatalf("unexpected tool set: %v", names)
	}
	if len(hsmcp.WriteToolNames()) < 10 {
		t.Fatalf("write tools not tracked: %v", hsmcp.WriteToolNames())
	}
}
