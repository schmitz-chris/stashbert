// Package recognize reads name, brand and package size of a product from its
// photo with a language model of OpenAI, Google Gemini or Anthropic
// (ADR-0021). The providers are called directly over HTTP. The API key is
// sent only in a request header; it never appears in a URL, an error or the
// log.
package recognize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// The providers, as stored in the settings and named in the API.
const (
	OpenAI    = "openai"
	Gemini    = "gemini"
	Anthropic = "anthropic"
)

// The default model of each provider: an inexpensive current model with
// image input, taken from the model pages of the providers on 2026-09-25.
// The settings can name another model, because model names change quickly.
const (
	DefaultOpenAIModel    = "gpt-6-luna"
	DefaultGeminiModel    = "gemini-3.5-flash-lite"
	DefaultAnthropicModel = "claude-sonnet-5"
)

// DefaultModel returns the default model of provider, or "" for an unknown
// provider.
func DefaultModel(provider string) string {
	switch provider {
	case OpenAI:
		return DefaultOpenAIModel
	case Gemini:
		return DefaultGeminiModel
	case Anthropic:
		return DefaultAnthropicModel
	}
	return ""
}

// Endpoints holds the base URLs of the provider APIs, without a trailing
// slash. Tests point them to fake servers.
type Endpoints struct {
	OpenAI    string
	Gemini    string
	Anthropic string
}

// DefaultEndpoints are the base URLs of the real provider APIs.
var DefaultEndpoints = Endpoints{
	OpenAI:    "https://api.openai.com",
	Gemini:    "https://generativelanguage.googleapis.com",
	Anthropic: "https://api.anthropic.com",
}

// Timeout is the time limit of each request to a provider.
const Timeout = 60 * time.Second

// maxResponseBytes bounds the response body that is read from a provider.
const maxResponseBytes = 4 << 20

// Length limits of the product fields in characters (architecture.md, 5).
const (
	maxNameLength        = 120
	maxBrandLength       = 120
	maxPackageSizeLength = 40
)

// prompt is the instruction to the model; the schema of the answer is sent
// separately as structured output.
const prompt = "Read the product packaging in this photo. " +
	"Return the product name, the brand and the package size exactly as they are printed on the package. " +
	`The package size is the quantity with its unit, for example "500 g". ` +
	"Return null for every field that you cannot read on the photo. Never guess or invent a value."

// fieldDescriptions describe the fields of the answer in the schema.
var fieldDescriptions = map[string]string{
	"name":         "Product name as printed on the package, or null if it is not readable.",
	"brand":        "Brand as printed on the package, or null if it is not readable.",
	"package_size": `Quantity with its unit as printed on the package, for example "500 g", or null if it is not readable.`,
}

// fields are the fields of the answer in their order.
var fields = []string{"name", "brand", "package_size"}

// Result is what a provider reads on the photo. A nil field is not readable.
type Result struct {
	Name        *string `json:"name"`
	Brand       *string `json:"brand"`
	PackageSize *string `json:"package_size"`
}

// Client sends requests to the providers. It is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	endpoints  Endpoints
	timeout    time.Duration
	userAgent  string
}

// NewClient returns a Client that sends its requests with httpClient to
// endpoints, each with the time limit timeout (usually Timeout), and with
// userAgent as the User-Agent header.
func NewClient(httpClient *http.Client, endpoints Endpoints, timeout time.Duration, userAgent string) *Client {
	return &Client{httpClient: httpClient, endpoints: endpoints, timeout: timeout, userAgent: userAgent}
}

// InvalidAPIKey returns the 422 invalid_api_key error.
func InvalidAPIKey() *httpx.Error {
	return httpx.NewError(http.StatusUnprocessableEntity, "invalid_api_key", "Der Anbieter nimmt den API-Schlüssel nicht an")
}

// Failed returns the 502 recognition_failed error.
func Failed() *httpx.Error {
	return httpx.NewError(http.StatusBadGateway, "recognition_failed", "Der Anbieter ist nicht erreichbar oder liefert keine brauchbare Antwort")
}

// Disabled returns the 409 recognition_disabled error.
func Disabled() *httpx.Error {
	return httpx.NewError(http.StatusConflict, "recognition_disabled", "Die Produkterkennung ist nicht eingerichtet")
}

// NoImage returns the 409 no_image error.
func NoImage() *httpx.Error {
	return httpx.NewError(http.StatusConflict, "no_image", "Das Produkt hat kein Foto")
}

// CheckKey asks provider for its model list with key. A key that the
// provider rejects results in an error wrapping InvalidAPIKey, every other
// failure in one wrapping Failed.
func (c *Client) CheckKey(ctx context.Context, provider, key string) error {
	var url string
	switch provider {
	case OpenAI:
		url = c.endpoints.OpenAI + "/v1/models"
	case Gemini:
		url = c.endpoints.Gemini + "/v1beta/models"
	case Anthropic:
		url = c.endpoints.Anthropic + "/v1/models"
	default:
		return fmt.Errorf("check key: %w: unknown provider %q", Failed(), provider)
	}
	return c.do(ctx, provider, http.MethodGet, url, key, nil, nil)
}

// Recognize sends the image with its media type (image/jpeg, image/png or
// image/webp) to provider with model and key and returns what the model
// reads on it: trimmed, cut to the length limits of the product fields
// (architecture.md, 5) and nil where it is empty. A key that the provider
// rejects results in an error wrapping InvalidAPIKey, every other failure in
// one wrapping Failed.
func (c *Client) Recognize(ctx context.Context, provider, model, key string, image []byte, mediaType string) (Result, error) {
	var text string
	var err error
	switch provider {
	case OpenAI:
		text, err = c.openAI(ctx, model, key, image, mediaType)
	case Gemini:
		text, err = c.gemini(ctx, model, key, image, mediaType)
	case Anthropic:
		text, err = c.anthropic(ctx, model, key, image, mediaType)
	default:
		err = fmt.Errorf("recognize: %w: unknown provider %q", Failed(), provider)
	}
	if err != nil {
		return Result{}, err
	}
	var r Result
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		return Result{}, fmt.Errorf("%s: %w: decode answer: %w", provider, Failed(), err)
	}
	return Result{
		Name:        clean(r.Name, maxNameLength),
		Brand:       clean(r.Brand, maxBrandLength),
		PackageSize: clean(r.PackageSize, maxPackageSizeLength),
	}, nil
}

// clean trims s and cuts it to max characters; spaces left at the end of a
// cut value are removed. An empty result is nil.
func clean(s *string, max int) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if utf8.RuneCountInString(v) > max {
		v = strings.TrimSpace(string([]rune(v)[:max]))
	}
	if v == "" {
		return nil
	}
	return &v
}

// do sends a request with the JSON of body (nil for none) to url within the
// time limit and decodes a 200 answer into out (nil to ignore it). key goes
// only into the authentication header of provider. The answer of the
// provider never goes into an error, because it can repeat parts of the key.
func (c *Client) do(ctx context.Context, provider, method, url, key string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: %w: encode request: %w", provider, Failed(), err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("%s: %w: build request: %w", provider, Failed(), err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent)
	switch provider {
	case OpenAI:
		req.Header.Set("Authorization", "Bearer "+key)
	case Gemini:
		req.Header.Set("x-goog-api-key", key)
	case Anthropic:
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w: %w", provider, Failed(), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%s: %w: read answer: %w", provider, Failed(), err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden,
		provider == Gemini && resp.StatusCode == http.StatusBadRequest && geminiKeyInvalid(data):
		return fmt.Errorf("%s: %w: status %d", provider, InvalidAPIKey(), resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%s: %w: status %d", provider, Failed(), resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%s: %w: decode response: %w", provider, Failed(), err)
		}
	}
	return nil
}
