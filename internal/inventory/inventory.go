// Package inventory is the single place that reads and writes hosts, services and projects.
// The JSON API, the pages and the MCP server all go through it, so rules (validation,
// soft delete, audit log) live in one spot.
package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

type Address struct {
	ID        int64  `json:"id,omitempty"`
	Kind      string `json:"kind"` // lan | tailscale | public | other
	Address   string `json:"address"`
	IsPrimary bool   `json:"is_primary"`
}

type SSHAccess struct {
	ID     int64  `json:"id,omitempty"`
	User   string `json:"user"`
	Port   int    `json:"port"`
	Status string `json:"status"` // unknown | key_only | password_allowed | unreachable (set by the checker)
	Vault  string `json:"vault_url,omitempty"`
}

type Host struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Hostname    string       `json:"hostname,omitempty"`
	Kind        string       `json:"kind"` // vm | lxc | bare | vps
	OS          string       `json:"os,omitempty"`
	Description string       `json:"description,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Tags        []string     `json:"tags"`
	Lifecycle   string       `json:"lifecycle"` // planned | active | deprecated | decommissioned
	LocationID  *int64       `json:"location_id,omitempty"`
	Location    *LocationRef `json:"location,omitempty"`
	Addresses   []Address    `json:"addresses"`
	SSH         []SSHAccess  `json:"ssh"`
	Services    []Service    `json:"services,omitempty"`
	Projects    []ProjectRef `json:"projects,omitempty"`
	Access      []Access     `json:"access,omitempty"`
	Facts       *HostFacts   `json:"facts,omitempty"`
	Warnings    []string     `json:"warnings,omitempty"` // duplicate address / port hints on write
}

// ProjectRef is the compact project entry shown on a host page.
type ProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Env  string `json:"env,omitempty"`
}

type Service struct {
	ID       int64    `json:"id"`
	HostID   int64    `json:"host_id"`
	Name     string   `json:"name"`
	Type     string   `json:"type,omitempty"`
	Port     int      `json:"port,omitempty"`
	Protocol string   `json:"protocol"` // tcp | udp | http | https
	Path     string   `json:"path,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Version  string   `json:"version,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Icon     string   `json:"icon,omitempty"` // cached favicon path, e.g. /icons/abc.png
	Tags     []string `json:"tags"`
}

type Project struct {
	ID          int64            `json:"id"`
	Name        string           `json:"name"`
	Slug        string           `json:"slug"`
	Description string           `json:"description,omitempty"`
	Group       string           `json:"group,omitempty"`    // group name; sending a new one creates it
	GroupID     *int64           `json:"group_id,omitempty"` // resolved on read and accepted on write
	OpenSource  bool             `json:"is_open_source"`
	Runbook     string           `json:"runbook_md,omitempty"`
	Status      string           `json:"status"` // active | archived
	Links       []ProjectLink    `json:"links"`
	AppLinks    []AppLink        `json:"app_links"`
	Envs        []ProjectEnv     `json:"envs"`
	Hosts       []ProjectHost    `json:"hosts"`
	Services    []ProjectService `json:"services"`
	Connections []Connection     `json:"connections"`
	Access      []Access         `json:"access"`
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) audit(ctx context.Context, tx *sql.Tx, actor, action, entity string, id int64, v any) error {
	b, _ := json.Marshal(v)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(actor,action,entity,entity_id,diff) VALUES(?,?,?,?,?)`, actor, action, entity, id, string(b))
	return err
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func oneOf(v string, def string, allowed ...string) (string, error) {
	if v == "" {
		return def, nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", invalid("%q is not one of %s", v, strings.Join(allowed, ", "))
}

func (h *Host) normalize() error {
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" {
		return invalid("name is required")
	}
	var err error
	if h.Kind, err = oneOf(h.Kind, "vm", "vm", "lxc", "bare", "vps"); err != nil {
		return err
	}
	if h.Lifecycle, err = oneOf(h.Lifecycle, "active", "planned", "active", "deprecated", "decommissioned"); err != nil {
		return err
	}
	primaries := 0
	seenAddr := map[string]bool{}
	for i := range h.Addresses {
		a := &h.Addresses[i]
		a.Address = strings.TrimSpace(a.Address)
		if a.Address == "" {
			return invalid("address %d is empty", i+1)
		}
		if seenAddr[a.Address] {
			return invalid("address %s is repeated", a.Address)
		}
		seenAddr[a.Address] = true
		if a.Kind, err = oneOf(a.Kind, "lan", "lan", "tailscale", "public", "other"); err != nil {
			return err
		}
		if a.IsPrimary {
			primaries++
		}
	}
	if primaries > 1 {
		return invalid("only one primary address")
	}
	if primaries == 0 && len(h.Addresses) > 0 {
		h.Addresses[0].IsPrimary = true
	}
	for i := range h.SSH {
		x := &h.SSH[i]
		if strings.TrimSpace(x.User) == "" {
			return invalid("ssh login %d needs a user", i+1)
		}
		if x.Port == 0 {
			x.Port = 22
		}
		if x.Port < 1 || x.Port > 65535 {
			return invalid("ssh port %d out of range", x.Port)
		}
		if x.Status == "" {
			x.Status = "unknown"
		}
	}
	if h.Tags == nil {
		h.Tags = []string{}
	}
	if h.Addresses == nil {
		h.Addresses = []Address{}
	}
	if h.SSH == nil {
		h.SSH = []SSHAccess{}
	}
	return nil
}

func (s *Store) CreateHost(ctx context.Context, actor string, h Host) (*Host, error) {
	if err := h.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkLocation(ctx, h.LocationID); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO hosts(name,hostname,kind,os,description,icon,lifecycle,location_id) VALUES(?,?,?,?,?,?,?,?)`,
			h.Name, h.Hostname, h.Kind, h.OS, h.Description, h.Icon, h.Lifecycle, h.LocationID)
		if err != nil {
			return err
		}
		h.ID, _ = r.LastInsertId()
		if err := s.writeChildren(ctx, tx, &h); err != nil {
			return err
		}
		if err := s.setTags(ctx, tx, "host", h.ID, h.Tags); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "create", "host", h.ID, h)
	})
	if err != nil {
		return nil, err
	}
	got, err := s.GetHost(ctx, h.ID)
	return s.withWarnings(ctx, got, err)
}

func (s *Store) writeChildren(ctx context.Context, tx *sql.Tx, h *Host) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM host_addresses WHERE host_id=?`, h.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_access WHERE host_id=?`, h.ID); err != nil {
		return err
	}
	for _, a := range h.Addresses {
		if _, err := tx.ExecContext(ctx, `INSERT INTO host_addresses(host_id,kind,address,is_primary) VALUES(?,?,?,?)`, h.ID, a.Kind, a.Address, a.IsPrimary); err != nil {
			return err
		}
	}
	for _, x := range h.SSH {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ssh_access(host_id,user,port,status,vault_url) VALUES(?,?,?,?,?)`, h.ID, x.User, x.Port, x.Status, x.Vault); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetHost(ctx context.Context, id int64) (*Host, error) {
	h := &Host{}
	var locationID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id,name,COALESCE(hostname,''),kind,COALESCE(os,''),COALESCE(description,''),COALESCE(icon,''),lifecycle,location_id
		FROM hosts WHERE id=? AND deleted_at IS NULL`, id).Scan(&h.ID, &h.Name, &h.Hostname, &h.Kind, &h.OS, &h.Description, &h.Icon, &h.Lifecycle, &locationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if locationID.Valid {
		v := locationID.Int64
		h.LocationID = &v
	}
	if h.Tags, err = s.tagsFor(ctx, "host", id); err != nil {
		return nil, err
	}
	if h.Location, err = s.locationOf(ctx, h.LocationID); err != nil {
		return nil, err
	}
	h.Addresses, h.SSH = []Address{}, []SSHAccess{}
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,address,is_primary FROM host_addresses WHERE host_id=? ORDER BY is_primary DESC,id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.Kind, &a.Address, &a.IsPrimary); err != nil {
			rows.Close()
			return nil, err
		}
		h.Addresses = append(h.Addresses, a)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,user,port,status,COALESCE(vault_url,'') FROM ssh_access WHERE host_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x SSHAccess
		if err := rows.Scan(&x.ID, &x.User, &x.Port, &x.Status, &x.Vault); err != nil {
			rows.Close()
			return nil, err
		}
		h.SSH = append(h.SSH, x)
	}
	rows.Close()
	if h.Services, err = s.ListServices(ctx, id); err != nil {
		return nil, err
	}
	if h.Projects, err = s.projectsForHost(ctx, id); err != nil {
		return nil, err
	}
	if h.Access, err = s.accesses(ctx, "host", id); err != nil {
		return nil, err
	}
	if facts, err := s.GetHostFacts(ctx, id); err == nil {
		h.Facts = facts
	}
	return h, nil
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// ListHosts returns hosts (with addresses and SSH logins; no services) ordered by name.
func (s *Store) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM hosts WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := []Host{}
	for _, id := range ids {
		h, err := s.GetHost(ctx, id)
		if err != nil {
			return nil, err
		}
		h.Services = nil
		out = append(out, *h)
	}
	return out, nil
}

func (s *Store) UpdateHost(ctx context.Context, actor string, id int64, h Host) (*Host, error) {
	if _, err := s.GetHost(ctx, id); err != nil {
		return nil, err
	}
	h.ID = id
	if err := h.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkLocation(ctx, h.LocationID); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE hosts SET name=?,hostname=?,kind=?,os=?,description=?,icon=?,lifecycle=?,location_id=? WHERE id=?`,
			h.Name, h.Hostname, h.Kind, h.OS, h.Description, h.Icon, h.Lifecycle, h.LocationID, id); err != nil {
			return err
		}
		if err := s.writeChildren(ctx, tx, &h); err != nil {
			return err
		}
		if err := s.setTags(ctx, tx, "host", id, h.Tags); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "update", "host", id, h)
	})
	if err != nil {
		return nil, err
	}
	got, err := s.GetHost(ctx, id)
	return s.withWarnings(ctx, got, err)
}

// DeleteHost soft-deletes (restorable).
func (s *Store) DeleteHost(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "hosts", "host", id, true)
}
func (s *Store) RestoreHost(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "hosts", "host", id, false)
}

func (s *Store) softDelete(ctx context.Context, actor, table, entity string, id int64, del bool) error {
	q, action := `UPDATE `+table+` SET deleted_at=datetime('now') WHERE id=? AND deleted_at IS NULL`, "delete"
	if !del {
		q, action = `UPDATE `+table+` SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`, "restore"
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, q, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, action, entity, id, nil)
	})
}

// ---- services ----

func (v *Service) normalize() error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return invalid("name is required")
	}
	if v.Port < 0 || v.Port > 65535 {
		return invalid("port %d out of range", v.Port)
	}
	var err error
	v.Protocol, err = oneOf(v.Protocol, "tcp", "tcp", "udp", "http", "https")
	return err
}

func (s *Store) AddService(ctx context.Context, actor string, hostID int64, v Service) (*Service, error) {
	if _, err := s.GetHost(ctx, hostID); err != nil {
		return nil, err
	}
	if err := v.normalize(); err != nil {
		return nil, err
	}
	v.HostID = hostID
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO services(host_id,name,type,port,protocol,path,domain,version,notes,icon) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			hostID, v.Name, v.Type, nullInt(v.Port), v.Protocol, v.Path, v.Domain, v.Version, v.Notes, v.Icon)
		if err != nil {
			return err
		}
		v.ID, _ = r.LastInsertId()
		if err := s.setTags(ctx, tx, "service", v.ID, v.Tags); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "create", "service", v.ID, v)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func (s *Store) ListServices(ctx context.Context, hostID int64) ([]Service, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,host_id,name,COALESCE(type,''),COALESCE(port,0),protocol,COALESCE(path,''),COALESCE(domain,''),COALESCE(version,''),COALESCE(notes,''),COALESCE(icon,'')
		FROM services WHERE host_id=? AND deleted_at IS NULL ORDER BY port,name`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Service{}
	for rows.Next() {
		var v Service
		if err := rows.Scan(&v.ID, &v.HostID, &v.Name, &v.Type, &v.Port, &v.Protocol, &v.Path, &v.Domain, &v.Version, &v.Notes, &v.Icon); err != nil {
			return nil, err
		}
		v.Tags = []string{}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		if out[i].Tags, err = s.tagsFor(ctx, "service", out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetServiceIcon stores the cached favicon path for a service.
func (s *Store) SetServiceIcon(ctx context.Context, actor string, id int64, icon string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE services SET icon=? WHERE id=? AND deleted_at IS NULL`, icon, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, "icon", "service", id, map[string]string{"icon": icon})
	})
}

// GetService returns one service by id (with tags and host id).
func (s *Store) GetService(ctx context.Context, id int64) (*Service, error) {
	v := &Service{}
	err := s.db.QueryRowContext(ctx, `SELECT id,host_id,name,COALESCE(type,''),COALESCE(port,0),protocol,
		COALESCE(path,''),COALESCE(domain,''),COALESCE(version,''),COALESCE(notes,''),COALESCE(icon,'')
		FROM services WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&v.ID, &v.HostID, &v.Name, &v.Type, &v.Port, &v.Protocol, &v.Path, &v.Domain, &v.Version, &v.Notes, &v.Icon)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if v.Tags, err = s.tagsFor(ctx, "service", id); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Store) UpdateService(ctx context.Context, actor string, id int64, v Service) (*Service, error) {
	if err := v.normalize(); err != nil {
		return nil, err
	}
	v.ID = id
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE services SET name=?,type=?,port=?,protocol=?,path=?,domain=?,version=?,notes=?,icon=? WHERE id=? AND deleted_at IS NULL`,
			v.Name, v.Type, nullInt(v.Port), v.Protocol, v.Path, v.Domain, v.Version, v.Notes, v.Icon, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if err := s.setTags(ctx, tx, "service", id, v.Tags); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "update", "service", id, v)
	})
	if err != nil {
		return nil, err
	}
	return &v, s.db.QueryRowContext(ctx, `SELECT host_id FROM services WHERE id=?`, id).Scan(&v.HostID)
}

func (s *Store) DeleteService(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "services", "service", id, true)
}
func (s *Store) RestoreService(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "services", "service", id, false)
}

// ---- projects ----

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (p *Project) normalize() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return invalid("name is required")
	}
	if p.Slug = slugify(p.Slug); p.Slug == "" {
		p.Slug = slugify(p.Name)
	}
	if p.Slug == "" {
		return invalid("name must contain letters or digits")
	}
	var err error
	p.Status, err = oneOf(p.Status, "active", "active", "archived")
	return err
}

func (s *Store) CreateProject(ctx context.Context, actor string, p Project) (*Project, error) {
	if err := p.normalize(); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		gid, err := s.resolveGroupID(ctx, tx, &p)
		if err != nil {
			return err
		}
		p.GroupID = gid
		r, err := tx.ExecContext(ctx, `INSERT INTO projects(name,slug,description,group_id,is_open_source,runbook_md,status) VALUES(?,?,?,?,?,?,?)`,
			p.Name, p.Slug, p.Description, p.GroupID, p.OpenSource, p.Runbook, p.Status)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("slug %q is already used", p.Slug)
			}
			return err
		}
		p.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "project", p.ID, p)
	})
	if err != nil {
		return nil, err
	}
	return s.GetProject(ctx, p.ID)
}

const projCols = `p.id,p.name,p.slug,COALESCE(p.description,''),COALESCE(g.name,''),p.group_id,p.is_open_source,COALESCE(p.runbook_md,''),p.status`
const projFrom = `projects p LEFT JOIN groups g ON g.id=p.group_id AND g.deleted_at IS NULL`

func scanProject(sc interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var gid sql.NullInt64
	err := sc.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.Group, &gid, &p.OpenSource, &p.Runbook, &p.Status)
	if gid.Valid {
		p.GroupID = &gid.Int64
	}
	return p, err
}

func (s *Store) GetProject(ctx context.Context, id int64) (*Project, error) {
	p, err := scanProject(s.db.QueryRowContext(ctx, `SELECT `+projCols+` FROM `+projFrom+` WHERE p.id=? AND p.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if err := s.loadProjectChildren(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProjectBySlug is GetProject keyed by the URL slug.
func (s *Store) GetProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	p, err := scanProject(s.db.QueryRowContext(ctx, `SELECT `+projCols+` FROM `+projFrom+` WHERE p.slug=? AND p.deleted_at IS NULL`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if err := s.loadProjectChildren(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+projCols+` FROM `+projFrom+` WHERE p.deleted_at IS NULL ORDER BY g.name,p.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, actor string, id int64, p Project) (*Project, error) {
	if _, err := s.GetProject(ctx, id); err != nil {
		return nil, err
	}
	if err := p.normalize(); err != nil {
		return nil, err
	}
	p.ID = id
	err := s.tx(ctx, func(tx *sql.Tx) error {
		gid, err := s.resolveGroupID(ctx, tx, &p)
		if err != nil {
			return err
		}
		p.GroupID = gid
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET name=?,slug=?,description=?,group_id=?,is_open_source=?,runbook_md=?,status=? WHERE id=?`,
			p.Name, p.Slug, p.Description, p.GroupID, p.OpenSource, p.Runbook, p.Status, id); err != nil {
			if isUniqueErr(err) {
				return invalid("slug %q is already used", p.Slug)
			}
			return err
		}
		return s.audit(ctx, tx, actor, "update", "project", id, p)
	})
	if err != nil {
		return nil, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) DeleteProject(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "projects", "project", id, true)
}
func (s *Store) RestoreProject(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "projects", "project", id, false)
}

// GetSSHAccess returns one SSH login and the host it belongs to.
func (s *Store) GetSSHAccess(ctx context.Context, id int64) (*SSHAccess, int64, error) {
	x := &SSHAccess{}
	var hostID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id,host_id,user,port,status,COALESCE(vault_url,'') FROM ssh_access WHERE id=?`, id).
		Scan(&x.ID, &hostID, &x.User, &x.Port, &x.Status, &x.Vault)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return x, hostID, nil
}
