// Package integration contains end-to-end tests that exercise the full stack
// against a real Kubernetes API server (via controller-runtime envtest).
//
// These tests require the KUBEBUILDER_ASSETS environment variable to point to
// a directory containing kube-apiserver and etcd binaries. When the environment
// is not set up, every test skips gracefully with an informative message.
//
// To run locally:
//
//	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
//	export KUBEBUILDER_ASSETS=$(setup-envtest use -p path 1.31.x)
//	go test ./internal/integration/...
package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// envtestScheme builds a runtime.Scheme with CNPG + core types.
func envtestScheme() *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = cnpgv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

// startEnvtest starts a real K8s API server via envtest. It installs the CNPG
// CRDs from the cloudnative-pg/api module so our CRUD operations work.
// The returned cancel function stops the env.
func startEnvtest(t *testing.T) (*envtest.Environment, client.Client) {
	t.Helper()

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set — skipping envtest integration tests")
	}

	// Locate CNPG CRD files from testdata/ bundled in this package.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	crdDir := filepath.Join(filepath.Dir(thisFile), "testdata", "crds")

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{crdDir},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("envtest Start: %v", err)
	}

	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Logf("envtest Stop: %v", err)
		}
	})

	scheme := envtestScheme()
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("build envtest client: %v", err)
	}

	return testEnv, k8sClient
}



// createNamespace creates a test namespace and registers cleanup.
func createNamespace(t *testing.T, c client.Client, name string) {
	t.Helper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := c.Create(t.Context(), ns); err != nil && !k8serrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %q: %v", name, err)
	}
	t.Cleanup(func() {
		_ = c.Delete(t.Context(), ns)
	})
}
