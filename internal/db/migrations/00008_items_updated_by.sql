-- +goose Up
ALTER TABLE items ADD COLUMN updated_by TEXT;
ALTER TABLE item_revisions ADD COLUMN author_verified INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE item_revisions DROP COLUMN author_verified;
ALTER TABLE items DROP COLUMN updated_by;
