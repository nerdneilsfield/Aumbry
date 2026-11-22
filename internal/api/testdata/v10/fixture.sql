-- Fixed v1.0 (commit 1e3c74c) database used by upgrade_v10_test.go.
-- Schema: the Up section of internal/db/migrations/00001_initial.sql at 1e3c74c, verbatim, plus the
-- goose_db_version table as v1.0 left it (only version 1 applied; items has no created_by column).
-- Data: a protected tree (Protected has a password, Inner below it has none), one public directory,
-- and three legacy items. The directory password is "fixture-pw".

CREATE TABLE goose_db_version (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	);
INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1), (1, 1);

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

INSERT INTO directories (id, parent_id, name, password_hash) VALUES
    ('d0root000001', NULL, 'Protected', '$2a$04$TWfqdFnvnk2eXG4UkjzydOM9w4pOjvcaaE2.y2hjeIrKHqvlXTgCC'),
    ('d0child00001', 'd0root000001', 'Inner', NULL),
    ('d0pub0000001', NULL, 'Public', NULL);

INSERT INTO items (id, external_key, type, title, report_date, summary, source_agent, xml_storage_path, plain_text) VALUES
    ('legacy000001', 'legacy:inner', 'report', 'Legacy inner', '2026-01-10', 'inner summary', 'v10-agent', '/xml/legacy000001-v10.xml', 'zebra inner'),
    ('legacy000002', 'legacy:public', 'post', 'Legacy public', '2026-01-11', 'public summary', 'v10-agent', '/xml/legacy000002-v10.xml', 'zebra public'),
    ('legacy000003', 'legacy:top', 'report', 'Legacy top', '2026-01-12', 'top summary', 'v10-agent', '/xml/legacy000003-v10.xml', 'zebra top');

INSERT INTO directory_items (directory_id, item_id) VALUES
    ('d0child00001', 'legacy000001'),
    ('d0pub0000001', 'legacy000002'),
    ('d0root000001', 'legacy000003');

INSERT INTO items_fts (item_id, title, summary, tags, plain_text) VALUES
    ('legacy000001', 'Legacy inner', 'inner summary', '', 'zebra inner'),
    ('legacy000002', 'Legacy public', 'public summary', '', 'zebra public'),
    ('legacy000003', 'Legacy top', 'top summary', '', 'zebra top');
