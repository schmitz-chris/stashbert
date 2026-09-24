package store_test

import (
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// TestMigrateOutbox applies 0005 to a database at version 0004, checks the
// table outbox of architecture.md 11.4, migrates down to 0004 and up again.
func TestMigrateOutbox(t *testing.T) {
	ctx := testContext(t)
	sqlDB := openDB(t, ctx)
	// p is not closed: Close would close sqlDB, which openDB closes after the test.
	p, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, store.Migrations)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatalf("migrate to 0004: %v", err)
	}
	mustInsert(t, ctx, sqlDB, "products", "p", row{"stock": 2, "target": 5, "crate_size": 20})
	schemaBefore, productsBefore := schemaState(t, ctx, sqlDB), tableRows(t, ctx, sqlDB, "products")

	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("migrate to 0005: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 5 {
		t.Fatalf("version = %d, want 5", v)
	}
	cols := queryStrings(t, ctx, sqlDB, `SELECT name || ' ' || type || iif("notnull", ' NOT NULL', '') ||
		coalesce(' DEFAULT ' || dflt_value, '') || iif(pk, ' PK', '') FROM pragma_table_info('outbox') ORDER BY cid`)
	want := []string{"seq INTEGER PK", "topic TEXT NOT NULL", "payload TEXT NOT NULL", "created_at TEXT NOT NULL",
		"attempts INTEGER NOT NULL DEFAULT 0", "last_error TEXT"}
	if !slices.Equal(cols, want) {
		t.Errorf("columns of outbox:\n got %q\nwant %q", cols, want)
	}

	// seq counts up and is not used again after the newest entry is deleted
	// (AUTOINCREMENT), so it keeps the order.
	q := db.New(sqlDB)
	for _, topic := range []string{"events/a", "events/b", "events/c"} {
		if err := q.InsertOutboxEntry(ctx, db.InsertOutboxEntryParams{Topic: topic, Payload: "{}", CreatedAt: testTime}); err != nil {
			t.Fatalf("insert %s: %v", topic, err)
		}
		if topic == "events/b" {
			if err := q.DeleteOutboxEntry(ctx, 2); err != nil {
				t.Fatalf("delete entry 2: %v", err)
			}
		}
	}
	entries, err := q.ListOutboxEntries(ctx, 50)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	var seqs []int64
	for _, e := range entries {
		seqs = append(seqs, e.Seq)
		if e.Attempts != 0 || e.LastError != nil {
			t.Errorf("entry %d: attempts %d, last_error %v, want 0 and NULL", e.Seq, e.Attempts, e.LastError)
		}
	}
	if !slices.Equal(seqs, []int64{1, 3}) {
		t.Errorf("seq = %v, want [1 3]", seqs)
	}
	if err := insertRow(ctx, sqlDB, "outbox", row{"topic": "events/a", "payload": "{}", "created_at": testTime, "attempts": -1}); err == nil {
		t.Error("insert with attempts -1: got nil error, want a CHECK violation")
	}

	if _, err := p.Down(ctx); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if v := dbVersion(t, ctx, p); v != 4 {
		t.Errorf("version after down = %d, want 4", v)
	}
	if after := schemaState(t, ctx, sqlDB); after != schemaBefore {
		t.Errorf("schema after down:\n%s\nwant as before 0005:\n%s", after, schemaBefore)
	}
	if after := tableRows(t, ctx, sqlDB, "products"); after != productsBefore {
		t.Errorf("products after down:\n%s\nwant as before 0005:\n%s", after, productsBefore)
	}

	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatalf("migrate to 0005 after down: %v", err)
	}
	if n := countRows(t, ctx, sqlDB, "outbox"); n != 0 {
		t.Errorf("outbox has %d rows after migrating up again, want 0", n)
	}
}
