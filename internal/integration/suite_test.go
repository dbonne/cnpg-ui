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

	// Locate CNPG CRD files from the cached module.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	_ = thisFile

	// Resolve CNPG API module path to find bundled CRDs.
	// The cloudnative-pg/api module ships CRD YAMLs under config/crd/bases/.
	// We use the module cache path to load them.
	cnpgModuleDir := cnpgCRDDir(t)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{cnpgModuleDir},
		ErrorIfCRDPathMissing: false, // CRD dir may not exist in all versions
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

// cnpgCRDDir returns the CRD directory from the cloudnative-pg/api module cache.
// Returns an empty string if not found (envtest will ignore missing paths with
// ErrorIfCRDPathMissing: false).
func cnpgCRDDir(t *testing.T) string {
	t.Helper()
	modPath := os.Getenv("GOPATH")
	if modPath == "" {
		home, _ := os.UserHomeDir()
		modPath = filepath.Join(home, "go")
	}
	// Try to find the CNPG api module in the module cache.
	cacheBase := filepath.Join(modPath, "pkg", "mod", "github.com", "cloudnative-pg")
	entries, err := os.ReadDir(cacheBase)
	if err != nil {
		t.Logf("could not read CNPG module cache at %s: %v", cacheBase, err)
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		crdPath := filepath.Join(cacheBase, e.Name(), "config", "crd", "bases")
		if _, err := os.Stat(crdPath); err == nil {
			return crdPath
		}
	}
	return ""
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
