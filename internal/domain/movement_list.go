package domain

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// MovementQuery selects a page of movements (architecture.md 6.2, listMovements).
type MovementQuery struct {
	// ProductID limits the page to the movements of one product if it is not nil.
	ProductID *string
	// Cursor is the id of the last movement of the previous page or nil for
	// the first page.
	Cursor *string
	// Limit is the maximum number of movements on the page. It must be at
	// least 1; the spec allows 1 to 200.
	Limit int64
}

// MovementPage is a page of movements.
type MovementPage struct {
	// Items holds the movements by id descending. It is never nil.
	Items []Movement
	// NextCursor is the id of the last item if more movements follow,
	// otherwise nil.
	NextCursor *string
}

// ListMovements returns the page of movements selected by in, newest first:
// at most in.Limit movements by id descending, only those with an id below
// in.Cursor if it is set and only those of in.ProductID if it is set. An
// unknown product results in an empty page. The queries use the primary key
// or the index (product_id, id) and read one movement more than in.Limit to
// find out whether another page follows. A cursor that is no UUID results in
// 400 invalid_request.
func ListMovements(ctx context.Context, sqlDB *sql.DB, in MovementQuery) (MovementPage, error) {
	var cursor string
	if in.Cursor != nil {
		u, err := uuid.Parse(*in.Cursor)
		if err != nil {
			return MovementPage{}, httpx.BadRequest("cursor ist keine UUID")
		}
		// The ids are stored in lower case, so the comparison needs the
		// canonical form.
		cursor = u.String()
	}

	q := db.New(sqlDB)
	n := in.Limit + 1
	var (
		rows []db.Movement
		err  error
	)
	switch {
	case in.ProductID == nil && in.Cursor == nil:
		rows, err = q.ListMovements(ctx, n)
	case in.ProductID == nil:
		rows, err = q.ListMovementsBefore(ctx, db.ListMovementsBeforeParams{ID: cursor, Limit: n})
	case in.Cursor == nil:
		rows, err = q.ListProductMovements(ctx, db.ListProductMovementsParams{ProductID: *in.ProductID, Limit: n})
	default:
		rows, err = q.ListProductMovementsBefore(ctx, db.ListProductMovementsBeforeParams{
			ProductID: *in.ProductID, ID: cursor, Limit: n,
		})
	}
	if err != nil {
		return MovementPage{}, fmt.Errorf("list movements: %w", err)
	}

	more := int64(len(rows)) > in.Limit
	if more {
		rows = rows[:in.Limit]
	}
	items := make([]Movement, len(rows))
	for i, row := range rows {
		m, err := movementFromDB(row)
		if err != nil {
			return MovementPage{}, err
		}
		items[i] = m
	}
	page := MovementPage{Items: items}
	if more {
		page.NextCursor = &items[len(items)-1].ID
	}
	return page, nil
}
