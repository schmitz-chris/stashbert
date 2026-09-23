package httpx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/httpx"
)

func TestLogging(t *testing.T) {
	tests := []struct {
		path      string
		status    int
		wantLevel string
	}{
		{"/api/v1/health", http.StatusOK, "DEBUG"},
		{"/api/v1/products", http.StatusNotFound, "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.status) })

			httpx.Logging(logger, next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))

			var got struct {
				Level, Method, Path string
				Status              int
				DurationMS          *float64 `json:"duration_ms"`
			}
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("decode log entry %q: %v", buf.String(), err)
			}
			if got.Level != tt.wantLevel || got.Method != http.MethodGet || got.Path != tt.path || got.Status != tt.status || got.DurationMS == nil {
				t.Errorf("log entry = %s, want level %s, GET %s, status %d, duration_ms", buf.String(), tt.wantLevel, tt.path, tt.status)
			}
		})
	}
}
