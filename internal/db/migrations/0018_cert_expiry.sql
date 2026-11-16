-- +goose Up
-- Latest TLS certificate expiry seen by a tls check, for the calendar/alerts.
ALTER TABLE checks ADD COLUMN cert_days INTEGER NOT NULL DEFAULT -1;
ALTER TABLE checks ADD COLUMN cert_issuer TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE checks DROP COLUMN cert_issuer;
ALTER TABLE checks DROP COLUMN cert_days;
