// Package db opens SQLite and applies the initial schema.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open initializes the database in dataDir and applies schema migrations.
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dataDir, "aumbry.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)

	if err := migrate(context.Background(), database); err != nil {
		return nil, errors.Join(err, database.Close())
	}
	return database, nil
}

func migrate(ctx context.Context, database *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, statement := range pragmas {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply SQLite setting %q: %w", statement, err)
		}
	}

	const migration = `
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
);`
	if _, err := database.ExecContext(ctx, migration); err != nil {
		return fmt.Errorf("apply initial SQLite migration: %w", err)
	}
	return nil
}
