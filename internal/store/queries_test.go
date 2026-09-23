package store_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

var createdAt = store.FormatTime(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))

func TestSettingWriteAndRead(t *testing.T) {
	ctx := testContext(t)
	q := db.New(openMigratedDB(t, ctx))

	if _, err := q.GetSetting(ctx, "password_hash"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetSetting before the first write: err = %v, want sql.ErrNoRows", err)
	}
	// The second write replaces the value of the existing key.
	for _, value := range []string{"first", "second"} {
		if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: "password_hash", Value: value}); err != nil {
			t.Fatalf("UpsertSetting(%q): %v", value, err)
		}
		got, err := q.GetSetting(ctx, "password_hash")
		if err != nil {
			t.Fatalf("GetSetting after writing %q: %v", value, err)
		}
		if got != value {
			t.Errorf("GetSetting = %q, want %q", got, value)
		}
	}
}

func TestMembersCreateListGet(t *testing.T) {
	ctx := testContext(t)
	q := db.New(openMigratedDB(t, ctx))

	created := []db.Member{
		{ID: "m1", Name: "Chris", CreatedAt: createdAt},
		{ID: "m2", Name: "anna", CreatedAt: createdAt},
		{ID: "m3", Name: "Bert", CreatedAt: createdAt},
	}
	for _, m := range created {
		if err := q.CreateMember(ctx, db.CreateMemberParams{ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt}); err != nil {
			t.Fatalf("CreateMember(%q): %v", m.Name, err)
		}
	}

	got, err := q.ListMembers(ctx)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	// Case-insensitive order; a binary sort would put "anna" last.
	want := []db.Member{created[1], created[2], created[0]}
	if !slices.Equal(got, want) {
		t.Errorf("ListMembers = %v, want %v", got, want)
	}

	member, err := q.GetMember(ctx, "m1")
	if err != nil {
		t.Fatalf("GetMember(m1): %v", err)
	}
	if member != created[0] {
		t.Errorf("GetMember(m1) = %v, want %v", member, created[0])
	}
	if _, err := q.GetMember(ctx, "unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetMember(unknown): err = %v, want sql.ErrNoRows", err)
	}
}

func TestCreateMemberRejectsNameDifferingOnlyInCase(t *testing.T) {
	ctx := testContext(t)
	q := db.New(openMigratedDB(t, ctx))

	if err := q.CreateMember(ctx, db.CreateMemberParams{ID: "m1", Name: "Chris", CreatedAt: createdAt}); err != nil {
		t.Fatalf("CreateMember(Chris): %v", err)
	}
	err := q.CreateMember(ctx, db.CreateMemberParams{ID: "m2", Name: "chris", CreatedAt: createdAt})
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		t.Fatalf("CreateMember(chris): err = %v, want a UNIQUE constraint error", err)
	}

	members, err := q.ListMembers(ctx)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("ListMembers returned %d members, want 1", len(members))
	}
}
