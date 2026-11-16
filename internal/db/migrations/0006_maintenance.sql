-- +goose Up
-- M8.1: maintenance windows suppress alerts for one target or everything.
CREATE TABLE maintenance_windows (
  id INTEGER PRIMARY KEY,
  target_entity TEXT NOT NULL DEFAULT '',
  target_id INTEGER NOT NULL DEFAULT 0,
  starts_at TEXT NOT NULL,
  ends_at TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  deleted_at TEXT
);
CREATE INDEX maintenance_active ON maintenance_windows(ends_at);

-- +goose Down
DROP TABLE maintenance_windows;
