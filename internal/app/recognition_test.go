package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/recognize"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// validAPIKey is the only key the fake providers accept; its last four
// characters are 9876.
const validAPIKey = "sk-test-valid-key-9876"

// fakeAI imitates the model lists of the three providers and the Responses
// API of OpenAI. It accepts only validAPIKey and repeats a rejected key in
// its answer, as OpenAI does in part.
type fakeAI struct {
	*httptest.Server
	mu sync.Mutex
	// status is the status of every answer to a valid key.
	status int
	// answer is the text of the model answer.
	answer string
	// images are the image data URLs of the recognition requests.
	images []string
	// delay is the time a recognition request or a key check waits before
	// its answer.
	delay time.Duration
}

func newFakeAI(t *testing.T) *fakeAI {
	t.Helper()
	f := &fakeAI{status: http.StatusOK, answer: `{"name": "Kidneybohnen", "brand": "Bonduelle", "package_size": null}`}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + r.Header.Get("x-goog-api-key") + r.Header.Get("x-api-key")
		switch {
		case key != validAPIKey:
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error": {"message": "Incorrect API key provided: `+key+`"}}`)
		case f.status != http.StatusOK:
			w.WriteHeader(f.status)
		case r.Method == http.MethodGet:
			time.Sleep(f.delay)
			io.WriteString(w, `{"object": "list", "data": [], "models": []}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
			var body struct {
				Input []struct {
					Content []struct {
						ImageURL string `json:"image_url"`
					} `json:"content"`
				} `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Input) != 1 || len(body.Input[0].Content) != 2 {
				t.Errorf("unexpected request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.images = append(f.images, body.Input[0].Content[1].ImageURL)
			time.Sleep(f.delay)
			answer, _ := json.Marshal(f.answer)
			io.WriteString(w, `{"status": "completed", "output": [{"type": "message", "content": [{"type": "output_text", "text": `+string(answer)+`}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// setStatus sets the status of the answers to a valid key.
func (f *fakeAI) setStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

// recognitionApp is a handler from app.NewHandler whose recognition asks
// fake, and everything it wrote into its log and its responses.
type recognitionApp struct {
	h        http.Handler
	db       *sql.DB
	imageDir string
	fake     *fakeAI
	log      *bytes.Buffer
	// bodies are the bodies of all responses sent through send.
	bodies []string
}

func newRecognitionApp(t *testing.T) *recognitionApp {
	t.Helper()
	a := &recognitionApp{imageDir: t.TempDir(), fake: newFakeAI(t), log: &bytes.Buffer{}}
	endpoints := recognize.Endpoints{OpenAI: a.fake.URL, Gemini: a.fake.URL, Anthropic: a.fake.URL}
	a.h, a.db = newAppWithDeps(t, app.Deps{
		Publisher: events.Nop{}, Lookuper: lookup.NewDisabledClient(), ImageDir: a.imageDir,
		Logger:     slog.New(slog.NewJSONHandler(a.log, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Recognizer: recognize.NewClient(a.fake.Client(), endpoints, 5*time.Second, "StashBert/test"),
	})
	// Neither the log nor a response may contain the key (ADR-0021).
	t.Cleanup(func() {
		if strings.Contains(a.log.String(), validAPIKey) {
			t.Errorf("the log contains the API key: %s", a.log.String())
		}
		for _, body := range a.bodies {
			if strings.Contains(body, validAPIKey) {
				t.Errorf("a response contains the API key: %s", body)
			}
		}
	})
	return a
}

// send sends a request with the JSON body (none if empty) to the app and
// returns the response.
func (a *recognitionApp) send(method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	a.h.ServeHTTP(rec, req)
	a.bodies = append(a.bodies, rec.Body.String())
	return rec
}

// checkOK asserts that rec is a 200 JSON response with the body want.
func checkOK(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if !jsonEqual(t, rec.Body.String(), want) {
		t.Errorf("body = %s, want %s", rec.Body.String(), want)
	}
}

// checkCode asserts that rec is a problem with status and code.
func checkCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var p struct{ Code string }
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if rec.Code != status || p.Code != code || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("response = %d %s, want a problem %d %s", rec.Code, rec.Body.String(), status, code)
	}
}

// recognitionSettings returns the RecognitionSettings JSON with provider,
// model and keyHint (JSON values) and the default models.
func recognitionSettings(provider, model, keyHint string) string {
	return `{"provider": ` + provider + `, "model": ` + model + `, "key_set": ` + map[bool]string{true: "true", false: "false"}[keyHint != "null"] +
		`, "key_hint": ` + keyHint + `, "default_models": {"openai": "gpt-6-luna", "gemini": "gemini-3.5-flash-lite", "anthropic": "claude-sonnet-5"}}`
}

func TestRecognitionSettings(t *testing.T) {
	a := newRecognitionApp(t)
	const path = "/api/v1/integrations/recognition"

	checkOK(t, a.send(http.MethodGet, path, ""), recognitionSettings("null", "null", "null"))

	// A new key is checked with the model list and then stored.
	checkOK(t, a.send(http.MethodPut, path, `{"provider": "openai", "api_key": " `+validAPIKey+` "}`),
		recognitionSettings(`"openai"`, "null", `"9876"`))
	checkOK(t, a.send(http.MethodGet, path, ""), recognitionSettings(`"openai"`, "null", `"9876"`))
	if got := storedSettings(t, a.db); got[recognize.SettingAPIKey] != validAPIKey || got["recognition_provider"] != "openai" || len(got) != 2 {
		t.Errorf("stored settings = %v, want provider and trimmed key", got)
	}

	// Without key, the stored key stays for the same provider.
	checkOK(t, a.send(http.MethodPut, path, `{"provider": "openai", "model": " gpt-test "}`),
		recognitionSettings(`"openai"`, `"gpt-test"`, `"9876"`))
	// An empty model means the default model; an empty key means no key.
	checkOK(t, a.send(http.MethodPut, path, `{"provider": "openai", "model": "", "api_key": ""}`),
		recognitionSettings(`"openai"`, "null", `"9876"`))

	// A new provider needs a new key.
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "gemini"}`), http.StatusBadRequest, "invalid_request")
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "anthropic", "api_key": " "}`), http.StatusBadRequest, "invalid_request")
	// A key that the provider rejects is not stored.
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "gemini", "api_key": "sk-wrong-key"}`), http.StatusUnprocessableEntity, "invalid_api_key")
	// An unreachable provider is recognition_failed.
	a.fake.setStatus(http.StatusInternalServerError)
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "anthropic", "api_key": "`+validAPIKey+`"}`), http.StatusBadGateway, "recognition_failed")
	a.fake.setStatus(http.StatusOK)
	// An unknown provider fails at the validator.
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "mistral", "api_key": "`+validAPIKey+`"}`), http.StatusBadRequest, "invalid_request")
	checkOK(t, a.send(http.MethodGet, path, ""), recognitionSettings(`"openai"`, "null", `"9876"`))

	// Another provider with its key.
	checkOK(t, a.send(http.MethodPut, path, `{"provider": "anthropic", "model": "claude-test", "api_key": "`+validAPIKey+`"}`),
		recognitionSettings(`"anthropic"`, `"claude-test"`, `"9876"`))

	// provider null removes all three settings.
	checkOK(t, a.send(http.MethodPut, path, `{"provider": null, "model": "x", "api_key": "`+validAPIKey+`"}`),
		recognitionSettings("null", "null", "null"))
	if got := storedSettings(t, a.db); len(got) != 0 {
		t.Errorf("stored settings = %v, want none", got)
	}
	checkOK(t, a.send(http.MethodGet, path, ""), recognitionSettings("null", "null", "null"))
	// Without a stored key, the provider needs one.
	checkCode(t, a.send(http.MethodPut, path, `{"provider": "openai"}`), http.StatusBadRequest, "invalid_request")
}

// productJSON returns GET /api/v1/products/{id} of a.
func (a *recognitionApp) productJSON(t *testing.T, id string) string {
	t.Helper()
	rec := a.send(http.MethodGet, "/api/v1/products/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET product %s: status %d", id, rec.Code)
	}
	return rec.Body.String()
}

func TestRecognizeProduct(t *testing.T) {
	a := newRecognitionApp(t)
	insertProducts(t, a.db)
	// p2 has the image file p2.jpg, p1 none, p4 one without file.
	photo := testJPEG(t)
	writeFile(t, a.imageDir, "p2.jpg", photo)
	setImageFile(t, a.db, "p4", "p4.jpg")
	recognition := func(id string) *httptest.ResponseRecorder {
		return a.send(http.MethodPost, "/api/v1/products/"+id+"/recognition", "")
	}

	checkCode(t, recognition("p2"), http.StatusConflict, "recognition_disabled")
	checkCode(t, recognition("unknown"), http.StatusNotFound, "not_found")

	checkOK(t, a.send(http.MethodPut, "/api/v1/integrations/recognition", `{"provider": "openai", "api_key": "`+validAPIKey+`"}`),
		recognitionSettings(`"openai"`, "null", `"9876"`))
	checkCode(t, recognition("unknown"), http.StatusNotFound, "not_found")
	checkCode(t, recognition("p1"), http.StatusConflict, "no_image")
	checkCode(t, recognition("p4"), http.StatusConflict, "no_image")

	before := a.productJSON(t, "p2")
	checkOK(t, recognition("p2"), `{"name": "Kidneybohnen", "brand": "Bonduelle", "package_size": null}`)
	if after := a.productJSON(t, "p2"); after != before {
		t.Errorf("product after the recognition = %s, want it unchanged %s", after, before)
	}
	a.fake.mu.Lock()
	images := a.fake.images
	a.fake.mu.Unlock()
	if want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(photo); len(images) != 1 || images[0] != want {
		t.Errorf("the provider got %d images, want the photo of p2 as a data URL", len(images))
	}

	a.fake.setStatus(http.StatusUnauthorized)
	checkCode(t, recognition("p2"), http.StatusUnprocessableEntity, "invalid_api_key")
	a.fake.setStatus(http.StatusInternalServerError)
	checkCode(t, recognition("p2"), http.StatusBadGateway, "recognition_failed")
	if !strings.Contains(a.log.String(), "recognition provider request failed") {
		t.Errorf("log %s, want the failed provider requests", a.log.String())
	}
}

func TestBackupWithoutAPIKey(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	a := newRecognitionApp(t)
	checkOK(t, a.send(http.MethodPut, "/api/v1/integrations/recognition", `{"provider": "openai", "api_key": "`+validAPIKey+`"}`),
		recognitionSettings(`"openai"`, "null", `"9876"`))

	_, files := unpackBackup(t, bytes.NewReader(downloadFrom(t, a.h)))

	data := files["stashbert.db"].data
	if len(data) == 0 {
		t.Fatal("the archive has no stashbert.db")
	}
	if bytes.Contains(data, []byte(validAPIKey)) {
		t.Error("stashbert.db in the archive contains the API key")
	}
	path := filepath.Join(t.TempDir(), "stashbert.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	copyDB, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer copyDB.Close()
	if got := storedSettings(t, copyDB); len(got) != 1 || got["recognition_provider"] != "openai" {
		t.Errorf("settings in the archive = %v, want only the provider", got)
	}
	// The running database keeps the key.
	if got := storedSettings(t, a.db); got[recognize.SettingAPIKey] != validAPIKey {
		t.Errorf("settings in the running database = %v, want the key", got)
	}
	checkOK(t, a.send(http.MethodGet, "/api/v1/integrations/recognition", ""), recognitionSettings(`"openai"`, "null", `"9876"`))
}

func TestRecognizeProductOutlastsWriteTimeout(t *testing.T) {
	a := newRecognitionApp(t)
	insertProducts(t, a.db)
	writeFile(t, a.imageDir, "p2.jpg", testJPEG(t))
	checkOK(t, a.send(http.MethodPut, "/api/v1/integrations/recognition", `{"provider": "openai", "api_key": "`+validAPIKey+`"}`),
		recognitionSettings(`"openai"`, "null", `"9876"`))
	// The provider answers after the WriteTimeout of the server; the route
	// extends the write deadline (architecture.md, 4.4).
	a.fake.mu.Lock()
	a.fake.delay = 500 * time.Millisecond
	a.fake.mu.Unlock()
	srv := httptest.NewUnstartedServer(a.h)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Post(srv.URL+"/api/v1/products/p2/recognition", "", nil)
	if err != nil {
		t.Fatalf("POST recognition: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !jsonEqual(t, string(body), `{"name": "Kidneybohnen", "brand": "Bonduelle", "package_size": null}`) {
		t.Errorf("answer = %d %s, want 200 with the result", resp.StatusCode, body)
	}
}

func TestSetRecognitionSettingsOutlastsWriteTimeout(t *testing.T) {
	a := newRecognitionApp(t)
	// The key check answers after the WriteTimeout of the server; the route
	// extends the write deadline (architecture.md, 4.4).
	a.fake.mu.Lock()
	a.fake.delay = 500 * time.Millisecond
	a.fake.mu.Unlock()
	srv := httptest.NewUnstartedServer(a.h)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/integrations/recognition",
		strings.NewReader(`{"provider": "openai", "api_key": "`+validAPIKey+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("PUT recognition settings: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read answer: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !jsonEqual(t, string(body), recognitionSettings(`"openai"`, "null", `"9876"`)) {
		t.Errorf("answer = %d %s, want 200 with the settings", resp.StatusCode, body)
	}
}
