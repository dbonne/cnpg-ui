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

func TestSessionValidatorAdapter_ValidSession(t *testing.T) {
	scheme := k8s.NewScheme()
	hash, _ := auth.HashPassword("pass")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cnpg-ui-credentials"},
		Data:       map[string][]byte{"username": []byte("admin"), "password_hash": []byte(hash)},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	cfg := &config.Config{K8sNamespace: "default", SecretName: "cnpg-ui-credentials", SessionTTL: 30 * time.Minute}
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fc, store)

	sess, _ := svc.Login(context.Background(), "admin", "pass")
	adapter := auth.NewSessionValidatorAdapter(svc)

	username, err := adapter.ValidateSession(sess.ID)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if username != "admin" {
		t.Errorf("username = %q, want %q", username, "admin")
	}
}

func TestSessionValidatorAdapter_InvalidSession(t *testing.T) {
	scheme := k8s.NewScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	cfg := &config.Config{K8sNamespace: "default", SecretName: "cnpg-ui-credentials", SessionTTL: 30 * time.Minute}
	store := auth.NewStore(cfg.SessionTTL)
	svc := auth.NewService(cfg, fc, store)
	adapter := auth.NewSessionValidatorAdapter(svc)

	_, err := adapter.ValidateSession("bogus-id")
	if err == nil {
		t.Error("ValidateSession() with bogus ID must return error")
	}
}
