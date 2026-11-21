package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const (
	maxIngestBody = 100 << 20
	maxXMLSize    = 10 << 20
	maxAssetSize  = 20 << 20
	// formMemory is how much of the multipart form is held in memory before it spills to disk.
	formMemory = 32 << 20
)

// assetTypes maps the sniffed MIME type of an accepted figure to its storage extension.
var assetTypes = map[string]string{
	"image/png":     ".png",
	"image/jpeg":    ".jpg",
	"image/webp":    ".webp",
	"image/svg+xml": ".svg",
}

type storedAsset struct {
	hash, mime, path string
	size             int
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// splitDirs accepts repeated and comma-separated dir ids.
func splitDirs(values []string) []string {
	var parts []string
	for _, v := range values {
		parts = append(parts, strings.Split(v, ",")...)
	}
	return dedupe(parts)
}

func (s *Server) missingDirs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(`SELECT id FROM directories WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	var found []string
	if err := s.db.SelectContext(ctx, &found, query, args...); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, id := range found {
		have[id] = true
	}
	var missing []string
	for _, id := range ids {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func readFile(fh *multipart.FileHeader, limit int64) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

func invalidResult(file string, errs ...schema.ValidationError) schema.ValidationResult {
	return schema.ValidationResult{Valid: false, File: file, Errors: errs}
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if !s.auth.CheckToken(r.Header.Get("Authorization")) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="aumbry"`)
		writeError(w, http.StatusUnauthorized, "unauthorized", "a valid API token is required")
		return
	}
	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBody)
	if err := r.ParseMultipartForm(formMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body exceeds the 100MB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "expected a multipart/form-data body: "+err.Error())
		return
	}
	form := r.MultipartForm
	defer func() { _ = form.RemoveAll() }()

	// Request fields.
	xmlFiles := form.File["xml"]
	if len(xmlFiles) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "exactly one xml file field is required")
		return
	}
	xmlHeader := xmlFiles[0]
	if xmlHeader.Size > maxXMLSize {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "xml exceeds the 10MB limit")
		return
	}
	assetFiles := append(append([]*multipart.FileHeader{}, form.File["assets[]"]...), form.File["assets"]...)
	srcs := form.Value["asset_srcs"]
	if len(srcs) != len(assetFiles) {
		writeError(w, http.StatusBadRequest, "invalid_request",
			fmt.Sprintf("asset_srcs must list one figure src per uploaded asset (%d files, %d srcs)", len(assetFiles), len(srcs)))
		return
	}
	uploads := map[string]*multipart.FileHeader{}
	for i, fh := range assetFiles {
		src, ok := schema.NormalizeAssetPath(srcs[i])
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid asset_srcs value %q", srcs[i]))
			return
		}
		if uploads[src] != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("asset %q was uploaded twice", src))
			return
		}
		if fh.Size > maxAssetSize {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("asset %q exceeds the 20MB limit", src))
			return
		}
		uploads[src] = fh
	}
	dirs := splitDirs(form.Value["dirs"])
	if missing, err := s.missingDirs(ctx, dirs); err != nil {
		internalError(w, r, err)
		return
	} else if len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "unknown_dirs", "unknown directories: "+strings.Join(missing, ", "))
		return
	}

	// Which item is written.
	itemID := strings.TrimSpace(r.FormValue("item_id"))
	externalKey := strings.TrimSpace(r.FormValue("external_key"))
	created := false
	switch {
	case itemID != "":
		exists, err := s.itemExists(ctx, itemID)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if !exists {
			writeError(w, http.StatusNotFound, "not_found", "item_id does not exist")
			return
		}
	case externalKey != "":
		err := s.db.GetContext(ctx, &itemID, `SELECT id FROM items WHERE external_key = ?`, externalKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			internalError(w, r, err)
			return
		}
	}
	if itemID == "" {
		id, err := newID()
		if err != nil {
			internalError(w, r, err)
			return
		}
		itemID, created = id, true
	}

	// Schema validation; a figure exists when its file was uploaded.
	input, err := readFile(xmlHeader, maxXMLSize)
	if err != nil {
		internalError(w, r, err)
		return
	}
	doc, issues := schema.Parse(input, schema.Options{HasAsset: func(src string) bool { return uploads[src] != nil }})
	if doc == nil || len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, invalidResult(xmlHeader.Filename, issues...))
		return
	}

	// Figures.
	stored := make([]storedAsset, 0, len(doc.Assets))
	rewrite := make(map[string]string, len(doc.Assets))
	var assetIssues []schema.ValidationError
	for _, src := range doc.Assets {
		data, err := readFile(uploads[src], maxAssetSize)
		if err != nil {
			internalError(w, r, err)
			return
		}
		kind := mimetype.Detect(data).String()
		if _, ok := assetTypes[kind]; !ok {
			assetIssues = append(assetIssues, schema.ValidationError{
				Code: "INVALID_ASSET", Field: "figure.src",
				Message: fmt.Sprintf("%s is a %s file; only png, jpeg, webp and svg images are accepted", src, kind),
			})
			continue
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		asset := storedAsset{hash: hash, mime: kind, path: "/assets/" + hash + assetTypes[kind], size: len(data)}
		exists, err := s.store.Exists(ctx, asset.path)
		if err == nil && !exists {
			err = s.store.Put(ctx, asset.path, bytes.NewReader(data))
		}
		if err != nil {
			s.storageError(w, r, err)
			return
		}
		stored = append(stored, asset)
		rewrite[src] = "/api/v1/assets/" + hash
	}
	if len(assetIssues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, invalidResult(xmlHeader.Filename, assetIssues...))
		return
	}

	rewritten, err := doc.RewriteAssets(rewrite)
	if err != nil {
		internalError(w, r, err)
		return
	}
	xmlPath := "/xml/" + itemID + ".xml"
	if err := s.store.Put(ctx, xmlPath, bytes.NewReader(rewritten)); err != nil {
		s.storageError(w, r, err)
		return
	}

	mounted, err := s.saveItem(ctx, saveItem{
		id: itemID, created: created, externalKey: externalKey, doc: doc, xmlPath: xmlPath, assets: stored, dirs: dirs,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "item_id does not exist")
		return
	case db.IsUniqueViolation(err):
		if created {
			_ = s.store.Delete(ctx, xmlPath)
		}
		writeError(w, http.StatusConflict, "conflict", "external_key is already used by another item")
		return
	case err != nil:
		if created {
			_ = s.store.Delete(ctx, xmlPath)
		}
		internalError(w, r, err)
		return
	}

	prefix, status := "/r/", http.StatusOK
	if doc.Meta.Type == "post" {
		prefix = "/p/"
	}
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{
		"id": itemID, "type": doc.Meta.Type, "url": prefix + itemID, "dirs": mounted,
		"assets": len(distinctHashes(stored)), "created": created,
	})
}

func distinctHashes(assets []storedAsset) map[string]bool {
	set := map[string]bool{}
	for _, a := range assets {
		set[a.hash] = true
	}
	return set
}

func (s *Server) storageError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "storage failure", "error", err)
	writeError(w, http.StatusBadGateway, "storage_error", "the private storage is unavailable")
}

type saveItem struct {
	id          string
	created     bool
	externalKey string
	doc         *schema.Document
	xmlPath     string
	assets      []storedAsset
	dirs        []string
}

// saveItem writes the item, its assets, its search index row and its new mounts in one transaction
// and returns the resulting mount set. Existing mounts are kept.
func (s *Server) saveItem(ctx context.Context, in saveItem) ([]string, error) {
	meta := in.doc.Meta
	tags, err := marshalJSON(orEmpty(meta.Tags))
	if err != nil {
		return nil, err
	}
	related, err := marshalJSON(orEmpty(meta.RelatedIDs))
	if err != nil {
		return nil, err
	}
	plain := in.doc.PlainText()
	var key any
	if in.externalKey != "" {
		key = in.externalKey
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if in.created {
		_, err = tx.ExecContext(ctx, `INSERT INTO items (id, external_key, type, title, report_date, summary, tags_json,
			source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.id, key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE items SET external_key = COALESCE(?, external_key), type = ?, title = ?,
			report_date = ?, summary = ?, tags_json = ?, source_agent = ?, canonical_url = ?, related_ids_json = ?,
			xml_storage_path = ?, plain_text = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain, in.id)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				err = sql.ErrNoRows
			}
		}
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM item_assets WHERE item_id = ?`, in.id); err != nil {
		return nil, err
	}
	for _, a := range in.assets {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO assets (hash, mime_type, size_bytes, storage_path) VALUES (?, ?, ?, ?)`,
			a.hash, a.mime, a.size, a.path); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_assets (item_id, asset_hash) VALUES (?, ?)`, in.id, a.hash); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM items_fts WHERE item_id = ?`, in.id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO items_fts (item_id, title, summary, tags, plain_text) VALUES (?, ?, ?, ?, ?)`,
		in.id, meta.Title, meta.Summary, strings.Join(meta.Tags, " "), plain); err != nil {
		return nil, err
	}

	for _, dir := range in.dirs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO directory_items (directory_id, item_id) VALUES (?, ?)`, dir, in.id); err != nil {
			return nil, err
		}
	}
	mounted, err := itemMounts(ctx, tx, in.id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if mounted == nil {
		mounted = []string{}
	}
	return mounted, nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
