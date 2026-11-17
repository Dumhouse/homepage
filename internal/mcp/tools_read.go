package mcp

import (
	"context"
	"fmt"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/inventory"
)

type searchArgs struct {
	Query string `json:"q" jsonschema:"words to search for (host, service, project, app link, location)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results, default 20"`
}

type searchOut struct {
	Results []inventory.SearchResult `json:"results"`
}

type pageArgs struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"page size, default 50"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
}

type hostPage struct {
	Hosts      []inventory.Host `json:"hosts"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type projectPage struct {
	Projects   []inventory.Project `json:"projects"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type idArgs struct {
	ID int64 `json:"id" jsonschema:"entity id"`
}

type projectRef struct {
	ID   int64  `json:"id,omitempty" jsonschema:"project id (preferred if you have it)"`
	Slug string `json:"slug,omitempty" jsonschema:"project slug, e.g. acme"`
}

type accessArgs struct {
	TargetEntity string `json:"target_entity,omitempty" jsonschema:"host, service, project, domain or provider; empty lists all"`
	TargetID     int64  `json:"target_id,omitempty"`
}

type usedByArgs struct {
	TargetKind string `json:"target_kind" jsonschema:"host or service"`
	TargetID   int64  `json:"target_id"`
}

type freePortArgs struct {
	HostID int64 `json:"host_id"`
	Near   int   `json:"near,omitempty" jsonschema:"start looking at this port, default 3000"`
}

type portOut struct {
	Port int `json:"port"`
}

func addReadTools(s *sdk.Server, inv *inventory.Store) {
	addRead(s, "search", "Search hosts, services, projects, app links and locations. Ranked exact > prefix > contains.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a searchArgs) (*sdk.CallToolResult, searchOut, error) {
			res, err := inv.Search(ctx, a.Query, a.Limit)
			if err != nil {
				return nil, searchOut{}, err
			}
			return text(fmt.Sprintf("%d result(s) for %q", len(res), a.Query), searchOut{Results: res})
		})

	addRead(s, "list_hosts", "List hosts with addresses and SSH logins (no services). Paginated with limit + cursor.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a pageArgs) (*sdk.CallToolResult, hostPage, error) {
			all, err := inv.ListHosts(ctx)
			if err != nil {
				return nil, hostPage{}, err
			}
			limit := a.Limit
			if limit <= 0 {
				limit = 50
			}
			off, _ := strconv.Atoi(a.Cursor)
			if off < 0 || off > len(all) {
				off = 0
			}
			end := off + limit
			out := hostPage{Hosts: all[off:min(end, len(all))]}
			if end < len(all) {
				out.NextCursor = strconv.Itoa(end)
			}
			return text(fmt.Sprintf("%d host(s)%s", len(out.Hosts), cursorNote(out.NextCursor)), out)
		})

	addRead(s, "get_host", "Get one host with addresses, SSH logins, services, access entries and projects.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, inventory.Host, error) {
			h, err := inv.GetHost(ctx, a.ID)
			if err != nil {
				return nil, inventory.Host{}, err
			}
			return text(hostSummary(h), *h)
		})

	addRead(s, "list_services", "List the services on one host.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, []inventory.Service, error) {
			vs, err := inv.ListServices(ctx, a.ID)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d service(s)", len(vs)), vs)
		})

	addRead(s, "list_projects", "List projects (no children). Paginated with limit + cursor.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a pageArgs) (*sdk.CallToolResult, projectPage, error) {
			all, err := inv.ListProjects(ctx)
			if err != nil {
				return nil, projectPage{}, err
			}
			limit := a.Limit
			if limit <= 0 {
				limit = 50
			}
			off, _ := strconv.Atoi(a.Cursor)
			if off < 0 || off > len(all) {
				off = 0
			}
			end := off + limit
			out := projectPage{Projects: all[off:min(end, len(all))]}
			if end < len(all) {
				out.NextCursor = strconv.Itoa(end)
			}
			return text(fmt.Sprintf("%d project(s)%s", len(out.Projects), cursorNote(out.NextCursor)), out)
		})

	addRead(s, "get_project", "Get one project with links, environments, app links, hosts, services, access and connections.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a projectRef) (*sdk.CallToolResult, inventory.Project, error) {
			p, err := resolveProject(ctx, inv, a.ID, a.Slug)
			if err != nil {
				return nil, inventory.Project{}, err
			}
			return text(projectSummary(p), *p)
		})

	addRead(s, "list_locations", "List locations (Proxmox node, VPS provider, shelf) with host counts.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []inventory.Location, error) {
			locs, err := inv.ListLocations(ctx)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d location(s)", len(locs)), locs)
		})

	addRead(s, "list_tags", "List tags with how many hosts/services use them.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, []inventory.Tag, error) {
			tags, err := inv.ListTags(ctx)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d tag(s)", len(tags)), tags)
		})

	addRead(s, "list_access", "List logins (method/account/Vaultwarden link only; never secrets), optionally for one target.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a accessArgs) (*sdk.CallToolResult, []inventory.Access, error) {
			as, err := inv.ListAccess(ctx, a.TargetEntity, a.TargetID)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d access entr%s", len(as), plural(len(as), "y", "ies")), as)
		})

	addRead(s, "list_app_links", "List a project's app links and environments with resolved URLs.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a projectRef) (*sdk.CallToolResult, inventory.Project, error) {
			p, err := resolveProject(ctx, inv, a.ID, a.Slug)
			if err != nil {
				return nil, inventory.Project{}, err
			}
			return text(fmt.Sprintf("%s: %d app link(s), %d env(s)", p.Name, len(p.AppLinks), len(p.Envs)), *p)
		})

	addRead(s, "suggest_free_port", "Suggest a free TCP port on a host, skipping ports used on the same addresses.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a freePortArgs) (*sdk.CallToolResult, portOut, error) {
			p, err := inv.FreePort(ctx, a.HostID, a.Near)
			if err != nil {
				return nil, portOut{}, err
			}
			return text(fmt.Sprintf("port %d is free", p), portOut{Port: p})
		})

	addRead(s, "used_by", `Reverse lookup: which projects depend on a host or service ("what breaks if this is down?").`,
		func(ctx context.Context, _ *sdk.CallToolRequest, a usedByArgs) (*sdk.CallToolResult, []inventory.UsedBy, error) {
			used, err := inv.UsedBy(ctx, a.TargetKind, a.TargetID)
			if err != nil {
				return nil, nil, err
			}
			return text(fmt.Sprintf("%d project(s) use it", len(used)), used)
		})
}

func cursorNote(next string) string {
	if next == "" {
		return ""
	}
	return " (more: cursor " + next + ")"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
