package k8s_test

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// TestNewScheme verifies the scheme contains CNPG and core types.
func TestNewScheme(t *testing.T) {
	t.Parallel()

	s := k8s.NewScheme()

	// CNPG Cluster must be registered.
	gvks, _, err := s.ObjectKinds(&cnpgv1.Cluster{})
	if err != nil {
		t.Fatalf("Cluster not registered in scheme: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("expected at least one GVK for Cluster, got none")
	}

	// Core v1 Pod must be registered (from k8s.io/client-go).
	gvks, _, err = s.ObjectKinds(&corev1.Pod{})
	if err != nil {
		t.Fatalf("Pod not registered in scheme: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("expected at least one GVK for Pod, got none")
	}
}

// TestClientListClusters verifies the ClusterReader interface using a fake client.
func TestClientListClusters(t *testing.T) {
	t.Parallel()

	scheme := k8s.NewScheme()

	existing := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "default",
		},
		Status: cnpgv1.ClusterStatus{
			Phase: cnpgv1.PhaseHealthy,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	reader := k8s.NewClusterReader(fakeClient)

	clusters, err := reader.ListClusters(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListClusters returned error: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Name != "my-cluster" {
		t.Errorf("expected cluster name %q, got %q", "my-cluster", clusters[0].Name)
	}
}

// TestClientListClustersMultipleNamespaces verifies filtering by namespace.
func TestClientListClustersMultipleNamespaces(t *testing.T) {
	t.Parallel()

	scheme := k8s.NewScheme()

	objects := []runtime.Object{
		&cnpgv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "ns-a"},
		},
		&cnpgv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-b", Namespace: "ns-b"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		Build()

	reader := k8s.NewClusterReader(fakeClient)

	// Only ns-a clusters.
	clusters, err := reader.ListClusters(context.Background(), "ns-a")
	if err != nil {
		t.Fatalf("ListClusters returned error: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster in ns-a, got %d", len(clusters))
	}
	if clusters[0].Name != "cluster-a" {
		t.Errorf("expected cluster-a, got %q", clusters[0].Name)
	}
}

// TestClientGetCluster verifies single cluster retrieval.
func TestClientGetCluster(t *testing.T) {
	t.Parallel()

	scheme := k8s.NewScheme()

	existing := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-cluster",
			Namespace: "prod",
		},
		Status: cnpgv1.ClusterStatus{
			Phase: cnpgv1.PhaseSwitchover,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	reader := k8s.NewClusterReader(fakeClient)

	cluster, err := reader.GetCluster(context.Background(), "prod", "target-cluster")
	if err != nil {
		t.Fatalf("GetCluster returned error: %v", err)
	}
	if cluster.Name != "target-cluster" {
		t.Errorf("expected target-cluster, got %q", cluster.Name)
	}
	if cluster.Status.Phase != cnpgv1.PhaseSwitchover {
		t.Errorf("expected phase %q, got %q", cnpgv1.PhaseSwitchover, cluster.Status.Phase)
	}
}

// TestInformerManagerLifecycle verifies that Start/Stop does not panic
// and that WaitForSync returns within timeout on a context cancel.
func TestInformerManagerLifecycle(t *testing.T) {
	t.Parallel()

	// A nil rest.Config signals test mode — returns a no-op manager.
	mgr := k8s.NewInformerManager(nil, k8s.NewScheme())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start must not panic (no-op in test mode).
	mgr.Start(ctx)

	// WaitForSync returns false in no-op mode without blocking past timeout.
	synced := mgr.WaitForSync(ctx)
	if synced {
		t.Error("expected WaitForSync to return false for no-op manager")
	}
}
