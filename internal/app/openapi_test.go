package app_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
)

func TestServeOpenAPISpec(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	h := newHandler(t)

	rec := get(h, "/api/v1/openapi.yaml")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Errorf("body differs from api/openapi.yaml (%d bytes, want %d)", rec.Body.Len(), len(want))
	}
}

func TestOpenAPISpecOtherMethodsReachValidator(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/openapi.yaml", nil))

	checkProblemCode(t, rec, http.StatusNotFound, "not_found")
}

func TestOpenAPISpecIsLogged(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newAppWithDeps(t, app.Deps{
		Logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		Publisher: events.Nop{},
		Lookuper:  lookup.NewDisabledClient(),
		ImageDir:  t.TempDir(),
	})

	get(h, "/api/v1/openapi.yaml")

	var entry struct {
		Msg, Method, Path string
		Status            int
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode log %q: %v", logs.String(), err)
	}
	if entry.Msg != "http request" || entry.Method != http.MethodGet || entry.Path != "/api/v1/openapi.yaml" || entry.Status != http.StatusOK {
		t.Errorf("log = %s, want http request entry for GET /api/v1/openapi.yaml with status 200", logs.String())
	}
}
