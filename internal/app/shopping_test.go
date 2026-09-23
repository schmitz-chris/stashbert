package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// shoppingProduct is a test product for the shopping list. brand and minStock
// are nil for NULL.
type shoppingProduct struct {
	id, name      string
	brand         any
	stock, target int
	minStock      any
}

// insertShoppingProducts inserts ps in their order with direct SQL.
func insertShoppingProducts(t *testing.T, db *sql.DB, ps ...shoppingProduct) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, p := range ps {
		if _, err := db.ExecContext(ctx, `INSERT INTO products (id, name, brand, stock, target, min_stock,
			origin, lookup_state, needs_review, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'manual', 'none', 0, '2026-09-23T12:00:00.000Z', '2026-09-23T12:00:00.000Z')`,
			p.id, p.name, p.brand, p.stock, p.target, p.minStock); err != nil {
			t.Fatalf("insert product %s: %v", p.id, err)
		}
	}
}

// getShoppingList sends GET /api/v1/shopping-list to h, asserts a 200 JSON
// response and returns its body.
func getShoppingList(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := get(h, "/api/v1/shopping-list")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return rec.Body.String()
}

// checkShoppingList asserts that the shopping list of h is the JSON want.
func checkShoppingList(t *testing.T, h http.Handler, want string) {
	t.Helper()
	body := getShoppingList(t, h)
	var got, w any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	if !reflect.DeepEqual(got, w) {
		t.Errorf("body = %s\nwant %s", body, want)
	}
}

func TestGetShoppingListEmpty(t *testing.T) {
	h, db := newApp(t)

	t.Run("without products", func(t *testing.T) {
		if got, want := strings.TrimSpace(getShoppingList(t, h)), `{"items":[]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("nothing missing", func(t *testing.T) {
		insertShoppingProducts(t, db,
			shoppingProduct{"p1", "Spaghetti", nil, 6, 5, nil},
			shoppingProduct{"p2", "Zucker", nil, 0, 0, nil})

		if got, want := strings.TrimSpace(getShoppingList(t, h)), `{"items":[]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})
}

func TestGetShoppingListMissing(t *testing.T) {
	const empty = `{"items": []}`
	tests := []struct {
		name    string
		product shoppingProduct
		want    string
	}{
		{"below target", shoppingProduct{"p1", "Kidneybohnen", "Bonduelle", 2, 5, nil},
			`{"items": [{"product_id": "p1", "name": "Kidneybohnen", "brand": "Bonduelle", "missing": 3, "stock": 2, "target": 5}]}`},
		{"above target", shoppingProduct{"p1", "Spaghetti", nil, 6, 5, nil}, empty},
		{"at target", shoppingProduct{"p1", "Spaghetti", nil, 5, 5, nil}, empty},
		{"above min_stock", shoppingProduct{"p1", "Mehl", nil, 2, 4, 1}, empty},
		{"at min_stock", shoppingProduct{"p1", "Mehl", nil, 1, 4, 1}, empty},
		{"below min_stock", shoppingProduct{"p1", "Mehl", nil, 0, 4, 1},
			`{"items": [{"product_id": "p1", "name": "Mehl", "brand": null, "missing": 4, "stock": 0, "target": 4}]}`},
		{"target 0", shoppingProduct{"p1", "Zucker", nil, 0, 0, nil}, empty},
		{"target 0 and min_stock 0", shoppingProduct{"p1", "Zucker", nil, 0, 0, 0}, empty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertShoppingProducts(t, db, tt.product)

			checkShoppingList(t, h, tt.want)
		})
	}
}

func TestGetShoppingListSorted(t *testing.T) {
	h, db := newApp(t)
	// Sorted by name COLLATE NOCASE and then id, the list is a2, a3, a4, a5.
	// The binary order of the names would be a3, a5, a2, a4. a5 is inserted
	// before a4 so that only the id decides between the two equal names.
	insertShoppingProducts(t, db,
		shoppingProduct{"a5", "Mehl", nil, 0, 4, 1},
		shoppingProduct{"a4", "mehl", "Aurora", 1, 3, nil},
		shoppingProduct{"a1", "Spaghetti", "Barilla", 6, 5, nil},
		shoppingProduct{"a6", "Zucker", nil, 0, 0, nil},
		shoppingProduct{"a3", "Kidneybohnen", "Bonduelle", 2, 5, nil},
		shoppingProduct{"a2", "apfelmus", nil, 0, 2, nil})

	checkShoppingList(t, h, `{"items": [
		{"product_id": "a2", "name": "apfelmus", "brand": null, "missing": 2, "stock": 0, "target": 2},
		{"product_id": "a3", "name": "Kidneybohnen", "brand": "Bonduelle", "missing": 3, "stock": 2, "target": 5},
		{"product_id": "a4", "name": "mehl", "brand": "Aurora", "missing": 2, "stock": 1, "target": 3},
		{"product_id": "a5", "name": "Mehl", "brand": null, "missing": 4, "stock": 0, "target": 4}
	]}`)
}

func TestGetShoppingListFollowsMovements(t *testing.T) {
	h, db := newApp(t)
	insertShoppingProducts(t, db,
		shoppingProduct{"p1", "Kidneybohnen", nil, 2, 5, nil},
		shoppingProduct{"p2", "Spaghetti", nil, 6, 5, nil})
	checkShoppingList(t, h, `{"items": [
		{"product_id": "p1", "name": "Kidneybohnen", "brand": null, "missing": 3, "stock": 2, "target": 5}
	]}`)

	steps := []struct {
		body string
		want string
	}{
		{`{"product_id": "p2", "kind": "consume", "quantity": 2}`, `{"items": [
			{"product_id": "p1", "name": "Kidneybohnen", "brand": null, "missing": 3, "stock": 2, "target": 5},
			{"product_id": "p2", "name": "Spaghetti", "brand": null, "missing": 1, "stock": 4, "target": 5}
		]}`},
		{`{"product_id": "p1", "kind": "add", "quantity": 3}`, `{"items": [
			{"product_id": "p2", "name": "Spaghetti", "brand": null, "missing": 1, "stock": 4, "target": 5}
		]}`},
	}
	for _, s := range steps {
		if rec := post(h, "/api/v1/movements", s.body); rec.Code != http.StatusCreated {
			t.Fatalf("POST %s: status = %d, want %d, body %s", s.body, rec.Code, http.StatusCreated, rec.Body.String())
		}

		checkShoppingList(t, h, s.want)
	}
}
