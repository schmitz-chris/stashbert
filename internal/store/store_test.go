package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func openDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "stashbert.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func openMigratedDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db := openDB(t, ctx)
	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func exec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// schemaState describes the schema and the goose version table, so two states can be compared.
func schemaState(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT type, name, coalesce(sql, '') FROM sqlite_schema ORDER BY type, name")
	if err != nil {
		t.Fatalf("query schema: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("scan schema: %v", err)
		}
		fmt.Fprintf(&b, "%s %s: %s\n", typ, name, ddl)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var versions int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version").Scan(&versions); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	fmt.Fprintf(&b, "goose versions: %d\n", versions)
	return b.String()
}

func TestMigrateTwice(t *testing.T) {
	ctx := testContext(t)
	db := openMigratedDB(t, ctx)
	exec(t, ctx, db, "INSERT INTO settings (key, value) VALUES ('test_key', 'x')")
	before := schemaState(t, ctx, db)

	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	if after := schemaState(t, ctx, db); after != before {
		t.Errorf("second Migrate changed the schema:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'test_key'").Scan(&value); err != nil {
		t.Fatalf("read setting after second Migrate: %v", err)
	}
	if value != "x" {
		t.Errorf("setting = %q, want %q", value, "x")
	}
}

func TestPragmasOnEveryConnection(t *testing.T) {
	ctx := testContext(t)
	db := openDB(t, ctx)

	// Hold four connections at once so each one is a separate pool connection.
	conns := make([]*sql.Conn, 4)
	for i := range conns {
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn %d: %v", i, err)
		}
		defer c.Close()
		conns[i] = c
	}
	pragmas := []struct{ name, want string }{
		{"foreign_keys", "1"},
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"synchronous", "1"}, // NORMAL
	}
	for i, c := range conns {
		for _, p := range pragmas {
			var got string
			if err := c.QueryRowContext(ctx, "PRAGMA "+p.name).Scan(&got); err != nil {
				t.Fatalf("conn %d: PRAGMA %s: %v", i, p.name, err)
			}
			if got != p.want {
				t.Errorf("conn %d: PRAGMA %s = %q, want %q", i, p.name, got, p.want)
			}
		}
	}
}

// TestMigratedTables checks that Migrate creates only the goose table and
// settings, and that settings is STRICT.
func TestMigratedTables(t *testing.T) {
	ctx := testContext(t)
	db := openMigratedDB(t, ctx)

	rows, err := db.QueryContext(ctx, "PRAGMA table_list")
	if err != nil {
		t.Fatalf("PRAGMA table_list: %v", err)
	}
	defer rows.Close()
	strict := map[string]bool{}
	for rows.Next() {
		var schema, name, typ string
		var ncol, withoutRowid, isStrict int
		if err := rows.Scan(&schema, &name, &typ, &ncol, &withoutRowid, &isStrict); err != nil {
			t.Fatalf("scan table_list: %v", err)
		}
		// SQLite's own tables (sqlite_schema, sqlite_sequence) are not part of the schema.
		if schema == "main" && typ == "table" && !strings.HasPrefix(name, "sqlite_") {
			strict[name] = isStrict == 1
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read table_list: %v", err)
	}

	if got, want := slices.Sorted(maps.Keys(strict)), []string{"goose_db_version", "settings"}; !slices.Equal(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
	if !strict["settings"] {
		t.Error("table settings is not STRICT")
	}
}

func TestOpenRejectsQuestionMark(t *testing.T) {
	ctx := testContext(t)
	if db, err := store.Open(ctx, filepath.Join(t.TempDir(), "a?b.db")); err == nil {
		db.Close()
		t.Fatal("Open succeeded for a path with '?', want error")
	}
}
