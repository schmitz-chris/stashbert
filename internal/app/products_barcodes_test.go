package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// removeBarcode sends a DELETE request for the barcode code of the product id to h.
func removeBarcode(h http.Handler, id, code string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/products/"+id+"/barcodes/"+code, nil))
	return rec
}

func TestAddBarcode(t *testing.T) {
	// See insertProducts: p3 has no barcodes.
	tests := []struct {
		name string
		body string
		want string
	}{
		{"EAN-13 with units", `{"code": "4006381333931", "units": 6}`, `{"code": "4006381333931", "units": 6}`},
		{"EAN-8 with default units", `{"code": "96385074"}`, `{"code": "96385074", "units": 1}`},
		{"UPC-A", `{"code": "036000291452"}`, `{"code": "0036000291452", "units": 1}`},
		{"GTIN-14 with leading zero", `{"code": "04006381333931", "units": 2}`, `{"code": "4006381333931", "units": 2}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)

			rec := post(h, "/api/v1/products/p3/barcodes", tt.body)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("Location = %q, want none", loc)
			}
			var got, want map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %s: %v", rec.Body.String(), err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatalf("decode want: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("barcode = %v, want %v", got, want)
			}
			// The barcode belongs to p3 now.
			checkFields(t, decodeProduct(t, get(h, "/api/v1/products/p3")), `{"barcodes": [`+tt.want+`]}`)
			if n := countRows(t, db, "barcodes"); n != 4 {
				t.Errorf("barcodes = %d, want 4", n)
			}
		})
	}
}

func TestAddBarcodeErrors(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	before := get(h, "/api/v1/products").Body.String()

	// See insertProducts: p2 has 4001686301265 and 0034000470693, p4 has 3017620422003.
	tests := []struct {
		name   string
		id     string
		body   string
		status int
		code   string
	}{
		{"wrong check digit", "p3", `{"code": "4006381333932"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"not a barcode", "p3", `{"code": "abc"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"wrong length", "p3", `{"code": "1234567"}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"in use by another product", "p3", `{"code": "3017620422003"}`, http.StatusConflict, "barcode_in_use"},
		{"in use by the same product", "p2", `{"code": "4001686301265"}`, http.StatusConflict, "barcode_in_use"},
		{"in use as UPC-A", "p2", `{"code": "034000470693"}`, http.StatusConflict, "barcode_in_use"},
		{"unknown product", "unbekannt", `{"code": "4006381333931"}`, http.StatusNotFound, "not_found"},
		{"units 0", "p3", `{"code": "4006381333931", "units": 0}`, http.StatusBadRequest, "invalid_request"},
		{"code missing", "p3", `{"units": 1}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "/api/v1/products/"+tt.id+"/barcodes", tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is changed.
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := countRows(t, db, "barcodes"); n != 3 {
				t.Errorf("barcodes = %d, want 3", n)
			}
		})
	}
}

func TestRemoveBarcode(t *testing.T) {
	// See insertProducts: p2 has 4001686301265 and 0034000470693 (units 6).
	tests := []struct {
		name string
		code string
		left string
	}{
		{"normalized", "4001686301265", `{"code": "0034000470693", "units": 6}`},
		{"GTIN-14 with leading zero", "04001686301265", `{"code": "0034000470693", "units": 6}`},
		{"UPC-A", "034000470693", `{"code": "4001686301265", "units": 1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)
			insertProducts(t, db)

			rec := removeBarcode(h, "p2", tt.code)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
			checkFields(t, decodeProduct(t, get(h, "/api/v1/products/p2")), `{"barcodes": [`+tt.left+`]}`)
			if n := countRows(t, db, "barcodes"); n != 2 {
				t.Errorf("barcodes = %d, want 2", n)
			}
			// The barcode is gone, so removing it again fails.
			checkProblemCode(t, removeBarcode(h, "p2", tt.code), http.StatusNotFound, "not_found")
		})
	}
}

func TestRemoveBarcodeErrors(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	before := get(h, "/api/v1/products").Body.String()

	// See insertProducts: p2 has 4001686301265 and 0034000470693, p4 has 3017620422003.
	tests := []struct {
		name string
		id   string
		code string
	}{
		{"wrong check digit", "p2", "4001686301266"},
		{"not a barcode", "p2", "abc"},
		{"wrong length", "p2", "1234567"},
		{"barcode of another product", "p2", "3017620422003"},
		{"unassigned barcode", "p2", "4006381333931"},
		{"unknown product", "unbekannt", "4001686301265"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := removeBarcode(h, tt.id, tt.code)

			checkProblemCode(t, rec, http.StatusNotFound, "not_found")
			// Nothing is changed.
			if after := get(h, "/api/v1/products").Body.String(); after != before {
				t.Errorf("products = %s\nwant %s", after, before)
			}
			if n := countRows(t, db, "barcodes"); n != 3 {
				t.Errorf("barcodes = %d, want 3", n)
			}
		})
	}
}
