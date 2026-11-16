package checks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaintenanceWindow silences alerts for one target (entity+id), one entity type
// (entity, id 0) or everything (entity "", id 0) between two UTC times.
type MaintenanceWindow struct {
	ID           int64  `json:"id"`
	TargetEntity string `json:"target_entity,omitempty"`
	TargetID     int64  `json:"target_id,omitempty"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
	Reason       string `json:"reason,omitempty"`
	Active       bool   `json:"active,omitempty"`
}

const timeLayout = "2006-01-02 15:04:05"

// TimeLayout is the DB/API timestamp format used by maintenance windows.
const TimeLayout = timeLayout

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{timeLayout, "2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: bad time %q", ErrInvalid, s)
}

// AddMaintenance validates and stores a window. Empty starts_at means now.
func (s *Store) AddMaintenance(ctx context.Context, actor string, w MaintenanceWindow) (*MaintenanceWindow, error) {
	if w.TargetEntity != "" {
		ok := false
		for _, e := range []string{"host", "service", "app_link", "ssh_access", "domain"} {
			if w.TargetEntity == e {
				ok = true
			}
		}
		if !ok {
			return nil, invalid("unknown target_entity %q", w.TargetEntity)
		}
	}
	start := time.Now().UTC()
	if w.StartsAt != "" {
		t, err := parseTime(w.StartsAt)
		if err != nil {
			return nil, err
		}
		start = t
	}
	end, err := parseTime(w.EndsAt)
	if err != nil {
		return nil, err
	}
	if !end.After(start) {
		return nil, invalid("ends_at must be after starts_at")
	}
	w.StartsAt = start.UTC().Format(timeLayout)
	w.EndsAt = end.UTC().Format(timeLayout)
	w.Reason = strings.TrimSpace(w.Reason)
	err = s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO maintenance_windows(target_entity,target_id,starts_at,ends_at,reason) VALUES(?,?,?,?,?)`,
			w.TargetEntity, w.TargetID, w.StartsAt, w.EndsAt, w.Reason)
		if err != nil {
			return err
		}
		w.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "maintenance", w.ID, w)
	})
	if err != nil {
		return nil, err
	}
	w.Active = start.Before(time.Now()) && end.After(time.Now())
	return &w, nil
}

// ListMaintenance returns current and upcoming windows (newest end first).
func (s *Store) ListMaintenance(ctx context.Context) ([]MaintenanceWindow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,target_entity,target_id,starts_at,ends_at,COALESCE(reason,'')
		 FROM maintenance_windows
		 WHERE deleted_at IS NULL AND ends_at >= datetime('now','-1 hour')
		 ORDER BY starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	out := []MaintenanceWindow{}
	for rows.Next() {
		var w MaintenanceWindow
		if err := rows.Scan(&w.ID, &w.TargetEntity, &w.TargetID, &w.StartsAt, &w.EndsAt, &w.Reason); err != nil {
			return nil, err
		}
		start, _ := parseTime(w.StartsAt)
		end, _ := parseTime(w.EndsAt)
		w.Active = !start.After(now) && end.After(now)
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteMaintenance soft-deletes a window.
func (s *Store) DeleteMaintenance(ctx context.Context, actor string, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`UPDATE maintenance_windows SET deleted_at=datetime('now') WHERE id=? AND deleted_at IS NULL`, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, "delete", "maintenance", id, nil)
	})
}

// InMaintenance reports whether alerts for this target are currently silenced.
func (s *Store) InMaintenance(ctx context.Context, entity string, id int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM maintenance_windows
		 WHERE deleted_at IS NULL
		   AND starts_at <= datetime('now') AND ends_at >= datetime('now')
		   AND (target_entity='' OR (target_entity=? AND (target_id=0 OR target_id=?)))`,
		entity, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return n > 0, err
}
