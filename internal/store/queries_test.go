package store_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/store/db"
)

func TestSettingWriteAndRead(t *testing.T) {
	ctx := testContext(t)
	q := db.New(openMigratedDB(t, ctx))

	if _, err := q.GetSetting(ctx, "test_key"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetSetting before the first write: err = %v, want sql.ErrNoRows", err)
	}
	// The second write replaces the value of the existing key.
	for _, value := range []string{"first", "second"} {
		if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: "test_key", Value: value}); err != nil {
			t.Fatalf("UpsertSetting(%q): %v", value, err)
		}
		got, err := q.GetSetting(ctx, "test_key")
		if err != nil {
			t.Fatalf("GetSetting after writing %q: %v", value, err)
		}
		if got != value {
			t.Errorf("GetSetting = %q, want %q", got, value)
		}
	}
}
