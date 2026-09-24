package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/schmitz-chris/stashbert/internal/store"
)

// tableRows returns all rows of table ordered by rowid as text, one line per
// row with the column names and values.
func tableRows(t *testing.T, ctx context.Context, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY rowid")
	if err != nil {
		t.Fatalf("query %s: %v", table, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", table, err)
	}
	var b strings.Builder
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		for i, c := range cols {
			fmt.Fprintf(&b, "%s=%v ", c, values[i])
		}
		b.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	return b.String()
}

// dbVersion returns the current goose version of the database of p.
func dbVersion(t *testing.T, ctx context.Context, p *goose.Provider) int64 {
	t.Helper()
	v, err := p.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("GetDBVersion: %v", err)
	}
	return v
}

// productMarked returns the stored marked of the product id.
func productMarked(t *testing.T, ctx context.Context, db *sql.DB, id string) int {
	t.Helper()
	var marked int
	if err := db.QueryRowContext(ctx, "SELECT marked FROM products WHERE id = ?", id).Scan(&marked); err != nil {
		t.Fatalf("read marked of %s: %v", id, err)
	}
	return marked
}

// TestMigrateMarked applies 0003 to a database at version 0002 with a
// product, migrates it down to 0002 and up again.
func TestMigrateMarked(t *testing.T) {
	ctx := testContext(t)
	db := openDB(t, ctx)
	// p is not closed: Close would close db, which openDB closes after the test.
	p, err := goose.NewProvider(goose.DialectSQLite3, db, store.Migrations)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := p.UpTo(ctx, 2); err != nil {
		t.Fatalf("migrate to 0002: %v", err)
	}
	mustInsert(t, ctx, db, "products", "p", row{"stock": 2, "target": 5})
	schemaBefore, productsBefore := schemaState(t, ctx, db), tableRows(t, ctx, db, "products")

	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("migrate to 0003: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 3 {
		t.Fatalf("version = %d, want 3", v)
	}
	if m := productMarked(t, ctx, db, "p"); m != 0 {
		t.Errorf("marked of the existing product = %d, want 0", m)
	}
	exec(t, ctx, db, "UPDATE products SET marked = 1 WHERE id = 'p'")

	if _, err := p.Down(ctx); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 2 {
		t.Errorf("version after down = %d, want 2", v)
	}
	if after := schemaState(t, ctx, db); after != schemaBefore {
		t.Errorf("schema after down:\n%s\nwant as before 0003:\n%s", after, schemaBefore)
	}
	if after := tableRows(t, ctx, db, "products"); after != productsBefore {
		t.Errorf("products after down:\n%s\nwant as before 0003:\n%s", after, productsBefore)
	}

	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("migrate to 0003 after down: %v", err)
	}
	if m := productMarked(t, ctx, db, "p"); m != 0 {
		t.Errorf("marked after migrating up again = %d, want 0", m)
	}
}
