package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/schmitz-chris/stashbert/internal/httpx"
)

type problemBody struct {
	Type, Title, Detail, Code string
	Status                    int
}

// checkProblem asserts that rec holds a problem with status, code and detail.
func checkProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code, detail string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d", rec.Code, status)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p problemBody
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	want := problemBody{Type: "about:blank", Title: http.StatusText(status), Detail: detail, Code: code, Status: status}
	if p != want {
		t.Errorf("body = %+v, want %+v", p, want)
	}
}

func TestResponseErrorHandler(t *testing.T) {
	t.Run("httpx.Error", func(t *testing.T) {
		var logs bytes.Buffer
		handle := responseErrorHandler(slog.New(slog.NewJSONHandler(&logs, nil)))

		rec := httptest.NewRecorder()
		handle(rec, httptest.NewRequest(http.MethodPost, "/products", nil), httpx.NewError(http.StatusConflict, "barcode_in_use", "Barcode ist vergeben"))

		checkProblem(t, rec, http.StatusConflict, "barcode_in_use", "Barcode ist vergeben")
		if logs.Len() != 0 {
			t.Errorf("unexpected log: %s", logs.String())
		}
	})

	t.Run("other error", func(t *testing.T) {
		var logs bytes.Buffer
		handle := responseErrorHandler(slog.New(slog.NewJSONHandler(&logs, nil)))

		rec := httptest.NewRecorder()
		handle(rec, httptest.NewRequest(http.MethodGet, "/products", nil), errors.New("database is locked"))

		checkProblem(t, rec, http.StatusInternalServerError, "internal", "")
		if !strings.Contains(logs.String(), `"level":"ERROR"`) || !strings.Contains(logs.String(), "database is locked") {
			t.Errorf("log = %q, want error entry with the cause", logs.String())
		}
	})
}

func TestValidationErrorHandler(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
		detail string
	}{
		{"method not allowed", routers.ErrMethodNotAllowed, http.StatusMethodNotAllowed, "method_not_allowed", ""},
		{"path not found", routers.ErrPathNotFound, http.StatusNotFound, "not_found", ""},
		{"invalid request", errors.New("request body has an error"), http.StatusBadRequest, "invalid_request", "request body has an error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			validationErrorHandler(context.Background(), tt.err, rec, httptest.NewRequest(http.MethodGet, "/", nil), nethttpmiddleware.ErrorHandlerOpts{})

			checkProblem(t, rec, tt.status, tt.code, tt.detail)
		})
	}
}

func TestRequestErrorHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	requestErrorHandler(rec, httptest.NewRequest(http.MethodPost, "/", nil), errors.New("can't decode JSON body"))

	checkProblem(t, rec, http.StatusBadRequest, "invalid_request", "can't decode JSON body")
}

func TestPanicReturnsInternal(t *testing.T) {
	var logs bytes.Buffer
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h, err := newChain(slog.New(slog.NewJSONHandler(&logs, nil)), panicking, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("newChain: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	checkProblem(t, rec, http.StatusInternalServerError, "internal", "")
	var entry struct{ Level, Panic, Stack string }
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode log %q: %v", logs.String(), err)
	}
	if entry.Level != "ERROR" || entry.Panic != "boom" || !strings.Contains(entry.Stack, "goroutine") {
		t.Errorf("log = %s, want error entry with panic value and stack trace", logs.String())
	}
}

func TestWebPanicReturnsInternal(t *testing.T) {
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h, err := newChain(slog.New(slog.DiscardHandler), http.NotFoundHandler(), panicking)
	if err != nil {
		t.Fatalf("newChain: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/vorrat", nil))

	checkProblem(t, rec, http.StatusInternalServerError, "internal", "")
}
