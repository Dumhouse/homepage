package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// addResources exposes Markdown summaries as resources:
//
//	homeservices://host/{id}
//	homeservices://project/{slug}
//	homeservices://inventory
func addResources(s *sdk.Server, inv *inventory.Store) {
	read := func(uri, text string) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: uri, MIMEType: "text/markdown", Text: text,
		}}}, nil
	}

	s.AddResourceTemplate(&sdk.ResourceTemplate{
		Name: "host", Title: "Host summary", URITemplate: "homeservices://host/{id}",
		MIMEType: "text/markdown", Description: "Markdown summary of one host: addresses, SSH, services, projects, access.",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		raw := strings.TrimPrefix(req.Params.URI, "homeservices://host/")
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("host resource needs a numeric id: %q", raw)
		}
		md, err := inv.HostMarkdown(ctx, id)
		if err != nil {
			return nil, err
		}
		return read(req.Params.URI, md)
	})

	s.AddResourceTemplate(&sdk.ResourceTemplate{
		Name: "project", Title: "Project summary", URITemplate: "homeservices://project/{slug}",
		MIMEType: "text/markdown", Description: "Markdown summary of one project: links, envs, app links, hosts, connections.",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		slug := strings.TrimPrefix(req.Params.URI, "homeservices://project/")
		md, err := inv.ProjectMarkdown(ctx, slug)
		if err != nil {
			return nil, err
		}
		return read(req.Params.URI, md)
	})

	s.AddResource(&sdk.Resource{
		Name: "inventory", Title: "Whole inventory", URI: "homeservices://inventory",
		MIMEType: "text/markdown", Description: "All hosts and projects as one Markdown page.",
	}, func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		md, err := inv.InventoryMarkdown(ctx)
		if err != nil {
			return nil, err
		}
		return read(req.Params.URI, md)
	})
}
