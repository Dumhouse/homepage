-- +goose Up
-- Domains can be paid in another currency; keep what it was worth when paid,
-- like costs do.
ALTER TABLE domains ADD COLUMN paid_at TEXT;
ALTER TABLE domains ADD COLUMN charged_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN charged_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN charged_note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE domains DROP COLUMN charged_note;
ALTER TABLE domains DROP COLUMN charged_currency;
ALTER TABLE domains DROP COLUMN charged_cents;
ALTER TABLE domains DROP COLUMN paid_at;
