// Package ui provides HTTP handlers for the HTMX-powered web UI.
package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/ui"
)

// ── Mock service implementations ──────────────────────────────────────────────

// mockClusterLister stubs ClusterLister with a fixed list.
type mockClusterLister struct {
	clusters []api.ClusterSummary
	err      error
}

func (m *mockClusterLister) List(_ context.Context) ([]api.ClusterSummary, error) {
	return m.clusters, m.err
}

// mockClusterGetter stubs ClusterGetter with a fixed ClusterDetail.
type mockClusterGetter struct {
	detail *api.ClusterDetail
	err    error
}

func (m *mockClusterGetter) Get(_ context.Context, _ string) (*api.ClusterDetail, error) {
	return m.detail, m.err
}

// mockBackupLister stubs BackupLister with a fixed backup list.
type mockBackupLister struct {
	backups []api.BackupSummary
	err     error
}

func (m *mockBackupLister) ListBackups(_ context.Context, _ string) ([]api.BackupSummary, error) {
	return m.backups, m.err
}

// mockPoolerLister stubs PoolerLister.
type mockPoolerLister struct {
	poolers []api.PoolerSummary
	err     error
}

func (m *mockPoolerLister) List(_ context.Context, _ string) ([]api.PoolerSummary, error) {
	return m.poolers, m.err
}

// mockConfigGetter stubs ConfigGetter.
type mockConfigGetter struct {
	resp *api.PgConfigResponse
	err  error
}

func (m *mockConfigGetter) GetConfig(_ context.Context, _ string) (*api.PgConfigResponse, error) {
	return m.resp, m.err
}

// ── Test router builder ───────────────────────────────────────────────────────

// newIntegrationHandler builds a UIHandler with all mock services injected.
func newIntegrationHandler(t *testing.T,
	lister ui.ClusterLister,
	getter ui.ClusterGetter,
	backups ui.BackupLister,
	poolers ui.PoolerLister,
	cfg ui.ConfigGetter,
) *ui.UIHandler {
	t.Helper()
	h, err := ui.NewUIHandler(lister, getter, backups, poolers, cfg)
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}
	return h
}

// newFullRouter registers all UI routes including panel endpoints.
func newFullRouter(h *ui.UIHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/ui/login", h.LoginPage)
	r.Get("/ui/clusters", h.ListClusters)
	r.Get("/ui/clusters/{name}", h.ClusterDetail)
	r.Get("/ui/clusters/{name}/overview", h.OverviewPanel)
	r.Get("/ui/clusters/{name}/logs-panel", h.LogsPanel)
	r.Get("/ui/clusters/{name}/config-panel", h.ConfigPanel)
	r.Get("/ui/clusters/{name}/backups-panel", h.BackupsPanel)
	return r
}

// ── 1. Login page renders ─────────────────────────────────────────────────────

func TestIntegration_LoginPage_rendersForm(t *testing.T) {
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/login", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<form`) {
		t.Error("expected <form> element in login page")
	}
	if !strings.Contains(body, `name="username"`) {
		t.Error("expected username input in login page")
	}
	if !strings.Contains(body, `type="password"`) {
		t.Error("expected password input in login page")
	}
}

// ── 2. Cluster list shows clusters ────────────────────────────────────────────

func TestIntegration_ClusterList_showsClusters(t *testing.T) {
	lister := &mockClusterLister{
		clusters: []api.ClusterSummary{
			{Name: "prod-db", Status: api.StatusHealthy, Instances: 3},
		},
	}
	h := newIntegrationHandler(t, lister, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "prod-db") {
		t.Errorf("expected cluster name 'prod-db' in list, got:\n%s", body)
	}
	if !strings.Contains(body, "HEALTHY") {
		t.Errorf("expected status badge 'HEALTHY' in list, got:\n%s", body)
	}
	if !strings.Contains(body, "3") {
		t.Errorf("expected instance count '3' in list, got:\n%s", body)
	}
}

// ── 3. Cluster list empty state ───────────────────────────────────────────────

func TestIntegration_ClusterList_emptyState(t *testing.T) {
	lister := &mockClusterLister{clusters: []api.ClusterSummary{}}
	h := newIntegrationHandler(t, lister, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No clusters") {
		t.Errorf("expected 'No clusters' message in empty state, got:\n%s", body)
	}
}

// ── 4. Cluster detail loads real data ─────────────────────────────────────────

func TestIntegration_ClusterDetail_loadsRealData(t *testing.T) {
	getter := &mockClusterGetter{
		detail: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name:      "my-cluster",
				Status:    api.StatusFault,
				Instances: 2,
			},
			StorageSize: "10Gi",
		},
	}
	h := newIntegrationHandler(t, nil, getter, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/my-cluster", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "my-cluster") {
		t.Errorf("expected cluster name 'my-cluster' in detail, got:\n%s", body)
	}
	// Real status from getter — not hardcoded HEALTHY
	if !strings.Contains(body, string(api.StatusFault)) {
		t.Errorf("expected status 'FAULT' from real data, got:\n%s", body)
	}
}

func TestIntegration_ClusterDetail_fallsBackWithoutGetter(t *testing.T) {
	// Without a getter, detail page still renders (graceful degradation)
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/other-cluster", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "other-cluster") {
		t.Errorf("expected cluster name in detail even without getter, got:\n%s", body)
	}
}

// ── 5. Overview panel renders ─────────────────────────────────────────────────

func TestIntegration_OverviewPanel_rendersInstances(t *testing.T) {
	getter := &mockClusterGetter{
		detail: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name:           "prod-db",
				Status:         api.StatusHealthy,
				Instances:      3,
				ReadyInstances: 3,
				CurrentPrimary: "prod-db-1",
			},
			StorageSize: "20Gi",
		},
	}
	h := newIntegrationHandler(t, nil, getter, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/overview", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "prod-db-1") {
		t.Errorf("expected primary name 'prod-db-1' in overview panel, got:\n%s", body)
	}
	if !strings.Contains(body, "3") {
		t.Errorf("expected instance count '3' in overview panel, got:\n%s", body)
	}
	if !strings.Contains(body, "20Gi") {
		t.Errorf("expected storage '20Gi' in overview panel, got:\n%s", body)
	}
}

func TestIntegration_OverviewPanel_noLayoutWrapper(t *testing.T) {
	// Panel is an HTML fragment — must NOT contain a full HTML page structure
	getter := &mockClusterGetter{
		detail: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{Name: "x", Status: api.StatusHealthy, Instances: 1},
		},
	}
	h := newIntegrationHandler(t, nil, getter, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/x/overview", nil)
	router.ServeHTTP(w, r)

	body := w.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("overview panel must be an HTML fragment, not a full page")
	}
}

// ── 6. Logs panel renders ─────────────────────────────────────────────────────

func TestIntegration_LogsPanel_rendersSSESetup(t *testing.T) {
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/logs-panel", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	// Logs panel must reference the log stream endpoint
	if !strings.Contains(body, "logs") {
		t.Errorf("expected log stream reference in logs panel, got:\n%s", body)
	}
}

func TestIntegration_LogsPanel_noLayoutWrapper(t *testing.T) {
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/logs-panel", nil)
	router.ServeHTTP(w, r)

	body := w.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("logs panel must be an HTML fragment, not a full page")
	}
}

// ── 7. Config panel renders ───────────────────────────────────────────────────

func TestIntegration_ConfigPanel_rendersParameters(t *testing.T) {
	cfg := &mockConfigGetter{
		resp: &api.PgConfigResponse{
			Parameters: []api.PgParamMetadata{
				{Name: "max_connections", CurrentValue: "100", Type: "string"},
				{Name: "shared_buffers", CurrentValue: "128MB", Type: "string", RequiresRestart: true},
			},
		},
	}
	h := newIntegrationHandler(t, nil, nil, nil, nil, cfg)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/config-panel", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "max_connections") {
		t.Errorf("expected parameter 'max_connections' in config panel, got:\n%s", body)
	}
	if !strings.Contains(body, "100") {
		t.Errorf("expected value '100' for max_connections in config panel, got:\n%s", body)
	}
	if !strings.Contains(body, "shared_buffers") {
		t.Errorf("expected parameter 'shared_buffers' in config panel, got:\n%s", body)
	}
}

func TestIntegration_ConfigPanel_noLayoutWrapper(t *testing.T) {
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/config-panel", nil)
	router.ServeHTTP(w, r)

	body := w.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("config panel must be an HTML fragment, not a full page")
	}
}

// ── 8. Backups panel renders ──────────────────────────────────────────────────

func TestIntegration_BackupsPanel_rendersBackups(t *testing.T) {
	backups := &mockBackupLister{
		backups: []api.BackupSummary{
			{Name: "backup-001", ClusterName: "prod-db", Phase: "completed", Method: "barmanObjectStore"},
			{Name: "backup-002", ClusterName: "prod-db", Phase: "running", Method: "barmanObjectStore"},
		},
	}
	h := newIntegrationHandler(t, nil, nil, backups, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/backups-panel", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "backup-001") {
		t.Errorf("expected backup 'backup-001' in backups panel, got:\n%s", body)
	}
	if !strings.Contains(body, "completed") {
		t.Errorf("expected phase 'completed' in backups panel, got:\n%s", body)
	}
	if !strings.Contains(body, "backup-002") {
		t.Errorf("expected backup 'backup-002' in backups panel, got:\n%s", body)
	}
}

// ── 9. Backups panel empty state ──────────────────────────────────────────────

func TestIntegration_BackupsPanel_emptyState(t *testing.T) {
	backups := &mockBackupLister{backups: []api.BackupSummary{}}
	h := newIntegrationHandler(t, nil, nil, backups, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/backups-panel", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No backups") {
		t.Errorf("expected 'No backups' message in empty backups panel, got:\n%s", body)
	}
}

func TestIntegration_BackupsPanel_noLayoutWrapper(t *testing.T) {
	h := newIntegrationHandler(t, nil, nil, nil, nil, nil)
	router := newFullRouter(h)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ui/clusters/prod-db/backups-panel", nil)
	router.ServeHTTP(w, r)

	body := w.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("backups panel must be an HTML fragment, not a full page")
	}
}
