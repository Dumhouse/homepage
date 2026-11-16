package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ExportData is the whole live inventory in one JSON document. Ids are the
// database ids so repeated exports are diffable.
type ExportData struct {
	Version    int        `json:"version"`
	ExportedAt string     `json:"exported_at"`
	Locations  []Location `json:"locations"`
	Groups     []Group    `json:"groups"`
	Tags       []Tag      `json:"tags"`
	Hosts      []Host     `json:"hosts"`
	Projects   []Project  `json:"projects"`
}

// Export reads every live entity. Services, addresses, SSH logins, access,
// links, envs, app links and connections are embedded in their parent.
func (s *Store) Export(ctx context.Context) (*ExportData, error) {
	out := &ExportData{Version: 1, ExportedAt: time.Now().UTC().Format(time.RFC3339), Hosts: []Host{}, Projects: []Project{}}
	var err error
	if out.Locations, err = s.ListLocations(ctx); err != nil {
		return nil, err
	}
	if out.Groups, err = s.ListGroups(ctx); err != nil {
		return nil, err
	}
	if out.Tags, err = s.ListTags(ctx); err != nil {
		return nil, err
	}
	if out.Hosts, err = s.ListHosts(ctx); err != nil {
		return nil, err
	}
	// ListHosts leaves services out; load each host fully.
	for i := range out.Hosts {
		full, err := s.GetHost(ctx, out.Hosts[i].ID)
		if err != nil {
			return nil, err
		}
		out.Hosts[i] = *full
	}
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		full, err := s.GetProject(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		out.Projects = append(out.Projects, *full)
	}
	return out, nil
}

// Counts is how many locations/hosts/projects an import touches.
type Counts struct {
	Locations int `json:"locations"`
	Hosts     int `json:"hosts"`
	Projects  int `json:"projects"`
}

// ImportSummary is the dry-run/report result. Dry-run is the default at the API.
type ImportSummary struct {
	Mode     string   `json:"mode"`
	DryRun   bool     `json:"dry_run"`
	Create   Counts   `json:"create"`
	Update   Counts   `json:"update"`
	Delete   Counts   `json:"delete"`
	Warnings []string `json:"warnings,omitempty"`
}

// Import merges or replaces the inventory from an export document.
//
//   - merge: match locations by name, hosts by name and projects by slug; update
//     those and add anything new. Existing children are kept unless the incoming
//     item has the same natural key.
//   - replace: hard-delete all inventory rows (audit log and tokens stay) and
//     create the document. Only call with dryRun=false after reviewing the summary.
func (s *Store) Import(ctx context.Context, actor string, data *ExportData, mode string, dryRun bool) (*ImportSummary, error) {
	if mode == "" {
		mode = "merge"
	}
	if mode != "merge" && mode != "replace" {
		return nil, invalid("mode must be merge or replace")
	}
	if err := validateExport(data); err != nil {
		return nil, err
	}
	sum := &ImportSummary{Mode: mode, DryRun: dryRun}

	if dryRun {
		switch mode {
		case "merge":
			for _, l := range data.Locations {
				if s.locationByName(ctx, l.Name) == 0 {
					sum.Create.Locations++
				} else {
					sum.Update.Locations++
				}
			}
			for _, h := range data.Hosts {
				if id := s.hostByName(ctx, h.Name); id == 0 {
					sum.Create.Hosts++
				} else {
					sum.Update.Hosts++
				}
			}
			for _, p := range data.Projects {
				if id := s.projectBySlug(ctx, p.Slug); id == 0 {
					sum.Create.Projects++
				} else {
					sum.Update.Projects++
				}
			}
		case "replace":
			sum.Create = Counts{len(data.Locations), len(data.Hosts), len(data.Projects)}
			live, err := s.Export(ctx)
			if err != nil {
				return nil, err
			}
			sum.Delete = Counts{len(live.Locations), len(live.Hosts), len(live.Projects)}
		}
		return sum, nil
	}

	if mode == "replace" {
		live, err := s.Export(ctx)
		if err != nil {
			return nil, err
		}
		sum.Delete = Counts{len(live.Locations), len(live.Hosts), len(live.Projects)}
		if err := s.wipe(ctx, actor); err != nil {
			return nil, err
		}
	}

	// Locations first: hosts reference them by name.
	locIDs := map[string]int64{}
	for _, l := range data.Locations {
		l.ID = 0
		if id := s.locationByName(ctx, l.Name); id != 0 {
			up, err := s.UpdateLocation(ctx, actor, id, l)
			if err != nil {
				return nil, err
			}
			locIDs[l.Name] = up.ID
			sum.Update.Locations++
			continue
		}
		created, err := s.CreateLocation(ctx, actor, l)
		if err != nil {
			return nil, err
		}
		locIDs[l.Name] = created.ID
		sum.Create.Locations++
	}

	// Hosts (with services and access). Keep old→new ids for project links.
	hostIDs := map[string]int64{}
	oldHostIDs := map[int64]int64{}
	oldSvcIDs := map[int64]int64{}
	for _, h := range data.Hosts {
		oldID := h.ID
		services := h.Services
		accesses := h.Access
		if h.Location != nil {
			if id, ok := locIDs[h.Location.Name]; ok {
				h.LocationID = &id
			}
		} else {
			h.LocationID = nil
		}
		h.Location = nil
		h.Projects = nil
		h.Access = nil
		h.Services = nil
		h.Warnings = nil

		var id int64
		if existing := s.hostByName(ctx, h.Name); existing != 0 {
			h.ID = existing
			if _, err := s.UpdateHost(ctx, actor, existing, h); err != nil {
				return nil, err
			}
			id = existing
			sum.Update.Hosts++
		} else {
			h.ID = 0
			created, err := s.CreateHost(ctx, actor, h)
			if err != nil {
				return nil, err
			}
			id = created.ID
			sum.Create.Hosts++
		}
		hostIDs[h.Name] = id
		if oldID != 0 {
			oldHostIDs[oldID] = id
		}
		for _, v := range services {
			old := v.ID
			v.ID = 0
			v.HostID = 0
			existingSvc := s.serviceByName(ctx, id, v.Name)
			if existingSvc != 0 {
				if old != 0 {
					oldSvcIDs[old] = existingSvc
				}
				continue
			}
			created, err := s.AddService(ctx, actor, id, v)
			if err != nil {
				return nil, err
			}
			if old != 0 {
				oldSvcIDs[old] = created.ID
			}
		}
		if err := s.importAccess(ctx, actor, "host", id, accesses, nil); err != nil {
			return nil, err
		}
	}

	// Projects and their children.
	for _, p := range data.Projects {
		children := p
		name, slug, group := p.Name, p.Slug, p.Group
		p.Links, p.AppLinks, p.Envs, p.Hosts, p.Services, p.Connections, p.Access = nil, nil, nil, nil, nil, nil, nil
		p.Group, p.GroupID = group, nil
		p.ID = 0

		var id int64
		existing := s.projectBySlug(ctx, slug)
		if existing != 0 {
			if _, err := s.UpdateProject(ctx, actor, existing, p); err != nil {
				return nil, err
			}
			id = existing
			sum.Update.Projects++
		} else {
			created, err := s.CreateProject(ctx, actor, p)
			if err != nil {
				return nil, err
			}
			id = created.ID
			sum.Create.Projects++
		}

		accessMap := map[int64]int64{}
		if err := s.importAccess(ctx, actor, "project", id, children.Access, accessMap); err != nil {
			return nil, err
		}
		current, err := s.GetProject(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.importProjectChildren(ctx, actor, id, current, &children, accessMap, hostIDs, oldHostIDs, oldSvcIDs); err != nil {
			return nil, fmt.Errorf("project %s: %w", name, err)
		}
	}
	return sum, nil
}

func (s *Store) importProjectChildren(ctx context.Context, actor string, id int64, cur *Project, data *Project,
	accessMap map[int64]int64, hostIDs map[string]int64, oldHostIDs, oldSvcIDs map[int64]int64) error {

	for _, l := range data.Links {
		if currentHasLink(cur, l) {
			continue
		}
		l.ID, l.ProjectID = 0, id
		if _, err := s.AddProjectLink(ctx, actor, id, l); err != nil {
			return err
		}
	}
	for _, e := range data.Envs {
		if currentHasEnv(cur, e.Name) {
			continue
		}
		e.ID, e.ProjectID, e.Links, e.Hosts, e.Services = 0, id, nil, nil, nil
		if _, err := s.AddProjectEnv(ctx, actor, id, e); err != nil {
			return err
		}
	}
	for _, l := range data.AppLinks {
		if currentHasAppLink(cur, l) {
			continue
		}
		oldAccess := l.AccessID
		l.ID, l.ProjectID, l.URL = 0, id, ""
		l.AccessID = nil
		if oldAccess != nil {
			if mapped, ok := accessMap[*oldAccess]; ok {
				l.AccessID = &mapped
			}
		}
		if _, err := s.AddAppLink(ctx, actor, id, l); err != nil {
			return err
		}
	}
	for _, h := range data.Hosts {
		hostID, ok := hostIDs[h.Name]
		if !ok {
			hostID = oldHostIDs[h.HostID]
		}
		if hostID == 0 || currentHasHost(cur, hostID, h.Env) {
			continue
		}
		if err := s.LinkProjectHost(ctx, actor, id, hostID, h.Env); err != nil {
			return err
		}
	}
	for _, v := range data.Services {
		svcID, ok := oldSvcIDs[v.ServiceID]
		if !ok || currentHasService(cur, svcID, v.Env) {
			continue
		}
		if err := s.LinkProjectService(ctx, actor, id, svcID, v.Env); err != nil {
			return err
		}
	}
	for _, c := range data.Connections {
		if currentHasConnection(cur, c) {
			continue
		}
		oldTarget, oldAccess := c.TargetID, c.AccessID
		c.ID, c.ProjectID, c.TargetID, c.AccessID = 0, id, nil, nil
		switch c.TargetKind {
		case "host":
			if oldTarget != nil {
				if mapped := oldHostIDs[*oldTarget]; mapped != 0 {
					c.TargetID = &mapped
				}
			}
		case "service":
			if oldTarget != nil {
				if mapped := oldSvcIDs[*oldTarget]; mapped != 0 {
					c.TargetID = &mapped
				}
			}
		}
		if c.TargetKind != "url" && c.TargetID == nil {
			continue // its target did not survive the import
		}
		if oldAccess != nil {
			if mapped, ok := accessMap[*oldAccess]; ok {
				c.AccessID = &mapped
			}
		}
		if _, err := s.CreateConnection(ctx, actor, c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) importAccess(ctx context.Context, actor, entity string, targetID int64, list []Access, mapping map[int64]int64) error {
	if len(list) == 0 {
		return nil
	}
	existing, err := s.accesses(ctx, entity, targetID)
	if err != nil {
		return err
	}
	for _, a := range list {
		old := a.ID
		a.ID, a.TargetID, a.TargetEntity = 0, targetID, entity
		var found int64
		for _, e := range existing {
			if e.Label == a.Label {
				found = e.ID
				break
			}
		}
		if found != 0 {
			if mapping != nil && old != 0 {
				mapping[old] = found
			}
			continue
		}
		created, err := s.CreateAccess(ctx, actor, a)
		if err != nil {
			return err
		}
		if mapping != nil && old != 0 {
			mapping[old] = created.ID
		}
	}
	return nil
}

func (s *Store) wipe(ctx context.Context, actor string) error {
	stmts := []string{
		`DELETE FROM project_connections`, `DELETE FROM project_services`, `DELETE FROM project_hosts`,
		`DELETE FROM app_links`, `DELETE FROM project_links`, `DELETE FROM project_envs`, `DELETE FROM access`,
		`DELETE FROM services`, `DELETE FROM ssh_access`, `DELETE FROM host_addresses`,
		`DELETE FROM projects`, `DELETE FROM hosts`, `DELETE FROM locations`, `DELETE FROM groups`,
		`DELETE FROM taggings`, `DELETE FROM tags`,
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, actor, "import_replace", "inventory", 0, nil)
	})
}

func (s *Store) locationByName(ctx context.Context, name string) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM locations WHERE name=? AND deleted_at IS NULL`, name).Scan(&id); err != nil {
		return 0
	}
	return id
}

func (s *Store) hostByName(ctx context.Context, name string) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE name=? AND deleted_at IS NULL ORDER BY id LIMIT 1`, name).Scan(&id); err != nil {
		return 0
	}
	return id
}

func (s *Store) projectBySlug(ctx context.Context, slug string) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE slug=? AND deleted_at IS NULL`, slug).Scan(&id); err != nil {
		return 0
	}
	return id
}

func (s *Store) serviceByName(ctx context.Context, hostID int64, name string) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM services WHERE host_id=? AND name=? AND deleted_at IS NULL`, hostID, name).Scan(&id); err != nil {
		return 0
	}
	return id
}

func validateExport(d *ExportData) error {
	if d == nil {
		return invalid("empty document")
	}
	slugs := map[string]bool{}
	for i, p := range d.Projects {
		if p.Name == "" {
			return invalid("project %d has no name", i+1)
		}
		slug := slugify(p.Slug)
		if slug == "" {
			slug = slugify(p.Name)
		}
		if slugs[slug] {
			return invalid("duplicate project slug %q", slug)
		}
		slugs[slug] = true
	}
	names := map[string]bool{}
	for i, h := range d.Hosts {
		if h.Name == "" {
			return invalid("host %d has no name", i+1)
		}
		if names[h.Name] {
			return invalid("duplicate host name %q", h.Name)
		}
		names[h.Name] = true
	}
	return nil
}

func currentHasLink(p *Project, l ProjectLink) bool {
	for _, x := range p.Links {
		if x.URL == l.URL {
			return true
		}
	}
	return false
}
func currentHasEnv(p *Project, name string) bool {
	for _, x := range p.Envs {
		if x.Name == name {
			return true
		}
	}
	return false
}
func currentHasAppLink(p *Project, l AppLink) bool {
	for _, x := range p.AppLinks {
		if x.Label == l.Label && x.OnlyEnv == l.OnlyEnv {
			return true
		}
	}
	return false
}
func currentHasHost(p *Project, id int64, env string) bool {
	for _, x := range p.Hosts {
		if x.HostID == id && x.Env == env {
			return true
		}
	}
	return false
}
func currentHasService(p *Project, id int64, env string) bool {
	for _, x := range p.Services {
		if x.ServiceID == id && x.Env == env {
			return true
		}
	}
	return false
}
func currentHasConnection(p *Project, c Connection) bool {
	for _, x := range p.Connections {
		if x.Role == c.Role && x.URL == c.URL && x.Detail == c.Detail {
			return true
		}
	}
	return false
}
