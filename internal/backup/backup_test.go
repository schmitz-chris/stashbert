package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// sourceAPIKey is the API key of the product recognition in the database
// of openSource.
const sourceAPIKey = "sk-test-secret-key-4711"

// openSource returns a database in t.TempDir() with the tables a (3 rows)
// and b (5 rows), the table settings as in the migrations with the API key
// of the product recognition (sourceAPIKey) and one other setting, and the
// path of its file.
func openSource(t *testing.T, ctx context.Context) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stashbert.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	exec(t, ctx, db, "CREATE TABLE a (id INTEGER PRIMARY KEY, v TEXT NOT NULL) STRICT")
	exec(t, ctx, db, "CREATE TABLE b (id INTEGER PRIMARY KEY, v TEXT NOT NULL) STRICT")
	for i := range 3 {
		exec(t, ctx, db, "INSERT INTO a (v) VALUES (?)", strings.Repeat("a", i+1))
	}
	for i := range 5 {
		exec(t, ctx, db, "INSERT INTO b (v) VALUES (?)", strings.Repeat("b", i+1))
	}
	exec(t, ctx, db, "CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT")
	exec(t, ctx, db, "INSERT INTO settings (key, value) VALUES ('recognition_api_key', ?), ('shopping_target_id', 'todo.einkauf')", sourceAPIKey)
	return db, path
}

func exec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func count(t *testing.T, ctx context.Context, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// dirNames returns the sorted names of the entries in dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestRunWritesReadableCopy(t *testing.T) {
	ctx := testContext(t)
	src, srcPath := openSource(t, ctx)
	// The rows are still in the WAL file of the source, not in its main file.
	if fi, err := os.Stat(srcPath + "-wal"); err != nil || fi.Size() == 0 {
		t.Fatalf("source WAL file: %v, want a file that is not empty", err)
	}
	dir := filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, 9, 23, 4, 5, 6, 0, time.FixedZone("CEST", 2*60*60))

	path, err := backup.Run(ctx, src, dir, 14, now)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if want := filepath.Join(dir, "stashbert-20260923-020506.db"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat backup dir: %v", err)
	}
	if perm := fi.Mode().Perm(); perm&^0o750 != 0 {
		t.Errorf("backup dir mode = %v, want at most 0750", perm)
	}
	// Only the backup file itself goes into another directory, so no WAL
	// file of the source can be involved in reading it.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	copyPath := filepath.Join(t.TempDir(), "copy.db")
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	cp, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer cp.Close()
	var check string
	if err := cp.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if check != "ok" {
		t.Errorf("integrity_check = %q, want ok", check)
	}
	for _, table := range []string{"a", "b"} {
		if got, want := count(t, ctx, cp, table), count(t, ctx, src, table); got != want {
			t.Errorf("rows in %s = %d, want %d", table, got, want)
		}
	}
}

func TestRunKeepsNewest(t *testing.T) {
	ctx := testContext(t)
	src, _ := openSource(t, ctx)
	dir := t.TempDir()
	// Files that Run must not delete: a backup before migrations, a foreign
	// file and names close to the pattern, all older than the backups.
	others := []string{
		"notes.txt",
		"pre-migration-20200101-000000.db",
		"stashbert-20200101-000000.db.bak",
		"stashbert-2020-01-01.db",
	}
	for _, name := range others {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	start := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)

	for i := range 4 {
		if _, err := backup.Run(ctx, src, dir, 2, start.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}

	want := slices.Sorted(slices.Values(append([]string{
		"stashbert-20260903-030000.db",
		"stashbert-20260904-030000.db",
	}, others...)))
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestRunExistingFile(t *testing.T) {
	ctx := testContext(t)
	src, _ := openSource(t, ctx)
	dir := t.TempDir()
	now := time.Date(2026, 9, 23, 4, 5, 6, 0, time.UTC)
	path, err := backup.Run(ctx, src, dir, 14, now)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	// Another copy would differ from the first one.
	exec(t, ctx, src, "INSERT INTO a (v) VALUES ('new')")

	if _, err := backup.Run(ctx, src, dir, 14, now); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second Run: error %v, want fs.ErrExist", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read backup after second Run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("second Run changed the first backup")
	}
	if got, want := dirNames(t, dir), []string{filepath.Base(path)}; !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

// An existing empty file would be overwritten by VACUUM INTO.
func TestPreMigrationExistingEmptyFile(t *testing.T) {
	ctx := testContext(t)
	src, _ := openSource(t, ctx)
	dir := t.TempDir()
	path := filepath.Join(dir, "pre-migration-20260923-040506.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	if _, err := backup.PreMigration(ctx, src, dir, time.Date(2026, 9, 23, 4, 5, 6, 0, time.UTC)); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("PreMigration: error %v, want fs.ErrExist", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Errorf("existing file: %v, want it unchanged and empty", err)
	}
}

func TestRunRejectsKeepBelowOne(t *testing.T) {
	ctx := testContext(t)
	src, _ := openSource(t, ctx)
	dir := t.TempDir()

	if _, err := backup.Run(ctx, src, dir, 0, time.Now()); err == nil {
		t.Fatal("Run with keep 0 succeeded, want error")
	}
	if got := dirNames(t, dir); len(got) != 0 {
		t.Errorf("files = %v, want none", got)
	}
}

// lineWriter sends every write, one log record each, to its channel.
type lineWriter chan string

func (w lineWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

func TestStartRunsRightAway(t *testing.T) {
	ctx := testContext(t)
	src, _ := openSource(t, ctx)
	dir := t.TempDir()
	logs := make(lineWriter, 10)
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		backup.Start(jobCtx, src, dir, 14, time.Hour, slog.New(slog.NewJSONHandler(logs, nil)))
	}()

	select {
	case line := <-logs:
		if !strings.Contains(line, `"level":"INFO"`) || !strings.Contains(line, `"msg":"backup written"`) ||
			!strings.Contains(line, `stashbert-`) {
			t.Errorf("log = %s, want info backup written with the file", line)
		}
	case <-ctx.Done():
		t.Fatal("no log record before the deadline")
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Start did not return after its context ended")
	}
	names := dirNames(t, dir)
	if len(names) != 1 || !strings.HasPrefix(names[0], "stashbert-") {
		t.Errorf("files = %v, want one backup", names)
	}
}
