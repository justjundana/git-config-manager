package crypto

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	svc := NewService()

	key, err := svc.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	plaintext := []byte("secret token value")

	encrypted, err := svc.Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	if bytes.Equal(encrypted, plaintext) {
		t.Error("encrypted data should differ from plaintext")
	}

	decrypted, err := svc.Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	svc := NewService()

	key1, _ := svc.GenerateKey()
	key2, _ := svc.GenerateKey()

	encrypted, err := svc.Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	_, err = svc.Decrypt(encrypted, key2)
	if err == nil {
		t.Error("Decrypt() with wrong key should fail")
	}
}

func TestDecryptTooShort(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	_, err := svc.Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("Decrypt() with short ciphertext should fail")
	}
}

func TestGenerateKey(t *testing.T) {
	svc := NewService()

	key1, err := svc.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	if len(key1) != 32 {
		t.Errorf("key length = %d, want 32", len(key1))
	}

	key2, _ := svc.GenerateKey()
	if bytes.Equal(key1, key2) {
		t.Error("GenerateKey() should produce unique keys")
	}
}

func TestGenerateSalt(t *testing.T) {
	svc := NewService()

	salt1, err := svc.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	if len(salt1) != 16 {
		t.Errorf("salt length = %d, want 16", len(salt1))
	}

	salt2, _ := svc.GenerateSalt()
	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt() should produce unique salts")
	}
}

func TestDeriveKey(t *testing.T) {
	svc := NewService()

	salt, _ := svc.GenerateSalt()
	key1 := svc.DeriveKey("password123", salt)
	key2 := svc.DeriveKey("password123", salt)

	if !bytes.Equal(key1, key2) {
		t.Error("DeriveKey() with same inputs should produce same key")
	}

	key3 := svc.DeriveKey("different", salt)
	if bytes.Equal(key1, key3) {
		t.Error("DeriveKey() with different passwords should produce different keys")
	}

	if len(key1) != 32 {
		t.Errorf("derived key length = %d, want 32", len(key1))
	}
}

func TestDeriveKeyArgon2id(t *testing.T) {
	svc := NewService()

	salt, _ := svc.GenerateSalt()
	key1 := svc.DeriveKeyArgon2id("password123", salt)
	key2 := svc.DeriveKeyArgon2id("password123", salt)

	if !bytes.Equal(key1, key2) {
		t.Error("DeriveKeyArgon2id() with same inputs should produce same key")
	}

	key3 := svc.DeriveKeyArgon2id("different", salt)
	if bytes.Equal(key1, key3) {
		t.Error("DeriveKeyArgon2id() with different passwords should produce different keys")
	}

	if len(key1) != 32 {
		t.Errorf("derived key length = %d, want 32", len(key1))
	}

	// Argon2id and PBKDF2 must produce different keys for the same input
	keyPBKDF2 := svc.DeriveKey("password123", salt)
	if bytes.Equal(key1, keyPBKDF2) {
		t.Error("Argon2id and PBKDF2 should produce different keys")
	}
}

func TestHash(t *testing.T) {
	svc := NewService()

	hash1 := svc.Hash([]byte("hello"))
	hash2 := svc.Hash([]byte("hello"))
	hash3 := svc.Hash([]byte("world"))

	if hash1 != hash2 {
		t.Error("Hash() of same input should be identical")
	}
	if hash1 == hash3 {
		t.Error("Hash() of different inputs should differ")
	}
	if len(hash1) != 64 {
		t.Errorf("Hash() length = %d, want 64 (SHA-256 hex)", len(hash1))
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	// Test various sizes
	sizes := []int{0, 1, 15, 16, 17, 100, 1024, 65536}
	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i % 256)
		}

		enc, err := svc.Encrypt(data, key)
		if err != nil {
			t.Fatalf("Encrypt(%d bytes) error: %v", size, err)
		}

		dec, err := svc.Decrypt(enc, key)
		if err != nil {
			t.Fatalf("Decrypt(%d bytes) error: %v", size, err)
		}

		if !bytes.Equal(dec, data) {
			t.Errorf("round-trip failed for %d bytes", size)
		}
	}
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	svc := NewService()
	_, err := svc.Encrypt([]byte("data"), []byte("short"))
	if err == nil {
		t.Error("Encrypt with invalid key length should error")
	}
}

func TestEncrypt_RandReaderError(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	// Override randReader with a failing reader
	old := randReader
	randReader = &errReader{}
	defer func() { randReader = old }()

	_, err := svc.Encrypt([]byte("data"), key)
	if err == nil {
		t.Error("Encrypt with failing rand reader should error")
	}
}

type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, errors.New("rand failure")
}

func TestEncrypt_NewGCMError(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	old := newGCMFn
	newGCMFn = func(cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("gcm failure")
	}
	defer func() { newGCMFn = old }()

	_, err := svc.Encrypt([]byte("data"), key)
	if err == nil {
		t.Error("Encrypt with failing GCM should error")
	}
}

func TestDecrypt_NewGCMError(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	old := newGCMFn
	newGCMFn = func(cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("gcm failure")
	}
	defer func() { newGCMFn = old }()

	_, err := svc.Decrypt(make([]byte, 50), key)
	if err == nil {
		t.Error("Decrypt with failing GCM should error")
	}
}

func TestDecrypt_InvalidKeyLength(t *testing.T) {
	svc := NewService()
	_, err := svc.Decrypt([]byte("0123456789abcdef0123456789abcdef"), []byte("short"))
	if err == nil {
		t.Error("Decrypt with invalid key length should error")
	}
}

func TestDeriveKey_DifferentSalts(t *testing.T) {
	svc := NewService()
	salt1, _ := svc.GenerateSalt()
	salt2, _ := svc.GenerateSalt()

	key1 := svc.DeriveKey("password", salt1)
	key2 := svc.DeriveKey("password", salt2)

	if bytes.Equal(key1, key2) {
		t.Error("DeriveKey with different salts should produce different keys")
	}
}

func TestNewService(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService should return non-nil")
	}
}

func TestHash_Empty(t *testing.T) {
	svc := NewService()
	h := svc.Hash([]byte{})
	if len(h) != 64 {
		t.Errorf("Hash of empty = %d chars, want 64", len(h))
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	enc, err := svc.Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	dec, err := svc.Decrypt(enc, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(dec) != 0 {
		t.Errorf("expected empty, got %d bytes", len(dec))
	}
}

func TestEncrypt_DifferentCiphertextEachTime(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()
	plaintext := []byte("same plaintext data")

	enc1, _ := svc.Encrypt(plaintext, key)
	enc2, _ := svc.Encrypt(plaintext, key)

	if bytes.Equal(enc1, enc2) {
		t.Error("Encrypt should produce different ciphertext each time (random nonce)")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()

	enc, _ := svc.Encrypt([]byte("secret"), key)

	// Tamper with the ciphertext (after the nonce)
	if len(enc) > 15 {
		enc[14] ^= 0xFF
	}

	_, err := svc.Decrypt(enc, key)
	if err == nil {
		t.Error("Decrypt of tampered ciphertext should error")
	}
}

func TestDeriveKey_EmptyPassword(t *testing.T) {
	svc := NewService()
	salt, _ := svc.GenerateSalt()

	key := svc.DeriveKey("", salt)
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}
}

func TestDeriveKey_LongPassword(t *testing.T) {
	svc := NewService()
	salt, _ := svc.GenerateSalt()

	longPass := string(make([]byte, 1000))
	key := svc.DeriveKey(longPass, salt)
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}
}

func TestHash_Deterministic(t *testing.T) {
	svc := NewService()
	data := []byte("consistent data")

	h1 := svc.Hash(data)
	h2 := svc.Hash(data)
	if h1 != h2 {
		t.Error("Hash should be deterministic")
	}
}

func TestGenerateSalt_UniqueMultiple(t *testing.T) {
	svc := NewService()
	salts := make(map[string]bool)

	for i := 0; i < 10; i++ {
		salt, err := svc.GenerateSalt()
		if err != nil {
			t.Fatalf("GenerateSalt: %v", err)
		}
		key := string(salt)
		if salts[key] {
			t.Error("GenerateSalt produced duplicate")
		}
		salts[key] = true
	}
}

func TestGenerateKey_UniqueMultiple(t *testing.T) {
	svc := NewService()
	keys := make(map[string]bool)

	for i := 0; i < 10; i++ {
		key, err := svc.GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		k := string(key)
		if keys[k] {
			t.Error("GenerateKey produced duplicate")
		}
		keys[k] = true
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()
	ct, err := svc.Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	pt, err := svc.Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(pt) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(pt))
	}
}

func TestDecrypt_ShortCiphertext(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()
	_, err := svc.Decrypt([]byte{1, 2, 3}, key)
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	svc := NewService()
	key1, _ := svc.GenerateKey()
	key2, _ := svc.GenerateKey()
	ct, _ := svc.Encrypt([]byte("secret"), key1)
	_, err := svc.Decrypt(ct, key2)
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestEncrypt_InvalidKeyLengthV2(t *testing.T) {
	svc := NewService()
	_, err := svc.Encrypt([]byte("data"), []byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestDecrypt_InvalidKeyLengthV2(t *testing.T) {
	svc := NewService()
	_, err := svc.Decrypt(make([]byte, 100), []byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	svc := NewService()
	salt := []byte("fixed-salt-value")
	k1 := svc.DeriveKey("password", salt)
	k2 := svc.DeriveKey("password", salt)
	if string(k1) != string(k2) {
		t.Error("DeriveKey not deterministic")
	}
}

func TestDeriveKey_DifferentPasswords(t *testing.T) {
	svc := NewService()
	salt := []byte("fixed-salt-value")
	k1 := svc.DeriveKey("pass1", salt)
	k2 := svc.DeriveKey("pass2", salt)
	if string(k1) == string(k2) {
		t.Error("different passwords produced same key")
	}
}

func TestGenerateSalt_Length(t *testing.T) {
	svc := NewService()
	salt, err := svc.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt: %v", err)
	}
	if len(salt) != 16 {
		t.Errorf("salt len = %d, want 16", len(salt))
	}
}

func TestGenerateKey_Length(t *testing.T) {
	svc := NewService()
	key, err := svc.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key len = %d, want 32", len(key))
	}
}

func TestHash_DeterministicV2(t *testing.T) {
	svc := NewService()
	h1 := svc.Hash([]byte("hello"))
	h2 := svc.Hash([]byte("hello"))
	if h1 != h2 {
		t.Error("Hash not deterministic")
	}
}

func TestHash_DifferentInputs(t *testing.T) {
	svc := NewService()
	h1 := svc.Hash([]byte("hello"))
	h2 := svc.Hash([]byte("world"))
	if h1 == h2 {
		t.Error("different inputs produced same hash")
	}
}

func TestEncrypt_LargeData(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}
	ct, err := svc.Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}
	pt, err := svc.Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}
	if len(pt) != len(data) {
		t.Errorf("size mismatch: %d vs %d", len(pt), len(data))
	}
}

// errRand is a reader that always fails.
type errRand struct{}

func (errRand) Read([]byte) (int, error) {
	return 0, errors.New("rand failure")
}

func withFailingRand(fn func()) {
	old := randReader
	randReader = errRand{}
	defer func() { randReader = old }()
	fn()
}

func TestEncrypt_RandFailure(t *testing.T) {
	svc := NewService()
	key, _ := svc.GenerateKey()
	withFailingRand(func() {
		_, err := svc.Encrypt([]byte("data"), key)
		if err == nil {
			t.Fatal("expected error when rand fails")
		}
	})
}

func TestGenerateSalt_RandFailure(t *testing.T) {
	svc := NewService()
	withFailingRand(func() {
		_, err := svc.GenerateSalt()
		if err == nil {
			t.Fatal("expected error when rand fails")
		}
	})
}

func TestGenerateKey_RandFailure(t *testing.T) {
	svc := NewService()
	withFailingRand(func() {
		_, err := svc.GenerateKey()
		if err == nil {
			t.Fatal("expected error when rand fails")
		}
	})
}

func TestEncrypt_RandFailsOnNonce(t *testing.T) {
	// Ensure randReader works for key gen but fails for nonce
	svc := NewService()
	old := randReader
	defer func() { randReader = old }()

	// Generate key with real rand
	key, _ := svc.GenerateKey()

	// Now fail rand for nonce generation
	randReader = errRand{}
	_, err := svc.Encrypt([]byte("hello"), key)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRandReader_Default(t *testing.T) {
	// Verify the default randReader is crypto/rand.Reader
	if randReader != rand.Reader {
		t.Error("randReader should default to crypto/rand.Reader")
	}
}
