// Package mcp exposes the inventory to AI agents over the Model Context Protocol.
// It is a thin adapter: every tool goes through inventory.Store, so validation,
// soft delete and the audit log behave exactly like the UI.
package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/changesets"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/request"
)

const instructions = `Home Services is a self-hosted inventory of a home lab: hosts (VMs, LXCs, bare metal),
their addresses and SSH logins, services on them, projects, project environments/app links, and
"access" entries that describe how a human logs in.

Rules:
- Passwords and secrets are never stored here. An access entry has a method, an account name and
  optionally a Vaultwarden item URL. Never ask the user for a password; point them at Vaultwarden.
- Prefer get_host / get_project before writing. Use update_host (patch) so omitted fields are kept.
- Deleting is a soft delete; restore with the restore tool.
- Every write is audited with this token's name as the actor.
- Think in the user's terms: a project is an app, an environment is production/staging, an app link
  is a path like /admin or /sidekiq, a connected service is something the project depends on.`

// New builds the MCP server with all read and write tools.
func New(inv *inventory.Store, chk *checks.Scheduler, cs *changesets.Store, intg *integrations.Manager) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "homeservices", Version: "0.1.0"},
		&sdk.ServerOptions{Instructions: instructions})
	addReadTools(s, inv)
	addWriteTools(s, inv)
	addCheckTools(s, inv, chk)
	addChangeTools(s, cs)
	addSuggestTools(s, inv, intg)
	addDetectTools(s, cs)
	addResources(s, inv)
	return s
}

// NewReadOnly is New without the write tools, for read-scope tokens: the agent only sees what it
// may call. Write handlers still check the scope themselves.
func NewReadOnly(inv *inventory.Store, chk *checks.Scheduler, cs *changesets.Store, intg *integrations.Manager) *sdk.Server {
	s := New(inv, chk, cs, intg)
	s.RemoveTools(WriteToolNames()...)
	return s
}

// ProposeOnlyTools are the write tools a propose-scope token may still call.
var ProposeOnlyTools = map[string]bool{
	"propose_changes":    true,
	"request_connection": true,
}

// DirectWriteTools is every write tool except the proposal ones.
func DirectWriteTools() []string {
	out := []string{}
	for _, n := range WriteToolNames() {
		if !ProposeOnlyTools[n] {
			out = append(out, n)
		}
	}
	return out
}

// NewPropose is New without direct write tools: the agent can read and queue
// proposed changes, but only a human can apply them.
func NewPropose(inv *inventory.Store, chk *checks.Scheduler, cs *changesets.Store, intg *integrations.Manager) *sdk.Server {
	s := New(inv, chk, cs, intg)
	s.RemoveTools(DirectWriteTools()...)
	return s
}

var (
	writeMu    sync.Mutex
	writeNames = map[string]bool{}
)

// WriteToolNames lists every tool registered with addWrite.
func WriteToolNames() []string {
	writeMu.Lock()
	defer writeMu.Unlock()
	out := make([]string, 0, len(writeNames))
	for n := range writeNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// text returns a one-line human summary plus the structured value.
func text[T any](summary string, v T) (*sdk.CallToolResult, T, error) {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: summary}}}, v, nil
}

func addRead[In, Out any](s *sdk.Server, name, desc string, fn sdk.ToolHandlerFor[In, Out]) {
	sdk.AddTool(s, &sdk.Tool{
		Name: name, Description: desc,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, fn)
}

func addWrite[In, Out any](s *sdk.Server, name, desc string, destructive bool, fn sdk.ToolHandlerFor[In, Out]) {
	writeMu.Lock()
	writeNames[name] = true
	writeMu.Unlock()
	sdk.AddTool(s, &sdk.Tool{
		Name: name, Description: desc,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive},
	}, fn)
}

func actor(ctx context.Context) string {
	if a := request.Actor(ctx); a != "" {
		return a
	}
	return "mcp"
}

func needWrite(ctx context.Context) error {
	if !request.CanWrite(ctx) {
		return fmt.Errorf("this token is read-only or propose-only; create one with --scope write")
	}
	return nil
}

// needPropose allows both propose and write tokens to queue changes.
func needPropose(ctx context.Context) error {
	if !request.CanPropose(ctx) {
		return fmt.Errorf("this token cannot propose changes; create one with --scope propose or write")
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func resolveProject(ctx context.Context, inv *inventory.Store, id int64, slug string) (*inventory.Project, error) {
	if id > 0 {
		return inv.GetProject(ctx, id)
	}
	if slug == "" {
		return nil, fmt.Errorf("project id or slug is required")
	}
	return inv.GetProjectBySlug(ctx, slug)
}

// locationID resolves a location name to an id, creating the location when new.
func locationID(ctx context.Context, inv *inventory.Store, name string) (*int64, error) {
	if name == "" {
		return nil, nil
	}
	return inv.EnsureLocation(ctx, actor(ctx), name)
}

func hostSummary(h *inventory.Host) string {
	s := fmt.Sprintf("%s (#%d, %s)", h.Name, h.ID, h.Kind)
	if h.Location != nil {
		s += " at " + h.Location.Name
	}
	if len(h.Addresses) > 0 {
		s += " — " + h.Addresses[0].Address
	}
	if len(h.Warnings) > 0 {
		s += fmt.Sprintf(" — %d warning(s)", len(h.Warnings))
	}
	return s
}

func projectSummary(p *inventory.Project) string {
	s := fmt.Sprintf("%s (%s, #%d)", p.Name, p.Slug, p.ID)
	if p.Group != "" {
		s += " in " + p.Group
	}
	return s
}
