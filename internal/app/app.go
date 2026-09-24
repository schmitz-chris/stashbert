// Package app is the composition root: it assembles the complete HTTP handler chain.
package app

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	apispec "github.com/schmitz-chris/stashbert/api"
	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/webui"
)

// Deps holds the dependencies created in main.
type Deps struct {
	Logger  *slog.Logger
	Version string
	DB      *sql.DB
	// Publisher receives the domain events after each commit.
	Publisher events.Publisher
	// Lookuper looks up unknown barcodes booked with add or marked for
	// shopping (architecture.md, 7.2).
	Lookuper domain.Lookuper
	// ImageDir is the directory of the product image files, DATA_DIR/images
	// (architecture.md, 7.3).
	ImageDir string
	// WebUI holds the built web UI served under / (architecture.md, 4.2).
	// nil means the web UI embedded in the binary (webui.Dist).
	WebUI fs.FS
	// Snapshots takes the requests of POST /shopping-list/snapshot
	// (architecture.md, 11.3). It is nil without MQTT; the endpoint then
	// answers 409 mqtt_disabled.
	Snapshots api.SnapshotRequester
	// OnChange is called after every successful changing API request, so
	// that the summary is published again (architecture.md, 11.5). It must
	// not block. It is nil without MQTT.
	OnChange func()
}

// NewHandler builds the handler chain (architecture.md, 4.4).
func NewHandler(cfg config.Config, d Deps) (http.Handler, error) {
	server := api.NewServer(api.ServerDeps{
		Version: d.Version, DB: d.DB, Publisher: d.Publisher, Lookuper: d.Lookuper, ImageDir: d.ImageDir,
		Snapshots: d.Snapshots,
	})
	strictHandler := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: responseErrorHandler(d.Logger),
	})
	apiHandler := api.HandlerWithOptions(strictHandler, api.StdHTTPServerOptions{
		ErrorHandlerFunc: requestErrorHandler,
	})
	web := d.WebUI
	if web == nil {
		var err error
		if web, err = webui.Dist(); err != nil {
			return nil, err
		}
	}
	return newChain(d.Logger, apiHandler, webui.Handler(web), d.OnChange)
}

// newChain wraps the generated API handler, from outside to inside:
// Recover, Logging, the change notification (only if onChange is not nil),
// StripPrefix("/api/v1"), request validator.
// For PUT /api/v1/products/{id}/image the body is limited to
// lookup.MaxImageBytes before StripPrefix, because the validator reads the
// whole body (architecture.md, 4.4).
// GET /api/v1/openapi.yaml passes Recover and Logging but bypasses the
// validator, because the route is not part of the spec (architecture.md, 6.2).
// All other paths go to the web UI handler, also inside Recover and Logging.
func newChain(logger *slog.Logger, apiHandler, webHandler http.Handler, onChange func()) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}
	// Without servers the validator matches paths without the /api/v1 prefix and ignores the Host header.
	spec.Servers = nil
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		ErrorHandlerWithOpts: validationErrorHandler,
	})

	apiChain := http.StripPrefix("/api/v1", validator(apiHandler))
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", apiChain)
	// The more specific pattern wins over /api/v1/ for GET and HEAD; other
	// methods on this path still reach the validator.
	mux.HandleFunc("GET /api/v1/openapi.yaml", serveSpec)
	// Also more specific than /api/v1/: an uploaded image is limited before
	// the validator reads it.
	mux.Handle("PUT /api/v1/products/{id}/image", http.MaxBytesHandler(apiChain, lookup.MaxImageBytes))
	mux.Handle("/", webHandler)

	var h http.Handler = mux
	if onChange != nil {
		h = notifyChanges(mux, onChange)
	}
	return httpx.Recover(logger, httpx.Logging(logger, h)), nil
}

// notifyChanges calls onChange after every request under /api/v1/ with a
// method other than GET and HEAD and a 2xx status (architecture.md, 11.5).
func notifyChanges(next http.Handler, onChange func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if status := rec.statusCode(); status >= 200 && status < 300 {
			onChange()
		}
	})
}

// statusRecorder remembers the status code written to the ResponseWriter.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	// Informational 1xx responses may precede the final status.
	if r.status == 0 && code >= 200 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// serveSpec writes the embedded api/openapi.yaml.
func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	// The status is already sent, so a failed write cannot be reported to the client.
	_, _ = w.Write(apispec.OpenAPI)
}
