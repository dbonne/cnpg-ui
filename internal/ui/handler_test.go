// Package ui provides HTTP handlers for the HTMX-powered web UI.
package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/ui"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestHandler creates a Handler pointing at the real embedded templates.
func newTestHandler(t *testing.T) *ui.Handler {
	t.Helper()
	h, err := ui.NewUIHandler(nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}
	return h
}

func newChiRouter(h *ui.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/ui/login", h.LoginPage)
	r.Get("/ui/clusters", h.ListClusters)
	r.Get("/ui/clusters/{name}", h.ClusterDetail)
	return r
}

// ── 6.5.1: NewUIHandler returns non-nil handler without error ─────────────────

func TestNewUIHandler_success(t *testing.T) {
	h, err := ui.NewUIHandler(nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

// ── 6.5.2: LoginPage renders text/html with login form ────────────────────────

func TestUIHandler_LoginPage_rendersHTML(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/login", nil)

	h.LoginPage(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<form") {
		t.Errorf("expected <form> in login page body, got: %s", body)
	}
	if !strings.Contains(body, `action="/ui/login"`) {
		t.Errorf("expected form action /ui/login in body, got: %s", body)
	}
}

func TestUIHandler_LoginPage_containsPasswordField(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/login", nil)

	h.LoginPage(w, r)

	body := w.Body.String()
	if !strings.Contains(body, `type="password"`) {
		t.Errorf("expected password input in login page, got: %s", body)
	}
	if !strings.Contains(body, `name="username"`) {
		t.Errorf("expected username input in login page, got: %s", body)
	}
}

// ── 6.5.3: ListClusters renders text/html cluster list ───────────────────────

func TestUIHandler_ListClusters_rendersHTML(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters", nil)

	h.ListClusters(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Clusters") {
		t.Errorf("expected 'Clusters' heading in cluster list, got: %s", body)
	}
}

func TestUIHandler_ListClusters_containsHTMXRefresh(t *testing.T) {
	h := newTestHandler(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters", nil)

	h.ListClusters(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "hx-trigger") {
		t.Errorf("expected HTMX hx-trigger attribute for auto-refresh in cluster list, got: %s", body)
	}
}

// ── 6.5.4: ClusterDetail renders text/html detail page ───────────────────────

func TestUIHandler_ClusterDetail_rendersHTML(t *testing.T) {
	h := newTestHandler(t)
	router := newChiRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/my-cluster", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "my-cluster") {
		t.Errorf("expected cluster name 'my-cluster' in detail page, got: %s", body)
	}
}

func TestUIHandler_ClusterDetail_containsTabs(t *testing.T) {
	h := newTestHandler(t)
	router := newChiRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/test-cluster", nil)
	router.ServeHTTP(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Overview") {
		t.Errorf("expected 'Overview' tab in detail page, got: %s", body)
	}
	if !strings.Contains(body, "Backups") {
		t.Errorf("expected 'Backups' tab in detail page, got: %s", body)
	}
}

// ── 6.5.5: StatusBadgeClass pure function maps status to CSS class ────────────

func TestStatusBadgeClass(t *testing.T) {
	cases := []struct {
		status api.NormalizedStatus
		want   string
	}{
		{api.StatusHealthy, "badge-green"},
		{api.StatusTransient, "badge-yellow"},
		{api.StatusFault, "badge-red"},
		{api.StatusHibernated, "badge-blue"},
		{api.NormalizedStatus("UNKNOWN"), "badge-gray"},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := ui.StatusBadgeClass(tc.status)
			if got != tc.want {
				t.Errorf("StatusBadgeClass(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
