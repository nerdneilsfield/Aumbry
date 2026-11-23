package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

var xmlPathSHA256Re = regexp.MustCompile(`^/xml/[A-Za-z0-9_-]{12}-([0-9a-f]{64})\.xml$`)

// xmlSHA256FromPath extracts the content SHA-256 hex embedded in canonical /xml/<id>-<sha256>.xml paths,
// falling back deterministically to hashing the path itself for synthetic/legacy test rows.
func xmlSHA256FromPath(xmlPath string) string {
	if m := xmlPathSHA256Re.FindStringSubmatch(xmlPath); m != nil {
		return m[1]
	}
	sum := sha256.Sum256([]byte("path:" + xmlPath))
	return hex.EncodeToString(sum[:])
}

// resolveXMLSHA256 returns the embedded SHA-256 for canonical paths without storage I/O,
// and reads from storage only for legacy/synthetic non-canonical XML paths.
func (s *Server) resolveXMLSHA256(ctx context.Context, xmlPath string) string {
	if m := xmlPathSHA256Re.FindStringSubmatch(xmlPath); m != nil {
		return m[1]
	}
	rc, err := s.store.Open(ctx, xmlPath)
	if err != nil {
		return xmlSHA256FromPath(xmlPath)
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, rc); err != nil {
		return xmlSHA256FromPath(xmlPath)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type revMount struct {
	DirID        string `db:"directory_id"`
	WasProtected bool   `db:"was_protected"`
}

// canViewRevisionMounts checks B1 revision access: caller may access at least one recorded mount
// directory AND that directory has not since had its protection cleared if it was protected when
// the revision was published.
func canViewRevisionMounts(t *dirTree, v viewer, mounts []revMount) bool {
	if v.admin {
		return true
	}
	for _, m := range mounts {
		if m.WasProtected && t.isPublic(m.DirID) {
			continue
		}
		if t.accessible(v, m.DirID) {
			return true
		}
	}
	return false
}

type revisionSnapshot struct {
	itemID    string
	title     string
	summary   string
	date      string
	xmlPath   string
	xmlSHA256 string
	createdBy string
	mounts    []string
}

func (s *Server) ensureBaselineRevisionTx(ctx context.Context, tx *sqlx.Tx, itemID, xmlSHA256 string) error {
	var row struct {
		LastRev   int    `db:"last_rev"`
		Title     string `db:"title"`
		Date      string `db:"report_date"`
		Summary   string `db:"summary"`
		XMLPath   string `db:"xml_storage_path"`
		UpdatedBy string `db:"updated_by"`
	}
	if err := tx.GetContext(ctx, &row, `SELECT last_rev, title, report_date, summary, xml_storage_path,
		COALESCE(updated_by, '') AS updated_by FROM items WHERE id = ?`, itemID); err != nil {
		return err
	}
	var revCount int
	if err := tx.GetContext(ctx, &revCount, `SELECT count(*) FROM item_revisions WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	if revCount > 0 || row.LastRev > 0 {
		return nil
	}
	oldMounts, err := itemMounts(ctx, tx, itemID)
	if err != nil {
		return err
	}
	if len(oldMounts) == 0 {
		return nil
	}
	oldAssets := []string{}
	if err = tx.SelectContext(ctx, &oldAssets, `SELECT asset_hash FROM item_assets WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return err
	}
	sha := xmlSHA256
	if sha == "" {
		sha = xmlSHA256FromPath(row.XMLPath)
	}
	_, err = insertRevisionTx(ctx, tx, t, 1, revisionSnapshot{
		itemID:    itemID,
		title:     row.Title,
		summary:   row.Summary,
		date:      row.Date,
		xmlPath:   row.XMLPath,
		xmlSHA256: sha,
		createdBy: row.UpdatedBy,
		mounts:    oldMounts,
	}, oldAssets)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE items SET last_rev = 1 WHERE id = ? AND last_rev < 1`, itemID)
	return err
}

// maybeSnapshotRevisionTx inserts a new published revision snapshot inside tx when either
// no revision exists yet or snap.xmlSHA256 differs from the latest recorded revision. When the
// content matches the latest revision (e.g. remounting an unchanged item into a new directory Q),
// it merges snap.mounts onto the latest revision's mounts so Q's readers stay in sync.
func (s *Server) maybeSnapshotRevisionTx(ctx context.Context, tx *sqlx.Tx, snap revisionSnapshot) error {
	if len(snap.mounts) == 0 {
		return nil
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		return err
	}
	var itemLastRev int
	if err = tx.GetContext(ctx, &itemLastRev, `SELECT last_rev FROM items WHERE id = ?`, snap.itemID); err != nil {
		return err
	}
	var latest struct {
		ID        int64  `db:"id"`
		Rev       int    `db:"rev"`
		XMLSHA256 string `db:"xml_sha256"`
	}
	err = tx.GetContext(ctx, &latest, `SELECT id, rev, xml_sha256 FROM item_revisions
		WHERE item_id = ? ORDER BY rev DESC LIMIT 1`, snap.itemID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && latest.XMLSHA256 == snap.xmlSHA256 {
		for _, dirID := range snap.mounts {
			wasProt := 0
			if !t.isPublic(dirID) {
				wasProt = 1
			}
			if _, execErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_mounts
				(revision_id, directory_id, was_protected) VALUES (?, ?, ?)`,
				latest.ID, dirID, wasProt); execErr != nil {
				return execErr
			}
		}
		return nil
	}

	nextRev := max(itemLastRev, latest.Rev) + 1
	revID, err := insertRevisionTx(ctx, tx, t, nextRev, snap, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE items SET last_rev = ? WHERE id = ?`, nextRev, snap.itemID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_assets (revision_id, asset_hash)
		SELECT ?, asset_hash FROM item_assets WHERE item_id = ?`, revID, snap.itemID); err != nil {
		return err
	}

	if s.maxRevisions > 0 {
		_, err = tx.ExecContext(ctx, `DELETE FROM item_revisions
			WHERE item_id = ? AND id NOT IN (
				SELECT id FROM item_revisions WHERE item_id = ? ORDER BY rev DESC LIMIT ?
			)`, snap.itemID, snap.itemID, s.maxRevisions)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertRevisionTx(ctx context.Context, tx *sqlx.Tx, t *dirTree, rev int, snap revisionSnapshot, copyAssetsFromItem []string) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO item_revisions
		(item_id, rev, title, summary, report_date, xml_storage_path, xml_sha256, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		snap.itemID, rev, snap.title, snap.summary, snap.date, snap.xmlPath, snap.xmlSHA256, snap.createdBy)
	if err != nil {
		return 0, err
	}
	revID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, hash := range copyAssetsFromItem {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_assets (revision_id, asset_hash) VALUES (?, ?)`,
			revID, hash); err != nil {
			return 0, err
		}
	}
	for _, dirID := range snap.mounts {
		wasProt := 0
		if t != nil && !t.isPublic(dirID) {
			wasProt = 1
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_mounts (revision_id, directory_id, was_protected) VALUES (?, ?, ?)`,
			revID, dirID, wasProt); err != nil {
			return 0, err
		}
	}
	return revID, nil
}

func revisionMounts(ctx context.Context, q sqlx.QueryerContext, revisionID int64) ([]revMount, error) {
	var mounts []revMount
	err := sqlx.SelectContext(ctx, q, &mounts, `SELECT directory_id, was_protected FROM item_revision_mounts
		WHERE revision_id = ? ORDER BY directory_id`, revisionID)
	return mounts, err
}

func revisionMountsMap(ctx context.Context, q sqlx.QueryerContext, revisionIDs []int64) (map[int64][]revMount, error) {
	out := make(map[int64][]revMount, len(revisionIDs))
	if len(revisionIDs) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT revision_id, directory_id, was_protected FROM item_revision_mounts
		WHERE revision_id IN (?) ORDER BY revision_id, directory_id`, revisionIDs)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		RevisionID   int64  `db:"revision_id"`
		DirID        string `db:"directory_id"`
		WasProtected bool   `db:"was_protected"`
	}
	if err = sqlx.SelectContext(ctx, q, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RevisionID] = append(out[row.RevisionID], revMount{
			DirID:        row.DirID,
			WasProtected: row.WasProtected,
		})
	}
	return out, nil
}

func revisionStatsFrom(ctx context.Context, q sqlx.QueryerContext, t *dirTree, v viewer, itemID string) (count, currentRev int, err error) {
	if v.admin {
		var row struct {
			Cnt    int `db:"cnt"`
			MaxRev int `db:"max_rev"`
		}
		if err = sqlx.GetContext(ctx, q, &row, `SELECT count(*) AS cnt, COALESCE(MAX(rev), 0) AS max_rev FROM item_revisions WHERE item_id = ?`, itemID); err != nil {
			return 0, 0, err
		}
		return row.Cnt, row.MaxRev, nil
	}
	var revs []struct {
		ID  int64 `db:"id"`
		Rev int   `db:"rev"`
	}
	if err = sqlx.SelectContext(ctx, q, &revs, `SELECT id, rev FROM item_revisions WHERE item_id = ? ORDER BY rev DESC`, itemID); err != nil {
		return 0, 0, err
	}
	if len(revs) == 0 {
		return 0, 0, nil
	}
	ids := make([]int64, len(revs))
	for i, rv := range revs {
		ids[i] = rv.ID
	}
	mountsByRev, err := revisionMountsMap(ctx, q, ids)
	if err != nil {
		return 0, 0, err
	}
	for _, rv := range revs {
		if !canViewRevisionMounts(t, v, mountsByRev[rv.ID]) {
			continue
		}
		count++
		if rv.Rev > currentRev {
			currentRev = rv.Rev
		}
	}
	return count, currentRev, nil
}

type itemRevisionSummary struct {
	Rev       int    `json:"rev"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Date      string `json:"date"`
	CreatedAt string `json:"created_at"`
	IsCurrent bool   `json:"is_current"`
}

func (s *Server) listItemRevisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var itemXMLPath string
	err = sqlx.GetContext(ctx, tx, &itemXMLPath, `SELECT xml_storage_path FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	v := s.viewerFor(r, t)
	ok, err := canViewItemFrom(ctx, tx, t, v, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "this item is not accessible")
		return
	}

	var rows []struct {
		ID        int64  `db:"id"`
		Rev       int    `db:"rev"`
		Title     string `db:"title"`
		Summary   string `db:"summary"`
		Date      string `db:"report_date"`
		XMLSHA256 string `db:"xml_sha256"`
		CreatedAt string `db:"created_at"`
	}
	if err = sqlx.SelectContext(ctx, tx, &rows, `SELECT id, rev, title, summary, report_date, xml_sha256,
		strftime('%Y-%m-%dT%H:%M:%SZ', created_at) AS created_at
		FROM item_revisions WHERE item_id = ? ORDER BY rev DESC`, id); err != nil {
		internalError(w, r, err)
		return
	}

	var mountsByRev map[int64][]revMount
	if !v.admin && len(rows) > 0 {
		ids := make([]int64, len(rows))
		for i, row := range rows {
			ids[i] = row.ID
		}
		mountsByRev, err = revisionMountsMap(ctx, tx, ids)
		if err != nil {
			internalError(w, r, err)
			return
		}
	}
	_ = tx.Rollback()

	liveSHA := s.resolveXMLSHA256(ctx, itemXMLPath)
	maxRev := 0
	if len(rows) > 0 {
		maxRev = rows[0].Rev
	}

	revs := make([]itemRevisionSummary, 0, len(rows))
	currentRev := 0
	for _, row := range rows {
		if !v.admin && !canViewRevisionMounts(t, v, mountsByRev[row.ID]) {
			continue
		}
		if row.Rev > currentRev {
			currentRev = row.Rev
		}
		revs = append(revs, itemRevisionSummary{
			Rev:       row.Rev,
			Title:     row.Title,
			Summary:   row.Summary,
			Date:      row.Date,
			CreatedAt: row.CreatedAt,
			IsCurrent: row.Rev == maxRev && row.XMLSHA256 == liveSHA,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"item_id":     id,
		"current_rev": currentRev,
		"revisions":   revs,
	})
}

func (s *Server) getItemRevisionContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	rev, err := strconv.Atoi(chi.URLParam(r, "rev"))
	if err != nil || rev < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision must be a positive integer")
		return
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	exists, err := itemExistsFrom(ctx, tx, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	v := s.viewerFor(r, t)
	ok, err := canViewItemFrom(ctx, tx, t, v, id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden", "this item is not accessible")
		return
	}

	var revRow struct {
		ID      int64  `db:"id"`
		XMLPath string `db:"xml_storage_path"`
	}
	err = sqlx.GetContext(ctx, tx, &revRow, `SELECT id, xml_storage_path FROM item_revisions WHERE item_id = ? AND rev = ?`, id, rev)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "revision not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !v.admin {
		mounts, mountErr := revisionMounts(ctx, tx, revRow.ID)
		if mountErr != nil {
			internalError(w, r, mountErr)
			return
		}
		if !canViewRevisionMounts(t, v, mounts) {
			writeError(w, http.StatusForbidden, "forbidden", "this revision is not accessible")
			return
		}
	}
	_ = tx.Rollback()

	body, err := s.store.Open(ctx, revRow.XMLPath)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "revision content is missing from storage")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, body)
}

func (s *Server) adminDeleteRevision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	rev, err := strconv.Atoi(chi.URLParam(r, "rev"))
	if err != nil || rev < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "revision must be a positive integer")
		return
	}

	var preXMLPath string
	err = s.db.GetContext(ctx, &preXMLPath, `SELECT xml_storage_path FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	resolvedPreSHA := s.resolveXMLSHA256(ctx, preXMLPath)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var itemXMLPath string
	err = tx.GetContext(ctx, &itemXMLPath, `SELECT xml_storage_path FROM items WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "item not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	liveSHA := resolvedPreSHA
	if itemXMLPath != preXMLPath {
		liveSHA = xmlSHA256FromPath(itemXMLPath)
	}

	var maxRev int
	if err = tx.GetContext(ctx, &maxRev, `SELECT COALESCE(MAX(rev), 0) FROM item_revisions WHERE item_id = ?`, id); err != nil {
		internalError(w, r, err)
		return
	}

	var revRow struct {
		ID        int64  `db:"id"`
		XMLSHA256 string `db:"xml_sha256"`
	}
	err = tx.GetContext(ctx, &revRow, `SELECT id, xml_sha256 FROM item_revisions WHERE item_id = ? AND rev = ?`, id, rev)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "revision not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if rev == maxRev && liveSHA == revRow.XMLSHA256 {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("cannot delete current revision %d; push a new update or delete the item", rev))
		return
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM item_revisions WHERE id = ?`, revRow.ID); err != nil {
		internalError(w, r, err)
		return
	}
	if err = tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
