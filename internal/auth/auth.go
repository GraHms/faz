// Package auth hashes passwords and issues session tokens.
//
// Passwords use PBKDF2 with SHA-256 from the standard library, at 600 000
// iterations by default (OWASP's 2023 figure for this hash). Session tokens
// are 32 random bytes; only their SHA-256 is stored.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultIterations for new password hashes.
const DefaultIterations = 600_000

// HashPassword returns a self describing hash: pbkdf2-sha256$iter$salt$key.
func HashPassword(password string, iterations int) (string, error) {
	if iterations <= 0 {
		iterations = DefaultIterations
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches hash, in constant time.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidatePassword enforces the minimum rule: at least 8 characters.
func ValidatePassword(p string) error {
	if len([]rune(p)) < 8 {
		return errors.New("password must have at least 8 characters")
	}
	return nil
}

// NewToken returns a session token for the client and the hash to store.
func NewToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, HashToken(t), nil
}

// HashToken is what the database stores for a token.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
