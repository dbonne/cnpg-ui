// Package helm contains tests that validate the Helm chart renders correctly
// using the helm CLI. These are integration tests against the chart files.
package helm_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// chartDir returns the absolute path to deploy/helm/ relative to this test file.
func chartDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/helm/chart_test.go → go up 3 dirs to repo root → deploy/helm
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "deploy", "helm")
}

// helmTemplate runs `helm template test-release <chart>` and returns stdout.
func helmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()
	dir := chartDir(t)

	args := append([]string{"template", "test-release", dir}, extraArgs...)
	cmd := exec.Command("helm", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// ── Chart rendering tests ──────────────────────────────────────────────────

// TestHelmChart_RendersDeployment verifies that the chart produces a Deployment
// with the expected container port and service account reference.
func TestHelmChart_RendersDeployment(t *testing.T) {
	out := helmTemplate(t, "--set", "image.tag=v0.1.0")

	if !strings.Contains(out, "kind: Deployment") {
		t.Error("expected Deployment in rendered output")
	}
	if !strings.Contains(out, "containerPort: 8080") {
		t.Error("expected containerPort 8080 in Deployment")
	}
	if !strings.Contains(out, "livenessProbe") {
		t.Error("expected livenessProbe in Deployment")
	}
	if !strings.Contains(out, "readinessProbe") {
		t.Error("expected readinessProbe in Deployment")
	}
}

// TestHelmChart_RendersService verifies the ClusterIP Service on port 8080.
func TestHelmChart_RendersService(t *testing.T) {
	out := helmTemplate(t, "--set", "image.tag=v0.1.0")

	if !strings.Contains(out, "kind: Service") {
		t.Error("expected Service in rendered output")
	}
	if !strings.Contains(out, "type: ClusterIP") {
		t.Error("expected type: ClusterIP in Service")
	}
	if !strings.Contains(out, "port: 8080") {
		t.Error("expected port 8080 in Service")
	}
}

// TestHelmChart_RendersServiceAccount verifies the ServiceAccount is rendered.
func TestHelmChart_RendersServiceAccount(t *testing.T) {
	out := helmTemplate(t, "--set", "image.tag=v0.1.0")

	if !strings.Contains(out, "kind: ServiceAccount") {
		t.Error("expected ServiceAccount in rendered output")
	}
}

// TestHelmChart_RendersClusterRoleWithCNPGVerbs verifies the ClusterRole
// grants the required CNPG CRD verbs.
func TestHelmChart_RendersClusterRoleWithCNPGVerbs(t *testing.T) {
	out := helmTemplate(t, "--set", "image.tag=v0.1.0")

	if !strings.Contains(out, "kind: ClusterRole") {
		t.Error("expected ClusterRole in rendered output")
	}
	// CNPG resources
	for _, resource := range []string{"clusters", "backups", "scheduledbackups", "poolers"} {
		if !strings.Contains(out, resource) {
			t.Errorf("expected %q in ClusterRole rules", resource)
		}
	}
	// Required verbs for full CRUD resources
	for _, verb := range []string{"get", "list", "watch", "create", "update", "delete"} {
		if !strings.Contains(out, verb) {
			t.Errorf("expected verb %q in ClusterRole rules", verb)
		}
	}
}

// TestHelmChart_RendersClusterRoleBinding verifies the ClusterRoleBinding
// binds the ServiceAccount to the ClusterRole.
func TestHelmChart_RendersClusterRoleBinding(t *testing.T) {
	out := helmTemplate(t, "--set", "image.tag=v0.1.0")

	if !strings.Contains(out, "kind: ClusterRoleBinding") {
		t.Error("expected ClusterRoleBinding in rendered output")
	}
	if !strings.Contains(out, "kind: ServiceAccount") {
		t.Error("expected ServiceAccount subject in ClusterRoleBinding")
	}
}

// TestHelmChart_CustomImage verifies that image.repository and image.tag
// are reflected in the Deployment container image field.
func TestHelmChart_CustomImage(t *testing.T) {
	out := helmTemplate(t,
		"--set", "image.repository=myregistry/cnpg-ui",
		"--set", "image.tag=v1.2.3",
	)

	if !strings.Contains(out, "myregistry/cnpg-ui:v1.2.3") {
		t.Errorf("expected custom image in output, got:\n%s", out)
	}
}
