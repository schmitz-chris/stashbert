package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// writeFile writes data to the file name in dir.
func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// setImageFile stores name as the image file of the product id.
func setImageFile(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "UPDATE products SET image_file = ? WHERE id = ?", name, id); err != nil {
		t.Fatalf("set image file of %s: %v", id, err)
	}
}

func TestGetProductImage(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	// p2 has the image file p2.jpg (see insertProducts).
	setImageFile(t, db, "p1", "p1.png")
	setImageFile(t, db, "p4", "p4.webp")

	tests := []struct {
		id, file, contentType string
	}{
		{"p2", "p2.jpg", "image/jpeg"},
		{"p1", "p1.png", "image/png"},
		{"p4", "p4.webp", "image/webp"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data := []byte("bytes of " + tt.file)
			writeFile(t, imageDir, tt.file, data)

			rec := get(h, "/api/v1/products/"+tt.id+"/image")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			for header, want := range map[string]string{
				"Content-Type":   tt.contentType,
				"Content-Length": strconv.Itoa(len(data)),
				"Cache-Control":  "private, max-age=86400",
			} {
				if got := rec.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
			if !bytes.Equal(rec.Body.Bytes(), data) {
				t.Errorf("body = %q, want %q", rec.Body.Bytes(), data)
			}
		})
	}
}

func TestGetProductImageNotFound(t *testing.T) {
	base := t.TempDir()
	imageDir := filepath.Join(base, "images")
	if err := os.Mkdir(imageDir, 0o700); err != nil {
		t.Fatalf("create image dir: %v", err)
	}
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	// p3 has no image file and the file p2.jpg of p2 is missing. The files
	// of p4 and p1 exist, but their stored names are not accepted.
	writeFile(t, base, "p4.jpg", []byte("jpg"))
	setImageFile(t, db, "p4", "../p4.jpg")
	writeFile(t, imageDir, "p1.gif", []byte("gif"))
	setImageFile(t, db, "p1", "p1.gif")

	tests := []struct {
		name, id string
	}{
		{"without image file", "p3"},
		{"missing file", "p2"},
		{"path separator", "p4"},
		{"unknown extension", "p1"},
		{"unknown id", "unbekannt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(h, "/api/v1/products/"+tt.id+"/image")

			checkProblemCode(t, rec, http.StatusNotFound, "not_found")
		})
	}
}

// testPNG returns a small PNG image.
func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// testJPEG returns a small JPEG image.
func testJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// testWebP is the start of a WebP image, enough for http.DetectContentType.
var testWebP = []byte("RIFF\x24\x00\x00\x00WEBPVP8 \x18\x00\x00\x00test webp")

// putImage sends body with contentType as the image of the product id to h.
func putImage(h http.Handler, id, contentType string, body io.Reader) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/products/"+id+"/image", body)
	req.Header.Set("Content-Type", contentType)
	h.ServeHTTP(rec, req)
	return rec
}

// deleteImage sends a DELETE request for the image of the product id to h.
func deleteImage(h http.Handler, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/products/"+id+"/image", nil))
	return rec
}

// imageRow holds the image columns and updated_at of a stored product.
type imageRow struct {
	File, SourceURL sql.NullString
	UpdatedAt       string
}

// storedImageRow returns the image columns and updated_at of the product id.
func storedImageRow(t *testing.T, db *sql.DB, id string) imageRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var r imageRow
	err := db.QueryRowContext(ctx, "SELECT image_file, image_source_url, updated_at FROM products WHERE id = ?", id).
		Scan(&r.File, &r.SourceURL, &r.UpdatedAt)
	if err != nil {
		t.Fatalf("read product %s: %v", id, err)
	}
	return r
}

// checkUploaded asserts that the product id has an uploaded image file with
// the extension ext, which is the only file in imageDir and is served with
// contentType and data. The product has no image URL and was updated at or
// after start.
func checkUploaded(t *testing.T, h http.Handler, db *sql.DB, imageDir, id, ext, contentType string, data []byte, start time.Time) {
	t.Helper()
	row := storedImageRow(t, db, id)
	if want := regexp.MustCompile(`^` + id + `-upload-[0-9]+\.` + ext + `$`); !want.MatchString(row.File.String) {
		t.Errorf("image_file = %v, want a match of %s", row.File, want)
	}
	if row.SourceURL.Valid {
		t.Errorf("image_source_url = %q, want NULL", row.SourceURL.String)
	}
	if updatedAt, err := store.ParseTime(row.UpdatedAt); err != nil || updatedAt.Before(start) {
		t.Errorf("updated_at = %q, want the time of the upload", row.UpdatedAt)
	}
	if got := imageFiles(t, imageDir); !slices.Equal(got, []string{row.File.String}) {
		t.Errorf("image files = %q, want only %q", got, row.File.String)
	}

	rec := get(h, "/api/v1/products/"+id+"/image")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET image: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != contentType {
		t.Errorf("GET image: Content-Type = %q, want %q", ct, contentType)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Errorf("GET image: body differs from the uploaded image")
	}
}

func TestUploadProductImage(t *testing.T) {
	tests := []struct {
		name, contentType, ext string
		data                   func(t *testing.T) []byte
	}{
		{"jpeg", "image/jpeg", "jpg", testJPEG},
		{"png", "image/png", "png", testPNG},
		{"webp", "image/webp", "webp", func(*testing.T) []byte { return testWebP }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageDir := t.TempDir()
			h, db := newAppWithImages(t, events.Nop{}, imageDir)
			insertProducts(t, db)
			// p2 has the image p2.jpg loaded from its image URL.
			writeFile(t, imageDir, "p2.jpg", []byte("OFF image"))
			data := tt.data(t)
			start := time.Now().UTC().Truncate(time.Millisecond)

			rec := putImage(h, "p2", tt.contentType, bytes.NewReader(data))

			p := decodeProduct(t, rec)
			if p["id"] != "p2" || p["has_image"] != true {
				t.Errorf("product = %v, want p2 with has_image true", p)
			}
			checkUpdatedSince(t, p, start)
			checkUploaded(t, h, db, imageDir, "p2", tt.ext, tt.contentType, data, start)
		})
	}
}

func TestUploadProductImageReplacesUpload(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	pngData, jpegData := testPNG(t), testJPEG(t)
	start := time.Now().UTC().Truncate(time.Millisecond)

	// The type follows from the content, not from the Content-Type.
	if rec := putImage(h, "p1", "image/jpeg", bytes.NewReader(pngData)); rec.Code != http.StatusOK {
		t.Fatalf("first upload: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkUploaded(t, h, db, imageDir, "p1", "png", "image/png", pngData, start)
	if rec := putImage(h, "p1", "image/png", bytes.NewReader(jpegData)); rec.Code != http.StatusOK {
		t.Fatalf("second upload: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	checkUploaded(t, h, db, imageDir, "p1", "jpg", "image/jpeg", jpegData, start)
}

// countingReader counts the bytes read from it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// paddedPNG returns a PNG signature padded with zero bytes to size bytes.
func paddedPNG(size int) []byte {
	data := make([]byte, size)
	copy(data, "\x89PNG\r\n\x1a\n")
	return data
}

func TestUploadProductImageInvalid(t *testing.T) {
	tests := []struct {
		name, contentType string
		body              []byte
		status            int
		code              string
	}{
		{"text as jpeg", "image/jpeg", []byte("kein Bild"), http.StatusUnprocessableEntity, "invalid_image"},
		{"gif", "image/gif", []byte("GIF89a\x01\x00\x01\x00"), http.StatusUnprocessableEntity, "invalid_image"},
		{"over 2 MB", "image/png", paddedPNG(lookup.MaxImageBytes + 1), http.StatusUnprocessableEntity, "invalid_image"},
		{"far over 2 MB", "image/png", paddedPNG(4 * lookup.MaxImageBytes), http.StatusUnprocessableEntity, "invalid_image"},
		{"empty", "image/png", nil, http.StatusBadRequest, "invalid_request"},
		{"no image type", "application/json", testPNG(t), http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageDir := t.TempDir()
			h, db := newAppWithImages(t, events.Nop{}, imageDir)
			insertProducts(t, db)
			writeFile(t, imageDir, "p2.jpg", []byte("OFF image"))
			before := storedImageRow(t, db, "p2")
			body := &countingReader{r: bytes.NewReader(tt.body)}

			rec := putImage(h, "p2", tt.contentType, body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// The limit applies before the validator reads the body.
			if body.n > lookup.MaxImageBytes+1 {
				t.Errorf("read %d bytes of the body, want at most %d", body.n, lookup.MaxImageBytes+1)
			}
			if got := storedImageRow(t, db, "p2"); got != before {
				t.Errorf("product = %+v, want unchanged %+v", got, before)
			}
			if got := imageFiles(t, imageDir); !slices.Equal(got, []string{"p2.jpg"}) {
				t.Errorf("image files = %q, want only p2.jpg", got)
			}
		})
	}
}

func TestUploadProductImageLargest(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	data := paddedPNG(lookup.MaxImageBytes)
	start := time.Now().UTC().Truncate(time.Millisecond)

	rec := putImage(h, "p1", "image/png", bytes.NewReader(data))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	checkUploaded(t, h, db, imageDir, "p1", "png", "image/png", data, start)
}

func TestUploadProductImageLimitInDomain(t *testing.T) {
	imageDir := t.TempDir()
	_, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	body := &countingReader{r: bytes.NewReader(paddedPNG(4 * lookup.MaxImageBytes))}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := domain.UploadProductImage(ctx, db, imageDir, "p1", body)

	var e *httpx.Error
	if !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity || e.Code != "invalid_image" {
		t.Errorf("err = %v, want 422 invalid_image", err)
	}
	if body.n != lookup.MaxImageBytes+1 {
		t.Errorf("read %d bytes of the body, want %d", body.n, lookup.MaxImageBytes+1)
	}
	if got := imageFiles(t, imageDir); len(got) != 0 {
		t.Errorf("image files = %q, want none", got)
	}
}

func TestUploadProductImageUnknownProduct(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)

	rec := putImage(h, "unbekannt", "image/png", bytes.NewReader(testPNG(t)))

	checkProblemCode(t, rec, http.StatusNotFound, "not_found")
	if got := imageFiles(t, imageDir); len(got) != 0 {
		t.Errorf("image files = %q, want none", got)
	}
}

// failingTransport records every request and lets it fail.
type failingTransport struct {
	mu       sync.Mutex
	requests []string
}

func (ft *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.requests = append(ft.requests, req.URL.String())
	return nil, errors.New("test transport: no requests allowed")
}

// runImageFetcher runs one pass of an image fetcher for db and imageDir with
// the default hosts and transport.
func runImageFetcher(t *testing.T, db *sql.DB, imageDir string, transport http.RoundTripper) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	fetcher := lookup.NewImageFetcher(db, &http.Client{Transport: transport}, imageDir, lookup.DefaultImageHosts,
		slog.New(slog.DiscardHandler))
	if err := fetcher.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

func TestDeleteProductImage(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)
	// p2 has the image p2.jpg loaded from its image URL; the file of p1 is
	// missing; p4 has no image.
	writeFile(t, imageDir, "p2.jpg", []byte("OFF image"))
	writeFile(t, imageDir, "p3.png", []byte("other image"))
	setImageFile(t, db, "p3", "p3.png")
	setImageFile(t, db, "p1", "p1.png")

	for _, id := range []string{"p2", "p1", "p4"} {
		t.Run(id, func(t *testing.T) {
			start := time.Now().UTC().Truncate(time.Millisecond)

			rec := deleteImage(h, id)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			row := storedImageRow(t, db, id)
			if row.File.Valid || row.SourceURL.Valid {
				t.Errorf("image_file = %v, image_source_url = %v, want both NULL", row.File, row.SourceURL)
			}
			if updatedAt, err := store.ParseTime(row.UpdatedAt); err != nil || updatedAt.Before(start) {
				t.Errorf("updated_at = %q, want the time of the request", row.UpdatedAt)
			}
			if p := decodeProduct(t, get(h, "/api/v1/products/"+id)); p["has_image"] != false {
				t.Errorf("has_image = %v, want false", p["has_image"])
			}
			checkProblemCode(t, get(h, "/api/v1/products/"+id+"/image"), http.StatusNotFound, "not_found")
		})
	}

	// Only the file of p3 is left, and the image fetcher loads nothing.
	if got := imageFiles(t, imageDir); !slices.Equal(got, []string{"p3.png"}) {
		t.Errorf("image files = %q, want only p3.png", got)
	}
	transport := &failingTransport{}
	runImageFetcher(t, db, imageDir, transport)
	if len(transport.requests) != 0 {
		t.Errorf("image fetcher requests = %q, want none", transport.requests)
	}
	if row := storedImageRow(t, db, "p2"); row.File.Valid || row.SourceURL.Valid {
		t.Errorf("p2 after the image fetcher: image_file = %v, image_source_url = %v, want both NULL", row.File, row.SourceURL)
	}
}

func TestDeleteProductImageUnknownProduct(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	checkProblemCode(t, deleteImage(h, "unbekannt"), http.StatusNotFound, "not_found")
}

// TestUploadProductImageWhileFetcherRuns uploads a photo while the image
// fetcher downloads the image of the old image URL of the product. The
// fetcher must neither replace nor remove the uploaded file, and it loads
// nothing afterwards.
func TestUploadProductImageWhileFetcherRuns(t *testing.T) {
	imageDir := t.TempDir()
	h, db := newAppWithImages(t, events.Nop{}, imageDir)
	insertProducts(t, db)

	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("OFF image"))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, "UPDATE products SET image_source_url = ? WHERE id = 'p1'", srv.URL+"/p1.jpg"); err != nil {
		t.Fatalf("set image URL: %v", err)
	}
	fetcher := lookup.NewImageFetcher(db, srv.Client(), imageDir, []string{u.Host}, slog.New(slog.DiscardHandler))

	done := make(chan error, 1)
	go func() { done <- fetcher.RunOnce(ctx) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("image fetcher sent no request")
	}
	// A JPEG, so the fetcher writes a file with the same extension.
	data := testJPEG(t)
	start := time.Now().UTC().Truncate(time.Millisecond)
	rec := putImage(h, "p1", "image/jpeg", bytes.NewReader(data))
	close(release)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := <-done; err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	checkUploaded(t, h, db, imageDir, "p1", "jpg", "image/jpeg", data, start)

	if err := fetcher.RunOnce(ctx); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("image requests = %d, want 1", n)
	}
	checkUploaded(t, h, db, imageDir, "p1", "jpg", "image/jpeg", data, start)
}
