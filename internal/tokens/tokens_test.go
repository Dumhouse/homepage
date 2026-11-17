package tokens_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/tokens"
)

func newStore(t *testing.T) *tokens.Store {
	t.Helper()
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return tokens.New(d)
}

func TestCreateVerifyRevoke(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	tok, err := s.Create(ctx, "claude-code", "write")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Plaintext == "" || !strings.HasPrefix(tok.Plaintext, "hs_"+tok.PublicID+"_") {
		t.Fatalf("plaintext format: %q", tok.Plaintext)
	}
	got, err := s.Verify(ctx, tok.Plaintext)
	if err != nil || got.Name != "claude-code" || !got.CanWrite() {
		t.Fatalf("verify: %v %+v", err, got)
	}
	if _, err := s.Verify(ctx, "Bearer "+tok.Plaintext); err != nil {
		t.Errorf("Bearer prefix: %v", err)
	}
	for _, bad := range []string{"", "nope", "hs_abc_def", tok.Plaintext + "x"} {
		if _, err := s.Verify(ctx, bad); !errors.Is(err, tokens.ErrInvalid) {
			t.Errorf("%q should be invalid, got %v", bad, err)
		}
	}
	if _, err := s.Create(ctx, "claude-code", "read"); !errors.Is(err, tokens.ErrInvalid) {
		t.Errorf("duplicate name should be invalid, got %v", err)
	}
	if _, err := s.Create(ctx, "x", "admin"); !errors.Is(err, tokens.ErrInvalid) {
		t.Errorf("bad scope should be invalid, got %v", err)
	}

	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0].Plaintext != "" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := s.Revoke(ctx, "nope"); !errors.Is(err, tokens.ErrNotFound) {
		t.Errorf("revoke unknown: %v", err)
	}
	if err := s.Revoke(ctx, "claude-code"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, tok.Plaintext); !errors.Is(err, tokens.ErrInvalid) {
		t.Errorf("revoked token should fail, got %v", err)
	}
	if err := s.Revoke(ctx, "claude-code"); !errors.Is(err, tokens.ErrNotFound) {
		t.Errorf("second revoke: %v", err)
	}
	// the name is free again after revoke
	if _, err := s.Create(ctx, "claude-code", "read"); err != nil {
		t.Errorf("recreate after revoke: %v", err)
	}
}

func TestProposeScope(t *testing.T) {
	ctx, s := context.Background(), newStore(t)
	tok, err := s.Create(ctx, "planner", "propose")
	if err != nil {
		t.Fatal(err)
	}
	if !tok.CanPropose() || tok.CanWrite() {
		t.Errorf("scope capabilities: %+v", tok)
	}
	got, err := s.Verify(ctx, tok.Plaintext)
	if err != nil || got.Scope != "propose" {
		t.Fatalf("verify: %v %+v", err, got)
	}
	if _, err := s.Create(ctx, "x", "admin"); !errors.Is(err, tokens.ErrInvalid) {
		t.Errorf("bad scope: %v", err)
	}
}
