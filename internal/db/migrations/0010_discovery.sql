-- +goose Up
-- M7.4: discovery inbox and fingerprints.
CREATE TABLE discovered_hosts (
  id INTEGER PRIMARY KEY,
  address TEXT NOT NULL UNIQUE,
  hostname TEXT,
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','ignored','registered')),
  host_id INTEGER REFERENCES hosts(id),
  note TEXT
);
CREATE TABLE discovered_ports (
  id INTEGER PRIMARY KEY,
  address TEXT NOT NULL,
  port INTEGER NOT NULL,
  protocol TEXT NOT NULL DEFAULT 'tcp',
  service_guess TEXT,
  title TEXT,
  banner TEXT,
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  ignored INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX discovered_ports_unique ON discovered_ports(address, port, protocol);

-- +goose Down
DROP TABLE discovered_ports;
DROP TABLE discovered_hosts;
