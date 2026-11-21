package db

import (
	"context"
	"path/filepath"
	"testing"
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
