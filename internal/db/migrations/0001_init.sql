-- +goose Up
CREATE TABLE locations (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, kind TEXT NOT NULL DEFAULT 'other',
  url TEXT, notes TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE TABLE hosts (
  id INTEGER PRIMARY KEY, location_id INTEGER REFERENCES locations(id), name TEXT NOT NULL, hostname TEXT,
  kind TEXT NOT NULL DEFAULT 'vm', proxmox_vmid INTEGER, os TEXT, description TEXT, icon TEXT, tags TEXT,
  lifecycle TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE TABLE host_addresses (
  id INTEGER PRIMARY KEY, host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'lan', address TEXT NOT NULL, is_primary INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE ssh_access (
  id INTEGER PRIMARY KEY, host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  user TEXT NOT NULL, port INTEGER NOT NULL DEFAULT 22, auth_methods TEXT, status TEXT NOT NULL DEFAULT 'unknown',
  last_checked_at TEXT, vault_url TEXT
);
CREATE TABLE services (
  id INTEGER PRIMARY KEY, host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE, name TEXT NOT NULL,
  type TEXT, port INTEGER, protocol TEXT NOT NULL DEFAULT 'tcp', path TEXT, domain TEXT, icon TEXT, version TEXT,
  notes TEXT, deleted_at TEXT
);
CREATE TABLE projects (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE, icon TEXT, description TEXT,
  group_name TEXT, is_open_source INTEGER NOT NULL DEFAULT 0, runbook_md TEXT, status TEXT NOT NULL DEFAULT 'active',
  deleted_at TEXT
);
CREATE TABLE api_tokens (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, scope TEXT NOT NULL CHECK (scope IN ('read','write')),
  hash TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL DEFAULT (datetime('now')), last_used_at TEXT, revoked_at TEXT
);
CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY, at TEXT NOT NULL DEFAULT (datetime('now')), actor TEXT NOT NULL, action TEXT NOT NULL,
  entity TEXT, entity_id INTEGER, diff TEXT
);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);

-- +goose Down
DROP TABLE settings; DROP TABLE audit_log; DROP TABLE api_tokens; DROP TABLE projects;
DROP TABLE services; DROP TABLE ssh_access; DROP TABLE host_addresses; DROP TABLE hosts; DROP TABLE locations;
