package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/auth"
)

// serve sends a request with a deadline to h. A non-empty body is sent with contentType.
func serve(t *testing.T, h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// checkProblemCode asserts that rec is a problem with status and code.
func checkProblemCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p struct{ Code string }
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if p.Code != code {
		t.Errorf("code = %q, want %q", p.Code, code)
	}
}

// checkConfigured asserts the body of GET /api/v1/setup.
func checkConfigured(t *testing.T, h http.Handler, want bool) {
	t.Helper()
	rec := serve(t, h, http.MethodGet, "/api/v1/setup", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /setup: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	wantBody := `{"configured":false}`
	if want {
		wantBody = `{"configured":true}`
	}
	if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
		t.Errorf("GET /setup: body = %s, want %s", got, wantBody)
	}
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestGetSetupFreshInstance(t *testing.T) {
	h, _ := newApp(t)
	checkConfigured(t, h, false)
}

func TestCreateSetup(t *testing.T) {
	h, db := newApp(t)

	rec := serve(t, h, http.MethodPost, "/api/v1/setup", "application/json", `{"password":"geheim123","members":[" Chris ","Anna"]}`)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("POST /setup: status = %d, body %q; want 204 without body", rec.Code, rec.Body.String())
	}
	checkConfigured(t, h, true)

	var hash string
	if err := db.QueryRowContext(t.Context(), "SELECT value FROM settings WHERE key = 'password_hash'").Scan(&hash); err != nil {
		t.Fatalf("read password_hash: %v", err)
	}
	if ok, err := auth.VerifyPassword("geheim123", hash); err != nil || !ok {
		t.Errorf("VerifyPassword(stored hash) = %v, %v; want true, nil", ok, err)
	}

	rows, err := db.QueryContext(t.Context(), "SELECT id, name FROM members ORDER BY name")
	if err != nil {
		t.Fatalf("read members: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan member: %v", err)
		}
		if u, err := uuid.Parse(id); err != nil || u.Version() != 7 || id != strings.ToLower(id) {
			t.Errorf("member id %q is not a lowercase UUIDv7", id)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read members: %v", err)
	}
	if got, want := strings.Join(names, ","), "Anna,Chris"; got != want {
		t.Errorf("member names = %s, want %s (trimmed)", got, want)
	}

	rec = serve(t, h, http.MethodPost, "/api/v1/setup", "application/json", `{"password":"anderes123","members":["Bert"]}`)
	checkProblemCode(t, rec, http.StatusConflict, "already_configured")
	if n := count(t, db, "members"); n != 2 {
		t.Errorf("members after second POST = %d, want 2", n)
	}
}

func TestCreateSetupInvalidRequest(t *testing.T) {
	tests := []struct {
		name, contentType, body string
	}{
		{"password with 7 characters", "application/json", `{"password":"1234567","members":["Chris"]}`},
		{"no members", "application/json", `{"password":"geheim123","members":[]}`},
		{"names differing only in case", "application/json", `{"password":"geheim123","members":["Chris","chris"]}`},
		{"name only of spaces", "application/json", `{"password":"geheim123","members":["   "]}`},
		{"text/plain", "text/plain", `{"password":"geheim123","members":["Chris"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, db := newApp(t)

			rec := serve(t, h, http.MethodPost, "/api/v1/setup", tt.contentType, tt.body)

			checkProblemCode(t, rec, http.StatusBadRequest, "invalid_request")
			checkConfigured(t, h, false)
			if n := count(t, db, "members"); n != 0 {
				t.Errorf("members = %d, want 0 (nothing stored)", n)
			}
		})
	}
}
