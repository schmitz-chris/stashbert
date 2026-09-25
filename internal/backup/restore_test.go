package backup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// Image names as StashBert stores them: from the image fetcher and from an
// upload.
const (
	fetchedImage  = "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2b.jpg"
	uploadedImage = "0199a1b2-c3d4-7e5f-8a6b-7c8d9e0f1a2c-upload-1758800000000.png"
)

// migratedDB returns a migrated database in t.TempDir() with the setting
// marker = value, and the path of its file.
func migratedDB(t *testing.T, ctx context.Context, value string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stashbert.db")
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	exec(t, ctx, db, "INSERT INTO settings (key, value) VALUES ('marker', ?)", value)
	return db, path
}

// marker returns the setting marker of the database file at path.
func marker(t *testing.T, ctx context.Context, path string) string {
	t.Helper()
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'marker'").Scan(&value); err != nil {
		t.Fatalf("read marker of %s: %v", path, err)
	}
	return value
}

// downloadArchive returns a backup archive of db as GET /backup writes it,
// with the images fetchedImage and uploadedImage, and the images by name.
func downloadArchive(t *testing.T, ctx context.Context, db *sql.DB) ([]byte, map[string][]byte) {
	t.Helper()
	useTempDir(t)
	imageDir := t.TempDir()
	images := map[string][]byte{fetchedImage: []byte("fetched jpeg"), uploadedImage: []byte("uploaded png")}
	for name, data := range images {
		if err := os.WriteFile(filepath.Join(imageDir, name), data, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	a, err := backup.NewArchive(ctx, db, imageDir)
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	defer a.Close()
	data, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	return data, images
}

// entry is an entry of a test archive.
type entry struct {
	hdr  tar.Header
	data []byte
}

// file returns a regular file entry.
func file(name string, data []byte) entry {
	return entry{hdr: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o640, Size: int64(len(data))}, data: data}
}

// dir returns a directory entry.
func dir(name string) entry {
	return entry{hdr: tar.Header{Typeflag: tar.TypeDir, Name: name, Mode: 0o750}}
}

// tarGz returns a tar.gz with entries.
func tarGz(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := tw.WriteHeader(&e.hdr); err != nil {
			t.Fatalf("write header %s: %v", e.hdr.Name, err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatalf("write %s: %v", e.hdr.Name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// copyOf returns a copy of db made with VACUUM INTO as bytes.
func copyOf(t *testing.T, ctx context.Context, db *sql.DB) []byte {
	t.Helper()
	path, err := backup.PreMigration(ctx, db, t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("copy database: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// corruptCopy returns a copy of a database with many rows whose middle page
// is overwritten, so that it opens but fails PRAGMA integrity_check.
func corruptCopy(t *testing.T, ctx context.Context) []byte {
	t.Helper()
	db, _ := migratedDB(t, ctx, "corrupt")
	for i := range 500 {
		exec(t, ctx, db, "INSERT INTO settings (key, value) VALUES (?, ?)", fmt.Sprintf("key-%03d", i), strings.Repeat("v", 200))
	}
	data := copyOf(t, ctx, db)
	const pageSize = 4096
	page := len(data) / pageSize / 2
	for i := range 64 {
		data[page*pageSize+i] = 0xff
	}
	return data
}

// checkError asserts that err is an *httpx.Error with status and code.
func checkError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *httpx.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want an *httpx.Error with code %s", err, code)
	}
	if e.Status != status || e.Code != code {
		t.Errorf("err = %d %s (%s), want %d %s", e.Status, e.Code, e.Detail, status, code)
	}
}

// checkNoRest asserts that dataDir is empty.
func checkNoRest(t *testing.T, dataDir string) {
	t.Helper()
	if names := dirNames(t, dataDir); len(names) != 0 {
		t.Errorf("data dir = %v, want it empty", names)
	}
}

// checkPending asserts that dataDir/restore/pending/ holds exactly a
// database with the setting marker = value and the images.
func checkPending(t *testing.T, ctx context.Context, dataDir, value string, images map[string][]byte) {
	t.Helper()
	if names := dirNames(t, filepath.Join(dataDir, "restore")); !slices.Equal(names, []string{"pending"}) {
		t.Fatalf("restore/ = %v, want only pending", names)
	}
	pending := filepath.Join(dataDir, "restore", "pending")
	if names := dirNames(t, pending); !slices.Equal(names, []string{"images", "stashbert.db"}) {
		t.Fatalf("pending/ = %v, want images and stashbert.db", names)
	}
	checkImages(t, filepath.Join(pending, "images"), images)
	if got := marker(t, ctx, filepath.Join(pending, "stashbert.db")); got != value {
		t.Errorf("marker = %q, want %q", got, value)
	}
}

// checkImages asserts that dir holds exactly the images.
func checkImages(t *testing.T, dir string, images map[string][]byte) {
	t.Helper()
	var want []string
	for name := range images {
		want = append(want, name)
	}
	slices.Sort(want)
	if names := dirNames(t, dir); !slices.Equal(names, want) {
		t.Fatalf("images = %v, want %v", names, want)
	}
	for name, data := range images {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("image %s = %q, want %q", name, got, data)
		}
	}
}

func TestStageArchiveFromDownload(t *testing.T) {
	ctx := testContext(t)
	db, _ := migratedDB(t, ctx, "from download")
	archive, images := downloadArchive(t, ctx, db)
	dataDir := t.TempDir()

	if err := backup.Stage(ctx, dataDir, bytes.NewReader(archive), store.Migrations, backup.MaxUnpackedBytes); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if names := dirNames(t, dataDir); !slices.Equal(names, []string{"restore"}) {
		t.Errorf("data dir = %v, want only restore", names)
	}
	checkPending(t, ctx, dataDir, "from download", images)
}

func TestStageRejects(t *testing.T) {
	ctx := testContext(t)
	db, _ := migratedDB(t, ctx, "valid")
	valid := copyOf(t, ctx, db)
	archive, _ := downloadArchive(t, ctx, db)

	tooNewDB, _ := migratedDB(t, ctx, "too new")
	exec(t, ctx, tooNewDB, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (9999, 1)")
	foreignDB, _ := openSource(t, ctx)
	image := file("images/"+fetchedImage, []byte("jpeg"))

	tests := []struct {
		name        string
		archive     []byte
		maxUnpacked int64
		status      int
		code        string
	}{
		{name: "without stashbert.db", archive: tarGz(t, dir("images/"), image), code: "invalid_backup"},
		{name: "with ../", archive: tarGz(t, file("stashbert.db", valid), file("../stashbert.db", valid)), code: "invalid_backup"},
		{name: "with ../ under images", archive: tarGz(t, file("stashbert.db", valid), file("images/../../"+fetchedImage, []byte("x"))), code: "invalid_backup"},
		{name: "with absolute path", archive: tarGz(t, file("/stashbert.db", valid)), code: "invalid_backup"},
		{
			name: "with symbolic link",
			archive: tarGz(t, file("stashbert.db", valid), dir("images/"),
				entry{hdr: tar.Header{Typeflag: tar.TypeSymlink, Name: "images/" + fetchedImage, Linkname: "/etc/passwd"}}),
			code: "invalid_backup",
		},
		{
			name: "with hard link",
			archive: tarGz(t, file("stashbert.db", valid),
				entry{hdr: tar.Header{Typeflag: tar.TypeLink, Name: "images/" + fetchedImage, Linkname: "stashbert.db"}}),
			code: "invalid_backup",
		},
		{
			name: "with device",
			archive: tarGz(t, file("stashbert.db", valid),
				entry{hdr: tar.Header{Typeflag: tar.TypeChar, Name: "images/" + fetchedImage, Devmajor: 1, Devminor: 3}}),
			code: "invalid_backup",
		},
		{name: "with foreign file", archive: tarGz(t, file("stashbert.db", valid), file("notes.txt", []byte("x"))), code: "invalid_backup"},
		{name: "with foreign image name", archive: tarGz(t, file("stashbert.db", valid), file("images/foto.jpg", []byte("x"))), code: "invalid_backup"},
		{name: "with subdirectory", archive: tarGz(t, file("stashbert.db", valid), dir("images/sub/")), code: "invalid_backup"},
		{name: "with stashbert.db twice", archive: tarGz(t, file("stashbert.db", valid), file("stashbert.db", valid)), code: "invalid_backup"},
		{name: "no gzip", archive: []byte("this is no archive"), code: "invalid_backup"},
		{name: "gzip without tar", archive: gzipped(t, []byte("this is no tar")), code: "invalid_backup"},
		{name: "cut off", archive: archive[:len(archive)/2], code: "invalid_backup"},
		{name: "no database", archive: tarGz(t, file("stashbert.db", randomData(64<<10))), code: "invalid_backup"},
		{name: "corrupt database", archive: tarGz(t, file("stashbert.db", corruptCopy(t, ctx))), code: "invalid_backup"},
		{name: "database without migrations", archive: tarGz(t, file("stashbert.db", copyOf(t, ctx, foreignDB))), code: "invalid_backup"},
		{name: "newer migration", archive: tarGz(t, file("stashbert.db", copyOf(t, ctx, tooNewDB))), code: "backup_too_new"},
		{name: "too large", archive: archive, maxUnpacked: 16 << 10, status: http.StatusRequestEntityTooLarge, code: "backup_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.maxUnpacked == 0 {
				tt.maxUnpacked = backup.MaxUnpackedBytes
			}
			if tt.status == 0 {
				tt.status = http.StatusUnprocessableEntity
			}
			dataDir := t.TempDir()

			err := backup.Stage(ctx, dataDir, bytes.NewReader(tt.archive), store.Migrations, tt.maxUnpacked)

			checkError(t, err, tt.status, tt.code)
			checkNoRest(t, dataDir)
		})
	}
}

// gzipped returns data compressed with gzip.
func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// randomData returns n random bytes.
func randomData(n int) []byte {
	b := make([]byte, n)
	rand.NewChaCha8([32]byte{2}).Read(b)
	return b
}

func TestStageSecondIsInProgress(t *testing.T) {
	ctx := testContext(t)
	first, _ := migratedDB(t, ctx, "first")
	firstArchive, images := downloadArchive(t, ctx, first)
	second, _ := migratedDB(t, ctx, "second")
	secondArchive, _ := downloadArchive(t, ctx, second)
	dataDir := t.TempDir()
	if err := backup.Stage(ctx, dataDir, bytes.NewReader(firstArchive), store.Migrations, backup.MaxUnpackedBytes); err != nil {
		t.Fatalf("first Stage: %v", err)
	}

	err := backup.Stage(ctx, dataDir, bytes.NewReader(secondArchive), store.Migrations, backup.MaxUnpackedBytes)

	checkError(t, err, http.StatusConflict, "restore_in_progress")
	checkPending(t, ctx, dataDir, "first", images)
}

// failingReader returns the data and then err.
type failingReader struct {
	data []byte
	err  error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestStageReadError(t *testing.T) {
	ctx := testContext(t)
	db, _ := migratedDB(t, ctx, "read error")
	archive, _ := downloadArchive(t, ctx, db)
	for _, n := range []int{0, 10, len(archive) / 2} {
		dataDir := t.TempDir()
		limit := &http.MaxBytesError{Limit: int64(n)}

		err := backup.Stage(ctx, dataDir, &failingReader{data: archive[:n], err: limit}, store.Migrations, backup.MaxUnpackedBytes)

		var tooLarge *http.MaxBytesError
		if !errors.As(err, &tooLarge) {
			t.Errorf("after %d bytes: err = %v, want the error of the reader", n, err)
		}
		checkNoRest(t, dataDir)
	}
}

// writeData fills dir like DATA_DIR: stashbert.db, its WAL and SHM files
// with the texts "old db", "old wal" and "old shm", a daily backup and
// images/ with the images.
func writeData(t *testing.T, dir string, images map[string][]byte) {
	t.Helper()
	for name, data := range map[string]string{"stashbert.db": "old db", "stashbert.db-wal": "old wal", "stashbert.db-shm": "old shm", "backups/stashbert-20260924-030000.db": "daily"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "images"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, data := range images {
		if err := os.WriteFile(filepath.Join(dir, "images", name), data, 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

// checkFile asserts that the file at path holds want.
func checkFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func TestApplyPending(t *testing.T) {
	ctx := testContext(t)
	db, _ := migratedDB(t, ctx, "restored")
	archive, images := downloadArchive(t, ctx, db)
	dataDir := t.TempDir()
	oldImages := map[string][]byte{"0199a1b2-c3d4-7e5f-8a6b-000000000001.webp": []byte("old webp")}
	writeData(t, dataDir, oldImages)
	if err := backup.Stage(ctx, dataDir, bytes.NewReader(archive), store.Migrations, backup.MaxUnpackedBytes); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	// The rest of an aborted upload.
	if err := os.MkdirAll(filepath.Join(dataDir, "restore", "incoming-123", "images"), 0o750); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	now := time.Date(2026, 9, 25, 14, 30, 5, 0, time.FixedZone("CEST", 2*60*60))

	if err := backup.ApplyPending(dataDir, now, slog.New(slog.NewJSONHandler(&logs, nil))); err != nil {
		t.Fatalf("ApplyPending: %v", err)
	}

	want := []string{"backups", "images", "stashbert.db", "vor-restore-20260925-123005"}
	if names := dirNames(t, dataDir); !slices.Equal(names, want) {
		t.Fatalf("data dir = %v, want %v", names, want)
	}
	previous := filepath.Join(dataDir, "vor-restore-20260925-123005")
	if names := dirNames(t, previous); !slices.Equal(names, []string{"images", "stashbert.db", "stashbert.db-shm", "stashbert.db-wal"}) {
		t.Fatalf("vor-restore = %v, want the old database with WAL and SHM and images", names)
	}
	checkFile(t, filepath.Join(previous, "stashbert.db"), "old db")
	checkFile(t, filepath.Join(previous, "stashbert.db-wal"), "old wal")
	checkFile(t, filepath.Join(previous, "stashbert.db-shm"), "old shm")
	checkImages(t, filepath.Join(previous, "images"), oldImages)
	checkFile(t, filepath.Join(dataDir, "backups", "stashbert-20260924-030000.db"), "daily")
	checkImages(t, filepath.Join(dataDir, "images"), images)
	if got := marker(t, ctx, filepath.Join(dataDir, "stashbert.db")); got != "restored" {
		t.Errorf("marker = %q, want restored", got)
	}
	// Every step is in the log: the start, four moves out, two in, the
	// removed restore/ and the end.
	for _, want := range []string{`"msg":"apply backup"`, `"msg":"apply backup: removed"`, `"msg":"backup applied","previous":"` + previous + `"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log misses %s:\n%s", want, logs.String())
		}
	}
	if n := strings.Count(logs.String(), `"msg":"apply backup: moved"`); n != 6 {
		t.Errorf("log has %d moves, want 6:\n%s", n, logs.String())
	}
}

func TestApplyPendingIntoEmptyDataDir(t *testing.T) {
	ctx := testContext(t)
	db, _ := migratedDB(t, ctx, "restored")
	archive, images := downloadArchive(t, ctx, db)
	dataDir := t.TempDir()
	if err := backup.Stage(ctx, dataDir, bytes.NewReader(archive), store.Migrations, backup.MaxUnpackedBytes); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if err := backup.ApplyPending(dataDir, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("ApplyPending: %v", err)
	}

	// Without an old state there is no vor-restore directory.
	if names := dirNames(t, dataDir); !slices.Equal(names, []string{"images", "stashbert.db"}) {
		t.Fatalf("data dir = %v, want images and stashbert.db", names)
	}
	checkImages(t, filepath.Join(dataDir, "images"), images)
	if got := marker(t, ctx, filepath.Join(dataDir, "stashbert.db")); got != "restored" {
		t.Errorf("marker = %q, want restored", got)
	}
}

func TestApplyPendingWithoutPending(t *testing.T) {
	images := map[string][]byte{fetchedImage: []byte("old jpeg")}
	for _, rest := range []string{"", "restore", "restore/incoming-123/images"} {
		t.Run("rest "+rest, func(t *testing.T) {
			dataDir := t.TempDir()
			writeData(t, dataDir, images)
			if rest != "" {
				if err := os.MkdirAll(filepath.Join(dataDir, rest), 0o750); err != nil {
					t.Fatal(err)
				}
			}

			if err := backup.ApplyPending(dataDir, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("ApplyPending: %v", err)
			}

			// Only a rest in restore/ is gone.
			want := []string{"backups", "images", "stashbert.db", "stashbert.db-shm", "stashbert.db-wal"}
			if names := dirNames(t, dataDir); !slices.Equal(names, want) {
				t.Fatalf("data dir = %v, want %v", names, want)
			}
			checkFile(t, filepath.Join(dataDir, "stashbert.db"), "old db")
			checkFile(t, filepath.Join(dataDir, "stashbert.db-wal"), "old wal")
			checkImages(t, filepath.Join(dataDir, "images"), images)
		})
	}
}

func TestApplyPendingWithoutDatabase(t *testing.T) {
	images := map[string][]byte{fetchedImage: []byte("old jpeg")}
	dataDir := t.TempDir()
	writeData(t, dataDir, images)
	if err := os.MkdirAll(filepath.Join(dataDir, "restore", "pending", "images"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := backup.ApplyPending(dataDir, time.Now(), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("ApplyPending: err = nil, want an error")
	}

	// Nothing has moved.
	want := []string{"backups", "images", "restore", "stashbert.db", "stashbert.db-shm", "stashbert.db-wal"}
	if names := dirNames(t, dataDir); !slices.Equal(names, want) {
		t.Fatalf("data dir = %v, want %v", names, want)
	}
	checkFile(t, filepath.Join(dataDir, "stashbert.db"), "old db")
	checkImages(t, filepath.Join(dataDir, "images"), images)
}
