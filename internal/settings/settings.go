// Package settings stores user preferences (home view, sidebar, tile click,
// theme JSON) server-side so they survive a new browser, with localStorage as
// the fast path in the UI.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalid = errors.New("invalid setting")

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

// Get returns the value for key, or def when it is not set.
func (s *Store) Get(ctx context.Context, key, def string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return def, nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// All returns every stored setting.
func (s *Store) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Delete removes a setting (used to reset per-domain expiry notices).
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, key)
	return err
}

// Set upserts a setting and writes an audit row.
func (s *Store) Set(ctx context.Context, actor, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("%w: key is required", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value); err != nil {
		tx.Rollback()
		return err
	}
	diff, _ := json.Marshal(map[string]string{"key": key, "value": value})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_log(actor,action,entity,entity_id,diff) VALUES(?,?,?,?,?)`,
		actor, "update", "setting", 0, string(diff)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
