package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// OpenAndMigrate opens the database file at path with store.Open and applies
// the migrations from fsys with store.Migrate. If the file existed before and
// migrations are pending, it first writes a copy with backup.PreMigration
// into backupDir, named after now (architecture.md, 9.3). The function lives
// here so that store does not depend on backup. On error the database is
// closed again; otherwise the caller closes it.
func OpenAndMigrate(ctx context.Context, path, backupDir string, migrations fs.FS, now time.Time) (*sql.DB, error) {
	existed := true
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		existed = false
	} else if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	db, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db, backupDir, migrations, existed, now); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate applies the migrations from fsys to db, after a copy with
// backup.PreMigration if the database file existed and migrations are pending.
func migrate(ctx context.Context, db *sql.DB, backupDir string, fsys fs.FS, existed bool, now time.Time) error {
	if existed {
		pending, err := store.HasPending(ctx, db, fsys)
		if err != nil {
			return err
		}
		if pending {
			if _, err := backup.PreMigration(ctx, db, backupDir, now); err != nil {
				return fmt.Errorf("back up before migrations: %w", err)
			}
		}
	}
	return store.Migrate(ctx, db, fsys)
}
