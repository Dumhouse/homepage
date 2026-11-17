// Package changesets stores proposed changes (from agents or plugin syncs) and
// applies them only after review. Undo soft-deletes entities created by the set.
package changesets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Dumhouse/homepage/internal/inventory"
)

var ErrNotFound = errors.New("changeset not found")

type Op struct {
	ID        int64             `json:"id,omitempty"`
	Action    string            `json:"action"` // create | update | link
	Entity    string            `json:"entity"` // host | service | project | domain | dns_record
	Ref       string            `json:"ref"`
	Data      map[string]string `json:"data,omitempty"`
	Note      string            `json:"note,omitempty"`
	AppliedID int64             `json:"applied_entity_id,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type ChangeSet struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Status    string `json:"status"`
	Actor     string `json:"actor,omitempty"`
	CreatedAt string `json:"created_at"`
	AppliedAt string `json:"applied_at,omitempty"`
	Ops       []Op   `json:"ops"`
}

type Store struct {
	db  *sql.DB
	inv *inventory.Store
}

func New(db *sql.DB, inv *inventory.Store) *Store { return &Store{db: db, inv: inv} }

func (s *Store) Create(ctx context.Context, actor, title, source string, ops []Op) (*ChangeSet, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = source + " changes"
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("%w: no ops", inventory.ErrInvalid)
	}
	var id int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO changesets(title,source,actor) VALUES(?,?,?)`, title, source, actor)
		if err != nil {
			return err
		}
		id, _ = r.LastInsertId()
		for _, op := range ops {
			data, _ := json.Marshal(op.Data)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO changeset_ops(changeset_id,action,entity,ref,data,note) VALUES(?,?,?,?,?,?)`,
				id, op.Action, op.Entity, op.Ref, string(data), op.Note); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, actor, "propose", "changeset", id, map[string]any{"title": title, "ops": len(ops)})
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, id int64) (*ChangeSet, error) {
	c := &ChangeSet{}
	var applied sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,title,source,status,COALESCE(actor,''),created_at,applied_at
		 FROM changesets WHERE id=?`, id).
		Scan(&c.ID, &c.Title, &c.Source, &c.Status, &c.Actor, &c.CreatedAt, &applied)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if applied.Valid {
		c.AppliedAt = applied.String
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,action,entity,COALESCE(ref,''),data,COALESCE(note,''),applied_entity_id,COALESCE(error,'')
		 FROM changeset_ops WHERE changeset_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c.Ops = []Op{}
	for rows.Next() {
		var op Op
		var raw string
		var appliedID sql.NullInt64
		if err := rows.Scan(&op.ID, &op.Action, &op.Entity, &op.Ref, &raw, &op.Note, &appliedID, &op.Error); err != nil {
			return nil, err
		}
		op.Data = map[string]string{}
		_ = json.Unmarshal([]byte(raw), &op.Data)
		if appliedID.Valid {
			op.AppliedID = appliedID.Int64
		}
		c.Ops = append(c.Ops, op)
	}
	return c, rows.Err()
}

func (s *Store) List(ctx context.Context, status string) ([]ChangeSet, error) {
	q := `SELECT id FROM changesets`
	var args []any
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []ChangeSet{}
	for _, id := range ids {
		c, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// Apply runs every op it understands. Unsupported ops are kept with an error so
// the reviewer can see exactly what was skipped.
func (s *Store) Apply(ctx context.Context, actor string, id int64) (*ChangeSet, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != "pending" {
		return nil, fmt.Errorf("%w: changeset is %s", inventory.ErrInvalid, c.Status)
	}
	applied := 0
	for _, op := range c.Ops {
		entityID, opErr := s.applyOp(ctx, actor, op)
		msg := ""
		if opErr != nil {
			msg = opErr.Error()
		} else {
			applied++
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE changeset_ops SET applied_entity_id=?, error=? WHERE id=?`,
			nullID(entityID), msg, op.ID); err != nil {
			return nil, err
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE changesets SET status='applied', applied_at=datetime('now') WHERE id=?`, id); err != nil {
		return nil, err
	}
	_ = s.auditDB(ctx, actor, "apply", "changeset", id, map[string]any{"applied": applied})
	return s.Get(ctx, id)
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (s *Store) applyOp(ctx context.Context, actor string, op Op) (int64, error) {
	data := op.Data
	name := func() string {
		if data["name"] != "" {
			return data["name"]
		}
		return op.Ref
	}
	switch op.Entity {
	case "host":
		// Only ids of things this changeset created are recorded: Undo
		// deletes them, so an existing host must never be returned.
		if existing := s.findHost(ctx, name(), data["address"]); existing != 0 {
			if h, err := s.inv.GetHost(ctx, existing); err == nil && len(h.Addresses) == 0 && data["address"] != "" {
				h.Addresses = []inventory.Address{{Kind: orDefault(data["address_kind"], "lan"), Address: data["address"], IsPrimary: true}}
				if _, err := s.inv.UpdateHost(ctx, actor, existing, *h); err != nil {
					return 0, err
				}
			}
			return 0, nil
		}
		loc, err := s.inv.EnsureLocation(ctx, actor, data["location"])
		if err != nil {
			return 0, err
		}
		tags := []string{"proposed"}
		for _, t := range strings.Split(data["tags"], ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		h := inventory.Host{Name: name(), Kind: data["kind"], Tags: tags, LocationID: loc, OS: data["os"]}
		var notes []string
		if data["proxmox_vmid"] != "" {
			notes = append(notes, "Proxmox VMID "+data["proxmox_vmid"]+" on "+data["location"])
		}
		if data["memory_mb"] != "" && data["memory_mb"] != "0" {
			notes = append(notes, data["memory_mb"]+" MB RAM, "+data["disk_gb"]+" GB disk")
		}
		if data["summary"] != "" {
			notes = append([]string{data["summary"]}, notes...)
		}
		h.Description = strings.Join(notes, " · ")
		if data["status"] == "stopped" || data["status"] == "off" {
			h.Tags = append(h.Tags, "stopped")
		}
		if data["address"] != "" {
			h.Addresses = []inventory.Address{{Kind: orDefault(data["address_kind"], "lan"), Address: data["address"], IsPrimary: true}}
		}
		if data["address2"] != "" && data["address2"] != data["address"] {
			h.Addresses = append(h.Addresses, inventory.Address{Kind: orDefault(data["address2_kind"], "lan"), Address: data["address2"]})
		}
		created, err := s.inv.CreateHost(ctx, actor, h)
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "project":
		created, err := s.inv.CreateProject(ctx, actor, inventory.Project{Name: name(), Group: data["group"]})
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "domain":
		for _, d := range s.domains(ctx) {
			if d.Name == op.Ref {
				return 0, nil // already there; never hand Undo an existing id
			}
		}
		created, err := s.inv.CreateDomain(ctx, actor, inventory.Domain{
			Name: op.Ref, Source: data["source"], Nameservers: data["nameservers"],
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "dns_record":
		domainName := data["domain"]
		var domainID int64
		for _, d := range s.domains(ctx) {
			if d.Name == domainName {
				domainID = d.ID
			}
		}
		if domainID == 0 {
			return 0, fmt.Errorf("domain %q is not in the inventory", domainName)
		}
		rec := inventory.DomainRecord{Name: data["name"], Type: data["type"], Content: data["content"]}
		if rec.Name == "" {
			rec.Name = op.Ref
		}
		if rec.Type == "" {
			rec.Type = "A"
		}
		created, err := s.inv.AddDomainRecord(ctx, actor, domainID, rec)
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "connection":
		var proj *inventory.Project
		projects, _ := s.inv.ListProjects(ctx)
		for i := range projects {
			if projects[i].Slug == data["project"] || projects[i].Name == data["project"] {
				proj = &projects[i]
				break
			}
		}
		if proj == nil {
			return 0, fmt.Errorf("project %q is not in the inventory", data["project"])
		}
		c := inventory.Connection{
			ProjectID: proj.ID, TargetKind: data["target_kind"], URL: data["url"],
			Role: data["role"], Detail: data["detail"], ImpactNote: data["impact_note"],
		}
		port, _ := strconv.ParseInt(data["target_id"], 10, 64)
		if port != 0 {
			c.TargetID = &port
		}
		created, err := s.inv.CreateConnection(ctx, actor, c)
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "service":
		if op.Action == "link" {
			return s.linkService(ctx, actor, data)
		}
		hostName := data["host"]
		var hostID int64
		hosts, _ := s.inv.ListHosts(ctx)
		for _, h := range hosts {
			if h.Name == hostName {
				hostID = h.ID
				break
			}
			for _, a := range h.Addresses {
				if a.Address == hostName {
					hostID = h.ID
					break
				}
			}
		}
		if hostID == 0 {
			return 0, fmt.Errorf("host %q (name or address) is not in the inventory", hostName)
		}
		port := 0
		fmt.Sscanf(data["port"], "%d", &port)
		created, err := s.inv.AddService(ctx, actor, hostID, inventory.Service{
			Name: name(), Port: port, Protocol: data["protocol"], Domain: data["domain"],
		})
		if err != nil {
			return 0, err
		}
		return created.ID, nil
	case "address":
		return s.linkAddress(ctx, actor, data)
	case "cost":
		return s.createCost(ctx, actor, op)
	case "facts":
		if n, missing, _ := s.ApplyFacts(ctx, actor, []Op{op}); n == 0 {
			return 0, fmt.Errorf("no host %v in the inventory", missing)
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported entity %q", op.Entity)
	}
}

func (s *Store) findHost(ctx context.Context, name, address string) int64 {
	hosts, _ := s.inv.ListHosts(ctx)
	for _, h := range hosts {
		if name != "" && h.Name == name {
			return h.ID
		}
		if address != "" {
			for _, a := range h.Addresses {
				if a.Address == address {
					return h.ID
				}
			}
		}
	}
	return 0
}

func (s *Store) domains(ctx context.Context) []inventory.Domain {
	list, _ := s.inv.ListDomains(ctx)
	return list
}

func (s *Store) Reject(ctx context.Context, actor string, id int64) error {
	return s.setStatus(ctx, actor, id, "pending", "rejected", "reject")
}

// Undo soft-deletes the entities this set created and marks it undone.
func (s *Store) Undo(ctx context.Context, actor string, id int64) error {
	c, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if c.Status != "applied" {
		return fmt.Errorf("%w: changeset is %s", inventory.ErrInvalid, c.Status)
	}
	for _, op := range c.Ops {
		if op.AppliedID == 0 {
			continue
		}
		switch op.Entity {
		case "connection":
			_ = s.inv.DeleteConnection(ctx, actor, op.AppliedID)
		case "host":
			_ = s.inv.DeleteHost(ctx, actor, op.AppliedID)
		case "project":
			_ = s.inv.DeleteProject(ctx, actor, op.AppliedID)
		case "domain":
			_ = s.inv.DeleteDomain(ctx, actor, op.AppliedID)
		case "dns_record":
			_ = s.inv.DeleteDomainRecord(ctx, actor, op.AppliedID)
		case "service":
			_ = s.inv.DeleteService(ctx, actor, op.AppliedID)
		}
	}
	return s.setStatus(ctx, actor, id, "applied", "undone", "undo")
}

func (s *Store) setStatus(ctx context.Context, actor string, id int64, from, to, action string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE changesets SET status=? WHERE id=? AND status=?`, to, id, from)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: changeset is not %s", inventory.ErrInvalid, from)
		}
		return s.audit(ctx, tx, actor, action, "changeset", id, map[string]string{"status": to})
	})
}

func (s *Store) auditDB(ctx context.Context, actor, action, entity string, id int64, v any) error {
	b, _ := json.Marshal(v)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log(actor,action,entity,entity_id,diff) VALUES(?,?,?,?,?)`,
		actor, action, entity, id, string(b))
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

func (s *Store) audit(ctx context.Context, tx *sql.Tx, actor, action, entity string, id int64, v any) error {
	b, _ := json.Marshal(v)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_log(actor,action,entity,entity_id,diff) VALUES(?,?,?,?,?)`,
		actor, action, entity, id, string(b))
	return err
}

// linkService records that a domain is served by address:port (a reverse
// proxy route): it sets the domain on the service on that port, or adds the
// service. Returns the id only when a service was created.
func (s *Store) linkService(ctx context.Context, actor string, data map[string]string) (int64, error) {
	hostID := s.hostLoose(ctx, data["address"], data["address"]) // an IP or a host name
	if hostID == 0 {
		return 0, fmt.Errorf("no host has the address %s yet — add or import it first, then apply again", data["address"])
	}
	h, err := s.inv.GetHost(ctx, hostID)
	if err != nil {
		return 0, err
	}
	port := 0
	fmt.Sscanf(data["port"], "%d", &port)
	for _, v := range h.Services {
		if v.Port == port {
			if v.Domain == data["domain"] {
				return 0, nil
			}
			if v.Domain != "" {
				return 0, fmt.Errorf("%s:%d already has the domain %s", h.Name, port, v.Domain)
			}
			v.Domain = data["domain"]
			_, err := s.inv.UpdateService(ctx, actor, v.ID, v)
			return 0, err
		}
	}
	name := strings.Split(data["domain"], ".")[0]
	created, err := s.inv.AddService(ctx, actor, hostID, inventory.Service{
		Name: name, Port: port, Protocol: "http", Domain: data["domain"],
	})
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

// Fresh drops ops that would change nothing, so a sync only proposes what
// is new. Ops it can't judge are kept.
func (s *Store) Fresh(ctx context.Context, ops []Op) []Op {
	hosts, _ := s.inv.ListHosts(ctx)
	full := map[int64]*inventory.Host{}
	for _, h := range hosts {
		if f, err := s.inv.GetHost(ctx, h.ID); err == nil {
			full[h.ID] = f
		}
	}
	domains := s.domains(ctx)
	records := map[string]bool{}
	for _, d := range domains {
		if f, err := s.inv.GetDomain(ctx, d.ID); err == nil {
			for _, r := range f.Records {
				records[d.Name+"|"+r.Name+"|"+r.Type+"|"+r.Content] = true
			}
		}
	}
	out := []Op{}
	for _, op := range ops {
		switch op.Entity {
		case "host":
			name := op.Data["name"]
			if name == "" {
				name = op.Ref
			}
			if id := s.findHost(ctx, name, op.Data["address"]); id != 0 {
				if h := full[id]; h != nil && (len(h.Addresses) > 0 || op.Data["address"] == "") {
					continue
				}
			}
		case "domain":
			found := false
			for _, d := range domains {
				found = found || d.Name == op.Ref
			}
			if found {
				continue
			}
		case "dns_record":
			if records[op.Data["domain"]+"|"+op.Data["name"]+"|"+op.Data["type"]+"|"+op.Data["content"]] {
				continue
			}
		case "address":
			id := s.hostLoose(ctx, op.Data["host"], op.Data["match"])
			if id == 0 && op.Data["optional"] != "" {
				continue // a device that isn't documented (a phone, a laptop)
			}
			if h := full[id]; h != nil {
				has := false
				for _, a := range h.Addresses {
					has = has || a.Address == op.Data["address"] ||
						(op.Data["only_if_empty"] != "" && a.Kind == orDefault(op.Data["kind"], "other"))
				}
				if has {
					continue
				}
			}
		case "cost":
			if s.costExists(ctx, orDefault(op.Data["label"], op.Ref), orDefault(op.Data["source"], "plugin")) {
				continue
			}
		case "service":
			if op.Action == "link" {
				if id := s.hostLoose(ctx, op.Data["address"], op.Data["address"]); id != 0 && full[id] != nil {
					done := false
					for _, v := range full[id].Services {
						done = done || (strconv.Itoa(v.Port) == op.Data["port"] && v.Domain == op.Data["domain"])
					}
					if done {
						continue
					}
				}
			} else if id := s.hostLoose(ctx, op.Data["host"], op.Data["host"]); id != 0 && full[id] != nil {
				has := false
				for _, v := range full[id].Services {
					has = has || strconv.Itoa(v.Port) == op.Data["port"]
				}
				if has {
					continue
				}
			}
		}
		out = append(out, op)
	}
	return out
}
