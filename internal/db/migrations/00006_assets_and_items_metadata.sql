-- +goose Up
ALTER TABLE assets ADD COLUMN duration_seconds INTEGER;
ALTER TABLE assets ADD COLUMN page_count INTEGER;

ALTER TABLE items ADD COLUMN primary_audio_hash TEXT;
ALTER TABLE items ADD COLUMN primary_audio_chapters_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE items ADD COLUMN last_rev INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE items DROP COLUMN last_rev;
ALTER TABLE items DROP COLUMN primary_audio_chapters_json;
ALTER TABLE items DROP COLUMN primary_audio_hash;
ALTER TABLE assets DROP COLUMN page_count;
ALTER TABLE assets DROP COLUMN duration_seconds;
