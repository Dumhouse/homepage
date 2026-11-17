# Contributing

Thanks for poking at Home Services. It is a small Go app with no JS build step; the fastest loop is
`make test` and `make run`.

## Setup

```bash
go test ./...     # ~5 s; store tests use temp SQLite files
make run          # http://localhost:8080, data in ./data
```

Optional:

```bash
./bin/homeservices demo                # seed an example inventory first (empty HS_DATA)
HS_AUTH=password HS_PASSWORD_HASH=$(./bin/homeservices hash-password) ./bin/homeservices
```

## Rules that keep the codebase sane

- **One writer.** HTTP handlers, MCP tools and pages call `inventory.Store` (or a sibling store).
  No SQL in handlers. Every write is transactional and writes `audit_log`.
- **Soft delete** on all user-visible tables; restore is a first-class action.
- **Migrations are append-only** (`000N_….sql`), never edit a shipped one.
- **No secrets in SQLite.** Integration tokens go through the SecretStore; access entries are
  Vaultwarden links.
- **No CDN.** Templates/static files are embedded; htmx is vendored.
- **Tests per layer:** store on temp SQLite, HTTP via `httptest`, MCP via the in-memory transport,
  UI by checking rendered pages/redirects. `go vet` and `go test ./...` must stay green.

## Adding a plugin

1. Add `internal/integrations/<name>.go` returning an `*integrations.Plugin`:
   `Fields` (text/password/url/bool/int, `Secret` for secrets), `Test`, `Sync`.
2. `Sync` must be **read-only** and return `[]integrations.Op` proposals — never call inventory.
3. Register it in `main` next to Caddy/Proxmox/Cloudflare.
4. Add tests with `httptest` fixtures for the upstream API.
5. Add the plugin to `docs/integrations.md` (the table is generated from the plugin list; add setup notes if it needs
   something created on the other side) and to `CHANGELOG.md`. More in
   [docs/architecture.md](docs/architecture.md#adding-an-integration).

## Adding a page

1. Add a handler in `internal/web/pages.go`, a route, and a template under `templates/pages/`.
2. Add the template name to `pageFiles` in `internal/web/ui.go`.
3. Keep forms server-rendered; use `<details>` for inline add forms unless a drawer is clearly better.
4. Add a test that covers create → render → validation error → delete/undo.

## MCP changes

Tools live in `internal/mcp` and are thin adapters over stores. Set `readOnlyHint`/`destructiveHint`
correctly, keep patches partial, and never expose secrets. Anything that writes should remain
usable by `propose_changes` for review.

## Commit style

Short imperative subject; mention task IDs like `M8.3` when they exist. One logical change per
commit, tests included.

## Docs and screenshots

- Documentation is Markdown in `docs/`; the GitHub Pages site is built from it by `python3 site/build.py`
  (needs `pip install markdown`). Open `_site/index.html` to preview.
- Screenshots and GIFs in `docs/images/` are taken from the fictional demo lab (`homeservices demo`). Never use real
  addresses, hostnames or domains in docs or tests — use `192.168.1.x`, `192.0.2.x` and `example.com`.

## Pull requests

Keep them focused, add a test, and note user-visible changes in `CHANGELOG.md`. By contributing you agree that your work is
licensed under AGPL-3.0, like the rest of the project. Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).
