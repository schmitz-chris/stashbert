// Package store opens the SQLite database and migrates its schema (ADR-0005).
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // registers the database/sql driver "sqlite"
)

// dsnParams is appended to the file path. The driver applies the pragmas to
// every new connection and starts transactions with BEGIN IMMEDIATE.
const dsnParams = "?_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_txlock=immediate"

// Open opens the SQLite database file at path and creates it if it does not
// exist. The caller closes the returned database.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	// The driver splits the DSN at the first '?', so such a path would open a different file.
	if strings.Contains(path, "?") {
		return nil, fmt.Errorf("open database %q: path must not contain '?'", path)
	}
	db, err := sql.Open("sqlite", path+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	db.SetMaxOpenConns(4)
	// sql.Open does not connect; the first connection creates the file and applies the pragmas.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	return db, nil
}
