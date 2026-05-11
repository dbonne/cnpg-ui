package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/middleware"
)

// ── Logging middleware ─────────────────────────────────────────────────────

// TestRequestLogger_SetsRequestIDHeader verifies that RequestLogger injects an
// X-Request-ID header in the response when none is present in the request.
func TestRequestLogger_SetsRequestIDHeader(t *testing.T) {
	h := middleware.RequestLogger()(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID response header should be set")
	}
}

// TestRequestLogger_PreservesIncomingRequestID verifies that an existing
// X-Request-ID header from the client is echoed back in the response.
func TestRequestLogger_PreservesIncomingRequestID(t *testing.T) {
	h := middleware.RequestLogger()(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "client-provided-id")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	got := rr.Header().Get("X-Request-ID")
	if got != "client-provided-id" {
		t.Errorf("X-Request-ID = %q, want %q", got, "client-provided-id")
	}
}

// TestRequestLogger_InjectsRequestIDIntoContext verifies that the request ID is
// available in the request context via RequestIDFromContext.
func TestRequestLogger_InjectsRequestIDIntoContext(t *testing.T) {
	var capturedID string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = middleware.RequestIDFromContext(r.Context())
	})

	h := middleware.RequestLogger()(captureHandler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "ctx-test-id")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if capturedID != "ctx-test-id" {
		t.Errorf("RequestIDFromContext = %q, want %q", capturedID, "ctx-test-id")
	}
}

// TestRequestLogger_GeneratesUniqueIDsWhenNotProvided verifies that two
// requests without X-Request-ID get different generated IDs.
func TestRequestLogger_GeneratesUniqueIDsWhenNotProvided(t *testing.T) {
	var id1, id2 string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if id1 == "" {
			id1 = middleware.RequestIDFromContext(r.Context())
		} else {
			id2 = middleware.RequestIDFromContext(r.Context())
		}
	})

	h := middleware.RequestLogger()(captureHandler)

	req1 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)

	if id1 == "" || id2 == "" {
		t.Error("expected non-empty request IDs")
	}
	if id1 == id2 {
		t.Errorf("expected unique request IDs, both got %q", id1)
	}
}

// TestRequestLogger_PassesRequestThrough verifies that the middleware calls the
// next handler and does not swallow the response status.
func TestRequestLogger_PassesRequestThrough(t *testing.T) {
	notFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	h := middleware.RequestLogger()(notFound)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}
