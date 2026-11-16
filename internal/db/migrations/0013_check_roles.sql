-- +goose Up
-- A service can have a LAN and a public check of the same kind (params.role).
DROP INDEX checks_target;
CREATE UNIQUE INDEX checks_target ON checks(target_entity, target_id, kind, COALESCE(json_extract(params, '$.role'), '')) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX checks_target;
CREATE UNIQUE INDEX checks_target ON checks(target_entity, target_id, kind) WHERE deleted_at IS NULL;
