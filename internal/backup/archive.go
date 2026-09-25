package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Archive is a fresh backup for a download (architecture.md, 6.2): a tar.gz
// in its own temporary directory with a copy of the database as stashbert.db
// and the image files under images/. Reading starts at the beginning of the
// archive. Close removes the directory with the copy and the archive.
type Archive struct {
	*os.File
	// Size is the length of the archive in bytes.
	Size int64
	dir  string
}

// ArchiveName returns the file name of a download archive made at now:
// stashbert-<YYYYMMDD-HHMMSS>.tar.gz with the time in UTC.
func ArchiveName(now time.Time) string {
	return "stashbert-" + now.UTC().Format(stampLayout) + ".tar.gz"
}

// NewArchive writes a copy of db with VACUUM INTO into a new directory in
// os.TempDir and packs it as stashbert.db, together with the regular files
// of imageDir under images/, into a tar.gz in the same directory. A missing
// imageDir gives an empty images/. An image file that disappears while the
// archive is written is left out. On error the directory is removed again;
// otherwise the caller must call Close.
func NewArchive(ctx context.Context, db *sql.DB, imageDir string) (_ *Archive, err error) {
	dir, err := os.MkdirTemp("", "stashbert-download-*")
	if err != nil {
		return nil, fmt.Errorf("backup archive: create temporary directory: %w", err)
	}
	defer func() {
		if err != nil {
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("backup archive: remove temporary directory: %w", rmErr))
			}
		}
	}()

	dbPath, err := vacuumInto(ctx, db, dir, "stashbert.db")
	if err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "stashbert.tar.gz"))
	if err != nil {
		return nil, fmt.Errorf("backup archive: %w", err)
	}
	if err := writeArchive(ctx, f, dbPath, imageDir); err != nil {
		f.Close()
		return nil, err
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("backup archive: %w", err)
	}
	return &Archive{File: f, Size: size, dir: dir}, nil
}

// Close closes the archive and removes its temporary directory.
func (a *Archive) Close() error {
	err := a.File.Close()
	if rmErr := os.RemoveAll(a.dir); rmErr != nil {
		err = errors.Join(err, rmErr)
	}
	if err != nil {
		return fmt.Errorf("backup archive: close: %w", err)
	}
	return nil
}

// writeArchive writes the tar.gz with the file at dbPath as stashbert.db and
// the directory images/ with the regular files of imageDir to w.
func writeArchive(ctx context.Context, w io.Writer, dbPath, imageDir string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	dbTime, err := addFile(tw, dbPath, "stashbert.db")
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "images/", Mode: 0o750, ModTime: dbTime}); err != nil {
		return fmt.Errorf("backup archive: %w", err)
	}
	entries, err := os.ReadDir(imageDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup archive: %w", err)
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		// A client that went away does not need the rest of the archive.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("backup archive: %w", err)
		}
		if _, err := addFile(tw, filepath.Join(imageDir, e.Name()), "images/"+e.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("backup archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("backup archive: %w", err)
	}
	return nil
}

// addFile writes the file at path to tw as a regular file called name with
// mode 0o640 and returns its modification time. The size comes from the
// opened file, so a file removed after opening is still written completely.
func addFile(tw *tar.Writer, path, name string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("backup archive: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return time.Time{}, fmt.Errorf("backup archive: %w", err)
	}
	hdr := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o640, Size: info.Size(), ModTime: info.ModTime()}
	if err := tw.WriteHeader(hdr); err != nil {
		return time.Time{}, fmt.Errorf("backup archive: %s: %w", name, err)
	}
	if _, err := io.CopyN(tw, f, info.Size()); err != nil {
		return time.Time{}, fmt.Errorf("backup archive: %s: %w", name, err)
	}
	return info.ModTime(), nil
}
