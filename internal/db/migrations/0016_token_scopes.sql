-- +goose Up
-- M9.2: add the propose scope. SQLite cannot alter a CHECK constraint, so the
-- small token table is rebuilt in place.
CREATE TABLE api_tokens_new (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('read','propose','write')),
  hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_used_at TEXT,
  revoked_at TEXT,
  public_id TEXT
);
INSERT INTO api_tokens_new(id,name,scope,hash,created_at,last_used_at,revoked_at,public_id)
  SELECT id,name,scope,hash,created_at,last_used_at,revoked_at,public_id FROM api_tokens;
DROP TABLE api_tokens;
ALTER TABLE api_tokens_new RENAME TO api_tokens;
CREATE UNIQUE INDEX api_tokens_public_id ON api_tokens(public_id);

-- +goose Down
CREATE TABLE api_tokens_old (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('read','write')),
  hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_used_at TEXT,
  revoked_at TEXT,
  public_id TEXT
);
INSERT INTO api_tokens_old(id,name,scope,hash,created_at,last_used_at,revoked_at,public_id)
  SELECT id,name,CASE WHEN scope='propose' THEN 'read' ELSE scope END,hash,created_at,last_used_at,revoked_at,public_id FROM api_tokens;
DROP TABLE api_tokens;
ALTER TABLE api_tokens_old RENAME TO api_tokens;
CREATE UNIQUE INDEX api_tokens_public_id ON api_tokens(public_id);
