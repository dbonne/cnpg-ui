package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReader is the interface for reading CNPG Cluster resources.
// It is implemented by clusterReader (backed by controller-runtime client)
// and can be mocked in unit tests.
type ClusterReader interface {
	// ListClusters returns all Cluster resources in the given namespace.
	ListClusters(ctx context.Context, namespace string) ([]cnpgv1.Cluster, error)
	// GetCluster returns the named Cluster resource in the given namespace.
	GetCluster(ctx context.Context, namespace, name string) (*cnpgv1.Cluster, error)
}

// clusterReader implements ClusterReader using a controller-runtime client.
type clusterReader struct {
	client client.Client
}

// NewClusterReader creates a ClusterReader backed by the given controller-runtime client.
// In production, pass the client created by NewClient. In tests, pass a fake.Client.
func NewClusterReader(c client.Client) ClusterReader {
	return &clusterReader{client: c}
}

// ListClusters lists all CNPG Cluster resources in the given namespace.
func (r *clusterReader) ListClusters(ctx context.Context, namespace string) ([]cnpgv1.Cluster, error) {
	var list cnpgv1.ClusterList
	if err := r.client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list clusters in %q: %w", namespace, err)
	}
	return list.Items, nil
}

// GetCluster retrieves a single CNPG Cluster by namespace and name.
func (r *clusterReader) GetCluster(ctx context.Context, namespace, name string) (*cnpgv1.Cluster, error) {
	var cluster cnpgv1.Cluster
	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := r.client.Get(ctx, key, &cluster); err != nil {
		return nil, fmt.Errorf("get cluster %q/%q: %w", namespace, name, err)
	}
	return &cluster, nil
}

// NewScheme builds a runtime.Scheme that includes:
//   - Core Kubernetes types (Pods, Secrets, ConfigMaps, etc.)
//   - CNPG CRD types (Cluster, Backup, ScheduledBackup, Pooler)
//
// This scheme is used to construct both the production controller-runtime
// client and the fake.Client used in tests.
func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()

	// Core Kubernetes types.
	_ = clientgoscheme.AddToScheme(s)

	// CNPG CRD types.
	_ = cnpgv1.AddToScheme(s)

	// corev1 is included via clientgoscheme, but be explicit for clarity.
	_ = corev1.AddToScheme(s)

	return s
}

// NewClient creates a production controller-runtime client.
// It tries in-cluster configuration first (pod environment); if that fails
// it falls back to ~/.kube/config for local development.
//
// Returns an error if neither configuration source is available.
func NewClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// Not running inside a cluster — try kubeconfig.
		cfg, err = loadKubeconfig()
		if err != nil {
			return nil, fmt.Errorf("no valid k8s config found (tried in-cluster and kubeconfig): %w", err)
		}
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create controller-runtime client: %w", err)
	}
	return c, nil
}

// loadKubeconfig loads a REST config from ~/.kube/config.
func loadKubeconfig() (*rest.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	kubeconfigPath := filepath.Join(home, ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}
