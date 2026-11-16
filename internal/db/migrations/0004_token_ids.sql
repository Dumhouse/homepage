-- +goose Up
ALTER TABLE api_tokens ADD COLUMN public_id TEXT;
CREATE UNIQUE INDEX api_tokens_public_id ON api_tokens(public_id);

-- +goose Down
DROP INDEX api_tokens_public_id;
ALTER TABLE api_tokens DROP COLUMN public_id;
