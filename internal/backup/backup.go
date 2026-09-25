// Package backup writes copies of the SQLite database with VACUUM INTO
// (architecture.md, 9.3): a daily backup, a backup before migrations and an
// archive with the images for a download (architecture.md, 6.2).
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// stampLayout formats the UTC time in the names of the backup files.
const stampLayout = "20060102-150405"

// dailyName matches exactly the names of the files that Run writes and
// prunes. Files with other names in the directory stay as they are.
var dailyName = regexp.MustCompile(`^stashbert-[0-9]{8}-[0-9]{6}\.db$`)

// Run writes a copy of db to dir/stashbert-<YYYYMMDD-HHMMSS>.db, named after
// now in UTC, and returns its path. It creates dir with mode 0750 if needed.
// If the file already exists, Run returns an error wrapping fs.ErrExist and
// writes nothing. After the copy it keeps only the keep newest files of this
// name pattern, ordered by the time in their names, and deletes the others;
// all other files in dir stay. If only the deletion fails, Run returns the
// path of the new copy together with the error. keep must be at least 1.
func Run(ctx context.Context, db *sql.DB, dir string, keep int, now time.Time) (string, error) {
	if keep < 1 {
		return "", fmt.Errorf("backup: keep is %d, must be at least 1", keep)
	}
	path, err := vacuumInto(ctx, db, dir, "stashbert-"+now.UTC().Format(stampLayout)+".db")
	if err != nil {
		return "", err
	}
	if err := prune(dir, keep); err != nil {
		return path, err
	}
	return path, nil
}

// PreMigration writes a copy of db to dir/pre-migration-<YYYYMMDD-HHMMSS>.db,
// named after now in UTC, and returns its path. It creates dir with mode 0750
// if needed. If the file already exists, PreMigration returns an error
// wrapping fs.ErrExist and writes nothing. Run never deletes these files.
func PreMigration(ctx context.Context, db *sql.DB, dir string, now time.Time) (string, error) {
	return vacuumInto(ctx, db, dir, "pre-migration-"+now.UTC().Format(stampLayout)+".db")
}

// Start calls Run with time.Now() once right away and then at every tick of
// a ticker with interval until ctx ends. A written backup is logged with
// level info and its path, a failed run with level error; the next tick runs
// again. Start blocks until ctx ends.
func Start(ctx context.Context, db *sql.DB, dir string, keep int, interval time.Duration, logger *slog.Logger) {
	run := func() {
		path, err := Run(ctx, db, dir, keep, time.Now())
		switch {
		case err != nil && ctx.Err() == nil:
			logger.LogAttrs(ctx, slog.LevelError, "write backup", slog.String("error", err.Error()))
		case err == nil:
			logger.LogAttrs(ctx, slog.LevelInfo, "backup written", slog.String("file", path))
		}
		// A run that fails because ctx ended is no error of the job.
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// vacuumInto writes a copy of db to dir/name with VACUUM INTO and returns its
// path. The path is bound as a parameter, never inserted into the SQL text.
func vacuumInto(ctx context.Context, db *sql.DB, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("backup: create directory: %w", err)
	}
	path := filepath.Join(dir, name)
	// VACUUM INTO fails for an existing file that is not empty, but it
	// overwrites an empty one; so the check comes first.
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("backup %s: %w", path, fs.ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("backup %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		// A failed VACUUM INTO can leave an incomplete file behind, which must
		// not count as a backup.
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
		return "", fmt.Errorf("backup %s: %w", path, err)
	}
	return path, nil
}

// Daily returns the times in the names of the regular backups in dir, the
// files stashbert-<YYYYMMDD-HHMMSS>.db that Run writes, oldest first. The
// times are in UTC. Other files, such as pre-migration-*.db, do not count,
// nor does a name with an impossible time. A missing dir means no backups.
func Daily(dir string) ([]time.Time, error) {
	names, err := dailyNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup: list: %w", err)
	}
	var times []time.Time
	for _, name := range names {
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, "stashbert-"), ".db")
		if t, err := time.Parse(stampLayout, stamp); err == nil {
			times = append(times, t)
		}
	}
	return times, nil
}

// dailyNames returns the names of the regular files in dir that match
// dailyName, sorted. The fixed width of the time in the names makes their
// lexical order the chronological one.
func dailyNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && dailyName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// prune deletes all regular files in dir whose names match dailyName except
// the keep newest.
func prune(dir string, keep int) error {
	names, err := dailyNames(dir)
	if err != nil {
		return fmt.Errorf("backup: prune: %w", err)
	}
	var errs []error
	for _, name := range names[:max(len(names)-keep, 0)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("backup: prune: %w", err)
	}
	return nil
}
