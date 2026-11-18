package mcp_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	hsmcp "github.com/Dumhouse/homepage/internal/mcp"
	"github.com/Dumhouse/homepage/internal/request"
)

// dummyArgs fills every required property of a tool's input schema with a
// harmless value of the right type, so the call reaches the handler.
func dummyArgs(t *testing.T, schema any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(schema)
	var s struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, name := range s.Required {
		typ := ""
		switch v := s.Properties[name].Type.(type) {
		case string:
			typ = v
		case []any:
			for _, x := range v {
				if x != "null" {
					typ, _ = x.(string)
				}
			}
		}
		switch typ {
		case "integer", "number":
			out[name] = 1
		case "boolean":
			out[name] = false
		case "array":
			out[name] = []any{}
		case "object":
			out[name] = map[string]any{}
		default:
			out[name] = "x"
		}
	}
	return out
}

type scopeEnv struct {
	inv   *inventory.Store
	chk   *checks.Scheduler
	cs    *changesets.Store
	tools map[string]*sdk.Tool
}

func newScopeEnv(t *testing.T) *scopeEnv {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	inv := inventory.New(d)
	e := &scopeEnv{inv: inv, chk: checks.NewScheduler(d, inv), cs: changesets.New(d, inv), tools: map[string]*sdk.Tool{}}
	// A host and a project so tools that take an id have something to hit.
	if _, err := inv.CreateHost(context.Background(), "t", inventory.Host{Name: "x", Kind: "vm"}); err != nil {
		t.Fatal(err)
	}
	session := connect(t, context.Background(), e.inv, e.chk, e.cs)
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range list.Tools {
		e.tools[tl.Name] = tl
	}
	return e
}

func (e *scopeEnv) audits(t *testing.T) int {
	t.Helper()
	list, err := e.inv.ListAudit(context.Background(), "", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

// Every write tool refuses a read token, and every direct write tool refuses
// a propose token — and nothing is written either way. Adding a write tool
// that forgets its scope check fails here.
func TestEveryWriteToolEnforcesScope(t *testing.T) {
	e := newScopeEnv(t)
	writes := hsmcp.WriteToolNames()
	if len(writes) < 20 {
		t.Fatalf("expected the full write tool set, got %v", writes)
	}
	for _, scope := range []string{"read", "propose"} {
		ctx := request.WithScope(request.WithActor(context.Background(), "mcp:"+scope), scope)
		session := connect(t, ctx, e.inv, e.chk, e.cs)
		for _, name := range writes {
			tl, ok := e.tools[name]
			if !ok {
				t.Fatalf("write tool %s is not listed", name)
			}
			if tl.Annotations == nil || tl.Annotations.ReadOnlyHint {
				t.Errorf("%s must not claim to be read-only", name)
			}
			allowed := scope == "propose" && hsmcp.ProposeOnlyTools[name]
			before := e.audits(t)
			res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: dummyArgs(t, tl.InputSchema)})
			if err != nil {
				t.Fatalf("%s/%s: %v", scope, name, err)
			}
			msg := contentText(res)
			if allowed {
				if res.IsError && (strings.Contains(msg, "read-only") || strings.Contains(msg, "cannot propose")) {
					t.Errorf("propose token should be allowed to call %s: %s", name, msg)
				}
				continue
			}
			if !res.IsError {
				t.Errorf("%s token called %s successfully", scope, name)
				continue
			}
			if !strings.Contains(msg, "read-only") && !strings.Contains(msg, "cannot propose") {
				t.Errorf("%s/%s failed for the wrong reason (scope must be checked first): %s", scope, name, msg)
			}
			if after := e.audits(t); after != before {
				t.Errorf("%s/%s wrote %d audit rows despite refusing", scope, name, after-before)
			}
		}
	}
}

// The propose server hides direct writes; the read-only server hides all.
func TestProposeServerToolSet(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	inv := inventory.New(d)
	server := hsmcp.NewPropose(inv, checks.NewScheduler(d, inv), changesets.New(d, inv), nil)
	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range list.Tools {
		names[tl.Name] = true
	}
	for _, n := range hsmcp.DirectWriteTools() {
		if names[n] {
			t.Errorf("propose server lists direct write tool %s", n)
		}
	}
	for n := range hsmcp.ProposeOnlyTools {
		if !names[n] {
			t.Errorf("propose server lacks %s", n)
		}
	}
	if !names["search"] || !names["used_by"] {
		t.Fatal("read tools missing")
	}
}

// Integration secrets never reach an agent, whatever it reads.
func TestNoSecretsInAgentOutput(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	inv := inventory.New(d)
	const secret = "tok-SUPERSECRET-123"
	im := integrations.NewManager(d, integrations.NewMemorySecrets())
	im.Register(integrations.Proxmox())
	if err := im.Save(ctx, "t", "proxmox", map[string]string{"api_url": "https://pve.example:8006", "token_id": "root@pam!hs", "token_secret": secret}); err != nil {
		t.Fatal(err)
	}
	h, _ := inv.CreateHost(ctx, "t", inventory.Host{Name: "pve", Kind: "bare", Addresses: []inventory.Address{{Address: "192.0.2.10"}}})
	if _, err := inv.CreateAccess(ctx, "t", inventory.Access{TargetEntity: "host", TargetID: h.ID, Label: "web", Method: "password",
		Account: "root", VaultItemURL: "https://vault.example/#/vault?itemId=abc"}); err != nil {
		t.Fatal(err)
	}
	server := hsmcp.New(inv, checks.NewScheduler(d, inv), changesets.New(d, inv), im)
	rctx := request.WithScope(request.WithActor(ctx, "mcp:reader"), "read")
	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(rctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var all strings.Builder
	list, _ := session.ListTools(ctx, nil)
	for _, tl := range list.Tools {
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			continue
		}
		res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tl.Name, Arguments: dummyArgs(t, tl.InputSchema)})
		if err != nil {
			continue
		}
		b, _ := json.Marshal(res)
		all.Write(b)
	}
	for _, uri := range []string{"homeservices://inventory", "homeservices://host/" + itoa(h.ID)} {
		if res, err := session.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri}); err == nil {
			b, _ := json.Marshal(res)
			all.Write(b)
		}
	}
	out := all.String()
	if strings.Contains(out, secret) {
		t.Fatal("an integration secret reached the agent")
	}
	if !strings.Contains(out, "vault.example") {
		t.Fatal("sanity: the agent should see the vault link (that is the point)")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
