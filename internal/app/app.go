// Package app is the composition root: it assembles the complete HTTP handler chain.
package app

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/httpx"
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
}

// NewHandler builds the handler chain (architecture.md, 4.4).
func NewHandler(cfg config.Config, d Deps) (http.Handler, error) {
	server := api.NewServer(api.ServerDeps{Version: d.Version, DB: d.DB, Publisher: d.Publisher, Lookuper: d.Lookuper})
	strictHandler := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: responseErrorHandler(d.Logger),
	})
	apiHandler := api.HandlerWithOptions(strictHandler, api.StdHTTPServerOptions{
		ErrorHandlerFunc: requestErrorHandler,
	})
	return newChain(d.Logger, apiHandler)
}

// newChain wraps the generated API handler, from outside to inside:
// Recover, Logging, StripPrefix("/api/v1"), request validator.
func newChain(logger *slog.Logger, apiHandler http.Handler) (http.Handler, error) {
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

	return httpx.Recover(logger, httpx.Logging(logger, mux)), nil
}
