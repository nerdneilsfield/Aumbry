package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

const reindexBatchSize = 100

func newReindexCmd(configPath *string) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild search indexes, backlinks, primary audio projections and baseline revisions (writing runs require the server to be stopped)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := loadAndWarn(cmd, *configPath)
			if err != nil {
				return err
			}
			if !dryRun {
				lock, lockErr := lockOffline(res.Config.DataDir, "reindex", "--dry-run")
				if lockErr != nil {
					return lockErr
				}
				defer func() { _ = lock.Close() }()
			}

			database, err := db.OpenExisting(res.Config.DataDir)
			if err != nil {
				return err
			}
			defer func() { _ = database.Close() }()

			store, err := openStore(res.Config, false)
			if err != nil {
				return fmt.Errorf("configure %s storage: %w", res.Config.Storage, err)
			}
			return runReindex(cmd.Context(), database, store, dryRun, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing anything (safe while server runs)")
	return cmd
}

// reindexEntry is the recomputed projection of one item.
type reindexEntry struct {
	id               string
	title            string
	summary          string
	reportDate       string
	tags             string
	plain            string
	xmlPath          string
	xmlSHA256        string
	createdBy        string
	links            []schema.OutgoingLink
	primaryAudioHash string
	chaptersJSON     string
	changed          bool // items.plain_text differs from the recomputed text
}

// runReindex re-derives items.plain_text, items_fts, item_links, primary_audio_*, and backfills
// rev=1 for mounted pre-v0.1.4 items from stored XML.
func runReindex(ctx context.Context, database *sqlx.DB, store storage.Store, dryRun bool, out io.Writer) error {
	var items []struct {
		ID        string `db:"id"`
		XMLPath   string `db:"xml_storage_path"`
		UpdatedBy string `db:"updated_by"`
	}
	if err := database.SelectContext(ctx, &items, `SELECT id, xml_storage_path, COALESCE(updated_by, '') AS updated_by FROM items ORDER BY id`); err != nil {
		return fmt.Errorf("query items: %w", err)
	}

	var failures []string
	changed, done := 0, 0
	for start := 0; start < len(items); start += reindexBatchSize {
		end := min(start+reindexBatchSize, len(items))
		var batch []reindexEntry
		for _, item := range items[start:end] {
			if err := ctx.Err(); err != nil {
				return err
			}
			entry, err := projectItem(ctx, database, store, item.ID, item.XMLPath, item.UpdatedBy)
			if err != nil {
				failures = append(failures, fmt.Sprintf("item %s (%s): %v", item.ID, item.XMLPath, err))
				continue
			}
			batch = append(batch, entry)
			if entry.changed {
				changed++
			}
		}
		done += len(batch)
		if dryRun {
			continue
		}
		if err := writeReindexBatch(ctx, database, batch); err != nil {
			return fmt.Errorf("write reindex batch: %w", err)
		}
	}

	verb := "reindexed"
	if dryRun {
		verb = "dry-run: would reindex"
	}
	for _, f := range failures {
		if _, err := fmt.Fprintf(out, "error: %s\n", f); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "%s %d of %d items (%d with changed plain_text), %d failed\n", verb, done, len(items), changed, len(failures)); err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("reindex: %d item(s) failed; their search data was left unchanged", len(failures))
	}
	return nil
}

func marshalCompactJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(buf.Bytes())), nil
}

func projectItem(ctx context.Context, database *sqlx.DB, store storage.Store, id, xmlPath, createdBy string) (reindexEntry, error) {
	r, err := store.Open(ctx, xmlPath)
	if err != nil {
		return reindexEntry{}, fmt.Errorf("read xml: %w", err)
	}
	input, err := io.ReadAll(r)
	if err = errors.Join(err, r.Close()); err != nil {
		return reindexEntry{}, fmt.Errorf("read xml: %w", err)
	}
	// Stored XML already has its asset srcs rewritten to asset URLs, so local path checks are skipped.
	doc, issues := schema.Parse(input, schema.Options{StoredAssets: true})
	if doc == nil || len(issues) > 0 {
		msgs := make([]string, 0, len(issues))
		for _, is := range issues {
			msgs = append(msgs, fmt.Sprintf("%s %s: %s", is.Code, is.Field, is.Message))
		}
		return reindexEntry{}, fmt.Errorf("stored xml is invalid: %s", strings.Join(msgs, "; "))
	}
	plain := doc.PlainText()
	var same bool
	if err = database.GetContext(ctx, &same, `SELECT plain_text = ? FROM items WHERE id = ?`, plain, id); err != nil {
		return reindexEntry{}, fmt.Errorf("read plain_text: %w", err)
	}

	var primaryHash string
	chaptersJSON := "[]"
	if pa := doc.PrimaryAudio(); pa != nil {
		primaryHash = pa.Hash
		chJSON, chErr := marshalCompactJSON(pa.Chapters)
		if chErr != nil {
			return reindexEntry{}, fmt.Errorf("encode audio chapters: %w", chErr)
		}
		chaptersJSON = chJSON
	}

	sum := sha256.Sum256(input)
	return reindexEntry{
		id:               id,
		title:            doc.Meta.Title,
		summary:          doc.Meta.Summary,
		reportDate:       doc.Meta.Date,
		tags:             strings.Join(doc.Meta.Tags, " "),
		plain:            plain,
		xmlPath:          xmlPath,
		xmlSHA256:        hex.EncodeToString(sum[:]),
		createdBy:        createdBy,
		links:            doc.OutgoingLinks(),
		primaryAudioHash: primaryHash,
		chaptersJSON:     chaptersJSON,
		changed:          !same,
	}, nil
}

// writeReindexBatch updates search projections, audio metadata, backlinks, and baseline revisions.
func writeReindexBatch(ctx context.Context, database *sqlx.DB, batch []reindexEntry) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	dirRows := []struct {
		ID           string `db:"id"`
		ParentID     string `db:"parent_id"`
		PasswordHash string `db:"password_hash"`
	}{}
	if err = tx.SelectContext(ctx, &dirRows, `SELECT id, COALESCE(parent_id, '') AS parent_id, COALESCE(password_hash, '') AS password_hash FROM directories`); err != nil {
		return fmt.Errorf("load directories: %w", err)
	}
	dirMap := make(map[string]struct{ parent, pw string }, len(dirRows))
	for _, d := range dirRows {
		dirMap[d.ID] = struct{ parent, pw string }{parent: d.ParentID, pw: d.PasswordHash}
	}
	dirWasProtected := func(dirID string) int {
		seen := make(map[string]bool)
		cur := dirID
		for cur != "" {
			if seen[cur] {
				return 1
			}
			seen[cur] = true
			d, ok := dirMap[cur]
			if !ok {
				return 1
			}
			if d.pw != "" {
				return 1
			}
			cur = d.parent
		}
		return 0
	}

	for _, e := range batch {
		if _, err = tx.ExecContext(ctx, `UPDATE items SET plain_text = ?, primary_audio_hash = NULLIF(?, ''),
			primary_audio_chapters_json = ? WHERE id = ?`, e.plain, e.primaryAudioHash, e.chaptersJSON, e.id); err != nil {
			return fmt.Errorf("update item %s: %w", e.id, err)
		}
		if err = db.ReplaceItemFTS(ctx, tx, e.id, e.title, e.summary, e.tags, e.plain); err != nil {
			return fmt.Errorf("rebuild fts for item %s: %w", e.id, err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM item_links WHERE source_item_id = ?`, e.id); err != nil {
			return fmt.Errorf("clear links for item %s: %w", e.id, err)
		}
		for _, link := range e.links {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_links (source_item_id, target_item_id, kind) VALUES (?, ?, ?)`,
				e.id, link.TargetItemID, link.Kind); err != nil {
				return fmt.Errorf("insert link for item %s: %w", e.id, err)
			}
		}

		// Backfill rev=1 for mounted pre-v0.1.4 items that have no revisions yet.
		var lastRev, revCount int
		if err = tx.GetContext(ctx, &lastRev, `SELECT last_rev FROM items WHERE id = ?`, e.id); err != nil {
			return fmt.Errorf("read last_rev for item %s: %w", e.id, err)
		}
		if err = tx.GetContext(ctx, &revCount, `SELECT count(*) FROM item_revisions WHERE item_id = ?`, e.id); err != nil {
			return fmt.Errorf("count revisions for item %s: %w", e.id, err)
		}
		if revCount == 0 && lastRev == 0 {
			var mounts []string
			if err = tx.SelectContext(ctx, &mounts, `SELECT directory_id FROM directory_items WHERE item_id = ? ORDER BY directory_id`, e.id); err != nil {
				return fmt.Errorf("list mounts for item %s: %w", e.id, err)
			}
			if len(mounts) > 0 {
				authorVerified := 0
				if e.createdBy != "" {
					authorVerified = 1
				}
				res, insErr := tx.ExecContext(ctx, `INSERT INTO item_revisions
					(item_id, rev, title, summary, report_date, xml_storage_path, xml_sha256, created_by, author_verified)
					VALUES (?, 1, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
					e.id, e.title, e.summary, e.reportDate, e.xmlPath, e.xmlSHA256, e.createdBy, authorVerified)
				if insErr != nil {
					return fmt.Errorf("backfill rev 1 for item %s: %w", e.id, insErr)
				}
				revID, idErr := res.LastInsertId()
				if idErr != nil {
					return fmt.Errorf("rev 1 id for item %s: %w", e.id, idErr)
				}
				if _, err = tx.ExecContext(ctx, `UPDATE items SET last_rev = 1 WHERE id = ? AND last_rev < 1`, e.id); err != nil {
					return fmt.Errorf("update last_rev for item %s: %w", e.id, err)
				}
				if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_assets (revision_id, asset_hash)
					SELECT ?, asset_hash FROM item_assets WHERE item_id = ?`, revID, e.id); err != nil {
					return fmt.Errorf("backfill rev 1 assets for item %s: %w", e.id, err)
				}
				for _, dirID := range mounts {
					if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_revision_mounts (revision_id, directory_id, was_protected) VALUES (?, ?, ?)`,
						revID, dirID, dirWasProtected(dirID)); err != nil {
						return fmt.Errorf("backfill rev 1 mount for item %s: %w", e.id, err)
					}
				}
			}
		}
	}
	return tx.Commit()
}
