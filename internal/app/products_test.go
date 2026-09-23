package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// insertProducts inserts test products and barcodes with direct SQL. Sorted by
// name COLLATE NOCASE and then id, the order is p2, p3, p4, p1. The binary
// order of the names would be p3, p4, p1, p2. p3 is inserted before p2 so that
// only the id decides between the two equal names.
func insertProducts(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	statements := []string{
		`INSERT INTO products (id, name, brand, package_size, stock, target, min_stock, note,
			origin, lookup_state, needs_review, image_source_url, image_file, created_at, updated_at) VALUES
			('p3', 'Kidneybohnen', NULL, NULL, 0, 4, 1, NULL,
				'manual', 'none', 0, NULL, NULL, '2026-09-23T12:00:00.000Z', '2026-09-23T12:00:00.000Z'),
			('p1', 'Zucker', NULL, NULL, 0, 0, NULL, NULL,
				'manual', 'none', 0, NULL, NULL, '2026-09-23T12:00:00.000Z', '2026-09-23T12:00:00.000Z'),
			('p2', 'kidneybohnen', 'Bonduelle', '400 g', 2, 5, NULL, 'Im Keller',
				'openfoodfacts', 'done', 1, 'https://images.openfoodfacts.org/p2.jpg', 'p2.jpg',
				'2026-09-23T12:00:00.123Z', '2026-09-24T08:30:00.456Z'),
			('p4', 'Mehl', NULL, '1 kg', 2, 4, 1, NULL,
				'placeholder', 'pending', 1, NULL, NULL, '2026-09-23T12:00:00.000Z', '2026-09-23T12:00:00.000Z')`,
		`INSERT INTO barcodes (code, product_id, units, created_at) VALUES
			('4001686301265', 'p2', 1, '2026-09-23T12:00:00.000Z'),
			('3017620422003', 'p4', 1, '2026-09-23T12:00:00.000Z'),
			('0034000470693', 'p2', 6, '2026-09-23T12:00:00.000Z')`,
	}
	for _, s := range statements {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("insert test data: %v", err)
		}
	}
}

// get sends a GET request to h and returns the response.
func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestListProducts(t *testing.T) {
	h, db := newApp(t)

	t.Run("empty", func(t *testing.T) {
		rec := get(h, "/api/v1/products")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if got, want := strings.TrimSpace(rec.Body.String()), `{"items":[]}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("sorted with missing and barcodes", func(t *testing.T) {
		insertProducts(t, db)

		rec := get(h, "/api/v1/products")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var list struct {
			Items []struct {
				ID       string `json:"id"`
				Missing  int    `json:"missing"`
				Barcodes []struct {
					Code  string `json:"code"`
					Units int    `json:"units"`
				} `json:"barcodes"`
			} `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode body %s: %v", rec.Body.String(), err)
		}

		// Each product as "id missing=<n> [code×units ...]".
		var got []string
		for _, p := range list.Items {
			codes := []string{}
			for _, b := range p.Barcodes {
				codes = append(codes, fmt.Sprintf("%s×%d", b.Code, b.Units))
			}
			got = append(got, fmt.Sprintf("%s missing=%d %v", p.ID, p.Missing, codes))
		}
		want := []string{
			"p2 missing=3 [0034000470693×6 4001686301265×1]",
			"p3 missing=4 []",
			"p4 missing=0 [3017620422003×1]",
			"p1 missing=0 []",
		}
		if !slices.Equal(got, want) {
			t.Errorf("products:\n got %q\nwant %q", got, want)
		}
	})
}

func TestGetProduct(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	tests := []struct {
		id   string
		want string
	}{
		{"p2", `{
			"id": "p2", "name": "kidneybohnen", "brand": "Bonduelle", "package_size": "400 g", "note": "Im Keller",
			"stock": 2, "target": 5, "min_stock": null, "missing": 3,
			"needs_review": true, "origin": "openfoodfacts", "lookup_state": "done", "has_image": true,
			"barcodes": [{"code": "0034000470693", "units": 6}, {"code": "4001686301265", "units": 1}],
			"created_at": "2026-09-23T12:00:00.123Z", "updated_at": "2026-09-24T08:30:00.456Z"
		}`},
		{"p3", `{
			"id": "p3", "name": "Kidneybohnen", "brand": null, "package_size": null, "note": null,
			"stock": 0, "target": 4, "min_stock": 1, "missing": 4,
			"needs_review": false, "origin": "manual", "lookup_state": "none", "has_image": false,
			"barcodes": [],
			"created_at": "2026-09-23T12:00:00Z", "updated_at": "2026-09-23T12:00:00Z"
		}`},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			rec := get(h, "/api/v1/products/"+tt.id)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got, want any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %s: %v", rec.Body.String(), err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatalf("decode want: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %s\nwant %s", rec.Body.String(), tt.want)
			}
		})
	}
}

func TestGetProductUnknownID(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)

	rec := get(h, "/api/v1/products/unbekannt")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p struct {
		Status int
		Code   string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if p.Status != http.StatusNotFound || p.Code != "not_found" {
		t.Errorf("body = %s, want status 404 and code not_found", rec.Body.String())
	}
}
