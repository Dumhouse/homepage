-- +goose Up
-- The day a cost was paid and what it cost in your own currency then, so a
-- purchase in another currency doesn't change value with today's rates.
ALTER TABLE costs ADD COLUMN paid_at TEXT;
ALTER TABLE costs ADD COLUMN charged_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE costs ADD COLUMN charged_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE costs ADD COLUMN charged_note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE costs DROP COLUMN charged_note;
ALTER TABLE costs DROP COLUMN charged_currency;
ALTER TABLE costs DROP COLUMN charged_cents;
ALTER TABLE costs DROP COLUMN paid_at;
