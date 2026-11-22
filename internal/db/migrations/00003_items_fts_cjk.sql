-- +goose Up
-- Bigram-expanded copy of items_fts for Chinese, Japanese and Korean text (see internal/cjk).
-- Existing rows are not migrated; `aumbry-server reindex` fills the table.
CREATE VIRTUAL TABLE IF NOT EXISTS items_fts_cjk USING fts5(
    item_id UNINDEXED, title, summary, tags, plain_text, tokenize = 'unicode61'
);

-- +goose Down
DROP TABLE IF EXISTS items_fts_cjk;
