package httpx

import (
	"encoding/json"
	"net/http"
)

// problem is the RFC 9457 body with the additional field code (architecture.md, 6.1).
type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Code   string `json:"code"`
}

// WriteProblem writes an RFC 9457 problem as application/problem+json with type about:blank.
func WriteProblem(w http.ResponseWriter, status int, code, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	// The status is already sent, so a failed write cannot be reported to the client.
	_ = json.NewEncoder(w).Encode(problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
	})
}

// Error is a domain error that handlers return; it is written as a problem.
type Error struct {
	Status int
	Code   string
	Title  string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// NewError returns an Error whose Title is the standard text of status.
func NewError(status int, code, detail string) *Error {
	return &Error{Status: status, Code: code, Title: http.StatusText(status), Detail: detail}
}

// BadRequest returns a 400 invalid_request error.
func BadRequest(detail string) *Error {
	return NewError(http.StatusBadRequest, "invalid_request", detail)
}

// NotFound returns a 404 not_found error.
func NotFound(detail string) *Error {
	return NewError(http.StatusNotFound, "not_found", detail)
}
