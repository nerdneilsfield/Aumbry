package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"

	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const (
	dirTokenPrefix   = "ad_"
	dirTokenAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	dirTokenRandLen  = 32
	maxFeedItems     = 50
)

func generateDirToken() (plaintext, tokenHash string, err error) {
	raw, err := gonanoid.Generate(dirTokenAlphabet, dirTokenRandLen)
	if err != nil {
		return "", "", err
	}
	plaintext = dirTokenPrefix + raw
	return plaintext, auth.TokenDigest(plaintext), nil
}

type dirTokenInfo struct {
	ID          string `db:"id" json:"id"`
	DirectoryID string `db:"directory_id" json:"directory_id"`
	Label       string `db:"label" json:"label"`
	CreatedAt   string `db:"created_at" json:"created_at"`
}

func (s *Server) adminListDirTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirID := chi.URLParam(r, "id")
	exists, err := s.dirExists(ctx, dirID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}
	var tokens []dirTokenInfo
	if err = s.db.SelectContext(ctx, &tokens, `SELECT id, directory_id, label,
		strftime('%Y-%m-%dT%H:%M:%SZ', created_at) AS created_at
		FROM directory_tokens WHERE directory_id = ? ORDER BY created_at DESC, id ASC`, dirID); err != nil {
		internalError(w, r, err)
		return
	}
	if tokens == nil {
		tokens = []dirTokenInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"directory_id": dirID,
		"tokens":       tokens,
	})
}

func (s *Server) adminCreateDirToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirID := chi.URLParam(r, "id")
	var body struct {
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" || utf8.RuneCountInString(label) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_request", "label must be 1 to 100 characters")
		return
	}
	exists, err := s.dirExists(ctx, dirID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}

	tokID, err := newID()
	if err != nil {
		internalError(w, r, err)
		return
	}
	plaintext, hash, err := generateDirToken()
	if err != nil {
		internalError(w, r, err)
		return
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO directory_tokens (id, directory_id, label, token_hash) VALUES (?, ?, ?, ?)`,
		tokID, dirID, label, hash); err != nil {
		internalError(w, r, err)
		return
	}
	var createdAt string
	if err = s.db.GetContext(ctx, &createdAt, `SELECT strftime('%Y-%m-%dT%H:%M:%SZ', created_at) FROM directory_tokens WHERE id = ?`, tokID); err != nil {
		internalError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":           tokID,
		"directory_id": dirID,
		"label":        label,
		"token":        plaintext,
		"created_at":   createdAt,
	})
}

func (s *Server) adminRotateDirToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirID := chi.URLParam(r, "id")
	tokID := chi.URLParam(r, "token_id")
	plaintext, hash, err := generateDirToken()
	if err != nil {
		internalError(w, r, err)
		return
	}
	res, err := s.db.ExecContext(ctx, `UPDATE directory_tokens SET token_hash = ? WHERE id = ? AND directory_id = ?`, hash, tokID, dirID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "directory token not found")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           tokID,
		"directory_id": dirID,
		"token":        plaintext,
	})
}

func (s *Server) adminDeleteDirToken(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM directory_tokens WHERE id = ? AND directory_id = ?`,
		chi.URLParam(r, "token_id"), chi.URLParam(r, "id"))
	if err != nil {
		internalError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "directory token not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolvePublicURL(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if len(s.trustedProxies) > 0 {
		if peer, ok := parseRemoteAddr(r.RemoteAddr); ok && inAnyPrefix(peer, s.trustedProxies) {
			if fp := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); fp != "" {
				first, _, _ := strings.Cut(fp, ",")
				if cand := strings.ToLower(strings.TrimSpace(first)); cand == "http" || cand == "https" {
					scheme = cand
				}
			}
			if fh := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); fh != "" {
				first, _, _ := strings.Cut(fh, ",")
				if cand := strings.TrimSpace(first); cand != "" {
					host = cand
				}
			}
		}
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func verifyDirTokenTx(ctx context.Context, q sqlx.QueryerContext, dirID, rawTok string) (bool, error) {
	if !strings.HasPrefix(rawTok, dirTokenPrefix) || len(rawTok) <= len(dirTokenPrefix) {
		return false, nil
	}
	var cnt int
	err := sqlx.GetContext(ctx, q, &cnt, `SELECT count(*) FROM directory_tokens WHERE directory_id = ? AND token_hash = ?`,
		dirID, auth.TokenDigest(rawTok))
	return cnt > 0, err
}

func (s *Server) authorizeFeed(ctx context.Context, q sqlx.QueryerContext, t *dirTree, r *http.Request, dirID string) (dir *dirRec, allowedDirs []string, rawTok string, protected bool, ok bool, status int, code, msg string) {
	dir = t.dirs[dirID]
	if dir == nil {
		return nil, nil, "", false, false, http.StatusNotFound, "not_found", "directory not found"
	}
	rawTok = queryTokenFromContext(r)
	isPub := t.isPublic(dirID)
	protected = !isPub

	if isPub && rawTok == "" {
		return dir, t.publicSubtree(dirID), "", false, true, http.StatusOK, "", ""
	}
	if rawTok == "" {
		return nil, nil, "", true, false, http.StatusUnauthorized, "unauthorized", "directory read token is required"
	}
	valid, err := verifyDirTokenTx(ctx, q, dirID, rawTok)
	if err != nil {
		return nil, nil, "", true, false, http.StatusInternalServerError, "internal", err.Error()
	}
	if !valid {
		return nil, nil, "", true, false, http.StatusUnauthorized, "unauthorized", "invalid directory read token"
	}
	// If the directory itself was protected OR a valid token was explicitly supplied on dirID,
	// apply A1 subtree boundary (stops at any independently locked descendant).
	allowed := t.subtreeVisibleByDirToken(dirID)
	if len(allowed) == 0 {
		return nil, nil, "", true, false, http.StatusForbidden, "forbidden", "directory is not accessible"
	}
	return dir, allowed, rawTok, protected, true, http.StatusOK, "", ""
}

type rssDocument struct {
	XMLName     xml.Name   `xml:"rss"`
	Version     string     `xml:"version,attr"`
	XmlnsDC     string     `xml:"xmlns:dc,attr"`
	XmlnsItunes string     `xml:"xmlns:itunes,attr,omitempty"`
	Channel     rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title          string    `xml:"title"`
	Link           string    `xml:"link"`
	Description    string    `xml:"description"`
	Language       string    `xml:"language"`
	ItunesExplicit string    `xml:"itunes:explicit,omitempty"`
	Items          []rssItem `xml:"item"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type rssItem struct {
	Title          string        `xml:"title"`
	Link           string        `xml:"link"`
	GUID           rssGUID       `xml:"guid"`
	PubDate        string        `xml:"pubDate"`
	Creator        string        `xml:"dc:creator"`
	Category       []string      `xml:"category,omitempty"`
	Description    string        `xml:"description"`
	Enclosure      *rssEnclosure `xml:"enclosure,omitempty"`
	ItunesDuration string        `xml:"itunes:duration,omitempty"`
}

type feedItemRow struct {
	ID            string         `db:"id"`
	Type          string         `db:"type"`
	Title         string         `db:"title"`
	ReportDate    string         `db:"report_date"`
	CreatedAt     string         `db:"created_at"`
	Summary       string         `db:"summary"`
	TagsJSON      string         `db:"tags_json"`
	SourceAgent   string         `db:"source_agent"`
	AudioHash     sql.NullString `db:"primary_audio_hash"`
	ChaptersJSON  string         `db:"primary_audio_chapters_json"`
	AudioMIME     sql.NullString `db:"audio_mime"`
	AudioSize     sql.NullInt64  `db:"audio_size"`
	AudioDuration sql.NullInt64  `db:"audio_duration"`
}

func (s *Server) getDirRSS(w http.ResponseWriter, r *http.Request) {
	s.serveFeed(w, r, false)
}

func (s *Server) getDirPodcast(w http.ResponseWriter, r *http.Request) {
	s.serveFeed(w, r, true)
}

func (s *Server) serveFeed(w http.ResponseWriter, r *http.Request, podcastOnly bool) {
	ctx := r.Context()
	dirID := chi.URLParam(r, "dir_id")

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	dir, allowedDirs, rawTok, protected, ok, status, code, msg := s.authorizeFeed(ctx, tx, t, r, dirID)
	if !ok {
		if status == http.StatusInternalServerError {
			internalError(w, r, errors.New(msg))
			return
		}
		writeError(w, status, code, msg)
		return
	}

	var rows []feedItemRow
	if len(allowedDirs) > 0 {
		extraWhere := ""
		if podcastOnly {
			extraWhere = ` AND i.primary_audio_hash IS NOT NULL`
		}
		query, args, inErr := sqlx.In(`SELECT DISTINCT i.id, i.type, i.title, i.report_date,
			strftime('%Y-%m-%dT%H:%M:%SZ', i.created_at) AS created_at,
			i.summary, i.tags_json, i.source_agent,
			i.primary_audio_hash, i.primary_audio_chapters_json,
			a.mime_type AS audio_mime, a.size_bytes AS audio_size, a.duration_seconds AS audio_duration
			FROM items i
			LEFT JOIN assets a ON a.hash = i.primary_audio_hash
			WHERE EXISTS (SELECT 1 FROM directory_items di WHERE di.item_id = i.id AND di.directory_id IN (?))`+extraWhere+`
			ORDER BY i.report_date DESC, i.created_at DESC, i.id ASC
			LIMIT ?`, allowedDirs, maxFeedItems)
		if inErr != nil {
			internalError(w, r, inErr)
			return
		}
		if selErr := sqlx.SelectContext(ctx, tx, &rows, query, args...); selErr != nil {
			internalError(w, r, selErr)
			return
		}
	}
	_ = tx.Rollback()

	baseURL := s.resolvePublicURL(r)
	hasAudio := podcastOnly
	items := make([]rssItem, 0, len(rows))
	for _, row := range rows {
		prefix := "/r/"
		if row.Type == "post" {
			prefix = "/p/"
		}
		itemLink := baseURL + prefix + row.ID
		desc := row.Summary

		var enc *rssEnclosure
		var itunesDur string
		if row.AudioHash.Valid && row.AudioHash.String != "" && row.AudioMIME.Valid && row.AudioSize.Valid {
			hasAudio = true
			var encURL string
			if protected || rawTok != "" {
				encURL = fmt.Sprintf("%s/feeds/%s/assets/%s?token=%s", baseURL, dirID, row.AudioHash.String, rawTok)
			} else {
				encURL = fmt.Sprintf("%s/api/v1/assets/%s", baseURL, row.AudioHash.String)
			}
			enc = &rssEnclosure{
				URL:    encURL,
				Length: row.AudioSize.Int64,
				Type:   row.AudioMIME.String,
			}
			if podcastOnly && row.AudioDuration.Valid {
				itunesDur = fmt.Sprintf("%d", row.AudioDuration.Int64)
			}
		}
		if podcastOnly {
			desc = formatPodcastDescription(row.Summary, row.ChaptersJSON)
		}

		items = append(items, rssItem{
			Title:          row.Title,
			Link:           itemLink,
			GUID:           rssGUID{IsPermaLink: "false", Value: "aumbry:item:" + row.ID},
			PubDate:        formatRSSPubDate(row.ReportDate, row.CreatedAt),
			Creator:        row.SourceAgent,
			Category:       parseTags(row.TagsJSON),
			Description:    desc,
			Enclosure:      enc,
			ItunesDuration: itunesDur,
		})
	}

	channelDesc := dir.Description
	if channelDesc == "" {
		channelDesc = dir.Name
	}
	doc := rssDocument{
		Version: "2.0",
		XmlnsDC: "http://purl.org/dc/elements/1.1/",
		Channel: rssChannel{
			Title:       dir.Name,
			Link:        baseURL + "/d/" + dirID,
			Description: channelDesc,
			Language:    "zh-CN",
			Items:       items,
		},
	}
	if hasAudio {
		doc.XmlnsItunes = "http://www.itunes.com/dtds/podcast-1.0.dtd"
		doc.Channel.ItunesExplicit = "false"
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if encErr := enc.Encode(doc); encErr != nil {
		internalError(w, r, encErr)
		return
	}
	buf.WriteByte('\n')

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if protected || rawTok != "" {
		w.Header().Set("Cache-Control", "private, no-store")
	} else if s.publicURL == "" {
		w.Header().Set("Cache-Control", "private, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func formatRSSPubDate(reportDate, createdAt string) string {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(reportDate))
	if err != nil {
		if c, cErr := time.Parse(time.RFC3339, createdAt); cErr == nil {
			return c.UTC().Format(time.RFC1123Z)
		}
		return time.Unix(0, 0).UTC().Format(time.RFC1123Z)
	}
	hour, minute, sec := 0, 0, 0
	if c, cErr := time.Parse(time.RFC3339, createdAt); cErr == nil {
		utc := c.UTC()
		hour, minute, sec = utc.Hour(), utc.Minute(), utc.Second()
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, sec, 0, time.UTC).Format(time.RFC1123Z)
}

func formatPodcastDescription(summary, chaptersJSON string) string {
	var chapters []schema.AudioChapter
	if err := json.Unmarshal([]byte(chaptersJSON), &chapters); err != nil || len(chapters) == 0 {
		return summary
	}
	var b strings.Builder
	b.WriteString(summary)
	b.WriteString("\n\nChapters:\n")
	for _, ch := range chapters {
		fmt.Fprintf(&b, "%s — %s\n", ch.Start, ch.Title)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *Server) getFeedAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirID := chi.URLParam(r, "dir_id")
	hash := chi.URLParam(r, "hash")
	if !hashPattern.MatchString(hash) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
		return
	}

	rawTok := queryTokenFromContext(r)
	if !strings.HasPrefix(rawTok, dirTokenPrefix) || len(rawTok) <= len(dirTokenPrefix) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "directory read token is required")
		return
	}

	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	t, err := loadTreeFrom(ctx, tx)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if t.dirs[dirID] == nil {
		writeError(w, http.StatusNotFound, "not_found", "directory not found")
		return
	}
	valid, err := verifyDirTokenTx(ctx, tx, dirID, rawTok)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid directory read token")
		return
	}
	allowedDirs := t.subtreeVisibleByDirToken(dirID)
	if len(allowedDirs) == 0 {
		writeError(w, http.StatusForbidden, "forbidden", "directory is not accessible")
		return
	}

	asset, err := loadAssetMetaTx(ctx, tx, hash)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "asset not found")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	var matched int
	query, args, err := sqlx.In(`SELECT count(*) FROM item_assets ia
		JOIN directory_items di ON di.item_id = ia.item_id
		WHERE ia.asset_hash = ? AND di.directory_id IN (?)`, hash, allowedDirs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err = sqlx.GetContext(ctx, tx, &matched, query, args...); err != nil {
		internalError(w, r, err)
		return
	}
	_ = tx.Rollback()

	if matched == 0 {
		writeError(w, http.StatusForbidden, "forbidden", "this asset is not accessible with this directory token")
		return
	}

	s.serveAssetRange(w, r, asset, "private, no-store")
}
