package auth_test

import (
	"testing"
	"time"

	"github.com/dbonne/cnpg-ui/internal/auth"
)

func TestNewStore_CreateSession(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	sess, err := store.Create("alice")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.Username != "alice" {
		t.Errorf("Username = %q, want %q", sess.Username, "alice")
	}
	if sess.ID == "" {
		t.Error("ID must not be empty")
	}
	if sess.ExpiresAt.IsZero() {
		t.Error("ExpiresAt must not be zero")
	}
	if !sess.ExpiresAt.After(time.Now()) {
		t.Error("ExpiresAt must be in the future")
	}
}

func TestStore_Get_ValidSession(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	created, _ := store.Create("bob")

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Username != "bob" {
		t.Errorf("Username = %q, want %q", got.Username, "bob")
	}
}

func TestStore_Get_UnknownSession(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	_, err := store.Get("nonexistent-id")
	if err == nil {
		t.Error("Get() with unknown ID must return error")
	}
}

func TestStore_Get_ExpiredSession(t *testing.T) {
	// Very short TTL so the session expires immediately.
	store := auth.NewStore(1 * time.Millisecond)
	created, _ := store.Create("carol")
	time.Sleep(5 * time.Millisecond)

	_, err := store.Get(created.ID)
	if err == nil {
		t.Error("Get() on expired session must return error")
	}
}

func TestStore_Delete_InvalidatesSession(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	created, _ := store.Create("dave")

	store.Delete(created.ID)

	_, err := store.Get(created.ID)
	if err == nil {
		t.Error("Get() after Delete() must return error")
	}
}

func TestStore_DeleteAll_ByUsername(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	// Create two sessions for the same user.
	s1, _ := store.Create("eve")
	s2, _ := store.Create("eve")
	// Also create a session for a different user.
	s3, _ := store.Create("frank")

	store.DeleteAll("eve")

	// Both eve sessions must be gone.
	if _, err := store.Get(s1.ID); err == nil {
		t.Error("session s1 should be gone after DeleteAll")
	}
	if _, err := store.Get(s2.ID); err == nil {
		t.Error("session s2 should be gone after DeleteAll")
	}
	// Frank's session must remain.
	if _, err := store.Get(s3.ID); err != nil {
		t.Errorf("frank's session must remain, got error: %v", err)
	}
}

func TestNewStore_IDIsUnique(t *testing.T) {
	store := auth.NewStore(30 * time.Minute)
	s1, _ := store.Create("alice")
	s2, _ := store.Create("alice")
	if s1.ID == s2.ID {
		t.Error("two sessions for the same user must have different IDs")
	}
}
