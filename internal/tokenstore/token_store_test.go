package tokenstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	cryptoSvc "github.com/justjundana/git-config-manager/internal/service/crypto"
	"github.com/justjundana/git-config-manager/pkg/logger"

	"github.com/zalando/go-keyring"
)

// --- helpers ---------------------------------------------------------------

func newPlainStore(t *testing.T) *TokenStore {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = false
	cfg.Security.AllowPlaintextTokens = true
	log := logger.New(logger.LevelError, os.Stderr)
	return NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
}

func newEncryptedStore(t *testing.T, password string) *TokenStore {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)
	prompt := func(_ string) (string, error) { return password, nil }
	return NewTokenStore(cfg, cryptoSvc.NewService(), log, prompt)
}

// --- plain-text backend ----------------------------------------------------

func TestTokenStore_Plain_SaveLoadDelete(t *testing.T) {
	ts := newPlainStore(t)

	if err := ts.Save("myprofile", "ghp_plain123"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	tok, err := ts.Load("myprofile")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok != "ghp_plain123" {
		t.Errorf("token = %q, want %q", tok, "ghp_plain123")
	}
	if err := ts.Delete("myprofile"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := ts.Load("myprofile"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestTokenStore_Plain_DisabledWithoutExplicitOptIn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = false
	cfg.Security.MasterPassword = false
	cfg.Security.AllowPlaintextTokens = false
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), nil)

	if err := ts.Save("plain-disabled", "tok"); err == nil || !strings.Contains(err.Error(), "plaintext token storage disabled") {
		t.Fatalf("Save error = %v", err)
	}
	if _, err := ts.Load("plain-disabled"); err == nil || !strings.Contains(err.Error(), "plaintext token storage disabled") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestTokenStore_PlainBackendRejectsDirectUseWhenDisabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = false
	cfg.Security.AllowPlaintextTokens = false
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), nil)

	if err := ts.savePlain("plain-disabled-direct", "tok"); err == nil || !strings.Contains(err.Error(), "plaintext token storage disabled") {
		t.Fatalf("savePlain error = %v", err)
	}
	if _, err := ts.loadPlain("plain-disabled-direct"); err == nil || !strings.Contains(err.Error(), "plaintext token storage disabled") {
		t.Fatalf("loadPlain error = %v", err)
	}
}

func TestTokenStore_Plain_EmptyFile(t *testing.T) {
	ts := newPlainStore(t)

	path, _ := sanitizeTokenPath("empty")
	os.MkdirAll(config.GCMDir()+"/tokens", 0o700)
	os.WriteFile(path, []byte("  \n"), 0o600)

	_, err := ts.Load("empty")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

// --- encrypted backend -----------------------------------------------------

func TestTokenStore_Encrypted_SaveLoad(t *testing.T) {
	ts := newEncryptedStore(t, "s3cret-pa$$word")

	if err := ts.Save("encrypted", "ghp_encrypted_tok"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-create store with same password to verify we can reload.
	ts2 := NewTokenStore(ts.cfg, ts.crypto, ts.log, func(_ string) (string, error) {
		return "s3cret-pa$$word", nil
	})
	tok, err := ts2.Load("encrypted")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok != "ghp_encrypted_tok" {
		t.Errorf("token = %q, want %q", tok, "ghp_encrypted_tok")
	}
}

func TestTokenStore_Encrypted_WrongPassword(t *testing.T) {
	ts := newEncryptedStore(t, "correct-password")
	if err := ts.Save("wrongpw", "ghp_secret"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load with a different password — decryption should fail.
	ts2 := NewTokenStore(ts.cfg, ts.crypto, ts.log, func(_ string) (string, error) {
		return "wrong-password", nil
	})
	_, err := ts2.Load("wrongpw")
	if err == nil {
		t.Fatal("expected error for wrong master password")
	}
}

func TestTokenStore_Encrypted_NoPrompt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)

	ts := NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	err := ts.Save("noprompt", "tok")
	if err == nil {
		t.Fatal("expected error when promptFunc is nil")
	}
}

func TestTokenStore_Encrypted_EmptyPassword(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), log, func(_ string) (string, error) {
		return "", nil
	})
	err := ts.Save("emptypw", "tok")
	if err == nil {
		t.Fatal("expected error for empty master password")
	}
}

func TestTokenStore_Encrypted_CorruptFile(t *testing.T) {
	ts := newEncryptedStore(t, "mypass")
	path, _ := sanitizeTokenPath("corrupt")
	os.MkdirAll(config.GCMDir()+"/tokens", 0o700)
	// Write garbage that's too short.
	os.WriteFile(path, []byte{0x00, 0x10}, 0o600)

	_, err := ts.Load("corrupt")
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

// --- backend selection -----------------------------------------------------

func TestTokenStore_BackendSelection_Keychain(t *testing.T) {
	// Keychain operations may trigger OS-level dialogs (macOS Keychain
	// authorization) that block headless test runs. We verify the
	// backend-selection logic only: when UseKeychain is true, the store
	// should NOT fall through to the encrypted-file backend.
	if os.Getenv("GCM_TEST_KEYCHAIN") == "" {
		t.Skip("skipping keychain test (set GCM_TEST_KEYCHAIN=1 to run)")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = true
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)

	err := ts.Save("keychaintest", "tok")
	if err != nil && containsStr(err.Error(), "master password") {
		t.Errorf("keychain should take priority, got master-password error: %v", err)
	}
	if err == nil {
		_ = ts.Delete("keychaintest")
	}
}

func TestTokenStore_SetPromptFunc(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)
	// Start with nil prompt, then set it via SetPromptFunc.
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	ts.SetPromptFunc(func(_ string) (string, error) {
		return "replaced", nil
	})
	// Save should work now using the injected prompt.
	err := ts.Save("setprompt", "ghp_test")
	if err != nil {
		t.Fatalf("Save with SetPromptFunc: %v", err)
	}
}

// --- delete fallback -------------------------------------------------------

func TestTokenStore_Delete_NonExistent(t *testing.T) {
	ts := newPlainStore(t)
	if err := ts.Delete("doesnotexist"); err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestTokenStore_Delete_InvalidProfile(t *testing.T) {
	ts := newPlainStore(t)
	if err := ts.Delete("../escape"); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestTokenStore_Plain_LoadNonExistent(t *testing.T) {
	ts := newPlainStore(t)
	_, err := ts.Load("nosuchprofile")
	if err == nil {
		t.Fatal("expected error loading nonexistent token")
	}
}

func TestTokenStore_Plain_SaveOverwrite(t *testing.T) {
	ts := newPlainStore(t)
	ts.Save("prof", "tok1")
	ts.Save("prof", "tok2")
	tok, err := ts.Load("prof")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok2" {
		t.Errorf("got %q, want tok2", tok)
	}
}

func TestTokenStore_Encrypted_DeleteThenLoad(t *testing.T) {
	ts := newEncryptedStore(t, "pass123")
	ts.Save("ep", "secret")
	ts.Delete("ep")
	_, err := ts.Load("ep")
	if err == nil {
		t.Fatal("expected error loading deleted token")
	}
}

func TestTokenStore_Encrypted_EmptyTokenAfterDecrypt(t *testing.T) {
	ts := newEncryptedStore(t, "pass123")
	// Save empty-ish token (whitespace only)
	ts.Save("ep", "   ")
	_, err := ts.Load("ep")
	if err == nil {
		t.Fatal("expected error for empty token after decryption")
	}
}

func TestTokenStore_SanitizeTokenPath_EmptyProfile(t *testing.T) {
	_, err := sanitizeTokenPath("")
	if err == nil {
		t.Fatal("expected error for empty profile")
	}
}

func TestTokenStore_SanitizeTokenPath_Slash(t *testing.T) {
	_, err := sanitizeTokenPath("a/b")
	if err == nil {
		t.Fatal("expected error for slash in profile")
	}
}

func TestTokenStore_SanitizeTokenPath_Backslash(t *testing.T) {
	_, err := sanitizeTokenPath(`a\b`)
	if err == nil {
		t.Fatal("expected error for backslash in profile")
	}
}

func TestTokenStore_SanitizeTokenPath_DotDot(t *testing.T) {
	_, err := sanitizeTokenPath("..")
	if err == nil {
		t.Fatal("expected error for '..'")
	}
}

func TestTokenStore_SanitizeTokenPath_Valid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	path, err := sanitizeTokenPath("myprofile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestTokenStore_Delete_PlainBackend(t *testing.T) {
	ts := newPlainStore(t)
	ts.Save("delme", "tok")
	if err := ts.Delete("delme"); err != nil {
		t.Fatal(err)
	}
	_, err := ts.Load("delme")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestTokenStore_Encrypted_CorruptSaltLen(t *testing.T) {
	ts := newEncryptedStore(t, "pass123")
	// Save a valid token first to get the tokens dir created
	ts.Save("corr", "valid")
	// Overwrite with corrupt data: salt len > payload
	path, _ := sanitizeTokenPath("corr")
	os.WriteFile(path, []byte{0xFF, 0xFF, 0x01}, 0600)
	_, err := ts.Load("corr")
	if err == nil {
		t.Fatal("expected error for corrupt salt length")
	}
}

// =============================================================================
// Additional coverage: Delete error paths
// =============================================================================

func TestTokenStore_Delete_WithExistingToken(t *testing.T) {
	ts := newPlainStore(t)
	ts.Save("deleteexisting", "ghp_token123")

	if err := ts.Delete("deleteexisting"); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	// Verify it's actually gone
	_, err := ts.Load("deleteexisting")
	if err == nil {
		t.Fatal("expected error loading deleted token")
	}
}

func TestTokenStore_Delete_EmptyProfileName(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Delete("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestTokenStore_Delete_PathTraversalSlash(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Delete("foo/bar")
	if err == nil {
		t.Fatal("expected error for path with slash")
	}
}

func TestTokenStore_Delete_PathTraversalBackslash(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Delete(`foo\bar`)
	if err == nil {
		t.Fatal("expected error for path with backslash")
	}
}

func TestTokenStore_Delete_PathTraversalDotDot(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Delete("..profile")
	if err == nil {
		t.Fatal("expected error for path traversal with ..")
	}
}

// =============================================================================
// Additional coverage: Save/Load with path sanitization
// =============================================================================

func TestTokenStore_Save_EmptyProfile(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Save("", "token")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestTokenStore_Load_EmptyProfile(t *testing.T) {
	ts := newPlainStore(t)
	_, err := ts.Load("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestTokenStore_Save_PathTraversal(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Save("../escape", "token")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestTokenStore_Load_PathTraversal(t *testing.T) {
	ts := newPlainStore(t)
	_, err := ts.Load("../escape")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

// =============================================================================
// Additional coverage: Encrypted backend - prompt error
// =============================================================================

func TestTokenStore_Encrypted_PromptReturnsError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = true
	cfg.Security.MasterPassword = true
	log := logger.New(logger.LevelError, os.Stderr)

	ts := NewTokenStore(cfg, cryptoSvc.NewService(), log, func(_ string) (string, error) {
		return "", fmt.Errorf("user cancelled")
	})
	err := ts.Save("prompterr", "tok")
	if err == nil {
		t.Fatal("expected error when prompt returns error")
	}
}

// =============================================================================
// Additional coverage: Delete paths
// =============================================================================

func TestTokenStore_Delete_FileExistsPlainBackend(t *testing.T) {
	ts := newPlainStore(t)
	// Save and then delete to cover successful delete path
	if err := ts.Save("deltest", "ghp_todelete"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Verify token exists
	if _, err := ts.Load("deltest"); err != nil {
		t.Fatalf("Load before delete: %v", err)
	}
	// Delete
	if err := ts.Delete("deltest"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Verify it's gone
	if _, err := ts.Load("deltest"); err == nil {
		t.Fatal("expected error loading deleted token")
	}
}

func TestTokenStore_Delete_NonExistentFile(t *testing.T) {
	ts := newPlainStore(t)
	// Deleting a token that was never saved should succeed (os.IsNotExist is ignored)
	err := ts.Delete("never-saved-profile")
	if err != nil {
		t.Fatalf("Delete nonexistent: %v", err)
	}
}

func TestTokenStore_Delete_EncryptedBackend(t *testing.T) {
	ts := newEncryptedStore(t, "testpass")
	// Save encrypted
	if err := ts.Save("encdelete", "ghp_encrypted"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Delete goes through deleteFile since not keychain
	if err := ts.Delete("encdelete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Verify deletion
	ts2 := NewTokenStore(ts.cfg, ts.crypto, ts.log, func(_ string) (string, error) {
		return "testpass", nil
	})
	if _, err := ts2.Load("encdelete"); err == nil {
		t.Fatal("expected error loading deleted encrypted token")
	}
}

func TestTokenStore_Delete_PathTraversalProfile(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.Delete("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal in delete")
	}
}

func TestTokenStore_Plain_SaveAndOverwrite(t *testing.T) {
	ts := newPlainStore(t)
	if err := ts.Save("overwrite", "first-token"); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := ts.Save("overwrite", "second-token"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	tok, err := ts.Load("overwrite")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok != "second-token" {
		t.Errorf("token = %q, want %q", tok, "second-token")
	}
}

func TestTokenStore_Encrypted_LoadNonExistent(t *testing.T) {
	ts := newEncryptedStore(t, "pass123")
	_, err := ts.Load("nonexistent-encrypted")
	if err == nil {
		t.Fatal("expected error loading nonexistent encrypted token")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestTokenStore_WriteTokenFile_UnwritableDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		Security: config.SecurityConfig{EncryptTokens: false, AllowPlaintextTokens: true},
	}

	// Create .gcm dir, then make tokens a read-only dir with a file inside
	gcmDir := filepath.Join(tmp, ".gcm")
	tokensDir := filepath.Join(gcmDir, "tokens")
	os.MkdirAll(tokensDir, 0o755)
	os.Chmod(tokensDir, 0o000)
	defer os.Chmod(tokensDir, 0o755)

	ts := NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), nil)
	err := ts.Save("blocked-profile", "token123")
	if err == nil {
		t.Fatal("expected error when tokens dir is unwritable")
	}
}

func TestTokenStore_Encrypted_CorruptPayloadTooShort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}

	// Write a too-short encrypted token file
	tokenDir := filepath.Join(tmp, ".gcm", "tokens")
	os.MkdirAll(tokenDir, 0o700)
	os.WriteFile(filepath.Join(tokenDir, "short.token"), []byte{0x00}, 0o600)

	ts := NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), func(msg string) (string, error) {
		return "password", nil
	})
	_, err := ts.Load("short")
	if err == nil {
		t.Fatal("expected error for too-short encrypted file")
	}
}

func TestTokenStore_Encrypted_CorruptSaltLenTooBig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}

	// Write token file with salt length bigger than payload
	tokenDir := filepath.Join(tmp, ".gcm", "tokens")
	os.MkdirAll(tokenDir, 0o700)
	data := []byte{0xFF, 0xFF, 0x01, 0x02, 0x03} // salt len = 65535, payload only 3 bytes
	os.WriteFile(filepath.Join(tokenDir, "bigsalt.token"), data, 0o600)

	ts := NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), func(msg string) (string, error) {
		return "password", nil
	})
	_, err := ts.Load("bigsalt")
	if err == nil {
		t.Fatal("expected error for corrupt salt length")
	}
}

func TestTokenStore_Encrypted_SaveNoPromptFunc(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}

	ts := NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), nil)
	err := ts.Save("noprompt", "token")
	if err == nil {
		t.Fatal("expected error when no prompt func set for encrypted save")
	}
}

func TestTokenStore_Encrypted_LoadNoPromptFunc(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}

	// Write a valid-looking encrypted file
	tokenDir := filepath.Join(tmp, ".gcm", "tokens")
	os.MkdirAll(tokenDir, 0o700)
	// salt len=16, then 16 bytes of salt, then some ciphertext
	payload := make([]byte, 2+16+32)
	payload[0] = 0
	payload[1] = 16
	os.WriteFile(filepath.Join(tokenDir, "noprompt.token"), payload, 0o600)

	ts := NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), nil)
	_, err := ts.Load("noprompt")
	if err == nil {
		t.Fatal("expected error when no prompt func for encrypted load")
	}
}

func TestTokenStore_ZeroPassword(t *testing.T) {
	ts := newEncryptedStore(t, "super-secret")

	if _, err := ts.getMasterPassword(); err != nil {
		t.Fatalf("getMasterPassword: %v", err)
	}
	if len(ts.cachedPassword) == 0 {
		t.Fatal("expected cached password to be populated")
	}

	ts.ZeroPassword()
	for i, b := range ts.cachedPassword {
		if b != 0 {
			t.Fatalf("cachedPassword[%d] = %d, want 0", i, b)
		}
	}
}

// =============================================================================
// Keychain backend tests using mock keyring functions
// =============================================================================

func withMockKeyring(store map[string]string, fn func()) {
	oldSet := keyringSet
	oldGet := keyringGet
	oldDel := keyringDelete
	defer func() {
		keyringSet = oldSet
		keyringGet = oldGet
		keyringDelete = oldDel
	}()

	keyringSet = func(service, user, password string) error {
		store[service+"/"+user] = password
		return nil
	}
	keyringGet = func(service, user string) (string, error) {
		v, ok := store[service+"/"+user]
		if !ok {
			return "", fmt.Errorf("not found")
		}
		return v, nil
	}
	keyringDelete = func(service, user string) error {
		delete(store, service+"/"+user)
		return nil
	}
	fn()
}

func newKeychainStore(t *testing.T) *TokenStore {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := &config.Config{
		Security: config.SecurityConfig{UseKeychain: true},
	}
	return NewTokenStore(cfg, nil, logger.New(logger.LevelError, os.Stderr), nil)
}

func TestTokenStore_Keychain_SaveLoadDelete(t *testing.T) {
	store := make(map[string]string)
	withMockKeyring(store, func() {
		ts := newKeychainStore(t)
		if err := ts.Save("kc-prof", "ghp_keychain_token"); err != nil {
			t.Fatalf("Save: %v", err)
		}
		tok, err := ts.Load("kc-prof")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if tok != "ghp_keychain_token" {
			t.Errorf("got %q, want ghp_keychain_token", tok)
		}
		if err := ts.Delete("kc-prof"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := ts.Load("kc-prof"); err == nil {
			t.Fatal("expected error after delete")
		}
	})
}

func TestTokenStore_Keychain_SaveError(t *testing.T) {
	oldSet := keyringSet
	defer func() { keyringSet = oldSet }()
	keyringSet = func(_, _, _ string) error { return fmt.Errorf("keychain locked") }

	ts := newKeychainStore(t)
	err := ts.Save("kc-err", "tok")
	if err == nil || !strings.Contains(err.Error(), "plaintext token storage disabled") {
		t.Fatalf("expected fail-closed keychain error, got: %v", err)
	}
}

func TestTokenStore_Keychain_SaveErrorPlainFallbackRequiresOptIn(t *testing.T) {
	oldSet := keyringSet
	defer func() { keyringSet = oldSet }()
	keyringSet = func(_, _, _ string) error { return fmt.Errorf("keychain locked") }

	ts := newKeychainStore(t)
	ts.cfg.Security.AllowPlaintextTokens = true
	if err := ts.Save("kc-plain-fallback", "tok"); err != nil {
		t.Fatalf("Save with explicit plaintext fallback: %v", err)
	}
	tok, err := ts.Load("kc-plain-fallback")
	if err != nil {
		t.Fatalf("Load after fallback: %v", err)
	}
	if tok != "tok" {
		t.Errorf("got token %q, want %q", tok, "tok")
	}
}

func TestTokenStore_Keychain_LoadError(t *testing.T) {
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()
	keyringGet = func(_, _ string) (string, error) { return "", fmt.Errorf("not found") }

	ts := newKeychainStore(t)
	_, err := ts.Load("kc-err")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTokenStore_Keychain_LoadEmptyToken(t *testing.T) {
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()
	keyringGet = func(_, _ string) (string, error) { return "", nil }

	ts := newKeychainStore(t)
	_, err := ts.Load("kc-empty")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestTokenStore_Keychain_DeleteNotFound(t *testing.T) {
	store := make(map[string]string)
	withMockKeyring(store, func() {
		ts := newKeychainStore(t)
		err := ts.Delete("kc-missing")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})
}

func TestTokenStore_Keychain_DeleteError(t *testing.T) {
	oldDel := keyringDelete
	defer func() { keyringDelete = oldDel }()
	keyringDelete = func(_, _ string) error { return fmt.Errorf("access denied") }

	ts := newKeychainStore(t)
	err := ts.Delete("kc-err")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsKeyringNotFound(t *testing.T) {
	if !isKeyringNotFound(keyring.ErrNotFound) {
		t.Error("expected true for keyring.ErrNotFound")
	}
	if isKeyringNotFound(fmt.Errorf("other error")) {
		t.Error("expected false for other error")
	}
}

// --- sanitizeTokenPath edge cases ------------------------------------------

func TestSanitizeTokenPath_EmptyProfile(t *testing.T) {
	_, err := sanitizeTokenPath("")
	if err == nil {
		t.Fatal("expected error for empty profile")
	}
}

func TestSanitizeTokenPath_SlashInName(t *testing.T) {
	_, err := sanitizeTokenPath("foo/bar")
	if err == nil {
		t.Fatal("expected error for slash in profile name")
	}
}

func TestSanitizeTokenPath_BackslashInName(t *testing.T) {
	_, err := sanitizeTokenPath(`foo\bar`)
	if err == nil {
		t.Fatal("expected error for backslash in profile name")
	}
}

func TestSanitizeTokenPath_DotDotInName(t *testing.T) {
	_, err := sanitizeTokenPath("..escape")
	if err == nil {
		t.Fatal("expected error for .. in profile name")
	}
}

func TestSanitizeTokenPath_ValidName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := sanitizeTokenPath("valid-profile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
}

// --- writeTokenFile / deleteFile edge cases --------------------------------

func TestWriteTokenFile_InvalidProfile(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.writeTokenFile("../escape", []byte("data"))
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
}

func TestDeleteFile_ReadOnlyDir(t *testing.T) {
	ts := newPlainStore(t)
	// Save a token first, then make the directory read-only
	ts.Save("readonlytest", "tok")
	tokenPath, _ := sanitizeTokenPath("readonlytest")
	dir := filepath.Dir(tokenPath)
	os.Chmod(dir, 0o500)
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	err := ts.deleteFile("readonlytest")
	if err == nil {
		t.Fatal("expected error removing file from read-only dir")
	}
}

func TestTokenStore_Encrypted_V1BackwardCompat(t *testing.T) {
	// Simulate a v1 (PBKDF2) encrypted token file written by an older version
	// and verify the new code can still decrypt it.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	password := "myOldPassword"
	token := "ghp_legacy_v1_token"
	crypto := cryptoSvc.NewService()

	// Produce a v1-format file: [2-byte saltLen | salt | ciphertext]
	salt, err := crypto.GenerateSalt()
	if err != nil {
		t.Fatal(err)
	}
	key := crypto.DeriveKey(password, salt) // PBKDF2
	ciphertext, err := crypto.Encrypt([]byte(token), key)
	if err != nil {
		t.Fatal(err)
	}

	var buf []byte
	buf = append(buf, 0x00, byte(len(salt))) // uint16 big-endian = 16
	buf = append(buf, salt...)
	buf = append(buf, ciphertext...)

	tokenDir := filepath.Join(tmp, ".gcm", "tokens")
	os.MkdirAll(tokenDir, 0o700)
	if err := os.WriteFile(filepath.Join(tokenDir, "v1profile.token"), buf, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}
	ts := NewTokenStore(cfg, crypto, logger.New(logger.LevelError, os.Stderr), func(_ string) (string, error) {
		return password, nil
	})

	loaded, err := ts.Load("v1profile")
	if err != nil {
		t.Fatalf("Load v1 token: %v", err)
	}
	if loaded != token {
		t.Errorf("Load v1 = %q, want %q", loaded, token)
	}
}

func TestTokenStore_Encrypted_V2RoundTrip(t *testing.T) {
	// Verify v2 (Argon2id) save + load works end-to-end.
	ts := newEncryptedStore(t, "argon2pass")

	if err := ts.Save("v2profile", "ghp_argon2id_token"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := ts.Load("v2profile")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != "ghp_argon2id_token" {
		t.Errorf("Load = %q, want %q", loaded, "ghp_argon2id_token")
	}

	// Verify the on-disk file starts with the v2 magic byte
	tokenPath, _ := sanitizeTokenPath("v2profile")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("reading token file: %v", err)
	}
	if data[0] != tokenFormatV2 {
		t.Errorf("token file first byte = 0x%02x, want 0x%02x (v2 marker)", data[0], tokenFormatV2)
	}
}

// =============================================================================
// Coverage: writeTokenFile hook-based error paths
// =============================================================================

func TestWriteTokenFile_MkdirAllError(t *testing.T) {
	old := osMkdirAll
	defer func() { osMkdirAll = old }()
	osMkdirAll = func(_ string, _ os.FileMode) error { return fmt.Errorf("disk full") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from MkdirAll")
	}
}

func TestWriteTokenFile_CreateTempError(t *testing.T) {
	old := osCreateTemp
	defer func() { osCreateTemp = old }()
	osCreateTemp = func(_ string, _ string) (*os.File, error) { return nil, fmt.Errorf("no space") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from CreateTemp")
	}
}

func TestWriteTokenFile_WriteError(t *testing.T) {
	old := fileWrite
	defer func() { fileWrite = old }()
	fileWrite = func(f *os.File, _ []byte) (int, error) { return 0, fmt.Errorf("write error") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from Write")
	}
}

func TestWriteTokenFile_ChmodError(t *testing.T) {
	old := fileChmod
	defer func() { fileChmod = old }()
	fileChmod = func(_ *os.File, _ os.FileMode) error { return fmt.Errorf("chmod error") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from Chmod")
	}
}

func TestWriteTokenFile_SyncError(t *testing.T) {
	old := fileSync
	defer func() { fileSync = old }()
	fileSync = func(_ *os.File) error { return fmt.Errorf("sync error") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from Sync")
	}
}

func TestWriteTokenFile_CloseError(t *testing.T) {
	old := fileClose
	defer func() { fileClose = old }()
	fileClose = func(f *os.File) error {
		f.Close() // actually close it to avoid leak
		return fmt.Errorf("close error")
	}

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from Close")
	}
}

func TestWriteTokenFile_RenameError(t *testing.T) {
	old := osRename
	defer func() { osRename = old }()
	osRename = func(_, _ string) error { return fmt.Errorf("rename error") }

	ts := newPlainStore(t)
	err := ts.Save("prof", "token")
	if err == nil {
		t.Fatal("expected error from Rename")
	}
}

// =============================================================================
// Coverage: sanitizeTokenPath hook-based error paths
// =============================================================================

func TestSanitizeTokenPath_FilepathAbsError(t *testing.T) {
	old := filepathAbs
	defer func() { filepathAbs = old }()
	filepathAbs = func(_ string) (string, error) { return "", fmt.Errorf("abs error") }

	_, err := sanitizeTokenPath("validname")
	if err == nil {
		t.Fatal("expected error from filepath.Abs")
	}
}

func TestSanitizeTokenPath_FilepathRelError(t *testing.T) {
	old := filepathRel
	defer func() { filepathRel = old }()
	filepathRel = func(_, _ string) (string, error) { return "", fmt.Errorf("rel error") }

	t.Setenv("HOME", t.TempDir())
	_, err := sanitizeTokenPath("validname")
	if err == nil {
		t.Fatal("expected error from filepath.Rel")
	}
}

// =============================================================================
// Coverage: keychain fallback to encrypted backend
// =============================================================================

func TestTokenStore_Keychain_SaveFallbackToEncrypted(t *testing.T) {
	// When keychain fails AND EncryptTokens+MasterPassword are set,
	// saveKeychain should fallback to saveEncrypted.
	oldSet := keyringSet
	defer func() { keyringSet = oldSet }()
	keyringSet = func(_, _, _ string) error { return fmt.Errorf("keychain locked") }

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := &config.Config{
		Security: config.SecurityConfig{
			UseKeychain:    true,
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}
	ts := NewTokenStore(cfg, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), func(_ string) (string, error) {
		return "testpass", nil
	})

	err := ts.Save("fallback-enc", "ghp_encrypted_fallback")
	if err != nil {
		t.Fatalf("Save with encrypted fallback: %v", err)
	}

	// Verify we can load via encrypted backend directly
	cfg2 := &config.Config{
		Security: config.SecurityConfig{
			UseKeychain:    false,
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}
	ts2 := NewTokenStore(cfg2, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), func(_ string) (string, error) {
		return "testpass", nil
	})
	tok, err := ts2.Load("fallback-enc")
	if err != nil {
		t.Fatalf("Load encrypted fallback: %v", err)
	}
	if tok != "ghp_encrypted_fallback" {
		t.Errorf("token = %q", tok)
	}
}

func TestTokenStore_Keychain_LoadFallbackToEncrypted(t *testing.T) {
	// When keychain read fails, loadKeychain should try loadEncrypted
	// if EncryptTokens+MasterPassword are set and the file exists.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	password := "testpass"
	cfg := &config.Config{
		Security: config.SecurityConfig{
			UseKeychain:    true,
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}

	// First, save a token via encrypted backend directly
	cfgEnc := &config.Config{
		Security: config.SecurityConfig{
			EncryptTokens:  true,
			MasterPassword: true,
		},
	}
	tsEnc := NewTokenStore(cfgEnc, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), func(_ string) (string, error) {
		return password, nil
	})
	if err := tsEnc.Save("kc-fallback", "ghp_from_file"); err != nil {
		t.Fatalf("Save encrypted: %v", err)
	}

	// Now try to load via keychain store with keyringGet failing
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()
	keyringGet = func(_, _ string) (string, error) { return "", fmt.Errorf("keychain unavailable") }

	ts := NewTokenStore(cfg, cryptoSvc.NewService(), logger.New(logger.LevelError, os.Stderr), func(_ string) (string, error) {
		return password, nil
	})
	tok, err := ts.Load("kc-fallback")
	if err != nil {
		t.Fatalf("Load with encrypted fallback: %v", err)
	}
	if tok != "ghp_from_file" {
		t.Errorf("token = %q, want ghp_from_file", tok)
	}
}

// =============================================================================
// Coverage: crypto hook error paths in saveEncrypted
// =============================================================================

func TestTokenStore_Encrypted_GenerateSaltError(t *testing.T) {
	old := generateSalt
	defer func() { generateSalt = old }()
	generateSalt = func(_ *cryptoSvc.Service) ([]byte, error) {
		return nil, fmt.Errorf("entropy exhausted")
	}

	ts := newEncryptedStore(t, "pass")
	err := ts.Save("salterr", "token")
	if err == nil {
		t.Fatal("expected error from GenerateSalt")
	}
}

func TestTokenStore_Encrypted_EncryptError(t *testing.T) {
	old := encryptData
	defer func() { encryptData = old }()
	encryptData = func(_ *cryptoSvc.Service, _, _ []byte) ([]byte, error) {
		return nil, fmt.Errorf("encryption failed")
	}

	ts := newEncryptedStore(t, "pass")
	err := ts.Save("encerr", "token")
	if err == nil {
		t.Fatal("expected error from Encrypt")
	}
}

// =============================================================================
// Coverage: loadEncrypted corrupt v2 file edge cases
// =============================================================================

func TestTokenStore_Encrypted_V2TooShortPayload(t *testing.T) {
	// v2 file with length >= 2+tokenSaltLen (18) but < 3+tokenSaltLen (19)
	// This passes the outer length check but fails the v2-specific inner check.
	ts := newEncryptedStore(t, "pass")
	path, _ := sanitizeTokenPath("v2short")
	os.MkdirAll(filepath.Dir(path), 0o700)
	data := make([]byte, 18) // exactly 2+tokenSaltLen, passes outer check
	data[0] = tokenFormatV2  // enters v2 branch
	os.WriteFile(path, data, 0o600)

	_, err := ts.Load("v2short")
	if err == nil {
		t.Fatal("expected error for too-short v2 payload")
	}
}

func TestTokenStore_Encrypted_V2BadSaltLen(t *testing.T) {
	// v2 file where sLen < 1 or 3+sLen > len(payload)
	ts := newEncryptedStore(t, "pass")
	path, _ := sanitizeTokenPath("v2badsalt")
	os.MkdirAll(filepath.Dir(path), 0o700)
	// Make a file long enough to pass len check (>= 19) but with bad salt len
	data := make([]byte, 20)
	data[0] = tokenFormatV2
	data[1] = 0x00
	data[2] = 0x00 // salt len = 0, which is < 1
	os.WriteFile(path, data, 0o600)

	_, err := ts.Load("v2badsalt")
	if err == nil {
		t.Fatal("expected error for bad v2 salt length")
	}
}

func TestTokenStore_Encrypted_V1BadSaltLen(t *testing.T) {
	// v1 file where saltLen < 1 or 2+saltLen > len(payload)
	// Must pass the first check: len(payload) >= 2+tokenSaltLen (18)
	ts := newEncryptedStore(t, "pass")
	path, _ := sanitizeTokenPath("v1badsalt")
	os.MkdirAll(filepath.Dir(path), 0o700)
	// 20 bytes, first byte != 0x02 (v1 format), salt len = 0
	data := make([]byte, 20)
	data[0] = 0x00
	data[1] = 0x00 // salt len = 0, which is < 1
	os.WriteFile(path, data, 0o600)

	_, err := ts.Load("v1badsalt")
	if err == nil {
		t.Fatal("expected error for bad v1 salt length")
	}
}

func TestIsKeychainAvailable_Available(t *testing.T) {
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()

	// Simulate keychain that returns ErrNotFound (keychain works, key just doesn't exist)
	keyringGet = func(_, _ string) (string, error) { return "", keyring.ErrNotFound }

	if !IsKeychainAvailable() {
		t.Error("expected IsKeychainAvailable to return true when keychain returns ErrNotFound")
	}
}

func TestIsKeychainAvailable_Available_NoError(t *testing.T) {
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()

	// Simulate keychain that returns no error (key unexpectedly exists)
	keyringGet = func(_, _ string) (string, error) { return "value", nil }

	if !IsKeychainAvailable() {
		t.Error("expected IsKeychainAvailable to return true when keychain returns no error")
	}
}

func TestIsKeychainAvailable_Unavailable(t *testing.T) {
	oldGet := keyringGet
	defer func() { keyringGet = oldGet }()

	// Simulate keychain unavailable (e.g. unsupported platform)
	keyringGet = func(_, _ string) (string, error) { return "", fmt.Errorf("unsupported platform") }

	if IsKeychainAvailable() {
		t.Error("expected IsKeychainAvailable to return false when keychain errors")
	}
}
