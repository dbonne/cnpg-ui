package auth_test

import (
	"testing"

	"github.com/dbonne/cnpg-ui/internal/auth"
)

func TestHashPassword_ReturnsNonEmptyHash(t *testing.T) {
	hash, err := auth.HashPassword("secretpassword")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == "" {
		t.Error("hash must not be empty")
	}
	if hash == "secretpassword" {
		t.Error("hash must not equal the original password")
	}
}

func TestHashPassword_DifferentCallsProduceDifferentHashes(t *testing.T) {
	// bcrypt salts: same password → different hashes each time.
	h1, _ := auth.HashPassword("mypassword")
	h2, _ := auth.HashPassword("mypassword")
	if h1 == h2 {
		t.Error("two hashes for the same password must differ (bcrypt salting)")
	}
}

func TestCheckPassword_CorrectPassword(t *testing.T) {
	hash, _ := auth.HashPassword("correct-horse-battery-staple")
	if err := auth.CheckPassword(hash, "correct-horse-battery-staple"); err != nil {
		t.Errorf("CheckPassword() with correct password must not error, got: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, _ := auth.HashPassword("correct-horse-battery-staple")
	if err := auth.CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("CheckPassword() with wrong password must return error")
	}
}

func TestCheckPassword_EmptyHash(t *testing.T) {
	if err := auth.CheckPassword("", "any-password"); err == nil {
		t.Error("CheckPassword() with empty hash must return error")
	}
}
