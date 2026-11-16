package inventory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Location groups hosts by where they physically live (a Proxmox node, a VPS
// provider, a shelf). Hosts point at one location; the location page lists them.
type Location struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // proxmox | vps | dedicated | cloud | bare-metal | other
	URL       string `json:"url,omitempty"`
	Notes     string `json:"notes,omitempty"`
	HostCount int    `json:"host_count"`
}

// LocationRef is the compact form embedded in a Host.
type LocationRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (l *Location) normalize() error {
	l.Name = strings.TrimSpace(l.Name)
	if l.Name == "" {
		return invalid("name is required")
	}
	var err error
	if l.Kind, err = oneOf(l.Kind, "other", "proxmox", "vps", "dedicated", "cloud", "bare-metal", "other"); err != nil {
		return err
	}
	l.URL = strings.TrimSpace(l.URL)
	if l.URL != "" && !isHTTPURL(l.URL) {
		return invalid("url must be absolute http(s)")
	}
	return nil
}

func (s *Store) CreateLocation(ctx context.Context, actor string, l Location) (*Location, error) {
	if err := l.normalize(); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO locations(name,kind,url,notes) VALUES(?,?,?,?)`, l.Name, l.Kind, l.URL, l.Notes)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("location %q already exists", l.Name)
			}
			return err
		}
		l.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "location", l.ID, l)
	})
	if err != nil {
		return nil, err
	}
	return s.GetLocation(ctx, l.ID)
}

func (s *Store) GetLocation(ctx context.Context, id int64) (*Location, error) {
	l := &Location{}
	var url, notes sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,kind,url,notes FROM locations WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&l.ID, &l.Name, &l.Kind, &url, &notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	l.URL, l.Notes = url.String, notes.String
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM hosts WHERE location_id=? AND deleted_at IS NULL`, id).Scan(&l.HostCount); err != nil {
		return nil, err
	}
	return l, nil
}

// ListLocations returns locations ordered by name, each with its live host count.
func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM locations WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE`)
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
	out := []Location{}
	for _, id := range ids {
		l, err := s.GetLocation(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (s *Store) UpdateLocation(ctx context.Context, actor string, id int64, l Location) (*Location, error) {
	if _, err := s.GetLocation(ctx, id); err != nil {
		return nil, err
	}
	if err := l.normalize(); err != nil {
		return nil, err
	}
	l.ID = id
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE locations SET name=?,kind=?,url=?,notes=? WHERE id=? AND deleted_at IS NULL`,
			l.Name, l.Kind, l.URL, l.Notes, id)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("location %q already exists", l.Name)
			}
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, "update", "location", id, l)
	})
	if err != nil {
		return nil, err
	}
	return s.GetLocation(ctx, id)
}

// DeleteLocation refuses to remove a location that still has live hosts.
func (s *Store) DeleteLocation(ctx context.Context, actor string, id int64) error {
	l, err := s.GetLocation(ctx, id)
	if err != nil {
		return err
	}
	if l.HostCount > 0 {
		return invalid("%d host%s use%s it — move them first", l.HostCount, plural(l.HostCount), pluralVerb(l.HostCount))
	}
	return s.softDelete(ctx, actor, "locations", "location", id, true)
}

func (s *Store) RestoreLocation(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "locations", "location", id, false)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralVerb(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

// locationOf loads the live location for a host, or nil when unset/deleted.
func (s *Store) locationOf(ctx context.Context, id *int64) (*LocationRef, error) {
	if id == nil {
		return nil, nil
	}
	var l LocationRef
	err := s.db.QueryRowContext(ctx, `SELECT id,name FROM locations WHERE id=? AND deleted_at IS NULL`, *id).Scan(&l.ID, &l.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// checkLocation makes sure a referenced location exists and is live.
func (s *Store) checkLocation(ctx context.Context, id *int64) error {
	if id == nil {
		return nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE id=? AND deleted_at IS NULL`, *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("location %d does not exist", *id)
	}
	return nil
}

// EnsureLocation returns the id of the named live location, creating it when new.
func (s *Store) EnsureLocation(ctx context.Context, actor, name string) (*int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	if id := s.locationByName(ctx, name); id != 0 {
		return &id, nil
	}
	l, err := s.CreateLocation(ctx, actor, Location{Name: name})
	if err != nil {
		return nil, err
	}
	return &l.ID, nil
}

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE")
}
