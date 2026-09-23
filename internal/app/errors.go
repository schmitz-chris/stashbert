package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// writeError writes e as a problem.
func writeError(w http.ResponseWriter, e *httpx.Error) {
	httpx.WriteProblem(w, e.Status, e.Code, e.Title, e.Detail)
}

// validationErrorHandler maps errors of the request validator to problems.
func validationErrorHandler(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
	switch {
	case errors.Is(err, routers.ErrMethodNotAllowed):
		writeError(w, httpx.NewError(http.StatusMethodNotAllowed, "method_not_allowed", ""))
	case errors.Is(err, routers.ErrPathNotFound):
		writeError(w, httpx.NotFound(""))
	default:
		writeError(w, httpx.BadRequest(err.Error()))
	}
}

// requestErrorHandler handles errors while decoding a request (strict handler)
// or binding its parameters (std-http handler).
func requestErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	writeError(w, httpx.BadRequest(err.Error()))
}

// responseErrorHandler writes an *httpx.Error returned by a handler as a problem.
// Any other error becomes 500 internal and is logged.
func responseErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		var e *httpx.Error
		if errors.As(err, &e) {
			writeError(w, e)
			return
		}
		logger.LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
		writeError(w, httpx.NewError(http.StatusInternalServerError, "internal", ""))
	}
}
