package app_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
)

// One migration, and the same one with a second.
var (
	oneMigration = fstest.MapFS{
		"0001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (id INTEGER PRIMARY KEY) STRICT;\n")},
	}
	twoMigrations = fstest.MapFS{
		"0001_a.sql": oneMigration["0001_a.sql"],
		"0002_b.sql": {Data: []byte("-- +goose Up\nCREATE TABLE b (id INTEGER PRIMARY KEY) STRICT;\n")},
	}
)

// openAndMigrate calls app.OpenAndMigrate and closes the database at the end of the test.
func openAndMigrate(t *testing.T, ctx context.Context, path, backupDir string, fsys fstest.MapFS, now time.Time) *sql.DB {
	t.Helper()
	db, err := app.OpenAndMigrate(ctx, path, backupDir, fsys, now)
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// backupNames returns the names of the files in dir; a missing dir has none.
func backupNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// hasTable reports whether db has a table with the name.
func hasTable(t *testing.T, ctx context.Context, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
		t.Fatalf("look up table %s: %v", name, err)
	}
	return n == 1
}

func TestOpenAndMigrateNewDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")

	db := openAndMigrate(t, ctx, filepath.Join(dataDir, "stashbert.db"), backupDir, twoMigrations, time.Now())

	if !hasTable(t, ctx, db, "a") || !hasTable(t, ctx, db, "b") {
		t.Error("tables a and b are missing after the migrations")
	}
	if names := backupNames(t, backupDir); len(names) != 0 {
		t.Errorf("backups = %v, want none for a new database", names)
	}
}

func TestOpenAndMigratePendingMigrations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "stashbert.db")
	backupDir := filepath.Join(dataDir, "backups")
	old := openAndMigrate(t, ctx, path, backupDir, oneMigration, time.Now())
	if _, err := old.ExecContext(ctx, "INSERT INTO a (id) VALUES (1), (2)"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	old.Close()
	now := time.Date(2026, 9, 23, 4, 5, 6, 0, time.UTC)

	db := openAndMigrate(t, ctx, path, backupDir, twoMigrations, now)

	if !hasTable(t, ctx, db, "b") {
		t.Error("table b is missing after the migrations")
	}
	names := backupNames(t, backupDir)
	if len(names) != 1 || names[0] != "pre-migration-20260923-040506.db" {
		t.Fatalf("backups = %v, want [pre-migration-20260923-040506.db]", names)
	}
	pre, err := sql.Open("sqlite", filepath.Join(backupDir, names[0]))
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer pre.Close()
	if got := countRows(t, pre, "a"); got != 2 {
		t.Errorf("rows in a in the backup = %d, want 2", got)
	}
	if hasTable(t, ctx, pre, "b") {
		t.Error("the backup has table b, want the state before the migrations")
	}
}

func TestOpenAndMigrateNothingPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "stashbert.db")
	backupDir := filepath.Join(dataDir, "backups")
	openAndMigrate(t, ctx, path, backupDir, oneMigration, time.Now()).Close()

	openAndMigrate(t, ctx, path, backupDir, oneMigration, time.Now())

	if names := backupNames(t, backupDir); len(names) != 0 {
		t.Errorf("backups = %v, want none without pending migrations", names)
	}
}
