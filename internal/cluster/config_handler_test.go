package cluster_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/cluster"
)

// stubConfigService is a test double for cluster.ConfigService.
type stubConfigService struct {
	getResult *api.PgConfigResponse
	getErr    error
	putResult *api.PgConfigResponse
	putErr    error
}

func (s *stubConfigService) GetConfig(_ context.Context, _ string) (*api.PgConfigResponse, error) {
	return s.getResult, s.getErr
}
func (s *stubConfigService) UpdateConfig(_ context.Context, _ string, _ api.UpdatePgConfigRequest) (*api.PgConfigResponse, error) {
	return s.putResult, s.putErr
}

// TestGetPostgresConfig_Handler_OK verifies GetPostgresConfig returns 200 with params.
func TestGetPostgresConfig_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubConfigService{
		getResult: &api.PgConfigResponse{
			Parameters: []api.PgParamMetadata{
				{Name: "shared_buffers", Type: "string", CurrentValue: "128MB", RequiresRestart: true},
				{Name: "work_mem", Type: "string", CurrentValue: "4MB", RequiresRestart: false},
			},
			RequiresRestart: false,
		},
	}
	h := cluster.NewConfigHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/postgres-config", nil,
		map[string]string{"name": "prod"})
	h.GetPostgresConfig(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp api.PgConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Parameters) != 2 {
		t.Errorf("param count: got %d, want 2", len(resp.Parameters))
	}
	if resp.Parameters[0].Name != "shared_buffers" {
		t.Errorf("first param name: got %q, want shared_buffers", resp.Parameters[0].Name)
	}
	if !resp.Parameters[0].RequiresRestart {
		t.Error("shared_buffers should require restart")
	}
}

// TestGetPostgresConfig_Handler_NotFound verifies 404 for missing cluster.
func TestGetPostgresConfig_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubConfigService{getErr: fmt.Errorf("cluster \"ghost\" not found")}
	h := cluster.NewConfigHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/ghost/postgres-config", nil,
		map[string]string{"name": "ghost"})
	h.GetPostgresConfig(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestUpdatePostgresConfig_Handler_OK verifies UpdatePostgresConfig returns 200.
func TestUpdatePostgresConfig_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubConfigService{
		putResult: &api.PgConfigResponse{
			Parameters: []api.PgParamMetadata{
				{Name: "work_mem", Type: "string", CurrentValue: "64MB", RequiresRestart: false},
			},
			RequiresRestart: false,
		},
	}
	h := cluster.NewConfigHandler(svc)

	body, _ := json.Marshal(api.UpdatePgConfigRequest{
		Parameters: map[string]string{"work_mem": "64MB"},
	})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/prod/postgres-config",
		body, map[string]string{"name": "prod"})
	h.UpdatePostgresConfig(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp api.PgConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Parameters[0].CurrentValue != "64MB" {
		t.Errorf("work_mem: got %q, want 64MB", resp.Parameters[0].CurrentValue)
	}
}

// TestUpdatePostgresConfig_Handler_InvalidBody verifies 400 for bad JSON.
func TestUpdatePostgresConfig_Handler_InvalidBody(t *testing.T) {
	t.Parallel()

	svc := &stubConfigService{}
	h := cluster.NewConfigHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/prod/postgres-config",
		[]byte("not json"), map[string]string{"name": "prod"})
	h.UpdatePostgresConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdatePostgresConfig_Handler_ValidationError verifies 422 for invalid param value.
func TestUpdatePostgresConfig_Handler_ValidationError(t *testing.T) {
	t.Parallel()

	svc := &stubConfigService{putErr: fmt.Errorf("validation: invalid value for max_connections")}
	h := cluster.NewConfigHandler(svc)

	body, _ := json.Marshal(api.UpdatePgConfigRequest{
		Parameters: map[string]string{"max_connections": "abc"},
	})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/prod/postgres-config",
		body, map[string]string{"name": "prod"})
	h.UpdatePostgresConfig(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}
