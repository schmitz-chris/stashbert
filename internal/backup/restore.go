package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// MaxUploadBytes is the largest backup archive that POST /backup/restore
// takes, 1 GB (ADR-0020).
const MaxUploadBytes int64 = 1 << 30

// MaxUnpackedBytes is the most an uploaded backup archive may unpack to,
// 4 GB (ADR-0020).
const MaxUnpackedBytes int64 = 4 << 30

// Names in DATA_DIR for restoring a backup (architecture.md, 9.3).
const (
	restoreDirName = "restore"
	pendingName    = "pending"
	incomingPrefix = "incoming-"
	previousPrefix = "vor-restore-"
	dbName         = "stashbert.db"
	imagesName     = "images"
)

// dataNames are the names in DATA_DIR that a restore replaces: the database
// with its WAL and SHM files and the image directory.
var dataNames = []string{dbName, dbName + "-wal", dbName + "-shm", imagesName}

// imageName matches the names of the stored product images: <id>.<ext> from
// the image fetcher and <id>-upload-<unix-millis>.<ext> from an upload, with
// the product ID as a UUID in lower case (architecture.md, 7.3).
var imageName = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(-upload-[0-9]+)?\.(jpg|png|webp)$`)

// TooLarge returns the 413 backup_too_large error for an upload larger than
// MaxUploadBytes.
func TooLarge() *httpx.Error {
	return httpx.NewError(http.StatusRequestEntityTooLarge, "backup_too_large", "Das Backup ist größer als 1 GB")
}

// InProgress returns the 409 restore_in_progress error.
func InProgress() *httpx.Error {
	return httpx.NewError(http.StatusConflict, "restore_in_progress", "Es wird schon ein Backup eingespielt")
}

// invalidBackup returns the 422 invalid_backup error with detail.
func invalidBackup(detail string) *httpx.Error {
	return httpx.NewError(http.StatusUnprocessableEntity, "invalid_backup", detail)
}

// Stage unpacks the backup archive from r, a tar.gz from GET /backup, into
// dataDir/restore/incoming-<random>/, checks it and renames it to
// dataDir/restore/pending/, where ApplyPending finds it (ADR-0020). Nothing
// else in dataDir changes. The archive is unpacked while it is read, never
// held in memory.
//
// The archive may hold only the regular file stashbert.db, the directory
// images/ and regular files in it named like the stored product images.
// Anything else, a missing stashbert.db, data that is no tar.gz, and a
// database that fails PRAGMA integrity_check or has no migration of
// StashBert result in 422 invalid_backup. A database with a migration newer
// than the newest in migrations results in 422 backup_too_new, more than
// maxUnpacked unpacked bytes in 413 backup_too_large. If a backup is already
// pending, the result is 409 restore_in_progress. An error of r is returned
// wrapped, so that the caller can recognize an *http.MaxBytesError. On every
// error incoming-… is removed again, and so is an empty restore/.
//
// Stage does not guard against concurrent calls; the caller runs one at a
// time.
func Stage(ctx context.Context, dataDir string, r io.Reader, migrations fs.FS, maxUnpacked int64) (err error) {
	restoreDir := filepath.Join(dataDir, restoreDirName)
	pendingDir := filepath.Join(restoreDir, pendingName)
	if err := checkNotPending(pendingDir); err != nil {
		return err
	}
	if err := os.MkdirAll(restoreDir, 0o750); err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	incoming, err := os.MkdirTemp(restoreDir, incomingPrefix+"*")
	if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	defer func() {
		if err != nil {
			if rmErr := os.RemoveAll(incoming); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("stage backup: remove %s: %w", incoming, rmErr))
			}
			// restore/ goes too if it is empty now; otherwise it stays.
			_ = os.Remove(restoreDir)
		}
	}()

	u := &unpacker{dir: incoming, body: &recordingReader{r: r}}
	if err := u.unpack(ctx, maxUnpacked); err != nil {
		return err
	}
	if err := checkDatabase(ctx, filepath.Join(incoming, dbName), migrations); err != nil {
		return err
	}
	if err := checkNotPending(pendingDir); err != nil {
		return err
	}
	if err := os.Rename(incoming, pendingDir); err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	return nil
}

// checkNotPending returns 409 restore_in_progress if pendingDir exists.
func checkNotPending(pendingDir string) error {
	_, err := os.Lstat(pendingDir)
	switch {
	case err == nil:
		return InProgress()
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	return fmt.Errorf("stage backup: %w", err)
}

// recordingReader remembers the first error of r other than io.EOF.
type recordingReader struct {
	r   io.Reader
	err error
}

func (r *recordingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}

// unpacker writes the entries of an uploaded archive into dir.
type unpacker struct {
	dir  string
	body *recordingReader
	// data is the stream after gzip, limited to one byte more than allowed;
	// N is 0 after too many bytes.
	data *io.LimitedReader
}

// unpack reads the tar.gz from u.body, at most maxUnpacked bytes after
// gzip, and writes its entries into u.dir.
func (u *unpacker) unpack(ctx context.Context, maxUnpacked int64) error {
	gz, err := gzip.NewReader(u.body)
	if err != nil {
		return u.readError(err, "Das Backup ist kein tar.gz")
	}
	u.data = &io.LimitedReader{R: gz, N: maxUnpacked + 1}
	tr := tar.NewReader(u.data)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stage backup: %w", err)
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return u.readError(err, "Das Backup ist kein gültiges tar.gz")
		}
		if err := u.unpackEntry(hdr, tr); err != nil {
			return err
		}
	}
	// The rest of the stream must be valid too, up to the checksum at the
	// end of the gzip data.
	if _, err := io.Copy(io.Discard, u.data); err != nil || u.data.N == 0 {
		return u.readError(err, "Das Backup ist kein gültiges tar.gz")
	}
	return nil
}

// readError returns the error for err, which occurred while reading the
// archive: the wrapped error of the body, 413 backup_too_large after too
// many unpacked bytes, otherwise 422 invalid_backup with detail.
func (u *unpacker) readError(err error, detail string) error {
	switch {
	case u.body.err != nil:
		return fmt.Errorf("stage backup: read upload: %w", u.body.err)
	case u.data != nil && u.data.N == 0:
		return httpx.NewError(http.StatusRequestEntityTooLarge, "backup_too_large", "Das Backup ist entpackt größer als 4 GB")
	}
	return invalidBackup(detail + ": " + err.Error())
}

// unpackEntry writes the entry hdr with the data from tr into u.dir if it is
// allowed.
func (u *unpacker) unpackEntry(hdr *tar.Header, tr io.Reader) error {
	image, isImage := strings.CutPrefix(hdr.Name, imagesName+"/")
	switch {
	case hdr.Name == dbName && hdr.Typeflag == tar.TypeReg:
		return u.writeFile(dbName, tr)
	case (hdr.Name == imagesName+"/" || hdr.Name == imagesName) && hdr.Typeflag == tar.TypeDir:
		return u.makeImageDir()
	case isImage && hdr.Typeflag == tar.TypeReg && imageName.MatchString(image):
		if err := u.makeImageDir(); err != nil {
			return err
		}
		return u.writeFile(filepath.Join(imagesName, image), tr)
	}
	return invalidBackup(fmt.Sprintf("Das Backup enthält %q; erlaubt sind nur stashbert.db und Produktbilder unter images/", hdr.Name))
}

// makeImageDir creates the directory images in u.dir if it does not exist.
func (u *unpacker) makeImageDir() error {
	if err := os.Mkdir(filepath.Join(u.dir, imagesName), 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("stage backup: %w", err)
	}
	return nil
}

// writeFile writes the data from tr as the new file name in u.dir. A name
// that already exists means a duplicate entry, 422 invalid_backup.
func (u *unpacker) writeFile(name string, tr io.Reader) (err error) {
	f, err := os.OpenFile(filepath.Join(u.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errors.Is(err, fs.ErrExist) {
		return invalidBackup(fmt.Sprintf("Das Backup enthält %s mehrfach", filepath.ToSlash(name)))
	}
	if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("stage backup: %w", cerr)
		}
	}()
	if _, err := io.Copy(f, tr); err != nil {
		// Only writing the file fails with an *fs.PathError.
		if pathErr := (*fs.PathError)(nil); errors.As(err, &pathErr) {
			return fmt.Errorf("stage backup: %w", err)
		}
		return u.readError(err, "Das Backup ist kein gültiges tar.gz")
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	return nil
}

// checkDatabase checks the unpacked database at path: it must exist, pass
// PRAGMA integrity_check, have a migration of StashBert and none newer than
// the newest in migrations. Opening it switches it to WAL; closing it again
// removes its WAL and SHM files.
func checkDatabase(ctx context.Context, path string, migrations fs.FS) (err error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return invalidBackup("Im Backup fehlt stashbert.db")
	} else if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("stage backup: %w", ctx.Err())
		}
		// The error names the path in DATA_DIR, which the client does not
		// need to know.
		return invalidBackup("stashbert.db ist keine SQLite-Datenbank")
	}
	defer func() {
		if cerr := db.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("stage backup: close database: %w", cerr)
		}
	}()

	// Without problems, integrity_check returns the single row "ok",
	// otherwise the first row is a problem.
	var result string
	err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
	if err == nil && result != "ok" {
		err = errors.New(result)
	}
	if err != nil {
		return invalidBackup("stashbert.db ist beschädigt: " + err.Error())
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrations)
	if err != nil {
		return fmt.Errorf("stage backup: create migration provider: %w", err)
	}
	// provider.Close would close db, which is closed above.
	current, newest, err := provider.GetVersions(ctx)
	if err != nil {
		return invalidBackup("stashbert.db hat keine lesbare Migrationsversion: " + err.Error())
	}
	if current > newest {
		return httpx.NewError(http.StatusUnprocessableEntity, "backup_too_new",
			fmt.Sprintf("Das Backup stammt von einer neueren Version von StashBert (Migration %d, bekannt bis %d)", current, newest))
	}
	if current < 1 {
		return invalidBackup("stashbert.db ist keine Datenbank von StashBert")
	}
	return nil
}

// ApplyPending applies a backup that Stage laid ready in
// dataDir/restore/pending/ (ADR-0020). It must run before the database is
// opened. It moves what exists of stashbert.db, its WAL and SHM files and
// images/ from dataDir to dataDir/vor-restore-<YYYYMMDD-HHMMSS>/, named after
// now in UTC, and moves the database and the images from pending/ into
// dataDir. Then it removes dataDir/restore/, including the rest of an
// aborted upload (incoming-…). Without pending/ it only removes such a rest.
// Every step is logged. On error ApplyPending stops at once; the caller must
// not start then, rather than run with half a state.
func ApplyPending(dataDir string, now time.Time, logger *slog.Logger) error {
	restoreDir := filepath.Join(dataDir, restoreDirName)
	pendingDir := filepath.Join(restoreDir, pendingName)
	if _, err := os.Lstat(pendingDir); errors.Is(err, fs.ErrNotExist) {
		return removeRestoreDir(restoreDir, logger)
	} else if err != nil {
		return fmt.Errorf("apply backup: %w", err)
	}
	logger.Info("apply backup", slog.String("dir", pendingDir))
	if _, err := os.Lstat(filepath.Join(pendingDir, dbName)); err != nil {
		return fmt.Errorf("apply backup: %w", err)
	}
	previousDir := filepath.Join(dataDir, previousPrefix+now.UTC().Format(stampLayout))
	moved, err := moveData(dataDir, previousDir, logger)
	if err != nil {
		return err
	}
	if _, err := moveData(pendingDir, dataDir, logger); err != nil {
		return err
	}
	if err := removeRestoreDir(restoreDir, logger); err != nil {
		return err
	}
	if moved == 0 {
		previousDir = ""
	}
	logger.Info("backup applied", slog.String("previous", previousDir))
	return nil
}

// moveData moves what exists of the database files and images/ from dir to
// target, which is created before the first move if needed, and returns the
// number of moved entries.
func moveData(dir, target string, logger *slog.Logger) (moved int, err error) {
	for _, name := range dataNames {
		from, to := filepath.Join(dir, name), filepath.Join(target, name)
		if _, err := os.Lstat(from); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return moved, fmt.Errorf("apply backup: %w", err)
		}
		if err := os.Mkdir(target, 0o750); err != nil && !errors.Is(err, fs.ErrExist) {
			return moved, fmt.Errorf("apply backup: %w", err)
		}
		if err := os.Rename(from, to); err != nil {
			return moved, fmt.Errorf("apply backup: %w", err)
		}
		logger.Info("apply backup: moved", slog.String("from", from), slog.String("to", to))
		moved++
	}
	return moved, nil
}

// removeRestoreDir removes restoreDir with everything in it, if it exists.
func removeRestoreDir(restoreDir string, logger *slog.Logger) error {
	if _, err := os.Lstat(restoreDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.RemoveAll(restoreDir); err != nil {
		return fmt.Errorf("apply backup: remove %s: %w", restoreDir, err)
	}
	logger.Info("apply backup: removed", slog.String("dir", restoreDir))
	return nil
}
