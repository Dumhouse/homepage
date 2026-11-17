# Architecture

Home Services is deliberately boring: one Go binary, one SQLite file, server-rendered HTML with a little htmx.

```
 browser ─────┐
 AI agent ────┼──▶  chi router ──▶  pages / JSON API / MCP tools ──▶  inventory.Store ──▶ SQLite
 curl / cron ─┘                                                            ▲
                          scheduler (checks) · discovery · integrations ───┘ (via changesets)
```

## Stack

| | |
|---|---|
| Language | Go, `CGO_ENABLED=0` (pure-Go SQLite driver, so cross-compiling is trivial) |
| Storage | SQLite in WAL mode, append-only SQL migrations embedded in the binary |
| HTTP | [chi](https://github.com/go-chi/chi) router |
| UI | `html/template` rendered on the server, [htmx](https://htmx.org) (vendored), small hand-written JS — **no build step, no CDN** |
| AI | the official MCP Go SDK, Streamable HTTP, JSON responses |
| Secrets | AES-GCM `secrets.enc` keyed by `HS_MASTER_KEY` — never in SQLite |

## Layout

```
cmd/homeservices      flags and CLI (token, migrate, backup, import, scan, demo)
internal/inventory    the only writer: validation, soft delete, audit, search, export/import
internal/checks       scheduler, probes (tcp/http/tls/dns/ssh), auto checks, availability, retention
internal/alerts       ntfy / webhook notifiers, dedupe and flap guard
internal/digest       weekly summary
internal/expiry       domain renewal reminders
internal/discovery    subnet scanner, fingerprints, favicon hashes, background runner
internal/facts        read-only SSH facts
internal/integrations plugin registry, instances, secret store and the plugins
internal/changesets   propose → review → apply / undo
internal/export       Homepage, Ansible, Prometheus and NetBox generators
internal/mcp          MCP tools and resources
internal/web          router, JSON API, server-rendered pages
internal/fx, rdap     exchange rates, domain registration lookups
internal/auth         none / password providers, sessions, login rate limit
internal/tokens       API and MCP tokens (SHA-256 at rest)
internal/themes       theme validation
internal/demo         the fictional example lab
templates/ static/ assets/   embedded UI files
deploy/               systemd units, install and update scripts
```

## Ground rules

1. **One writer.** HTTP handlers, MCP tools and pages all call `inventory.Store` (or a sibling store). There is no SQL in
   handlers. Every write is one transaction that also writes an `audit_log` row.
2. **Soft delete** on every user-visible table; restore is a first-class action.
3. **Migrations are append-only** (`internal/db/migrations/000N_….sql`); a shipped one is never edited.
4. **No secrets in SQLite.** Integration secrets go through the `SecretStore`; logins are links to a password manager.
5. **Agents propose, humans apply.** Plugins and `propose`-scope tokens never write the inventory directly.
6. **Everything is local.** No CDN, no analytics, no outbound calls except the probes you configured, the alert
   destinations you set, and optional public lookups (RDAP, ECB rates, favicons) you can turn off.

## Testing

```bash
make test        # go test ./... — about five seconds
```

Layers: stores on temporary SQLite files; HTTP through `httptest`; MCP through the in-memory transport; pages by
rendering and checking redirects, flashes and content; plugins against `httptest` fixtures of the upstream API. CI also runs
`go vet`, `staticcheck`, `go test -race` and `go mod tidy -diff`.

## Adding an integration

A plugin is one file in `internal/integrations/` returning a `*Plugin`:

```go
func MyThing() *Plugin {
    return &Plugin{
        ID: "mything", Name: "My Thing", Category: CatServers,
        Description: "What it reads and what you get.",
        Fields: []Field{
            {Key: "url", Label: "URL", Kind: "url", Required: true, Help: "e.g. http://192.168.1.10"},
            {Key: "token", Label: "API token", Kind: "password", Secret: true, Required: true},
        },
        Test: func(ctx context.Context, cfg Config) error { /* prove the credentials work */ },
        Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
            // read-only: return proposals, never touch the inventory
            return []Op{hostOp("nas", "192.168.1.20", "bare", "My OS 1.0", "summary", "mything")}, nil
        },
    }
}
```

1. `Sync` **must be read-only** and return `[]Op` — host / service / address / cost / facts ops (see `network.go`,
   `storage.go` for the helpers `hostOp`, `serviceOp`, `factsOp`, `leaseOp`).
2. Use `getJSON` from `httpx.go` for HTTP: it turns common failures into sentences that say what to fix.
3. Register it in `cmd/homeservices/main.go`.
4. Add a test with an `httptest` server that mimics the upstream API (copy `network_test.go`).
5. Add a line to [Integrations](integrations.md) if it needs setup beyond its field help.

Secret fields are stored encrypted per instance, so a plugin automatically supports several copies (`id~2`, `id~3`…).

## Adding a check kind, a page, an MCP tool

- **Check kind:** probe in `internal/checks/probes.go`, kind allowed in the migration's `CHECK` constraint (add a new migration).
- **Page:** a handler in `internal/web/pages_*.go`, a template in `templates/pages/`, routed in `pages.go`, plus a test that renders it.
- **MCP tool:** register in `internal/mcp`, scope it (read / propose / write) and test it over the in-memory transport.
