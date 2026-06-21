// Package apierr defines the platform-wide HTTP error format (ADR-0008).
// Every service returns this shape on 4xx and 5xx responses. The same shape is
// declared as the Error response in each service's OpenAPI spec; this Go type
// implements it.
package apierr

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Error is the canonical platform error response. JSON shape:
//
//	{ "code": "string", "message": "string", "details": {...} }
type Error struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Write serialises the error to w with the appropriate status code.
func (e *Error) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	err := json.NewEncoder(w).Encode(e)
	if err != nil {
		slog.Error("apierr: encode response", "error", err)
	}
}

// Helpers for the common cases. Service code calls these instead of building
// the struct by hand to keep the codes consistent across services.

func BadRequest(msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func Unauthorized() *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "missing or invalid credentials"}
}

func Forbidden(msg string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func NotFound(resource string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: resource + " not found"}
}

func Conflict(msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: "conflict", Message: msg}
}

func Internal(msg string) *Error {
	return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: msg}
}

// As unwraps err to an *Error if possible.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
