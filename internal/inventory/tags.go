package inventory

import (
	"context"
	"database/sql"
	"strings"
)

// Tag is a label shared by hosts and services. Tagging is by name in the API:
// sending a name that does not exist yet creates it (combobox behaviour).
type Tag struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Usage int    `json:"usage"`
}

// Group is a project group shown as a section on the projects page.
type Group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

// normalizeTags trims, lowercases and dedupes tag names.
func normalizeTags(names []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// setTags replaces an entity's tags, creating missing tag rows.
func (s *Store) setTags(ctx context.Context, tx *sql.Tx, entity string, id int64, names []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM taggings WHERE entity=? AND entity_id=?`, entity, id); err != nil {
		return err
	}
	for _, name := range normalizeTags(names) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO taggings(tag_id,entity,entity_id) SELECT id,?,? FROM tags WHERE name=?`, entity, id, name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) tagsFor(ctx context.Context, entity string, id int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.name FROM taggings tg JOIN tags t ON t.id=tg.tag_id
		 WHERE tg.entity=? AND tg.entity_id=? ORDER BY t.name COLLATE NOCASE`, entity, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListTags returns every tag with how many live hosts and services use it.
func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name, COALESCE(t.color,''), COUNT(tg.entity_id)
		FROM tags t LEFT JOIN taggings tg ON tg.tag_id=t.id
		GROUP BY t.id ORDER BY t.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Name, &t.Color, &t.Usage); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,sort FROM groups WHERE deleted_at IS NULL ORDER BY sort,name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Sort); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// resolveGroup finds or creates a group by name inside a transaction.
func resolveGroup(ctx context.Context, tx *sql.Tx, name string) (*int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE name=?`, name).Scan(&id)
	if err == nil {
		return &id, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	r, err := tx.ExecContext(ctx, `INSERT INTO groups(name) VALUES(?)`, name)
	if err != nil {
		return nil, err
	}
	id, _ = r.LastInsertId()
	return &id, nil
}

// resolveGroupID maps an API group reference (id or name) to a live group id.
func (s *Store) resolveGroupID(ctx context.Context, tx *sql.Tx, p *Project) (*int64, error) {
	if strings.TrimSpace(p.Group) != "" {
		return resolveGroup(ctx, tx, p.Group)
	}
	if p.GroupID == nil {
		return nil, nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups WHERE id=? AND deleted_at IS NULL`, *p.GroupID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, invalid("group %d does not exist", *p.GroupID)
	}
	return p.GroupID, nil
}
