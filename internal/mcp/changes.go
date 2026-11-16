package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dawidof/home-services/internal/changesets"
)

type proposeArgs struct {
	Title  string          `json:"title" jsonschema:"short human title for the review queue"`
	Source string          `json:"source,omitempty" jsonschema:"where the proposal came from, e.g. mcp:claude-code"`
	Ops    []changesets.Op `json:"ops" jsonschema:"changes to propose; nothing is written until a human applies them"`
}

type changesetsOut struct {
	Changesets []changesets.ChangeSet `json:"changesets"`
}

func addChangeTools(s *sdk.Server, cs *changesets.Store) {
	addWrite(s, "propose_changes", "Propose inventory changes for human review. Nothing is applied until the user approves it on the Changes page.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a proposeArgs) (*sdk.CallToolResult, changesets.ChangeSet, error) {
			if err := needPropose(ctx); err != nil {
				return nil, changesets.ChangeSet{}, err
			}
			source := a.Source
			if source == "" {
				source = "mcp:" + actor(ctx)
			}
			created, err := cs.Create(ctx, actor(ctx), a.Title, source, a.Ops)
			if err != nil {
				return nil, changesets.ChangeSet{}, err
			}
			return text(fmt.Sprintf("Proposed changeset #%d (%d op(s)) — review on /changesets", created.ID, len(created.Ops)), *created)
		})

	addRead(s, "list_changesets", "List changesets (default: pending).",
		func(ctx context.Context, _ *sdk.CallToolRequest, a struct {
			Status string `json:"status,omitempty"`
		}) (*sdk.CallToolResult, changesetsOut, error) {
			status := a.Status
			if status == "" {
				status = "pending"
			}
			list, err := cs.List(ctx, status)
			if err != nil {
				return nil, changesetsOut{}, err
			}
			return text(fmt.Sprintf("%d changeset(s)", len(list)), changesetsOut{Changesets: list})
		})

	addRead(s, "get_changeset", "Get one changeset with all ops and apply errors.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, changesets.ChangeSet, error) {
			cs1, err := cs.Get(ctx, a.ID)
			if err != nil {
				return nil, changesets.ChangeSet{}, err
			}
			return text(fmt.Sprintf("Changeset #%d: %s (%s)", cs1.ID, cs1.Title, cs1.Status), *cs1)
		})
}
