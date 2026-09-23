package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations holds the embedded migration files at its root, where goose looks for them.
var Migrations fs.FS = func() fs.FS {
	// fs.Sub only fails for an invalid directory name, and "migrations" is valid.
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}()

// Migrate applies all pending migrations from fsys to db. Without pending
// migrations it changes nothing. db stays open.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	// provider.Close would close db, which belongs to the caller.
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
