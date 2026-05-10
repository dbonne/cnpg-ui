package pooler_test

import (
	"context"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/k8s"
	"github.com/dbonne/cnpg-ui/internal/pooler"
)

func newScheme() *runtime.Scheme { return k8s.NewScheme() }

func fakePooler(name, ns, clusterName string, poolerType cnpgv1.PoolerType, instances int32) *cnpgv1.Pooler {
	return &cnpgv1.Pooler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: cnpgv1.PoolerSpec{
			Cluster:   cnpgv1.LocalObjectReference{Name: clusterName},
			Type:      poolerType,
			Instances: &instances,
			PgBouncer: &cnpgv1.PgBouncerSpec{
				PoolMode: cnpgv1.PgBouncerPoolModeSession,
			},
		},
	}
}

// TestListPoolers_ReturnsPoolersForCluster verifies List filters by cluster.
func TestListPoolers_ReturnsPoolersForCluster(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	p1 := fakePooler("prod-rw", "default", "prod", cnpgv1.PoolerTypeRW, 2)
	p2 := fakePooler("prod-ro", "default", "prod", cnpgv1.PoolerTypeRO, 1)
	p3 := fakePooler("other-rw", "default", "other-cluster", cnpgv1.PoolerTypeRW, 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p1, p2, p3).
		Build()

	svc := pooler.NewService(fakeClient, "default")
	poolers, err := svc.List(context.Background(), "prod")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(poolers) != 2 {
		t.Fatalf("got %d poolers, want 2", len(poolers))
	}

	// Verify type and poolMode are mapped correctly
	var rwFound bool
	for _, p := range poolers {
		if p.Name == "prod-rw" {
			rwFound = true
			if p.Type != "rw" {
				t.Errorf("Type: got %q, want rw", p.Type)
			}
			if p.Instances != 2 {
				t.Errorf("Instances: got %d, want 2", p.Instances)
			}
			if p.PoolMode != "session" {
				t.Errorf("PoolMode: got %q, want session", p.PoolMode)
			}
		}
	}
	if !rwFound {
		t.Error("prod-rw pooler not found")
	}
}

// TestListPoolers_Empty verifies List returns empty (not nil) when no poolers exist.
func TestListPoolers_Empty(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := pooler.NewService(fakeClient, "default")

	poolers, err := svc.List(context.Background(), "prod")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if poolers == nil {
		t.Error("List returned nil, want empty slice")
	}
}

// TestGetPooler_Found verifies Get returns the correct pooler.
func TestGetPooler_Found(t *testing.T) {
	t.Parallel()

	scheme := newScheme()
	p := fakePooler("prod-rw", "default", "prod", cnpgv1.PoolerTypeRW, 2)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p).
		Build()

	svc := pooler.NewService(fakeClient, "default")
	result, err := svc.Get(context.Background(), "prod", "prod-rw")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if result.Name != "prod-rw" {
		t.Errorf("Name: got %q, want prod-rw", result.Name)
	}
	if result.ClusterName != "prod" {
		t.Errorf("ClusterName: got %q, want prod", result.ClusterName)
	}
	if result.Type != "rw" {
		t.Errorf("Type: got %q, want rw", result.Type)
	}
}

// TestGetPooler_NotFound verifies Get returns an error for missing poolers.
func TestGetPooler_NotFound(t *testing.T) {
	t.Parallel()

	fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
	svc := pooler.NewService(fakeClient, "default")

	_, err := svc.Get(context.Background(), "prod", "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent pooler, got nil")
	}
}
