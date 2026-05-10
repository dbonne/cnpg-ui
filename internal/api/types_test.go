package api_test

import (
	"encoding/json"
	"testing"

	"github.com/dbonne/cnpg-ui/internal/api"
)

// TestClusterSummaryJSONRoundtrip verifies that ClusterSummary serializes
// correctly and that required fields are present in the JSON output.
func TestClusterSummaryJSONRoundtrip(t *testing.T) {
	t.Parallel()

	cs := api.ClusterSummary{
		Name:           "prod",
		Namespace:      "default",
		Instances:      3,
		ReadyInstances: 2,
		Status:         api.StatusHealthy,
		Phase:          "Cluster in healthy state",
	}

	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("marshal ClusterSummary: %v", err)
	}

	var out api.ClusterSummary
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal ClusterSummary: %v", err)
	}

	if out.Name != "prod" {
		t.Errorf("Name: got %q, want %q", out.Name, "prod")
	}
	if out.Instances != 3 {
		t.Errorf("Instances: got %d, want 3", out.Instances)
	}
	if out.Status != api.StatusHealthy {
		t.Errorf("Status: got %q, want %q", out.Status, api.StatusHealthy)
	}
}

// TestErrorJSONRoundtrip verifies that the API Error type serializes to
// the schema expected by the spec: {error, code, details?}.
func TestErrorJSONRoundtrip(t *testing.T) {
	t.Parallel()

	ae := api.APIError{
		Error:   "resource not found",
		Code:    api.CodeNotFound,
		Details: nil,
	}

	data, err := json.Marshal(ae)
	if err != nil {
		t.Fatalf("marshal APIError: %v", err)
	}

	// Verify JSON field names match spec
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, ok := raw["error"]; !ok {
		t.Error("expected field 'error' in JSON, not found")
	}
	if _, ok := raw["code"]; !ok {
		t.Error("expected field 'code' in JSON, not found")
	}
	if raw["error"] != "resource not found" {
		t.Errorf("error field: got %v, want %q", raw["error"], "resource not found")
	}
}

// TestNormalizedStatusValues verifies the four status constants match the spec.
func TestNormalizedStatusValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status api.NormalizedStatus
		want   string
	}{
		{api.StatusHealthy, "HEALTHY"},
		{api.StatusTransient, "TRANSIENT"},
		{api.StatusFault, "FAULT"},
		{api.StatusHibernated, "HIBERNATED"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if string(tc.status) != tc.want {
				t.Errorf("NormalizedStatus: got %q, want %q", tc.status, tc.want)
			}
		})
	}
}

// TestCreateClusterRequestValidation verifies the CreateClusterRequest JSON fields.
func TestCreateClusterRequestValidation(t *testing.T) {
	t.Parallel()

	req := api.CreateClusterRequest{
		Name:        "test-cluster",
		Instances:   3,
		StorageSize: "10Gi",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out api.CreateClusterRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Name != "test-cluster" {
		t.Errorf("Name: got %q, want %q", out.Name, "test-cluster")
	}
	if out.Instances != 3 {
		t.Errorf("Instances: got %d, want 3", out.Instances)
	}
	if out.StorageSize != "10Gi" {
		t.Errorf("StorageSize: got %q, want %q", out.StorageSize, "10Gi")
	}
}
