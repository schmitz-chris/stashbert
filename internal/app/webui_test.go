package app_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
)

const webIndex = `<!doctype html><html><body><script>console.log(1)</script></body></html>`

// newAppWithWebUI is like newApp but serves index.html from a fstest.MapFS
// as the web UI and logs into logs.
func newAppWithWebUI(t *testing.T, logs *bytes.Buffer) http.Handler {
	t.Helper()
	h, _ := newAppWithDeps(t, app.Deps{
		Logger:    slog.New(slog.NewJSONHandler(logs, nil)),
		Publisher: events.Nop{},
		Lookuper:  lookup.NewDisabledClient(),
		ImageDir:  t.TempDir(),
		WebUI:     fstest.MapFS{"index.html": {Data: []byte(webIndex)}},
	})
	return h
}

func TestWebUIServedUnderRoot(t *testing.T) {
	var logs bytes.Buffer
	h := newAppWithWebUI(t, &logs)

	for _, target := range []string{"/", "/vorrat"} {
		rec := get(h, target)

		if rec.Code != http.StatusOK || rec.Body.String() != webIndex {
			t.Errorf("%s: status = %d, body = %q, want 200 and index.html", target, rec.Code, rec.Body.String())
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' 'wasm-unsafe-eval' 'sha256-") {
			t.Errorf("%s: Content-Security-Policy = %q, want script-src with a hash", target, csp)
		}
	}
	var entry struct{ Msg, Path string }
	if err := json.NewDecoder(&logs).Decode(&entry); err != nil {
		t.Fatalf("decode log %q: %v", logs.String(), err)
	}
	if entry.Msg != "http request" || entry.Path != "/" {
		t.Errorf("log = %+v, want http request entry for /", entry)
	}
}

func TestWebUIKeepsAPI(t *testing.T) {
	var logs bytes.Buffer
	h := newAppWithWebUI(t, &logs)

	t.Run("health", func(t *testing.T) {
		rec := get(h, "/api/v1/health")

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got, want := strings.TrimSpace(rec.Body.String()), `{"status":"ok","version":"dev"}`; got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})
	t.Run("openapi.yaml", func(t *testing.T) {
		rec := get(h, "/api/v1/openapi.yaml")

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
			t.Errorf("Content-Type = %q, want application/yaml", ct)
		}
	})
	t.Run("unknown v1 path", func(t *testing.T) {
		checkProblemCode(t, get(h, "/api/v1/unbekannt"), http.StatusNotFound, "not_found")
	})
	t.Run("other API version", func(t *testing.T) {
		rec := get(h, "/api/v2/x")

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "html") || strings.Contains(rec.Body.String(), "<") {
			t.Errorf("Content-Type = %q, body = %q, want no HTML", ct, rec.Body.String())
		}
	})
}

func TestWebUIEmbeddedByDefault(t *testing.T) {
	rec := get(newHandler(t), "/")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy is missing")
	}
}
