package cluster_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/cluster"
)

// stubService is a test double for cluster.Service.
type stubService struct {
	listResult   []api.ClusterSummary
	listErr      error
	getResult    *api.ClusterDetail
	getErr       error
	createResult *api.ClusterDetail
	createErr    error
	updateResult *api.ClusterDetail
	updateErr    error
	scaleResult  *api.ClusterDetail
	scaleErr     error
	deleteErr    error
}

func (s *stubService) List(_ context.Context) ([]api.ClusterSummary, error) {
	return s.listResult, s.listErr
}
func (s *stubService) Get(_ context.Context, _ string) (*api.ClusterDetail, error) {
	return s.getResult, s.getErr
}
func (s *stubService) Create(_ context.Context, _ *api.CreateClusterRequest) (*api.ClusterDetail, error) {
	return s.createResult, s.createErr
}
func (s *stubService) Update(_ context.Context, _ string, _ api.UpdateClusterRequest) (*api.ClusterDetail, error) {
	return s.updateResult, s.updateErr
}
func (s *stubService) Scale(_ context.Context, _ string, _ api.ScaleClusterRequest) (*api.ClusterDetail, error) {
	return s.scaleResult, s.scaleErr
}
func (s *stubService) Delete(_ context.Context, _ string) error {
	return s.deleteErr
}

// routedRequest creates a request with chi URL params set, mimicking a real chi router.
func routedRequest(method, path string, body []byte, params map[string]string) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}

	// Inject chi URL params
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r
}

// TestListClusters_Handler_OK verifies ListClusters returns 200 with a JSON array.
func TestListClusters_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		listResult: []api.ClusterSummary{
			{Name: "prod", Namespace: "default", Instances: 3, ReadyInstances: 3, Status: api.StatusHealthy},
			{Name: "dev", Namespace: "default", Instances: 1, ReadyInstances: 1, Status: api.StatusTransient},
		},
	}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	h.ListClusters(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var clusters []api.ClusterSummary
	if err := json.NewDecoder(w.Body).Decode(&clusters); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(clusters) != 2 {
		t.Errorf("clusters count: got %d, want 2", len(clusters))
	}
}

// TestListClusters_Handler_ServiceError verifies that a service error returns 500.
func TestListClusters_Handler_ServiceError(t *testing.T) {
	t.Parallel()

	svc := &stubService{listErr: fmt.Errorf("k8s unavailable")}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	h.ListClusters(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// TestGetCluster_Handler_OK verifies GetCluster returns 200 with ClusterDetail.
func TestGetCluster_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		getResult: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name: "prod", Namespace: "default",
				Instances: 3, ReadyInstances: 3, Status: api.StatusHealthy,
			},
		},
	}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod", nil, map[string]string{"name": "prod"})
	h.GetCluster(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var detail api.ClusterDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Name != "prod" {
		t.Errorf("Name: got %q, want %q", detail.Name, "prod")
	}
}

// TestGetCluster_Handler_NotFound verifies GetCluster returns 404 for missing clusters.
func TestGetCluster_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubService{getErr: fmt.Errorf("cluster \"ghost\" not found")}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/ghost", nil, map[string]string{"name": "ghost"})
	h.GetCluster(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestCreateCluster_Handler_OK verifies CreateCluster returns 201 on success.
func TestCreateCluster_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		createResult: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name: "new-cluster", Namespace: "default",
				Instances: 1, Status: api.StatusTransient,
			},
		},
	}
	h := cluster.NewHandler(svc)

	body, _ := json.Marshal(api.CreateClusterRequest{Name: "new-cluster", Instances: 1, StorageSize: "10Gi"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.CreateCluster(w, r)

	if w.Code != http.StatusCreated {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusCreated)
	}

	var detail api.ClusterDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Name != "new-cluster" {
		t.Errorf("Name: got %q, want %q", detail.Name, "new-cluster")
	}
}

// TestCreateCluster_Handler_InvalidBody verifies CreateCluster returns 400 for bad JSON.
func TestCreateCluster_Handler_InvalidBody(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/clusters",
		strings.NewReader("not json"))
	r.Header.Set("Content-Type", "application/json")
	h.CreateCluster(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateCluster_Handler_OK verifies UpdateCluster returns 200 with updated detail.
func TestUpdateCluster_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		updateResult: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name: "prod", Instances: 5, Status: api.StatusHealthy,
			},
			StorageSize: "20Gi",
		},
	}
	h := cluster.NewHandler(svc)

	body, _ := json.Marshal(api.UpdateClusterRequest{Instances: 5, StorageSize: "20Gi"})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/prod",
		body, map[string]string{"name": "prod"})
	h.UpdateCluster(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var detail api.ClusterDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Instances != 5 {
		t.Errorf("Instances: got %d, want 5", detail.Instances)
	}
	if detail.StorageSize != "20Gi" {
		t.Errorf("StorageSize: got %q, want 20Gi", detail.StorageSize)
	}
}

// TestUpdateCluster_Handler_NotFound verifies UpdateCluster returns 404 for missing cluster.
func TestUpdateCluster_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubService{updateErr: fmt.Errorf("cluster \"ghost\" not found")}
	h := cluster.NewHandler(svc)

	body, _ := json.Marshal(api.UpdateClusterRequest{Instances: 3})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/ghost",
		body, map[string]string{"name": "ghost"})
	h.UpdateCluster(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestUpdateCluster_Handler_InvalidBody verifies UpdateCluster returns 400 for bad JSON.
func TestUpdateCluster_Handler_InvalidBody(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPut, "/api/v1/clusters/prod",
		[]byte("not json"), map[string]string{"name": "prod"})
	h.UpdateCluster(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestScaleCluster_Handler_OK verifies ScaleCluster returns 200.
func TestScaleCluster_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		scaleResult: &api.ClusterDetail{
			ClusterSummary: api.ClusterSummary{
				Name: "prod", Instances: 3, Status: api.StatusHealthy,
			},
		},
	}
	h := cluster.NewHandler(svc)

	body, _ := json.Marshal(api.ScaleClusterRequest{Instances: 3})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPatch, "/api/v1/clusters/prod/scale",
		body, map[string]string{"name": "prod"})
	h.ScaleCluster(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var detail api.ClusterDetail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Instances != 3 {
		t.Errorf("Instances: got %d, want 3", detail.Instances)
	}
}

// TestScaleCluster_Handler_InvalidInstances verifies that instances < 1 returns 422.
func TestScaleCluster_Handler_InvalidInstances(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	h := cluster.NewHandler(svc)

	body, _ := json.Marshal(api.ScaleClusterRequest{Instances: 0})
	w := httptest.NewRecorder()
	r := routedRequest(http.MethodPatch, "/api/v1/clusters/prod/scale",
		body, map[string]string{"name": "prod"})
	h.ScaleCluster(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

// TestDeleteCluster_Handler_OK verifies DeleteCluster returns 204.
func TestDeleteCluster_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodDelete, "/api/v1/clusters/prod", nil, map[string]string{"name": "prod"})
	h.DeleteCluster(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNoContent)
	}
}

// TestDeleteCluster_Handler_NotFound verifies DeleteCluster returns 404.
func TestDeleteCluster_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubService{deleteErr: fmt.Errorf("cluster \"ghost\" not found")}
	h := cluster.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodDelete, "/api/v1/clusters/ghost", nil, map[string]string{"name": "ghost"})
	h.DeleteCluster(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}
