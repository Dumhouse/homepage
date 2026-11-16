-- +goose Up
-- M2.5/M2.6: real groups and tags, hosts' comma list migrated into taggings.

CREATE TABLE groups (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  sort INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT
);

CREATE TABLE tags (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  color TEXT
);

CREATE TABLE taggings (
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  entity TEXT NOT NULL,
  entity_id INTEGER NOT NULL,
  PRIMARY KEY (tag_id, entity, entity_id)
);
CREATE INDEX taggings_entity ON taggings(entity, entity_id);

ALTER TABLE projects ADD COLUMN group_id INTEGER REFERENCES groups(id);

INSERT INTO groups(name)
  SELECT DISTINCT group_name FROM projects
  WHERE group_name IS NOT NULL AND trim(group_name) <> '';
UPDATE projects SET group_id = (SELECT id FROM groups WHERE groups.name = projects.group_name)
  WHERE group_name IS NOT NULL AND trim(group_name) <> '';

INSERT OR IGNORE INTO tags(name)
  SELECT DISTINCT trim(j.value)
  FROM hosts h, json_each('["' || replace(h.tags, ',', '","') || '"]') j
  WHERE h.tags IS NOT NULL AND trim(h.tags) <> '';
INSERT OR IGNORE INTO taggings(tag_id, entity, entity_id)
  SELECT t.id, 'host', h.id
  FROM hosts h, json_each('["' || replace(h.tags, ',', '","') || '"]') j
  JOIN tags t ON t.name = trim(j.value)
  WHERE h.tags IS NOT NULL AND trim(h.tags) <> '';

ALTER TABLE hosts DROP COLUMN tags;
ALTER TABLE projects DROP COLUMN group_name;

-- +goose Down
ALTER TABLE projects ADD COLUMN group_name TEXT;
UPDATE projects SET group_name = (SELECT name FROM groups WHERE groups.id = projects.group_id);
ALTER TABLE hosts ADD COLUMN tags TEXT;
UPDATE hosts SET tags = (
  SELECT group_concat(t.name, ',') FROM taggings tg JOIN tags t ON t.id = tg.tag_id
  WHERE tg.entity = 'host' AND tg.entity_id = hosts.id
);
ALTER TABLE projects DROP COLUMN group_id;
DROP TABLE taggings;
DROP TABLE tags;
DROP TABLE groups;
