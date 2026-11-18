# Security

Home Services is designed for a trusted home network. Read this before exposing it to the internet.

## Report a vulnerability

Open a private security advisory on the repository, or email the maintainer listed in the repo
profile. Please do not file a public issue for an exploitable bug. We aim to reply within a week.

## Threat model

- **Trusted LAN by default.** `HS_AUTH=none` lets anyone who can reach the port use the UI.
  If the app is reachable from Tailscale, the internet or a shared network, set
  `HS_AUTH=password` (and `HS_COOKIE_SECURE=1` behind HTTPS), or `HS_AUTH=proxy` to let your
  single-sign-on proxy decide. Proxy mode refuses to start without `HS_TRUSTED_PROXIES` and returns
  403 to any client that is not one of those proxies, so the user header cannot be forged directly.
- **API/MCP tokens are mandatory** for `/mcp` and `/api/v1`, independent of UI auth. Tokens are
  random 128-bit secrets stored as SHA-256; `read` tokens cannot write. Prefer one token per agent
  and revoke unused ones from Settings › MCP.
- **No passwords in the database.** Access entries store method, account and a Vaultwarden item
  URL. Integration secrets live in `data/secrets.enc`, AES-GCM encrypted with `HS_MASTER_KEY`;
  without the key they are kept in memory only.
- **Outbound requests** happen only for: checks (TCP/HTTP/TLS/DNS/SSH), ntfy/webhook alerts,
  favicon fetching (private/LAN targets refused unless enabled), integration plugin APIs and the
  subnet scanner (TCP connect + banner/title only), and — only if you switch it on — a daily
  request to GitHub's public "latest release" endpoint. No inbound tunnel is created.
- **SSH facts** use a read-only user and a key at `HS_SSH_KEY`, with `InsecureIgnoreHostKey` for
  LAN convenience. Only fixed read-only commands are executed.
- **Import/apply is reviewed.** Agent and plugin proposals go through the changeset queue; applying
  is a human action and undo soft-deletes what the set created.
- **Deleting is soft** everywhere; Settings › Trash (when built) is the only hard delete.

## Hardening checklist

- Run behind Caddy/nginx with TLS if it leaves the LAN; set `HS_COOKIE_SECURE=1`.
- Behind a reverse proxy, set `HS_TRUSTED_PROXIES` to the proxy addresses so the login limiter sees real clients.
  Forwarding headers are ignored unless the direct peer is in that list, which prevents address spoofing.
- Use `HS_AUTH=password` and a long random password; rotate `HS_SESSION_KEY` to sign out everyone.
- Give each agent a separate token; use `read` unless it must write; revoke after a test run.
- Keep `/etc/homeservices/env` mode 0600 and back it up with the database.
- Allow only the ports you need at the firewall; the app itself listens on one port.
- Review `/problems` and the audit timeline after giving an agent write access.

## Known limitations

- CSRF relies on SameSite cookies, not per-form tokens.
- One shared UI password; no users/roles or MFA in password mode (use `HS_AUTH=proxy` with an SSO
  proxy for per-user sign-in and MFA).
- Login is rate-limited in memory (five failures per IP in five minutes, then `429` with `Retry-After`).
  The JSON API and MCP are not rate-limited; put a reverse proxy in front if exposed.
- The Docker image and example configs are not hardened for hostile multi-tenant hosts.
