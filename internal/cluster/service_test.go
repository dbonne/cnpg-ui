package cluster_test

import (
	"context"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/cluster"
	"github.com/dbonne/cnpg-ui/internal/k8s"
)

func newScheme() *runtime.Scheme {
	return k8s.NewScheme()
}

// fakeCluster builds a CNPG Cluster for tests.
func fakeCluster(name, namespace, phase string, instances int) *cnpgv1.Cluster {
	return &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: instances,
		},
		Status: cnpgv1.ClusterStatus{
			Phase:          phase,
			Instances:      instances,
			ReadyInstances: instances,
			CurrentPrimary: name + "-1",
		},
	}
}

// TestListClusters_ReturnsAllClusters verifies that List returns all clusters
// with correct field mapping and normalized status.
func TestListClusters_ReturnsAllClusters(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 3)
	dev := fakeCluster("dev", "default", cnpgv1.PhaseSwitchover, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl, dev).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	clusters, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}

	// Find prod cluster and verify status mapping
	var found *api.ClusterSummary
	for i := range clusters {
		if clusters[i].Name == "prod" {
			found = &clusters[i]
			break
		}
	}
	if found == nil {
		t.Fatal("prod cluster not found in list")
	}
	if found.Status != api.StatusHealthy {
		t.Errorf("prod status: got %q, want HEALTHY", found.Status)
	}
	if found.Instances != 3 {
		t.Errorf("prod instances: got %d, want 3", found.Instances)
	}
}

// TestListClusters_Empty verifies that List returns an empty slice when no
// clusters exist (not nil, which would break JSON marshaling to {}).
func TestListClusters_Empty(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	clusters, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if clusters == nil {
		t.Error("List returned nil, want empty slice")
	}
	if len(clusters) != 0 {
		t.Errorf("got %d clusters, want 0", len(clusters))
	}
}

// TestGetCluster_Found verifies Get returns the correct cluster detail.
func TestGetCluster_Found(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 2)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	detail, err := svc.Get(context.Background(), "prod")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if detail.Name != "prod" {
		t.Errorf("Name: got %q, want %q", detail.Name, "prod")
	}
	if detail.Status != api.StatusHealthy {
		t.Errorf("Status: got %q, want HEALTHY", detail.Status)
	}
	if detail.Instances != 2 {
		t.Errorf("Instances: got %d, want 2", detail.Instances)
	}
}

// TestGetCluster_NotFound verifies Get returns an error for missing clusters.
func TestGetCluster_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	_, err := svc.Get(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent cluster, got nil")
	}
}

// TestCreateCluster_MinimalSpec verifies that a cluster is created from a
// minimal request (name + instances + storageSize) and the CR exists after.
func TestCreateCluster_MinimalSpec(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	req := api.CreateClusterRequest{
		Name:        "new-cluster",
		Instances:   1,
		StorageSize: "10Gi",
	}
	detail, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if detail.Name != "new-cluster" {
		t.Errorf("Name: got %q, want %q", detail.Name, "new-cluster")
	}
	if detail.Instances != 1 {
		t.Errorf("Instances: got %d, want 1", detail.Instances)
	}

	// Verify the CR was actually created in the fake client
	var list cnpgv1.ClusterList
	if err := fakeClient.List(context.Background(), &list); err != nil {
		t.Fatalf("list after create: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("CR count after create: got %d, want 1", len(list.Items))
	}
}

// TestUpdateCluster_Instances verifies Update patches instances field.
func TestUpdateCluster_Instances(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		WithStatusSubresource(cl).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	detail, err := svc.Update(context.Background(), "prod", api.UpdateClusterRequest{Instances: 4})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if detail.Instances != 4 {
		t.Errorf("Instances after update: got %d, want 4", detail.Instances)
	}
}

// TestUpdateCluster_NotFound verifies Update returns an error for missing clusters.
func TestUpdateCluster_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	_, err := svc.Update(context.Background(), "ghost", api.UpdateClusterRequest{Instances: 2})
	if err == nil {
		t.Fatal("expected error for non-existent cluster, got nil")
	}
}

// TestUpdateCluster_InvalidStorageSize verifies Update rejects bad storage quantity.
func TestUpdateCluster_InvalidStorageSize(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	_, err := svc.Update(context.Background(), "prod", api.UpdateClusterRequest{StorageSize: "not-a-size"})
	if err == nil {
		t.Fatal("expected error for invalid storageSize, got nil")
	}
}

// TestScaleCluster_Up verifies that Scale updates the instances field in the CR.
func TestScaleCluster_Up(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		WithStatusSubresource(cl).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	detail, err := svc.Scale(context.Background(), "prod", api.ScaleClusterRequest{Instances: 3})
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	if detail.Instances != 3 {
		t.Errorf("Instances after scale: got %d, want 3", detail.Instances)
	}
}

// TestScaleCluster_NotFound verifies Scale returns an error for missing clusters.
func TestScaleCluster_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	_, err := svc.Scale(context.Background(), "ghost", api.ScaleClusterRequest{Instances: 3})
	if err == nil {
		t.Fatal("expected error for non-existent cluster, got nil")
	}
}

// TestDeleteCluster_Existing verifies that an existing cluster is deleted.
func TestDeleteCluster_Existing(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	cl := fakeCluster("prod", "default", cnpgv1.PhaseHealthy, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cl).
		Build()

	svc := cluster.NewService(fakeClient, "default")
	if err := svc.Delete(context.Background(), "prod"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it was removed
	var list cnpgv1.ClusterList
	if err := fakeClient.List(context.Background(), &list); err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("CR count after delete: got %d, want 0", len(list.Items))
	}
}

// TestDeleteCluster_NotFound verifies Delete returns an error for missing clusters.
func TestDeleteCluster_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := cluster.NewService(fakeClient, "default")

	err := svc.Delete(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent cluster, got nil")
	}
}
