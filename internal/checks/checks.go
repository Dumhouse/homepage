// Package checks stores health checks, runs probes and keeps the state machine.
// Auto-created checks are reconciled from the inventory, so new services and SSH
// logins start being watched without extra bookkeeping.
package checks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound = errors.New("check not found")
	ErrInvalid  = errors.New("invalid check")
)

type Check struct {
	ID            int64          `json:"id"`
	TargetEntity  string         `json:"target_entity"`
	TargetID      int64          `json:"target_id"`
	Kind          string         `json:"kind"`
	IntervalS     int            `json:"interval_s"`
	Params        map[string]any `json:"params"`
	Enabled       bool           `json:"enabled"`
	Auto          bool           `json:"auto"`
	State         string         `json:"state"`
	FailCount     int            `json:"fail_count"`
	LastOKAt      *string        `json:"last_ok_at,omitempty"`
	LastRunAt     *string        `json:"last_run_at,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	LastLatencyMS int            `json:"last_latency_ms,omitempty"`
	CertDays      int            `json:"cert_days,omitempty"`
	CertIssuer    string         `json:"cert_issuer,omitempty"`
	CreatedAt     string         `json:"created_at"`
}

type Result struct {
	OK         bool
	LatencyMS  int
	Message    string
	SSHStatus  string // key_only | password_allowed | unreachable, for ssh checks
	CertDays   int    // TLS checks
	CertIssuer string
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func (c *Check) normalize() error {
	var err error
	if c.TargetEntity, err = oneOf(c.TargetEntity, "host", "host", "service", "app_link", "ssh_access", "domain"); err != nil {
		return err
	}
	if c.TargetID == 0 {
		return invalid("target_id is required")
	}
	if c.Kind, err = oneOf(c.Kind, "tcp", "tcp", "http", "tls", "ssh", "dns"); err != nil {
		return err
	}
	if c.IntervalS == 0 {
		c.IntervalS = 300
	}
	if c.IntervalS < 30 {
		return invalid("interval_s must be at least 30")
	}
	if c.Params == nil {
		c.Params = map[string]any{}
	}
	if !c.Enabled {
		c.Enabled = true
	}
	return nil
}

func oneOf(v, def string, allowed ...string) (string, error) {
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

func marshalParams(p map[string]any) string {
	b, _ := json.Marshal(p)
	return string(b)
}

func unmarshalParams(s string) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func (s *Store) Create(ctx context.Context, actor string, c Check) (*Check, error) {
	if err := c.normalize(); err != nil {
		return nil, err
	}
	if c.State == "" {
		c.State = "unknown"
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO checks(target_entity,target_id,kind,interval_s,params,enabled,auto,state)
			 VALUES(?,?,?,?,?,?,?,?)`,
			c.TargetEntity, c.TargetID, c.Kind, c.IntervalS, marshalParams(c.Params), c.Enabled, c.Auto, c.State)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return invalid("a %s check already exists for %s #%d", c.Kind, c.TargetEntity, c.TargetID)
			}
			return err
		}
		c.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "check", c.ID, c)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, c.ID)
}

func (s *Store) Get(ctx context.Context, id int64) (*Check, error) {
	c := &Check{}
	var params string
	var enabled, auto int
	var lastOK, lastRun sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,target_entity,target_id,kind,interval_s,params,enabled,auto,state,fail_count,
		        last_ok_at,last_run_at,COALESCE(last_error,''),COALESCE(last_latency_ms,0),
		        cert_days,COALESCE(cert_issuer,''),created_at
		 FROM checks WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&c.ID, &c.TargetEntity, &c.TargetID, &c.Kind, &c.IntervalS, &params, &enabled, &auto,
			&c.State, &c.FailCount, &lastOK, &lastRun, &c.LastError, &c.LastLatencyMS,
			&c.CertDays, &c.CertIssuer, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	c.Params = unmarshalParams(params)
	c.Enabled = enabled == 1
	c.Auto = auto == 1
	if lastOK.Valid {
		c.LastOKAt = &lastOK.String
	}
	if lastRun.Valid {
		c.LastRunAt = &lastRun.String
	}
	return c, nil
}

// List returns checks for one target, or all when entity is empty.
func (s *Store) List(ctx context.Context, entity string, id int64) ([]Check, error) {
	q := `SELECT id FROM checks WHERE deleted_at IS NULL`
	var args []any
	if entity != "" {
		q += ` AND target_entity=?`
		args = append(args, entity)
	}
	if id != 0 {
		q += ` AND target_id=?`
		args = append(args, id)
	}
	q += ` ORDER BY kind`
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []Check{}
	for _, id := range ids {
		c, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// Due returns enabled checks whose interval has elapsed.
func (s *Store) Due(ctx context.Context, limit int) ([]Check, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM checks
		 WHERE enabled=1 AND deleted_at IS NULL
		   AND (last_run_at IS NULL OR last_run_at <= datetime('now', '-' || interval_s || ' seconds'))
		 ORDER BY COALESCE(last_run_at,'') LIMIT ?`, limit)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []Check{}
	for _, id := range ids {
		c, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, id, true)
}
func (s *Store) Restore(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, id, false)
}

func (s *Store) softDelete(ctx context.Context, actor string, id int64, del bool) error {
	q := `UPDATE checks SET deleted_at=datetime('now') WHERE id=? AND deleted_at IS NULL`
	action := "delete"
	if !del {
		q = `UPDATE checks SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`
		action = "restore"
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, q, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, action, "check", id, nil)
	})
}

// SetEnabled pauses/resumes a check.
func (s *Store) SetEnabled(ctx context.Context, actor string, id int64, enabled bool) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE checks SET enabled=? WHERE id=? AND deleted_at IS NULL`, enabled, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		action := "disable"
		if enabled {
			action = "enable"
		}
		return s.audit(ctx, tx, actor, action, "check", id, nil)
	})
}

// Record stores one result and advances the state machine:
// up → failing(n) after 1–2 failures → down after 3; down → recovering → up.
func (s *Store) Record(ctx context.Context, c Check, res Result) error {
	state, fails := nextState(c.State, res.OK, c.FailCount)
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO check_results(check_id,ok,latency_ms,message) VALUES(?,?,?,?)`,
			c.ID, res.OK, res.LatencyMS, res.Message); err != nil {
			return err
		}
		cert := ""
		var certArgs []any
		if c.Kind == "tls" {
			cert = `, cert_days=?, cert_issuer=?`
			certArgs = []any{res.CertDays, res.CertIssuer}
		}
		if res.OK {
			args := append([]any{state, fails, res.LatencyMS}, certArgs...)
			args = append(args, c.ID)
			if _, err := tx.ExecContext(ctx,
				`UPDATE checks SET state=?, fail_count=?, last_run_at=datetime('now'),
				        last_ok_at=datetime('now'), last_latency_ms=?, last_error=''`+cert+`
				 WHERE id=?`, args...); err != nil {
				return err
			}
		} else {
			args := append([]any{state, fails, res.LatencyMS, res.Message}, certArgs...)
			args = append(args, c.ID)
			if _, err := tx.ExecContext(ctx,
				`UPDATE checks SET state=?, fail_count=?, last_run_at=datetime('now'),
				        last_latency_ms=?, last_error=?`+cert+`
				 WHERE id=?`, args...); err != nil {
				return err
			}
		}
		// Raw results are the history; auditing every interval would flood the log.
		return nil
	})
}

// Certificates returns TLS checks with a known certificate expiry within days.
func (s *Store) Certificates(ctx context.Context, days int) ([]Check, error) {
	list, err := s.List(ctx, "", 0)
	if err != nil {
		return nil, err
	}
	out := []Check{}
	for _, c := range list {
		if c.Kind != "tls" || c.CertDays < 0 || c.CertDays > days {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// SetSSHStatus mirrors an SSH probe result onto ssh_access.status.
func (s *Store) SetSSHStatus(ctx context.Context, sshID int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE ssh_access SET status=?, last_checked_at=datetime('now') WHERE id=?`, status, sshID)
	return err
}

func nextState(prev string, ok bool, fails int) (string, int) {
	if ok {
		if prev == "down" {
			return "recovering", 0
		}
		return "up", 0
	}
	n := fails + 1
	if n >= 3 {
		return "down", n
	}
	return "failing", n
}

// Result is one row of history for the timeline.
type ResultRow struct {
	At        string `json:"at"`
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latency_ms,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Recent returns the newest results for a check.
func (s *Store) Recent(ctx context.Context, checkID int64, limit int) ([]ResultRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT at,ok,COALESCE(latency_ms,0),COALESCE(message,'') FROM check_results
		 WHERE check_id=? ORDER BY id DESC LIMIT ?`, checkID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResultRow{}
	for rows.Next() {
		var r ResultRow
		var ok int
		if err := rows.Scan(&r.At, &ok, &r.LatencyMS, &r.Message); err != nil {
			return nil, err
		}
		r.OK = ok == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Problem is a failing/down check with enough target detail for a list.
type Problem struct {
	CheckID      int64  `json:"check_id"`
	TargetEntity string `json:"target_entity"`
	TargetID     int64  `json:"target_id"`
	Title        string `json:"title,omitempty"`
	Kind         string `json:"kind"`
	State        string `json:"state"`
	LastError    string `json:"last_error,omitempty"`
	LastRunAt    string `json:"last_run_at,omitempty"`
}

// ListProblems returns failing/down checks, newest first.
func (s *Store) ListProblems(ctx context.Context, limit int) ([]Problem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,target_entity,target_id,kind,state,COALESCE(last_error,''),COALESCE(last_run_at,'')
		 FROM checks WHERE deleted_at IS NULL AND enabled=1 AND state IN ('failing','down')
		 ORDER BY CASE state WHEN 'down' THEN 0 ELSE 1 END, last_run_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Problem{}
	for rows.Next() {
		var p Problem
		if err := rows.Scan(&p.CheckID, &p.TargetEntity, &p.TargetID, &p.Kind, &p.State, &p.LastError, &p.LastRunAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Counts summarises enabled checks by state (plus paused checks).
type Counts struct {
	Up, Failing, Down, Recovering, Unknown, Disabled int
}

func (s *Store) CheckCounts(ctx context.Context) (Counts, error) {
	var c Counts
	rows, err := s.db.QueryContext(ctx,
		`SELECT state, enabled, COUNT(*) FROM checks WHERE deleted_at IS NULL GROUP BY state, enabled`)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var enabled, n int
		if err := rows.Scan(&state, &enabled, &n); err != nil {
			return c, err
		}
		if enabled == 0 {
			c.Disabled += n
			continue
		}
		switch state {
		case "up":
			c.Up += n
		case "failing":
			c.Failing += n
		case "down":
			c.Down += n
		case "recovering":
			c.Recovering += n
		default:
			c.Unknown += n
		}
	}
	return c, rows.Err()
}

// Prune rolls raw results older than 30 days into hourly buckets and drops
// rollups older than a year.
func (s *Store) Prune(ctx context.Context) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO check_rollups(check_id,hour,ok_count,fail_count,avg_ms)
			SELECT check_id, strftime('%Y-%m-%dT%H:00', at) AS hour, SUM(ok), SUM(1-ok), CAST(AVG(latency_ms) AS INTEGER)
			FROM check_results WHERE at < datetime('now','-30 days')
			GROUP BY check_id, hour`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM check_results WHERE at < datetime('now','-30 days')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM check_rollups WHERE hour < strftime('%Y-%m-%dT%H:00', datetime('now','-365 days'))`)
		return err
	})
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
