// Package lookup looks up unknown barcodes in Open Food Facts
// (architecture.md, 7.2 and 7.3).
package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/time/rate"
)

var (
	// ErrRateLimited is returned when Open Food Facts answers 429, or by
	// TryLookup when the limiter has no free token.
	ErrRateLimited = errors.New("open food facts: rate limited")
	// ErrUnavailable is returned for 5xx answers, network errors, the end of
	// the context and any other answer that is neither a product nor a 404.
	ErrUnavailable = errors.New("open food facts: unavailable")
	// ErrDisabled is returned by every lookup of a client created with
	// NewDisabledClient.
	ErrDisabled = errors.New("open food facts: lookups disabled")
)

const (
	// query is the fixed query string from architecture.md, 7.2.
	query = "product_type=all&lc=de&fields=code,product_name,product_name_de,generic_name_de,brands,quantity,product_quantity,product_quantity_unit,image_front_url,product_type"
	// maxBodyBytes bounds the response body that is decoded.
	maxBodyBytes = 1 << 20

	placeholderPrefix    = "Neues Produkt "
	maxNameLength        = 120
	maxBrandLength       = 120
	maxPackageSizeLength = 40
)

// Result is a mapped lookup result (architecture.md, 7.2, step 3). Values are
// trimmed but not shortened; Finalize prepares them for storage. The JSON
// form is the payload of the lookups cache.
type Result struct {
	Found       bool   `json:"found"`
	Name        string `json:"name"`
	Brand       string `json:"brand"`
	PackageSize string `json:"package_size"`
	ImageURL    string `json:"image_url"`
	ProductType string `json:"product_type"`
}

// Client queries the Open Food Facts API. It is safe for concurrent use.
type Client struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
	limiter    *rate.Limiter
	disabled   bool
}

// NewClient returns a client for the API at baseURL (for example
// "https://world.openfoodfacts.org"). userAgent is sent as is and should have
// the form "StashBert/<version> (<OFF_CONTACT>)". httpClient follows the
// redirects to the sister databases. limiter is shared by all lookups and is
// asked once per lookup, not per HTTP request.
func NewClient(baseURL, userAgent string, httpClient *http.Client, limiter *rate.Limiter) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  userAgent,
		httpClient: httpClient,
		limiter:    limiter,
	}
}

// NewDisabledClient returns a client whose Lookup and TryLookup always return
// ErrDisabled. It is used when OFF_CONTACT is empty.
func NewDisabledClient() *Client {
	return &Client{disabled: true}
}

// Lookup waits for a limiter token within ctx and then looks up code.
// A 404 answer returns a Result with Found = false and no error.
func (c *Client) Lookup(ctx context.Context, code string) (Result, error) {
	if c.disabled {
		return Result{}, ErrDisabled
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return Result{}, fmt.Errorf("%w: wait for limiter: %w", ErrUnavailable, err)
	}
	return c.fetch(ctx, code)
}

// TryLookup is like Lookup, but takes a limiter token only if one is free.
// Otherwise it returns ErrRateLimited without sending a request.
func (c *Client) TryLookup(ctx context.Context, code string) (Result, error) {
	if c.disabled {
		return Result{}, ErrDisabled
	}
	if !c.limiter.Allow() {
		return Result{}, fmt.Errorf("%w: no free limiter token", ErrRateLimited)
	}
	return c.fetch(ctx, code)
}

// Finalize prepares r for storage: a missing name becomes
// "Neues Produkt <code>", then Name and Brand are cut to 120 and PackageSize
// to 40 characters. Spaces left at the end of a cut value are removed.
func Finalize(r Result, code string) Result {
	if strings.TrimSpace(r.Name) == "" {
		r.Name = placeholderPrefix + code
	}
	r.Name = strings.TrimSpace(truncate(r.Name, maxNameLength))
	r.Brand = strings.TrimSpace(truncate(r.Brand, maxBrandLength))
	r.PackageSize = strings.TrimSpace(truncate(r.PackageSize, maxPackageSizeLength))
	return r
}

func (c *Client) fetch(ctx context.Context, code string) (Result, error) {
	u := c.baseURL + "/api/v3.6/product/" + url.PathEscape(code) + "?" + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{}, fmt.Errorf("%w: build request: %w", ErrUnavailable, err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Result{}, nil
	case http.StatusTooManyRequests:
		return Result{}, fmt.Errorf("%w: status %d", ErrRateLimited, resp.StatusCode)
	default:
		return Result{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	var body struct {
		Product *product `json:"product"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&body); err != nil {
		return Result{}, fmt.Errorf("%w: decode response: %w", ErrUnavailable, err)
	}
	if body.Product == nil {
		return Result{}, fmt.Errorf("%w: response without product", ErrUnavailable)
	}
	return body.Product.result(), nil
}

// product holds the requested fields of an Open Food Facts product.
type product struct {
	ProductName         string          `json:"product_name"`
	ProductNameDE       string          `json:"product_name_de"`
	GenericNameDE       string          `json:"generic_name_de"`
	Brands              string          `json:"brands"`
	Quantity            string          `json:"quantity"`
	ProductQuantity     json.RawMessage `json:"product_quantity"`
	ProductQuantityUnit string          `json:"product_quantity_unit"`
	ImageFrontURL       string          `json:"image_front_url"`
	ProductType         string          `json:"product_type"`
}

// result maps p as described in architecture.md, 7.2, step 3.
func (p *product) result() Result {
	brand, _, _ := strings.Cut(p.Brands, ",")
	return Result{
		Found:       true,
		Name:        firstNonEmpty(p.ProductNameDE, p.ProductName, p.GenericNameDE),
		Brand:       strings.TrimSpace(brand),
		PackageSize: p.packageSize(),
		ImageURL:    strings.TrimSpace(p.ImageFrontURL),
		ProductType: strings.TrimSpace(p.ProductType),
	}
}

// packageSize returns quantity, otherwise product_quantity and
// product_quantity_unit separated by a space (as in "400 g").
func (p *product) packageSize() string {
	if q := strings.TrimSpace(p.Quantity); q != "" {
		return q
	}
	amount := quantityText(p.ProductQuantity)
	if amount == "" {
		return ""
	}
	return strings.TrimSpace(amount + " " + strings.TrimSpace(p.ProductQuantityUnit))
}

// quantityText returns product_quantity as text. Open Food Facts sends a
// number; a string is accepted as well.
func quantityText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// truncate cuts s to at most n characters (runes, not bytes).
func truncate(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
