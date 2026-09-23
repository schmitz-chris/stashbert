package lookup_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// maxImageBytes is the largest image that is stored.
const maxImageBytes = 2 * 1024 * 1024

// Test images. Their content is not checked, only stored.
var (
	jpegImage = []byte("\xff\xd8\xff\xe0 test jpeg")
	pngImage  = []byte("\x89PNG\r\n\x1a\n test png")
	webpImage = []byte("RIFF\x00\x00\x00\x00WEBP test webp")
)

// imageContext returns a context for a test of the image fetcher. Its
// deadline is later than the image timeout of 30 s.
func imageContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// withImage returns a reviewed product id with the image URL src, created
// and updated at storedBefore.
func withImage(id, src string) product {
	return product{ID: id, Name: "Produkt " + id, Origin: "openfoodfacts", LookupState: "done",
		ImageSourceURL: &src, CreatedAt: storedBefore, UpdatedAt: storedBefore}
}

// imageColumns holds the columns of a stored product that the image fetcher
// reads or writes.
type imageColumns struct {
	SourceURL *string
	File      *string
	UpdatedAt string
}

// storedImage returns the image columns of the product id, and false if
// there is no such product.
func storedImage(t *testing.T, sqlDB *sql.DB, id string) (imageColumns, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var c imageColumns
	err := sqlDB.QueryRowContext(ctx, "SELECT image_source_url, image_file, updated_at FROM products WHERE id = ?",
		id).Scan(&c.SourceURL, &c.File, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return imageColumns{}, false
	}
	if err != nil {
		t.Fatalf("read product %s: %v", id, err)
	}
	return c, true
}

// checkImage asserts that the product id is stored with the image columns of
// want. An empty want.UpdatedAt means an updated_at at or after start.
func checkImage(t *testing.T, sqlDB *sql.DB, id string, want imageColumns, start time.Time) {
	t.Helper()
	got, ok := storedImage(t, sqlDB, id)
	if !ok {
		t.Fatalf("product %s is not stored", id)
	}
	if want.UpdatedAt == "" {
		at, err := store.ParseTime(got.UpdatedAt)
		if err != nil || at.Before(start) || at.After(time.Now()) {
			t.Errorf("product %s: updated_at = %q, want the time of the run", id, got.UpdatedAt)
		}
		want.UpdatedAt = got.UpdatedAt
	}
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Errorf("product %s:\n got %s\nwant %s", id, g, w)
	}
}

// dirFiles returns the names of the entries of dir, none if it does not
// exist.
func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// recordingTransport records the URL of every request and the time left
// until the deadline of its context (-1 without deadline), then sends it
// with base. Without base every request fails. It counts the bytes read from
// the response bodies.
type recordingTransport struct {
	base http.RoundTripper
	read atomic.Int64

	mu        sync.Mutex
	urls      []string
	remaining []time.Duration
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	remaining := time.Duration(-1)
	if deadline, ok := req.Context().Deadline(); ok {
		remaining = time.Until(deadline)
	}
	rt.mu.Lock()
	rt.urls = append(rt.urls, req.URL.String())
	rt.remaining = append(rt.remaining, remaining)
	rt.mu.Unlock()
	if rt.base == nil {
		return nil, errors.New("test transport: no requests allowed")
	}
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, n: &rt.read}
	return resp, nil
}

// requests returns the URLs of all requests so far.
func (rt *recordingTransport) requests() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return slices.Clone(rt.urls)
}

// countingBody adds the number of bytes read to n.
type countingBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n.Add(int64(n))
	return n, err
}

// imageTest is an image fetcher that loads from a TLS test server, whose
// host is the only allowed one, into dir.
type imageTest struct {
	db        *sql.DB
	dir       string
	srv       *httptest.Server
	client    *http.Client
	transport *recordingTransport
	logs      *bytes.Buffer
	fetcher   *lookup.ImageFetcher
}

// newImageTest returns an imageTest with a migrated database, a directory
// that does not exist yet, and a test server that answers with h.
func newImageTest(t *testing.T, h http.HandlerFunc) *imageTest {
	t.Helper()
	return newImageTestWithDB(t, openDB(t), h)
}

// newImageTestWithDB is like newImageTest with the database sqlDB.
func newImageTestWithDB(t *testing.T, sqlDB *sql.DB, h http.HandlerFunc) *imageTest {
	t.Helper()
	it := &imageTest{db: sqlDB, dir: filepath.Join(t.TempDir(), "images"), logs: &bytes.Buffer{}}
	it.srv = httptest.NewTLSServer(h)
	t.Cleanup(it.srv.Close)
	it.transport = &recordingTransport{base: it.srv.Client().Transport}
	it.client = &http.Client{Transport: it.transport}
	u, err := url.Parse(it.srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	it.fetcher = lookup.NewImageFetcher(it.db, it.client, it.dir, []string{u.Host},
		slog.New(slog.NewJSONHandler(it.logs, nil)))
	return it
}

// runOnce runs one pass of the fetcher.
func (it *imageTest) runOnce(t *testing.T) {
	t.Helper()
	if err := it.fetcher.RunOnce(imageContext(t)); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

// serveImage answers with status 200, contentType and body. An empty
// contentType sends no Content-Type. Without Content-Length the body is
// sent in chunks.
func serveImage(w http.ResponseWriter, contentType string, body []byte, withLength bool) {
	if contentType == "" {
		w.Header()["Content-Type"] = nil
	} else {
		w.Header().Set("Content-Type", contentType)
	}
	if withLength {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	for chunk := range slices.Chunk(body, 64*1024) {
		if _, err := w.Write(chunk); err != nil {
			return
		}
	}
}

func TestDefaultImageHosts(t *testing.T) {
	want := []string{
		"images.openfoodfacts.org",
		"images.openbeautyfacts.org",
		"images.openpetfoodfacts.org",
		"images.openproductsfacts.org",
	}
	if !slices.Equal(lookup.DefaultImageHosts, want) {
		t.Errorf("DefaultImageHosts = %v, want %v", lookup.DefaultImageHosts, want)
	}
}

func TestImagesRejectURLWithoutRequest(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"foreign host", "https://images.example.org/front_de.jpg"},
		{"http", "http://images.openfoodfacts.org/images/products/400/168/630/1265/front_de.jpg"},
		{"subdomain of allowed host", "https://static.images.openfoodfacts.org/front_de.jpg"},
		{"allowed host as prefix", "https://images.openfoodfacts.org.example.org/front_de.jpg"},
		{"allowed host with port", "https://images.openfoodfacts.org:8443/front_de.jpg"},
		{"other scheme", "ftp://images.openfoodfacts.org/front_de.jpg"},
		{"without scheme", "images.openfoodfacts.org/front_de.jpg"},
		{"invalid URL", "https://images.openfoodfacts.org/%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			insertProduct(t, sqlDB, withImage("p1", tt.src))
			transport := &recordingTransport{}
			dir := filepath.Join(t.TempDir(), "images")
			fetcher := lookup.NewImageFetcher(sqlDB, &http.Client{Transport: transport}, dir,
				lookup.DefaultImageHosts, slog.New(slog.DiscardHandler))
			start := time.Now().UTC().Truncate(time.Millisecond)

			if err := fetcher.RunOnce(imageContext(t)); err != nil {
				t.Fatalf("RunOnce: %v", err)
			}

			if got := transport.requests(); len(got) != 0 {
				t.Errorf("requests = %v, want none", got)
			}
			checkImage(t, sqlDB, "p1", imageColumns{}, start)
			if files := dirFiles(t, dir); len(files) != 0 {
				t.Errorf("files = %v, want none", files)
			}
		})
	}
}

func TestImagesStore(t *testing.T) {
	largest := bytes.Repeat([]byte{0xab}, maxImageBytes)
	it := newImageTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/front.jpg":
			serveImage(w, "image/jpeg", jpegImage, true)
		case "/front.png":
			serveImage(w, "image/png; charset=binary", pngImage, true)
		case "/front.webp":
			serveImage(w, "Image/WebP", webpImage, false)
		case "/largest.jpg":
			serveImage(w, "image/jpeg", largest, false)
		default:
			http.NotFound(w, r)
		}
	})
	want := map[string][]byte{"p1.jpg": jpegImage, "p2.png": pngImage, "p3.webp": webpImage, "p4.jpg": largest}
	for i, path := range []string{"/front.jpg", "/front.png", "/front.webp", "/largest.jpg"} {
		p := withImage(fmt.Sprintf("p%d", i+1), it.srv.URL+path)
		p.CreatedAt = fmt.Sprintf("2026-09-0%dT12:00:00.000Z", i+1)
		insertProduct(t, it.db, p)
	}
	start := time.Now().UTC().Truncate(time.Millisecond)

	it.runOnce(t)

	for i, name := range []string{"p1.jpg", "p2.png", "p3.webp", "p4.jpg"} {
		id := fmt.Sprintf("p%d", i+1)
		src := it.srv.URL + []string{"/front.jpg", "/front.png", "/front.webp", "/largest.jpg"}[i]
		checkImage(t, it.db, id, imageColumns{SourceURL: &src, File: new(name)}, start)

		path := filepath.Join(it.dir, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read image of %s: %v", id, err)
		}
		if !bytes.Equal(got, want[name]) {
			t.Errorf("%s holds %d bytes, want the %d bytes of the image", name, len(got), len(want[name]))
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode of %s = %v, want 0640", name, info.Mode().Perm())
		}
	}
	if files := dirFiles(t, it.dir); !slices.Equal(files, []string{"p1.jpg", "p2.png", "p3.webp", "p4.jpg"}) {
		t.Errorf("files = %v, want only the four images", files)
	}
	info, err := os.Stat(it.dir)
	if err != nil {
		t.Fatalf("stat image dir: %v", err)
	}
	// The umask may remove bits of 0750, but never adds any.
	if perm := info.Mode().Perm(); perm&^0o750 != 0 || perm&0o700 != 0o700 {
		t.Errorf("mode of image dir = %v, want 0750", perm)
	}
}

func TestImagesRejectAnswer(t *testing.T) {
	tooLarge := bytes.Repeat([]byte{0xab}, maxImageBytes+1)
	tests := []struct {
		name   string
		handle http.HandlerFunc
	}{
		{"not found", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }},
		{"html", func(w http.ResponseWriter, _ *http.Request) {
			serveImage(w, "text/html; charset=utf-8", []byte("<html></html>"), true)
		}},
		{"gif", func(w http.ResponseWriter, _ *http.Request) { serveImage(w, "image/gif", []byte("GIF89a"), true) }},
		{"without content type", func(w http.ResponseWriter, _ *http.Request) { serveImage(w, "", jpegImage, true) }},
		{"too large with length", func(w http.ResponseWriter, _ *http.Request) {
			serveImage(w, "image/jpeg", tooLarge, true)
		}},
		{"too large without length", func(w http.ResponseWriter, _ *http.Request) {
			serveImage(w, "image/jpeg", tooLarge, false)
		}},
		{"far too large without length", func(w http.ResponseWriter, _ *http.Request) {
			serveImage(w, "image/jpeg", bytes.Repeat([]byte{0xab}, 4*maxImageBytes), false)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := newImageTest(t, tt.handle)
			insertProduct(t, it.db, withImage("p1", it.srv.URL+"/front.jpg"))
			start := time.Now().UTC().Truncate(time.Millisecond)

			it.runOnce(t)

			if got := it.transport.requests(); len(got) != 1 {
				t.Errorf("requests = %v, want one", got)
			}
			checkImage(t, it.db, "p1", imageColumns{}, start)
			if files := dirFiles(t, it.dir); len(files) != 0 {
				t.Errorf("files = %v, want none", files)
			}
			if n := it.transport.read.Load(); n > maxImageBytes+1 {
				t.Errorf("read %d bytes of the body, want at most %d", n, maxImageBytes+1)
			}
		})
	}
}

func TestImagesTemporaryError(t *testing.T) {
	tests := []struct {
		name string
		// fail answers the request for the image of p1.
		fail http.HandlerFunc
		// reason is part of the logged error.
		reason string
	}{
		{"server error", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, "status 500"},
		{"unavailable", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "later", http.StatusServiceUnavailable)
		}, "status 503"},
		{"forbidden", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusForbidden)
		}, "status 403"},
		{"connection closed", func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
		}, "EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := newImageTest(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/fail.jpg" {
					tt.fail(w, r)
					return
				}
				serveImage(w, "image/jpeg", jpegImage, true)
			})
			failing := withImage("p1", it.srv.URL+"/fail.jpg")
			insertProduct(t, it.db, failing)
			loaded := withImage("p2", it.srv.URL+"/front.jpg")
			loaded.CreatedAt = "2026-09-02T12:00:00.000Z"
			insertProduct(t, it.db, loaded)
			start := time.Now().UTC().Truncate(time.Millisecond)

			it.runOnce(t)

			// The error leaves p1 as it is; the run goes on with p2.
			checkImage(t, it.db, "p1", imageColumns{SourceURL: failing.ImageSourceURL, UpdatedAt: storedBefore}, start)
			checkImage(t, it.db, "p2", imageColumns{SourceURL: loaded.ImageSourceURL, File: new("p2.jpg")}, start)
			if files := dirFiles(t, it.dir); !slices.Equal(files, []string{"p2.jpg"}) {
				t.Errorf("files = %v, want only p2.jpg", files)
			}

			var record struct {
				Level     string `json:"level"`
				ProductID string `json:"product_id"`
				Error     string `json:"error"`
			}
			if err := json.Unmarshal(it.logs.Bytes(), &record); err != nil {
				t.Fatalf("log = %q, want one JSON record: %v", it.logs.String(), err)
			}
			if record.Level != "WARN" || record.ProductID != "p1" || !strings.Contains(record.Error, tt.reason) {
				t.Errorf("log record = %+v, want a warning about p1 with %q", record, tt.reason)
			}
		})
	}
}

func TestImagesTimeout(t *testing.T) {
	it := newImageTest(t, func(w http.ResponseWriter, _ *http.Request) {
		serveImage(w, "image/jpeg", jpegImage, true)
	})
	insertProduct(t, it.db, withImage("p1", it.srv.URL+"/front.jpg"))

	it.runOnce(t)

	it.transport.mu.Lock()
	defer it.transport.mu.Unlock()
	// The lower bound only allows for the time between creating the context
	// and sending the request.
	if len(it.transport.remaining) != 1 || it.transport.remaining[0] <= 29*time.Second ||
		it.transport.remaining[0] > 30*time.Second {
		t.Errorf("request deadlines in %v, want one of at most 30 s", it.transport.remaining)
	}
}

func TestImagesRedirect(t *testing.T) {
	tests := []struct {
		name string
		// location returns the target of the redirect for the host of the
		// test server.
		location func(host, port string) string
		// followed reports whether the redirect is followed.
		followed bool
	}{
		{"allowed host", func(host, port string) string { return "https://" + net.JoinHostPort(host, port) + "/front.jpg" }, true},
		{"foreign host", func(_, port string) string { return "https://localhost:" + port + "/front.jpg" }, false},
		{"http", func(host, port string) string { return "http://" + net.JoinHostPort(host, port) + "/front.jpg" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var images atomic.Int32
			it := newImageTest(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					host, port, _ := net.SplitHostPort(r.Host)
					http.Redirect(w, r, tt.location(host, port), http.StatusFound)
					return
				}
				images.Add(1)
				serveImage(w, "image/jpeg", jpegImage, true)
			})
			src := it.srv.URL + "/redirect"
			insertProduct(t, it.db, withImage("p1", src))
			start := time.Now().UTC().Truncate(time.Millisecond)

			it.runOnce(t)

			if tt.followed {
				if n := images.Load(); n != 1 {
					t.Errorf("image requests = %d, want 1", n)
				}
				checkImage(t, it.db, "p1", imageColumns{SourceURL: &src, File: new("p1.jpg")}, start)
				if files := dirFiles(t, it.dir); !slices.Equal(files, []string{"p1.jpg"}) {
					t.Errorf("files = %v, want only p1.jpg", files)
				}
			} else {
				if got := it.transport.requests(); !slices.Equal(got, []string{src}) {
					t.Errorf("requests = %v, want only %s", got, src)
				}
				checkImage(t, it.db, "p1", imageColumns{}, start)
				if files := dirFiles(t, it.dir); len(files) != 0 {
					t.Errorf("files = %v, want none", files)
				}
			}
			if it.client.CheckRedirect != nil {
				t.Error("NewImageFetcher changed the CheckRedirect of the given client")
			}
		})
	}
}

func TestImagesAtMostTen(t *testing.T) {
	it := newImageTest(t, func(w http.ResponseWriter, _ *http.Request) {
		serveImage(w, "image/jpeg", jpegImage, true)
	})
	// Products without image URL or with an image file are older than all
	// others and are never loaded.
	withoutURL := withImage("without-url", "")
	withoutURL.ImageSourceURL, withoutURL.CreatedAt = nil, "2026-08-01T12:00:00.000Z"
	insertProduct(t, it.db, withoutURL)
	withFile := withImage("with-file", it.srv.URL+"/with-file.jpg")
	withFile.CreatedAt = "2026-08-01T12:00:00.000Z"
	insertProduct(t, it.db, withFile)
	exec(t, it.db, "UPDATE products SET image_file = 'with-file.jpg' WHERE id = 'with-file'")
	// 12 products, inserted in another order than created.
	for i := range 12 {
		minute := (i * 5) % 12
		p := withImage(fmt.Sprintf("p%02d", minute), fmt.Sprintf("%s/p%02d.jpg", it.srv.URL, minute))
		p.CreatedAt = fmt.Sprintf("2026-09-01T12:%02d:00.000Z", minute)
		insertProduct(t, it.db, p)
	}
	var want, wantFiles []string
	for minute := range 10 {
		want = append(want, fmt.Sprintf("%s/p%02d.jpg", it.srv.URL, minute))
		wantFiles = append(wantFiles, fmt.Sprintf("p%02d.jpg", minute))
	}

	it.runOnce(t)

	if got := it.transport.requests(); !slices.Equal(got, want) {
		t.Errorf("requests:\n got %v\nwant %v", got, want)
	}
	if files := dirFiles(t, it.dir); !slices.Equal(files, wantFiles) {
		t.Errorf("files = %v, want %v", files, wantFiles)
	}
	for _, id := range []string{"p10", "p11"} {
		src := fmt.Sprintf("%s/%s.jpg", it.srv.URL, id)
		checkImage(t, it.db, id, imageColumns{SourceURL: &src, UpdatedAt: storedBefore}, time.Time{})
	}
}

func TestImagesProductChangedDuringDownload(t *testing.T) {
	const changedAt = "2026-09-03T12:00:00.000Z"
	const newURL = "https://images.openfoodfacts.org/images/products/400/168/630/1265/front_de.2.jpg"
	tests := []struct {
		name   string
		change string
		// want is the stored product afterwards, nil if it is deleted.
		want *imageColumns
	}{
		{"deleted", "DELETE FROM products WHERE id = 'p1'", nil},
		{"new image URL", "UPDATE products SET image_source_url = '" + newURL + "', updated_at = '" + changedAt +
			"' WHERE id = 'p1'", &imageColumns{SourceURL: new(newURL), UpdatedAt: changedAt}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB := openDB(t)
			it := newImageTestWithDB(t, sqlDB, func(w http.ResponseWriter, r *http.Request) {
				// The change runs while the image is downloaded.
				if _, err := sqlDB.ExecContext(r.Context(), tt.change); err != nil {
					t.Errorf("change product: %v", err)
				}
				serveImage(w, "image/jpeg", jpegImage, true)
			})
			insertProduct(t, it.db, withImage("p1", it.srv.URL+"/front.jpg"))

			it.runOnce(t)

			if tt.want == nil {
				if _, ok := storedImage(t, it.db, "p1"); ok {
					t.Error("deleted product is stored again")
				}
			} else {
				checkImage(t, it.db, "p1", *tt.want, time.Time{})
			}
			if files := dirFiles(t, it.dir); len(files) != 0 {
				t.Errorf("files = %v, want none", files)
			}
		})
	}
}

func TestImagesDatabaseError(t *testing.T) {
	it := newImageTest(t, func(w http.ResponseWriter, _ *http.Request) {
		serveImage(w, "image/jpeg", jpegImage, true)
	})
	it.db.Close()
	if err := it.fetcher.RunOnce(imageContext(t)); err == nil {
		t.Error("RunOnce on a closed database: err = nil, want error")
	}
}

func TestImagesStart(t *testing.T) {
	asked := make(chan struct{})
	var once sync.Once
	it := newImageTest(t, func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(asked) })
		http.Error(w, "later", http.StatusServiceUnavailable)
	})
	insertProduct(t, it.db, withImage("p1", it.srv.URL+"/front.jpg"))
	ctx, cancel := context.WithCancel(imageContext(t))
	defer cancel()
	done := make(chan struct{})

	go func() {
		defer close(done)
		it.fetcher.Start(ctx, 10*time.Millisecond)
	}()

	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not load an image within 5 s")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5 s after the end of its context")
	}
}
