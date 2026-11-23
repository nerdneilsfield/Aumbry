-- +goose Up
CREATE TABLE IF NOT EXISTS directory_tokens (
    id           TEXT PRIMARY KEY,
    directory_id TEXT NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
    label        TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_directory_tokens_dir ON directory_tokens(directory_id);

-- +goose Down
DROP INDEX IF EXISTS idx_directory_tokens_dir;
DROP TABLE IF EXISTS directory_tokens;
