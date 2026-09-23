package lookup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// DefaultImageHosts are the hosts product images are loaded from
// (architecture.md, 7.3).
var DefaultImageHosts = []string{
	"images.openfoodfacts.org",
	"images.openbeautyfacts.org",
	"images.openpetfoodfacts.org",
	"images.openproductsfacts.org",
}

const (
	// maxImagesPerRun is the number of images loaded per run at most.
	maxImagesPerRun = 10
	// imageTimeout is the timeout of the download of one image.
	imageTimeout = 30 * time.Second
	// maxImageBytes is the largest image that is stored (architecture.md, 7.3).
	maxImageBytes = 2 * 1024 * 1024
	// maxImageRedirects is the number of redirects followed per image, as in
	// the default policy of http.Client.
	maxImageRedirects = 10
)

// errImageRejected marks an image that is never loaded: its URL or the
// target of a redirect is not https or not on an allowed host, or the answer
// is a 404, has another content type or is larger than 2 MB.
var errImageRejected = errors.New("image rejected")

// ImageFetcher loads the images of products in the background and stores
// them as files (architecture.md, 7.3). It publishes no events.
type ImageFetcher struct {
	db      *sql.DB
	client  *http.Client
	dir     string
	allowed map[string]bool
	logger  *slog.Logger
}

// NewImageFetcher returns an ImageFetcher for the products in sqlDB that
// stores the images in dir and logs to logger. It loads images with a copy of
// httpClient that follows a redirect only to https on a host in
// allowedHosts. Hosts are compared exactly with the host of the URL,
// including a port.
func NewImageFetcher(sqlDB *sql.DB, httpClient *http.Client, dir string, allowedHosts []string, logger *slog.Logger) *ImageFetcher {
	f := &ImageFetcher{
		db:      sqlDB,
		dir:     dir,
		allowed: make(map[string]bool, len(allowedHosts)),
		logger:  logger,
	}
	for _, host := range allowedHosts {
		f.allowed[host] = true
	}
	client := *httpClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !f.allowedURL(req.URL) {
			return fmt.Errorf("%w: redirect to %s", errImageRejected, req.URL.Redacted())
		}
		if len(via) >= maxImageRedirects {
			return fmt.Errorf("stopped after %d redirects", maxImageRedirects)
		}
		return nil
	}
	f.client = &client
	return f
}

// Start calls RunOnce at every tick of a ticker with interval until ctx ends;
// the first run starts one interval after the call. A failed run is logged
// with level error and the next tick runs again. Start blocks until ctx ends.
func (f *ImageFetcher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// A run that fails because ctx ended is no error of the job.
			if err := f.RunOnce(ctx); err != nil && ctx.Err() == nil {
				f.logger.LogAttrs(ctx, slog.LevelError, "load product images",
					slog.String("error", err.Error()))
			}
		}
	}
}

// RunOnce loads the images of at most 10 products with an image URL and
// without an image file, the oldest created_at first (architecture.md, 7.3).
// Each image is downloaded with a timeout of 30 s and stored as
// <dir>/<product_id>.<jpg|png|webp>; then the product gets the file name as
// its image file.
//
// A URL that is not https or not on an allowed host is removed from the
// product without a request, as is the URL of a redirect to such a URL, a
// 404, another content type than JPEG, PNG or WebP, and an image larger than
// 2 MB. Other errors of the download leave the product as it is; they are
// logged with level warn and the run goes on, so the next run tries again.
// RunOnce returns an error if the database or the file system fails or ctx
// ends.
func (f *ImageFetcher) RunOnce(ctx context.Context) error {
	rows, err := db.New(f.db).ListProductsWithoutImage(ctx, maxImagesPerRun)
	if err != nil {
		return fmt.Errorf("load images: list products: %w", err)
	}
	for _, row := range rows {
		if row.ImageSourceUrl == nil {
			continue
		}
		if err := f.load(ctx, row.ID, *row.ImageSourceUrl); err != nil {
			return err
		}
	}
	return nil
}

// load downloads the image at src for the product id and stores it.
func (f *ImageFetcher) load(ctx context.Context, id, src string) error {
	downloadCtx, cancel := context.WithTimeout(ctx, imageTimeout)
	data, ext, err := f.download(downloadCtx, src)
	cancel()
	switch {
	case errors.Is(err, errImageRejected):
		f.logger.LogAttrs(ctx, slog.LevelInfo, "reject product image",
			slog.String("product_id", id),
			slog.String("reason", err.Error()),
		)
		arg := db.ClearProductImageSourceParams{UpdatedAt: store.FormatTime(time.Now()), ID: id, ImageSourceUrl: &src}
		if err := db.New(f.db).ClearProductImageSource(ctx, arg); err != nil {
			return fmt.Errorf("load image of product %s: remove image URL: %w", id, err)
		}
		return nil
	case err != nil:
		if ctx.Err() != nil {
			return fmt.Errorf("load image of product %s: %w", id, ctx.Err())
		}
		f.logger.LogAttrs(ctx, slog.LevelWarn, "download product image",
			slog.String("product_id", id),
			slog.String("error", err.Error()),
		)
		return nil
	}

	name, err := f.write(id, ext, data)
	if err != nil {
		return fmt.Errorf("load image of product %s: %w", id, err)
	}
	arg := db.SetProductImageFileParams{ImageFile: &name, UpdatedAt: store.FormatTime(time.Now()), ID: id, ImageSourceUrl: &src}
	n, err := db.New(f.db).SetProductImageFile(ctx, arg)
	if err != nil {
		// The file stays: removing it could leave a stored file name without
		// a file. The next run writes it again.
		return fmt.Errorf("load image of product %s: set image file: %w", id, err)
	}
	if n == 0 {
		// The product was deleted or got another image URL during the
		// download.
		if err := os.Remove(filepath.Join(f.dir, name)); err != nil {
			return fmt.Errorf("load image of product %s: remove unused file: %w", id, err)
		}
	}
	return nil
}

// download loads the image at src within ctx. It returns the bytes of the
// image and the extension for its content type. Rejected images return an
// error wrapping errImageRejected. At most 2 MB and one byte of the body are
// read.
func (f *ImageFetcher) download(ctx context.Context, src string) ([]byte, string, error) {
	u, err := url.Parse(src)
	if err != nil || !f.allowedURL(u) {
		return nil, "", fmt.Errorf("%w: URL not allowed", errImageRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		// A redirect that CheckRedirect rejects wraps errImageRejected.
		return nil, "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", fmt.Errorf("%w: status %d", errImageRejected, resp.StatusCode)
	default:
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	ext, ok := imageExtension(contentType)
	if !ok {
		return nil, "", fmt.Errorf("%w: content type %q", errImageRejected, contentType)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", fmt.Errorf("%w: %d bytes", errImageRejected, resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("%w: more than %d bytes", errImageRejected, maxImageBytes)
	}
	return data, ext, nil
}

// imageExtension returns the extension of the stored file for an allowed
// content type of images (architecture.md, 7.3). Parameters such as
// "; charset=binary" are ignored, and so is the case of the media type.
func imageExtension(contentType string) (string, bool) {
	mediaType, _, _ := strings.Cut(contentType, ";")
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg":
		return "jpg", true
	case "image/png":
		return "png", true
	case "image/webp":
		return "webp", true
	}
	return "", false
}

// allowedURL reports whether u is https on an allowed host.
func (f *ImageFetcher) allowedURL(u *url.URL) bool {
	return u.Scheme == "https" && f.allowed[u.Host]
}

// write stores data as <dir>/<id>.<ext> with mode 0o640 and returns the file
// name. The data is written to a temporary file in dir first, which is then
// renamed, so the file never exists partially. dir is created with mode
// 0o750 if needed.
func (f *ImageFetcher) write(id, ext string, data []byte) (name string, err error) {
	if err := os.MkdirAll(f.dir, 0o750); err != nil {
		return "", fmt.Errorf("create image dir: %w", err)
	}
	tmp, err := os.CreateTemp(f.dir, "."+id+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary image file: %w", err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(0o640); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	name = id + "." + ext
	if err := os.Rename(tmp.Name(), filepath.Join(f.dir, name)); err != nil {
		return "", fmt.Errorf("rename image file: %w", err)
	}
	return name, nil
}
