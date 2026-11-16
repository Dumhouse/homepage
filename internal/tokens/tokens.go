// Package tokens issues and verifies API tokens for the JSON API and MCP.
//
// A token looks like hs_<12 hex id>_<32 hex secret>. Only the SHA-256 of the
// secret is stored, so a database leak cannot be replayed. The id is public and
// lets Verify find the row without scanning.
package tokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound = errors.New("token not found")
	ErrInvalid  = errors.New("invalid token")
)

// Token is the stored view; Plaintext is only returned by Create.
type Token struct {
	ID         int64   `json:"id"`
	PublicID   string  `json:"public_id"`
	Name       string  `json:"name"`
	Scope      string  `json:"scope"` // read | write
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
	Plaintext  string  `json:"token,omitempty"`
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Create makes a token and returns it with the plaintext (shown once).
func (s *Store) Create(ctx context.Context, name, scope string) (*Token, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if scope == "" {
		scope = "read"
	}
	if scope != "read" && scope != "propose" && scope != "write" {
		return nil, fmt.Errorf("%w: scope must be read, propose or write", ErrInvalid)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens WHERE name=? AND revoked_at IS NULL`, name).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("%w: a live token named %q already exists", ErrInvalid, name)
	}
	id, err := random(6)
	if err != nil {
		return nil, err
	}
	secret, err := random(16)
	if err != nil {
		return nil, err
	}
	t := &Token{PublicID: id, Name: name, Scope: scope, Plaintext: "hs_" + id + "_" + secret}
	r, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens(name,scope,hash,public_id) VALUES(?,?,?,?)`, name, scope, hash(secret), id)
	if err != nil {
		return nil, err
	}
	t.ID, _ = r.LastInsertId()
	return t, s.fillCreated(ctx, t)
}

func (s *Store) fillCreated(ctx context.Context, t *Token) error {
	return s.db.QueryRowContext(ctx, `SELECT created_at FROM api_tokens WHERE id=?`, t.ID).Scan(&t.CreatedAt)
}

// List returns all tokens, newest first, without secrets.
func (s *Store) List(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,public_id,name,scope,created_at,last_used_at,revoked_at
		 FROM api_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		var last, revoked sql.NullString
		if err := rows.Scan(&t.ID, &t.PublicID, &t.Name, &t.Scope, &t.CreatedAt, &last, &revoked); err != nil {
			return nil, err
		}
		if last.Valid {
			t.LastUsedAt = &last.String
		}
		if revoked.Valid {
			t.RevokedAt = &revoked.String
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Revoke marks a token dead. Revoking an unknown or already dead name is ErrNotFound.
func (s *Store) Revoke(ctx context.Context, name string) error {
	r, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at=datetime('now') WHERE name=? AND revoked_at IS NULL`, strings.TrimSpace(name))
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Verify checks a plaintext token (with or without a "Bearer " prefix).
// It also bumps last_used_at at most once a minute.
func (s *Store) Verify(ctx context.Context, plaintext string) (*Token, error) {
	plaintext = strings.TrimSpace(strings.TrimPrefix(plaintext, "Bearer "))
	parts := strings.Split(plaintext, "_")
	if len(parts) != 3 || parts[0] != "hs" || len(parts[1]) != 12 || len(parts[2]) != 32 {
		return nil, ErrInvalid
	}
	t := &Token{}
	var last, revoked sql.NullString
	var stored string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,public_id,name,scope,created_at,last_used_at,revoked_at,hash
		 FROM api_tokens WHERE public_id=?`, parts[1]).
		Scan(&t.ID, &t.PublicID, &t.Name, &t.Scope, &t.CreatedAt, &last, &revoked, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	if revoked.Valid {
		return nil, ErrInvalid
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hash(parts[2]))) != 1 {
		return nil, ErrInvalid
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at=datetime('now')
		 WHERE id=? AND (last_used_at IS NULL OR last_used_at < datetime('now','-60 seconds'))`, t.ID)
	if last.Valid {
		t.LastUsedAt = &last.String
	}
	return t, nil
}

// CanWrite reports whether the token scope allows direct writes.
func (t *Token) CanWrite() bool { return t.Scope == "write" }

// CanPropose reports whether the token can queue proposed changes.
func (t *Token) CanPropose() bool { return t.Scope == "propose" || t.Scope == "write" }
