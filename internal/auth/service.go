package auth

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dbonne/cnpg-ui/internal/config"
)

// ErrInvalidCredentials is returned by Login and ChangePassword when the
// supplied credentials do not match the stored ones.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service is the interface for authentication operations.
// All methods are safe to call concurrently.
type Service interface {
	// Login validates username + password against the K8s Secret, creates a
	// session, and returns it.
	Login(ctx context.Context, username, password string) (*Session, error)

	// Logout invalidates the session identified by sessionID.
	Logout(sessionID string)

	// ValidateSession returns the session for the given ID, or an error if the
	// session does not exist or has expired.
	ValidateSession(sessionID string) (*Session, error)

	// ChangePassword verifies oldPassword against the K8s Secret, replaces it
	// with newPassword (bcrypt-hashed), updates the Secret in-cluster, and
	// invalidates all active sessions for the user.
	ChangePassword(ctx context.Context, username, oldPassword, newPassword string) error
}

// service is the production implementation of Service.
type service struct {
	cfg    *config.Config
	client client.Client
	store  *Store
}

// NewService constructs a production auth.Service.
//   - cfg provides the namespace and secret name.
//   - k8sClient is used to read/update the credentials Secret.
//   - store is the session store (shared so tests can inspect it).
func NewService(cfg *config.Config, k8sClient client.Client, store *Store) Service {
	return &service{
		cfg:    cfg,
		client: k8sClient,
		store:  store,
	}
}

// Login validates credentials against the K8s Secret and creates a session.
func (s *service) Login(ctx context.Context, username, password string) (*Session, error) {
	storedUser, storedHash, err := s.loadCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	if username != storedUser {
		return nil, ErrInvalidCredentials
	}
	if err := CheckPassword(storedHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	sess, err := s.store.Create(username)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// Logout removes the session identified by sessionID from the store.
func (s *service) Logout(sessionID string) {
	s.store.Delete(sessionID)
}

// ValidateSession returns the session if it exists and has not expired.
func (s *service) ValidateSession(sessionID string) (*Session, error) {
	return s.store.Get(sessionID)
}

// ChangePassword verifies the old password, updates the K8s Secret with a
// new bcrypt hash, and invalidates all sessions for the user.
func (s *service) ChangePassword(ctx context.Context, username, oldPassword, newPassword string) error {
	storedUser, storedHash, err := s.loadCredentials(ctx)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if username != storedUser {
		return ErrInvalidCredentials
	}
	if err := CheckPassword(storedHash, oldPassword); err != nil {
		return ErrInvalidCredentials
	}

	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if err := s.updateSecret(ctx, username, newHash); err != nil {
		return fmt.Errorf("update secret: %w", err)
	}

	// Invalidate all active sessions for this user.
	s.store.DeleteAll(username)
	return nil
}

// loadCredentials fetches the K8s Secret and returns (username, passwordHash).
func (s *service) loadCredentials(ctx context.Context) (string, string, error) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: s.cfg.K8sNamespace,
		Name:      s.cfg.SecretName,
	}
	if err := s.client.Get(ctx, key, &secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", "", fmt.Errorf("credentials secret %q not found in namespace %q",
				s.cfg.SecretName, s.cfg.K8sNamespace)
		}
		return "", "", fmt.Errorf("get secret %q: %w", s.cfg.SecretName, err)
	}

	username := string(secret.Data["username"])
	passwordHash := string(secret.Data["password_hash"])
	if username == "" || passwordHash == "" {
		return "", "", fmt.Errorf("secret %q missing username or password_hash keys", s.cfg.SecretName)
	}
	return username, passwordHash, nil
}

// updateSecret patches the K8s Secret with the new username + password hash.
func (s *service) updateSecret(ctx context.Context, username, passwordHash string) error {
	var secret corev1.Secret
	key := types.NamespacedName{
		Namespace: s.cfg.K8sNamespace,
		Name:      s.cfg.SecretName,
	}
	if err := s.client.Get(ctx, key, &secret); err != nil {
		return fmt.Errorf("get secret for update: %w", err)
	}

	patch := client.MergeFrom(secret.DeepCopy())
	secret.Data["username"] = []byte(username)
	secret.Data["password_hash"] = []byte(passwordHash)

	if err := s.client.Patch(ctx, &secret, patch); err != nil {
		return fmt.Errorf("patch secret: %w", err)
	}
	return nil
}
