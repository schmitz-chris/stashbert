package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/schmitz-chris/stashbert/internal/store"
)

const testTime = "2026-09-23T12:00:00.000Z"

// row is one table row for a direct SQL insert, keyed by column name.
type row = map[string]any

// validRows returns a valid row for each inventory table with key as its
// primary key (the code for barcodes and lookups). Omitted columns are NULL
// or get their default. Barcodes and movements belong to the product "p".
var validRows = map[string]func(key string) row{
	"products": func(key string) row {
		return row{"id": key, "name": "Kidneybohnen", "origin": "manual", "lookup_state": "none",
			"needs_review": 0, "created_at": testTime, "updated_at": testTime}
	},
	"barcodes": func(key string) row {
		return row{"code": key, "product_id": "p", "created_at": testTime}
	},
	"movements": func(key string) row {
		return row{"id": key, "product_id": "p", "kind": "add", "delta": 1, "stock_after": 1, "created_at": testTime}
	},
	"lookups": func(key string) row {
		return row{"code": key, "source": "off", "found": 0, "fetched_at": testTime}
	},
}

// insertRow inserts r into table with a direct SQL insert.
func insertRow(ctx context.Context, db *sql.DB, table string, r row) error {
	cols := slices.Sorted(maps.Keys(r))
	args := make([]any, len(cols))
	for i, c := range cols {
		args[i] = r[c]
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (?%s)",
		table, strings.Join(cols, ", "), strings.Repeat(", ?", len(cols)-1))
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

// validRow returns the valid row of table with key, changed by changes.
func validRow(table, key string, changes row) row {
	r := validRows[table](key)
	maps.Copy(r, changes)
	return r
}

func mustInsert(t *testing.T, ctx context.Context, db *sql.DB, table, key string, changes row) {
	t.Helper()
	r := validRow(table, key, changes)
	if err := insertRow(ctx, db, table, r); err != nil {
		t.Fatalf("insert into %s %v: %v", table, r, err)
	}
}

// sqliteCode returns the extended SQLite result code of err, or 0.
func sqliteCode(err error) int {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return 0
	}
	return e.Code()
}

func countRows(t *testing.T, ctx context.Context, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestMigrateAfterInit applies 0002 and the later migrations to a database
// that is at version 0001 and holds data.
func TestMigrateAfterInit(t *testing.T) {
	ctx := testContext(t)
	db := openDB(t, ctx)

	initSQL, err := fs.ReadFile(store.Migrations, "0001_init.sql")
	if err != nil {
		t.Fatalf("read 0001_init.sql: %v", err)
	}
	if err := store.Migrate(ctx, db, fstest.MapFS{"0001_init.sql": {Data: initSQL}}); err != nil {
		t.Fatalf("Migrate to 0001: %v", err)
	}
	exec(t, ctx, db, "INSERT INTO settings (key, value) VALUES ('test_key', 'x')")

	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("Migrate after 0001: %v", err)
	}

	var version int64
	if err := db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&version); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if version != 4 {
		t.Errorf("goose version = %d, want 4", version)
	}
	if n := countRows(t, ctx, db, "settings"); n != 1 {
		t.Errorf("settings has %d rows after migration, want 1", n)
	}
	mustInsert(t, ctx, db, "products", "p", nil)
}

// TestInventoryColumns compares types, nullability and defaults with architecture.md 5:
// every column is NOT NULL unless the table there says NULL.
func TestInventoryColumns(t *testing.T) {
	ctx := testContext(t)
	db := openMigratedDB(t, ctx)

	want := map[string][]string{
		"products": {
			"id TEXT NOT NULL", "name TEXT NOT NULL", "brand TEXT NULL", "package_size TEXT NULL",
			"stock INTEGER NOT NULL DEFAULT 0", "target INTEGER NOT NULL DEFAULT 0", "min_stock INTEGER NULL",
			"note TEXT NULL", "origin TEXT NOT NULL", "lookup_state TEXT NOT NULL", "needs_review INTEGER NOT NULL",
			"image_source_url TEXT NULL", "image_file TEXT NULL", "created_at TEXT NOT NULL", "updated_at TEXT NOT NULL",
			"marked INTEGER NOT NULL DEFAULT 0", "crate_size INTEGER NULL",
		},
		"barcodes": {
			"code TEXT NOT NULL", "product_id TEXT NOT NULL", "units INTEGER NOT NULL DEFAULT 1", "created_at TEXT NOT NULL",
		},
		"movements": {
			"id TEXT NOT NULL", "product_id TEXT NOT NULL", "kind TEXT NOT NULL", "delta INTEGER NOT NULL",
			"stock_after INTEGER NOT NULL", "barcode TEXT NULL", "reverses_id TEXT NULL", "idempotency_key TEXT NULL",
			"request_hash TEXT NULL", "created_at TEXT NOT NULL",
		},
		"lookups": {
			"code TEXT NOT NULL", "source TEXT NOT NULL", "found INTEGER NOT NULL", "payload TEXT NULL",
			"fetched_at TEXT NOT NULL",
		},
	}
	for table, wantCols := range want {
		got := queryStrings(t, ctx, db, `SELECT name || ' ' || type || iif("notnull", ' NOT NULL', ' NULL') ||
			coalesce(' DEFAULT ' || dflt_value, '') FROM pragma_table_info(?) ORDER BY cid`, table)
		if !slices.Equal(got, wantCols) {
			t.Errorf("columns of %s:\n got %q\nwant %q", table, got, wantCols)
		}
	}

	for index, wantCols := range map[string][]string{
		"idx_barcodes_product_id":     {"barcodes", "product_id"},
		"idx_movements_product_id_id": {"movements", "product_id", "id"},
	} {
		got := queryStrings(t, ctx, db, "SELECT tbl_name FROM sqlite_schema WHERE type = 'index' AND name = ?", index)
		got = append(got, queryStrings(t, ctx, db, "SELECT name FROM pragma_index_info(?) ORDER BY seqno", index)...)
		if !slices.Equal(got, wantCols) {
			t.Errorf("index %s: table and columns = %q, want %q", index, got, wantCols)
		}
	}
}

func queryStrings(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan %s: %v", query, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s: %v", query, err)
	}
	return out
}

// TestInventoryChecks inserts rows that differ from a valid row in one rule each.
func TestInventoryChecks(t *testing.T) {
	ctx := testContext(t)
	db := openMigratedDB(t, ctx)
	mustInsert(t, ctx, db, "products", "p", nil)

	type check struct {
		table   string
		changes row
	}
	// Lengths count characters, not bytes: "ä" has two bytes in UTF-8.
	accepted := []check{
		{"products", row{"name": strings.Repeat("ä", 120), "brand": strings.Repeat("ä", 120),
			"package_size": strings.Repeat("ä", 40), "note": strings.Repeat("ä", 500)}},
		{"products", row{"name": "a"}},
		{"products", row{"stock": 0, "target": 0, "min_stock": 0}},
		{"products", row{"stock": 7, "target": 3, "min_stock": 3}},
		{"products", row{"needs_review": 1}},
		{"products", row{"marked": 0}},
		{"products", row{"marked": 1}},
		{"products", row{"crate_size": 2}},
		{"products", row{"crate_size": 100}},
		{"barcodes", row{"units": 1}},
		{"movements", row{"delta": 0, "stock_after": 0}},
		{"movements", row{"delta": -3, "stock_after": 0}},
		{"lookups", row{"found": 1, "payload": `{"name":"Kidneybohnen"}`}},
	}
	for _, origin := range []string{"openfoodfacts", "openbeautyfacts", "openpetfoodfacts", "openproductsfacts", "manual", "placeholder"} {
		accepted = append(accepted, check{"products", row{"origin": origin}})
	}
	for _, state := range []string{"none", "pending", "done", "not_found"} {
		accepted = append(accepted, check{"products", row{"lookup_state": state}})
	}
	for _, kind := range []string{"add", "consume", "inventory", "reversal", "merge"} {
		accepted = append(accepted, check{"movements", row{"kind": kind}})
	}
	for i, c := range accepted {
		mustInsert(t, ctx, db, c.table, fmt.Sprintf("ok%d", i), c.changes)
	}

	rejected := []check{
		{"products", row{"stock": -1}},
		{"products", row{"target": 2, "min_stock": 3}},
		{"products", row{"name": strings.Repeat("a", 121)}},
		{"products", row{"name": ""}},
		{"products", row{"brand": strings.Repeat("a", 121)}},
		{"products", row{"package_size": strings.Repeat("a", 41)}},
		{"products", row{"note": strings.Repeat("a", 501)}},
		{"products", row{"target": -1}},
		{"products", row{"min_stock": -1}},
		{"products", row{"origin": "foo"}},
		{"products", row{"lookup_state": "foo"}},
		{"products", row{"needs_review": 2}},
		{"products", row{"marked": 2}},
		{"products", row{"crate_size": 1}},
		{"products", row{"crate_size": 101}},
		{"barcodes", row{"units": 0}},
		{"movements", row{"kind": "foo"}},
		{"movements", row{"stock_after": -1}},
		{"lookups", row{"source": "foo"}},
		{"lookups", row{"found": 2}},
	}
	for i, c := range rejected {
		err := insertRow(ctx, db, c.table, validRow(c.table, fmt.Sprintf("bad%d", i), c.changes))
		if sqliteCode(err) != sqlite3.SQLITE_CONSTRAINT_CHECK {
			t.Errorf("insert into %s with %v: err = %v, want CHECK constraint violation", c.table, c.changes, err)
		}
	}
}

// TestInventoryKeys checks the unique and foreign keys and that deleting a
// product deletes its barcodes and movements, including reversals.
func TestInventoryKeys(t *testing.T) {
	ctx := testContext(t)
	db := openMigratedDB(t, ctx)
	mustInsert(t, ctx, db, "products", "p", nil)
	mustInsert(t, ctx, db, "barcodes", "4001686301265", nil)
	mustInsert(t, ctx, db, "movements", "m1", row{"idempotency_key": "k1"})
	mustInsert(t, ctx, db, "movements", "m2", row{"kind": "reversal", "delta": -1, "stock_after": 0, "reverses_id": "m1"})
	mustInsert(t, ctx, db, "lookups", "4001686301265", nil)

	for _, d := range []struct {
		name, table string
		r           row
	}{
		{"barcode code", "barcodes", validRow("barcodes", "4001686301265", nil)},
		{"reverses_id", "movements", validRow("movements", "m3", row{"reverses_id": "m1"})},
		{"idempotency_key", "movements", validRow("movements", "m4", row{"idempotency_key": "k1"})},
		{"lookup code and source", "lookups", validRow("lookups", "4001686301265", nil)},
	} {
		if err := insertRow(ctx, db, d.table, d.r); !store.IsUniqueViolation(err) {
			t.Errorf("duplicate %s: err = %v, want unique violation", d.name, err)
		}
	}

	for _, d := range []struct {
		name, table string
		r           row
	}{
		{"barcode product_id", "barcodes", validRow("barcodes", "20004002", row{"product_id": "unknown"})},
		{"movement product_id", "movements", validRow("movements", "m5", row{"product_id": "unknown"})},
		{"reverses_id", "movements", validRow("movements", "m6", row{"reverses_id": "unknown"})},
	} {
		if err := insertRow(ctx, db, d.table, d.r); sqliteCode(err) != sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
			t.Errorf("unknown %s: err = %v, want foreign key violation", d.name, err)
		}
	}

	exec(t, ctx, db, "DELETE FROM products WHERE id = 'p'")
	for _, table := range []string{"barcodes", "movements"} {
		if n := countRows(t, ctx, db, table); n != 0 {
			t.Errorf("%s has %d rows after deleting the product, want 0", table, n)
		}
	}
}
