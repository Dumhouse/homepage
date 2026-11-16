-- +goose Up
-- M5.1: checks, raw results and hourly rollups.

CREATE TABLE checks (
  id INTEGER PRIMARY KEY,
  target_entity TEXT NOT NULL CHECK (target_entity IN ('host','service','app_link','ssh_access','domain')),
  target_id INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('tcp','http','tls','ssh','dns')),
  interval_s INTEGER NOT NULL DEFAULT 300,
  params TEXT NOT NULL DEFAULT '{}',
  enabled INTEGER NOT NULL DEFAULT 1,
  auto INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'unknown',
  fail_count INTEGER NOT NULL DEFAULT 0,
  last_ok_at TEXT, last_run_at TEXT, last_error TEXT, last_latency_ms INTEGER,
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE UNIQUE INDEX checks_target ON checks(target_entity, target_id, kind) WHERE deleted_at IS NULL;
CREATE INDEX checks_due ON checks(enabled, last_run_at);

CREATE TABLE check_results (
  id INTEGER PRIMARY KEY,
  check_id INTEGER NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
  at TEXT NOT NULL DEFAULT (datetime('now')),
  ok INTEGER NOT NULL,
  latency_ms INTEGER,
  message TEXT
);
CREATE INDEX check_results_check ON check_results(check_id, at);

CREATE TABLE check_rollups (
  check_id INTEGER NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
  hour TEXT NOT NULL,
  ok_count INTEGER NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  avg_ms INTEGER,
  PRIMARY KEY (check_id, hour)
);

-- +goose Down
DROP TABLE check_rollups;
DROP TABLE check_results;
DROP TABLE checks;
