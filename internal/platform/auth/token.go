// Package auth provides token-based authentication for wmux daemon connections.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"os"
)

// TokenSize is the number of random bytes in an auth token.
const TokenSize = 32

// ErrInvalidToken is returned when a token file has unexpected content.
var ErrInvalidToken = errors.New("auth: invalid token")

// randReader is the source of random bytes used by Generate.
// It is a package-level variable so tests can substitute a failing reader.
var randReader io.Reader = rand.Reader

// Generate creates a new random auth token.
func Generate() ([]byte, error) {
	token := make([]byte, TokenSize)

	if _, err := io.ReadFull(randReader, token); err != nil {
		return nil, fmt.Errorf("auth: generate token: %w", err)
	}

	return token, nil
}

// SaveToFile writes a token to path with mode 0600.
func SaveToFile(token []byte, path string) error {
	if err := os.WriteFile(path, token, 0o600); err != nil {
		return fmt.Errorf("auth: save token to %q: %w", path, err)
	}

	return nil
}

// LoadFromFile reads a token from path and validates its size.
func LoadFromFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("auth: load token from %q: %w", path, err)
	}

	if len(data) != TokenSize {
		return nil, ErrInvalidToken
	}

	return data, nil
}

// Verify compares a stored token with a candidate using constant-time
// comparison to prevent timing attacks.
func Verify(stored, candidate []byte) bool {
	if len(candidate) != TokenSize {
		return false
	}

	return subtle.ConstantTimeCompare(stored, candidate) == 1
}

// Ensure loads an existing token from path or generates and saves a new one
// if the file does not exist.
func Ensure(path string) ([]byte, error) {
	token, err := LoadFromFile(path)
	if err == nil {
		return token, nil
	}

	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrInvalidToken) {
		return nil, err
	}

	token, err = Generate()
	if err != nil {
		return nil, err
	}

	if err := SaveToFile(token, path); err != nil {
		return nil, err
	}

	return token, nil
}
