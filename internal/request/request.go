// Package request carries per-request identity through context.Context:
// who is acting (audit actor) and what they may do (token scope).
package request

import "context"

type key int

const (
	actorKey key = iota
	scopeKey
)

// WithActor records the audit actor: "ui", "api:<token>" or "mcp:<token>".
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

// Actor returns the audit actor, or "" when the context has none.
func Actor(ctx context.Context) string {
	if v, ok := ctx.Value(actorKey).(string); ok {
		return v
	}
	return ""
}

// WithScope records "read", "propose" or "write".
func WithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, scopeKey, scope)
}

// Scope returns the token scope, or "" when unknown (UI sessions are set to write).
func Scope(ctx context.Context) string {
	if v, ok := ctx.Value(scopeKey).(string); ok {
		return v
	}
	return ""
}

// CanWrite reports whether the caller may modify data directly.
func CanWrite(ctx context.Context) bool { return Scope(ctx) == "write" }

// CanPropose reports whether the caller may queue changes for review.
func CanPropose(ctx context.Context) bool {
	s := Scope(ctx)
	return s == "propose" || s == "write"
}
