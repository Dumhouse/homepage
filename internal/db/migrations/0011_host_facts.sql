-- +goose Up
-- M8.5: read-only SSH facts, one row per host.
CREATE TABLE host_facts (
  host_id INTEGER PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
  collected_at TEXT NOT NULL DEFAULT (datetime('now')),
  os TEXT, kernel TEXT, hostname TEXT, uptime TEXT,
  disk_used_pct INTEGER NOT NULL DEFAULT 0,
  failed_units INTEGER NOT NULL DEFAULT 0,
  reboot_required TEXT, password_auth TEXT, root_login TEXT,
  key_fingerprints TEXT, error TEXT
);

-- +goose Down
DROP TABLE host_facts;
