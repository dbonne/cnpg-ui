// Package dockerfile contains tests that validate the Dockerfile's structure.
// These are compile-time and file-structure tests — they do not require
// running a Docker daemon.
package dockerfile_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dockerfilePath returns the absolute path to the project Dockerfile.
func dockerfilePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/dockerfile/dockerfile_test.go → go up 3 dirs → repo root
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "Dockerfile")
}

// readDockerfile reads the Dockerfile and returns its content as a slice of lines.
func readDockerfile(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(dockerfilePath(t))
	if err != nil {
		t.Fatalf("open Dockerfile: %v", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	return lines
}

// containsLine returns true if any line in lines contains substr (case-insensitive).
func containsLine(lines []string, substr string) bool {
	lower := strings.ToLower(substr)
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), lower) {
			return true
		}
	}
	return false
}

// countFROM counts the number of FROM directives — indicates multi-stage build.
func countFROM(lines []string) int {
	count := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			count++
		}
	}
	return count
}

// ── Dockerfile structure tests ─────────────────────────────────────────────

// TestDockerfile_Exists verifies the Dockerfile exists at the repo root.
func TestDockerfile_Exists(t *testing.T) {
	path := dockerfilePath(t)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Dockerfile not found at %s", path)
	}
}

// TestDockerfile_IsMultiStage verifies at least 2 FROM directives (builder + runtime).
func TestDockerfile_IsMultiStage(t *testing.T) {
	lines := readDockerfile(t)
	count := countFROM(lines)
	if count < 2 {
		t.Errorf("Dockerfile has %d FROM directives, want >= 2 (multi-stage)", count)
	}
}

// TestDockerfile_ExposesPort8080 verifies that EXPOSE 8080 is declared.
func TestDockerfile_ExposesPort8080(t *testing.T) {
	lines := readDockerfile(t)
	if !containsLine(lines, "EXPOSE 8080") {
		t.Error("Dockerfile does not contain 'EXPOSE 8080'")
	}
}

// TestDockerfile_HasGoBuilder verifies the builder stage uses a Go base image.
func TestDockerfile_HasGoBuilder(t *testing.T) {
	lines := readDockerfile(t)
	foundGoImage := false
	for _, l := range lines {
		upper := strings.ToUpper(l)
		if strings.HasPrefix(upper, "FROM ") && strings.Contains(strings.ToLower(l), "golang") {
			foundGoImage = true
			break
		}
	}
	if !foundGoImage {
		t.Error("Dockerfile builder stage should use a golang base image")
	}
}

// TestDockerfile_CopiesBinaryFromBuilder verifies COPY --from= is used to copy
// the binary from the builder stage into the runtime stage.
func TestDockerfile_CopiesBinaryFromBuilder(t *testing.T) {
	lines := readDockerfile(t)
	if !containsLine(lines, "COPY --from=") {
		t.Error("Dockerfile should COPY binary --from the builder stage")
	}
}

// TestDockerfile_DisablesCGO verifies CGO_ENABLED=0 is set to produce a static binary.
func TestDockerfile_DisablesCGO(t *testing.T) {
	lines := readDockerfile(t)
	if !containsLine(lines, "CGO_ENABLED=0") {
		t.Error("Dockerfile should set CGO_ENABLED=0 for a static binary")
	}
}

// TestDockerfile_RunsAsNonRoot verifies that the runtime stage uses a non-root USER.
func TestDockerfile_RunsAsNonRoot(t *testing.T) {
	lines := readDockerfile(t)
	foundUser := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(trimmed), "USER ") {
			// Ensure it's not USER root or USER 0.
			rest := strings.TrimSpace(trimmed[5:])
			if rest != "root" && rest != "0" {
				foundUser = true
				break
			}
		}
	}
	if !foundUser {
		t.Error("Dockerfile runtime stage should declare a non-root USER")
	}
}
