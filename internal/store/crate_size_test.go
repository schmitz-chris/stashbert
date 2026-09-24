package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/schmitz-chris/stashbert/internal/store"
)

// productCrateSize returns the stored crate_size of the product id, nil for NULL.
func productCrateSize(t *testing.T, ctx context.Context, db *sql.DB, id string) *int64 {
	t.Helper()
	var crateSize *int64
	if err := db.QueryRowContext(ctx, "SELECT crate_size FROM products WHERE id = ?", id).Scan(&crateSize); err != nil {
		t.Fatalf("read crate_size of %s: %v", id, err)
	}
	return crateSize
}

// TestMigrateCrateSize applies 0004 to a database at version 0003 with a
// product, migrates it down to 0003 and up again.
func TestMigrateCrateSize(t *testing.T) {
	ctx := testContext(t)
	db := openDB(t, ctx)
	// p is not closed: Close would close db, which openDB closes after the test.
	p, err := goose.NewProvider(goose.DialectSQLite3, db, store.Migrations)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := p.UpTo(ctx, 3); err != nil {
		t.Fatalf("migrate to 0003: %v", err)
	}
	mustInsert(t, ctx, db, "products", "p", row{"stock": 2, "target": 5, "marked": 1})
	schemaBefore, productsBefore := schemaState(t, ctx, db), tableRows(t, ctx, db, "products")

	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatalf("migrate to 0004: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 4 {
		t.Fatalf("version = %d, want 4", v)
	}
	if c := productCrateSize(t, ctx, db, "p"); c != nil {
		t.Errorf("crate_size of the existing product = %d, want NULL", *c)
	}
	exec(t, ctx, db, "UPDATE products SET crate_size = 24 WHERE id = 'p'")

	if _, err := p.Down(ctx); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 3 {
		t.Errorf("version after down = %d, want 3", v)
	}
	if after := schemaState(t, ctx, db); after != schemaBefore {
		t.Errorf("schema after down:\n%s\nwant as before 0004:\n%s", after, schemaBefore)
	}
	if after := tableRows(t, ctx, db, "products"); after != productsBefore {
		t.Errorf("products after down:\n%s\nwant as before 0004:\n%s", after, productsBefore)
	}

	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatalf("migrate to 0004 after down: %v", err)
	}
	if c := productCrateSize(t, ctx, db, "p"); c != nil {
		t.Errorf("crate_size after migrating up again = %d, want NULL", *c)
	}
}
