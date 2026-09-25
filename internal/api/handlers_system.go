package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// GetSystemStatus reports the state for the settings page (architecture.md,
// 6.2). last_backup_at is the time in the name of the newest regular backup,
// not the modification time of the file, which a copy would change.
func (s *Server) GetSystemStatus(ctx context.Context, request GetSystemStatusRequestObject) (GetSystemStatusResponseObject, error) {
	size, err := databaseSize(s.deps.DBPath)
	if err != nil {
		return nil, err
	}
	backups, err := backup.Daily(s.deps.BackupDir)
	if err != nil {
		return nil, err
	}
	status := SystemStatus{
		Version:       s.deps.Version,
		DatabaseSize:  size,
		BackupCount:   len(backups),
		LastBackupAt:  nullable.NewNullNullable[time.Time](),
		BackupKeep:    s.deps.BackupKeep,
		OpenFoodFacts: s.deps.OpenFoodFacts,
	}
	if n := len(backups); n > 0 {
		status.LastBackupAt = nullable.NewNullableWithValue(backups[n-1])
	}
	return GetSystemStatus200JSONResponse(status), nil
}

// databaseSize returns the size of the database file at path plus the size
// of its WAL file, if there is one.
func databaseSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("database size: %w", err)
	}
	size := info.Size()
	wal, err := os.Stat(path + "-wal")
	switch {
	case err == nil:
		size += wal.Size()
	case !errors.Is(err, fs.ErrNotExist):
		return 0, fmt.Errorf("database size: %w", err)
	}
	return size, nil
}

// DownloadBackup returns a fresh backup as a tar.gz (architecture.md, 6.2).
// Downloads run one after another: a request waits until the running
// download is sent or aborted, or until its client gives up. The archive is
// complete before the response starts, so a failure becomes a problem, and
// the response has a Content-Length, so a client notices a transfer that
// breaks off. The generated response closes the body after sending it, also
// when sending fails; that removes the temporary copy and lets the next
// download run.
func (s *Server) DownloadBackup(ctx context.Context, request DownloadBackupRequestObject) (DownloadBackupResponseObject, error) {
	select {
	case s.downloads <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for the running backup download: %w", ctx.Err())
	}
	now := time.Now()
	archive, err := backup.NewArchive(ctx, s.deps.DB, s.deps.ImageDir)
	if err != nil {
		<-s.downloads
		return nil, err
	}
	return DownloadBackup200ApplicationgzipResponse{
		Body:          &downloadBody{Archive: archive, release: func() { <-s.downloads }, logger: s.deps.Logger},
		Headers:       DownloadBackup200ResponseHeaders{ContentDisposition: `attachment; filename="` + backup.ArchiveName(now) + `"`},
		ContentLength: archive.Size,
	}, nil
}

// downloadBody is a backup archive as the body of a response.
type downloadBody struct {
	*backup.Archive
	release func()
	logger  *slog.Logger
}

// Close removes the archive with its temporary directory and ends the
// download. A failure is logged, because the response is already sent.
func (b *downloadBody) Close() error {
	defer b.release()
	if err := b.Archive.Close(); err != nil {
		b.logger.LogAttrs(context.Background(), slog.LevelError, "remove backup download", slog.String("error", err.Error()))
		return err
	}
	return nil
}

// RestoreBackup checks the uploaded backup and lays it ready with
// backup.Stage, then asks for the restart that applies it (ADR-0020). While
// another upload is checked, a request gets 409 restore_in_progress at once.
// A body over the limit of an http.MaxBytesReader results in 413
// backup_too_large. The route in internal/app checks the Content-Type and
// limits the body, because the request validator does not see this route
// (architecture.md, 4.4).
func (s *Server) RestoreBackup(ctx context.Context, request RestoreBackupRequestObject) (RestoreBackupResponseObject, error) {
	select {
	case s.restores <- struct{}{}:
		defer func() { <-s.restores }()
	default:
		return nil, backup.InProgress()
	}
	err := backup.Stage(ctx, s.deps.DataDir, request.Body, store.Migrations, backup.MaxUnpackedBytes)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, backup.TooLarge()
	}
	if err != nil {
		return nil, err
	}
	s.deps.Logger.LogAttrs(ctx, slog.LevelInfo, "backup ready to apply, restart requested")
	if s.deps.Restart != nil {
		s.deps.Restart()
	}
	return RestoreBackup202Response{}, nil
}
