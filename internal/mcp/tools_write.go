package mcp

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Dumhouse/homepage/internal/inventory"
)

type createHostArgs struct {
	Name        string      `json:"name"`
	Hostname    string      `json:"hostname,omitempty"`
	Kind        string      `json:"kind,omitempty" jsonschema:"vm, lxc, bare or vps (default vm)"`
	OS          string      `json:"os,omitempty"`
	Description string      `json:"description,omitempty"`
	Icon        string      `json:"icon,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Lifecycle   string      `json:"lifecycle,omitempty" jsonschema:"planned, active, deprecated or decommissioned"`
	Location    string      `json:"location,omitempty" jsonschema:"location name; created if missing"`
	Addresses   []addressIn `json:"addresses,omitempty"`
	SSH         []sshIn     `json:"ssh,omitempty"`
}

type addressIn struct {
	Kind      string `json:"kind,omitempty" jsonschema:"lan, tailscale, public or other (default lan)"`
	Address   string `json:"address"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

type sshIn struct {
	User     string `json:"user"`
	Port     int    `json:"port,omitempty" jsonschema:"default 22"`
	VaultURL string `json:"vault_url,omitempty" jsonschema:"Vaultwarden item URL, never a password"`
}

type updateHostArgs struct {
	ID          int64                    `json:"id"`
	Name        *string                  `json:"name,omitempty"`
	Hostname    *string                  `json:"hostname,omitempty"`
	Kind        *string                  `json:"kind,omitempty"`
	OS          *string                  `json:"os,omitempty"`
	Description *string                  `json:"description,omitempty"`
	Icon        *string                  `json:"icon,omitempty"`
	Lifecycle   *string                  `json:"lifecycle,omitempty"`
	Tags        *[]string                `json:"tags,omitempty"`
	Location    *string                  `json:"location,omitempty" jsonschema:"location name; empty string clears it"`
	Addresses   *[]inventory.AddressOp   `json:"addresses,omitempty"`
	SSH         *[]inventory.SSHAccessOp `json:"ssh,omitempty"`
}

type serviceArgs struct {
	HostID   int64    `json:"host_id"`
	Name     string   `json:"name"`
	Type     string   `json:"type,omitempty"`
	Port     int      `json:"port,omitempty"`
	Protocol string   `json:"protocol,omitempty" jsonschema:"tcp, udp, http or https"`
	Path     string   `json:"path,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Version  string   `json:"version,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type updateServiceArgs struct {
	ID       int64     `json:"id"`
	Name     *string   `json:"name,omitempty"`
	Type     *string   `json:"type,omitempty"`
	Port     *int      `json:"port,omitempty"`
	Protocol *string   `json:"protocol,omitempty"`
	Path     *string   `json:"path,omitempty"`
	Domain   *string   `json:"domain,omitempty"`
	Version  *string   `json:"version,omitempty"`
	Notes    *string   `json:"notes,omitempty"`
	Tags     *[]string `json:"tags,omitempty"`
}

type addSSHArgs struct {
	HostID   int64  `json:"host_id"`
	User     string `json:"user"`
	Port     int    `json:"port,omitempty"`
	VaultURL string `json:"vault_url,omitempty" jsonschema:"Vaultwarden item URL, never a password"`
}

type removeSSHArgs struct {
	HostID int64 `json:"host_id"`
	SSHID  int64 `json:"ssh_id"`
}

type createProjectArgs struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty" jsonschema:"group name; created if missing"`
	RunbookMD   string `json:"runbook_md,omitempty"`
	OpenSource  bool   `json:"is_open_source,omitempty"`
	Status      string `json:"status,omitempty" jsonschema:"active or archived"`
}

type updateProjectArgs struct {
	ID          int64   `json:"id,omitempty"`
	Slug        string  `json:"slug,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Group       *string `json:"group,omitempty"`
	RunbookMD   *string `json:"runbook_md,omitempty"`
	OpenSource  *bool   `json:"is_open_source,omitempty"`
	Status      *string `json:"status,omitempty"`
}

type linkProjectArgs struct {
	Project   string `json:"project,omitempty" jsonschema:"project slug"`
	ProjectID int64  `json:"project_id,omitempty"`
	HostID    int64  `json:"host_id"`
	Env       string `json:"env,omitempty" jsonschema:"environment name, e.g. production"`
}

type linkServiceArgs struct {
	Project   string `json:"project,omitempty"`
	ProjectID int64  `json:"project_id,omitempty"`
	ServiceID int64  `json:"service_id"`
	Env       string `json:"env,omitempty"`
}

type addLinkArgs struct {
	Project   string `json:"project,omitempty"`
	ProjectID int64  `json:"project_id,omitempty"`
	Kind      string `json:"kind,omitempty" jsonschema:"repo, docs, public, secrets or other"`
	URL       string `json:"url" jsonschema:"absolute http(s) URL"`
	Label     string `json:"label,omitempty"`
}

type addAppLinkArgs struct {
	Project     string `json:"project,omitempty"`
	ProjectID   int64  `json:"project_id,omitempty"`
	Label       string `json:"label"`
	Path        string `json:"path,omitempty" jsonschema:"path inside the app, e.g. /sidekiq"`
	URL         string `json:"url,omitempty" jsonschema:"absolute URL instead of a path"`
	OnlyEnv     string `json:"only_env,omitempty" jsonschema:"show only in this env; empty means all"`
	Kind        string `json:"kind,omitempty" jsonschema:"page, admin, jobs, db, docs, health, mcp or other"`
	Icon        string `json:"icon,omitempty"`
	AccessID    int64  `json:"access_id,omitempty"`
	HealthCheck bool   `json:"health_check,omitempty"`
}

type addEnvArgs struct {
	Project   string `json:"project,omitempty"`
	ProjectID int64  `json:"project_id,omitempty"`
	Name      string `json:"name" jsonschema:"production, staging, ..."`
	BaseURL   string `json:"base_url,omitempty" jsonschema:"absolute http(s) base URL"`
}

type addAccessArgs struct {
	TargetEntity string `json:"target_entity" jsonschema:"host, service, project, domain or provider"`
	TargetID     int64  `json:"target_id"`
	Label        string `json:"label"`
	URL          string `json:"url,omitempty"`
	Method       string `json:"method,omitempty" jsonschema:"password, google_sso, github_sso, oidc, ssh_key, token, passkey or none"`
	Account      string `json:"account,omitempty"`
	TwoFA        string `json:"twofa,omitempty" jsonschema:"totp, hardware, sms, provider or none"`
	VaultItemURL string `json:"vault_item_url,omitempty" jsonschema:"Vaultwarden item URL, never a secret"`
	Notes        string `json:"notes,omitempty"`
}

type connectArgs struct {
	Project    string `json:"project,omitempty"`
	ProjectID  int64  `json:"project_id,omitempty"`
	TargetKind string `json:"target_kind" jsonschema:"host, service, provider or url"`
	TargetID   int64  `json:"target_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Role       string `json:"role,omitempty" jsonschema:"code, ci, mirror, images, secrets, logins, alerts, dns, domain, docs, storage, monitoring or other"`
	Detail     string `json:"detail,omitempty"`
	ImpactNote string `json:"impact_note,omitempty" jsonschema:"what breaks if it is down"`
	AccessID   int64  `json:"access_id,omitempty"`
}

type restoreArgs struct {
	Entity string `json:"entity" jsonschema:"host, service or project"`
	ID     int64  `json:"id"`
}

type deletedOut struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func addWriteTools(s *sdk.Server, inv *inventory.Store) {
	addWrite(s, "create_host", "Create a host. Addresses default to lan; SSH logins default to port 22 and status unknown.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a createHostArgs) (*sdk.CallToolResult, inventory.Host, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Host{}, err
			}
			loc, err := locationID(ctx, inv, a.Location)
			if err != nil {
				return nil, inventory.Host{}, err
			}
			addrs := make([]inventory.Address, len(a.Addresses))
			for i, x := range a.Addresses {
				addrs[i] = inventory.Address{Kind: x.Kind, Address: x.Address, IsPrimary: x.IsPrimary}
			}
			ssh := make([]inventory.SSHAccess, len(a.SSH))
			for i, x := range a.SSH {
				ssh[i] = inventory.SSHAccess{User: x.User, Port: x.Port, Vault: x.VaultURL}
			}
			h, err := inv.CreateHost(ctx, actor(ctx), inventory.Host{
				Name: a.Name, Hostname: a.Hostname, Kind: a.Kind, OS: a.OS, Description: a.Description,
				Icon: a.Icon, Tags: a.Tags, Lifecycle: a.Lifecycle, LocationID: loc,
				Addresses: addrs, SSH: ssh,
			})
			if err != nil {
				return nil, inventory.Host{}, err
			}
			return text("Created "+hostSummary(h), *h)
		})

	addWrite(s, "update_host", "Change only the given host fields. Addresses/SSH are patched by id; new ones have id 0; set remove=true to delete. location=\"\" clears it.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a updateHostArgs) (*sdk.CallToolResult, inventory.Host, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Host{}, err
			}
			p := inventory.HostPatch{
				Name: a.Name, Hostname: a.Hostname, Kind: a.Kind, OS: a.OS, Description: a.Description,
				Icon: a.Icon, Lifecycle: a.Lifecycle, Tags: a.Tags, Addresses: a.Addresses, SSH: a.SSH,
			}
			if a.Location != nil {
				if *a.Location == "" {
					p.LocationID = ptr(int64(0))
				} else {
					id, err := locationID(ctx, inv, *a.Location)
					if err != nil {
						return nil, inventory.Host{}, err
					}
					p.LocationID = id
				}
			}
			h, err := inv.PatchHost(ctx, actor(ctx), a.ID, p)
			if err != nil {
				return nil, inventory.Host{}, err
			}
			return text("Updated "+hostSummary(h), *h)
		})

	addWrite(s, "delete_host", "Soft-delete a host. It can be restored.", true,
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			if err := inv.DeleteHost(ctx, actor(ctx), a.ID); err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Deleted host #%d (restorable)", a.ID), deletedOut{OK: true, Message: "soft deleted"})
		})

	addWrite(s, "add_service", "Add a service to a host. protocol defaults to tcp.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a serviceArgs) (*sdk.CallToolResult, inventory.Service, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Service{}, err
			}
			v, err := inv.AddService(ctx, actor(ctx), a.HostID, inventory.Service{
				Name: a.Name, Type: a.Type, Port: a.Port, Protocol: a.Protocol, Path: a.Path,
				Domain: a.Domain, Version: a.Version, Notes: a.Notes, Tags: a.Tags,
			})
			if err != nil {
				return nil, inventory.Service{}, err
			}
			return text(fmt.Sprintf("Added service %s (#%d) on host #%d", v.Name, v.ID, v.HostID), *v)
		})

	addWrite(s, "update_service", "Change only the given service fields.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a updateServiceArgs) (*sdk.CallToolResult, inventory.Service, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Service{}, err
			}
			cur, err := inv.GetService(ctx, a.ID)
			if err != nil {
				return nil, inventory.Service{}, err
			}
			if a.Name != nil {
				cur.Name = *a.Name
			}
			if a.Type != nil {
				cur.Type = *a.Type
			}
			if a.Port != nil {
				cur.Port = *a.Port
			}
			if a.Protocol != nil {
				cur.Protocol = *a.Protocol
			}
			if a.Path != nil {
				cur.Path = *a.Path
			}
			if a.Domain != nil {
				cur.Domain = *a.Domain
			}
			if a.Version != nil {
				cur.Version = *a.Version
			}
			if a.Notes != nil {
				cur.Notes = *a.Notes
			}
			if a.Tags != nil {
				cur.Tags = *a.Tags
			}
			v, err := inv.UpdateService(ctx, actor(ctx), a.ID, *cur)
			if err != nil {
				return nil, inventory.Service{}, err
			}
			return text(fmt.Sprintf("Updated service %s (#%d)", v.Name, v.ID), *v)
		})

	addWrite(s, "remove_service", "Soft-delete a service.", true,
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			if err := inv.DeleteService(ctx, actor(ctx), a.ID); err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Removed service #%d", a.ID), deletedOut{OK: true, Message: "soft deleted"})
		})

	addWrite(s, "add_ssh_access", "Add an SSH login to a host. Store only the Vaultwarden link, never a password.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a addSSHArgs) (*sdk.CallToolResult, inventory.Host, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Host{}, err
			}
			ops := []inventory.SSHAccessOp{{User: a.User, Port: a.Port, Vault: a.VaultURL}}
			h, err := inv.PatchHost(ctx, actor(ctx), a.HostID, inventory.HostPatch{SSH: &ops})
			if err != nil {
				return nil, inventory.Host{}, err
			}
			return text("Added SSH login to "+hostSummary(h), *h)
		})

	addWrite(s, "remove_ssh_access", "Remove one SSH login from a host by its ssh id.", true,
		func(ctx context.Context, _ *sdk.CallToolRequest, a removeSSHArgs) (*sdk.CallToolResult, inventory.Host, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Host{}, err
			}
			ops := []inventory.SSHAccessOp{{ID: a.SSHID, Remove: true}}
			h, err := inv.PatchHost(ctx, actor(ctx), a.HostID, inventory.HostPatch{SSH: &ops})
			if err != nil {
				return nil, inventory.Host{}, err
			}
			return text("Removed SSH login from "+hostSummary(h), *h)
		})

	addWrite(s, "create_project", "Create a project. Slug is derived from the name when omitted; group is created if new.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a createProjectArgs) (*sdk.CallToolResult, inventory.Project, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Project{}, err
			}
			p, err := inv.CreateProject(ctx, actor(ctx), inventory.Project{
				Name: a.Name, Slug: a.Slug, Description: a.Description, Group: a.Group,
				Runbook: a.RunbookMD, OpenSource: a.OpenSource, Status: a.Status,
			})
			if err != nil {
				return nil, inventory.Project{}, err
			}
			return text("Created "+projectSummary(p), *p)
		})

	addWrite(s, "update_project", "Change only the given project fields.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a updateProjectArgs) (*sdk.CallToolResult, inventory.Project, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Project{}, err
			}
			cur, err := resolveProject(ctx, inv, a.ID, a.Slug)
			if err != nil {
				return nil, inventory.Project{}, err
			}
			if a.Name != nil {
				cur.Name = *a.Name
			}
			if a.Description != nil {
				cur.Description = *a.Description
			}
			if a.Group != nil {
				cur.Group, cur.GroupID = *a.Group, nil
			}
			if a.RunbookMD != nil {
				cur.Runbook = *a.RunbookMD
			}
			if a.OpenSource != nil {
				cur.OpenSource = *a.OpenSource
			}
			if a.Status != nil {
				cur.Status = *a.Status
			}
			p, err := inv.UpdateProject(ctx, actor(ctx), cur.ID, *cur)
			if err != nil {
				return nil, inventory.Project{}, err
			}
			return text("Updated "+projectSummary(p), *p)
		})

	addWrite(s, "delete_project", "Soft-delete a project. It can be restored.", true,
		func(ctx context.Context, _ *sdk.CallToolRequest, a idArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			if err := inv.DeleteProject(ctx, actor(ctx), a.ID); err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Deleted project #%d (restorable)", a.ID), deletedOut{OK: true, Message: "soft deleted"})
		})

	addWrite(s, "link_project", "Link a host to a project (optionally per environment).", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a linkProjectArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, deletedOut{}, err
			}
			if err := inv.LinkProjectHost(ctx, actor(ctx), p.ID, a.HostID, a.Env); err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Linked host #%d to %s", a.HostID, p.Slug), deletedOut{OK: true, Message: "linked"})
		})

	addWrite(s, "link_project_service", "Link a service to a project (optionally per environment).", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a linkServiceArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, deletedOut{}, err
			}
			if err := inv.LinkProjectService(ctx, actor(ctx), p.ID, a.ServiceID, a.Env); err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Linked service #%d to %s", a.ServiceID, p.Slug), deletedOut{OK: true, Message: "linked"})
		})

	addWrite(s, "add_link", "Add a repo/docs/public/secrets link to a project.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a addLinkArgs) (*sdk.CallToolResult, inventory.ProjectLink, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.ProjectLink{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, inventory.ProjectLink{}, err
			}
			l, err := inv.AddProjectLink(ctx, actor(ctx), p.ID, inventory.ProjectLink{Kind: a.Kind, URL: a.URL, Label: a.Label})
			if err != nil {
				return nil, inventory.ProjectLink{}, err
			}
			return text(fmt.Sprintf("Added %s link to %s", l.Kind, p.Slug), *l)
		})

	addWrite(s, "add_app_link", "Add an app link (path or absolute URL) to a project; it appears in every env unless only_env is set.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a addAppLinkArgs) (*sdk.CallToolResult, inventory.AppLink, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.AppLink{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, inventory.AppLink{}, err
			}
			in := inventory.AppLink{
				Label: a.Label, Path: a.Path, OverrideURL: a.URL, OnlyEnv: a.OnlyEnv, Kind: a.Kind,
				Icon: a.Icon, HealthCheck: a.HealthCheck,
			}
			if a.AccessID > 0 {
				in.AccessID = &a.AccessID
			}
			l, err := inv.AddAppLink(ctx, actor(ctx), p.ID, in)
			if err != nil {
				return nil, inventory.AppLink{}, err
			}
			return text(fmt.Sprintf("Added app link %s to %s", l.Label, p.Slug), *l)
		})

	addWrite(s, "add_project_env", "Add an environment (production, staging) with its base URL.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a addEnvArgs) (*sdk.CallToolResult, inventory.ProjectEnv, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.ProjectEnv{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, inventory.ProjectEnv{}, err
			}
			e, err := inv.AddProjectEnv(ctx, actor(ctx), p.ID, inventory.ProjectEnv{Name: a.Name, BaseURL: a.BaseURL})
			if err != nil {
				return nil, inventory.ProjectEnv{}, err
			}
			return text(fmt.Sprintf("Added env %s to %s", e.Name, p.Slug), *e)
		})

	addWrite(s, "add_access", "Add a login entry (method/account/Vaultwarden link only — never a secret).", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a addAccessArgs) (*sdk.CallToolResult, inventory.Access, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Access{}, err
			}
			acc, err := inv.CreateAccess(ctx, actor(ctx), inventory.Access{
				TargetEntity: a.TargetEntity, TargetID: a.TargetID, Label: a.Label, URL: a.URL,
				Method: a.Method, Account: a.Account, TwoFA: a.TwoFA, VaultItemURL: a.VaultItemURL, Notes: a.Notes,
			})
			if err != nil {
				return nil, inventory.Access{}, err
			}
			return text(fmt.Sprintf("Added access %s for %s #%d", acc.Label, acc.TargetEntity, acc.TargetID), *acc)
		})

	addWrite(s, "connect_service", "Record an outside service a project depends on, with its impact if it is down.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a connectArgs) (*sdk.CallToolResult, inventory.Connection, error) {
			if err := needWrite(ctx); err != nil {
				return nil, inventory.Connection{}, err
			}
			p, err := resolveProject(ctx, inv, a.ProjectID, a.Project)
			if err != nil {
				return nil, inventory.Connection{}, err
			}
			c := inventory.Connection{
				ProjectID: p.ID, TargetKind: a.TargetKind, URL: a.URL, Role: a.Role,
				Detail: a.Detail, ImpactNote: a.ImpactNote,
			}
			if a.TargetID > 0 {
				c.TargetID = &a.TargetID
			}
			if a.AccessID > 0 {
				c.AccessID = &a.AccessID
			}
			created, err := inv.CreateConnection(ctx, actor(ctx), c)
			if err != nil {
				return nil, inventory.Connection{}, err
			}
			return text(fmt.Sprintf("Connected %s to %s (%s)", a.TargetKind, p.Slug, created.Role), *created)
		})

	addWrite(s, "restore", "Restore a soft-deleted host, service or project.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a restoreArgs) (*sdk.CallToolResult, deletedOut, error) {
			if err := needWrite(ctx); err != nil {
				return nil, deletedOut{}, err
			}
			var err error
			switch a.Entity {
			case "host":
				err = inv.RestoreHost(ctx, actor(ctx), a.ID)
			case "service":
				err = inv.RestoreService(ctx, actor(ctx), a.ID)
			case "project":
				err = inv.RestoreProject(ctx, actor(ctx), a.ID)
			default:
				return nil, deletedOut{}, fmt.Errorf("entity must be host, service or project")
			}
			if err != nil {
				return nil, deletedOut{}, err
			}
			return text(fmt.Sprintf("Restored %s #%d", a.Entity, a.ID), deletedOut{OK: true, Message: "restored"})
		})
}
