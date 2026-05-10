// Package auth provides session management, authentication, and authorization
// for the CNPG Web UI. All session state is held in memory using a sync.Map;
// this is intentionally simple for the single-replica v1 design.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrSessionNotFound is returned by Store.Get when the session does not exist
// or has expired.
var ErrSessionNotFound = errors.New("session not found or expired")

// Session represents an authenticated user session.
type Session struct {
	// ID is the opaque, cryptographically-random session identifier.
	// It is used as both the cookie value and the Bearer token value.
	ID string

	// Username is the authenticated user's name (from the K8s Secret).
	Username string

	// CreatedAt records when the session was created.
	CreatedAt time.Time

	// ExpiresAt records when the session expires; Get will reject it after this time.
	ExpiresAt time.Time
}

// Store is an in-memory session store backed by a sync.Map.
// It is safe for concurrent use.
type Store struct {
	mu  sync.Map
	ttl time.Duration
}

// NewStore creates a new Store with the given session TTL.
func NewStore(ttl time.Duration) *Store {
	return &Store{ttl: ttl}
}

// Create generates a new Session for the given username, stores it, and returns it.
// The session ID is a 32-byte cryptographically-random hex string.
func (s *Store) Create(username string) (*Session, error) {
	id, err := generateID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		ID:        id,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	s.mu.Store(id, sess)
	return sess, nil
}

// Get retrieves a session by ID. It returns ErrSessionNotFound if the session
// does not exist or has expired. Expired sessions are lazily deleted on access.
func (s *Store) Get(id string) (*Session, error) {
	v, ok := s.mu.Load(id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	sess := v.(*Session)
	if time.Now().After(sess.ExpiresAt) {
		s.mu.Delete(id)
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// Delete removes a single session by ID. It is idempotent.
func (s *Store) Delete(id string) {
	s.mu.Delete(id)
}

// DeleteAll removes all sessions for the given username. It is used to
// invalidate all active sessions after a password change.
func (s *Store) DeleteAll(username string) {
	s.mu.Range(func(key, value any) bool {
		sess := value.(*Session)
		if sess.Username == username {
			s.mu.Delete(key)
		}
		return true
	})
}

// generateID creates a 32-byte random session ID encoded as a hex string.
func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
