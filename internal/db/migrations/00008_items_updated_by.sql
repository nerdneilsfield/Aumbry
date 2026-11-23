-- +goose Up
ALTER TABLE items ADD COLUMN updated_by TEXT;

-- +goose Down
ALTER TABLE items DROP COLUMN updated_by;
