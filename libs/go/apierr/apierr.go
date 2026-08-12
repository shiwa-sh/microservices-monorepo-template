// Package apierr defines the platform-wide HTTP error format (ADR-0303).
// Every service returns this shape on 4xx and 5xx responses, and so does the edge,
// so a generated client has one error branch rather than two.
//
// The shape is [RFC 9457] problem details. Two members carry the weight:
//
//   - title is STABLE and does not vary with the instance. It is the status
//     reason phrase, so a client can switch on it without parsing prose.
//   - detail is instance-specific and safe to show a user. Never a stack trace, a
//     query, or an internal hostname — see [Internal], which is where that rule is
//     easiest to break.
//
// type stays "about:blank" unless two errors share a status code and a client has
// to tell them apart. That case takes a URN through [Error.WithType], never a
// dereferenceable URL: a fetchable type would put an error taxonomy into the flat
// public URL namespace ADR-0306 keeps free of one.
//
// instance is deliberately absent. trace_id identifies the occurrence and reaches
// the trace, where an instance URL reaches nothing.
//
// [RFC 9457]: https://www.rfc-editor.org/rfc/rfc9457
package apierr

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// TypeBlank is the default problem type. RFC 9457 gives it the meaning "the status
// code is the whole of the machine-readable semantics".
const TypeBlank = "about:blank"

// ValidationError is one field-level failure, as the errors extension carries it.
type ValidationError struct {
	// Pointer is an RFC 6901 JSON Pointer to the offending member.
	Pointer string `json:"pointer"`
	Message string `json:"message"`
}

// Error is the canonical platform error response.
type Error struct {
	// Status is duplicated into the body as `status`, per RFC 9457.
	Status  int               `json:"status"`
	Type    string            `json:"type"`
	Title   string            `json:"title"`
	Detail  string            `json:"detail,omitempty"`
	TraceID string            `json:"trace_id,omitempty"`
	Errors  []ValidationError `json:"errors,omitempty"`
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Title
	}
	return e.Title + ": " + e.Detail
}

// WithType sets a problem-type URN, for the case where two errors share a status
// code and a client handles them differently.
func (e *Error) WithType(urn string) *Error {
	e.Type = urn
	return e
}

// WithValidation attaches field-level failures. Populated from the generated
// validator rather than assembled by hand in a handler.
func (e *Error) WithValidation(errs ...ValidationError) *Error {
	e.Errors = append(e.Errors, errs...)
	return e
}

// WithTrace stamps the trace-id of the active span, so a user-reported error
// reaches its trace. It is a no-op outside a recording span, which keeps a unit
// test from having to build a tracer to construct an error.
//
// This lives here rather than in each handler because a handler that forgets it
// produces an error nobody can find, and there is no signal that it was forgotten.
func (e *Error) WithTrace(ctx context.Context) *Error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		e.TraceID = sc.TraceID().String()
	}
	return e
}

// Write serialises the error to w. Callers inside a request should prefer
// WriteContext, which stamps the trace-id.
func (e *Error) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	err := json.NewEncoder(w).Encode(e)
	if err != nil {
		slog.Error("apierr: encode response", "error", err)
	}
}

// WriteContext stamps the trace-id from ctx, then writes.
func (e *Error) WriteContext(ctx context.Context, w http.ResponseWriter) {
	e.WithTrace(ctx).Write(w)
}

// New builds an error with the status's reason phrase as its stable title.
func New(status int, detail string) *Error {
	return &Error{
		Status: status,
		Type:   TypeBlank,
		Title:  http.StatusText(status),
		Detail: detail,
	}
}

// Helpers for the common cases. Service code calls these rather than building the
// struct by hand, so `title` stays identical for a given status across services —
// which is the property that makes it switchable.

func BadRequest(detail string) *Error { return New(http.StatusBadRequest, detail) }

func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "missing or invalid credentials")
}

func Forbidden(detail string) *Error { return New(http.StatusForbidden, detail) }

func NotFound(resource string) *Error {
	return New(http.StatusNotFound, resource+" not found")
}

func Conflict(detail string) *Error { return New(http.StatusConflict, detail) }

// Internal is the one helper whose detail is NOT safe to pass through from an
// underlying error: a driver or a client message routinely carries a query, a
// hostname, or a credential. The cause is logged and the response says nothing.
// Pass the cause so it reaches the log; it never reaches the body.
func Internal(cause string) *Error {
	slog.Error("apierr: internal", "cause", cause)
	return New(http.StatusInternalServerError, "an internal error occurred")
}

// As unwraps err to an *Error if possible.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
