package recognize_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/recognize"
)

// testKey is the API key of the tests. The fake providers repeat it in
// their error answers, as OpenAI does in part.
const testKey = "sk-test-secret-key-abcd"

// testImage stands in for a PNG photo.
var testImage = []byte("\x89PNG\r\n\x1a\nphoto of a can")

// request is a request received by a fake provider.
type request struct {
	method, path, rawQuery string
	header                 http.Header
	body                   map[string]any
}

// fakeProvider answers every request with status and body, after waiting
// for delay or the end of the request, and records the last request.
type fakeProvider struct {
	*httptest.Server
	status int
	body   string
	delay  time.Duration

	mu  sync.Mutex
	got *request
}

func newFakeProvider(t *testing.T, status int, body string) *fakeProvider {
	t.Helper()
	f := &fakeProvider{status: status, body: body}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got := &request{method: r.Method, path: r.URL.Path, rawQuery: r.URL.RawQuery, header: r.Header.Clone()}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &got.body); err != nil {
				t.Errorf("request body is no JSON object: %v", err)
			}
		}
		f.mu.Lock()
		f.got = got
		f.mu.Unlock()
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		io.WriteString(w, f.body)
	}))
	t.Cleanup(f.Close)
	return f
}

// request returns the last request, failing the test without one.
func (f *fakeProvider) request(t *testing.T) *request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.got == nil {
		t.Fatal("the provider got no request")
	}
	return f.got
}

// client returns a Client that sends all requests to f with timeout.
func (f *fakeProvider) client(timeout time.Duration) *recognize.Client {
	return recognize.NewClient(f.Client(), recognize.Endpoints{OpenAI: f.URL, Gemini: f.URL, Anthropic: f.URL}, timeout, "StashBert/test")
}

// at returns the value at path in v, a decoded JSON value; strings select
// object members, ints array elements.
func at(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, p := range path {
		switch p := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("at %v: %v is no object", path, v)
			}
			v = m[p]
		case int:
			a, ok := v.([]any)
			if !ok || p >= len(a) {
				t.Fatalf("at %v: %v is no array with index %d", path, v, p)
			}
			v = a[p]
		}
	}
	return v
}

// decode returns the JSON value of s.
func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

// checkAt asserts that the value at path in v equals the JSON want.
func checkAt(t *testing.T, v any, want string, path ...any) {
	t.Helper()
	if got := at(t, v, path...); !reflect.DeepEqual(got, decode(t, want)) {
		t.Errorf("%v = %#v, want %s", path, got, want)
	}
}

// quote returns s as a JSON string.
func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// provider describes the API of a provider for the tests.
type provider struct {
	name string
	// answer returns a response body with the text of the answer.
	answer func(text string) string
	// noAnswer is a response body without usable text.
	noAnswer string
	// modelsPath is the path of the model list.
	modelsPath string
	// check asserts the provider specific parts of a recognition request.
	check func(t *testing.T, r *request)
}

// fieldsJSON are the fields of the answer in their order.
const fieldsJSON = `["name", "brand", "package_size"]`

var providers = []provider{
	{
		name: recognize.OpenAI,
		answer: func(text string) string {
			return `{"id": "resp_1", "object": "response", "status": "completed", "output": [
				{"type": "reasoning", "id": "rs_1", "summary": []},
				{"type": "message", "id": "msg_1", "status": "completed", "role": "assistant",
				 "content": [{"type": "output_text", "text": ` + quote(text) + `, "annotations": []}]}]}`
		},
		noAnswer: `{"id": "resp_1", "status": "completed", "output": [{"type": "message", "role": "assistant",
			"content": [{"type": "refusal", "refusal": "I cannot help with that."}]}]}`,
		modelsPath: "/v1/models",
		check: func(t *testing.T, r *request) {
			if r.path != "/v1/responses" {
				t.Errorf("path = %s, want /v1/responses", r.path)
			}
			if got := r.header.Get("Authorization"); got != "Bearer "+testKey {
				t.Errorf("Authorization = %q, want the key as bearer token", got)
			}
			checkAt(t, r.body, `"test-model"`, "model")
			checkAt(t, r.body, `false`, "store")
			checkAt(t, r.body, `"user"`, "input", 0, "role")
			checkAt(t, r.body, `"input_text"`, "input", 0, "content", 0, "type")
			checkPrompt(t, at(t, r.body, "input", 0, "content", 0, "text"))
			checkAt(t, r.body, `"input_image"`, "input", 0, "content", 1, "type")
			checkAt(t, r.body, quote("data:image/png;base64,"+base64.StdEncoding.EncodeToString(testImage)), "input", 0, "content", 1, "image_url")
			format := at(t, r.body, "text", "format")
			checkAt(t, format, `"json_schema"`, "type")
			checkAt(t, format, `true`, "strict")
			checkAt(t, format, `"object"`, "schema", "type")
			checkAt(t, format, `false`, "schema", "additionalProperties")
			checkAt(t, format, fieldsJSON, "schema", "required")
			for _, f := range []string{"name", "brand", "package_size"} {
				checkAt(t, format, `["string", "null"]`, "schema", "properties", f, "type")
			}
		},
	},
	{
		name: recognize.Gemini,
		answer: func(text string) string {
			return `{"candidates": [{"content": {"role": "model", "parts": [{"text": ` + quote(text) + `}]},
				"finishReason": "STOP", "index": 0}], "modelVersion": "test-model"}`
		},
		noAnswer:   `{"promptFeedback": {"blockReason": "SAFETY"}}`,
		modelsPath: "/v1beta/models",
		check: func(t *testing.T, r *request) {
			if r.path != "/v1beta/models/test-model:generateContent" {
				t.Errorf("path = %s, want /v1beta/models/test-model:generateContent", r.path)
			}
			if got := r.header.Get("x-goog-api-key"); got != testKey {
				t.Errorf("x-goog-api-key = %q, want the key", got)
			}
			checkAt(t, r.body, `"image/png"`, "contents", 0, "parts", 0, "inline_data", "mime_type")
			checkAt(t, r.body, quote(base64.StdEncoding.EncodeToString(testImage)), "contents", 0, "parts", 0, "inline_data", "data")
			checkPrompt(t, at(t, r.body, "contents", 0, "parts", 1, "text"))
			config := at(t, r.body, "generationConfig")
			checkAt(t, config, `"application/json"`, "responseMimeType")
			checkAt(t, config, `"OBJECT"`, "responseSchema", "type")
			checkAt(t, config, fieldsJSON, "responseSchema", "required")
			for _, f := range []string{"name", "brand", "package_size"} {
				checkAt(t, config, `"STRING"`, "responseSchema", "properties", f, "type")
				checkAt(t, config, `true`, "responseSchema", "properties", f, "nullable")
			}
		},
	},
	{
		name: recognize.Anthropic,
		answer: func(text string) string {
			return `{"id": "msg_1", "type": "message", "role": "assistant", "model": "test-model",
				"content": [{"type": "text", "text": ` + quote(text) + `}], "stop_reason": "end_turn"}`
		},
		noAnswer:   `{"id": "msg_1", "type": "message", "content": [], "stop_reason": "refusal"}`,
		modelsPath: "/v1/models",
		check: func(t *testing.T, r *request) {
			if r.path != "/v1/messages" {
				t.Errorf("path = %s, want /v1/messages", r.path)
			}
			if got := r.header.Get("x-api-key"); got != testKey {
				t.Errorf("x-api-key = %q, want the key", got)
			}
			if got := r.header.Get("anthropic-version"); got != "2023-06-01" {
				t.Errorf("anthropic-version = %q, want 2023-06-01", got)
			}
			checkAt(t, r.body, `"test-model"`, "model")
			checkAt(t, r.body, `1024`, "max_tokens")
			checkAt(t, r.body, `"user"`, "messages", 0, "role")
			checkAt(t, r.body, `{"type": "base64", "media_type": "image/png", "data": `+quote(base64.StdEncoding.EncodeToString(testImage))+`}`,
				"messages", 0, "content", 0, "source")
			checkAt(t, r.body, `"image"`, "messages", 0, "content", 0, "type")
			checkAt(t, r.body, `"text"`, "messages", 0, "content", 1, "type")
			checkPrompt(t, at(t, r.body, "messages", 0, "content", 1, "text"))
			format := at(t, r.body, "output_config", "format")
			checkAt(t, format, `"json_schema"`, "type")
			checkAt(t, format, `"object"`, "schema", "type")
			checkAt(t, format, `false`, "schema", "additionalProperties")
			checkAt(t, format, fieldsJSON, "schema", "required")
			for _, f := range []string{"name", "brand", "package_size"} {
				checkAt(t, format, `[{"type": "string"}, {"type": "null"}]`, "schema", "properties", f, "anyOf")
			}
		},
	},
}

// checkPrompt asserts that the instruction asks for null instead of
// guessing.
func checkPrompt(t *testing.T, v any) {
	t.Helper()
	text, _ := v.(string)
	for _, want := range []string{"name", "brand", "package size", `"500 g"`, "null", "Never guess"} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt %q does not contain %q", text, want)
		}
	}
}

// checkRequest asserts the common parts of a request: method, content
// type, user agent and no key in the URL.
func checkRequest(t *testing.T, r *request, method string) {
	t.Helper()
	if r.method != method {
		t.Errorf("method = %s, want %s", r.method, method)
	}
	if r.rawQuery != "" || strings.Contains(r.path, testKey) {
		t.Errorf("URL %s?%s, want no query and no key", r.path, r.rawQuery)
	}
	if got := r.header.Get("User-Agent"); got != "StashBert/test" {
		t.Errorf("User-Agent = %q, want StashBert/test", got)
	}
	if method == http.MethodPost && r.header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", r.header.Get("Content-Type"))
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ptr returns a pointer to s.
func ptr(s string) *string { return &s }

func TestRecognize(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		want   recognize.Result
	}{
		{
			name:   "all fields",
			answer: `{"name": " Kidneybohnen rot ", "brand": "Kaufland", "package_size": "400 g"}`,
			want:   recognize.Result{Name: ptr("Kidneybohnen rot"), Brand: ptr("Kaufland"), PackageSize: ptr("400 g")},
		},
		{
			name:   "null and empty fields",
			answer: `{"name": "Kidneybohnen", "brand": null, "package_size": "  "}`,
			want:   recognize.Result{Name: ptr("Kidneybohnen")},
		},
		{
			name:   "nothing readable",
			answer: `{"name": null, "brand": null, "package_size": null}`,
		},
	}
	for _, p := range providers {
		for _, tt := range tests {
			t.Run(p.name+"/"+tt.name, func(t *testing.T) {
				f := newFakeProvider(t, http.StatusOK, p.answer(tt.answer))

				got, err := f.client(5*time.Second).Recognize(testContext(t), p.name, "test-model", testKey, testImage, "image/png")

				if err != nil {
					t.Fatalf("Recognize: %v", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("result = %s, want %s", show(got), show(tt.want))
				}
				r := f.request(t)
				checkRequest(t, r, http.MethodPost)
				p.check(t, r)
			})
		}
	}
}

// show returns r as JSON for messages.
func show(r recognize.Result) string {
	b, _ := json.Marshal(r)
	return string(b)
}

func TestRecognizeCutsToLimits(t *testing.T) {
	answer := `{"name": ` + quote(strings.Repeat("ä", 119)+" Bohnen") + `, "brand": ` + quote(strings.Repeat("b", 130)) +
		`, "package_size": ` + quote("  "+strings.Repeat("9", 39)+" g  ") + `}`
	f := newFakeProvider(t, http.StatusOK, providers[0].answer(answer))

	got, err := f.client(5*time.Second).Recognize(testContext(t), recognize.OpenAI, "test-model", testKey, testImage, "image/png")

	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	// Cut to 120 and 40 characters (not bytes); a space left at the end is
	// removed.
	want := recognize.Result{Name: ptr(strings.Repeat("ä", 119)), Brand: ptr(strings.Repeat("b", 120)), PackageSize: ptr(strings.Repeat("9", 39))}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("result = %s, want %s", show(got), show(want))
	}
}

// checkCode asserts that err wraps an *httpx.Error with code and does not
// contain the key.
func checkCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *httpx.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Errorf("error = %v, want one with code %s", err, code)
	}
	if err != nil && strings.Contains(err.Error(), testKey) {
		t.Errorf("error %q contains the key", err)
	}
}

func TestRecognizeErrors(t *testing.T) {
	// The answers repeat the key, as OpenAI does in part.
	echo := `{"error": {"message": "Incorrect API key provided: ` + testKey + `"}}`
	for _, p := range providers {
		tests := []struct {
			name   string
			status int
			body   string
			delay  time.Duration
			want   string
		}{
			{"401", http.StatusUnauthorized, echo, 0, "invalid_api_key"},
			{"403", http.StatusForbidden, echo, 0, "invalid_api_key"},
			{"500", http.StatusInternalServerError, echo, 0, "recognition_failed"},
			{"timeout", http.StatusOK, p.answer(`{"name": null, "brand": null, "package_size": null}`), 5 * time.Second, "recognition_failed"},
			{"no answer", http.StatusOK, p.noAnswer, 0, "recognition_failed"},
			{"answer is no JSON", http.StatusOK, p.answer("Kidneybohnen, 400 g"), 0, "recognition_failed"},
			{"response is no JSON", http.StatusOK, "<html>", 0, "recognition_failed"},
		}
		for _, tt := range tests {
			t.Run(p.name+"/"+tt.name, func(t *testing.T) {
				f := newFakeProvider(t, tt.status, tt.body)
				f.delay = tt.delay

				start := time.Now()
				_, err := f.client(200*time.Millisecond).Recognize(testContext(t), p.name, "test-model", testKey, testImage, "image/png")

				checkCode(t, err, tt.want)
				if d := time.Since(start); d > 2*time.Second {
					t.Errorf("Recognize took %v, want it to end after the timeout of 200 ms", d)
				}
			})
		}
	}
}

func TestCheckKey(t *testing.T) {
	echo := `{"error": {"message": "Incorrect API key provided: ` + testKey + `"}}`
	geminiInvalid := `{"error": {"code": 400, "message": "API key not valid. Please pass a valid API key.", "status": "INVALID_ARGUMENT",
		"details": [{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "API_KEY_INVALID", "domain": "googleapis.com"}]}}`
	for _, p := range providers {
		tests := []struct {
			name   string
			status int
			body   string
			want   string // "" for no error
		}{
			{"valid", http.StatusOK, `{"data": [], "models": []}`, ""},
			{"401", http.StatusUnauthorized, echo, "invalid_api_key"},
			{"403", http.StatusForbidden, echo, "invalid_api_key"},
			{"500", http.StatusInternalServerError, echo, "recognition_failed"},
			{"400", http.StatusBadRequest, `{"error": {"code": 400, "status": "INVALID_ARGUMENT"}}`, "recognition_failed"},
		}
		// Gemini answers an invalid key with 400 and the reason
		// API_KEY_INVALID.
		wantInvalid := "recognition_failed"
		if p.name == recognize.Gemini {
			wantInvalid = "invalid_api_key"
		}
		tests = append(tests, struct {
			name   string
			status int
			body   string
			want   string
		}{"400 API_KEY_INVALID", http.StatusBadRequest, geminiInvalid, wantInvalid})
		for _, tt := range tests {
			t.Run(p.name+"/"+tt.name, func(t *testing.T) {
				f := newFakeProvider(t, tt.status, tt.body)

				err := f.client(5*time.Second).CheckKey(testContext(t), p.name, testKey)

				if tt.want == "" {
					if err != nil {
						t.Errorf("CheckKey: %v", err)
					}
				} else {
					checkCode(t, err, tt.want)
				}
				r := f.request(t)
				checkRequest(t, r, http.MethodGet)
				if r.path != p.modelsPath {
					t.Errorf("path = %s, want %s", r.path, p.modelsPath)
				}
				header := map[string]string{recognize.OpenAI: "Authorization", recognize.Gemini: "x-goog-api-key", recognize.Anthropic: "x-api-key"}[p.name]
				if got := r.header.Get(header); !strings.HasSuffix(got, testKey) {
					t.Errorf("%s = %q, want the key", header, got)
				}
			})
		}
	}
}

func TestCheckKeyTimeout(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, `{"data": []}`)
	f.delay = 5 * time.Second

	err := f.client(200*time.Millisecond).CheckKey(testContext(t), recognize.OpenAI, testKey)

	checkCode(t, err, "recognition_failed")
}
