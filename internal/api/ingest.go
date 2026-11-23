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
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gabriel-vasile/mimetype"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/media"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const (
	maxXMLSize       = 10 << 20
	maxStoredXMLSize = maxXMLSize + (1 << 20) // 11 MiB bound accounts for rewritten local/remote asset URLs
	maxImageSize     = 20 << 20
	maxPDFSize       = 50 << 20
	maxAudioSize     = 100 << 20
	// formMemory is how much of the multipart form is held in memory before it spills to disk.
	formMemory = 32 << 20
)

// imageAssetTypes maps the sniffed MIME type of an accepted figure to its storage extension.
var imageAssetTypes = map[string]string{
	"image/png":     ".png",
	"image/jpeg":    ".jpg",
	"image/webp":    ".webp",
	"image/svg+xml": ".svg",
}

var audioAssetExts = map[string]string{
	"audio/mpeg": ".mp3",
	"audio/wav":  ".wav",
	"audio/mp4":  ".m4a",
}

type storedAsset struct {
	src             string
	open            func() (io.ReadSeekCloser, error)
	hash            string
	mime            string
	path            string
	size            int64
	durationSeconds *int
	pageCount       *int
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
	if err = s.db.SelectContext(ctx, &found, query, args...); err != nil {
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

// normalizeExternalKey validates a non-empty external_key string: 1 to 200 UTF-8 runes, no control characters.
func normalizeExternalKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("external_key must not be blank")
	}
	if !utf8.ValidString(trimmed) {
		return "", errors.New("external_key must be valid UTF-8")
	}
	if utf8.RuneCountInString(trimmed) > 200 {
		return "", errors.New("external_key must be 1 to 200 characters after trimming")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", errors.New("external_key must not contain control characters")
		}
	}
	return trimmed, nil
}

// externalKeyField returns the trimmed external_key form field: 1 to 200 characters without control characters.
func externalKeyField(form *multipart.Form) (string, error) {
	vals, present := form.Value["external_key"]
	if !present {
		return "", nil
	}
	if len(vals) > 1 {
		return "", errors.New("duplicate external_key field")
	}
	return normalizeExternalKey(vals[0])
}

type ingestParams struct {
	tokenDigest string
	draft       bool
	createOnly  bool
	externalKey string
	itemID      string
	dirs        []string
	fileLabel   string
	xmlBytes    []byte
	uploads     map[string]*multipart.FileHeader
}

type ingestOutcome struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	URL       string   `json:"url"`
	Dirs      []string `json:"dirs"`
	Assets    int      `json:"assets"`
	Created   bool     `json:"created"`
	Draft     bool     `json:"draft"`
	Published bool     `json:"published"`
}

func (s *Server) acquireIngestSlot() (func(), bool) {
	if s.ingestSem == nil {
		return func() {}, true
	}
	select {
	case s.ingestSem <- struct{}{}:
		return func() { <-s.ingestSem }, true
	default:
		return nil, false
	}
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	tokenDigest, ok := s.auth.AuthenticateToken(r.Header.Get("Authorization"))
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="aumbry"`)
		writeError(w, http.StatusUnauthorized, "unauthorized", "a valid API token is required")
		return
	}
	release, slotOK := s.acquireIngestSlot()
	if !slotOK {
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "too many concurrent ingest requests, try again later")
		return
	}
	defer release()
	ctx := r.Context()

	maxBody := s.maxIngestBody
	if maxBody <= 0 {
		maxBody = defaultRouterIngestBody
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := r.ParseMultipartForm(formMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("request body exceeds the %dMB limit", maxBody/(1<<20)))
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
	externalKey, err := externalKeyField(form)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	itemID := strings.TrimSpace(r.FormValue("item_id"))

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
			fmt.Sprintf("asset_srcs must list one asset src per uploaded asset (%d files, %d srcs)", len(assetFiles), len(srcs)))
		return
	}
	uploads := map[string]*multipart.FileHeader{}
	for i, fh := range assetFiles {
		src, okNorm := schema.NormalizeAssetPath(srcs[i])
		if !okNorm {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid asset_srcs value %q", srcs[i]))
			return
		}
		if uploads[src] != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("asset %q was uploaded twice", src))
			return
		}
		if fh.Size > maxAudioSize {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("asset %q exceeds the 100MB limit", src))
			return
		}
		uploads[src] = fh
	}
	dirs := splitDirs(form.Value["dirs"])
	input, err := readFile(xmlHeader, maxXMLSize)
	if err != nil {
		internalError(w, r, err)
		return
	}

	outcome, status, code, msg, valRes, storageErr, internalErr := s.executeIngest(ctx, ingestParams{
		tokenDigest: tokenDigest,
		draft:       draft,
		createOnly:  createOnly,
		externalKey: externalKey,
		itemID:      itemID,
		dirs:        dirs,
		fileLabel:   xmlHeader.Filename,
		xmlBytes:    input,
		uploads:     uploads,
	})
	if storageErr != nil {
		s.storageError(w, r, storageErr)
		return
	}
	if internalErr != nil {
		internalError(w, r, internalErr)
		return
	}
	if valRes != nil {
		writeJSON(w, http.StatusUnprocessableEntity, valRes)
		return
	}
	if status != 0 {
		writeError(w, status, code, msg)
		return
	}
	httpStatus := http.StatusOK
	if outcome.Created {
		httpStatus = http.StatusCreated
	}
	writeJSON(w, httpStatus, outcome)
}

func (s *Server) executeIngest(ctx context.Context, in ingestParams) (outcome *ingestOutcome, errStatus int, errCode, errMsg string, valRes *schema.ValidationResult, storageErr, internalErr error) {
	if len(in.xmlBytes) > maxXMLSize {
		return nil, http.StatusRequestEntityTooLarge, "too_large", "xml exceeds the 10MB limit", nil, nil, nil
	}
	if in.createOnly && in.itemID != "" {
		return nil, http.StatusBadRequest, "invalid_request", "create_only cannot be used together with item_id", nil, nil, nil
	}
	if in.createOnly && in.externalKey == "" {
		return nil, http.StatusBadRequest, "invalid_request", "create_only requires external_key", nil, nil, nil
	}
	if in.draft && len(in.dirs) > 0 {
		return nil, http.StatusBadRequest, "invalid_request", "draft cannot be combined with dirs", nil, nil, nil
	}
	missing, err := s.missingDirs(ctx, in.dirs)
	if err != nil {
		return nil, 0, "", "", nil, nil, err
	}
	if len(missing) > 0 {
		return nil, http.StatusBadRequest, "unknown_dirs", "unknown directories: " + strings.Join(missing, ", "), nil, nil, nil
	}

	itemID := in.itemID
	created := false
	switch {
	case in.createOnly:
		var existingID string
		err = s.db.GetContext(ctx, &existingID, `SELECT id FROM items WHERE external_key = ?`, in.externalKey)
		if err == nil {
			return nil, http.StatusConflict, "key_exists", "external_key already exists", nil, nil, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, "", "", nil, nil, err
		}
	case itemID != "":
		var exists bool
		err = s.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)`, itemID)
		if err != nil {
			return nil, 0, "", "", nil, nil, err
		}
		if !exists {
			return nil, http.StatusNotFound, "not_found", "item_id does not exist", nil, nil, nil
		}
	case in.externalKey != "":
		err = s.db.GetContext(ctx, &itemID, `SELECT id FROM items WHERE external_key = ?`, in.externalKey)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, "", "", nil, nil, err
		}
	}
	if itemID == "" {
		id, idErr := newID()
		if idErr != nil {
			return nil, 0, "", "", nil, nil, idErr
		}
		itemID, created = id, true
	} else if in.draft {
		mounts, mountErr := itemMounts(ctx, s.db, itemID)
		if mountErr != nil {
			return nil, 0, "", "", nil, nil, mountErr
		}
		if len(mounts) > 0 {
			return nil, http.StatusConflict, "already_published",
				"item is already published; run `aumbry unpublish` first or push without --draft", nil, nil, nil
		}
	}

	doc, issues := schema.Parse(in.xmlBytes, schema.Options{HasAsset: func(src string) bool { return in.uploads[src] != nil }})
	if doc == nil || len(issues) > 0 {
		res := invalidResult(in.fileLabel, issues...)
		return nil, 0, "", "", &res, nil, nil
	}

	// Pass 1: stream-validate & hash every local asset without buffering entire binary files in memory.
	stored := make([]storedAsset, 0, len(doc.AssetRefs)+len(doc.RemoteFigures))
	rewrite := make(map[string]string, len(doc.AssetRefs)+len(doc.RemoteFigures))
	hashBySrc := make(map[string]string, len(doc.AssetRefs)+len(doc.RemoteFigures))
	pageCountBySrc := make(map[string]int)
	var assetIssues []schema.ValidationError

	for _, ref := range doc.AssetRefs {
		fh := in.uploads[ref.Path]
		asset, issue, tooBigMsg, inspectErr := inspectUploadedAsset(ctx, ref, fh)
		if inspectErr != nil {
			return nil, 0, "", "", nil, nil, inspectErr
		}
		if tooBigMsg != "" {
			return nil, http.StatusRequestEntityTooLarge, "too_large", tooBigMsg, nil, nil, nil
		}
		if issue != nil {
			assetIssues = append(assetIssues, *issue)
			continue
		}
		if asset.pageCount != nil {
			pageCountBySrc[ref.Path] = *asset.pageCount
		}
		stored = append(stored, asset)
		rewrite[ref.Path] = "/api/v1/assets/" + asset.hash
		hashBySrc[ref.Path] = asset.hash
	}
	for _, pr := range doc.AttachmentPageRefs {
		maxPage := pageCountBySrc[pr.AttachmentSrc]
		if maxPage > 0 && (pr.Page < 1 || pr.Page > maxPage) {
			assetIssues = append(assetIssues, schema.ValidationError{
				Line:    pr.Line,
				Code:    "INVALID_TARGET",
				Field:   pr.Field,
				Message: fmt.Sprintf("page target %q exceeds PDF %q page count (%d)", pr.Target, pr.AttachmentSrc, maxPage),
				Hint:    fmt.Sprintf("use a page number between 1 and %d", maxPage),
			})
		}
	}
	if len(assetIssues) > 0 {
		res := invalidResult(in.fileLabel, assetIssues...)
		return nil, 0, "", "", &res, nil, nil
	}

	// Pass 1b: SSRF-safe localization of remote <figure src="http(s)://..."> assets (v1.2).
	remoteAssets, remoteCleanup, remoteIssues := s.localizeRemoteImages(ctx, doc.RemoteFigures)
	defer remoteCleanup()
	if len(remoteIssues) > 0 {
		res := invalidResult(in.fileLabel, remoteIssues...)
		return nil, 0, "", "", &res, nil, nil
	}
	for _, ra := range remoteAssets {
		stored = append(stored, ra)
		rewrite[ra.src] = "/api/v1/assets/" + ra.hash
		hashBySrc[ra.src] = ra.hash
	}

	// Rewrite and enforce the 11 MiB storage limit BEFORE uploading assets so oversized documents
	// fail fast without leaving unreferenced assets in object storage.
	rewritten, err := doc.RewriteAssets(rewrite)
	if err != nil {
		return nil, 0, "", "", nil, nil, err
	}
	if len(rewritten) > maxStoredXMLSize {
		return nil, http.StatusRequestEntityTooLarge, "too_large", "rewritten xml exceeds the 11MB storage limit", nil, nil, nil
	}

	// Pass 2: upload deduplicated assets via streaming open().
	uploadedPath := make(map[string]bool, len(stored))
	for _, asset := range stored {
		if uploadedPath[asset.path] {
			continue
		}
		uploadedPath[asset.path] = true
		if uploadErr := s.uploadStoredAsset(ctx, asset); uploadErr != nil {
			return nil, 0, "", "", nil, uploadErr, nil
		}
	}

	// The content is stored under a path derived from its hash, so a version is never overwritten:
	// the live item keeps serving its old object until the transaction commits the new path.
	sum := sha256.Sum256(rewritten)
	xmlSHA256 := hex.EncodeToString(sum[:])
	xmlPath := "/xml/" + itemID + "-" + xmlSHA256 + ".xml"
	present, err := s.store.Exists(ctx, xmlPath)
	if err == nil && !present {
		err = s.store.Put(ctx, xmlPath, bytes.NewReader(rewritten))
	}
	if err != nil {
		return nil, 0, "", "", nil, err, nil
	}

	mounted, err := s.saveItem(ctx, saveItem{
		id:          itemID,
		created:     created,
		draft:       in.draft,
		externalKey: in.externalKey,
		createdBy:   in.tokenDigest,
		doc:         doc,
		xmlPath:     xmlPath,
		xmlSHA256:   xmlSHA256,
		assets:      stored,
		hashBySrc:   hashBySrc,
		dirs:        in.dirs,
	})
	if err != nil {
		switch {
		case errors.Is(err, errAlreadyPublished):
			return nil, http.StatusConflict, "already_published",
				"item is already published; run `aumbry unpublish` first or push without --draft", nil, nil, nil
		case errors.Is(err, sql.ErrNoRows):
			return nil, http.StatusNotFound, "not_found", "item_id does not exist", nil, nil, nil
		case db.IsUniqueViolation(err):
			if in.createOnly {
				return nil, http.StatusConflict, "key_exists", "external_key already exists", nil, nil, nil
			}
			return nil, http.StatusConflict, "conflict", "external_key is already used by another item", nil, nil, nil
		default:
			return nil, 0, "", "", nil, nil, err
		}
	}

	prefix := "/r/"
	if doc.Meta.Type == "post" {
		prefix = "/p/"
	}
	return &ingestOutcome{
		ID:        itemID,
		Type:      doc.Meta.Type,
		URL:       prefix + itemID,
		Dirs:      mounted,
		Assets:    len(distinctHashes(stored)),
		Created:   created,
		Draft:     in.draft,
		Published: len(mounted) > 0,
	}, 0, "", "", nil, nil, nil
}

func inspectUploadedAsset(ctx context.Context, ref schema.AssetRef, fh *multipart.FileHeader) (asset storedAsset, issue *schema.ValidationError, tooBigMsg string, err error) {
	limit := int64(maxImageSize)
	limitMB := 20
	switch ref.Kind {
	case schema.AssetKindAudio:
		limit = maxAudioSize
		limitMB = 100
	case schema.AssetKindPDF:
		limit = maxPDFSize
		limitMB = 50
	}
	if fh.Size > limit {
		return storedAsset{}, nil, fmt.Sprintf("asset %q exceeds the %dMB limit", ref.Path, limitMB), nil
	}

	f, openErr := fh.Open()
	if openErr != nil {
		return storedAsset{}, nil, "", openErr
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	written, copyErr := io.Copy(hasher, io.LimitReader(f, limit+1))
	if copyErr != nil {
		return storedAsset{}, nil, "", copyErr
	}
	if written > limit {
		return storedAsset{}, nil, fmt.Sprintf("asset %q exceeds the %dMB limit", ref.Path, limitMB), nil
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return storedAsset{}, nil, "", seekErr
	}

	detected, mimeErr := mimetype.DetectReader(f)
	if mimeErr != nil {
		return storedAsset{}, nil, "", mimeErr
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return storedAsset{}, nil, "", seekErr
	}

	hashHex := hex.EncodeToString(hasher.Sum(nil))
	detectedStr := detected.String()
	openFn := func() (io.ReadSeekCloser, error) {
		file, err := fh.Open()
		if err != nil {
			return nil, err
		}
		return file, nil
	}

	switch ref.Kind {
	case schema.AssetKindImage:
		ext, okImg := imageAssetTypes[detectedStr]
		if !okImg {
			return storedAsset{}, &schema.ValidationError{
				Code: "INVALID_ASSET", Field: "figure.src",
				Message: fmt.Sprintf("%s is a %s file; only png, jpeg, webp and svg images are accepted", ref.Path, detectedStr),
			}, "", nil
		}
		return storedAsset{
			src:  ref.Path,
			open: openFn,
			hash: hashHex,
			mime: detectedStr,
			path: "/assets/" + hashHex + ext,
			size: written,
		}, nil, "", nil
	case schema.AssetKindPDF:
		pdfMeta, pdfErr := media.InspectPDF(ctx, f, detectedStr)
		if pdfErr != nil {
			return storedAsset{}, &schema.ValidationError{
				Code: "INVALID_ATTACHMENT", Field: "attachment.src",
				Message: fmt.Sprintf("%s is not a valid readable PDF (%s): %v", ref.Path, detectedStr, pdfErr),
			}, "", nil
		}
		pages := pdfMeta.PageCount
		return storedAsset{
			src:       ref.Path,
			open:      openFn,
			hash:      hashHex,
			mime:      "application/pdf",
			path:      "/assets/" + hashHex + ".pdf",
			size:      written,
			pageCount: &pages,
		}, nil, "", nil
	default:
		audioMeta, audioErr := media.InspectAudio(f, path.Ext(ref.Path), detectedStr)
		if audioErr != nil {
			return storedAsset{}, &schema.ValidationError{
				Code: "INVALID_ASSET", Field: "audio.src",
				Message: fmt.Sprintf("%s is invalid or unsupported audio (%s): %v", ref.Path, detectedStr, audioErr),
			}, "", nil
		}
		ext := audioAssetExts[audioMeta.MIMEType]
		return storedAsset{
			src:             ref.Path,
			open:            openFn,
			hash:            hashHex,
			mime:            audioMeta.MIMEType,
			path:            "/assets/" + hashHex + ext,
			size:            written,
			durationSeconds: audioMeta.DurationSeconds,
		}, nil, "", nil
	}
}

func (s *Server) uploadStoredAsset(ctx context.Context, asset storedAsset) error {
	exists, err := s.store.Exists(ctx, asset.path)
	if err != nil || exists {
		return err
	}
	f, err := asset.open()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return seekErr
	}
	return s.store.Put(ctx, asset.path, f)
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
	xmlSHA256   string
	assets      []storedAsset
	hashBySrc   map[string]string
	dirs        []string
}

// saveItem writes the item, its assets, search index rows, outgoing links, mounts,
// and revision snapshots in one transaction and returns the resulting mount set.
func (s *Server) saveItem(ctx context.Context, in saveItem) (mounted []string, err error) {
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

	var primaryAudioHash sql.NullString
	primaryAudioChaptersJSON := "[]"
	if pa := in.doc.PrimaryAudio(); pa != nil {
		if h := in.hashBySrc[pa.Src]; h != "" {
			primaryAudioHash = sql.NullString{String: h, Valid: true}
		} else if pa.Hash != "" {
			primaryAudioHash = sql.NullString{String: pa.Hash, Valid: true}
		}
		chJSON, chErr := marshalJSON(pa.Chapters)
		if chErr != nil {
			return nil, chErr
		}
		primaryAudioChaptersJSON = chJSON
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if in.created {
		_, err = tx.ExecContext(ctx, `INSERT INTO items (id, external_key, type, title, report_date, summary, tags_json,
			source_agent, canonical_url, related_ids_json, xml_storage_path, plain_text, created_by, updated_by,
			primary_audio_hash, primary_audio_chapters_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			in.id, key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain, in.createdBy, in.createdBy,
			primaryAudioHash, primaryAudioChaptersJSON)
	} else {
		var pre struct {
			Title     string `db:"title"`
			Date      string `db:"report_date"`
			Summary   string `db:"summary"`
			XMLPath   string `db:"xml_storage_path"`
			CreatedBy string `db:"created_by"`
		}
		if err = tx.GetContext(ctx, &pre, `SELECT title, report_date, summary, xml_storage_path,
			COALESCE(created_by, '') AS created_by FROM items WHERE id = ?`, in.id); err != nil {
			return nil, err
		}
		oldMounts, mountErr := itemMounts(ctx, tx, in.id)
		if mountErr != nil {
			return nil, mountErr
		}
		if in.draft && len(oldMounts) > 0 {
			return nil, errAlreadyPublished
		}
		// Pre-migration 0-revision published items updated before reindex must preserve their prior version as rev=1.
		if !in.draft && len(oldMounts) > 0 {
			if baseErr := s.ensureBaselineRevisionTx(ctx, tx, in.id, ""); baseErr != nil {
				return nil, baseErr
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE items SET external_key = COALESCE(?, external_key), type = ?, title = ?,
			report_date = ?, summary = ?, tags_json = ?, source_agent = ?, canonical_url = ?, related_ids_json = ?,
			xml_storage_path = ?, plain_text = ?, updated_by = NULLIF(?, ''), primary_audio_hash = ?, primary_audio_chapters_json = ?,
			updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			key, meta.Type, meta.Title, meta.Date, meta.Summary, tags,
			meta.SourceAgent, meta.CanonicalURL, related, in.xmlPath, plain, in.createdBy,
			primaryAudioHash, primaryAudioChaptersJSON, in.id)
	}
	if err != nil {
		return nil, err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM item_assets WHERE item_id = ?`, in.id); err != nil {
		return nil, err
	}
	for _, a := range in.assets {
		var dur, pages any
		if a.durationSeconds != nil {
			dur = *a.durationSeconds
		}
		if a.pageCount != nil {
			pages = *a.pageCount
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO assets (hash, mime_type, size_bytes, storage_path, duration_seconds, page_count)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(hash) DO UPDATE SET
				duration_seconds = COALESCE(assets.duration_seconds, excluded.duration_seconds),
				page_count = COALESCE(assets.page_count, excluded.page_count)`,
			a.hash, a.mime, a.size, a.path, dur, pages); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_assets (item_id, asset_hash) VALUES (?, ?)`, in.id, a.hash); err != nil {
			return nil, err
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM item_links WHERE source_item_id = ?`, in.id); err != nil {
		return nil, err
	}
	for _, link := range in.doc.OutgoingLinks() {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_links (source_item_id, target_item_id, kind) VALUES (?, ?, ?)`,
			in.id, link.TargetItemID, link.Kind); err != nil {
			return nil, err
		}
	}

	if err = db.ReplaceItemFTS(ctx, tx, in.id, meta.Title, meta.Summary, strings.Join(meta.Tags, " "), plain); err != nil {
		return nil, err
	}

	for _, dir := range in.dirs {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO directory_items (directory_id, item_id) VALUES (?, ?)`, dir, in.id); err != nil {
			return nil, err
		}
	}
	mounted, err = itemMounts(ctx, tx, in.id)
	if err != nil {
		return nil, err
	}
	if !in.draft && len(mounted) > 0 {
		if snapErr := s.maybeSnapshotRevisionTx(ctx, tx, revisionSnapshot{
			itemID:    in.id,
			title:     meta.Title,
			summary:   meta.Summary,
			date:      meta.Date,
			xmlPath:   in.xmlPath,
			xmlSHA256: in.xmlSHA256,
			createdBy: in.createdBy,
			mounts:    mounted,
		}); snapErr != nil {
			return nil, snapErr
		}
	}
	if err = tx.Commit(); err != nil {
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
