package pooler_test

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
	"github.com/dbonne/cnpg-ui/internal/pooler"
)

// stubService is a test double for pooler.Service.
type stubService struct {
	listResult []api.PoolerSummary
	listErr    error
	getResult  *api.PoolerSummary
	getErr     error
}

func (s *stubService) List(_ context.Context, _ string) ([]api.PoolerSummary, error) {
	return s.listResult, s.listErr
}
func (s *stubService) Get(_ context.Context, _, _ string) (*api.PoolerSummary, error) {
	return s.getResult, s.getErr
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

// TestListPoolers_Handler_OK verifies ListPoolers returns 200 with pool array.
func TestListPoolers_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		listResult: []api.PoolerSummary{
			{Name: "prod-rw", ClusterName: "prod", Type: "rw", Instances: 2},
			{Name: "prod-ro", ClusterName: "prod", Type: "ro", Instances: 1},
		},
	}
	h := pooler.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/poolers", nil,
		map[string]string{"name": "prod"})
	h.ListPoolers(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var poolers []api.PoolerSummary
	if err := json.NewDecoder(w.Body).Decode(&poolers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(poolers) != 2 {
		t.Errorf("count: got %d, want 2", len(poolers))
	}
}

// TestListPoolers_Handler_Empty verifies ListPoolers returns 200 with empty array.
func TestListPoolers_Handler_Empty(t *testing.T) {
	t.Parallel()

	svc := &stubService{listResult: []api.PoolerSummary{}}
	h := pooler.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/poolers", nil,
		map[string]string{"name": "prod"})
	h.ListPoolers(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var poolers []api.PoolerSummary
	if err := json.NewDecoder(w.Body).Decode(&poolers); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(poolers) != 0 {
		t.Errorf("count: got %d, want 0", len(poolers))
	}
}

// TestGetPooler_Handler_OK verifies GetPooler returns 200 with pool detail.
func TestGetPooler_Handler_OK(t *testing.T) {
	t.Parallel()

	svc := &stubService{
		getResult: &api.PoolerSummary{
			Name: "prod-rw", ClusterName: "prod", Type: "rw", Instances: 2, PoolMode: "session",
		},
	}
	h := pooler.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/poolers/prod-rw", nil,
		map[string]string{"name": "prod", "poolerName": "prod-rw"})
	h.GetPooler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var p api.PoolerSummary
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Name != "prod-rw" {
		t.Errorf("Name: got %q, want prod-rw", p.Name)
	}
	if p.Type != "rw" {
		t.Errorf("Type: got %q, want rw", p.Type)
	}
}

// TestGetPooler_Handler_NotFound verifies GetPooler returns 404.
func TestGetPooler_Handler_NotFound(t *testing.T) {
	t.Parallel()

	svc := &stubService{getErr: fmt.Errorf("pooler \"ghost\" not found")}
	h := pooler.NewHandler(svc)

	w := httptest.NewRecorder()
	r := routedRequest(http.MethodGet, "/api/v1/clusters/prod/poolers/ghost", nil,
		map[string]string{"name": "prod", "poolerName": "ghost"})
	h.GetPooler(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}
