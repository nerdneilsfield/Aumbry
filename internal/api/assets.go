package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

const rangeWindowSize int64 = 1 << 20

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type assetMeta struct {
	MimeType    string `db:"mime_type"`
	SizeBytes   int64  `db:"size_bytes"`
	StoragePath string `db:"storage_path"`
	CreatedAt   string `db:"created_at"`
}

func (a assetMeta) modTime() time.Time {
	t, err := time.Parse(time.RFC3339, a.CreatedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func loadAssetMetaTx(ctx context.Context, q sqlx.QueryerContext, hash string) (assetMeta, error) {
	var asset assetMeta
	err := sqlx.GetContext(ctx, q, &asset, `SELECT mime_type, size_bytes, storage_path,
		strftime('%Y-%m-%dT%H:%M:%SZ', created_at) AS created_at
		FROM assets WHERE hash = ?`, hash)
	return asset, err
}

func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash := chi.URLParam(r, "hash")
	if !hashPattern.MatchString(hash) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
		return
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	asset, err := loadAssetMetaTx(ctx, tx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
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
	if !v.admin {
		allowed, authErr := canViewAssetTx(ctx, tx, t, v, hash)
		if authErr != nil {
			internalError(w, r, authErr)
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "this asset is not accessible")
			return
		}
	}
	_ = tx.Rollback()

	s.serveAssetRange(w, r, asset, "private, max-age=86400")
}

// canViewAssetTx checks whether viewer v may read asset hash via either:
//  1. a current item mount in item_assets, or
//  2. B1 dual-gate on a historical revision in item_revision_assets (both current item AND revision mounts accessible).
func canViewAssetTx(ctx context.Context, q sqlx.QueryerContext, t *dirTree, v viewer, hash string) (bool, error) {
	var liveMounts []string
	if err := sqlx.SelectContext(ctx, q, &liveMounts, `SELECT DISTINCT di.directory_id FROM item_assets ia
		JOIN directory_items di ON di.item_id = ia.item_id WHERE ia.asset_hash = ?`, hash); err != nil {
		return false, err
	}
	if len(t.visibleDirs(v, liveMounts)) > 0 {
		return true, nil
	}

	var revRefs []struct {
		RevisionID int64  `db:"revision_id"`
		ItemID     string `db:"item_id"`
	}
	if err := sqlx.SelectContext(ctx, q, &revRefs, `SELECT ira.revision_id, ir.item_id
		FROM item_revision_assets ira
		JOIN item_revisions ir ON ir.id = ira.revision_id
		WHERE ira.asset_hash = ?`, hash); err != nil {
		return false, err
	}
	if len(revRefs) == 0 {
		return false, nil
	}

	itemVisible := make(map[string]bool)
	for _, ref := range revRefs {
		curOK, cached := itemVisible[ref.ItemID]
		if !cached {
			mounts, err := itemMounts(ctx, q, ref.ItemID)
			if err != nil {
				return false, err
			}
			curOK = len(t.visibleDirs(v, mounts)) > 0
			itemVisible[ref.ItemID] = curOK
		}
		if !curOK {
			continue
		}
		revMounts, err := revisionMounts(ctx, q, ref.RevisionID)
		if err != nil {
			return false, err
		}
		if canViewRevisionMounts(t, v, revMounts) {
			return true, nil
		}
	}
	return false, nil
}

// noStoreWriter ensures Cache-Control stays private, no-store even when http.ServeContent's error paths
// (such as 416 Range Not Satisfiable) attempt to delete or omit Cache-Control headers.
type noStoreWriter struct {
	http.ResponseWriter
	cacheControl string
}

func (w noStoreWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", w.cacheControl)
	w.ResponseWriter.WriteHeader(code)
}

func (w noStoreWriter) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", w.cacheControl)
	return w.ResponseWriter.Write(p)
}

func (w noStoreWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (s *Server) serveAssetRange(w http.ResponseWriter, r *http.Request, asset assetMeta, cacheControl string) {
	respW := w
	if strings.Contains(cacheControl, "no-store") {
		respW = noStoreWriter{ResponseWriter: w, cacheControl: cacheControl}
	}

	// Fallback multi-range GET requests to a single full sequential GET so range-ignoring upstreams
	// (such as WebDAV servers answering 200 OK) only stream and skip once.
	rangeHeader := strings.TrimSpace(r.Header.Get("Range"))
	if r.Method == http.MethodGet && strings.Contains(rangeHeader, ",") {
		r2 := r.Clone(r.Context())
		r2.Header.Del("Range")
		r = r2
		rangeHeader = ""
	}

	seeker := newStoreRangeSeeker(r.Context(), s.store, asset.StoragePath, asset.SizeBytes, rangeHeader)
	defer func() { _ = seeker.Close() }()

	// Probe storage on GET before http.ServeContent commits 200/206 status headers so missing objects
	// return 404 JSON instead of 200 + empty body (while leaving HEAD purely metadata-only and 416 untouched).
	if r.Method == http.MethodGet && seeker.size > 0 {
		probeStart := int64(0)
		probeEnd := seeker.size - 1
		shouldProbe := rangeHeader == ""
		if !shouldProbe && seeker.hasHint {
			probeStart, probeEnd = seeker.hintStart, seeker.hintEnd
			shouldProbe = true
		}
		if shouldProbe {
			if err := seeker.prime(probeStart, probeEnd); err != nil {
				if strings.Contains(cacheControl, "no-store") {
					respW.Header().Set("Cache-Control", cacheControl)
				}
				if errors.Is(err, storage.ErrNotFound) {
					writeError(respW, http.StatusNotFound, "not_found", "asset is missing from storage")
					return
				}
				s.storageError(respW, r, err)
				return
			}
		}
	}

	respW.Header().Set("Content-Type", asset.MimeType)
	respW.Header().Set("Cache-Control", cacheControl)
	respW.Header().Set("X-Content-Type-Options", "nosniff")
	// An SVG opened directly must not run scripts.
	respW.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	respW.Header().Set("Accept-Ranges", "bytes")

	http.ServeContent(respW, r, "", asset.modTime(), seeker)
}

// storeRangeSeeker implements io.ReadSeeker over Store.OpenRange + authoritative SQLite sizeBytes
// so Go's stdlib http.ServeContent serves HEAD (0 reads), single/suffix ranges, and full GETs.
type storeRangeSeeker struct {
	ctx         context.Context
	store       storage.Store
	path        string
	size        int64
	rangeHeader string
	hasHint     bool
	hintStart   int64
	hintEnd     int64

	offset       int64
	streamPos    int64
	openedEnd    int64
	rc           io.ReadCloser
	prematureEOF bool

	hasPrimed   bool
	primedStart int64
	primedEnd   int64
	primedRC    io.ReadCloser
}

func newStoreRangeSeeker(ctx context.Context, store storage.Store, objectPath string, size int64, rangeHeader string) *storeRangeSeeker {
	s := &storeRangeSeeker{
		ctx:         ctx,
		store:       store,
		path:        objectPath,
		size:        max(size, 0),
		rangeHeader: rangeHeader,
	}
	s.hintStart, s.hintEnd, s.hasHint = parseSingleRangeHint(rangeHeader, s.size)
	return s
}

func (s *storeRangeSeeker) prime(start, end int64) error {
	rc, err := s.store.OpenRange(s.ctx, s.path, start, end)
	if err != nil {
		return err
	}
	s.hasPrimed = true
	s.primedStart = start
	s.primedEnd = end
	s.primedRC = rc
	return nil
}

func (s *storeRangeSeeker) Close() error {
	var err error
	if s.rc != nil {
		err = s.rc.Close()
		s.rc = nil
	}
	if s.primedRC != nil {
		err = errors.Join(err, s.primedRC.Close())
		s.primedRC = nil
		s.hasPrimed = false
	}
	return err
}

func (s *storeRangeSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = s.offset + offset
	case io.SeekEnd:
		next = s.size + offset
	default:
		return 0, errors.New("api: invalid whence")
	}
	if next < 0 {
		return 0, errors.New("api: negative seek offset")
	}
	if s.rc != nil && next != s.streamPos {
		_ = s.rc.Close()
		s.rc = nil
	}
	s.offset = next
	return next, nil
}

func (s *storeRangeSeeker) Read(p []byte) (int, error) {
	if s.prematureEOF {
		return 0, io.ErrUnexpectedEOF
	}
	if s.offset >= s.size {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if s.rc == nil {
			end := s.size - 1
			if s.hasHint && s.offset >= s.hintStart && s.offset <= s.hintEnd {
				end = s.hintEnd
			} else if strings.Contains(s.rangeHeader, ",") {
				end = min(s.offset+rangeWindowSize-1, s.size-1)
			}
			if s.hasPrimed && s.primedRC != nil {
				rc := s.primedRC
				pStart, pEnd := s.primedStart, s.primedEnd
				s.primedRC = nil
				s.hasPrimed = false
				if s.offset == pStart {
					s.rc = rc
					s.streamPos = pStart
					s.openedEnd = pEnd
				} else {
					_ = rc.Close()
				}
			}
			if s.rc == nil {
				rc, err := s.store.OpenRange(s.ctx, s.path, s.offset, end)
				if err != nil {
					return 0, err
				}
				s.rc = rc
				s.streamPos = s.offset
				s.openedEnd = end
			}
		}
		n, err := s.rc.Read(p)
		if n > 0 {
			s.offset += int64(n)
			s.streamPos += int64(n)
		}
		if errors.Is(err, io.EOF) {
			_ = s.rc.Close()
			s.rc = nil
			// If the backend ended the stream before delivering through openedEnd, the underlying
			// object is shorter than assets.size_bytes: fail loudly instead of reopening in a loop.
			if s.offset <= s.openedEnd {
				s.prematureEOF = true
				if n > 0 {
					return n, nil
				}
				return 0, io.ErrUnexpectedEOF
			}
			if n > 0 {
				return n, nil
			}
			if s.offset >= s.size {
				return 0, io.EOF
			}
			continue
		}
		return n, err
	}
}

func parseSingleRangeHint(header string, size int64) (start, end int64, ok bool) {
	if size <= 0 || header == "" {
		return 0, 0, false
	}
	const prefix = "bytes="
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return 0, 0, false
	}
	spec := strings.TrimSpace(header[len(prefix):])
	if spec == "" || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	left, right, hasDash := strings.Cut(spec, "-")
	if !hasDash {
		return 0, 0, false
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		// Suffix range: bytes=-N
		suffix, err := strconv.ParseInt(right, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix >= size {
			return 0, size - 1, true
		}
		return size - suffix, size - 1, true
	}
	sVal, err := strconv.ParseInt(left, 10, 64)
	if err != nil || sVal < 0 || sVal >= size {
		return 0, 0, false
	}
	if right == "" {
		return sVal, size - 1, true
	}
	eVal, err := strconv.ParseInt(right, 10, 64)
	if err != nil || eVal < sVal {
		return 0, 0, false
	}
	if eVal >= size {
		eVal = size - 1
	}
	return sVal, eVal, true
}
