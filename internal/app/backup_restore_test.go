package app_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// restoreApp is a systemApp whose handler counts the requested restarts
// and takes backups up to a limit.
type restoreApp struct {
	systemApp
	dataDir  string
	cfg      config.Config
	restarts atomic.Int32
}

// newRestoreApp returns a restoreApp whose handler comes from
// app.NewHandlerWithRestoreLimit with limit.
func newRestoreApp(t *testing.T, limit int64) *restoreApp {
	t.Helper()
	a := &restoreApp{systemApp: newSystemApp(t, nil)}
	a.dataDir = filepath.Dir(a.dbPath)
	var err error
	a.cfg, err = config.Load(func(key string) string {
		if key == "DATA_DIR" {
			return a.dataDir
		}
		return ""
	})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	a.h, err = app.NewHandlerWithRestoreLimit(a.cfg, app.Deps{
		Logger: slog.New(slog.DiscardHandler), Version: "dev", DB: a.db,
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(),
		ImageDir: a.imageDir, DBPath: a.dbPath, BackupDir: a.backupDir,
		Restart: func() { a.restarts.Add(1) },
	}, limit)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return a
}

// createProducts creates products with the names through h.
func createProducts(t *testing.T, h http.Handler, names ...string) {
	t.Helper()
	for _, name := range names {
		if rec := post(h, "/api/v1/products", `{"name": "`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("create %s: status %d, body %s", name, rec.Code, rec.Body.String())
		}
	}
}

// listedNames returns the names in GET /api/v1/products of h.
func listedNames(t *testing.T, h http.Handler) []string {
	t.Helper()
	body, _ := getJSON(t, h, "/api/v1/products").(map[string]any)
	items, _ := body["items"].([]any)
	var names []string
	for _, item := range items {
		p, _ := item.(map[string]any)
		name, _ := p["name"].(string)
		names = append(names, name)
	}
	return names
}

// downloadFrom returns the archive of GET /api/v1/backup of h.
func downloadFrom(t *testing.T, h http.Handler) []byte {
	t.Helper()
	rec := get(h, "/api/v1/backup")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/backup: status %d, body %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// postBackup sends POST /api/v1/backup/restore with body and contentType
// to h.
func postBackup(h http.Handler, contentType string, body io.Reader) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore", body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	h.ServeHTTP(rec, req)
	return rec
}

// checkRestoreRest asserts that DATA_DIR/restore/ does not exist.
func checkRestoreRest(t *testing.T, dataDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dataDir, "restore")); !os.IsNotExist(err) {
		t.Errorf("restore/: err = %v, want it missing", err)
	}
}

// slowReader returns data in chunks of size n with a pause before each.
type slowReader struct {
	data  []byte
	n     int
	pause time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.pause)
	n := copy(p[:min(len(p), r.n)], r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestRestoreUpload(t *testing.T) {
	source := newSystemApp(t, nil)
	createProducts(t, source.h, "Milch")
	archive := downloadFrom(t, source.h)
	target := newRestoreApp(t, backup.MaxUploadBytes)
	createProducts(t, target.h, "Alt")
	srv := httptest.NewUnstartedServer(target.h)
	// Without the longer deadlines, reading the slow body would fail after
	// 100 ms and writing the answer after 1 ms.
	srv.Config.ReadHeaderTimeout = 10 * time.Second
	srv.Config.ReadTimeout = 100 * time.Millisecond
	srv.Config.WriteTimeout = time.Millisecond
	srv.Start()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	body := &slowReader{data: archive, n: len(archive)/4 + 1, pause: 60 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/v1/backup/restore", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/gzip")

	resp, err := srv.Client().Do(req)

	if err != nil {
		t.Fatalf("POST /api/v1/backup/restore: %v", err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted || len(answer) != 0 {
		t.Fatalf("answer = %d %q, want 202 without body", resp.StatusCode, answer)
	}
	if n := target.restarts.Load(); n != 1 {
		t.Errorf("restarts = %d, want 1", n)
	}
	pending := filepath.Join(target.dataDir, "restore", "pending")
	if names := dirNames(t, pending); !slices.Equal(names, []string{"images", "stashbert.db"}) {
		t.Errorf("pending/ = %v, want images and stashbert.db", names)
	}
	// The running state stays until the restart.
	if names := listedNames(t, target.h); !slices.Equal(names, []string{"Alt"}) {
		t.Errorf("products = %v, want [Alt]", names)
	}
}

func TestRestoreUploadXGzip(t *testing.T) {
	source := newSystemApp(t, nil)
	archive := downloadFrom(t, source.h)
	target := newRestoreApp(t, backup.MaxUploadBytes)

	rec := postBackup(target.h, "application/x-gzip", bytes.NewReader(archive))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if n := target.restarts.Load(); n != 1 {
		t.Errorf("restarts = %d, want 1", n)
	}
}

func TestRestoreUploadTwice(t *testing.T) {
	source := newSystemApp(t, nil)
	archive := downloadFrom(t, source.h)
	target := newRestoreApp(t, backup.MaxUploadBytes)
	if rec := postBackup(target.h, "application/gzip", bytes.NewReader(archive)); rec.Code != http.StatusAccepted {
		t.Fatalf("first upload: status = %d, body %s", rec.Code, rec.Body.String())
	}

	rec := postBackup(target.h, "application/gzip", bytes.NewReader(archive))

	checkProblemCode(t, rec, http.StatusConflict, "restore_in_progress")
	if n := target.restarts.Load(); n != 1 {
		t.Errorf("restarts = %d, want 1", n)
	}
}

func TestRestoreUploadContentType(t *testing.T) {
	source := newSystemApp(t, nil)
	archive := downloadFrom(t, source.h)
	target := newRestoreApp(t, backup.MaxUploadBytes)

	for _, contentType := range []string{"", "application/octet-stream", "application/json", "application/gzip-foo", "gzip"} {
		t.Run(contentType, func(t *testing.T) {
			rec := postBackup(target.h, contentType, bytes.NewReader(archive))

			checkProblemCode(t, rec, http.StatusBadRequest, "invalid_request")
		})
	}
	if n := target.restarts.Load(); n != 0 {
		t.Errorf("restarts = %d, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(target.dataDir, "restore")); err == nil {
		t.Error("restore/ exists, want nothing unpacked")
	}
}

// unsizedReader hides the length of its reader, so that the request has no
// Content-Length.
type unsizedReader struct{ io.Reader }

func TestRestoreUploadTooLarge(t *testing.T) {
	source := newSystemApp(t, nil)
	writeFile(t, source.imageDir, "0199a1b2-c3d4-7e5f-8a6b-000000000001.jpg", randomBytes(16<<10))
	archive := downloadFrom(t, source.h)
	const limit = 8 << 10
	if len(archive) <= limit {
		t.Fatalf("archive has %d bytes, want more than %d", len(archive), limit)
	}
	target := newRestoreApp(t, limit)
	createProducts(t, target.h, "Alt")

	tests := []struct {
		name string
		body io.Reader
	}{
		{"with Content-Length", bytes.NewReader(archive)},
		{"without Content-Length", unsizedReader{bytes.NewReader(archive)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postBackup(target.h, "application/gzip", tt.body)

			checkProblemCode(t, rec, http.StatusRequestEntityTooLarge, "backup_too_large")
			checkRestoreRest(t, target.dataDir)
		})
	}
	if n := target.restarts.Load(); n != 0 {
		t.Errorf("restarts = %d, want 0", n)
	}
	if names := listedNames(t, target.h); !slices.Equal(names, []string{"Alt"}) {
		t.Errorf("products = %v, want [Alt]", names)
	}
}

func TestRestoreUploadInvalid(t *testing.T) {
	target := newRestoreApp(t, backup.MaxUploadBytes)
	createProducts(t, target.h, "Alt")

	rec := postBackup(target.h, "application/gzip", bytes.NewReader(randomBytes(64<<10)))

	checkProblemCode(t, rec, http.StatusUnprocessableEntity, "invalid_backup")
	checkRestoreRest(t, target.dataDir)
	if n := target.restarts.Load(); n != 0 {
		t.Errorf("restarts = %d, want 0", n)
	}
	if names := listedNames(t, target.h); !slices.Equal(names, []string{"Alt"}) {
		t.Errorf("products = %v, want [Alt]", names)
	}
}

func TestRestoreUploadAndApply(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := newSystemApp(t, nil)

	// Data through the API: two products with barcodes, movements, a
	// reversal and an uploaded image.
	beans := decodeCreated(t, post(source.h, "/api/v1/products", `{"name": "Kidneybohnen", "target": 5,
		"barcodes": [{"code": "4001686301265"}, {"code": "0034000470693", "units": 6}]}`))
	beansID, _ := beans["id"].(string)
	decodeCreated(t, post(source.h, "/api/v1/products", `{"name": "Zucker", "barcodes": [{"code": "3017620422003"}]}`))
	bookMovement(t, source.h, `{"barcode": "0034000470693", "kind": "add"}`)
	bookMovement(t, source.h, `{"product_id": "`+beansID+`", "kind": "consume", "quantity": 2}`)
	sugarAdd := bookMovement(t, source.h, `{"barcode": "3017620422003", "kind": "add", "quantity": 2}`)
	decodeMovementResult(t, reverse(source.h, sugarAdd))
	image := testPNG(t)
	if rec := putImage(source.h, beansID, "image/png", bytes.NewReader(image)); rec.Code != http.StatusOK {
		t.Fatalf("upload image: status %d, body %s", rec.Code, rec.Body.String())
	}
	original := readRestoreState(t, source.h, source.db)
	archive := downloadFrom(t, source.h)

	target := newRestoreApp(t, backup.MaxUploadBytes)
	createProducts(t, target.h, "Alt")
	oldImage := []byte("old image")
	writeFile(t, target.imageDir, "0199a1b2-c3d4-7e5f-8a6b-000000000001.jpg", oldImage)

	if rec := postBackup(target.h, "application/gzip", bytes.NewReader(archive)); rec.Code != http.StatusAccepted {
		t.Fatalf("upload: status = %d, body %s", rec.Code, rec.Body.String())
	}

	// The restart as in main: close the database, apply the backup, open
	// and migrate the database again.
	if err := target.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := backup.ApplyPending(target.dataDir, now, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("ApplyPending: %v", err)
	}
	restoredDB, err := app.OpenAndMigrate(ctx, target.dbPath, target.backupDir, store.Migrations, now)
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	t.Cleanup(func() { restoredDB.Close() })
	restored, err := app.NewHandler(target.cfg, app.Deps{
		Logger: slog.New(slog.DiscardHandler), Version: "dev", DB: restoredDB,
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(),
		ImageDir: target.imageDir, DBPath: target.dbPath, BackupDir: target.backupDir,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	checkRestoreState(t, readRestoreState(t, restored, restoredDB), original)
	rec := get(restored, "/api/v1/products/"+beansID+"/image")
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), image) {
		t.Errorf("GET image: status %d with %d bytes, want 200 with the uploaded image (%d bytes)", rec.Code, rec.Body.Len(), len(image))
	}
	// The old state is kept, restore/ is gone.
	previous := filepath.Join(target.dataDir, "vor-restore-20260925-120000")
	oldDB, err := os.ReadFile(filepath.Join(previous, "stashbert.db"))
	if err != nil {
		t.Fatalf("read old database: %v", err)
	}
	if names := productNames(t, oldDB); !slices.Equal(names, []string{"Alt"}) {
		t.Errorf("products in the old database = %v, want [Alt]", names)
	}
	if got, err := os.ReadFile(filepath.Join(previous, "images", "0199a1b2-c3d4-7e5f-8a6b-000000000001.jpg")); err != nil || !bytes.Equal(got, oldImage) {
		t.Errorf("old image = %q, %v, want %q", got, err, oldImage)
	}
	if _, err := os.Stat(filepath.Join(target.dataDir, "restore")); !os.IsNotExist(err) {
		t.Errorf("restore/: err = %v, want it removed", err)
	}
}

// dirNames returns the sorted names of the entries in dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
