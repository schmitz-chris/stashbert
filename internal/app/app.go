// Package app is the composition root: it assembles the complete HTTP handler chain.
package app

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	apispec "github.com/schmitz-chris/stashbert/api"
	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/backup"
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
	// DBPath is the path of the database file, DATA_DIR/stashbert.db, for
	// GET /system (architecture.md, 6.2).
	DBPath string
	// BackupDir is the directory of the regular backups, DATA_DIR/backups,
	// for GET /system (architecture.md, 9.3).
	BackupDir string
	// WebUI holds the built web UI served under / (architecture.md, 4.2).
	// nil means the web UI embedded in the binary (webui.Dist).
	WebUI fs.FS
	// Snapshots takes the requests of POST /shopping-list/snapshot
	// (architecture.md, 11.3). It is nil without MQTT; the endpoint then
	// answers 409 mqtt_disabled.
	Snapshots api.SnapshotRequester
	// MQTT reports the connection and chooses the target list for
	// GET /integrations/mqtt and PUT /integrations/mqtt/target
	// (architecture.md, 11.7). It is nil without MQTT; GET then reports
	// disabled, and PUT answers 409 mqtt_disabled.
	MQTT api.MQTTIntegration
	// OnChange is called after every successful changing API request, so
	// that the summary is published again (architecture.md, 11.5). It must
	// not block. It is nil without MQTT.
	OnChange func()
	// Restart is called after POST /backup/restore has laid a backup ready
	// in DATA_DIR/restore; main then restarts in the same process, which
	// applies it (ADR-0020). It must not block. nil means no restart.
	Restart func()
}

// NewHandler builds the handler chain (architecture.md, 4.4).
func NewHandler(cfg config.Config, d Deps) (http.Handler, error) {
	return newHandler(cfg, d, backup.MaxUploadBytes)
}

// newHandler is NewHandler with restoreLimit as the largest body of
// POST /api/v1/backup/restore.
func newHandler(cfg config.Config, d Deps, restoreLimit int64) (http.Handler, error) {
	server := api.NewServer(api.ServerDeps{
		Version: d.Version, DB: d.DB, Publisher: d.Publisher, Lookuper: d.Lookuper, ImageDir: d.ImageDir,
		Snapshots: d.Snapshots, MQTT: d.MQTT,
		DBPath: d.DBPath, BackupDir: d.BackupDir, BackupKeep: cfg.BackupKeep, OpenFoodFacts: cfg.OFFContact != "",
		Logger: d.Logger, DataDir: cfg.DataDir, Restart: d.Restart,
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
	return newChain(d.Logger, apiHandler, webui.Handler(web), d.OnChange, restoreLimit)
}

// newChain wraps the generated API handler, from outside to inside:
// Recover, Logging, the change notification (only if onChange is not nil),
// StripPrefix("/api/v1"), request validator.
// For PUT /api/v1/products/{id}/image the body is limited to
// lookup.MaxImageBytes before StripPrefix, because the validator reads the
// whole body (architecture.md, 4.4).
// For GET /api/v1/backup the write deadline is extended to
// backupWriteTimeout before StripPrefix, because a larger backup takes
// longer than the WriteTimeout of the server (architecture.md, 6.2).
// POST /api/v1/backup/restore bypasses the validator, which would read the
// whole archive into memory; limitRestore checks it instead and limits its
// body to restoreLimit (architecture.md, 4.4).
// GET /api/v1/openapi.yaml passes Recover and Logging but bypasses the
// validator, because the route is not part of the spec (architecture.md, 6.2).
// All other paths go to the web UI handler, also inside Recover and Logging.
func newChain(logger *slog.Logger, apiHandler, webHandler http.Handler, onChange func(), restoreLimit int64) (http.Handler, error) {
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
	mux.Handle("GET /api/v1/backup", extendWriteDeadline(apiChain, backupWriteTimeout))
	mux.Handle("POST /api/v1/backup/restore", limitRestore(http.StripPrefix("/api/v1", apiHandler), restoreLimit))
	mux.Handle("/", webHandler)

	var h http.Handler = mux
	if onChange != nil {
		h = notifyChanges(mux, onChange)
	}
	return httpx.Recover(logger, httpx.Logging(logger, h)), nil
}

// backupWriteTimeout is the time a backup download has for its response,
// including the wait for a running download and writing the archive.
const backupWriteTimeout = 10 * time.Minute

// extendWriteDeadline sets the write deadline of the response to d from now
// and then calls next. A ResponseWriter without deadlines, such as
// httptest.ResponseRecorder, keeps none; a connection that is already
// closed fails at the first write anyway.
func extendWriteDeadline(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
		next.ServeHTTP(w, r)
	})
}

// restoreTimeout is the time an uploaded backup has for its request and
// its response, including checking the archive (ADR-0020).
const restoreTimeout = 10 * time.Minute

// limitRestore stands in for the validator on POST /api/v1/backup/restore
// (architecture.md, 4.4): a Content-Type other than application/gzip or
// application/x-gzip results in 400 invalid_request, a Content-Length over
// limit in 413 backup_too_large. The body is limited to limit bytes with an
// http.MaxBytesReader, and the read and write deadlines are extended to
// restoreTimeout from now, because the ReadTimeout and WriteTimeout of the
// server are too short for a large backup.
func limitRestore(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || (mediaType != "application/gzip" && mediaType != "application/x-gzip") {
			writeError(w, httpx.BadRequest("Content-Type muss application/gzip sein"))
			return
		}
		if r.ContentLength > limit {
			writeError(w, backup.TooLarge())
			return
		}
		rc := http.NewResponseController(w)
		deadline := time.Now().Add(restoreTimeout)
		// A ResponseWriter without deadlines, such as
		// httptest.ResponseRecorder, keeps none.
		_ = rc.SetReadDeadline(deadline)
		_ = rc.SetWriteDeadline(deadline)
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
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
