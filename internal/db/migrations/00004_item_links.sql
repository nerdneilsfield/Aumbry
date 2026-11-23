-- +goose Up
CREATE TABLE IF NOT EXISTS item_links (
    source_item_id TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    target_item_id TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK (kind IN ('news_detail', 'paper_related')),
    PRIMARY KEY (source_item_id, target_item_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_item_links_target ON item_links(target_item_id, source_item_id);

-- +goose Down
DROP INDEX IF EXISTS idx_item_links_target;
DROP TABLE IF EXISTS item_links;
