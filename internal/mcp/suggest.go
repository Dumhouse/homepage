package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/suggest"
)

type suggestionsOut struct {
	Suggestions []suggest.Suggestion `json:"suggestions"`
}

func addSuggestTools(s *sdk.Server, inv *inventory.Store, intg *integrations.Manager) {
	addRead(s, "suggest_plugins", "Suggest read-only integrations based on the inventory (Proxmox, Caddy, Cloudflare, Electricity).",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, suggestionsOut, error) {
			configured := map[string]bool{}
			if intg != nil {
				if states, err := intg.List(ctx); err == nil {
					for _, st := range states {
						configured[st.ID] = st.Configured()
					}
				}
			}
			list, err := suggest.For(ctx, inv, configured)
			if err != nil {
				return nil, suggestionsOut{}, err
			}
			return text(fmt.Sprintf("%d suggestion(s)", len(list)), suggestionsOut{Suggestions: list})
		})
}
