package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/middleware"
)

// stubService is a minimal auth.Service stub for middleware tests.
// It avoids bcrypt latency by using a plain-text comparison.
type stubService struct {
	sessions map[string]string // sessionID → username
}

func (s *stubService) ValidateSession(id string) (string, error) {
	if u, ok := s.sessions[id]; ok {
		return u, nil
	}
	return "", middleware.ErrUnauthorized
}

// okHandler is a simple handler that returns 200 if it is reached.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func newStub(sessions map[string]string) middleware.SessionValidator {
	return &stubService{sessions: sessions}
}

// ── API middleware ─────────────────────────────────────────────────────────

func TestAPIAuth_ValidBearerToken(t *testing.T) {
	svc := newStub(map[string]string{"valid-token": "admin"})
	h := middleware.APIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestAPIAuth_MissingAuthHeader_Returns401(t *testing.T) {
	svc := newStub(map[string]string{})
	h := middleware.APIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAPIAuth_InvalidToken_Returns401(t *testing.T) {
	svc := newStub(map[string]string{"valid-token": "admin"})
	h := middleware.APIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAPIAuth_MalformedAuthHeader_Returns401(t *testing.T) {
	svc := newStub(map[string]string{})
	h := middleware.APIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/clusters", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic, not Bearer
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// ── UI middleware ──────────────────────────────────────────────────────────

func TestUIAuth_ValidCookie(t *testing.T) {
	svc := newStub(map[string]string{"valid-session": "admin"})
	h := middleware.UIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/clusters", nil)
	req.AddCookie(&http.Cookie{Name: "cnpg-ui-session", Value: "valid-session"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestUIAuth_MissingCookie_RedirectsToLogin(t *testing.T) {
	svc := newStub(map[string]string{})
	h := middleware.UIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/clusters", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if loc != "/ui/login" {
		t.Errorf("Location = %q, want %q", loc, "/ui/login")
	}
}

func TestUIAuth_InvalidCookie_RedirectsToLogin(t *testing.T) {
	svc := newStub(map[string]string{"valid-session": "admin"})
	h := middleware.UIAuth(svc)(okHandler)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/clusters", nil)
	req.AddCookie(&http.Cookie{Name: "cnpg-ui-session", Value: "bad-session"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}
}

func TestUIAuth_UsernameInjectedIntoContext(t *testing.T) {
	svc := newStub(map[string]string{"valid-session": "alice"})

	var capturedUsername string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedUsername = middleware.UsernameFromContext(r.Context())
	})

	h := middleware.UIAuth(svc)(captureHandler)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/clusters", nil)
	req.AddCookie(&http.Cookie{Name: "cnpg-ui-session", Value: "valid-session"})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if capturedUsername != "alice" {
		t.Errorf("username in context = %q, want %q", capturedUsername, "alice")
	}
}
