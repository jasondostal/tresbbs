// Package auth handles password hashing and verification for tresbbs.
//
// The original TriBBS stored passwords in plain text (PASSWORD.DAT for the
// system password, and in the user record). For a modern recreation, we use
// bcrypt for password hashing — faithful to the original's intent (password
// verification) while being cryptographically secure.
//
// bcrypt is the right choice because:
//   - It's the standard for password hashing
//   - It's built into Go's golang.org/x/crypto package
//   - It handles salting automatically
//   - It's intentionally slow (brute-force resistant)
package auth

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password using bcrypt with the default cost (10).
// Returns the hashed password as a string suitable for storage.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword verifies a password against a bcrypt hash.
// Returns true if the password matches, false otherwise.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// IsHashed checks if a string looks like a bcrypt hash.
// This is useful for migrating from plain text passwords — if the stored
// password isn't hashed, we can hash it on next successful login.
func IsHashed(password string) bool {
	// bcrypt hashes always start with "$2a$" or "$2b$"
	return len(password) >= 4 && password[0] == '$' && password[1] == '2' && password[3] == '$'
}
