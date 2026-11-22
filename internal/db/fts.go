package db

import (
	"context"
	"database/sql"

	"github.com/nerdneilsfield/aumbry/internal/cjk"
)

// Execer is satisfied by *sql.Tx and *sqlx.Tx; callers pass their open transaction so the search
// tables change together with the item.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// DeleteItemFTS removes the item's rows from both full-text tables.
func DeleteItemFTS(ctx context.Context, tx Execer, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM items_fts WHERE item_id = ?`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM items_fts_cjk WHERE item_id = ?`, id)
	return err
}

// ReplaceItemFTS rewrites the item's rows in items_fts (original text, trigram) and items_fts_cjk
// (bigram-expanded text, see internal/cjk).
func ReplaceItemFTS(ctx context.Context, tx Execer, id, title, summary, tags, plain string) error {
	if err := DeleteItemFTS(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO items_fts (item_id, title, summary, tags, plain_text) VALUES (?, ?, ?, ?, ?)`,
		id, title, summary, tags, plain); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO items_fts_cjk (item_id, title, summary, tags, plain_text) VALUES (?, ?, ?, ?, ?)`,
		id, cjk.Expand(title), cjk.Expand(summary), cjk.Expand(tags), cjk.Expand(plain))
	return err
}
