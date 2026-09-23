package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// imageContentTypes maps the extensions of the stored image files to their
// content types (architecture.md, 7.3).
var imageContentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// ProductImage is an opened image file of a product. The caller closes File.
type ProductImage struct {
	File        *os.File
	ContentType string
	Size        int64
}

// OpenProductImage opens the image file of the product with id in imageDir.
// The content type follows from the extension of the file name.
//
// An unknown id, a product without image file and a missing file result in
// 404 not_found, and so does a stored file name that contains a path
// separator or has no extension of an image type.
func OpenProductImage(ctx context.Context, sqlDB *sql.DB, imageDir, id string) (ProductImage, error) {
	row, err := db.New(sqlDB).GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ProductImage{}, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return ProductImage{}, fmt.Errorf("open image of product %s: %w", id, err)
	}
	if row.ImageFile == nil {
		return ProductImage{}, httpx.NotFound("Produkt hat kein Bild")
	}
	path, ok := imagePath(imageDir, *row.ImageFile)
	contentType, known := imageContentTypes[filepath.Ext(*row.ImageFile)]
	if !ok || !known {
		return ProductImage{}, httpx.NotFound("Bilddatei nicht gefunden")
	}

	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ProductImage{}, httpx.NotFound("Bilddatei nicht gefunden")
	}
	if err != nil {
		return ProductImage{}, fmt.Errorf("open image of product %s: %w", id, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return ProductImage{}, fmt.Errorf("open image of product %s: %w", id, err)
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return ProductImage{}, httpx.NotFound("Bilddatei nicht gefunden")
	}
	return ProductImage{File: f, ContentType: contentType, Size: info.Size()}, nil
}

// imagePath returns the path of the stored image file name in imageDir. ok is
// false if name is empty or contains a path separator, so the path never
// leaves imageDir.
func imagePath(imageDir, name string) (path string, ok bool) {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	return filepath.Join(imageDir, name), true
}

// removeImage removes the stored image file name from imageDir. A missing
// file or a name that imagePath rejects is ignored, and so are other errors:
// the product is already deleted, and the file of a deleted product is never
// read again because product IDs are not reused.
func removeImage(imageDir, name string) {
	if path, ok := imagePath(imageDir, name); ok {
		_ = os.Remove(path)
	}
}
