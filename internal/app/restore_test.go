package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// newHandlerForDB returns the handler from app.NewHandler on db with
// events.Nop, lookup.NewDisabledClient and an empty image directory in
// t.TempDir().
func newHandlerForDB(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	h, err := app.NewHandler(cfg, app.Deps{
		Logger: slog.New(slog.DiscardHandler), Version: "dev", DB: db,
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(), ImageDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// getJSON sends GET path to h, asserts a 200 JSON response and returns the
// decoded body.
func getJSON(t *testing.T, h http.Handler, path string) any {
	t.Helper()
	rec := get(h, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want %d, body %s", path, rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET %s: Content-Type = %q, want application/json", path, ct)
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: decode body %s: %v", path, rec.Body.String(), err)
	}
	return body
}

// movementPages returns the items of all pages of GET /api/v1/movements on h
// with limit items per page and the number of pages. It follows next_cursor
// until it is null.
func movementPages(t *testing.T, h http.Handler, limit int) (items []any, pages int) {
	t.Helper()
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	for pages < 10 {
		body, _ := getJSON(t, h, "/api/v1/movements?"+query.Encode()).(map[string]any)
		page, ok := body["items"].([]any)
		if !ok {
			t.Fatalf("items = %v, want an array", body["items"])
		}
		items = append(items, page...)
		pages++
		next, ok := body["next_cursor"].(string)
		if !ok {
			if body["next_cursor"] != nil {
				t.Fatalf("next_cursor = %v, want a string or null", body["next_cursor"])
			}
			return items, pages
		}
		query.Set("cursor", next)
	}
	t.Fatalf("more than 10 pages of movements")
	return nil, 0
}

// restoreState is what the restore test compares: the bodies of
// GET /products and GET /shopping-list, the items of all pages of
// GET /movements and the stored rows of barcodes and movements, which also
// hold the fields the API does not show (created_at of the barcodes,
// idempotency_key and request_hash of the movements).
type restoreState struct {
	products, shoppingList    any
	movements                 []any
	movementPages             int
	barcodes, storedMovements map[string]storedRow
}

// readRestoreState reads the restoreState from h and its database db.
func readRestoreState(t *testing.T, h http.Handler, db *sql.DB) restoreState {
	t.Helper()
	s := restoreState{
		products:        getJSON(t, h, "/api/v1/products"),
		shoppingList:    getJSON(t, h, "/api/v1/shopping-list"),
		barcodes:        storedBarcodes(t, db),
		storedMovements: storedMovements(t, db),
	}
	s.movements, s.movementPages = movementPages(t, h, 3)
	return s
}

// checkRestoreState asserts that got equals want in each part.
func checkRestoreState(t *testing.T, got, want restoreState) {
	t.Helper()
	parts := []struct {
		name      string
		got, want any
	}{
		{"products", got.products, want.products},
		{"shopping list", got.shoppingList, want.shoppingList},
		{"movements", got.movements, want.movements},
		{"movement pages", got.movementPages, want.movementPages},
		{"stored barcodes", got.barcodes, want.barcodes},
		{"stored movements", got.storedMovements, want.storedMovements},
	}
	for _, p := range parts {
		if !reflect.DeepEqual(p.got, p.want) {
			t.Errorf("%s = %v\nwant %v", p.name, p.got, p.want)
		}
	}
}

func TestRestoreBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	h, db := newApp(t)

	// Data through the API: Kidneybohnen with two barcodes (one of 6 units),
	// target and min_stock; Zucker with one barcode; a placeholder from an
	// unknown barcode.
	beans := decodeCreated(t, post(h, "/api/v1/products", `{"name": "Kidneybohnen", "brand": "Bonduelle",
		"package_size": "400 g", "target": 5, "min_stock": 2, "note": "Im Keller",
		"barcodes": [{"code": "4001686301265"}, {"code": "0034000470693", "units": 6}]}`))
	beansID, _ := beans["id"].(string)
	decodeCreated(t, post(h, "/api/v1/products", `{"name": "Zucker", "barcodes": [{"code": "3017620422003"}]}`))
	bookMovement(t, h, `{"barcode": "0034000470693", "kind": "add"}`)
	bookMovement(t, h, `{"product_id": "`+beansID+`", "kind": "consume", "quantity": 2}`)
	sugarAdd := bookMovement(t, h, `{"barcode": "3017620422003", "kind": "add", "quantity": 2}`)
	bookMovement(t, h, `{"product_id": "`+beansID+`", "kind": "inventory", "stock": 3}`)
	decodeMovementResult(t, reverse(h, sugarAdd))
	placeholder, _ := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "4006381333931", "kind": "add"}`))
	checkFields(t, placeholder, `{"product_created": true, "warnings": ["placeholder_created"]}`)
	const keyedBody = `{"barcode": "4001686301265", "kind": "consume", "quantity": 2}`
	keyed, keyedMovement := decodeMovementResult(t, postWithKey(h, "/api/v1/movements", keyedBody, idempotencyKey))
	checkFields(t, keyed, `{"message": "Kidneybohnen 3 → 1"}`)

	original := readRestoreState(t, h, db)
	// The comparison below only means something if the data is there.
	products, _ := original.products.(map[string]any)
	if items, _ := products["items"].([]any); len(items) != 3 {
		t.Fatalf("products = %d, want 3", len(items))
	}
	if n := len(original.barcodes); n != 4 {
		t.Fatalf("barcodes = %d, want 4", n)
	}
	if n, pages := len(original.movements), original.movementPages; n != 7 || pages != 3 {
		t.Fatalf("movements = %d on %d pages, want 7 on 3", n, pages)
	}
	if n := len(original.storedMovements); n != 7 {
		t.Fatalf("stored movements = %d, want 7", n)
	}
	checkShoppingList(t, h, `{"items": [{"product_id": "`+beansID+`", "name": "Kidneybohnen", "brand": "Bonduelle",
		"missing": 4, "stock": 1, "target": 5, "marked": false}]}`)

	backupDir := filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC)
	backupPath, err := backup.Run(ctx, db, backupDir, 14, now)
	if err != nil {
		t.Fatalf("backup.Run: %v", err)
	}
	if want := filepath.Join(backupDir, "stashbert-20260924-030000.db"); backupPath != want {
		t.Fatalf("backup = %q, want %q", backupPath, want)
	}

	// Restore as in architecture.md, 9.3: only the backup file, named
	// stashbert.db, in a directory without WAL or SHM files.
	dataDir := t.TempDir()
	restoredPath := filepath.Join(dataDir, "stashbert.db")
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(restoredPath, data, 0o600); err != nil {
		t.Fatalf("write restored database: %v", err)
	}
	preMigrationDir := filepath.Join(dataDir, "backups")
	restoredDB, err := app.OpenAndMigrate(ctx, restoredPath, preMigrationDir, store.Migrations, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	t.Cleanup(func() { restoredDB.Close() })
	// All migrations are in the backup, so none is pending.
	if names := backupNames(t, preMigrationDir); len(names) != 0 {
		t.Errorf("pre-migration backups = %v, want none", names)
	}
	restored := newHandlerForDB(t, restoredDB)

	checkRestoreState(t, readRestoreState(t, restored, restoredDB), original)

	// The idempotency key is restored too: the same request gives the stored
	// movement and books nothing.
	again, againMovement := decodeMovementResult(t, postWithKey(restored, "/api/v1/movements", keyedBody, idempotencyKey))
	if !reflect.DeepEqual(againMovement, keyedMovement) {
		t.Errorf("movement = %v\n    want %v", againMovement, keyedMovement)
	}
	checkFields(t, again, `{"product_created": false, "warnings": [], "message": "Kidneybohnen 3 → 1"}`)
	if product := again["product"]; !reflect.DeepEqual(product, getJSON(t, restored, "/api/v1/products/"+beansID)) {
		t.Errorf("product = %v, want the restored product", product)
	}
	checkRestoreState(t, readRestoreState(t, restored, restoredDB), original)
}
