-- +goose Up
CREATE TABLE IF NOT EXISTS item_revisions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id          TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    rev              INTEGER NOT NULL,
    title            TEXT NOT NULL,
    summary          TEXT NOT NULL,
    report_date      TEXT NOT NULL,
    xml_storage_path TEXT NOT NULL,
    xml_sha256       TEXT NOT NULL,
    created_by       TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (item_id, rev)
);
CREATE INDEX IF NOT EXISTS idx_item_revisions_item_rev ON item_revisions(item_id, rev DESC);

CREATE TABLE IF NOT EXISTS item_revision_assets (
    revision_id INTEGER NOT NULL REFERENCES item_revisions(id) ON DELETE CASCADE,
    asset_hash  TEXT NOT NULL REFERENCES assets(hash) ON DELETE RESTRICT,
    PRIMARY KEY (revision_id, asset_hash)
);
CREATE INDEX IF NOT EXISTS idx_item_revision_assets_hash ON item_revision_assets(asset_hash);

-- Intentionally omit FK on directory_id so deleting a historical private directory
-- leaves an unknown directory_id that fails closed in dirTree.accessible.
-- was_protected records whether the directory was password-protected at snapshot time so
-- clearing that directory's password later never retroactively exposes historical revisions.
CREATE TABLE IF NOT EXISTS item_revision_mounts (
    revision_id   INTEGER NOT NULL REFERENCES item_revisions(id) ON DELETE CASCADE,
    directory_id  TEXT NOT NULL,
    was_protected INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (revision_id, directory_id)
);

-- +goose Down
DROP TABLE IF EXISTS item_revision_mounts;
DROP INDEX IF EXISTS idx_item_revision_assets_hash;
DROP TABLE IF EXISTS item_revision_assets;
DROP INDEX IF EXISTS idx_item_revisions_item_rev;
DROP TABLE IF EXISTS item_revisions;
