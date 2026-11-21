package db

import (
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
