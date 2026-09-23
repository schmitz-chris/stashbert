// Package app is the composition root: it assembles the complete HTTP handler chain.
package app

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	apispec "github.com/schmitz-chris/stashbert/api"
	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/webui"
)

// Deps holds the dependencies created in main.
type Deps struct {
	Logger  *slog.Logger
	Version string
	DB      *sql.DB
	// Publisher receives the domain events after each commit.
	Publisher events.Publisher
	// Lookuper looks up unknown barcodes booked with add (architecture.md, 7.2).
	Lookuper domain.Lookuper
	// ImageDir is the directory of the product image files, DATA_DIR/images
	// (architecture.md, 7.3).
	ImageDir string
	// WebUI holds the built web UI served under / (architecture.md, 4.2).
	// nil means the web UI embedded in the binary (webui.Dist).
	WebUI fs.FS
}

// NewHandler builds the handler chain (architecture.md, 4.4).
func NewHandler(cfg config.Config, d Deps) (http.Handler, error) {
	server := api.NewServer(api.ServerDeps{
		Version: d.Version, DB: d.DB, Publisher: d.Publisher, Lookuper: d.Lookuper, ImageDir: d.ImageDir,
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
	return newChain(d.Logger, apiHandler, webui.Handler(web))
}

// newChain wraps the generated API handler, from outside to inside:
// Recover, Logging, StripPrefix("/api/v1"), request validator.
// GET /api/v1/openapi.yaml passes Recover and Logging but bypasses the
// validator, because the route is not part of the spec (architecture.md, 6.2).
// All other paths go to the web UI handler, also inside Recover and Logging.
func newChain(logger *slog.Logger, apiHandler, webHandler http.Handler) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}
	// Without servers the validator matches paths without the /api/v1 prefix and ignores the Host header.
	spec.Servers = nil
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		ErrorHandlerWithOpts: validationErrorHandler,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", validator(apiHandler)))
	// The more specific pattern wins over /api/v1/ for GET and HEAD; other
	// methods on this path still reach the validator.
	mux.HandleFunc("GET /api/v1/openapi.yaml", serveSpec)
	mux.Handle("/", webHandler)

	return httpx.Recover(logger, httpx.Logging(logger, mux)), nil
}

// serveSpec writes the embedded api/openapi.yaml.
func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	// The status is already sent, so a failed write cannot be reported to the client.
	_, _ = w.Write(apispec.OpenAPI)
}
