package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a panic in next into a 500 internal problem and logs it with the stack trace.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			// http.ErrAbortHandler deliberately aborts the response; net/http handles it.
			if v == http.ErrAbortHandler {
				panic(v)
			}
			logger.LogAttrs(r.Context(), slog.LevelError, "panic",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())),
			)
			WriteProblem(w, http.StatusInternalServerError, "internal", http.StatusText(http.StatusInternalServerError), "")
		}()
		next.ServeHTTP(w, r)
	})
}
