-- +goose Up
-- IF NOT EXISTS keeps this a no-op for databases created before goose was adopted.
CREATE TABLE IF NOT EXISTS directories (
    id TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES directories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    password_hash TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_directories_parent ON directories(parent_id);

CREATE TABLE IF NOT EXISTS items (
    id TEXT PRIMARY KEY,
    external_key TEXT UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('report', 'post')),
    title TEXT NOT NULL,
    report_date TEXT NOT NULL,
    summary TEXT NOT NULL,
    tags_json TEXT NOT NULL DEFAULT '[]',
    source_agent TEXT NOT NULL,
    canonical_url TEXT NOT NULL DEFAULT '',
    related_ids_json TEXT NOT NULL DEFAULT '[]',
    xml_storage_path TEXT NOT NULL,
    plain_text TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_items_date ON items(report_date DESC);
CREATE INDEX IF NOT EXISTS idx_items_type ON items(type);

CREATE TABLE IF NOT EXISTS directory_items (
    directory_id TEXT NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
    item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    mounted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (directory_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_dir_items_item ON directory_items(item_id);

CREATE TABLE IF NOT EXISTS assets (
    hash TEXT PRIMARY KEY,
    mime_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    storage_path TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS item_assets (
    item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    asset_hash TEXT NOT NULL REFERENCES assets(hash) ON DELETE RESTRICT,
    PRIMARY KEY (item_id, asset_hash)
);
CREATE INDEX IF NOT EXISTS idx_item_assets_hash ON item_assets(asset_hash);

CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
    item_id UNINDEXED, title, summary, tags, plain_text, tokenize = 'trigram'
);

-- +goose Down
DROP TABLE IF EXISTS items_fts;
DROP TABLE IF EXISTS item_assets;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS directory_items;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS directories;
