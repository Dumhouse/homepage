package request

import (
	"context"
	"testing"
)

func TestActorAndScope(t *testing.T) {
	ctx := context.Background()
	if Actor(ctx) != "" || Scope(ctx) != "" {
		t.Fatal("empty context should have no actor or scope")
	}
	ctx = WithActor(ctx, "mcp:claude-code")
	ctx = WithScope(ctx, "propose")
	if Actor(ctx) != "mcp:claude-code" {
		t.Errorf("actor = %q", Actor(ctx))
	}
	if Scope(ctx) != "propose" {
		t.Errorf("scope = %q", Scope(ctx))
	}
	if CanWrite(ctx) {
		t.Error("propose must not write directly")
	}
	if !CanPropose(ctx) {
		t.Error("propose must be allowed to propose")
	}
}

func TestCanWriteMatrix(t *testing.T) {
	for _, tc := range []struct {
		scope          string
		write, propose bool
	}{
		{"", false, false},
		{"read", false, false},
		{"propose", false, true},
		{"write", true, true},
	} {
		ctx := WithScope(context.Background(), tc.scope)
		if got := CanWrite(ctx); got != tc.write {
			t.Errorf("scope %q CanWrite = %v", tc.scope, got)
		}
		if got := CanPropose(ctx); got != tc.propose {
			t.Errorf("scope %q CanPropose = %v", tc.scope, got)
		}
	}
}
