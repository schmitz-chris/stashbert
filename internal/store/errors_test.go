package store_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
)

func TestIsUniqueViolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE u (pk TEXT PRIMARY KEY, uq TEXT UNIQUE, n INTEGER NOT NULL) STRICT`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO u (pk, uq, n) VALUES ('a', 'x', 1)`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	for name, stmt := range map[string]string{
		"primary key": `INSERT INTO u (pk, uq, n) VALUES ('a', 'y', 1)`,
		"unique":      `INSERT INTO u (pk, uq, n) VALUES ('b', 'x', 1)`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		if !store.IsUniqueViolation(err) {
			t.Errorf("%s: IsUniqueViolation(%v) = false, want true", name, err)
		}
		if !store.IsUniqueViolation(fmt.Errorf("wrapped: %w", err)) {
			t.Errorf("%s: IsUniqueViolation should see through wrapping", name)
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO u (pk, uq, n) VALUES ('c', 'z', NULL)`)
	if err == nil || store.IsUniqueViolation(err) {
		t.Fatalf("NOT NULL violation must not count as unique violation, got %v", err)
	}
	if store.IsUniqueViolation(errors.New("other")) || store.IsUniqueViolation(nil) {
		t.Fatal("IsUniqueViolation must be false for other errors and nil")
	}
}
