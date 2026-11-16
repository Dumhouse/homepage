-- +goose Up
-- Electricity: one row per host that has a power figure. source is manual
-- (typed watts), ha (Home Assistant entity), shelly or tasmota (smart plug
-- address). avg_w is a running mean over the last ~24 h of readings.
CREATE TABLE host_power (
  host_id INTEGER PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT 'manual',
  ref TEXT NOT NULL DEFAULT '',
  manual_w REAL NOT NULL DEFAULT 0,
  last_w REAL NOT NULL DEFAULT 0,
  avg_w REAL NOT NULL DEFAULT 0,
  samples INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT,
  error TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE host_power;
