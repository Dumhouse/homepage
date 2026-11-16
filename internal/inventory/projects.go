package inventory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ---- project links (repo / docs / public / secrets) ----

type ProjectLink struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Kind      string `json:"kind"` // repo | docs | public | secrets | other
	URL       string `json:"url"`
	Label     string `json:"label,omitempty"`
	Sort      int    `json:"sort"`
}

func (l *ProjectLink) normalize() error {
	var err error
	if l.Kind, err = oneOf(l.Kind, "other", "repo", "docs", "public", "secrets", "other"); err != nil {
		return err
	}
	l.URL = strings.TrimSpace(l.URL)
	if !isHTTPURL(l.URL) {
		return invalid("url must be absolute http(s)")
	}
	l.Label = strings.TrimSpace(l.Label)
	if l.Label == "" {
		l.Label = l.Kind
	}
	return nil
}

func (s *Store) AddProjectLink(ctx context.Context, actor string, projectID int64, l ProjectLink) (*ProjectLink, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	if err := l.normalize(); err != nil {
		return nil, err
	}
	l.ProjectID = projectID
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO project_links(project_id,kind,url,label,sort) VALUES(?,?,?,?,?)`,
			projectID, l.Kind, l.URL, l.Label, l.Sort)
		if err != nil {
			return err
		}
		l.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "project_link", l.ID, l)
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) DeleteProjectLink(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "project_links", "project_link", id, true)
}

func (s *Store) projectLinks(ctx context.Context, projectID int64) ([]ProjectLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,kind,url,COALESCE(label,''),sort FROM project_links
		 WHERE project_id=? AND deleted_at IS NULL ORDER BY sort,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectLink{}
	for rows.Next() {
		var l ProjectLink
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Kind, &l.URL, &l.Label, &l.Sort); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---- environments & app links ----

// ProjectEnv is a deploy target (production, staging, …) with its base URL.
// App links are resolved against the base URL so the UI/API can show a full URL.
type ProjectEnv struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url,omitempty"`
	Sort      int       `json:"sort"`
	Links     []AppLink `json:"links"`
	Hosts     []HostRef `json:"hosts"`
	Services  []SvcRef  `json:"services"`
}

// HostRef is a compact host reference inside an environment.
type HostRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// SvcRef is a compact service reference inside an environment.
type SvcRef struct {
	ID     int64  `json:"id"`
	HostID int64  `json:"host_id"`
	Name   string `json:"name"`
	Port   int    `json:"port,omitempty"`
}

// AppLink is a path inside a project (/admin, /sidekiq, /mcp…) shown per env.
// URL is the resolved full URL and is only set in the env listing.
type AppLink struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Label       string `json:"label"`
	Path        string `json:"path,omitempty"`
	OverrideURL string `json:"override_url,omitempty"` // absolute URL used instead of base_url + path
	OnlyEnv     string `json:"only_env,omitempty"`     // empty = every env
	Kind        string `json:"kind"`                   // page | admin | jobs | db | docs | health | mcp | other
	Icon        string `json:"icon,omitempty"`
	AccessID    *int64 `json:"access_id,omitempty"`
	HealthCheck bool   `json:"health_check"`
	Sort        int    `json:"sort"`
	URL         string `json:"url,omitempty"` // resolved for the current env
}

func (l *AppLink) normalize() error {
	l.Label = strings.TrimSpace(l.Label)
	if l.Label == "" {
		return invalid("label is required")
	}
	var err error
	if l.Kind, err = oneOf(l.Kind, "page", "page", "admin", "jobs", "db", "docs", "health", "mcp", "other"); err != nil {
		return err
	}
	l.Path = strings.TrimSpace(l.Path)
	l.OverrideURL = strings.TrimSpace(l.OverrideURL)
	if l.OverrideURL != "" {
		if !isHTTPURL(l.OverrideURL) {
			return invalid("override_url must be absolute http(s)")
		}
	} else {
		if l.Path == "" {
			return invalid("path or override_url is required")
		}
		if !strings.HasPrefix(l.Path, "/") {
			l.Path = "/" + l.Path
		}
	}
	l.OnlyEnv = strings.TrimSpace(l.OnlyEnv)
	return nil
}

func (l AppLink) resolve(base string) string {
	if l.OverrideURL != "" {
		return l.OverrideURL
	}
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + l.Path
}

func (s *Store) AddProjectEnv(ctx context.Context, actor string, projectID int64, e ProjectEnv) (*ProjectEnv, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" {
		return nil, invalid("name is required")
	}
	e.BaseURL = strings.TrimSpace(e.BaseURL)
	if e.BaseURL != "" && !isHTTPURL(e.BaseURL) {
		return nil, invalid("base_url must be absolute http(s)")
	}
	e.ProjectID = projectID
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO project_envs(project_id,name,base_url,sort) VALUES(?,?,?,?)`,
			projectID, e.Name, e.BaseURL, e.Sort)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("environment %q already exists", e.Name)
			}
			return err
		}
		e.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "project_env", e.ID, e)
	})
	if err != nil {
		return nil, err
	}
	e.Links, e.Hosts, e.Services = []AppLink{}, []HostRef{}, []SvcRef{}
	return &e, nil
}

func (s *Store) DeleteProjectEnv(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "project_envs", "project_env", id, true)
}

func (s *Store) projectEnvs(ctx context.Context, projectID int64) ([]ProjectEnv, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,name,COALESCE(base_url,''),sort FROM project_envs
		 WHERE project_id=? AND deleted_at IS NULL ORDER BY sort,name COLLATE NOCASE`, projectID)
	if err != nil {
		return nil, err
	}
	out := []ProjectEnv{}
	for rows.Next() {
		var e ProjectEnv
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.BaseURL, &e.Sort); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links, err := s.projectAppLinks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	hosts, err := s.projectHosts(ctx, projectID)
	if err != nil {
		return nil, err
	}
	svcs, err := s.projectServices(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		e := &out[i]
		e.Links, e.Hosts, e.Services = []AppLink{}, []HostRef{}, []SvcRef{}
		for _, l := range links {
			if l.OnlyEnv != "" && l.OnlyEnv != e.Name {
				continue
			}
			l.URL = l.resolve(e.BaseURL)
			e.Links = append(e.Links, l)
		}
		for _, h := range hosts {
			if h.Env == "" || h.Env == e.Name {
				e.Hosts = append(e.Hosts, HostRef{ID: h.HostID, Name: h.Name, Kind: h.Kind})
			}
		}
		for _, v := range svcs {
			if v.Env == "" || v.Env == e.Name {
				e.Services = append(e.Services, SvcRef{ID: v.ServiceID, HostID: v.HostID, Name: v.Name, Port: v.Port})
			}
		}
	}
	return out, nil
}

func (s *Store) AddAppLink(ctx context.Context, actor string, projectID int64, l AppLink) (*AppLink, error) {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	if err := l.normalize(); err != nil {
		return nil, err
	}
	if l.AccessID != nil {
		if err := s.checkAccess(ctx, l.AccessID); err != nil {
			return nil, err
		}
	}
	l.ProjectID = projectID
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO app_links(project_id,label,path,url,only_env,kind,icon,access_id,health_check,sort)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			projectID, l.Label, l.Path, l.OverrideURL, l.OnlyEnv, l.Kind, l.Icon, l.AccessID, l.HealthCheck, l.Sort)
		if err != nil {
			return err
		}
		l.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "app_link", l.ID, l)
	})
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *Store) DeleteAppLink(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "app_links", "app_link", id, true)
}

func (s *Store) projectAppLinks(ctx context.Context, projectID int64) ([]AppLink, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,label,COALESCE(path,''),COALESCE(url,''),COALESCE(only_env,''),kind,
		        COALESCE(icon,''),access_id,health_check,sort
		 FROM app_links WHERE project_id=? AND deleted_at IS NULL ORDER BY sort,label COLLATE NOCASE`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AppLink{}
	for rows.Next() {
		var l AppLink
		var access sql.NullInt64
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.Label, &l.Path, &l.OverrideURL, &l.OnlyEnv, &l.Kind,
			&l.Icon, &access, &l.HealthCheck, &l.Sort); err != nil {
			return nil, err
		}
		if access.Valid {
			l.AccessID = &access.Int64
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ---- access (logins, never secrets) ----

type Access struct {
	ID           int64  `json:"id"`
	TargetEntity string `json:"target_entity"` // host | service | project | domain | provider
	TargetID     int64  `json:"target_id"`
	Label        string `json:"label"`
	URL          string `json:"url,omitempty"`
	Method       string `json:"method"` // password | google_sso | github_sso | oidc | ssh_key | token | passkey | none
	Account      string `json:"account,omitempty"`
	TwoFA        string `json:"twofa,omitempty"` // totp | hardware | sms | provider | none
	VaultItemURL string `json:"vault_item_url,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

func (a *Access) normalize() error {
	var err error
	if a.TargetEntity, err = oneOf(a.TargetEntity, "", "host", "service", "project", "domain", "provider", "other"); err != nil {
		return err
	}
	if a.TargetEntity == "" {
		return invalid("target_entity is required")
	}
	a.Label = strings.TrimSpace(a.Label)
	if a.Label == "" {
		return invalid("label is required")
	}
	if a.Method, err = oneOf(a.Method, "none", "password", "google_sso", "github_sso", "oidc", "ssh_key", "token", "passkey", "none"); err != nil {
		return err
	}
	if a.TwoFA, err = oneOf(a.TwoFA, "none", "totp", "hardware", "sms", "provider", "none"); err != nil {
		return err
	}
	a.URL = strings.TrimSpace(a.URL)
	a.VaultItemURL = strings.TrimSpace(a.VaultItemURL)
	if a.URL != "" && !isHTTPURL(a.URL) {
		return invalid("url must be absolute http(s)")
	}
	if a.VaultItemURL != "" && !isHTTPURL(a.VaultItemURL) {
		return invalid("vault_item_url must be absolute http(s)")
	}
	return nil
}

func (s *Store) CreateAccess(ctx context.Context, actor string, a Access) (*Access, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	if a.TargetID == 0 {
		return nil, invalid("target_id is required")
	}
	if err := s.checkTarget(ctx, a.TargetEntity, a.TargetID); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO access(target_entity,target_id,label,url,method,account,twofa,vault_item_url,notes)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			a.TargetEntity, a.TargetID, a.Label, a.URL, a.Method, a.Account, a.TwoFA, a.VaultItemURL, a.Notes)
		if err != nil {
			return err
		}
		a.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "access", a.ID, a)
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) DeleteAccess(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "access", "access", id, true)
}

func (s *Store) accesses(ctx context.Context, entity string, id int64) ([]Access, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,target_entity,target_id,label,COALESCE(url,''),method,COALESCE(account,''),
		        COALESCE(twofa,'none'),COALESCE(vault_item_url,''),COALESCE(notes,'')
		 FROM access WHERE target_entity=? AND target_id=? AND deleted_at IS NULL ORDER BY label`, entity, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Access{}
	for rows.Next() {
		var a Access
		if err := rows.Scan(&a.ID, &a.TargetEntity, &a.TargetID, &a.Label, &a.URL, &a.Method, &a.Account,
			&a.TwoFA, &a.VaultItemURL, &a.Notes); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAccess returns logins, optionally filtered by target ("" = all).
func (s *Store) ListAccess(ctx context.Context, entity string, id int64) ([]Access, error) {
	if entity != "" {
		return s.accesses(ctx, entity, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT target_entity,id FROM access WHERE deleted_at IS NULL ORDER BY target_entity,label`)
	if err != nil {
		return nil, err
	}
	var keys [][2]any
	for rows.Next() {
		var e string
		var i int64
		if err := rows.Scan(&e, &i); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, [2]any{e, i})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []Access{}
	for _, k := range keys {
		as, err := s.accesses(ctx, k[0].(string), k[1].(int64))
		if err != nil {
			return nil, err
		}
		out = append(out, as...)
	}
	return out, nil
}

func (s *Store) checkAccess(ctx context.Context, id *int64) error {
	if id == nil {
		return nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM access WHERE id=? AND deleted_at IS NULL`, *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("access %d does not exist", *id)
	}
	return nil
}

func (s *Store) checkTarget(ctx context.Context, entity string, id int64) error {
	var q string
	switch entity {
	case "host":
		q = `SELECT COUNT(*) FROM hosts WHERE id=? AND deleted_at IS NULL`
	case "service":
		q = `SELECT COUNT(*) FROM services WHERE id=? AND deleted_at IS NULL`
	case "project":
		q = `SELECT COUNT(*) FROM projects WHERE id=? AND deleted_at IS NULL`
	default:
		// domains/provider accounts arrive with M8; until then other targets are free-form.
		return nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, q, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("%s %d does not exist", entity, id)
	}
	return nil
}

// ---- project ↔ host / service ----

type ProjectHost struct {
	HostID int64  `json:"host_id"`
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Env    string `json:"env,omitempty"`
}

type ProjectService struct {
	ServiceID int64  `json:"service_id"`
	HostID    int64  `json:"host_id"`
	Name      string `json:"name"`
	Port      int    `json:"port,omitempty"`
	Env       string `json:"env,omitempty"`
}

func (s *Store) LinkProjectHost(ctx context.Context, actor string, projectID, hostID int64, env string) error {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return err
	}
	if _, err := s.GetHost(ctx, hostID); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO project_hosts(project_id,host_id,env) VALUES(?,?,?)
			 ON CONFLICT(project_id,host_id,env) DO NOTHING`, projectID, hostID, env); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "link", "project", projectID, map[string]any{"host_id": hostID, "env": env})
	})
}

func (s *Store) UnlinkProjectHost(ctx context.Context, actor string, projectID, hostID int64, env string) error {
	return s.unlink(ctx, actor, "project_hosts", "project", projectID, hostID, env)
}

func (s *Store) LinkProjectService(ctx context.Context, actor string, projectID, serviceID int64, env string) error {
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return err
	}
	var hostID int64
	if err := s.db.QueryRowContext(ctx, `SELECT host_id FROM services WHERE id=? AND deleted_at IS NULL`, serviceID).Scan(&hostID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO project_services(project_id,service_id,env) VALUES(?,?,?)
			 ON CONFLICT(project_id,service_id,env) DO NOTHING`, projectID, serviceID, env); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "link", "project", projectID, map[string]any{"service_id": serviceID, "env": env})
	})
}

func (s *Store) UnlinkProjectService(ctx context.Context, actor string, projectID, serviceID int64, env string) error {
	return s.unlink(ctx, actor, "project_services", "project", projectID, serviceID, env)
}

func (s *Store) unlink(ctx context.Context, actor, table, entity string, projectID, childID int64, env string) error {
	col := "host_id"
	if table == "project_services" {
		col = "service_id"
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE project_id=? AND `+col+`=? AND env=?`, projectID, childID, env)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, "unlink", entity, projectID, map[string]any{col: childID, "env": env})
	})
}

func (s *Store) projectHosts(ctx context.Context, projectID int64) ([]ProjectHost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ph.host_id,h.name,h.kind,ph.env FROM project_hosts ph
		 JOIN hosts h ON h.id=ph.host_id AND h.deleted_at IS NULL
		 WHERE ph.project_id=? ORDER BY ph.env,h.name COLLATE NOCASE`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectHost{}
	for rows.Next() {
		var h ProjectHost
		if err := rows.Scan(&h.HostID, &h.Name, &h.Kind, &h.Env); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) projectServices(ctx context.Context, projectID int64) ([]ProjectService, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ps.service_id,v.host_id,v.name,COALESCE(v.port,0),ps.env FROM project_services ps
		 JOIN services v ON v.id=ps.service_id AND v.deleted_at IS NULL
		 WHERE ps.project_id=? ORDER BY ps.env,v.name COLLATE NOCASE`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectService{}
	for rows.Next() {
		var v ProjectService
		if err := rows.Scan(&v.ServiceID, &v.HostID, &v.Name, &v.Port, &v.Env); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// projectsForHost lists the projects a host belongs to (used on the host page).
func (s *Store) projectsForHost(ctx context.Context, hostID int64) ([]ProjectRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id,p.name,p.slug,ph.env FROM project_hosts ph
		 JOIN projects p ON p.id=ph.project_id AND p.deleted_at IS NULL
		 WHERE ph.host_id=? ORDER BY p.name COLLATE NOCASE`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectRef{}
	for rows.Next() {
		var p ProjectRef
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Env); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- connected services ----

type Connection struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	TargetKind string `json:"target_kind"` // host | service | provider | url
	TargetID   *int64 `json:"target_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Role       string `json:"role"`
	Detail     string `json:"detail,omitempty"`
	ImpactNote string `json:"impact_note,omitempty"`
	AccessID   *int64 `json:"access_id,omitempty"`
	Source     string `json:"source"` // manual | detected
}

func (c *Connection) normalize() error {
	var err error
	if c.TargetKind, err = oneOf(c.TargetKind, "", "host", "service", "provider", "url"); err != nil {
		return err
	}
	if c.TargetKind == "" {
		return invalid("target_kind is required")
	}
	if c.Role, err = oneOf(c.Role, "other", "code", "ci", "mirror", "images", "secrets", "logins", "alerts", "dns",
		"domain", "docs", "storage", "monitoring", "other"); err != nil {
		return err
	}
	if c.Source, err = oneOf(c.Source, "manual", "manual", "detected"); err != nil {
		return err
	}
	c.URL = strings.TrimSpace(c.URL)
	if c.TargetKind == "url" {
		if !isHTTPURL(c.URL) {
			return invalid("url must be absolute http(s)")
		}
	} else if c.TargetID == nil {
		return invalid("target_id is required")
	}
	if c.URL != "" && c.TargetKind != "url" && !isHTTPURL(c.URL) {
		return invalid("url must be absolute http(s)")
	}
	return nil
}

func (s *Store) CreateConnection(ctx context.Context, actor string, c Connection) (*Connection, error) {
	if _, err := s.GetProject(ctx, c.ProjectID); err != nil {
		return nil, err
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	if c.TargetID != nil {
		if err := s.checkTarget(ctx, c.TargetKind, *c.TargetID); err != nil {
			return nil, err
		}
	}
	if err := s.checkAccess(ctx, c.AccessID); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO project_connections(project_id,target_kind,target_id,url,role,detail,impact_note,access_id,source)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			c.ProjectID, c.TargetKind, c.TargetID, c.URL, c.Role, c.Detail, c.ImpactNote, c.AccessID, c.Source)
		if err != nil {
			return err
		}
		c.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "connection", c.ID, c)
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) DeleteConnection(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "project_connections", "connection", id, true)
}

func (s *Store) projectConnections(ctx context.Context, projectID int64) ([]Connection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,target_kind,target_id,COALESCE(url,''),role,COALESCE(detail,''),
		        COALESCE(impact_note,''),access_id,source
		 FROM project_connections WHERE project_id=? AND deleted_at IS NULL ORDER BY role,id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		var tid, aid sql.NullInt64
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.TargetKind, &tid, &c.URL, &c.Role, &c.Detail,
			&c.ImpactNote, &aid, &c.Source); err != nil {
			return nil, err
		}
		if tid.Valid {
			c.TargetID = &tid.Int64
		}
		if aid.Valid {
			c.AccessID = &aid.Int64
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UsedBy answers "what breaks if this service/host goes down?".
type UsedBy struct {
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project"`
	ProjectSlug string `json:"slug"`
	Role        string `json:"role"`
	ImpactNote  string `json:"impact_note,omitempty"`
}

func (s *Store) UsedBy(ctx context.Context, targetKind string, targetID int64) ([]UsedBy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id,p.name,p.slug,c.role,COALESCE(c.impact_note,'')
		 FROM project_connections c JOIN projects p ON p.id=c.project_id AND p.deleted_at IS NULL
		 WHERE c.target_kind=? AND c.target_id=? AND c.deleted_at IS NULL
		 ORDER BY p.name COLLATE NOCASE`, targetKind, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsedBy{}
	for rows.Next() {
		var u UsedBy
		if err := rows.Scan(&u.ProjectID, &u.ProjectName, &u.ProjectSlug, &u.Role, &u.ImpactNote); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) loadProjectChildren(ctx context.Context, p *Project) error {
	var err error
	if p.Links, err = s.projectLinks(ctx, p.ID); err != nil {
		return err
	}
	if p.Envs, err = s.projectEnvs(ctx, p.ID); err != nil {
		return err
	}
	if p.AppLinks, err = s.projectAppLinks(ctx, p.ID); err != nil {
		return err
	}
	if p.Hosts, err = s.projectHosts(ctx, p.ID); err != nil {
		return err
	}
	if p.Services, err = s.projectServices(ctx, p.ID); err != nil {
		return err
	}
	if p.Connections, err = s.projectConnections(ctx, p.ID); err != nil {
		return err
	}
	if p.Access, err = s.accesses(ctx, "project", p.ID); err != nil {
		return err
	}
	return nil
}
