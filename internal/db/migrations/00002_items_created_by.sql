-- +goose Up
ALTER TABLE items ADD COLUMN created_by TEXT;

-- +goose Down
ALTER TABLE items DROP COLUMN created_by;
