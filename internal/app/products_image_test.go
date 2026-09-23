package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/events"
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
