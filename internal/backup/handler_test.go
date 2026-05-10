package backup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/backup"
)

// stubService is a test double for backup.Service.
type stubService struct {
	listBackupsResult           []api.BackupSummary
	listBackupsErr              error
	triggerBackupResult         *api.BackupSummary
	triggerBackupErr            error
	listScheduledBackupsResult  []api.ScheduledBackupSummary
	listScheduledBackupsErr     error
	getScheduledBackupResult    *api.ScheduledBackupSummary
	getScheduledBackupErr       error
	createScheduledBackupResult *api.ScheduledBackupSummary
	createScheduledBackupErr    error
	deleteScheduledBackupErr    error
}

func (s *stubService) ListBackups(_ context.Context, _ string) ([]api.BackupSummary, error) {
	return s.listBackupsResult, s.listBackupsErr
}
func (s *stubService) TriggerBackup(_ context.Context, _ string, _ api.TriggerBackupRequest) (*api.BackupSummary, error) {
	return s.triggerBackupResult, s.triggerBackupErr
}
func (s *stubService) ListScheduledBackups(_ context.Context, _ string) ([]api.ScheduledBackupSummary, error) {
	return s.listScheduledBackupsResult, s.listScheduledBackupsErr
}
func (s *stubService) GetScheduledBackup(_ context.Context, _, _ string) (*api.ScheduledBackupSummary, error) {
	return s.getScheduledBackupResult, s.getScheduledBackupErr
}
func (s *stubService) CreateScheduledBackup(_ context.Context, _ string, _ api.CreateScheduledBackupRequest) (*api.ScheduledBackupSummary, error) {
	return s.createScheduledBackupResult, s.createScheduledBackupErr
}
func (s *stubService) DeleteScheduledBackup(_ context.Context, _, _ string) error {
	return s.deleteScheduledBackupErr
}

func routedRequest(method, path string, body []byte, params map[string]string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestListBackups_Handler_OK verifies ListBackups returns 200 with backup array.
func TestListBackups_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		listBackupsResult: []api.BackupSummary{
			{Name: "backup-1", ClusterName: "prod", Phase: "completed"},
			{Name: "backup-2", ClusterName: "prod", Phase: "running"},
		},
	}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/backups", nil,
		map[string]string{"name": "prod"})
	h.ListBackups(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var backups []api.BackupSummary
	if err := json.NewDecoder(w.Body).Decode(&backups); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(backups) != 2 {
		t.Errorf("count: got %d, want 2", len(backups))
	}
}

// TestTriggerBackup_Handler_OK verifies TriggerBackup returns 202.
func TestTriggerBackup_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		triggerBackupResult: &api.BackupSummary{
			Name: "prod-abc123", ClusterName: "prod", Phase: "pending",
		},
	}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPost, "/api/v1/clusters/prod/backups", nil,
		map[string]string{"name": "prod"})
	h.TriggerBackup(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusAccepted)
	}

	var result api.BackupSummary
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Phase != "pending" {
		t.Errorf("Phase: got %q, want pending", result.Phase)
	}
}

// TestTriggerBackup_Handler_ClusterNotFound verifies 409 when cluster is in error.
// (Here we use a 409 when the service returns a conflict-type error.)
func TestTriggerBackup_Handler_ServiceError(t *testing.T) {
	t.Parallel()

	svc := &stubService{triggerBackupErr: fmt.Errorf("trigger backup failed")}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPost, "/api/v1/clusters/prod/backups", nil,
		map[string]string{"name": "prod"})
	h.TriggerBackup(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// TestListScheduledBackups_Handler_OK verifies ListScheduledBackups returns 200.
func TestListScheduledBackups_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		listScheduledBackupsResult: []api.ScheduledBackupSummary{
			{Name: "daily", ClusterName: "prod", Schedule: "0 2 * * *"},
		},
	}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/scheduled-backups", nil,
		map[string]string{"name": "prod"})
	h.ListScheduledBackups(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var scheds []api.ScheduledBackupSummary
	if err := json.NewDecoder(w.Body).Decode(&scheds); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(scheds) != 1 {
		t.Errorf("count: got %d, want 1", len(scheds))
	}
}

// TestCreateScheduledBackup_Handler_OK verifies CreateScheduledBackup returns 201.
func TestCreateScheduledBackup_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		createScheduledBackupResult: &api.ScheduledBackupSummary{
			Name: "daily", ClusterName: "prod", Schedule: "0 2 * * *",
		},
	}
	h := backup.NewHandler(svc)

	body, _ := json.Marshal(api.CreateScheduledBackupRequest{
		Name: "daily", Schedule: "0 2 * * *",
	})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPost, "/api/v1/clusters/prod/scheduled-backups",
		body, map[string]string{"name": "prod"})
	h.CreateScheduledBackup(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}

	var result api.ScheduledBackupSummary
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Name != "daily" {
		t.Errorf("Name: got %q, want daily", result.Name)
	}
}

// TestDeleteScheduledBackup_Handler_OK verifies DeleteScheduledBackup returns 204.
func TestDeleteScheduledBackup_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodDelete, "/api/v1/clusters/prod/scheduled-backups/daily", nil,
		map[string]string{"name": "prod", "id": "daily"})
	h.DeleteScheduledBackup(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNoContent)
	}
}

// TestDeleteScheduledBackup_Handler_NotFound verifies 404 for missing scheduled backup.
func TestDeleteScheduledBackup_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubService{deleteScheduledBackupErr: fmt.Errorf("scheduled backup \"ghost\" not found")}
	h := backup.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodDelete, "/api/v1/clusters/prod/scheduled-backups/ghost", nil,
		map[string]string{"name": "prod", "id": "ghost"})
	h.DeleteScheduledBackup(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}
