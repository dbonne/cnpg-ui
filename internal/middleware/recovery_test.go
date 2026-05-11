package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/middleware"
)

// ── Recovery middleware ────────────────────────────────────────────────────

// panicHandler triggers a panic to exercise the recovery middleware.
var panicHandler = http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
	panic("simulated panic")
})

// TestRecovery_NormalRequest_PassesThrough verifies that a non-panicking handler
// is unaffected by the recovery middleware.
func TestRecovery_NormalRequest_PassesThrough(t *testing.T) {
	h := middleware.Recovery()(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// TestRecovery_PanicInAPIPath_Returns500JSON verifies that a panic in an
// /api/* handler results in 500 with a JSON body.
func TestRecovery_PanicInAPIPath_Returns500JSON(t *testing.T) {
	h := middleware.Recovery()(panicHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "error") {
		t.Errorf("body = %q, expected JSON error field", body)
	}
}

// TestRecovery_PanicInUIPath_Returns500HTML verifies that a panic in a
// /ui/* handler results in 500 with an HTML body.
func TestRecovery_PanicInUIPath_Returns500HTML(t *testing.T) {
	h := middleware.Recovery()(panicHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/clusters", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "500") && !strings.Contains(body, "Internal Server Error") {
		t.Errorf("body = %q, expected 500/Internal Server Error", body)
	}
}

// TestRecovery_PanicInHealthPath_Returns500JSON verifies that a panic in a
// non-UI path returns JSON (fallback to API format).
func TestRecovery_PanicInHealthPath_Returns500JSON(t *testing.T) {
	h := middleware.Recovery()(panicHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON for non-UI path", ct)
	}
}
