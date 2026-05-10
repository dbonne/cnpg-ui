// Package errors defines the application error model used across all packages.
// AppError is the single error type returned by all service and handler layers.
// It carries both an HTTP status code and a typed code enum for consistent
// JSON and HTML rendering.
package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorCode is a typed string enum identifying the category of an error.
// Handlers and clients use this to take appropriate action without parsing
// free-form error messages.
type ErrorCode string

const (
	// CodeNotFound indicates the requested resource does not exist.
	CodeNotFound ErrorCode = "NOT_FOUND"

	// CodeUnauthorized indicates the request lacks valid authentication.
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"

	// CodeForbidden indicates the authenticated user lacks permission.
	CodeForbidden ErrorCode = "FORBIDDEN"

	// CodeConflict indicates a state conflict (e.g. resource already exists).
	CodeConflict ErrorCode = "CONFLICT"

	// CodeValidation indicates the request contains invalid or missing fields.
	CodeValidation ErrorCode = "VALIDATION_ERROR"

	// CodeInternal indicates an unexpected server-side failure.
	CodeInternal ErrorCode = "INTERNAL_ERROR"
)

// AppError is the unified application error type. It implements the error
// interface and can render itself as JSON (for API routes) or HTML (for UI
// routes) via RenderJSON / RenderHTML.
type AppError struct {
	// HTTPStatus is the HTTP status code to send in the response.
	// It is excluded from JSON serialization — the status is set on the writer.
	HTTPStatus int `json:"-"`

	// Message is the human-readable error description.
	Message string `json:"error"`

	// Code is the machine-readable error category.
	Code ErrorCode `json:"code"`
}

// New constructs an AppError with the given HTTP status, code, and message.
func New(httpStatus int, code ErrorCode, message string) *AppError {
	return &AppError{
		HTTPStatus: httpStatus,
		Message:    message,
		Code:       code,
	}
}

// Error implements the built-in error interface so AppError can be used
// wherever a Go error is expected.
func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// RenderJSON writes the error as a JSON response. It sets the appropriate
// Content-Type and HTTP status code on the response writer.
func (e *AppError) RenderJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(e)
}

// RenderHTML writes the error as an HTML fragment response. It sets the
// appropriate Content-Type and HTTP status code. The fragment is a minimal
// div suitable for HTMX swap into an error container.
func (e *AppError) RenderHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(e.HTTPStatus)
	fmt.Fprintf(w, `<div class="error-partial" data-code="%s"><p>%s</p></div>`,
		e.Code, e.Message)
}
