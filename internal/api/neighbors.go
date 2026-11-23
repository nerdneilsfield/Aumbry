package api

import (
	"context"
	"database/sql"
	"slices"

	"github.com/jmoiron/sqlx"
)

// itemNeighbors is the previous and next item, by date, inside one directory the caller may open.
// Prev is the earlier item and Next the later one; either is nil at the ends.
type itemNeighbors struct {
	Dir  dirRef       `json:"dir"`
	Prev *itemSummary `json:"prev"`
	Next *itemSummary `json:"next"`
}

// neighborsForFrom returns the neighbors of item id among the items mounted directly in one of visible,
// the directories v may open. hint (the directory the reader came from) wins when it is one of them;
// otherwise the first visible mount is used, so a hint naming a hidden directory changes nothing.
// It returns nil when the item has no visible mount.
func neighborsForFrom(ctx context.Context, q sqlx.QueryerContext, t *dirTree, visible []string, hint, id string) (*itemNeighbors, error) {
	if len(visible) == 0 {
		return nil, nil
	}
	dir := visible[0]
	if slices.Contains(visible, hint) {
		dir = hint
	}
	rec := t.dirs[dir]
	if rec == nil {
		return nil, nil
	}

	// The window order is the exact reverse of the directory listing (newest first), so "previous"
	// is the entry listed just below the item and "next" the one just above it.
	var ids struct {
		Prev sql.NullString `db:"prev_id"`
		Next sql.NullString `db:"next_id"`
	}
	err := sqlx.GetContext(ctx, q, &ids, `SELECT prev_id, next_id FROM (
		SELECT i.id, LAG(i.id) OVER w AS prev_id, LEAD(i.id) OVER w AS next_id
		FROM items i JOIN directory_items di ON di.item_id = i.id WHERE di.directory_id = ?
		WINDOW w AS (ORDER BY i.report_date, i.created_at, i.id DESC)
	) WHERE id = ?`, dir, id)
	if err != nil {
		return nil, err
	}
	out := &itemNeighbors{Dir: dirRef{ID: dir, Name: rec.Name}}
	if out.Prev, err = summaryOfFrom(ctx, q, ids.Prev); err != nil {
		return nil, err
	}
	if out.Next, err = summaryOfFrom(ctx, q, ids.Next); err != nil {
		return nil, err
	}
	return out, nil
}

// summaryOfFrom loads one item summary; nil for a NULL id.
func summaryOfFrom(ctx context.Context, q sqlx.QueryerContext, id sql.NullString) (*itemSummary, error) {
	if !id.Valid {
		return nil, nil
	}
	var row itemRow
	if err := sqlx.GetContext(ctx, q, &row, `SELECT `+itemColumns+` FROM items i WHERE i.id = ?`, id.String); err != nil {
		return nil, err
	}
	sum := row.summary()
	return &sum, nil
}
