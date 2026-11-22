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
	"unicode"
	"unicode/utf8"

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
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}

func invalidResult(file string, errs ...schema.ValidationError) schema.ValidationResult {
	return schema.ValidationResult{Valid: false, File: file, Errors: errs}
}

var errAlreadyPublished = errors.New("item is already published")

func parseOptionalBoolField(form *multipart.Form, name string) (bool, error) {
	vals, ok := form.Value[name]
	if !ok {
		return false, nil
	}
	if len(vals) > 1 {
		return false, fmt.Errorf("duplicate %s field", name)
	}
	switch strings.ToLower(strings.TrimSpace(vals[0])) {
	case "", "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, fmt.Errorf("invalid %s value %q", name, vals[0])
	}
}

func extractExternalKeyField(form *multipart.Form) (string, error) {
	vals, present := form.Value["external_key"]
	if !present {
		return "", nil
	}
	if len(vals) > 1 {
		return "", errors.New("duplicate external_key field")
	}
	trimmed := strings.TrimSpace(vals[0])
	if trimmed == "" {
		return "", errors.New("external_key must not be blank")
	}
	return trimmed, nil
}

func validateNewExternalKey(trimmed string) error {
	n := utf8.RuneCountInString(trimmed)
	if n < 1 || n > 200 {
		return errors.New("external_key must be 1 to 200 characters after trimming")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return errors.New("external_key must not contain control characters")
		}
	}
	return nil
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	tokenDigest, ok := s.auth.AuthenticateToken(r.Header.Get("Authorization"))
	if !ok {
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
	draft, err := parseOptionalBoolField(form, "draft")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	createOnly, err := parseOptionalBoolField(form, "create_only")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	externalKey, err := extractExternalKeyField(form)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	itemID := strings.TrimSpace(r.FormValue("item_id"))
	if createOnly && itemID != "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "create_only cannot be used together with item_id")
		return
	}
	if createOnly && externalKey == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "create_only requires external_key")
		return
	}

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
	if draft && len(dirs) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "draft cannot be combined with dirs")
		return
	}
	missing, err := s.missingDirs(ctx, dirs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "unknown_dirs", "unknown directories: "+strings.Join(missing, ", "))
		return
	}

	// Which item is written.
	created := false
	switch {
	case createOnly:
		var existingID string
		err = s.db.GetContext(ctx, &existingID, `SELECT id FROM items WHERE external_key = ?`, externalKey)
		if err == nil {
			writeError(w, http.StatusConflict, "key_exists", "external_key already exists")
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			internalError(w, r, err)
			return
		}
		// New keys must satisfy v1.1 validation; SQLite's UNIQUE constraint still adjudicates concurrent creates.
		if err = validateNewExternalKey(externalKey); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	case itemID != "":
		var currentKey sql.NullString
		err = s.db.GetContext(ctx, &currentKey, `SELECT external_key FROM items WHERE id = ?`, itemID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "item_id does not exist")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		// Existing legacy v1.0 keys remain valid when kept unchanged; setting or changing a key requires v1.1 validation.
		if externalKey != "" && (!currentKey.Valid || currentKey.String != externalKey) {
			if err = validateNewExternalKey(externalKey); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
		}
	case externalKey != "":
		err = s.db.GetContext(ctx, &itemID, `SELECT id FROM items WHERE external_key = ?`, externalKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			internalError(w, r, err)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			if err = validateNewExternalKey(externalKey); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
		}
	}
	if itemID == "" {
		id, idErr := newID()
		if idErr != nil {
			internalError(w, r, idErr)
			return
		}
		itemID, created = id, true
	} else if draft {
		mounts, mountErr := itemMounts(ctx, s.db, itemID)
		if mountErr != nil {
			internalError(w, r, mountErr)
			return
		}
		if len(mounts) > 0 {
			writeError(w, http.StatusConflict, "already_published",
				"item is already published; run `aumbry unpublish` first or push without --draft")
			return
		}
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
		data, readErr := readFile(uploads[src], maxAssetSize)
		if readErr != nil {
			internalError(w, r, readErr)
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
		exists, assetErr := s.store.Exists(ctx, asset.path)
		if assetErr == nil && !exists {
			assetErr = s.store.Put(ctx, asset.path, bytes.NewReader(data))
		}
		if assetErr != nil {
			s.storageError(w, r, assetErr)
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
	// The content is stored under a path derived from its hash, so a version is never overwritten:
	// the live item keeps serving its old object until the transaction commits the new path.
	sum := sha256.Sum256(rewritten)
	xmlPath := "/xml/" + itemID + "-" + hex.EncodeToString(sum[:]) + ".xml"
	present, err := s.store.Exists(ctx, xmlPath)
	if err == nil && !present {
		err = s.store.Put(ctx, xmlPath, bytes.NewReader(rewritten))
	}
	if err != nil {
		s.storageError(w, r, err)
		return
	}

	mounted, oldPath, err := s.saveItem(ctx, saveItem{
		id:          itemID,
		created:     created,
		draft:       draft,
		externalKey: externalKey,
		createdBy:   tokenDigest,
		doc:         doc,
		xmlPath:     xmlPath,
		assets:      stored,
		dirs:        dirs,
	})
	if err != nil {
		// Leave xmlPath in storage on failure so concurrent ingests sharing the same content-hashed path
		// (e.g. a simultaneous publish succeeding while a concurrent --draft fails with 409) never lose
		// their live object. Any unreferenced remnants are cleaned up by offline `aumbry-server gc`.
		switch {
		case errors.Is(err, errAlreadyPublished):
			writeError(w, http.StatusConflict, "already_published",
				"item is already published; run `aumbry unpublish` first or push without --draft")
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "not_found", "item_id does not exist")
		case db.IsUniqueViolation(err):
			if createOnly {
				writeError(w, http.StatusConflict, "key_exists", "external_key already exists")
			} else {
				writeError(w, http.StatusConflict, "conflict", "external_key is already used by another item")
			}
		default:
			internalError(w, r, err)
		}
		return
	}
	if oldPath != "" && oldPath != xmlPath {
		if err := s.store.Delete(ctx, oldPath); err != nil {
			s.logger.WarnContext(ctx, "delete superseded item content", "item", itemID, "path", oldPath, "error", err)
		}
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
		"draft": draft, "published": len(mounted) > 0,
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
	draft       bool
	externalKey string
	createdBy   string
	doc         *schema.Document
	xmlPath     string
	assets      []storedAsset
	dirs        []string
}

// saveItem writes the item, its assets, its search index row and its new mounts in one transaction
// and returns the resulting mount set and, for an update, the xml path it replaced. Existing mounts
// are kept.
func (s *Server) saveItem(ctx context.Context, in saveItem) (mounted []string, oldPath string, err error) {
	meta := in.doc.Meta
	tags, err := marshalJSON(orEmpty(meta.Tags))
	if err != nil {
		return nil, "", err
	}
	related, err := marshalJSON(orEmpty(meta.RelatedIDs))
	if err != nil {
		return nil, "", err
	}
	plain := in.doc.PlainText()
	var key any
	if in.externalKey != "" {
		key = in.externalKey
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	if in.created {
		_, err = tx.ExecContext(ctx, `INSERT INTO items (id, external_key, type, title, report_date, summary, tags_json,
			source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
			in.id, key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain, in.createdBy)
	} else {
		// Reading the replaced path first also proves the item still exists (sql.ErrNoRows otherwise).
		if err = tx.GetContext(ctx, &oldPath, `SELECT xml_storage_path FROM items WHERE id = ?`, in.id); err != nil {
			return nil, "", err
		}
		if in.draft {
			var mountCount int
			if err = tx.GetContext(ctx, &mountCount, `SELECT count(*) FROM directory_items WHERE item_id = ?`, in.id); err != nil {
				return nil, "", err
			}
			if mountCount > 0 {
				return nil, "", errAlreadyPublished
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE items SET external_key = COALESCE(?, external_key), type = ?, title = ?,
			report_date = ?, summary = ?, tags_json = ?, source_agent = ?, canonical_url = ?, related_ids_json = ?,
			xml_storage_path = ?, plain_text = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain, in.id)
	}
	if err != nil {
		return nil, "", err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM item_assets WHERE item_id = ?`, in.id); err != nil {
		return nil, "", err
	}
	for _, a := range in.assets {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO assets (hash, mime_type, size_bytes, storage_path) VALUES (?, ?, ?, ?)`,
			a.hash, a.mime, a.size, a.path); err != nil {
			return nil, "", err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_assets (item_id, asset_hash) VALUES (?, ?)`, in.id, a.hash); err != nil {
			return nil, "", err
		}
	}

	if err = db.ReplaceItemFTS(ctx, tx, in.id, meta.Title, meta.Summary, strings.Join(meta.Tags, " "), plain); err != nil {
		return nil, "", err
	}

	for _, dir := range in.dirs {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO directory_items (directory_id, item_id) VALUES (?, ?)`, dir, in.id); err != nil {
			return nil, "", err
		}
	}
	mounted, err = itemMounts(ctx, tx, in.id)
	if err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	if mounted == nil {
		mounted = []string{}
	}
	return mounted, oldPath, nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
