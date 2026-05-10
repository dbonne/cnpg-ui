package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for bcrypt. Cost 12 is a reasonable balance
// between security and latency (~300 ms on a modern server).
const bcryptCost = 12

// HashPassword computes a bcrypt hash of the plaintext password.
// The returned hash string is safe to store in a K8s Secret.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies that the plaintext password matches the bcrypt hash.
// It returns an error if they do not match or if the hash is malformed.
func CheckPassword(hash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}
