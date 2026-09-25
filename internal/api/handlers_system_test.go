package api_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// newBackupServer returns an api.Server on an empty migrated database in
// t.TempDir() with imageDir as the image directory. os.TempDir points to
// t.TempDir().
func newBackupServer(t *testing.T, ctx context.Context, imageDir string) *api.Server {
	t.Helper()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "stashbert.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// The archive removes a setting from its copy (ADR-0021).
	if err := store.Migrate(ctx, db, store.Migrations); err != nil {
		t.Fatalf("store.Migrate: %v", err)
	}
	t.Setenv("TMPDIR", t.TempDir())
	return api.NewServer(api.ServerDeps{DB: db, ImageDir: imageDir, Logger: slog.New(slog.DiscardHandler)})
}

// downloadBody returns the body of a successful DownloadBackup response.
func downloadBody(t *testing.T, resp api.DownloadBackupResponseObject, err error) io.ReadCloser {
	t.Helper()
	if err != nil {
		t.Fatalf("DownloadBackup: %v", err)
	}
	ok, isOK := resp.(api.DownloadBackup200ApplicationgzipResponse)
	if !isOK {
		t.Fatalf("DownloadBackup = %T, want the archive", resp)
	}
	body, isCloser := ok.Body.(io.ReadCloser)
	if !isCloser {
		t.Fatalf("body %T cannot be closed", ok.Body)
	}
	return body
}

func TestDownloadBackupRunsOneAfterAnother(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	s := newBackupServer(t, ctx, t.TempDir())

	resp, err := s.DownloadBackup(ctx, api.DownloadBackupRequestObject{})
	first := downloadBody(t, resp, err)

	// A second download waits until the first body is closed.
	waitCtx, cancelWait := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelWait()
	if _, err := s.DownloadBackup(waitCtx, api.DownloadBackupRequestObject{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second DownloadBackup while the first runs: %v, want it to wait until its context ends", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	resp, err = s.DownloadBackup(ctx, api.DownloadBackupRequestObject{})
	second := downloadBody(t, resp, err)
	if err := second.Close(); err != nil {
		t.Fatalf("close second: %v", err)
	}
}

func TestDownloadBackupFailureEndsDownload(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// A file instead of the image directory makes every download fail.
	imageDir := filepath.Join(t.TempDir(), "images")
	if err := os.WriteFile(imageDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := newBackupServer(t, ctx, imageDir)

	for i := range 2 {
		if _, err := s.DownloadBackup(ctx, api.DownloadBackupRequestObject{}); err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("DownloadBackup %d: %v, want the error of the image directory", i+1, err)
		}
	}
}
