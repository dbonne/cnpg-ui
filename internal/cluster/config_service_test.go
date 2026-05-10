package cluster_test

import (
	"context"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/cluster"
)

func fakeClusterWithParams(name, ns string, params map[string]string) *cnpgv1.Cluster {
	return &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: cnpgv1.ClusterSpec{
			Instances: 1,
			PostgresConfiguration: cnpgv1.PostgresConfiguration{
				Parameters: params,
			},
		},
	}
}

// TestGetConfig_ReturnsCurrentParameters verifies that GetConfig returns
// the current parameters from the Cluster spec with type metadata.
func TestGetConfig_ReturnsCurrentParameters(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeClusterWithParams("prod", "default", map[string]string{
		"shared_buffers": "256MB",
		"work_mem":       "4MB",
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewConfigService(fakeClient, "default")
	resp, err := svc.GetConfig(context.Background(), "prod")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}

	if len(resp.Parameters) != 2 {
		t.Fatalf("param count: got %d, want 2", len(resp.Parameters))
	}

	// Find shared_buffers and verify it requires restart
	var found *api.PgParamMetadata
	for i := range resp.Parameters {
		if resp.Parameters[i].Name == "shared_buffers" {
			found = &resp.Parameters[i]
			break
		}
	}
	if found == nil {
		t.Fatal("shared_buffers not found in parameters")
	}
	if found.CurrentValue != "256MB" {
		t.Errorf("shared_buffers value: got %q, want 256MB", found.CurrentValue)
	}
	if !found.RequiresRestart {
		t.Error("shared_buffers should require restart")
	}
}

// TestGetConfig_HotReloadableParam verifies work_mem does NOT require restart.
func TestGetConfig_HotReloadableParam(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeClusterWithParams("prod", "default", map[string]string{
		"work_mem": "64MB",
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewConfigService(fakeClient, "default")
	resp, err := svc.GetConfig(context.Background(), "prod")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}

	if len(resp.Parameters) != 1 {
		t.Fatalf("param count: got %d, want 1", len(resp.Parameters))
	}
	if resp.Parameters[0].RequiresRestart {
		t.Error("work_mem should NOT require restart")
	}
}

// TestGetConfig_NotFound verifies GetConfig returns an error for missing clusters.
func TestGetConfig_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewConfigService(fakeClient, "default")

	_, err := svc.GetConfig(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent cluster")
	}
}

// TestUpdateConfig_PatchesParameters verifies UpdateConfig patches the Cluster spec.
func TestUpdateConfig_PatchesParameters(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeClusterWithParams("prod", "default", map[string]string{
		"work_mem": "4MB",
	})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewConfigService(fakeClient, "default")
	resp, err := svc.UpdateConfig(context.Background(), "prod", api.UpdatePgConfigRequest{
		Parameters: map[string]string{"work_mem": "64MB"},
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// Find work_mem in result
	var found *api.PgParamMetadata
	for i := range resp.Parameters {
		if resp.Parameters[i].Name == "work_mem" {
			found = &resp.Parameters[i]
			break
		}
	}
	if found == nil {
		t.Fatal("work_mem not found in update response")
	}
	if found.CurrentValue != "64MB" {
		t.Errorf("work_mem: got %q, want 64MB", found.CurrentValue)
	}
}

// TestUpdateConfig_RequiresRestartFlagSet verifies that updating a restart-required
// param sets RequiresRestart=true in the response.
func TestUpdateConfig_RequiresRestartFlagSet(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeClusterWithParams("prod", "default", map[string]string{})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewConfigService(fakeClient, "default")
	resp, err := svc.UpdateConfig(context.Background(), "prod", api.UpdatePgConfigRequest{
		Parameters: map[string]string{"shared_buffers": "512MB"},
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	if !resp.RequiresRestart {
		t.Error("RequiresRestart should be true when updating shared_buffers")
	}
}
