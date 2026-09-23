package lookup_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"github.com/schmitz-chris/stashbert/internal/lookup"
)

const (
	foodCode    = "4001686301265"
	beautyCode  = "8710447445990"
	missingCode = "4009998877669"
	userAgent   = "StashBert/test (test@example.org)"
	fields      = "code,product_name,product_name_de,generic_name_de,brands,quantity,product_quantity,product_quantity_unit,image_front_url,product_type"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return b
}

// respond returns a handler that answers with status and body.
func respond(status int, contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// unlimited returns a limiter that never blocks.
func unlimited() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 1)
}

func newClient(baseURL string, limiter *rate.Limiter) *lookup.Client {
	return lookup.NewClient(baseURL, userAgent, &http.Client{}, limiter)
}

// countRequests wraps h and counts the requests it serves.
func countRequests(h http.Handler, n *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h.ServeHTTP(w, r)
	})
}

func TestLookupFood(t *testing.T) {
	srv := httptest.NewServer(respond(http.StatusOK, "application/json", readTestdata(t, "found_food.json")))
	defer srv.Close()

	got, err := newClient(srv.URL, unlimited()).Lookup(testContext(t), foodCode)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := lookup.Result{
		Found:       true,
		Name:        "Goldbären",
		Brand:       "Haribo",
		PackageSize: "200g",
		ImageURL:    "https://images.openfoodfacts.org/images/products/400/168/630/1265/front_de.64.400.jpg",
		ProductType: "food",
	}
	if got != want {
		t.Errorf("Lookup = %+v, want %+v", got, want)
	}
}

func TestLookupRequest(t *testing.T) {
	requests := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		respond(http.StatusOK, "application/json", readTestdata(t, "found_food.json"))(w, r)
	}))
	defer srv.Close()

	if _, err := newClient(srv.URL+"/", unlimited()).Lookup(testContext(t), foodCode); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	var got *http.Request
	select {
	case got = <-requests:
	default:
		t.Fatal("no request received")
	}
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if want := "/api/v3.6/product/" + foodCode; got.URL.Path != want {
		t.Errorf("path = %q, want %q", got.URL.Path, want)
	}
	if ua := got.Header.Get("User-Agent"); ua != userAgent {
		t.Errorf("User-Agent = %q, want %q", ua, userAgent)
	}
	if want := "product_type=all&lc=de&fields=" + fields; got.URL.RawQuery != want {
		t.Errorf("query = %q, want %q", got.URL.RawQuery, want)
	}
	wantValues := url.Values{"product_type": {"all"}, "lc": {"de"}, "fields": {fields}}
	if q := got.URL.Query(); q.Encode() != wantValues.Encode() {
		t.Errorf("query values = %v, want %v", q, wantValues)
	}
}

func TestLookupFollowsRedirect(t *testing.T) {
	beauty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/api/v3.6/product/" + beautyCode; r.URL.Path != want {
			t.Errorf("redirected path = %q, want %q", r.URL.Path, want)
		}
		if want := "product_type=all&lc=de&fields=" + fields; r.URL.RawQuery != want {
			t.Errorf("redirected query = %q, want %q", r.URL.RawQuery, want)
		}
		if ua := r.Header.Get("User-Agent"); ua != userAgent {
			t.Errorf("redirected User-Agent = %q, want %q", ua, userAgent)
		}
		respond(http.StatusOK, "application/json", readTestdata(t, "found_beauty.json"))(w, r)
	}))
	defer beauty.Close()
	world := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, beauty.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer world.Close()

	// One token per hour: the redirect must not need a second token.
	limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	got, err := newClient(world.URL, limiter).Lookup(testContext(t), beautyCode)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := lookup.Result{
		Found:       true,
		Name:        "Monsavon Gel Douche Passion Bien Fruitée 300ml",
		Brand:       "Monsavon",
		PackageSize: "300 ml",
		ImageURL:    "https://images.openbeautyfacts.org/images/products/871/044/744/5990/front_fr.65.400.jpg",
		ProductType: "beauty",
	}
	if got != want {
		t.Errorf("Lookup = %+v, want %+v", got, want)
	}
}

func TestLookupNotFound(t *testing.T) {
	srv := httptest.NewServer(respond(http.StatusNotFound, "application/json", readTestdata(t, "not_found.json")))
	defer srv.Close()

	got, err := newClient(srv.URL, unlimited()).Lookup(testContext(t), missingCode)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != (lookup.Result{}) {
		t.Errorf("Lookup = %+v, want zero Result (not found)", got)
	}
}

func TestLookupErrorStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        error
		notWant     error
	}{
		{"429 with HTML body", http.StatusTooManyRequests, "text/html", "<html><body>Too Many Requests</body></html>", lookup.ErrRateLimited, lookup.ErrUnavailable},
		{"503", http.StatusServiceUnavailable, "text/html", "<html><body>Service Unavailable</body></html>", lookup.ErrUnavailable, lookup.ErrRateLimited},
		{"500", http.StatusInternalServerError, "text/plain", "boom", lookup.ErrUnavailable, lookup.ErrRateLimited},
		{"200 with HTML body", http.StatusOK, "text/html", "<html><body>Maintenance</body></html>", lookup.ErrUnavailable, lookup.ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(respond(tt.status, tt.contentType, []byte(tt.body)))
			defer srv.Close()

			got, err := newClient(srv.URL, unlimited()).Lookup(testContext(t), foodCode)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Lookup error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, tt.notWant) {
				t.Errorf("Lookup error = %v, must not be %v", err, tt.notWant)
			}
			if got != (lookup.Result{}) {
				t.Errorf("Lookup = %+v, want zero Result", got)
			}
		})
	}
}

func TestLookupNetworkError(t *testing.T) {
	srv := httptest.NewServer(respond(http.StatusOK, "application/json", nil))
	baseURL := srv.URL
	srv.Close()

	_, err := newClient(baseURL, unlimited()).Lookup(testContext(t), foodCode)
	if !errors.Is(err, lookup.ErrUnavailable) {
		t.Fatalf("Lookup error = %v, want %v", err, lookup.ErrUnavailable)
	}
}

func TestLookupContextEnd(t *testing.T) {
	t.Run("deadline during request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(testContext(t), 50*time.Millisecond)
		defer cancel()
		_, err := newClient(srv.URL, unlimited()).Lookup(ctx, foodCode)
		if !errors.Is(err, lookup.ErrUnavailable) {
			t.Fatalf("Lookup error = %v, want %v", err, lookup.ErrUnavailable)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Lookup error = %v, want it to wrap %v", err, context.DeadlineExceeded)
		}
	})

	t.Run("canceled before request", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(countRequests(respond(http.StatusOK, "application/json", readTestdata(t, "found_food.json")), &requests))
		defer srv.Close()

		ctx, cancel := context.WithCancel(testContext(t))
		cancel()
		_, err := newClient(srv.URL, unlimited()).Lookup(ctx, foodCode)
		if !errors.Is(err, lookup.ErrUnavailable) {
			t.Fatalf("Lookup error = %v, want %v", err, lookup.ErrUnavailable)
		}
		if n := requests.Load(); n != 0 {
			t.Errorf("requests = %d, want 0", n)
		}
	})

	t.Run("no token within deadline", func(t *testing.T) {
		var requests atomic.Int32
		srv := httptest.NewServer(countRequests(respond(http.StatusOK, "application/json", readTestdata(t, "found_food.json")), &requests))
		defer srv.Close()

		limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
		if !limiter.Allow() {
			t.Fatal("limiter has no initial token")
		}
		ctx, cancel := context.WithTimeout(testContext(t), 2500*time.Millisecond)
		defer cancel()
		_, err := newClient(srv.URL, limiter).Lookup(ctx, foodCode)
		if !errors.Is(err, lookup.ErrUnavailable) {
			t.Fatalf("Lookup error = %v, want %v", err, lookup.ErrUnavailable)
		}
		if n := requests.Load(); n != 0 {
			t.Errorf("requests = %d, want 0", n)
		}
	})
}

func TestTryLookup(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(countRequests(respond(http.StatusOK, "application/json", readTestdata(t, "found_food.json")), &requests))
	defer srv.Close()

	limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	client := newClient(srv.URL, limiter)

	got, err := client.TryLookup(testContext(t), foodCode)
	if err != nil {
		t.Fatalf("first TryLookup: %v", err)
	}
	if !got.Found || got.Name != "Goldbären" {
		t.Errorf("first TryLookup = %+v, want Goldbären", got)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("requests after first TryLookup = %d, want 1", n)
	}

	// The only token is used up: no request may be sent.
	_, err = client.TryLookup(testContext(t), foodCode)
	if !errors.Is(err, lookup.ErrRateLimited) {
		t.Fatalf("second TryLookup error = %v, want %v", err, lookup.ErrRateLimited)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("requests after second TryLookup = %d, want 1", n)
	}
}

func TestDisabledClient(t *testing.T) {
	client := lookup.NewDisabledClient()
	if _, err := client.Lookup(testContext(t), foodCode); !errors.Is(err, lookup.ErrDisabled) {
		t.Errorf("Lookup error = %v, want %v", err, lookup.ErrDisabled)
	}
	if _, err := client.TryLookup(testContext(t), foodCode); !errors.Is(err, lookup.ErrDisabled) {
		t.Errorf("TryLookup error = %v, want %v", err, lookup.ErrDisabled)
	}
}

func TestFinalize(t *testing.T) {
	// 8 characters, 11 bytes; 25 times gives 200 characters and 275 bytes.
	long := strings.Repeat("Äpfelmüß", 25)
	if n := utf8.RuneCountInString(long); n != 200 {
		t.Fatalf("test name has %d characters, want 200", n)
	}
	prefix := func(s string, n int) string { return string([]rune(s)[:n]) }

	t.Run("cuts by characters", func(t *testing.T) {
		in := lookup.Result{
			Found:       true,
			Name:        long,
			Brand:       long,
			PackageSize: long,
			ImageURL:    "https://images.openfoodfacts.org/x.jpg",
			ProductType: "food",
		}
		got := lookup.Finalize(in, foodCode)
		want := lookup.Result{
			Found:       true,
			Name:        prefix(long, 120),
			Brand:       prefix(long, 120),
			PackageSize: prefix(long, 40),
			ImageURL:    in.ImageURL,
			ProductType: in.ProductType,
		}
		if got != want {
			t.Errorf("Finalize = %+v, want %+v", got, want)
		}
		if n := utf8.RuneCountInString(got.Name); n != 120 {
			t.Errorf("name has %d characters, want 120", n)
		}
		if !utf8.ValidString(got.Name) || !utf8.ValidString(got.PackageSize) {
			t.Error("Finalize produced invalid UTF-8")
		}
	})

	t.Run("keeps short values", func(t *testing.T) {
		in := lookup.Result{Found: true, Name: prefix(long, 120), Brand: "Haribo", PackageSize: "200g"}
		if got := lookup.Finalize(in, foodCode); got != in {
			t.Errorf("Finalize = %+v, want %+v", got, in)
		}
	})

	for _, tt := range []struct{ title, name string }{{"empty", ""}, {"blank", "   "}} {
		t.Run("fallback name for "+tt.title, func(t *testing.T) {
			got := lookup.Finalize(lookup.Result{Found: true, Name: tt.name, Brand: "Haribo"}, foodCode)
			want := lookup.Result{Found: true, Name: "Neues Produkt " + foodCode, Brand: "Haribo"}
			if got != want {
				t.Errorf("Finalize = %+v, want %+v", got, want)
			}
		})
	}
}
