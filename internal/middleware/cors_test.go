package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/middleware"
)

// ── CORS middleware ────────────────────────────────────────────────────────

// TestCORS_SameOriginDefault_NoOriginHeader verifies that when the request has
// no Origin header, the middleware passes through without adding CORS headers.
func TestCORS_SameOriginDefault_NoOriginHeader(t *testing.T) {
	h := middleware.CORS(nil)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty (no CORS header for same-origin)", got)
	}
}

// TestCORS_AllowedOrigin_SetsHeader verifies that an allowed origin gets
// Access-Control-Allow-Origin set to that exact origin.
func TestCORS_AllowedOrigin_SetsHeader(t *testing.T) {
	h := middleware.CORS([]string{"https://example.com"})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	got := rr.Header().Get("Access-Control-Allow-Origin")
	if got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
	}
}

// TestCORS_DisallowedOrigin_NoHeader verifies that an origin not in the allowed
// list does not receive Access-Control-Allow-Origin.
func TestCORS_DisallowedOrigin_NoHeader(t *testing.T) {
	h := middleware.CORS([]string{"https://example.com"})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	got := rr.Header().Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

// TestCORS_PreflightOptions_Returns204 verifies that an OPTIONS preflight
// request for an allowed origin returns 204 No Content with the required
// CORS headers.
func TestCORS_PreflightOptions_Returns204(t *testing.T) {
	h := middleware.CORS([]string{"https://example.com"})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/v1/clusters", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("missing Allow-Origin for preflight")
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing Access-Control-Allow-Methods for preflight")
	}
	if rr.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("missing Access-Control-Allow-Headers for preflight")
	}
}

// TestCORS_PreflightOptions_DisallowedOrigin_Returns204WithoutCORSHeaders
// verifies that a preflight from a disallowed origin gets 204 but no CORS
// headers (browser will reject it).
func TestCORS_PreflightOptions_DisallowedOrigin(t *testing.T) {
	h := middleware.CORS([]string{"https://example.com"})(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/v1/clusters", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for disallowed origin", got)
	}
}
