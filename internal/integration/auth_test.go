package integration_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dbonne/cnpg-ui/internal/auth"
	"github.com/dbonne/cnpg-ui/internal/config"
)

// testConfig returns a minimal config.Config pointing to the envtest credentials secret.
func testConfig(ns, secretName string) *config.Config {
	return &config.Config{
		K8sNamespace: ns,
		SecretName:   secretName,
		SessionTTL:   5 * time.Minute,
	}
}

// TestAuthFlow_LoginAndValidateSession_WithEnvtest exercises the full auth flow:
//  1. Create a K8s Secret with bcrypt-hashed password
//  2. Login via auth.Service → get session token
//  3. ValidateSession with the token → expect success
//  4. Logout → ValidateSession again → expect failure
func TestAuthFlow_LoginAndValidateSession_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-auth"
	const secretName = "cnpg-ui-credentials"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the credentials secret with a known bcrypt hash.
	// bcrypt hash of "test-password" at cost 4 (low cost for test speed).
	const passwordHash = "$2a$04$YHzL.jfYkqwKBaJbIXHzLuGQ0YR2hZ/5.E0q2p0n7qrS1mU3HqMz6"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns,
		},
		StringData: map[string]string{
			"username":      "admin",
			"password_hash": passwordHash,
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("create credentials secret: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), secret)
	})

	cfg := testConfig(ns, secretName)
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, k8sClient, store)

	// Login — note: the hash above may not match "test-password" exactly since
	// we're using a fixed hash. Use bcrypt.GenerateFromPassword in real tests.
	// For this integration test, we create a fresh hash via auth.HashPassword.
	hash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Update the secret with the correctly hashed password.
	secret.StringData = map[string]string{
		"username":      "admin",
		"password_hash": hash,
	}
	if err := k8sClient.Update(ctx, secret); err != nil {
		t.Fatalf("update credentials secret: %v", err)
	}

	// Login should succeed.
	session, err := svc.Login(ctx, "admin", "test-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.ID == "" {
		t.Error("Login: expected non-empty session ID")
	}
	if session.Username != "admin" {
		t.Errorf("Login: Username = %q, want %q", session.Username, "admin")
	}

	// ValidateSession should succeed.
	validated, err := svc.ValidateSession(session.ID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if validated.Username != "admin" {
		t.Errorf("ValidateSession: Username = %q, want %q", validated.Username, "admin")
	}

	// Logout.
	svc.Logout(session.ID)

	// ValidateSession after logout should fail.
	if _, err := svc.ValidateSession(session.ID); err == nil {
		t.Error("ValidateSession after Logout: expected error, got nil")
	}
}

// TestAuthFlow_InvalidCredentials_WithEnvtest verifies that Login fails with
// wrong credentials and does NOT create a session.
func TestAuthFlow_InvalidCredentials_WithEnvtest(t *testing.T) {
	_, k8sClient := startEnvtest(t)

	const ns = "envtest-auth-invalid"
	const secretName = "cnpg-ui-credentials"
	createNamespace(t, k8sClient, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hash, _ := auth.HashPassword("correct-password")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		StringData: map[string]string{
			"username":      "admin",
			"password_hash": hash,
		},
	}
	if err := k8sClient.Create(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), secret) })

	cfg := testConfig(ns, secretName)
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, k8sClient, store)

	// Wrong password — should fail.
	if _, err := svc.Login(ctx, "admin", "wrong-password"); err == nil {
		t.Error("Login with wrong password: expected error, got nil")
	}

	// Wrong username — should fail.
	if _, err := svc.Login(ctx, "nobody", "correct-password"); err == nil {
		t.Error("Login with wrong username: expected error, got nil")
	}
}
