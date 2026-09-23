package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/store"
)

// movementID returns the id of the test movement i. It is a UUID, and the ids
// sort like i.
func movementID(i int) string {
	return fmt.Sprintf("0199a5c4-7b1e-7c3a-9f00-%012d", i)
}

// insertMovements inserts the movement movementID(i) of the product
// productIDs[i] for each i with direct SQL. The order of insertion is random,
// so it differs from the order of the ids.
func insertMovements(t *testing.T, db *sql.DB, productIDs []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback()
	start := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	for _, i := range rand.New(rand.NewPCG(1, 2)).Perm(len(productIDs)) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO movements (id, product_id, kind, delta, stock_after, created_at)
			VALUES (?, ?, 'add', 1, ?, ?)`,
			movementID(i), productIDs[i], i+1, store.FormatTime(start.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("insert movement %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit movements: %v", err)
	}
}

// wantMovementIDs returns the ids of the test movements i with
// productIDs[i] == productID, or of all if productID is empty, by id
// descending.
func wantMovementIDs(productIDs []string, productID string) []string {
	var ids []string
	for i := len(productIDs) - 1; i >= 0; i-- {
		if productID == "" || productIDs[i] == productID {
			ids = append(ids, movementID(i))
		}
	}
	return ids
}

// listMovements sends GET /api/v1/movements with query to h, asserts a 200
// JSON response with the fields items and next_cursor and returns the ids of
// the items and next_cursor, "" for null.
func listMovements(t *testing.T, h http.Handler, query url.Values) (ids []string, next string) {
	t.Helper()
	rec := get(h, "/api/v1/movements?"+query.Encode())
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want %d, body %s", query.Encode(), rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor json.RawMessage `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	if body.Items == nil {
		t.Errorf("%s: items = %s, want an array", query.Encode(), rec.Body.String())
	}
	for _, m := range body.Items {
		ids = append(ids, m.ID)
	}
	switch string(body.NextCursor) {
	case "":
		t.Errorf("%s: body = %s, want next_cursor", query.Encode(), rec.Body.String())
	case "null":
	default:
		if err := json.Unmarshal(body.NextCursor, &next); err != nil || next == "" {
			t.Errorf("%s: next_cursor = %s, want a string or null", query.Encode(), body.NextCursor)
		}
	}
	return ids, next
}

// listAllMovements requests the first page with query, then follows
// next_cursor until it is null. It returns the ids of all pages and the number
// of items on each page. Each next_cursor must be the id of the last item.
func listAllMovements(t *testing.T, h http.Handler, query url.Values) (ids []string, sizes []int) {
	t.Helper()
	// Set replaces the values of cursor, so a shallow copy keeps query unchanged.
	q := maps.Clone(query)
	for range 10 {
		page, next := listMovements(t, h, q)
		ids = append(ids, page...)
		sizes = append(sizes, len(page))
		if next == "" {
			return ids, sizes
		}
		if len(page) == 0 || next != page[len(page)-1] {
			t.Fatalf("next_cursor = %q, want the id of the last item of %q", next, page)
		}
		q.Set("cursor", next)
	}
	t.Fatalf("%s: more than 10 pages", query.Encode())
	return nil, nil
}

func TestListMovementsEmpty(t *testing.T) {
	h := newHandler(t)

	for _, query := range []string{"", "?product_id=p2", "?limit=1&cursor=" + movementID(1)} {
		rec := get(h, "/api/v1/movements"+query)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want %d, body %s", query, rec.Code, http.StatusOK, rec.Body.String())
		}
		if got, want := strings.TrimSpace(rec.Body.String()), `{"items":[],"next_cursor":null}`; got != want {
			t.Errorf("%s: body = %s, want %s", query, got, want)
		}
	}
}

func TestListMovementsPages(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	products := slices.Repeat([]string{"p2"}, 120)
	insertMovements(t, db, products)

	ids, sizes := listAllMovements(t, h, url.Values{"limit": {"50"}})

	if want := []int{50, 50, 20}; !slices.Equal(sizes, want) {
		t.Errorf("page sizes = %v, want %v", sizes, want)
	}
	// All movements by id descending, each one once.
	if want := wantMovementIDs(products, ""); !slices.Equal(ids, want) {
		t.Errorf("ids =\n%q\nwant\n%q", ids, want)
	}
}

func TestListMovementsProductFilter(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	// 30 movements each of p2, p4 and p1, interleaved.
	var products []string
	for range 30 {
		products = append(products, "p2", "p4", "p1")
	}
	insertMovements(t, db, products)

	tests := []struct {
		name  string
		query url.Values
		want  []string
		sizes []int
	}{
		{"p2 with limit 12", url.Values{"product_id": {"p2"}, "limit": {"12"}},
			wantMovementIDs(products, "p2"), []int{12, 12, 6}},
		{"p4 with limit 10", url.Values{"product_id": {"p4"}, "limit": {"10"}},
			wantMovementIDs(products, "p4"), []int{10, 10, 10}},
		{"p1 with default limit", url.Values{"product_id": {"p1"}},
			wantMovementIDs(products, "p1"), []int{30}},
		{"product without movements", url.Values{"product_id": {"p3"}, "limit": {"5"}},
			nil, []int{0}},
		{"unknown product", url.Values{"product_id": {"unbekannt"}},
			nil, []int{0}},
		{"all with limit 40", url.Values{"limit": {"40"}},
			wantMovementIDs(products, ""), []int{40, 40, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, sizes := listAllMovements(t, h, tt.query)

			if !slices.Equal(sizes, tt.sizes) {
				t.Errorf("page sizes = %v, want %v", sizes, tt.sizes)
			}
			if !slices.Equal(ids, tt.want) {
				t.Errorf("ids =\n%q\nwant\n%q", ids, tt.want)
			}
		})
	}

	t.Run("cursor of another product", func(t *testing.T) {
		// movementID(61) belongs to p4; the page of p2 starts below it.
		ids, next := listMovements(t, h, url.Values{"product_id": {"p2"}, "limit": {"3"}, "cursor": {movementID(61)}})

		if want := []string{movementID(60), movementID(57), movementID(54)}; !slices.Equal(ids, want) {
			t.Errorf("ids = %q, want %q", ids, want)
		}
		if next != movementID(54) {
			t.Errorf("next_cursor = %q, want %q", next, movementID(54))
		}
	})
}

func TestListMovementsParameters(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	products := slices.Repeat([]string{"p2"}, 120)
	insertMovements(t, db, products)
	all := wantMovementIDs(products, "")

	tests := []struct {
		name  string
		query url.Values
		want  []string
		next  string
	}{
		{"default limit 50", url.Values{}, all[:50], all[49]},
		{"limit 1", url.Values{"limit": {"1"}}, all[:1], all[0]},
		{"limit 119", url.Values{"limit": {"119"}}, all[:119], all[118]},
		{"limit equal to the number of movements", url.Values{"limit": {"120"}}, all, ""},
		{"limit 200", url.Values{"limit": {"200"}}, all, ""},
		{"cursor with default limit", url.Values{"cursor": {movementID(100)}}, all[20:70], all[69]},
		{"cursor leaving exactly limit movements", url.Values{"limit": {"50"}, "cursor": {movementID(50)}}, all[70:], ""},
		{"cursor in upper case", url.Values{"limit": {"10"}, "cursor": {strings.ToUpper(movementID(60))}}, all[60:70], all[69]},
		{"cursor above all ids", url.Values{"limit": {"5"}, "cursor": {"ffffffff-ffff-7fff-bfff-ffffffffffff"}}, all[:5], all[4]},
		{"cursor below all ids", url.Values{"cursor": {"00000000-0000-0000-0000-000000000000"}}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, next := listMovements(t, h, tt.query)

			if !slices.Equal(ids, tt.want) {
				t.Errorf("ids =\n%q\nwant\n%q", ids, tt.want)
			}
			if next != tt.next {
				t.Errorf("next_cursor = %q, want %q", next, tt.next)
			}
		})
	}
}

func TestListMovementsInvalidParameters(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	insertMovements(t, db, []string{"p2", "p2"})

	for _, query := range []string{
		"limit=0",
		"limit=201",
		"limit=-1",
		"limit=zehn",
		"cursor=abc",
		"cursor=" + movementID(1)[:35],
		"cursor=" + movementID(1) + "0",
		"product_id=p2&cursor=abc",
	} {
		t.Run(query, func(t *testing.T) {
			checkProblemCode(t, get(h, "/api/v1/movements?"+query), http.StatusBadRequest, "invalid_request")
		})
	}
}

func TestListMovementsItems(t *testing.T) {
	h, db := newApp(t)
	insertProducts(t, db)
	// See insertProducts: p2 "kidneybohnen" has stock 2 and the barcode
	// 0034000470693 with 6 units, p4 "Mehl" has stock 2.
	_, first := decodeMovementResult(t, post(h, "/api/v1/movements", `{"barcode": "0034000470693", "kind": "add"}`))
	_, second := decodeMovementResult(t, post(h, "/api/v1/movements", `{"product_id": "p4", "kind": "consume"}`))
	// A reversal of the first movement with direct SQL. UUIDv7 ids of one
	// process increase, so it is the newest movement.
	reversalID := uuid.Must(uuid.NewV7()).String()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `INSERT INTO movements (id, product_id, kind, delta, stock_after, reverses_id, created_at)
		VALUES (?, 'p2', 'reversal', -6, 2, ?, '2026-09-24T10:00:00.000Z')`, reversalID, first["id"]); err != nil {
		t.Fatalf("insert reversal: %v", err)
	}

	rec := get(h, "/api/v1/movements")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor any              `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body.String(), err)
	}
	if len(body.Items) != 3 || body.NextCursor != nil {
		t.Fatalf("body = %s, want 3 items and next_cursor null", rec.Body.String())
	}
	var reversal map[string]any
	if err := json.Unmarshal([]byte(fmt.Sprintf(`{"id": %q, "product_id": "p2", "kind": "reversal", "delta": -6,
		"stock_after": 2, "barcode": null, "reverses_id": %q, "created_at": "2026-09-24T10:00:00Z"}`,
		reversalID, first["id"])), &reversal); err != nil {
		t.Fatalf("decode want: %v", err)
	}
	// The movements of createMovement are listed unchanged.
	for i, want := range []map[string]any{reversal, second, first} {
		if !reflect.DeepEqual(body.Items[i], want) {
			t.Errorf("item %d = %v\n    want %v", i, body.Items[i], want)
		}
	}
}
