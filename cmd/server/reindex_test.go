package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func reindexXML(title, text string) string {
	// The figure src is the rewritten form kept in storage, which upload validation would reject.
	return `<document version="1.1"><meta><title>` + title + `</title><type>report</type><date>2026-11-20</date>` +
		`<summary>sum</summary><source-agent>a</source-agent><tags><tag>tagone</tag></tags></meta><body>` +
		`<section title="Sec"><md><![CDATA[` + text + `]]></md>` +
		`<figure src="/api/v1/assets/` + strings.Repeat("a", 64) + `"/></section></body></document>`
}

func TestReindexCommand(t *testing.T) {
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

	// addItem stores an item whose plain_text and items_fts row are deliberately wrong.
	addItem := func(id, xmlBody, stale string) {
		t.Helper()
		xmlPath := "/xml/" + id + ".xml"
		if xmlBody != "" {
			if err = store.Put(ctx, xmlPath, strings.NewReader(xmlBody)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = database.ExecContext(ctx, `INSERT INTO items
			(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
			VALUES (?, NULL, 'report', 'T', '2026-11-20', 's', '[]', 'a', '', '[]', ?, ?)`, id, xmlPath, stale); err != nil {
			t.Fatal(err)
		}
		if _, err = database.ExecContext(ctx, `INSERT INTO items_fts (item_id, title, summary, tags, plain_text) VALUES (?, 'T', 's', '', ?)`, id, stale); err != nil {
			t.Fatal(err)
		}
	}
	// ftsHits is the long-term (FTS) search path and likeHits the short-term (plain_text LIKE) path of internal/api/search.go.
	ftsHits := func(term string) int {
		t.Helper()
		var n int
		if err = database.GetContext(ctx, &n, `SELECT count(*) FROM items_fts WHERE items_fts MATCH ?`, `"`+term+`"`); err != nil {
			t.Fatal(err)
		}
		return n
	}
	likeHits := func(term string) int {
		t.Helper()
		var n int
		if err = database.GetContext(ctx, &n, `SELECT count(*) FROM items WHERE plain_text LIKE ?`, "%"+term+"%"); err != nil {
			t.Fatal(err)
		}
		return n
	}
	runCLI := func(args ...string) (string, error) {
		var out bytes.Buffer
		cmd := newRootCmd(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cmd.SetOut(&out)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append([]string{"--config", cfgPath}, args...))
		execErr := cmd.Execute()
		return out.String(), execErr
	}

	addItem("itemaaaaaaaa", reindexXML("Alpha", "needlealpha qz"), "BROKEN")
	addItem("itembbbbbbbb", reindexXML("Beta", "needlebeta"), "BROKEN")
	if ftsHits("needlealpha") != 0 || likeHits("qz") != 0 {
		t.Fatal("fixture should start with stale search data")
	}

	// A running server holds the data_dir lock: writing reindex is refused, --dry-run is not and writes nothing.
	lock, err := db.TryLockDataDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runCLI("reindex"); err == nil || !strings.Contains(err.Error(), "refusing to run reindex") {
		_ = lock.Close()
		t.Fatalf("reindex with server lock = %v, want refusal", err)
	}
	dryOut, err := runCLI("reindex", "--dry-run")
	if err != nil {
		_ = lock.Close()
		t.Fatalf("reindex --dry-run with server lock = %v", err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dryOut, "would reindex 2 of 2 items (2 with changed plain_text), 0 failed") {
		t.Errorf("dry-run output = %q", dryOut)
	}
	if ftsHits("needlealpha") != 0 || likeHits("qz") != 0 || likeHits("BROKEN") != 2 {
		t.Error("--dry-run changed the database")
	}

	// Offline reindex restores both the FTS and the LIKE search paths.
	out, err := runCLI("reindex")
	if err != nil {
		t.Fatalf("reindex = %v\n%s", err, out)
	}
	if !strings.Contains(out, "reindexed 2 of 2 items (2 with changed plain_text), 0 failed") {
		t.Errorf("reindex output = %q", out)
	}
	if ftsHits("needlealpha") != 1 || ftsHits("needlebeta") != 1 || ftsHits("BROKEN") != 0 {
		t.Error("items_fts was not rebuilt")
	}
	if likeHits("qz") != 1 || likeHits("BROKEN") != 0 {
		t.Error("items.plain_text was not rebuilt")
	}
	var ftsCount int
	if err = database.GetContext(ctx, &ftsCount, `SELECT count(*) FROM items_fts`); err != nil || ftsCount != 2 {
		t.Fatalf("items_fts rows = %d, %v; want 2 (no duplicates)", ftsCount, err)
	}
	if out, err = runCLI("reindex", "--dry-run"); err != nil || !strings.Contains(out, "(0 with changed plain_text)") {
		t.Errorf("second dry-run = %q, %v; want nothing left to change", out, err)
	}

	// A malformed XML and a missing object are reported without stopping the others; their old data
	// stays and the run exits non-zero.
	addItem("itemcccccccc", "<document>", "KEEPC")
	addItem("itemdddddddd", "", "KEEPD")
	if _, err = database.ExecContext(ctx, `UPDATE items SET plain_text = 'BROKEN' WHERE id = 'itemaaaaaaaa'`); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI("reindex")
	if err == nil || !strings.Contains(err.Error(), "2 item(s) failed") {
		t.Fatalf("reindex with bad items = %v, want failure for 2 items", err)
	}
	for _, want := range []string{"item itemcccccccc", "INVALID_XML", "item itemdddddddd", "read xml", "reindexed 2 of 4 items (1 with changed plain_text), 2 failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if likeHits("qz") != 1 || likeHits("KEEPC") != 1 || likeHits("KEEPD") != 1 || ftsHits("KEEPC") != 1 || ftsHits("KEEPD") != 1 {
		t.Error("good item was not repaired or failed items lost their data")
	}
	if _, err = runCLI("reindex", "--dry-run"); err == nil {
		t.Error("dry-run with bad items should exit non-zero")
	}
}
