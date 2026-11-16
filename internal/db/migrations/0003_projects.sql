-- +goose Up
-- M2.7–M2.11: project links, environments + app links, access, project links to
-- hosts/services, and outside services a project depends on.

CREATE TABLE project_links (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind TEXT NOT NULL DEFAULT 'other' CHECK (kind IN ('repo','docs','public','secrets','other')),
  url TEXT NOT NULL, label TEXT, sort INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT
);
CREATE INDEX project_links_project ON project_links(project_id);

CREATE TABLE project_envs (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL, base_url TEXT, sort INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT,
  UNIQUE (project_id, name)
);

CREATE TABLE access (
  id INTEGER PRIMARY KEY,
  target_entity TEXT NOT NULL CHECK (target_entity IN ('host','service','project','domain','provider','other')),
  target_id INTEGER NOT NULL,
  label TEXT NOT NULL, url TEXT,
  method TEXT NOT NULL DEFAULT 'none' CHECK (method IN ('password','google_sso','github_sso','oidc','ssh_key','token','passkey','none')),
  account TEXT, twofa TEXT NOT NULL DEFAULT 'none' CHECK (twofa IN ('totp','hardware','sms','provider','none')),
  vault_item_url TEXT, notes TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE INDEX access_target ON access(target_entity, target_id);

CREATE TABLE app_links (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  label TEXT NOT NULL, path TEXT, url TEXT, only_env TEXT,
  kind TEXT NOT NULL DEFAULT 'page' CHECK (kind IN ('page','admin','jobs','db','docs','health','mcp','other')),
  icon TEXT, access_id INTEGER REFERENCES access(id), health_check INTEGER NOT NULL DEFAULT 0,
  sort INTEGER NOT NULL DEFAULT 0, deleted_at TEXT
);
CREATE INDEX app_links_project ON app_links(project_id);

CREATE TABLE project_hosts (
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  env TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project_id, host_id, env)
);

CREATE TABLE project_services (
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  service_id INTEGER NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  env TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project_id, service_id, env)
);

CREATE TABLE project_connections (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  target_kind TEXT NOT NULL CHECK (target_kind IN ('host','service','provider','url')),
  target_id INTEGER, url TEXT,
  role TEXT NOT NULL DEFAULT 'other',
  detail TEXT, impact_note TEXT,
  access_id INTEGER REFERENCES access(id),
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','detected')),
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE INDEX project_connections_target ON project_connections(target_kind, target_id);
CREATE INDEX project_connections_project ON project_connections(project_id);

-- +goose Down
DROP TABLE project_connections;
DROP TABLE project_services;
DROP TABLE project_hosts;
DROP TABLE access;
DROP TABLE app_links;
DROP TABLE project_envs;
DROP TABLE project_links;
