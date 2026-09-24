package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// summaryProduct is a test product for the summary. minStock and crateSize
// are nil for NULL.
type summaryProduct struct {
	id, name                           string
	stock, target, marked, needsReview int
	minStock, crateSize                any
}

// insertSummaryProducts inserts ps in their order with direct SQL.
func insertSummaryProducts(t *testing.T, db *sql.DB, ps ...summaryProduct) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, p := range ps {
		if _, err := db.ExecContext(ctx, `INSERT INTO products (id, name, stock, target, min_stock, marked, crate_size,
			origin, lookup_state, needs_review, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'manual', 'none', ?, '2026-09-24T12:00:00.000Z', '2026-09-24T12:00:00.000Z')`,
			p.id, p.name, p.stock, p.target, p.minStock, p.marked, p.crateSize, p.needsReview); err != nil {
			t.Fatalf("insert product %s: %v", p.id, err)
		}
	}
}

// summaryTestProducts are products for the summary, not in the order of their
// names. Sorted by name COLLATE NOCASE and then id, the shopping list is s6,
// s1, s4, s2, s3, s5.
var summaryTestProducts = []summaryProduct{
	// Something missing and to review.
	{id: "s3", name: "Kidneybohnen", stock: 2, target: 5, needsReview: 1},
	// Without target, empty and to review: not on the list.
	{id: "s9", name: "Zucker", needsReview: 1},
	// The same name in lower case, empty and below the minimum stock: sorted
	// before s3 by id.
	{id: "s2", name: "kidneybohnen", target: 4, minStock: 1},
	// Marked with crates and nothing missing.
	{id: "s1", name: "Cola", stock: 24, target: 12, marked: 1, crateSize: 6},
	// Crates, the example of architecture.md 11.3.
	{id: "s4", name: "Jever", stock: 7, target: 24, crateSize: 20},
	// Above the minimum stock: not on the list.
	{id: "s7", name: "Mehl", stock: 2, target: 4, minStock: 1},
	// Marked and something missing.
	{id: "s5", name: "Milch", stock: 1, target: 3, marked: 1},
	// Crates, nothing missing: not on the list.
	{id: "s8", name: "Wasser", stock: 30, target: 24, crateSize: 6},
	// Marked without target and empty.
	{id: "s6", name: "Apfelsaft", marked: 1},
}

// summaryTestJSON is the summary of summaryTestProducts.
const summaryTestJSON = `{
	"product_count": 9,
	"shopping_count": 6,
	"empty_count": 3,
	"review_count": 2,
	"shopping": [
		{"name": "Apfelsaft", "missing": 0, "quantity": 0, "unit": "piece"},
		{"name": "Cola", "missing": 0, "quantity": 0, "unit": "crate"},
		{"name": "Jever", "missing": 17, "quantity": 1, "unit": "crate"},
		{"name": "kidneybohnen", "missing": 4, "quantity": 4, "unit": "piece"},
		{"name": "Kidneybohnen", "missing": 3, "quantity": 3, "unit": "piece"},
		{"name": "Milch", "missing": 2, "quantity": 2, "unit": "piece"}
	],
	"shopping_truncated": false
}`

// getSummary sends GET /api/v1/summary to h, asserts a 200 JSON response and
// returns its body.
func getSummary(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := get(h, "/api/v1/summary")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return rec.Body.String()
}

// decodeJSON decodes the JSON s into a generic value.
func decodeJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

// checkJSON asserts that the JSON got equals the JSON want.
func checkJSON(t *testing.T, got, want string) {
	t.Helper()
	if !reflect.DeepEqual(decodeJSON(t, got), decodeJSON(t, want)) {
		t.Errorf("JSON = %s\nwant %s", got, want)
	}
}

func TestGetSummaryWithoutProducts(t *testing.T) {
	h := newHandler(t)

	got := strings.TrimSpace(getSummary(t, h))

	want := `{"empty_count":0,"product_count":0,"review_count":0,"shopping":[],"shopping_count":0,"shopping_truncated":false}`
	if got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestGetSummary(t *testing.T) {
	h, db := newApp(t)
	insertSummaryProducts(t, db, summaryTestProducts...)

	checkJSON(t, getSummary(t, h), summaryTestJSON)
}

// The counts match the shopping list and the filters "Leer" (stock 0) and
// "Prüfen" (needs_review) of the stock list in the web UI, and shopping holds
// the entries of the shopping list in its order.
func TestGetSummaryMatchesListsAndFilters(t *testing.T) {
	h, db := newApp(t)
	insertSummaryProducts(t, db, summaryTestProducts...)
	// See insertProducts: p2 and p4 need a review, p1 and p3 are empty.
	insertProducts(t, db)
	setMarked(t, db, "p1", 1)

	var summary struct {
		ProductCount  int `json:"product_count"`
		ShoppingCount int `json:"shopping_count"`
		EmptyCount    int `json:"empty_count"`
		ReviewCount   int `json:"review_count"`
		Shopping      []struct {
			Name    string `json:"name"`
			Missing int    `json:"missing"`
		} `json:"shopping"`
	}
	if err := json.Unmarshal([]byte(getSummary(t, h)), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	var products struct {
		Items []struct {
			Stock       int  `json:"stock"`
			NeedsReview bool `json:"needs_review"`
		} `json:"items"`
	}
	if err := json.Unmarshal(get(h, "/api/v1/products").Body.Bytes(), &products); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	var shopping struct {
		Items []struct {
			Name    string `json:"name"`
			Missing int    `json:"missing"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(getShoppingList(t, h)), &shopping); err != nil {
		t.Fatalf("decode shopping list: %v", err)
	}

	var empty, review int
	for _, p := range products.Items {
		if p.Stock == 0 {
			empty++
		}
		if p.NeedsReview {
			review++
		}
	}
	if summary.ProductCount != 13 || summary.ProductCount != len(products.Items) {
		t.Errorf("product_count = %d, want 13, the number of products %d", summary.ProductCount, len(products.Items))
	}
	if summary.EmptyCount != 5 || summary.EmptyCount != empty {
		t.Errorf("empty_count = %d, want 5, the number of products with stock 0 %d", summary.EmptyCount, empty)
	}
	if summary.ReviewCount != 4 || summary.ReviewCount != review {
		t.Errorf("review_count = %d, want 4, the number of products to review %d", summary.ReviewCount, review)
	}
	if summary.ShoppingCount != 9 || summary.ShoppingCount != len(shopping.Items) {
		t.Errorf("shopping_count = %d, want 9, the length of the shopping list %d", summary.ShoppingCount, len(shopping.Items))
	}
	if !reflect.DeepEqual(summary.Shopping, shopping.Items) {
		t.Errorf("shopping = %+v\nwant the shopping list %+v", summary.Shopping, shopping.Items)
	}
}

// shopping holds at most 100 entries, the first ones of the shopping list.
func TestGetSummaryTruncated(t *testing.T) {
	for _, n := range []int{100, 101} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			h, db := newApp(t)
			// Inserted in reverse order of the names, each with one missing.
			for i := n - 1; i >= 0; i-- {
				insertSummaryProducts(t, db, summaryProduct{id: fmt.Sprintf("t%03d", i), name: fmt.Sprintf("Produkt %03d", i), target: 1})
			}
			// Not on the list, sorted before all others.
			insertSummaryProducts(t, db, summaryProduct{id: "voll", name: "Aaa", stock: 1, target: 1})

			var summary struct {
				ProductCount      int              `json:"product_count"`
				ShoppingCount     int              `json:"shopping_count"`
				EmptyCount        int              `json:"empty_count"`
				Shopping          []map[string]any `json:"shopping"`
				ShoppingTruncated bool             `json:"shopping_truncated"`
			}
			if err := json.Unmarshal([]byte(getSummary(t, h)), &summary); err != nil {
				t.Fatalf("decode summary: %v", err)
			}

			if summary.ProductCount != n+1 || summary.ShoppingCount != n || summary.EmptyCount != n || summary.ShoppingTruncated != (n > 100) {
				t.Errorf("product_count, shopping_count, empty_count, shopping_truncated = %d, %d, %d, %v, want %d, %d, %d, %v",
					summary.ProductCount, summary.ShoppingCount, summary.EmptyCount, summary.ShoppingTruncated, n+1, n, n, n > 100)
			}
			if len(summary.Shopping) != 100 {
				t.Fatalf("shopping has %d entries, want 100", len(summary.Shopping))
			}
			for i, it := range summary.Shopping {
				want := map[string]any{"name": fmt.Sprintf("Produkt %03d", i), "missing": 1.0, "quantity": 1.0, "unit": "piece"}
				if !reflect.DeepEqual(it, want) {
					t.Errorf("shopping[%d] = %v, want %v", i, it, want)
				}
			}
		})
	}
}
