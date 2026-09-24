package domain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
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

// ImageTooLarge returns the 422 invalid_image error for an image larger than
// lookup.MaxImageBytes.
func ImageTooLarge() *httpx.Error {
	return httpx.NewError(http.StatusUnprocessableEntity, "invalid_image", "Das Bild ist größer als 2 MB")
}

// UploadProductImage stores the image in body as the image file of the
// product with id in imageDir and returns the product (architecture.md, 7.3).
// The type of the image is detected from its content with
// http.DetectContentType. The file is written through a temporary file under
// the name <id>-upload-<unix-millis>.<ext>, which the image fetcher never
// writes. In one transaction the product gets the file as its image file, no
// image URL and a new updated_at. After the commit the previous image file of
// the product is removed.
//
// At most lookup.MaxImageBytes and one byte of body are read. A larger image
// and an image that is no JPEG, PNG or WebP result in 422 invalid_image, an
// unknown id in 404 not_found; in these cases no file is written.
func UploadProductImage(ctx context.Context, sqlDB *sql.DB, imageDir, id string, body io.Reader) (Product, error) {
	data, err := io.ReadAll(io.LimitReader(body, lookup.MaxImageBytes+1))
	if err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: read image: %w", id, err)
	}
	if len(data) > lookup.MaxImageBytes {
		return Product{}, ImageTooLarge()
	}
	ext, ok := lookup.ImageExtension(http.DetectContentType(data))
	if !ok {
		return Product{}, httpx.NewError(http.StatusUnprocessableEntity, "invalid_image",
			"Das Bild ist kein JPEG, PNG oder WebP")
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf("upload image: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: %w", id, err)
	}
	var previous string
	if cur.ImageFile != nil {
		previous = *cur.ImageFile
	}
	now := time.Now()
	name := fmt.Sprintf("%s-upload-%d.%s", cur.ID, now.UnixMilli(), ext)
	if err := lookup.WriteImageFile(imageDir, name, data); err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: %w", id, err)
	}
	// Two uploads within the same millisecond write the same file name, so
	// the new file is only removed or the previous one kept if the names
	// differ.
	committed := false
	defer func() {
		if !committed && name != previous {
			removeImage(imageDir, name)
		}
	}()

	row, err := q.SetProductImage(ctx, db.SetProductImageParams{ImageFile: &name, UpdatedAt: store.FormatTime(now), ID: cur.ID})
	if err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: %w", id, err)
	}
	barcodes, err := q.ListProductBarcodes(ctx, row.ID)
	if err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: list barcodes: %w", id, err)
	}
	p, err := ProductFromDB(row, barcodes)
	if err != nil {
		return Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("upload image of product %s: commit: %w", id, err)
	}
	committed = true

	if previous != name {
		removeImage(imageDir, previous)
	}
	return p, nil
}

// DeleteProductImage removes the image of the product with id: in one
// transaction the product gets no image file, no image URL and a new
// updated_at, so the image fetcher does not load the image again. After the
// commit the image file is removed from imageDir; a missing file is no error,
// and neither is a product without image. An unknown id results in 404
// not_found.
func DeleteProductImage(ctx context.Context, sqlDB *sql.DB, imageDir, id string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete image: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)

	cur, err := q.GetProduct(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NotFound("Produkt nicht gefunden")
	}
	if err != nil {
		return fmt.Errorf("delete image of product %s: %w", id, err)
	}
	arg := db.SetProductImageParams{ImageFile: nil, UpdatedAt: store.FormatTime(time.Now()), ID: cur.ID}
	if _, err := q.SetProductImage(ctx, arg); err != nil {
		return fmt.Errorf("delete image of product %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete image of product %s: commit: %w", id, err)
	}

	if cur.ImageFile != nil {
		removeImage(imageDir, *cur.ImageFile)
	}
	return nil
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

// removeImage removes the image file name, which no product refers to any
// more, from imageDir. A missing file or a name that imagePath rejects is
// ignored, and so are other errors: the file is never read again, because
// product IDs are not reused and uploaded files get a new name each time.
func removeImage(imageDir, name string) {
	if path, ok := imagePath(imageDir, name); ok {
		_ = os.Remove(path)
	}
}
