package integration_test

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/api"
	"github.com/dbonne/cnpg-ui/internal/cluster"
)

// TestClusterCRUD_WithEnvtest exercises the full cluster CRUD lifecycle
// against a real K8s API server running via envtest.
//
// Scenario:
//  1. Create a CNPG Cluster CR in the test namespace
//  2. List via cluster.Service → expect 1 cluster in the response
//  3. Get by name → verify the name matches
//  4. Delete → list again → expect 0 clusters
func TestClusterCRUD_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-cluster-crud"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a minimal CNPG Cluster CR directly via the envtest client.
	cr := &cnpgv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: ns,
		},
		Spec: cnpgv1.ClusterSpec{
			Instances: 1,
			StorageConfiguration: cnpgv1.StorageConfiguration{
				Size: "1Gi",
			},
		},
	}
	if err := k8sClient.Create(ctx, cr); err != nil {
		t.Fatalf("create CNPG Cluster CR: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), cr)
	})

	svc := cluster.NewService(k8sClient, ns)

	// List — expect 1 cluster.
	clusters, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(clusters) != 1 {
		t.Errorf("List: got %d clusters, want 1", len(clusters))
	}
	if len(clusters) > 0 && clusters[0].Name != "test-cluster" {
		t.Errorf("List[0].Name = %q, want %q", clusters[0].Name, "test-cluster")
	}

	// Get — verify by name.
	detail, err := svc.Get(ctx, "test-cluster")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Name != "test-cluster" {
		t.Errorf("Get.Name = %q, want %q", detail.Name, "test-cluster")
	}

	// Delete — direct K8s delete.
	if err := k8sClient.Delete(ctx, cr); err != nil {
		t.Fatalf("delete Cluster CR: %v", err)
	}

	// List again — expect 0. Allow brief propagation.
	var remaining int
	for i := 0; i < 5; i++ {
		cls, _ := svc.List(ctx)
		remaining = len(cls)
		if remaining == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if remaining != 0 {
		t.Errorf("after delete: got %d clusters, want 0", remaining)
	}
}

// TestClusterList_EmptyNamespace_WithEnvtest verifies that listing clusters in
// an empty namespace returns an empty result (not an error).
func TestClusterList_EmptyNamespace_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-cluster-empty"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	svc := cluster.NewService(k8sClient, ns)
	clusters, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("List: got %d clusters in empty namespace, want 0", len(clusters))
	}
}

// TestClusterCreate_WithEnvtest verifies that creating a cluster via the service
// stores it in the K8s API server and makes it retrievable.
func TestClusterCreate_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-cluster-create"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc := cluster.NewService(k8sClient, ns)

	req := api.CreateClusterRequest{
		Name:        "created-cluster",
		Instances:   2,
		StorageSize: "5Gi",
	}
	detail, err := svc.Create(ctx, &req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if detail.Name != "created-cluster" {
		t.Errorf("Create.Name = %q, want %q", detail.Name, "created-cluster")
	}
	if detail.Instances != 2 {
		t.Errorf("Create.Instances = %d, want 2", detail.Instances)
	}

	// Verify it's actually in K8s.
	var stored cnpgv1.Cluster
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "created-cluster"}, &stored); err != nil {
		t.Fatalf("K8s Get after Create: %v", err)
	}
	if stored.Spec.Instances != 2 {
		t.Errorf("stored.Spec.Instances = %d, want 2", stored.Spec.Instances)
	}

	// Cleanup.
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), &stored)
	})
}
