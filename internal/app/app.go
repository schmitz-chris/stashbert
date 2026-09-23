// Package app is the composition root: it assembles the complete HTTP handler chain.
package app

import (
	"log/slog"
	"net/http"

	"github.com/schmitz-chris/stashbert/internal/api"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// Deps holds the dependencies created in main.
type Deps struct {
	Logger  *slog.Logger
	Version string
}

// NewHandler builds the handler chain (architecture.md, 4.4).
func NewHandler(cfg config.Config, d Deps) (http.Handler, error) {
	server := api.NewServer(api.ServerDeps{Version: d.Version})
	apiHandler := api.Handler(api.NewStrictHandler(server, nil))

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))

	return httpx.Logging(d.Logger, mux), nil
}
