package auth_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dbonne/cnpg-ui/internal/auth"
	"github.com/dbonne/cnpg-ui/internal/config"
	"github.com/dbonne/cnpg-ui/internal/k8s"
)

// buildCfg returns a minimal Config for auth service tests.
func buildCfg(ns, secretName string) *config.Config {
	return &config.Config{
		K8sNamespace: ns,
		SecretName:   secretName,
		SessionTTL:   30 * time.Minute,
	}
}

// buildSecret creates a K8s Secret with bcrypt-hashed password.
func buildSecret(ns, name, username, password string) *corev1.Secret {
	hash, _ := auth.HashPassword(password)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Data: map[string][]byte{
			"username":      []byte(username),
			"password_hash": []byte(hash),
		},
	}
}

func TestService_Login_ValidCredentials(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "hunter2")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	sess, err := svc.Login(context.Background(), "admin", "hunter2")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if sess.Username != "admin" {
		t.Errorf("Username = %q, want %q", sess.Username, "admin")
	}
	if sess.ID == "" {
		t.Error("session ID must not be empty")
	}
}

func TestService_Login_WrongPassword(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "hunter2")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	_, err := svc.Login(context.Background(), "admin", "wrongpass")
	if err == nil {
		t.Error("Login() with wrong password must return error")
	}
}

func TestService_Login_WrongUsername(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "hunter2")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	_, err := svc.Login(context.Background(), "notadmin", "hunter2")
	if err == nil {
		t.Error("Login() with wrong username must return error")
	}
}

func TestService_Login_MissingSecret(t *testing.T) {
	scheme := k8s.NewScheme()
	// No secret in the fake client.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	_, err := svc.Login(context.Background(), "admin", "hunter2")
	if err == nil {
		t.Error("Login() with missing Secret must return error")
	}
}

func TestService_ValidateSession_Valid(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "pass")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	sess, _ := svc.Login(context.Background(), "admin", "pass")
	got, err := svc.ValidateSession(sess.ID)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if got.Username != "admin" {
		t.Errorf("Username = %q, want %q", got.Username, "admin")
	}
}

func TestService_ValidateSession_InvalidID(t *testing.T) {
	scheme := k8s.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	_, err := svc.ValidateSession("made-up-id")
	if err == nil {
		t.Error("ValidateSession() with bogus ID must return error")
	}
}

func TestService_Logout_InvalidatesSession(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "pass")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	sess, _ := svc.Login(context.Background(), "admin", "pass")
	svc.Logout(sess.ID)

	_, err := svc.ValidateSession(sess.ID)
	if err == nil {
		t.Error("ValidateSession() after Logout() must return error")
	}
}

func TestService_ChangePassword_ValidOld(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "oldpass")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	// Login to get a session before changing password.
	sess, _ := svc.Login(context.Background(), "admin", "oldpass")

	if err := svc.ChangePassword(context.Background(), "admin", "oldpass", "newpass"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	// Old session must be invalidated after password change.
	if _, err := svc.ValidateSession(sess.ID); err == nil {
		t.Error("old session must be invalidated after password change")
	}

	// Must be able to login with new password.
	if _, err := svc.Login(context.Background(), "admin", "newpass"); err != nil {
		t.Errorf("Login() with new password must work, got: %v", err)
	}
}

func TestService_ChangePassword_WrongOld(t *testing.T) {
	scheme := k8s.NewScheme()
	secret := buildSecret("default", "cnpg-ui-credentials", "admin", "oldpass")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	cfg := buildCfg("default", "cnpg-ui-credentials")
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fakeClient, store)

	if err := svc.ChangePassword(context.Background(), "admin", "wrongold", "newpass"); err == nil {
		t.Error("ChangePassword() with wrong old password must return error")
	}
}
