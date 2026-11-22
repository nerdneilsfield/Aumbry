package api

import (
	"database/sql"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/db"
)

// v10FixtureSQL is a database as v1.0 (commit 1e3c74c) left it: schema version 1, no
// items.created_by column, a protected tree and three legacy items. See the file header.
//
//go:embed testdata/v10/fixture.sql
var v10FixtureSQL string

const (
	v10Root   = "d0root000001" // password "fixture-pw"
	v10Inner  = "d0child00001" // no password of its own, under v10Root
	v10Public = "d0pub0000001"

	v10ItemInner  = "legacy000001" // in v10Inner
	v10ItemPublic = "legacy000002" // in v10Public
	v10ItemTop    = "legacy000003" // in v10Root

	// v10UnlockCookie is the aumbry_unlocked cookie that v1.0 internal/auth (1e3c74c) issues after
	// unlocking v10Root. It was generated with cookieSecret and the fingerprint of the fixture's
	// v10Root password hash. securecookie rejects cookies older than 30 days, so the generator's
	// clock was pinned to 2100-01-01; the value is otherwise exactly what v1.0 emits.
	v10UnlockCookie = "aumbry_unlocked=NDEwMjQ0NDgwMHxFWkJPY1NVeXFzU2d3dXY0REFiOUFxMGhxZjQxTVg2RHI5NXdLVi1OMk9kckluZzh0bUdubGVVMHNRV0l0NFdvfE96WFvkJj-TWR4cJL5SzKcQD8iWa5yIwGiQDssi25C3"
)

func v10XML(title string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><document version="1.0"><meta><title>%s</title><type>report</type>`+
		`<date>2026-01-10</date><summary>s</summary><source-agent>v10-agent</source-agent></meta>`+
		`<body><section title="Main"><md><![CDATA[zebra]]></md></section></body></document>`, title)
}

// newV10Env writes the v1.0 database file, lets the current db.Open upgrade it and serves it.
func newV10Env(t *testing.T) *env {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy, err := sqlx.Open("sqlite", filepath.Join(dir, "aumbry.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(v10FixtureSQL); err != nil {
		t.Fatalf("load v1.0 fixture: %v", err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("v1.1 db.Open on a v1.0 database: %v", err)
	}
	e := newEnvOnDB(t, database, RouterOptions{}, nil)
	for id, title := range map[string]string{v10ItemInner: "Legacy inner", v10ItemPublic: "Legacy public", v10ItemTop: "Legacy top"} {
		e.store.files["/xml/"+id+"-v10.xml"] = []byte(v10XML(title))
	}
	return e
}

func (c *client) itemStatus(id string) (item, content int) {
	c.e.t.Helper()
	return c.do("GET", "/api/v1/items/"+id, nil).status, c.do("GET", "/api/v1/items/"+id+"/content", nil).status
}

func requireItemStatus(t *testing.T, c *client, id string, want int) {
	t.Helper()
	if item, content := c.itemStatus(id); item != want || content != want {
		t.Errorf("item %s: detail %d, content %d; want %d", id, item, content, want)
	}
}

func TestUpgradeFromV10Database(t *testing.T) {
	e := newV10Env(t)

	var version, items, nullCreatedBy int
	if err := e.db.QueryRow(`SELECT max(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err != nil || version < 2 {
		t.Fatalf("schema version = %d, %v; want the created_by migration applied", version, err)
	}
	if err := e.db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE created_by IS NULL) FROM items`).Scan(&items, &nullCreatedBy); err != nil {
		t.Fatal(err)
	}
	if items != 3 || nullCreatedBy != 3 {
		t.Fatalf("items = %d, with NULL created_by = %d; want 3 and 3", items, nullCreatedBy)
	}

	// Permissions are unchanged: the public item is readable, the protected tree is not.
	visitor := e.anon()
	requireItemStatus(t, visitor, v10ItemPublic, http.StatusOK)
	requireItemStatus(t, visitor, v10ItemInner, http.StatusForbidden)
	requireItemStatus(t, visitor, v10ItemTop, http.StatusForbidden)

	// The protected root's password still unlocks the whole subtree, including the inner directory.
	if resp := visitor.do("POST", "/api/v1/dirs/"+v10Inner+"/unlock", map[string]string{"password": "fixture-pw"}); resp.status != http.StatusOK {
		t.Fatalf("unlock: %d %s", resp.status, resp.body)
	}
	requireItemStatus(t, visitor, v10ItemInner, http.StatusOK)
	requireItemStatus(t, visitor, v10ItemTop, http.StatusOK)

	// `push --key` updates a legacy item in place and leaves created_by NULL and the mounts as they were.
	resp := e.anon().ingest(ingestReq{xml: xmlDoc(docOpts{title: "Legacy inner updated"}), key: "legacy:inner"})
	if resp.status != http.StatusOK || resp.json()["id"] != v10ItemInner {
		t.Fatalf("push --key on a legacy item: %d %s, want 200 for %s", resp.status, resp.body, v10ItemInner)
	}
	var createdBy sql.NullString
	if err := e.db.QueryRow(`SELECT created_by FROM items WHERE id = ?`, v10ItemInner).Scan(&createdBy); err != nil {
		t.Fatal(err)
	}
	if createdBy.Valid {
		t.Errorf("created_by = %q after updating a legacy item, want NULL", createdBy.String)
	}
	requireItemStatus(t, e.anon(), v10ItemInner, http.StatusForbidden)
	if body := visitor.do("GET", "/api/v1/items/"+v10ItemInner+"/content", nil).body; !strings.Contains(string(body), "Legacy inner updated") {
		t.Errorf("updated content not served: %s", body)
	}
}

func TestV10UnlockCookieStillAccepted(t *testing.T) {
	e := newV10Env(t)

	requireItemStatus(t, e.anon(), v10ItemInner, http.StatusForbidden)

	v := e.anon()
	v.cookie = v10UnlockCookie
	requireItemStatus(t, v, v10ItemInner, http.StatusOK)
	requireItemStatus(t, v, v10ItemTop, http.StatusOK)
	requireItemStatus(t, v, v10ItemPublic, http.StatusOK)
}
