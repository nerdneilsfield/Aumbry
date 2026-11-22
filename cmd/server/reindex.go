package main

import (
	"context"
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
		Short: "Rebuild items.plain_text and items_fts from the stored XML (writing runs require the server to be stopped)",
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

			store, err := storage.NewWebDAV(res.Config.WebDAV.URL, res.Config.WebDAV.User, res.Config.WebDAV.Password)
			if err != nil {
				return fmt.Errorf("configure WebDAV storage: %w", err)
			}
			return runReindex(cmd.Context(), database, store, dryRun, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing anything (safe while server runs)")
	return cmd
}

// reindexEntry is the recomputed search projection of one item.
type reindexEntry struct {
	id      string
	title   string
	summary string
	tags    string
	plain   string
	changed bool // items.plain_text differs from the recomputed text
}

// runReindex re-derives items.plain_text and the items_fts row of every item from its stored XML
// with the same schema.Document.PlainText projection that ingest uses. An item that cannot be read
// or parsed is reported and skipped; the others are still processed and the run fails at the end.
func runReindex(ctx context.Context, database *sqlx.DB, store storage.Store, dryRun bool, out io.Writer) error {
	var items []struct {
		ID      string `db:"id"`
		XMLPath string `db:"xml_storage_path"`
	}
	if err := database.SelectContext(ctx, &items, `SELECT id, xml_storage_path FROM items ORDER BY id`); err != nil {
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
			entry, err := projectItem(ctx, database, store, item.ID, item.XMLPath)
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

func projectItem(ctx context.Context, database *sqlx.DB, store storage.Store, id, xmlPath string) (reindexEntry, error) {
	r, err := store.Open(ctx, xmlPath)
	if err != nil {
		return reindexEntry{}, fmt.Errorf("read xml: %w", err)
	}
	input, err := io.ReadAll(r)
	if err = errors.Join(err, r.Close()); err != nil {
		return reindexEntry{}, fmt.Errorf("read xml: %w", err)
	}
	// Stored XML already has its figure src rewritten to asset URLs, so only the figure checks are skipped.
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
	return reindexEntry{
		id: id, title: doc.Meta.Title, summary: doc.Meta.Summary,
		tags: strings.Join(doc.Meta.Tags, " "), plain: plain, changed: !same,
	}, nil
}

// writeReindexBatch updates both projections in one transaction, mirroring ingest's saveItem:
// the short-term LIKE search reads items.plain_text and the long-term search reads items_fts.
func writeReindexBatch(ctx context.Context, database *sqlx.DB, batch []reindexEntry) error {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, e := range batch {
		if _, err = tx.ExecContext(ctx, `UPDATE items SET plain_text = ? WHERE id = ?`, e.plain, e.id); err != nil {
			return fmt.Errorf("update item %s: %w", e.id, err)
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM items_fts WHERE item_id = ?`, e.id); err != nil {
			return fmt.Errorf("clear fts for item %s: %w", e.id, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO items_fts (item_id, title, summary, tags, plain_text) VALUES (?, ?, ?, ?, ?)`,
			e.id, e.title, e.summary, e.tags, e.plain); err != nil {
			return fmt.Errorf("insert fts for item %s: %w", e.id, err)
		}
	}
	return tx.Commit()
}
