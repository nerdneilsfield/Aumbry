package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/nerdneilsfield/aumbry/internal/storage"
)

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash := chi.URLParam(r, "hash")
	if !hashPattern.MatchString(hash) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	var asset struct {
		MimeType    string `db:"mime_type"`
		StoragePath string `db:"storage_path"`
	}
	err := s.db.GetContext(ctx, &asset, `SELECT mime_type, storage_path FROM assets WHERE hash = ?`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// An asset is readable when any item that references it is readable.
	if v := s.viewerFor(r, t); !v.admin {
		var mounts []string
		err := s.db.SelectContext(ctx, &mounts, `SELECT DISTINCT di.directory_id FROM item_assets ia
			JOIN directory_items di ON di.item_id = ia.item_id WHERE ia.asset_hash = ?`, hash)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if len(t.visibleDirs(v, mounts)) == 0 {
			writeError(w, http.StatusForbidden, "forbidden", "this asset is not accessible")
			return
		}
	}
	body, err := s.store.Open(ctx, asset.StoragePath)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "asset is missing from storage")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", asset.MimeType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// An SVG opened directly must not run scripts.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, body)
}
