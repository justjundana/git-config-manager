// Package tokenstore implements provider-neutral secure token persistence with
// three storage backends chosen at runtime from the SecurityConfig:
//
//  1. OS Keychain  (Security.UseKeychain = true)
//     Uses the platform credential store via go-keyring: macOS Keychain,
//     Linux secret-service / KWallet, Windows Credential Manager.
//
//  2. Encrypted file  (Security.EncryptTokens = true, MasterPassword = true)
//     Derives an AES-256 key from a master password via Argon2id (time=3,
//     memory=64 MiB, threads=4). A random 16-byte salt is stored alongside
//     the ciphertext in the v2 on-disk format. Legacy v1 tokens (PBKDF2,
//     100 000 iterations, SHA-256) are transparently decrypted and
//     re-encrypted with Argon2id on next save. The master password is
//     prompted once per session through the promptFunc callback and cached
//     in memory for the process lifetime.
//
//  3. Plain-text file  (Security.AllowPlaintextTokens = true)
//     Token written with 0600 permissions. Simple, auditable, but relies
//     solely on filesystem ACLs and is disabled unless explicitly opted in.
//
// Every backend that touches the filesystem goes through sanitizeTokenPath()
// which prevents path-traversal attacks.
package tokenstore

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/provider"
	cryptoSvc "github.com/justjundana/git-config-manager/internal/service/crypto"
	"github.com/justjundana/git-config-manager/pkg/logger"

	"github.com/zalando/go-keyring"
)

const (
	// keychainService is the service label stored in the OS keychain.
	keychainService = "git-config-manager"

	// saltLen must match crypto.Service.GenerateSalt output (16 bytes).
	tokenSaltLen = 16

	// tokenFormatV2 is the magic byte that identifies the Argon2id-based
	// encrypted token format. Existing (v1) files start with 0x00 (the
	// high byte of uint16(16)), so 0x02 is unambiguous.
	tokenFormatV2 byte = 0x02
)

// Keyring function hooks – overridden in tests to avoid OS keychain.
var (
	keyringSet    = keyring.Set
	keyringGet    = keyring.Get
	keyringDelete = keyring.Delete
)

// IsKeychainAvailable performs a non-destructive probe to determine if the
// OS keychain is functional on this platform. It attempts a Get on a key
// that will never exist; if the error is ErrNotFound the keychain works.
// Any other error (e.g. ErrUnsupportedPlatform, D-Bus unreachable) means
// the keychain is not usable and callers should prefer file-based storage.
func IsKeychainAvailable() bool {
	_, err := keyringGet(keychainService, "__gcm_probe__")
	if err == nil || err == keyring.ErrNotFound {
		return true
	}
	return false
}

// File-operation hooks – overridden in tests to simulate I/O errors.
var (
	osMkdirAll   = os.MkdirAll
	osCreateTemp = os.CreateTemp
	osRename     = os.Rename
	fileWrite    = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	fileChmod    = func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) }
	fileSync     = func(f *os.File) error { return f.Sync() }
	fileClose    = func(f *os.File) error { return f.Close() }
	filepathAbs  = filepath.Abs
	filepathRel  = filepath.Rel
)

// Crypto-operation hooks – overridden in tests to simulate crypto errors.
var (
	generateSalt = func(c *cryptoSvc.Service) ([]byte, error) { return c.GenerateSalt() }
	encryptData  = func(c *cryptoSvc.Service, plaintext, key []byte) ([]byte, error) { return c.Encrypt(plaintext, key) }
)

var marshalTokenSet = json.Marshal

// PromptFunc asks the user for input (e.g. a master password).
// It is supplied by the CLI layer so the token store itself has no
// dependency on terminal I/O.
type PromptFunc func(msg string) (string, error)

// TokenStore handles secure persistence of provider access tokens.
type TokenStore struct {
	cfg        *config.Config
	crypto     *cryptoSvc.Service
	log        *logger.Logger
	promptFunc PromptFunc

	// cachedPassword caches the master password as a byte slice so it can
	// be zeroed if needed. The user is only prompted once per process lifetime.
	masterKeyOnce  sync.Once
	cachedPassword []byte
	masterKeyErr   error
}

// NewTokenStore creates a token store configured according to the
// SecurityConfig. promptFunc is called at most once to obtain the master
// password when encrypted-file storage is active.
func NewTokenStore(cfg *config.Config, crypto *cryptoSvc.Service, log *logger.Logger, prompt PromptFunc) *TokenStore {
	return &TokenStore{
		cfg:        cfg,
		crypto:     crypto,
		log:        log,
		promptFunc: prompt,
	}
}

// SetPromptFunc sets the callback used to ask the user for a master password.
// This allows the prompt to be injected after construction (e.g. by the CLI).
func (ts *TokenStore) SetPromptFunc(fn PromptFunc) {
	ts.promptFunc = fn
}

// Save persists a token for the given profile using the configured backend.
func (ts *TokenStore) Save(profile, token string) error {
	return ts.saveTokenValue(profile, token)
}

// Load retrieves the token for the given profile.
func (ts *TokenStore) Load(profile string) (string, error) {
	return ts.loadTokenValue(profile)
}

// Delete removes the stored token for the given profile.
func (ts *TokenStore) Delete(profile string) error {
	return ts.deleteTokenValue(profile)
}

// SaveTokenSet persists a provider-aware token set.
func (ts *TokenStore) SaveTokenSet(key provider.TokenKey, token provider.TokenSet) error {
	if token.AccessToken == "" {
		return fmt.Errorf("access token cannot be empty")
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	storageKey, err := providerTokenStorageKey(key)
	if err != nil {
		return err
	}
	payload, err := marshalTokenSet(token)
	if err != nil {
		return fmt.Errorf("marshaling token set: %w", err)
	}
	return ts.saveTokenValue(storageKey, string(payload))
}

// LoadTokenSet retrieves a provider-aware token set. For GitHub, it falls back
// to the legacy profile-only token key to preserve existing installations.
func (ts *TokenStore) LoadTokenSet(key provider.TokenKey) (provider.TokenSet, error) {
	storageKey, err := providerTokenStorageKey(key)
	if err != nil {
		return provider.TokenSet{}, err
	}
	raw, err := ts.loadTokenValue(storageKey)
	if err != nil && isLegacyGitHubTokenKey(key) {
		raw, err = ts.loadTokenValue(key.Profile)
	}
	if err != nil {
		return provider.TokenSet{}, err
	}

	var token provider.TokenSet
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		if err := json.Unmarshal([]byte(raw), &token); err != nil {
			return provider.TokenSet{}, fmt.Errorf("parsing token set: %w", err)
		}
	} else {
		token = provider.TokenSet{AccessToken: raw, AuthMethod: provider.AuthMethodLegacy}
	}
	if token.AccessToken == "" {
		return provider.TokenSet{}, fmt.Errorf("empty token for profile %q provider %q", key.Profile, key.Provider)
	}
	return token, nil
}

// DeleteTokenSet removes a provider-aware token set. GitHub legacy tokens are
// also deleted when the key resolves to the default GitHub provider.
func (ts *TokenStore) DeleteTokenSet(key provider.TokenKey) error {
	storageKey, err := providerTokenStorageKey(key)
	if err != nil {
		return err
	}
	if err := ts.deleteTokenValue(storageKey); err != nil {
		return err
	}
	if isLegacyGitHubTokenKey(key) {
		return ts.deleteTokenValue(key.Profile)
	}
	return nil
}

func (ts *TokenStore) saveTokenValue(storageKey, token string) error {
	if ts.cfg.Security.UseKeychain {
		return ts.saveKeychain(storageKey, token)
	}
	if ts.cfg.Security.EncryptTokens && ts.cfg.Security.MasterPassword {
		return ts.saveEncrypted(storageKey, token)
	}
	if !ts.cfg.Security.AllowPlaintextTokens {
		return plaintextTokenStorageDisabledError()
	}
	return ts.savePlain(storageKey, token)
}

func (ts *TokenStore) loadTokenValue(storageKey string) (string, error) {
	if ts.cfg.Security.UseKeychain {
		return ts.loadKeychain(storageKey)
	}
	if ts.cfg.Security.EncryptTokens && ts.cfg.Security.MasterPassword {
		return ts.loadEncrypted(storageKey)
	}
	if !ts.cfg.Security.AllowPlaintextTokens {
		return "", plaintextTokenStorageDisabledError()
	}
	return ts.loadPlain(storageKey)
}

func (ts *TokenStore) deleteTokenValue(storageKey string) error {
	if ts.cfg.Security.UseKeychain {
		return ts.deleteKeychain(storageKey)
	}
	return ts.deleteFile(storageKey)
}

func plaintextTokenStorageDisabledError() error {
	return fmt.Errorf("plaintext token storage disabled; enable security.use_keychain, enable security.encrypt_tokens with security.master_password, or explicitly set security.allow_plaintext_tokens")
}

func providerTokenStorageKey(key provider.TokenKey) (string, error) {
	if key.Profile == "" {
		return "", fmt.Errorf("profile name cannot be empty")
	}
	if key.Provider == "" {
		return "", fmt.Errorf("provider cannot be empty")
	}
	parts := []string{
		key.Profile,
		string(key.Provider),
		key.Host,
		key.Account,
	}
	if parts[2] == "" {
		parts[2] = "default"
	}
	if parts[3] == "" {
		parts[3] = "default"
	}
	for i, part := range parts {
		parts[i] = safeTokenComponent(part)
		if parts[i] == "" {
			return "", fmt.Errorf("invalid token key component")
		}
	}
	return strings.Join(parts, "__"), nil
}

func safeTokenComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func isLegacyGitHubTokenKey(key provider.TokenKey) bool {
	host := strings.ToLower(strings.TrimSpace(key.Host))
	return key.Provider == provider.GitHubID && key.Account == "" && (host == "" || host == "github.com")
}

// ---------------------------------------------------------------------------
// Backend 1: OS Keychain
// ---------------------------------------------------------------------------

func (ts *TokenStore) saveKeychain(profile, token string) error {
	if err := keyringSet(keychainService, profile, token); err != nil {
		ts.log.Debug("Keychain unavailable",
			logger.F("error", err.Error()), logger.F("profile", profile))
		if ts.cfg.Security.EncryptTokens && ts.cfg.Security.MasterPassword {
			return ts.saveEncrypted(profile, token)
		}
		if !ts.cfg.Security.AllowPlaintextTokens {
			return fmt.Errorf("saving token to keychain: %w; %v", err, plaintextTokenStorageDisabledError())
		}
		return ts.savePlain(profile, token)
	}
	ts.log.Debug("Token saved to OS keychain", logger.F("profile", profile))
	return nil
}

func (ts *TokenStore) loadKeychain(profile string) (string, error) {
	token, err := keyringGet(keychainService, profile)
	if err != nil {
		ts.log.Debug("Keychain read failed",
			logger.F("error", err.Error()), logger.F("profile", profile))
		if ts.cfg.Security.EncryptTokens && ts.cfg.Security.MasterPassword {
			if t, e := ts.loadEncrypted(profile); e == nil {
				return t, nil
			}
		}
		if ts.cfg.Security.AllowPlaintextTokens {
			if t, e := ts.loadPlain(profile); e == nil {
				return t, nil
			}
		}
		return "", fmt.Errorf("loading token from keychain: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("empty token in keychain for profile %q", profile)
	}
	return token, nil
}

func (ts *TokenStore) deleteKeychain(profile string) error {
	err := keyringDelete(keychainService, profile)
	if err != nil && !isKeyringNotFound(err) {
		return fmt.Errorf("deleting token from keychain: %w", err)
	}
	// Also clean up any leftover file from a previous storage mode.
	_ = ts.deleteFile(profile)
	return nil
}

// isKeyringNotFound returns true when the keyring reports that the
// requested secret does not exist. go-keyring surfaces this as
// keyring.ErrNotFound on all platforms.
func isKeyringNotFound(err error) bool {
	return err == keyring.ErrNotFound
}

// ---------------------------------------------------------------------------
// Backend 2: Encrypted file (PBKDF2-derived AES-256-GCM)
// ---------------------------------------------------------------------------

// On-disk layout: [ 2-byte salt-length (big-endian) | salt | ciphertext ]
// The ciphertext is produced by crypto.Service.Encrypt which prepends its
// own 12-byte GCM nonce internally.

func (ts *TokenStore) saveEncrypted(profile, token string) error {
	password, err := ts.getMasterPassword()
	if err != nil {
		return err
	}

	salt, err := generateSalt(ts.crypto)
	if err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	key := ts.crypto.DeriveKeyArgon2id(string(password), salt)
	defer zeroBytes(key)

	ciphertext, err := encryptData(ts.crypto, []byte(token), key)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	// v2 on-disk layout: [ 0x02 | 2-byte salt-length (big-endian) | salt | ciphertext ]
	buf := make([]byte, 1+2+len(salt)+len(ciphertext))
	buf[0] = tokenFormatV2
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(salt)))
	copy(buf[3:3+len(salt)], salt)
	copy(buf[3+len(salt):], ciphertext)

	return ts.writeTokenFile(profile, buf)
}

func (ts *TokenStore) loadEncrypted(profile string) (string, error) {
	payload, err := ts.readTokenFile(profile)
	if err != nil {
		return "", err
	}

	if len(payload) < 2+tokenSaltLen {
		return "", fmt.Errorf("corrupt encrypted token file for profile %q", profile)
	}

	var salt, ciphertext []byte
	var useArgon2 bool

	if payload[0] == tokenFormatV2 {
		// v2 format: [ 0x02 | 2-byte salt-len | salt | ciphertext ]
		useArgon2 = true
		if len(payload) < 3+tokenSaltLen {
			return "", fmt.Errorf("corrupt encrypted token file for profile %q", profile)
		}
		sLen := int(binary.BigEndian.Uint16(payload[1:3]))
		if sLen < 1 || 3+sLen > len(payload) {
			return "", fmt.Errorf("corrupt encrypted token file for profile %q", profile)
		}
		salt = payload[3 : 3+sLen]
		ciphertext = payload[3+sLen:]
	} else {
		// v1 format (legacy PBKDF2): [ 2-byte salt-len | salt | ciphertext ]
		saltLen := int(binary.BigEndian.Uint16(payload[0:2]))
		if saltLen < 1 || 2+saltLen > len(payload) {
			return "", fmt.Errorf("corrupt encrypted token file for profile %q", profile)
		}
		salt = payload[2 : 2+saltLen]
		ciphertext = payload[2+saltLen:]
	}

	password, err := ts.getMasterPassword()
	if err != nil {
		return "", err
	}

	var key []byte
	if useArgon2 {
		key = ts.crypto.DeriveKeyArgon2id(string(password), salt)
	} else {
		key = ts.crypto.DeriveKey(string(password), salt)
	}
	defer zeroBytes(key)

	plaintext, err := ts.crypto.Decrypt(ciphertext, key)
	if err != nil {
		return "", fmt.Errorf("decrypting token (wrong master password?): %w", err)
	}

	token := strings.TrimSpace(string(plaintext))
	if token == "" {
		return "", fmt.Errorf("empty token after decryption for profile %q", profile)
	}
	return token, nil
}

func (ts *TokenStore) getMasterPassword() ([]byte, error) {
	ts.masterKeyOnce.Do(func() {
		if ts.promptFunc == nil {
			ts.masterKeyErr = fmt.Errorf("master password required but no prompt function configured")
			return
		}
		var pw string
		pw, ts.masterKeyErr = ts.promptFunc("Enter master password")
		if ts.masterKeyErr != nil {
			ts.masterKeyErr = fmt.Errorf("reading master password: %w", ts.masterKeyErr)
			return
		}
		if pw == "" {
			ts.masterKeyErr = fmt.Errorf("master password cannot be empty")
			return
		}
		// Store as []byte so it can be zeroed on process shutdown if desired.
		ts.cachedPassword = []byte(pw)
	})
	return ts.cachedPassword, ts.masterKeyErr
}

// ZeroPassword zeroes the cached master password from memory.
// Call this during graceful shutdown to minimize the window of exposure.
func (ts *TokenStore) ZeroPassword() {
	for i := range ts.cachedPassword {
		ts.cachedPassword[i] = 0
	}
}

// ---------------------------------------------------------------------------
// Backend 3: Plain-text file (0600 permissions)
// ---------------------------------------------------------------------------

func (ts *TokenStore) savePlain(profile, token string) error {
	if !ts.cfg.Security.AllowPlaintextTokens {
		return plaintextTokenStorageDisabledError()
	}
	ts.log.Debug("Saving token in plain-text mode (file permissions only)",
		logger.F("profile", profile))
	return ts.writeTokenFile(profile, []byte(token))
}

func (ts *TokenStore) loadPlain(profile string) (string, error) {
	if !ts.cfg.Security.AllowPlaintextTokens {
		return "", plaintextTokenStorageDisabledError()
	}
	payload, err := ts.readTokenFile(profile)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(payload))
	if token == "" {
		return "", fmt.Errorf("empty token file for profile %q", profile)
	}
	return token, nil
}

// ---------------------------------------------------------------------------
// Shared file helpers
// ---------------------------------------------------------------------------

// sanitizeTokenPath builds and validates the on-disk token path for a
// profile, refusing names that would escape the tokens directory.
func sanitizeTokenPath(profile string) (string, error) {
	if profile == "" {
		return "", fmt.Errorf("profile name cannot be empty")
	}
	if strings.ContainsAny(profile, `/\`) || strings.Contains(profile, "..") {
		return "", fmt.Errorf("invalid profile name %q", profile)
	}
	tokenDir := filepath.Join(config.GCMDir(), "tokens")
	baseAbs, err := filepathAbs(tokenDir)
	if err != nil {
		return "", fmt.Errorf("resolving tokens dir: %w", err)
	}
	full := filepath.Clean(filepath.Join(baseAbs, profile+".token"))
	rel, err := filepathRel(baseAbs, full)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid profile name %q", profile)
	}
	return full, nil
}

func (ts *TokenStore) writeTokenFile(profile string, data []byte) error {
	tokenPath, err := sanitizeTokenPath(profile)
	if err != nil {
		return err
	}
	tokenDir := filepath.Dir(tokenPath)
	if err := osMkdirAll(tokenDir, 0700); err != nil {
		return fmt.Errorf("creating tokens directory: %w", err)
	}

	// Atomic write: temp file + sync + rename to prevent token corruption
	// if the process is interrupted (e.g., user Ctrl-C during save).
	tmp, err := osCreateTemp(tokenDir, ".token-*")
	if err != nil {
		return fmt.Errorf("creating temp token file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			os.Remove(tmpPath)
		}
	}()

	if _, err := fileWrite(tmp, data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing token: %w", err)
	}
	if err := fileChmod(tmp, 0600); err != nil {
		tmp.Close()
		return fmt.Errorf("setting token permissions: %w", err)
	}
	if err := fileSync(tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing token file: %w", err)
	}
	if err := fileClose(tmp); err != nil {
		return fmt.Errorf("closing token file: %w", err)
	}
	if err := osRename(tmpPath, tokenPath); err != nil {
		return fmt.Errorf("writing token: %w", err)
	}
	return nil
}

func (ts *TokenStore) readTokenFile(profile string) ([]byte, error) {
	tokenPath, err := sanitizeTokenPath(profile)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("reading token: %w", err)
	}
	return payload, nil
}

func (ts *TokenStore) deleteFile(profile string) error {
	tokenPath, err := sanitizeTokenPath(profile)
	if err != nil {
		return err
	}
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting token: %w", err)
	}
	return nil
}

// zeroBytes overwrites a byte slice with zeros to remove sensitive material
// (e.g. derived encryption keys) from memory as soon as they are no longer needed.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
