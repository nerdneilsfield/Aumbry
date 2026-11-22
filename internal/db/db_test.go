package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

func TestOpenInitializesDatabase(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'items_fts'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("items_fts table count = %d, want 1", count)
	}
}

// Every connection the pool opens, not just the first, must enforce foreign keys.
func TestEveryConnectionEnforcesForeignKeys(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Dropping idle connections forces the pool to open a brand-new one.
	database.SetMaxIdleConns(0)
	for range 2 {
		conn, err := database.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var on int
		err = conn.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&on)
		_ = conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		if on != 1 {
			t.Fatalf("PRAGMA foreign_keys = %d on a fresh connection, want 1", on)
		}
	}
}

// A database created by the pre-goose inline migration has the tables but no version table;
// opening it must succeed and keep its data.
func TestOpenAdoptsDatabaseCreatedBeforeGoose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	database, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO directories (id, name) VALUES ('d1', 'kept')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`ALTER TABLE items DROP COLUMN created_by`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DROP TABLE goose_db_version`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen legacy database: %v", err)
	}
	defer database.Close()
	var name string
	if err := database.QueryRow(`SELECT name FROM directories WHERE id = 'd1'`).Scan(&name); err != nil || name != "kept" {
		t.Fatalf("directory = %q, %v; want the pre-existing row", name, err)
	}
}

func TestMigrationLeavesLegacyCreatedByNull(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data")
	database, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, database.DB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 1); err != nil {
		t.Fatalf("DownTo(1): %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO items
		(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
		VALUES ('legacy000001', 'legacy:k', 'report', 'Legacy', '2026-11-20', 'S', '[]', 'a', '', '[]', '/xml/legacy.xml', 'legacy')`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen v1.0 DB: %v", err)
	}
	defer reopened.Close()

	var createdBy sql.NullString
	if err := reopened.QueryRowContext(ctx, `SELECT created_by FROM items WHERE id = 'legacy000001'`).Scan(&createdBy); err != nil {
		t.Fatal(err)
	}
	if createdBy.Valid {
		t.Fatalf("expected legacy created_by NULL, got %q", createdBy.String)
	}
}

func TestOpenExistingRejectsMissingDB(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "not-initialized")
	if _, err := OpenExisting(missingDir); err == nil {
		t.Fatal("OpenExisting() succeeded on missing DB, want error")
	}
}

func TestBackupDuringConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	live, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	insertItem := func(idx int) error {
		_, err := live.ExecContext(ctx, `INSERT INTO items
			(id, external_key, type, title, report_date, summary, tags_json, source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
			VALUES (?, ?, 'report', ?, '2026-11-21', 'sum', '[]', 'agent', '', '[]', ?, 'text')`,
			fmt.Sprintf("item%08d", idx), fmt.Sprintf("key:%d", idx), fmt.Sprintf("Title %d", idx), fmt.Sprintf("/xml/item%08d.xml", idx))
		return err
	}

	const preRows = 5
	for i := range preRows {
		if err := insertItem(i); err != nil {
			t.Fatal(err)
		}
	}

	stopWriters := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := preRows; ; i++ {
			select {
			case <-stopWriters:
				return
			default:
				if err := insertItem(i); err != nil {
					return
				}
			}
		}
	}()

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := Backup(ctx, dataDir, backupPath); err != nil {
		close(stopWriters)
		wg.Wait()
		t.Fatalf("Backup() error = %v", err)
	}
	close(stopWriters)
	wg.Wait()

	// Reject overwriting an existing backup path or targeting live DB.
	if err := Backup(ctx, dataDir, backupPath); err == nil {
		t.Error("Backup() allowed overwriting existing file")
	}
	if err := Backup(ctx, dataDir, filepath.Join(dataDir, "aumbry.db")); err == nil {
		t.Error("Backup() allowed targeting live database")
	}

	snap, err := sqlx.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()

	var integrity string
	if err := snap.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("PRAGMA integrity_check = %q, %v; want ok", integrity, err)
	}
	var count int
	if err := snap.QueryRow(`SELECT count(*) FROM items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < preRows {
		t.Fatalf("backup items count = %d, want >= %d", count, preRows)
	}
}

func TestTryLockDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	lock1, err := TryLockDataDir(dir)
	if err != nil {
		t.Fatalf("first TryLockDataDir() = %v", err)
	}
	if _, err := TryLockDataDir(dir); !errors.Is(err, ErrDataDirLocked) {
		_ = lock1.Close()
		t.Fatalf("second TryLockDataDir() = %v, want ErrDataDirLocked", err)
	}
	if err := lock1.Close(); err != nil {
		t.Fatalf("lock1.Close() = %v", err)
	}
	lock2, err := TryLockDataDir(dir)
	if err != nil {
		t.Fatalf("TryLockDataDir() after unlock = %v", err)
	}
	_ = lock2.Close()
}
