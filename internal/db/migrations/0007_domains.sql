-- +goose Up
-- M8.2: domains + their DNS records. Registrar data is manual for now; the
-- Cloudflare/OVH plugins will fill the same fields later.

CREATE TABLE domains (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  registrar TEXT, account TEXT,
  expires_at TEXT,
  auto_renew INTEGER NOT NULL DEFAULT 0,
  price_cents INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'EUR',
  payment_label TEXT,
  nameservers TEXT,
  dnssec INTEGER NOT NULL DEFAULT 0,
  transfer_lock INTEGER NOT NULL DEFAULT 0,
  notes TEXT,
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','cloudflare','ovh')),
  created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
);

CREATE TABLE dns_records (
  id INTEGER PRIMARY KEY,
  domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
  name TEXT NOT NULL, type TEXT NOT NULL, content TEXT,
  proxied INTEGER NOT NULL DEFAULT 0,
  matched_host_id INTEGER REFERENCES hosts(id),
  stale INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT
);
CREATE INDEX dns_records_domain ON dns_records(domain_id);

-- +goose Down
DROP TABLE dns_records;
DROP TABLE domains;
