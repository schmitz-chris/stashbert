package backup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// useTempDir points os.TempDir to a new empty directory and returns it.
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	return dir
}

// archiveEntry is an entry of an unpacked archive.
type archiveEntry struct {
	typeflag byte
	mode     int64
	data     []byte
}

// unpack reads the tar.gz from r and returns its entries by name in their
// order.
func unpack(t *testing.T, r io.Reader) (names []string, entries map[string]archiveEntry) {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	entries = map[string]archiveEntry{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		names = append(names, hdr.Name)
		entries[hdr.Name] = archiveEntry{typeflag: hdr.Typeflag, mode: hdr.Mode, data: data}
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	return names, entries
}

func TestDaily(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"stashbert-20260925-101500.db",
		"stashbert-20260101-020304.db",
		"pre-migration-20260926-000000.db",
		"stashbert-20261399-000000.db", // impossible time
		"stashbert-20260925-101500.db.tmp",
		"notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "stashbert-20260927-000000.db"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := backup.Daily(dir)

	want := []time.Time{
		time.Date(2026, 1, 1, 2, 3, 4, 0, time.UTC),
		time.Date(2026, 9, 25, 10, 15, 0, 0, time.UTC),
	}
	if err != nil || !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("Daily = %v, %v, want %v", got, err, want)
	}
	for _, tm := range got {
		if tm.Location() != time.UTC {
			t.Errorf("Daily: location of %v is %v, want UTC", tm, tm.Location())
		}
	}
}

func TestDailyWithoutDirectory(t *testing.T) {
	got, err := backup.Daily(filepath.Join(t.TempDir(), "backups"))

	if err != nil || len(got) != 0 {
		t.Errorf("Daily = %v, %v, want no backups", got, err)
	}
}

func TestArchiveName(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 30, 45, 0, time.FixedZone("CEST", 2*60*60))

	if got, want := backup.ArchiveName(now), "stashbert-20260925-103045.tar.gz"; got != want {
		t.Errorf("ArchiveName = %q, want %q", got, want)
	}
}

func TestNewArchive(t *testing.T) {
	ctx := testContext(t)
	db, _ := openSource(t, ctx)
	imageDir := t.TempDir()
	images := map[string][]byte{"a.jpg": []byte("jpeg data"), "b.png": bytes.Repeat([]byte("png "), 10000)}
	for name, data := range images {
		if err := os.WriteFile(filepath.Join(imageDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Only files go into the archive, no subdirectories and no hidden
	// temporary files of an image being written.
	if err := os.Mkdir(filepath.Join(imageDir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, ".a.jpg-123.tmp"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := useTempDir(t)

	a, err := backup.NewArchive(ctx, db, imageDir)
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	if names := dirNames(t, tmp); len(names) != 1 {
		t.Errorf("temporary directory has %v, want one directory for the archive", names)
	}
	data, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if names := dirNames(t, tmp); len(names) != 0 {
		t.Errorf("after Close the temporary directory has %v, want nothing", names)
	}
	if int64(len(data)) != a.Size {
		t.Errorf("Size = %d, want %d", a.Size, len(data))
	}
	names, entries := unpack(t, bytes.NewReader(data))
	if want := []string{"stashbert.db", "images/", "images/a.jpg", "images/b.png"}; !slices.Equal(names, want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	if e := entries["images/"]; e.typeflag != tar.TypeDir || e.mode != 0o750 {
		t.Errorf("images/: type %c, mode %o, want directory with mode 750", e.typeflag, e.mode)
	}
	for name, want := range images {
		if e := entries["images/"+name]; e.typeflag != tar.TypeReg || e.mode != 0o640 || !bytes.Equal(e.data, want) {
			t.Errorf("images/%s: type %c, mode %o, %d bytes, want the file with mode 640", name, e.typeflag, e.mode, len(e.data))
		}
	}
	e := entries["stashbert.db"]
	if e.typeflag != tar.TypeReg || e.mode != 0o640 {
		t.Errorf("stashbert.db: type %c, mode %o, want file with mode 640", e.typeflag, e.mode)
	}
	path := filepath.Join(t.TempDir(), "stashbert.db")
	if err := os.WriteFile(path, e.data, 0o600); err != nil {
		t.Fatal(err)
	}
	copyDB, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer copyDB.Close()
	if na, nb := count(t, ctx, copyDB, "a"), count(t, ctx, copyDB, "b"); na != 3 || nb != 5 {
		t.Errorf("copy has %d rows in a and %d in b, want 3 and 5", na, nb)
	}
}

func TestNewArchiveWithoutImageDirectory(t *testing.T) {
	ctx := testContext(t)
	db, _ := openSource(t, ctx)
	useTempDir(t)

	a, err := backup.NewArchive(ctx, db, filepath.Join(t.TempDir(), "images"))
	if err != nil {
		t.Fatalf("NewArchive: %v", err)
	}
	defer a.Close()

	if names, _ := unpack(t, a); !slices.Equal(names, []string{"stashbert.db", "images/"}) {
		t.Errorf("entries = %v, want stashbert.db and an empty images/", names)
	}
}

func TestNewArchiveErrorRemovesDirectory(t *testing.T) {
	ctx := testContext(t)
	db, _ := openSource(t, ctx)
	// A file instead of the image directory cannot be read as a directory.
	imageDir := filepath.Join(t.TempDir(), "images")
	if err := os.WriteFile(imageDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := useTempDir(t)

	if a, err := backup.NewArchive(ctx, db, imageDir); err == nil {
		a.Close()
		t.Fatal("NewArchive: no error, want one for the image directory")
	}

	if names := dirNames(t, tmp); len(names) != 0 {
		t.Errorf("temporary directory has %v, want nothing", names)
	}
}
