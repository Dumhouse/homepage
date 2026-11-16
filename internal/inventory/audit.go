package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// AuditEntry is one recorded write: who did what to which entity.
type AuditEntry struct {
	ID       int64           `json:"id"`
	At       string          `json:"at"`
	Actor    string          `json:"actor"`
	Action   string          `json:"action"`
	Entity   string          `json:"entity,omitempty"`
	EntityID *int64          `json:"entity_id,omitempty"`
	Diff     json.RawMessage `json:"diff,omitempty"`
}

// ListAudit returns the newest audit rows, optionally filtered to one entity
// (and one entity id). Used for host and project timelines.
func (s *Store) ListAudit(ctx context.Context, entity string, entityID int64, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	q := `SELECT id,at,actor,action,COALESCE(entity,''),entity_id,COALESCE(diff,'null') FROM audit_log`
	var where []string
	var args []any
	if entity != "" {
		where = append(where, "entity=?")
		args = append(args, entity)
	}
	if entityID != 0 {
		where = append(where, "entity_id=?")
		args = append(args, entityID)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var id sql.NullInt64
		var diff string
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.Entity, &id, &diff); err != nil {
			return nil, err
		}
		if id.Valid {
			e.EntityID = &id.Int64
		}
		e.Diff = json.RawMessage(diff)
		out = append(out, e)
	}
	return out, rows.Err()
}
