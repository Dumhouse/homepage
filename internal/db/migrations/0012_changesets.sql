-- +goose Up
-- M9: proposed changes from agents/plugins, reviewed before applying.
CREATE TABLE changesets (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'mcp',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','rejected','undone')),
  actor TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  applied_at TEXT
);
CREATE TABLE changeset_ops (
  id INTEGER PRIMARY KEY,
  changeset_id INTEGER NOT NULL REFERENCES changesets(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  entity TEXT NOT NULL,
  ref TEXT,
  data TEXT NOT NULL DEFAULT '{}',
  note TEXT,
  applied_entity_id INTEGER,
  error TEXT
);

-- +goose Down
DROP TABLE changeset_ops;
DROP TABLE changesets;
