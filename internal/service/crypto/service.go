// Package crypto provides encryption and hashing utilities for GCM.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

const (
	keyLen     = 32 // AES-256
	saltLen    = 16
	iterations = 100000 // PBKDF2 iterations (legacy)

	// Argon2id parameters (OWASP recommended minimums).
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
)

// Service provides cryptographic operations.
type Service struct{}

// randReader is the source of randomness. Tests may override it.
var randReader io.Reader = rand.Reader

// newGCMFn creates a GCM cipher mode. Tests may override it.
var newGCMFn = cipher.NewGCM

// NewService creates a new crypto service.
func NewService() *Service {
	return &Service{}
}

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
func (s *Service) Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := newGCMFn(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Prepend nonce to ciphertext
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the given key.
func (s *Service) Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := newGCMFn(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// DeriveKey derives an AES-256 key from a password using PBKDF2 (legacy).
// Retained for backward-compatible decryption of existing encrypted tokens.
func (s *Service) DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, iterations, keyLen, sha256.New)
}

// DeriveKeyArgon2id derives an AES-256 key from a password using Argon2id.
// This is the preferred KDF for new encryptions (memory-hard, resistant to
// GPU/ASIC attacks).
func (s *Service) DeriveKeyArgon2id(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, keyLen)
}

// GenerateSalt creates a random salt.
func (s *Service) GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(randReader, salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	return salt, nil
}

// GenerateKey creates a random AES-256 key.
func (s *Service) GenerateKey() ([]byte, error) {
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(randReader, key); err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}
	return key, nil
}

// Hash returns the SHA-256 hex digest of data.
func (s *Service) Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
