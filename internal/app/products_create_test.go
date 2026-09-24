package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/events"
)

// post sends a POST request with the JSON body to h and returns the response.
func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

// decodeCreated asserts that rec is a 201 response with a product whose
// Location header points to it, and returns the decoded product.
func decodeCreated(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var p map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	id, _ := p["id"].(string)
	if loc, want := rec.Header().Get("Location"), "/api/v1/products/"+id; loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
	return p
}

// checkProblemCode asserts that rec is a problem with status and code.
func checkProblemCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d, body %s", rec.Code, status, rec.Body.String())
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
	if p.Status != status || p.Code != code {
		t.Errorf("body = %s, want status %d and code %s", rec.Body.String(), status, code)
	}
}

// countRows returns the number of rows in table.
func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// checkFields asserts that product has the fields of want (a JSON object).
// Fields of product that are not in want are ignored.
func checkFields(t *testing.T, product map[string]any, want string) {
	t.Helper()
	var w map[string]any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	for k, v := range w {
		if !reflect.DeepEqual(product[k], v) {
			t.Errorf("%s = %v, want %v", k, product[k], v)
		}
	}
}

func TestCreateProductWithNameOnly(t *testing.T) {
	h, _ := newApp(t)
	before := time.Now().UTC().Truncate(time.Millisecond)

	rec := post(h, "/api/v1/products", `{"name": "  Kidneybohnen  "}`)

	got := decodeCreated(t, rec)
	checkFields(t, got, `{
		"name": "Kidneybohnen", "brand": null, "package_size": null, "note": null,
		"stock": 0, "target": 0, "min_stock": null, "missing": 0,
		"needs_review": false, "origin": "manual", "lookup_state": "none", "has_image": false, "crate_size": null,
		"barcodes": []
	}`)
	id, _ := got["id"].(string)
	if u, err := uuid.Parse(id); err != nil || u.Version() != 7 || id != strings.ToLower(id) {
		t.Errorf("id = %q, want UUIDv7 in lower case", id)
	}
	createdAt, err := time.Parse(time.RFC3339, got["created_at"].(string))
	if err != nil || createdAt.Before(before) || createdAt.After(time.Now()) {
		t.Errorf("created_at = %v, want the time of the request", got["created_at"])
	}
	if got["updated_at"] != got["created_at"] {
		t.Errorf("updated_at = %v, want created_at %v", got["updated_at"], got["created_at"])
	}

	// The product can be read at its Location.
	read := get(h, rec.Header().Get("Location"))
	if read.Code != http.StatusOK {
		t.Fatalf("GET Location: status = %d, want %d", read.Code, http.StatusOK)
	}
	var stored map[string]any
	if err := json.Unmarshal(read.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode body %s: %v", read.Body.String(), err)
	}
	if !reflect.DeepEqual(stored, got) {
		t.Errorf("GET Location = %s\nwant %s", read.Body.String(), rec.Body.String())
	}
}

func TestCreateProductWithAllFields(t *testing.T) {
	h, _ := newApp(t)

	// The UPC-A code 036000291452 is stored with 13 digits.
	rec := post(h, "/api/v1/products", `{
		"name": "Kidneybohnen", "brand": "  Bonduelle ", "package_size": "  ", "note": " Im Keller ",
		"target": 5, "min_stock": 5,
		"barcodes": [{"code": "4001686301265", "units": 6}, {"code": "036000291452"}]
	}`)

	got := decodeCreated(t, rec)
	checkFields(t, got, `{
		"name": "Kidneybohnen", "brand": "Bonduelle", "package_size": null, "note": "Im Keller",
		"stock": 0, "target": 5, "min_stock": 5, "missing": 5,
		"needs_review": false, "origin": "manual", "lookup_state": "none", "has_image": false,
		"barcodes": [{"code": "0036000291452", "units": 1}, {"code": "4001686301265", "units": 6}]
	}`)
	read := get(h, rec.Header().Get("Location"))
	var stored map[string]any
	if err := json.Unmarshal(read.Body.Bytes(), &stored); err != nil {
		t.Fatalf("decode body %s: %v", read.Body.String(), err)
	}
	if !reflect.DeepEqual(stored, got) {
		t.Errorf("GET Location = %s\nwant %s", read.Body.String(), rec.Body.String())
	}
}

func TestCreateProductTextLengths(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{"name trimmed before check", `{"name": "  ` + strings.Repeat("ä", 120) + `  "}`, http.StatusCreated},
		{"name too long", `{"name": "` + strings.Repeat("a", 121) + `"}`, http.StatusBadRequest},
		{"name blank", `{"name": "   "}`, http.StatusBadRequest},
		{"brand max", `{"name": "a", "brand": "` + strings.Repeat("b", 120) + `"}`, http.StatusCreated},
		{"brand too long", `{"name": "a", "brand": "` + strings.Repeat("b", 121) + `"}`, http.StatusBadRequest},
		{"package_size max", `{"name": "a", "package_size": "` + strings.Repeat("p", 40) + `"}`, http.StatusCreated},
		{"package_size too long", `{"name": "a", "package_size": "` + strings.Repeat("p", 41) + `"}`, http.StatusBadRequest},
		{"note max", `{"name": "a", "note": "` + strings.Repeat("n", 500) + `"}`, http.StatusCreated},
		{"note too long", `{"name": "a", "note": "` + strings.Repeat("n", 501) + `"}`, http.StatusBadRequest},
	}
	h, _ := newApp(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "/api/v1/products", tt.body)

			if tt.status == http.StatusCreated {
				decodeCreated(t, rec)
				return
			}
			checkProblemCode(t, rec, tt.status, "invalid_request")
		})
	}
}

func TestCreateProductErrors(t *testing.T) {
	var recorder events.Recorder
	h, db := newAppWithPublisher(t, &recorder)
	decodeCreated(t, post(h, "/api/v1/products", `{
		"name": "Kidneybohnen", "barcodes": [{"code": "4001686301265"}, {"code": "0036000291452"}]
	}`))

	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"wrong check digit", `{"name": "Mehl", "barcodes": [{"code": "4001686301266"}]}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"not a barcode", `{"name": "Mehl", "barcodes": [{"code": "abc"}]}`, http.StatusUnprocessableEntity, "invalid_barcode"},
		{"barcode in use", `{"name": "Mehl", "barcodes": [{"code": "4001686301265"}]}`, http.StatusConflict, "barcode_in_use"},
		{"barcode in use as UPC-A", `{"name": "Mehl", "barcodes": [{"code": "036000291452"}]}`, http.StatusConflict, "barcode_in_use"},
		{"second barcode in use", `{"name": "Mehl", "barcodes": [{"code": "3017620422003"}, {"code": "4001686301265"}]}`, http.StatusConflict, "barcode_in_use"},
		{"barcode twice in request", `{"name": "Mehl", "barcodes": [{"code": "3017620422003"}, {"code": "03017620422003"}]}`, http.StatusBadRequest, "invalid_request"},
		{"min_stock greater than target", `{"name": "Mehl", "target": 2, "min_stock": 3}`, http.StatusBadRequest, "invalid_request"},
		{"min_stock without target", `{"name": "Mehl", "min_stock": 1}`, http.StatusBadRequest, "invalid_request"},
		{"negative target", `{"name": "Mehl", "target": -1}`, http.StatusBadRequest, "invalid_request"},
		{"target above maximum", `{"name": "Mehl", "target": 100001}`, http.StatusBadRequest, "invalid_request"},
		{"units 0", `{"name": "Mehl", "barcodes": [{"code": "3017620422003", "units": 0}]}`, http.StatusBadRequest, "invalid_request"},
		{"name missing", `{}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "/api/v1/products", tt.body)

			checkProblemCode(t, rec, tt.status, tt.code)
			// Nothing is stored and no event is published.
			if n := countRows(t, db, "products"); n != 1 {
				t.Errorf("products = %d, want 1", n)
			}
			if n := countRows(t, db, "barcodes"); n != 2 {
				t.Errorf("barcodes = %d, want 2", n)
			}
			if n := len(recorder.Events()); n != 1 {
				t.Errorf("events = %d, want 1", n)
			}
		})
	}
}

func TestCreateProductPublishesEvent(t *testing.T) {
	var recorder events.Recorder
	h, _ := newAppWithPublisher(t, &recorder)

	got := decodeCreated(t, post(h, "/api/v1/products", `{"name": " Kidneybohnen "}`))

	recorded := recorder.Events()
	if len(recorded) != 1 {
		t.Fatalf("events = %+v, want one event", recorded)
	}
	e := recorded[0]
	if e.Type != events.TypeProductCreated || e.Source != events.Source {
		t.Errorf("event type = %q, source = %q, want %q and %q", e.Type, e.Source, events.TypeProductCreated, events.Source)
	}
	want := events.ProductCreatedData{ProductID: got["id"].(string), Name: "Kidneybohnen", Origin: "manual"}
	if data, ok := e.Data.(events.ProductCreatedData); !ok || data != want {
		t.Errorf("event data = %#v, want %#v", e.Data, want)
	}
}
