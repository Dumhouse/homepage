-- +goose Up
-- M8.3: recurring/one-time costs. Domain renewals are derived from `domains`
-- on the costs page, not duplicated here.
CREATE TABLE costs (
  id INTEGER PRIMARY KEY,
  label TEXT NOT NULL,
  target_entity TEXT NOT NULL DEFAULT '',
  target_id INTEGER NOT NULL DEFAULT 0,
  project_id INTEGER REFERENCES projects(id),
  amount_cents INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'EUR',
  cycle TEXT NOT NULL DEFAULT 'monthly' CHECK (cycle IN ('monthly','yearly','one_time','estimate')),
  next_renewal TEXT,
  auto_renew INTEGER NOT NULL DEFAULT 0,
  payment_label TEXT,
  notes TEXT,
  source TEXT NOT NULL DEFAULT 'manual',
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);
CREATE INDEX costs_project ON costs(project_id);

-- +goose Down
DROP TABLE costs;
