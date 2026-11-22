package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

// Acceptance 87: reindex rebuilds items_fts_cjk, e.g. after the table was emptied by hand, and
// --dry-run leaves it alone.
func TestReindexRebuildsCJKTable(t *testing.T) {
	ctx := context.Background()
	cfgPath, dataDir, davURL := setupMaintenanceFixture(t)
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	store, err := storage.NewWebDAV(davURL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	const id = "itemaaaaaaaa"
	if err = store.Put(ctx, "/xml/"+id+".xml", strings.NewReader(reindexXML("T", "大模型训练与评测"))); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
		VALUES (?, NULL, 'report', 'T', '2026-11-20', 's', '[]', 'a', '', '[]', ?, 'stale')`, id, "/xml/"+id+".xml"); err != nil {
		t.Fatal(err)
	}
	hits := func(phrase string) int {
		t.Helper()
		var n int
		if err = database.GetContext(ctx, &n, `SELECT count(*) FROM items_fts_cjk WHERE items_fts_cjk MATCH ?`, `"`+phrase+`"`); err != nil {
			t.Fatal(err)
		}
		return n
	}
	runCLI := func(args ...string) {
		t.Helper()
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		if err = cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	runCLI("reindex", "--dry-run")
	if hits("模型") != 0 {
		t.Fatal("--dry-run wrote items_fts_cjk")
	}
	runCLI("reindex")
	for _, phrase := range []string{"模型", "型训", "与评", "大模 模型 型训 训练"} {
		if hits(phrase) != 1 {
			t.Errorf("after reindex %q hits = %d, want 1", phrase, hits(phrase))
		}
	}

	if _, err = database.ExecContext(ctx, `DELETE FROM items_fts_cjk`); err != nil {
		t.Fatal(err)
	}
	if hits("模型") != 0 {
		t.Fatal("fixture: items_fts_cjk should be empty")
	}
	runCLI("reindex")
	runCLI("reindex") // a second run replaces the row instead of adding another
	var rows int
	if err = database.GetContext(ctx, &rows, `SELECT count(*) FROM items_fts_cjk`); err != nil || rows != 1 {
		t.Fatalf("items_fts_cjk rows = %d, %v; want 1", rows, err)
	}
	if hits("模型") != 1 {
		t.Error("reindex did not restore items_fts_cjk")
	}
}
