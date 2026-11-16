-- +goose Up
-- M7.1: integration plugin state. Secrets live in the SecretStore, never here.
CREATE TABLE integrations (
  plugin_id TEXT PRIMARY KEY,
  config TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_test_at TEXT,
  last_error TEXT,
  last_sync_at TEXT,
  last_ops TEXT
);

-- +goose Down
DROP TABLE integrations;
