package app_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// systemApp is a handler from app.NewHandler with DATA_DIR in t.TempDir():
// the migrated database stashbert.db, the image directory images and the
// backup directory backups (not created).
type systemApp struct {
	h                                http.Handler
	db                               *sql.DB
	dbPath, imageDir, backupDir, tmp string
}

// newSystemApp returns a systemApp whose configuration comes from env
// (DATA_DIR is set). os.TempDir points to a new empty directory, tmp.
func newSystemApp(t *testing.T, env map[string]string) systemApp {
	t.Helper()
	dataDir := t.TempDir()
	a := systemApp{
		dbPath:    filepath.Join(dataDir, "stashbert.db"),
		imageDir:  filepath.Join(dataDir, "images"),
		backupDir: filepath.Join(dataDir, "backups"),
		tmp:       t.TempDir(),
	}
	t.Setenv("TMPDIR", a.tmp)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := store.Open(ctx, a.dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	if err := os.Mkdir(a.imageDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(key string) string {
		if key == "DATA_DIR" {
			return dataDir
		}
		return env[key]
	})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	a.db = db
	a.h, err = app.NewHandler(cfg, app.Deps{
		Logger: slog.New(slog.DiscardHandler), Version: "dev", DB: db,
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(),
		ImageDir: a.imageDir, DBPath: a.dbPath, BackupDir: a.backupDir,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return a
}

// fileSize returns the size of the file at path, 0 if it does not exist.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// checkEmpty asserts that dir has no entries.
func checkEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%s has %d entries, want none (first %s)", dir, len(entries), entries[0].Name())
	}
}

func TestGetSystemStatus(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		backups []string
		want    string // without database_size
	}{
		{
			name: "without backups and without Open Food Facts",
			want: `{"version": "dev", "backup_count": 0, "last_backup_at": null, "backup_keep": 14, "open_food_facts": false}`,
		},
		{
			name: "with backups and Open Food Facts",
			env:  map[string]string{"OFF_CONTACT": "test@example.org", "BACKUP_KEEP": "7"},
			// Backups before migrations and other files do not count.
			backups: []string{
				"stashbert-20260923-080000.db", "stashbert-20260925-101500.db", "stashbert-20260924-080000.db",
				"pre-migration-20260926-000000.db", "notes.txt",
			},
			want: `{"version": "dev", "backup_count": 3, "last_backup_at": "2026-09-25T10:15:00Z", "backup_keep": 7, "open_food_facts": true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newSystemApp(t, tt.env)
			if len(tt.backups) > 0 {
				if err := os.Mkdir(a.backupDir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tt.backups {
				writeFile(t, a.backupDir, name, []byte("backup"))
			}
			// The database is in WAL mode; the size counts the WAL file too.
			wal := fileSize(t, a.dbPath+"-wal")
			if wal == 0 {
				t.Fatal("no WAL file, want one for the test")
			}
			wantSize := fileSize(t, a.dbPath) + wal

			rec := get(a.h, "/api/v1/system")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			want := tt.want[:len(tt.want)-1] + `, "database_size": ` + strconv.FormatInt(wantSize, 10) + "}"
			if !jsonEqual(t, rec.Body.String(), want) {
				t.Errorf("body = %s, want %s", rec.Body.String(), want)
			}
		})
	}
}

// archiveFile is a file of an unpacked backup archive.
type archiveFile struct {
	typeflag byte
	data     []byte
}

// unpackBackup reads the tar.gz from r and returns the names of its entries
// in their order and the entries by name.
func unpackBackup(t *testing.T, r io.Reader) ([]string, map[string]archiveFile) {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	files := map[string]archiveFile{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		names = append(names, hdr.Name)
		files[hdr.Name] = archiveFile{typeflag: hdr.Typeflag, data: data}
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	return names, files
}

// productNames opens data as a database and returns the names of its
// products, sorted.
func productNames(t *testing.T, data []byte) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "stashbert.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open database from archive: %v", err)
	}
	defer db.Close()
	var ok string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok); err != nil || ok != "ok" {
		t.Errorf("integrity_check = %q, %v, want ok", ok, err)
	}
	rows, err := db.QueryContext(ctx, "SELECT name FROM products ORDER BY name")
	if err != nil {
		t.Fatalf("query products: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

// randomBytes returns n bytes that gzip cannot compress.
func randomBytes(n int) []byte {
	b := make([]byte, n)
	r := rand.NewChaCha8([32]byte{1})
	r.Read(b)
	return b
}

// checkArchive asserts that data is a backup archive with the products
// wantProducts and exactly the images.
func checkArchive(t *testing.T, data []byte, wantProducts []string, images map[string][]byte) {
	t.Helper()
	names, files := unpackBackup(t, bytes.NewReader(data))
	wantNames := []string{"stashbert.db", "images/"}
	for name := range images {
		wantNames = append(wantNames, "images/"+name)
	}
	slices.Sort(wantNames[2:])
	if !slices.Equal(names, wantNames) {
		t.Fatalf("archive entries = %v, want %v", names, wantNames)
	}
	if f := files["images/"]; f.typeflag != tar.TypeDir {
		t.Errorf("images/ has type %c, want a directory", f.typeflag)
	}
	for name, want := range images {
		if f := files["images/"+name]; f.typeflag != tar.TypeReg || !bytes.Equal(f.data, want) {
			t.Errorf("images/%s: type %c with %d bytes, want the file with %d bytes", name, f.typeflag, len(f.data), len(want))
		}
	}
	if got := productNames(t, files["stashbert.db"].data); !slices.Equal(got, wantProducts) {
		t.Errorf("products in the archive = %v, want %v", got, wantProducts)
	}
}

var archiveName = regexp.MustCompile(`^attachment; filename="stashbert-([0-9]{8}-[0-9]{6})\.tar\.gz"$`)

func TestDownloadBackup(t *testing.T) {
	a := newSystemApp(t, nil)
	for _, name := range []string{"Milch", "Butter"} {
		if rec := post(a.h, "/api/v1/products", `{"name": "`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("create %s: status %d, body %s", name, rec.Code, rec.Body.String())
		}
	}
	images := map[string][]byte{"one.jpg": testJPEG(t), "two.png": testPNG(t), "three.webp": testWebP}
	for name, data := range images {
		writeFile(t, a.imageDir, name, data)
	}
	start := time.Now().UTC().Truncate(time.Second)

	rec := get(a.h, "/api/v1/backup")

	end := time.Now().UTC()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type = %q, want application/gzip", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	m := archiveName.FindStringSubmatch(cd)
	if m == nil {
		t.Fatalf("Content-Disposition = %q, want attachment with stashbert-<YYYYMMDD-HHMMSS>.tar.gz", cd)
	}
	if stamp, err := time.Parse("20060102-150405", m[1]); err != nil || stamp.Before(start) || stamp.After(end) {
		t.Errorf("time in the file name = %s, want between %s and %s in UTC", m[1], start, end)
	}
	if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(rec.Body.Len()) {
		t.Errorf("Content-Length = %q, want %d", cl, rec.Body.Len())
	}
	checkArchive(t, rec.Body.Bytes(), []string{"Butter", "Milch"}, images)
	checkEmpty(t, a.tmp)
}

func TestDownloadBackupWithoutImages(t *testing.T) {
	a := newSystemApp(t, nil)

	rec := get(a.h, "/api/v1/backup")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkArchive(t, rec.Body.Bytes(), nil, nil)
	checkEmpty(t, a.tmp)
}

// download gets GET /api/v1/backup from the server at url and returns the
// status and the body.
func download(t *testing.T, client *http.Client, url string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v1/backup", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/backup: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, body
}

func TestDownloadBackupOutlastsWriteTimeout(t *testing.T) {
	a := newSystemApp(t, nil)
	images := map[string][]byte{"large.jpg": randomBytes(4 << 20)}
	writeFile(t, a.imageDir, "large.jpg", images["large.jpg"])
	srv := httptest.NewUnstartedServer(a.h)
	// Without the longer deadline for the download, the connection would
	// fail at the first write after 1 ms.
	srv.Config.WriteTimeout = time.Millisecond
	srv.Start()
	defer srv.Close()

	status, body := download(t, srv.Client(), srv.URL)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	checkArchive(t, body, nil, images)
}

func TestDownloadBackupAborted(t *testing.T) {
	a := newSystemApp(t, nil)
	images := map[string][]byte{"large.jpg": randomBytes(16 << 20)}
	writeFile(t, a.imageDir, "large.jpg", images["large.jpg"])
	srv := httptest.NewServer(a.h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// The client reads the start of the archive and then goes away.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/backup", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/backup: %v", err)
	}
	if _, err := io.ReadFull(resp.Body, make([]byte, 1024)); err != nil {
		t.Fatalf("read start: %v", err)
	}
	resp.Body.Close()

	// The temporary copy disappears, and the next download runs.
	for {
		entries, err := os.ReadDir(a.tmp)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("temporary directory still has %d entries after the client went away", len(entries))
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, body := download(t, srv.Client(), srv.URL)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	checkArchive(t, body, nil, images)
	checkEmpty(t, a.tmp)
}

func TestDownloadBackupOtherMethods(t *testing.T) {
	a := newSystemApp(t, nil)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			a.h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/backup", nil))

			checkProblemCode(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
		})
	}
	checkEmpty(t, a.tmp)
}
