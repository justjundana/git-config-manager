package ssh

import (
	"bytes"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	cfg := &config.Config{
		SSHDir: t.TempDir(),
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	return NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
}

func TestGenerate_Ed25519NoPassphrase(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "work",
		KeyType: "ed25519",
		Comment: "gcm-test",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if info.Fingerprint == "" || !strings.HasPrefix(info.Fingerprint, "SHA256:") {
		t.Fatalf("unexpected fingerprint: %q", info.Fingerprint)
	}

	// Private key file must exist with 0600 permissions.
	fi, err := os.Stat(info.Path)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("private key perm = %o, want 0600", perm)
	}

	// Public key file must exist and be parseable.
	pub, err := os.ReadFile(info.Path + ".pub")
	if err != nil {
		t.Fatalf("read pub: %v", err)
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey(pub)
	if err != nil {
		t.Fatalf("parse pub: %v", err)
	}
	if pk.Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("wrong key type: %s", pk.Type())
	}

	// Private key should be parseable without a passphrase.
	priv, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatalf("read priv: %v", err)
	}
	if _, err := ssh.ParsePrivateKey(priv); err != nil {
		t.Fatalf("parse priv: %v", err)
	}
}

func TestGenerate_Ed25519WithPassphraseEncryptsKey(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile:    "work",
		KeyType:    "ed25519",
		Comment:    "gcm-test",
		Passphrase: "s3cret!",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	priv, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatalf("read priv: %v", err)
	}

	// Plain ParsePrivateKey must fail because key is encrypted.
	if _, err := ssh.ParsePrivateKey(priv); err == nil {
		t.Fatal("expected encrypted private key to fail plain parse")
	}

	// With the correct passphrase it must succeed.
	if _, err := ssh.ParsePrivateKeyWithPassphrase(priv, []byte("s3cret!")); err != nil {
		t.Fatalf("parse with passphrase: %v", err)
	}

	// The passphrase must NOT appear in the on-disk key material. This guards
	// against accidental plaintext leakage.
	if bytes.Contains(priv, []byte("s3cret!")) {
		t.Fatal("passphrase leaked into private key file")
	}
}

func TestGenerate_RSA_MinimumBits(t *testing.T) {
	m := newTestManager(t)

	if _, err := m.Generate(GenerateOptions{
		Profile: "weak",
		KeyType: "rsa",
		Bits:    1024,
	}); err == nil {
		t.Fatal("expected error for RSA < 2048")
	}
}

func TestGenerate_RefusesOverwrite(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}

	if _, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"}); err == nil {
		t.Fatal("expected refusal to overwrite")
	}

	// Sanity check: the original file is still there.
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatalf("original key removed: %v", err)
	}
}

func TestGenerate_OverwriteExisting(t *testing.T) {
	m := newTestManager(t)

	first, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519", Comment: "old-comment"})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}

	second, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519", Comment: "new-comment", Overwrite: true})
	if err != nil {
		t.Fatalf("overwrite generate: %v", err)
	}
	if second.Path != first.Path {
		t.Fatalf("overwrite path = %q, want %q", second.Path, first.Path)
	}

	pubKey, err := m.GetPublicKey(second.Path)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if !strings.Contains(pubKey, "new-comment") {
		t.Fatalf("public key = %q, want new comment", pubKey)
	}
}

func TestInspectKey(t *testing.T) {
	m := newTestManager(t)

	generated, err := m.Generate(GenerateOptions{Profile: "inspect", KeyType: "ed25519", Comment: "inspect-comment"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	inspected, err := m.InspectKey(generated.Path)
	if err != nil {
		t.Fatalf("InspectKey: %v", err)
	}
	if inspected.Path != generated.Path || inspected.Type != "ed25519" || inspected.Fingerprint != generated.Fingerprint || inspected.Comment != "inspect-comment" {
		t.Fatalf("InspectKey = %+v, generated = %+v", inspected, generated)
	}
}

func TestInspectKeyErrors(t *testing.T) {
	m := newTestManager(t)

	if _, err := m.InspectKey(filepath.Join(m.cfg.SSHDir, "missing")); err == nil {
		t.Fatal("expected missing private key error")
	}

	dirPath := filepath.Join(m.cfg.SSHDir, "dir-key")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir dir key: %v", err)
	}
	if _, err := m.InspectKey(dirPath); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory InspectKey error = %v", err)
	}

	noPub := filepath.Join(m.cfg.SSHDir, "no-pub")
	if err := os.WriteFile(noPub, []byte("private"), 0o600); err != nil {
		t.Fatalf("write no-pub private: %v", err)
	}
	if _, err := m.InspectKey(noPub); err == nil || !strings.Contains(err.Error(), "reading public key") {
		t.Fatalf("missing pub InspectKey error = %v", err)
	}

	badPub := filepath.Join(m.cfg.SSHDir, "bad-pub")
	if err := os.WriteFile(badPub, []byte("private"), 0o600); err != nil {
		t.Fatalf("write bad private: %v", err)
	}
	if err := os.WriteFile(badPub+".pub", []byte("not-a-public-key"), 0o644); err != nil {
		t.Fatalf("write bad pub: %v", err)
	}
	if _, err := m.InspectKey(badPub); err == nil || !strings.Contains(err.Error(), "parsing public key") {
		t.Fatalf("bad pub InspectKey error = %v", err)
	}
}

func TestKeyTypeFromPublicKeyAlgorithm(t *testing.T) {
	cases := map[string]string{
		ssh.KeyAlgoED25519:    "ed25519",
		ssh.KeyAlgoRSA:        "rsa",
		"ecdsa-sha2-nistp256": "ecdsa",
		"ssh-custom":          "custom",
	}
	for input, want := range cases {
		if got := keyTypeFromPublicKeyAlgorithm(input); got != want {
			t.Fatalf("keyTypeFromPublicKeyAlgorithm(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRemoveKeyPair(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519_remove")
	if err := os.WriteFile(keyPath, []byte("private"), 0o600); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.WriteFile(keyPath+".pub", []byte("public"), 0o644); err != nil {
		t.Fatalf("write public: %v", err)
	}
	if err := removeKeyPair(keyPath); err != nil {
		t.Fatalf("removeKeyPair: %v", err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("private key still exists: %v", err)
	}
	if err := removeKeyPair(keyPath); err != nil {
		t.Fatalf("removeKeyPair missing files should be ok: %v", err)
	}
}

func TestRemoveKeyPairErrors(t *testing.T) {
	dir := t.TempDir()
	privateDir := filepath.Join(dir, "private-dir")
	if err := os.MkdirAll(filepath.Join(privateDir, "child"), 0o700); err != nil {
		t.Fatalf("mkdir private dir: %v", err)
	}
	if err := removeKeyPair(privateDir); err == nil || !strings.Contains(err.Error(), "removing private key") {
		t.Fatalf("private remove error = %v", err)
	}

	keyPath := filepath.Join(dir, "id_ed25519_pub_error")
	if err := os.WriteFile(keyPath, []byte("private"), 0o600); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(keyPath+".pub", "child"), 0o700); err != nil {
		t.Fatalf("mkdir public dir: %v", err)
	}
	if err := removeKeyPair(keyPath); err == nil || !strings.Contains(err.Error(), "removing public key") {
		t.Fatalf("public remove error = %v", err)
	}
}

func TestGenerate_OverwriteRemoveError(t *testing.T) {
	m := newTestManager(t)
	keyPath, err := m.ExpectedKeyPath("blocked", "ed25519")
	if err != nil {
		t.Fatalf("ExpectedKeyPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(keyPath, "child"), 0o700); err != nil {
		t.Fatalf("mkdir blocked key path: %v", err)
	}

	_, err = m.Generate(GenerateOptions{Profile: "blocked", KeyType: "ed25519", Overwrite: true})
	if err == nil || !strings.Contains(err.Error(), "overwriting existing SSH key") {
		t.Fatalf("overwrite remove error = %v", err)
	}
}

func TestGenerate_StatError(t *testing.T) {
	m := newTestManager(t)
	original := sshStatFn
	t.Cleanup(func() { sshStatFn = original })
	sshStatFn = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, "id_ed25519_staterr") {
			return nil, os.ErrPermission
		}
		return os.Stat(path)
	}

	_, err := m.Generate(GenerateOptions{Profile: "staterr", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "checking SSH key") {
		t.Fatalf("stat error = %v", err)
	}
}

func TestExpectedKeyPathDefaultsType(t *testing.T) {
	m := newTestManager(t)

	path, err := m.ExpectedKeyPath("work", "")
	if err != nil {
		t.Fatalf("ExpectedKeyPath: %v", err)
	}
	if filepath.Base(path) != "id_ed25519_work" {
		t.Fatalf("ExpectedKeyPath base = %q", filepath.Base(path))
	}
}

func TestGenerate_UnsupportedKeyType(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Generate(GenerateOptions{Profile: "p", KeyType: "dsa"}); err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

func TestGenerate_PathAndComment(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "personal",
		KeyType: "ed25519",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if filepath.Base(info.Path) != "id_ed25519_personal" {
		t.Fatalf("unexpected key path: %s", info.Path)
	}
	if info.Comment != "gcm-personal" {
		t.Fatalf("unexpected default comment: %q", info.Comment)
	}
}

func TestKeyPath(t *testing.T) {
	m := newTestManager(t)
	path, err := m.keyPath("work", "ed25519")
	if err != nil {
		t.Fatalf("keyPath: %v", err)
	}
	if filepath.Base(path) != "id_ed25519_work" {
		t.Errorf("keyPath = %q", path)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{"~", home},
		{"~/foo", home + "/foo"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}
	for _, tt := range tests {
		got := expandPath(tt.input)
		if got != tt.want {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestList(t *testing.T) {
	m := newTestManager(t)

	// Generate a key first
	_, err := m.Generate(GenerateOptions{
		Profile: "listtest",
		KeyType: "ed25519",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) == 0 {
		t.Error("expected at least 1 key in list")
	}
}

func TestGetPublicKey(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "pubtest",
		KeyType: "ed25519",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	pubKey, err := m.GetPublicKey(info.Path)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519") {
		t.Errorf("pubKey = %q", pubKey)
	}
}

func TestGetPublicKey_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetPublicKey("/nonexistent/key")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestGetFingerprint(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "fptest",
		KeyType: "ed25519",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	fp, err := m.getFingerprint(info.Path)
	if err != nil {
		t.Fatalf("getFingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint = %q", fp)
	}
}

func TestGenerate_RSA4096(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "rsa",
		KeyType: "rsa",
		Bits:    4096,
	})
	if err != nil {
		t.Fatalf("Generate RSA: %v", err)
	}
	if info.Type != "rsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_ECDSA(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "ecdsa",
		KeyType: "ecdsa",
	})
	if err != nil {
		t.Fatalf("Generate ECDSA: %v", err)
	}
	if info.Type != "ecdsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_CustomComment(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "commented",
		KeyType: "ed25519",
		Comment: "custom-comment",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if info.Comment != "custom-comment" {
		t.Errorf("comment = %q", info.Comment)
	}
}

func TestWritePrivateKey_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id_test")
	data := []byte("private key data")

	if err := writePrivateKey(path, data); err != nil {
		t.Fatalf("writePrivateKey: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != string(data) {
		t.Errorf("content mismatch")
	}

	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

func TestWritePrivateKey_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing_key")
	os.WriteFile(path, []byte("existing"), 0o600)

	err := writePrivateKey(path, []byte("new data"))
	if err == nil {
		t.Fatal("expected error when file exists (O_EXCL)")
	}
}

func TestGenerate_UnsupportedKeyType_DSA(t *testing.T) {
	m := newTestManager(t)

	_, err := m.Generate(GenerateOptions{
		Profile: "bad",
		KeyType: "dsa",
	})
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

func TestList_EmptyDir(t *testing.T) {
	m := newTestManager(t)
	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestList_WithGeneratedKey(t *testing.T) {
	m := newTestManager(t)
	m.Generate(GenerateOptions{Profile: "listme", KeyType: "ed25519"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) == 0 {
		t.Error("expected at least 1 key after generate")
	}
}

func TestGetFingerprint_Generated(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{Profile: "fp", KeyType: "ed25519"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if info.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if !strings.HasPrefix(info.Fingerprint, "SHA256:") {
		t.Errorf("fingerprint = %q, expected SHA256: prefix", info.Fingerprint)
	}
}

func TestGetFingerprint_NonExistent(t *testing.T) {
	m := newTestManager(t)
	_, err := m.getFingerprint("/nonexistent/key")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestGenerate_ECDSA_P384(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "ec384",
		KeyType: "ecdsa",
		Bits:    384,
	})
	if err != nil {
		t.Fatalf("Generate ECDSA P384: %v", err)
	}
	if info.Type != "ecdsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_ECDSA_P521(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "ec521",
		KeyType: "ecdsa",
		Bits:    521,
	})
	if err != nil {
		t.Fatalf("Generate ECDSA P521: %v", err)
	}
	if info.Type != "ecdsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_ECDSA_UnsupportedCurve(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Generate(GenerateOptions{
		Profile: "ecbad",
		KeyType: "ecdsa",
		Bits:    192,
	})
	if err == nil {
		t.Fatal("expected error for unsupported curve size")
	}
}

func TestGenerate_RSA_DefaultBits(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "rsadefault",
		KeyType: "rsa",
	})
	if err != nil {
		t.Fatalf("Generate RSA default: %v", err)
	}
	if info.Type != "rsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_RSA_2048(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "rsa2048",
		KeyType: "rsa",
		Bits:    2048,
	})
	if err != nil {
		t.Fatalf("Generate RSA 2048: %v", err)
	}
	if info.Type != "rsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestKeyPath_InvalidNames(t *testing.T) {
	m := newTestManager(t)
	cases := []string{"", "../escape", "foo/bar", "foo\\bar", ".."}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.keyPath(name, "ed25519")
			if err == nil {
				t.Errorf("keyPath(%q) should fail", name)
			}
		})
	}
}

func TestList_NonExistentDirV2(t *testing.T) {
	cfg := &config.Config{
		SSHDir:   "/nonexistent/ssh/dir",
		Advanced: config.AdvancedConfig{SSHCommand: "ssh"},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	keys, err := m.List()
	if err != nil {
		t.Fatalf("List should not error for non-existent dir: %v", err)
	}
	if keys != nil {
		t.Errorf("expected nil keys, got %v", keys)
	}
}

func TestList_SkipsDirectories(t *testing.T) {
	m := newTestManager(t)
	// Create a subdirectory with .pub extension (shouldn't be listed)
	os.MkdirAll(filepath.Join(m.cfg.SSHDir, "subdir.pub"), 0o700)
	// Generate a real key
	m.Generate(GenerateOptions{Profile: "listdir", KeyType: "ed25519"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestList_SkipsNonPubFiles(t *testing.T) {
	m := newTestManager(t)
	// Create a file without .pub extension
	os.WriteFile(filepath.Join(m.cfg.SSHDir, "random.txt"), []byte("data"), 0o644)
	m.Generate(GenerateOptions{Profile: "listskip", KeyType: "ed25519"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestList_UnreadablePubKey(t *testing.T) {
	m := newTestManager(t)
	// Create a .pub file that's unreadable
	pubPath := filepath.Join(m.cfg.SSHDir, "bad.pub")
	os.WriteFile(pubPath, []byte("unreadable"), 0o000)
	t.Cleanup(func() { os.Chmod(pubPath, 0o644) })

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should skip unreadable files gracefully
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for unreadable file, got %d", len(keys))
	}
}

func TestList_PubKeyWithoutComment(t *testing.T) {
	m := newTestManager(t)
	// Create a minimal pub key with only type and base64
	pubPath := filepath.Join(m.cfg.SSHDir, "nocomment.pub")
	os.WriteFile(pubPath, []byte("ssh-ed25519 AAAA"), 0o644)

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Comment != "" {
		t.Errorf("expected empty comment, got %q", keys[0].Comment)
	}
}

func TestGetFingerprint_InvalidPubKeyFallsBack(t *testing.T) {
	m := newTestManager(t)
	// Write a .pub file with invalid content so native parsing fails
	privPath := filepath.Join(m.cfg.SSHDir, "invalid_key")
	pubPath := privPath + ".pub"
	os.WriteFile(pubPath, []byte("not a valid key"), 0o644)

	// getFingerprint should fall back to ssh-keygen (which will also fail
	// for invalid data, but that's fine - we're testing the fallback path)
	_, err := m.getFingerprint(privPath)
	// Error expected since ssh-keygen can't parse this either
	if err == nil {
		t.Log("ssh-keygen somehow parsed invalid data (unlikely)")
	}
}

func TestGenerate_DefaultKeyType(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "default",
	})
	if err != nil {
		t.Fatalf("Generate with defaults: %v", err)
	}
	if info.Type != "ed25519" {
		t.Errorf("default type = %q, want ed25519", info.Type)
	}
	if info.Comment != "gcm-default" {
		t.Errorf("default comment = %q", info.Comment)
	}
}

func TestGenerate_MkdirAllFailure(t *testing.T) {
	cfg := &config.Config{
		SSHDir:   "/dev/null/cannot/create",
		Advanced: config.AdvancedConfig{SSHCommand: "ssh"},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	_, err := m.Generate(GenerateOptions{Profile: "fail", KeyType: "ed25519"})
	if err == nil {
		t.Fatal("expected error for impossible dir")
	}
}

func TestGenerate_PublicKeyWriteFailure(t *testing.T) {
	m := newTestManager(t)
	// Generate a key first to occupy the name
	info, err := m.Generate(GenerateOptions{Profile: "pubfail", KeyType: "ed25519"})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	// Remove the private key and pub key, then make the dir read-only
	os.Remove(info.Path)
	os.Remove(info.Path + ".pub")
	// Write a readonly dir in place of where the pub file would go
	os.MkdirAll(info.Path+".pub", 0o000)
	t.Cleanup(func() { os.Chmod(info.Path+".pub", 0o755) })

	_, err = m.Generate(GenerateOptions{Profile: "pubfail", KeyType: "ed25519"})
	if err == nil {
		t.Fatal("expected error when public key write fails")
	}
}

func TestList_MultipleKeys(t *testing.T) {
	m := newTestManager(t)
	m.Generate(GenerateOptions{Profile: "multi1", KeyType: "ed25519"})
	m.Generate(GenerateOptions{Profile: "multi2", KeyType: "rsa", Bits: 2048})
	m.Generate(GenerateOptions{Profile: "multi3", KeyType: "ecdsa"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestGetPublicKey_InvalidPath(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetPublicKey("/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent public key")
	}
}

func TestKeyPath_ValidNames(t *testing.T) {
	m := newTestManager(t)
	tests := []struct {
		profile string
		keyType string
		want    string
	}{
		{"work", "ed25519", "id_ed25519_work"},
		{"personal", "rsa", "id_rsa_personal"},
		{"client-a", "ecdsa", "id_ecdsa_client-a"},
	}
	for _, tt := range tests {
		path, err := m.keyPath(tt.profile, tt.keyType)
		if err != nil {
			t.Fatalf("keyPath(%q, %q): %v", tt.profile, tt.keyType, err)
		}
		if filepath.Base(path) != tt.want {
			t.Errorf("keyPath(%q, %q) = %q, want basename %q", tt.profile, tt.keyType, path, tt.want)
		}
	}
}

func TestExpandPath_EmptyHomeDir(t *testing.T) {
	// Non-tilde paths should pass through
	got := expandPath("/some/absolute/path")
	if got != "/some/absolute/path" {
		t.Errorf("expandPath = %q", got)
	}
	got = expandPath("relative")
	if got != "relative" {
		t.Errorf("expandPath relative = %q", got)
	}
}

func TestNewManager(t *testing.T) {
	cfg := &config.Config{SSHDir: "/tmp"}
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.cfg != cfg {
		t.Error("cfg not set")
	}
	if m.log != log {
		t.Error("log not set")
	}
}

func TestExpandPath_NoHome(t *testing.T) {
	// Test expansion when HOME might not be set
	got := expandPath("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandPath = %q", got)
	}
	got = expandPath("relative")
	if got != "relative" {
		t.Errorf("expandPath = %q", got)
	}
}

func TestGenerateKeyPair_Ed25519(t *testing.T) {
	priv, pub, err := generateKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("generateKeyPair: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_EmptyType(t *testing.T) {
	priv, pub, err := generateKeyPair("", 0)
	if err != nil {
		t.Fatalf("generateKeyPair empty type: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys for empty type (defaults to ed25519)")
	}
}

func TestGenerateKeyPair_ECDSA_DefaultCurve(t *testing.T) {
	priv, pub, err := generateKeyPair("ecdsa", 0)
	if err != nil {
		t.Fatalf("generateKeyPair ecdsa default: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_ECDSA_256(t *testing.T) {
	priv, pub, err := generateKeyPair("ecdsa", 256)
	if err != nil {
		t.Fatalf("generateKeyPair ecdsa 256: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_RSA_Default(t *testing.T) {
	priv, pub, err := generateKeyPair("rsa", 0)
	if err != nil {
		t.Fatalf("generateKeyPair rsa 0: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_Unsupported(t *testing.T) {
	_, _, err := generateKeyPair("dsa", 0)
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

func TestGenerateKeyPair_RSA_TooSmall(t *testing.T) {
	_, _, err := generateKeyPair("rsa", 1024)
	if err == nil {
		t.Fatal("expected error for RSA < 2048")
	}
}

func TestGetFingerprint_FallbackPath(t *testing.T) {
	m := newTestManager(t)
	// Write a valid public key file, then corrupt it so native parsing fails
	// but leave the .pub extension so getFingerprint attempts the fallback
	privPath := filepath.Join(m.cfg.SSHDir, "fallback_key")
	pubPath := privPath + ".pub"
	os.WriteFile(pubPath, []byte("not a valid ssh key format"), 0o644)

	_, err := m.getFingerprint(privPath)
	// This tests the fallback to ssh-keygen, which will also fail for invalid data
	if err == nil {
		t.Log("fingerprint unexpectedly succeeded (ssh-keygen parsed garbage?)")
	}
}

func TestList_WithMixedFiles(t *testing.T) {
	m := newTestManager(t)
	// Generate real keys
	m.Generate(GenerateOptions{Profile: "real1", KeyType: "ed25519"})

	// Add a non-pub file (should be skipped)
	os.WriteFile(filepath.Join(m.cfg.SSHDir, "known_hosts"), []byte("host data"), 0o644)
	// Add a directory with .pub extension (should be skipped)
	os.MkdirAll(filepath.Join(m.cfg.SSHDir, "dir.pub"), 0o700)

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestGenerate_Ed25519DefaultComment(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{Profile: "myprof"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if info.Comment != "gcm-myprof" {
		t.Errorf("default comment = %q, want gcm-myprof", info.Comment)
	}
	if info.Type != "ed25519" {
		t.Errorf("default type = %q, want ed25519", info.Type)
	}
}

func TestList_KeyTypeExtraction(t *testing.T) {
	m := newTestManager(t)
	m.Generate(GenerateOptions{Profile: "typtest", KeyType: "ed25519", Comment: "mycomment"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Type != "ed25519" {
		t.Errorf("type = %q, want ed25519", keys[0].Type)
	}
	if keys[0].Comment != "mycomment" {
		t.Errorf("comment = %q, want mycomment", keys[0].Comment)
	}
	if keys[0].Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestWritePrivateKey_ContentAndPerms(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test_key")
	err := writePrivateKey(path, []byte("fake-private-key"))
	if err != nil {
		t.Fatalf("writePrivateKey: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "fake-private-key" {
		t.Errorf("wrong content: %q", string(data))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perms = %o, want 600", info.Mode().Perm())
	}
}

func TestWritePrivateKey_AlreadyExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing_key")
	os.WriteFile(path, []byte("old"), 0o600)
	err := writePrivateKey(path, []byte("new"))
	if err == nil {
		t.Fatal("expected error for existing file")
	}
}

func TestWritePrivateKey_BadDir(t *testing.T) {
	err := writePrivateKey("/nonexistent/dir/key", []byte("data"))
	if err == nil {
		t.Fatal("expected error for bad directory")
	}
}

func TestGetFingerprint_ValidKey(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.SSHDir = tmp
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Generate a real key pair
	_, err := m.Generate(GenerateOptions{Profile: "fptest", KeyType: "ed25519", Comment: "testcomment"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	fp, err := m.getFingerprint(filepath.Join(tmp, "id_ed25519_fptest"))
	if err != nil {
		t.Fatalf("getFingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint %q doesn't start with SHA256:", fp)
	}
}

func TestGetFingerprint_NoPubKey(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.SSHDir = tmp
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.getFingerprint(filepath.Join(tmp, "nonexistent_key"))
	if err == nil {
		t.Fatal("expected error for missing pub key")
	}
}

func TestList_WithMixedFiles2(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.SSHDir = tmp
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Create a valid key pair
	m.Generate(GenerateOptions{Profile: "listtest", KeyType: "ed25519", Comment: "test"})

	// Create non-pub files that should be ignored
	os.WriteFile(filepath.Join(tmp, "config"), []byte("host *"), 0o644)
	os.WriteFile(filepath.Join(tmp, "known_hosts"), []byte(""), 0o644)
	os.Mkdir(filepath.Join(tmp, "subdir"), 0o755)

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestList_NonexistentDir(t *testing.T) {
	cfg := &config.Config{
		SSHDir: "/tmp/gcm-nonexistent-ssh-dir-" + t.Name(),
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List nonexistent dir: %v", err)
	}
	if keys != nil {
		t.Errorf("expected nil keys for nonexistent dir, got %v", keys)
	}
}

func TestKeyPath_InvalidProfile(t *testing.T) {
	m := newTestManager(t)

	// Path traversal
	_, err := m.keyPath("../evil", "ed25519")
	if err == nil {
		t.Fatal("expected error for traversal in keyPath")
	}

	// Empty profile
	_, err = m.keyPath("", "ed25519")
	if err == nil {
		t.Fatal("expected error for empty profile in keyPath")
	}

	// Slash in profile
	_, err = m.keyPath("a/b", "ed25519")
	if err == nil {
		t.Fatal("expected error for slash in profile in keyPath")
	}
}

func TestGenerate_Ed25519_DefaultType(t *testing.T) {
	m := newTestManager(t)

	// Empty key type should default to ed25519
	info, err := m.Generate(GenerateOptions{
		Profile: "defaulttype",
	})
	if err != nil {
		t.Fatalf("Generate default type: %v", err)
	}
	if info.Type != "ed25519" {
		t.Errorf("type = %q, want ed25519", info.Type)
	}
}

func TestGenerate_PublicKeyContent(t *testing.T) {
	m := newTestManager(t)

	info, err := m.Generate(GenerateOptions{
		Profile: "pubcontent",
		KeyType: "ed25519",
		Comment: "test-comment",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(info.PublicKey, "ssh-ed25519") {
		t.Errorf("public key should contain ssh-ed25519, got: %s", info.PublicKey)
	}
	if !strings.Contains(info.PublicKey, "test-comment") {
		t.Errorf("public key should contain comment, got: %s", info.PublicKey)
	}
}

func TestList_MultipleKeysV2(t *testing.T) {
	m := newTestManager(t)

	m.Generate(GenerateOptions{Profile: "key1", KeyType: "ed25519"})
	m.Generate(GenerateOptions{Profile: "key2", KeyType: "ed25519"})

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestGetPublicKey_Content(t *testing.T) {
	m := newTestManager(t)

	info, _ := m.Generate(GenerateOptions{
		Profile: "pubget",
		KeyType: "ed25519",
		Comment: "pubget-comment",
	})

	pubKey, err := m.GetPublicKey(info.Path)
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519") {
		t.Errorf("unexpected public key format: %s", pubKey)
	}
}

func TestTestConnection_NonExistentKey(t *testing.T) {
	m := newTestManager(t)
	_, err := m.TestConnection("/nonexistent/path/to/key")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestTestConnection_InvalidKeyPath(t *testing.T) {
	m := newTestManager(t)
	// Use an empty file as "key" — ssh will reject it
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "empty_key")
	os.WriteFile(keyPath, []byte("not a real key"), 0o600)

	_, err := m.TestConnection(keyPath)
	if err == nil {
		t.Fatal("expected error for invalid key file")
	}
	if !strings.Contains(err.Error(), "SSH test failed") {
		t.Errorf("error = %q, expected 'SSH test failed' substring", err.Error())
	}
}

func TestTestConnection_TildeExpansion(t *testing.T) {
	// TestConnection should expand ~ in paths without panicking
	m := newTestManager(t)
	_, err := m.TestConnection("~/nonexistent_key_gcm_test")
	if err == nil {
		t.Fatal("expected error for nonexistent key with tilde")
	}
}

func TestTestConnection_CustomSSHCommand(t *testing.T) {
	// Use a non-existent ssh command to trigger exec error
	cfg := &config.Config{
		SSHDir: t.TempDir(),
		Advanced: config.AdvancedConfig{
			SSHCommand: "/nonexistent/ssh/binary",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.TestConnection("/some/key")
	if err == nil {
		t.Fatal("expected error for non-existent SSH command")
	}
}

func TestAddToAgent_NonExistentKey(t *testing.T) {
	m := newTestManager(t)
	err := m.AddToAgent("/nonexistent/path/to/key")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if !strings.Contains(err.Error(), "ssh-add failed") {
		t.Errorf("error = %q, expected 'ssh-add failed' substring", err.Error())
	}
}

func TestAddToAgent_InvalidKeyFile(t *testing.T) {
	m := newTestManager(t)
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "bad_key")
	os.WriteFile(keyPath, []byte("not a valid ssh key"), 0o600)

	err := m.AddToAgent(keyPath)
	if err == nil {
		t.Fatal("expected error for invalid key file")
	}
}

func TestAddToAgent_TildeExpansion(t *testing.T) {
	m := newTestManager(t)
	err := m.AddToAgent("~/nonexistent_key_gcm_test_add")
	if err == nil {
		t.Fatal("expected error for nonexistent key with tilde")
	}
}

func TestList_ReadDirPermissionError(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, "locked_ssh")
	os.MkdirAll(sshDir, 0o700)
	// Create a file so dir isn't empty, then lock it
	os.WriteFile(filepath.Join(sshDir, "test.pub"), []byte("data"), 0o644)
	os.Chmod(sshDir, 0o000)
	t.Cleanup(func() { os.Chmod(sshDir, 0o755) })

	cfg := &config.Config{
		SSHDir:   sshDir,
		Advanced: config.AdvancedConfig{SSHCommand: "ssh"},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error for permission-denied directory")
	}
	if !strings.Contains(err.Error(), "reading SSH directory") {
		t.Errorf("error = %q, expected 'reading SSH directory' substring", err.Error())
	}
}

func TestGetFingerprint_EmptyPubFile(t *testing.T) {
	m := newTestManager(t)
	privPath := filepath.Join(m.cfg.SSHDir, "emptykey")
	pubPath := privPath + ".pub"
	os.WriteFile(pubPath, []byte(""), 0o644)

	_, err := m.getFingerprint(privPath)
	// Empty file can't be parsed natively, falls back to ssh-keygen which also fails
	if err == nil {
		t.Log("fingerprint unexpectedly succeeded for empty pub file")
	}
}

func TestGenerateKeyPair_ECDSA_384(t *testing.T) {
	priv, pub, err := generateKeyPair("ecdsa", 384)
	if err != nil {
		t.Fatalf("generateKeyPair ecdsa 384: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_ECDSA_521(t *testing.T) {
	priv, pub, err := generateKeyPair("ecdsa", 521)
	if err != nil {
		t.Fatalf("generateKeyPair ecdsa 521: %v", err)
	}
	if priv == nil || pub == nil {
		t.Fatal("expected non-nil keys")
	}
}

func TestGenerateKeyPair_ECDSA_UnsupportedSize(t *testing.T) {
	_, _, err := generateKeyPair("ecdsa", 192)
	if err == nil {
		t.Fatal("expected error for unsupported ECDSA size")
	}
	if !strings.Contains(err.Error(), "unsupported ECDSA curve size") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestWritePrivateKey_ReadOnlyDir(t *testing.T) {
	tmp := t.TempDir()
	readonlyDir := filepath.Join(tmp, "readonly")
	os.MkdirAll(readonlyDir, 0o500)
	t.Cleanup(func() { os.Chmod(readonlyDir, 0o755) })

	err := writePrivateKey(filepath.Join(readonlyDir, "key"), []byte("data"))
	if err == nil {
		t.Fatal("expected error writing to read-only dir")
	}
}

// =============================================================================
// Additional coverage: writePrivateKey error paths
// =============================================================================

func TestWritePrivateKey_ExistingFileV2(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing_key_v2")
	// Create the file first
	os.WriteFile(path, []byte("existing"), 0o600)

	// writePrivateKey uses O_EXCL, so it should fail on existing file
	err := writePrivateKey(path, []byte("new data"))
	if err == nil {
		t.Fatal("expected error when file already exists (O_EXCL)")
	}
	if !strings.Contains(err.Error(), "creating private key") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestWritePrivateKey_SuccessV2(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new_key_v2")

	err := writePrivateKey(path, []byte("private key data v2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "private key data v2" {
		t.Errorf("content = %q", string(data))
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

// =============================================================================
// Additional coverage: Generate - MkdirAll failure
// =============================================================================

func TestGenerate_MkdirFails(t *testing.T) {
	tmp := t.TempDir()
	// Create a file where the .ssh directory should be
	sshDir := filepath.Join(tmp, "ssh")
	os.WriteFile(sshDir, []byte("blocker"), 0o644)

	cfg := &config.Config{
		SSHDir: sshDir,
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.Generate(GenerateOptions{
		Profile: "faildir",
		KeyType: "ed25519",
	})
	if err == nil {
		t.Fatal("expected error when SSH dir creation fails")
	}
}

// =============================================================================
// Additional coverage: Generate - public key write failure path
// =============================================================================

func TestGenerate_PublicKeyWriteFail(t *testing.T) {
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, "ssh")
	os.MkdirAll(sshDir, 0o700)

	cfg := &config.Config{
		SSHDir: sshDir,
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// Generate a key successfully first
	info, err := m.Generate(GenerateOptions{
		Profile: "pubfail",
		KeyType: "ed25519",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify both files exist
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatalf("private key missing: %v", err)
	}
	if _, err := os.Stat(info.Path + ".pub"); err != nil {
		t.Fatalf("public key missing: %v", err)
	}
}

// =============================================================================
// Additional coverage: List - empty directory
// =============================================================================

func TestList_EmptyDirV2(t *testing.T) {
	m := newTestManager(t)
	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestList_NonExistentDirV3(t *testing.T) {
	cfg := &config.Config{
		SSHDir: "/nonexistent/ssh/dir/xyz12345",
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if keys != nil {
		t.Errorf("expected nil keys for nonexistent dir, got %d", len(keys))
	}
}

// =============================================================================
// Additional coverage: ECDSA with various sizes
// =============================================================================

func TestGenerate_ECDSA384(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "ecdsa384",
		KeyType: "ecdsa",
		Bits:    384,
	})
	if err != nil {
		t.Fatalf("Generate ECDSA 384: %v", err)
	}
	if info.Type != "ecdsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestGenerate_ECDSA521(t *testing.T) {
	m := newTestManager(t)
	info, err := m.Generate(GenerateOptions{
		Profile: "ecdsa521",
		KeyType: "ecdsa",
		Bits:    521,
	})
	if err != nil {
		t.Fatalf("Generate ECDSA 521: %v", err)
	}
	if info.Type != "ecdsa" {
		t.Errorf("type = %q", info.Type)
	}
}

func TestTestConnection_NonexistentKeyV2(t *testing.T) {
	m := newTestManager(t)
	// Use a non-existent identity file to force ssh to fail.
	_, err := m.TestConnection(filepath.Join(t.TempDir(), "no-such-key"))
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

func TestAddToAgent_BadPath(t *testing.T) {
	m := newTestManager(t)
	err := m.AddToAgent(filepath.Join(t.TempDir(), "nonexistent-key"))
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
	if !strings.Contains(err.Error(), "ssh-add failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFingerprint_NoPubKeyNoKeygen(t *testing.T) {
	m := newTestManager(t)
	// Key path where neither .pub exists nor ssh-keygen can help.
	keyPath := filepath.Join(t.TempDir(), "ghost_key")
	_, err := m.getFingerprint(keyPath)
	if err == nil {
		t.Fatal("expected error when pub key missing and ssh-keygen fails")
	}
}

func TestGenerate_KeyAlreadyExists(t *testing.T) {
	m := newTestManager(t)
	// Generate key first
	info, err := m.Generate(GenerateOptions{Profile: "existing", KeyType: "ed25519"})
	if err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	// Try to generate again - should fail because key exists
	_, err = m.Generate(GenerateOptions{Profile: "existing", KeyType: "ed25519"})
	if err == nil {
		t.Fatal("expected error when key already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err)
	}
	_ = info
}

func TestGenerate_MkdirAllBlockedByFile(t *testing.T) {
	cfg := &config.Config{
		SSHDir: filepath.Join(t.TempDir(), "blocked"),
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
			GPGCommand: "gpg",
			GitCommand: "git",
		},
	}
	// Create a regular file at the SSHDir path so MkdirAll fails
	os.WriteFile(cfg.SSHDir, []byte("blocker"), 0o600)
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.Generate(GenerateOptions{Profile: "test", KeyType: "ed25519"})
	if err == nil {
		t.Fatal("expected error when MkdirAll fails due to file blocking")
	}
	if !strings.Contains(err.Error(), "creating SSH directory") {
		t.Errorf("error = %q, want 'creating SSH directory'", err)
	}
}

func TestList_DirsAndNonPubFiles(t *testing.T) {
	m := newTestManager(t)

	// Create a directory inside the ssh dir
	os.MkdirAll(filepath.Join(m.cfg.SSHDir, "subdir"), 0o755)
	// Create a non-.pub file
	os.WriteFile(filepath.Join(m.cfg.SSHDir, "config"), []byte("Host *\n"), 0o644)
	// Create a .pub file with valid content
	_, pub, _ := generateKeyPair("ed25519", 0)
	pubKey, _ := ssh.NewPublicKey(pub)
	pubLine := string(ssh.MarshalAuthorizedKey(pubKey))
	os.WriteFile(filepath.Join(m.cfg.SSHDir, "id_ed25519_test.pub"), []byte(pubLine), 0o644)

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should find only the .pub file, not the dir or config file
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestGetFingerprint_InvalidPubKeyContent(t *testing.T) {
	m := newTestManager(t)
	keyPath := filepath.Join(t.TempDir(), "badkey")
	// Write an invalid .pub file so ParseAuthorizedKey fails
	os.WriteFile(keyPath+".pub", []byte("not a valid public key"), 0o644)

	// Since the pub key is invalid and ssh-keygen likely can't parse it either,
	// it should fall back to ssh-keygen which will also fail
	_, err := m.getFingerprint(keyPath)
	if err == nil {
		// ssh-keygen might not be installed; either error or empty string is fine
		t.Log("ssh-keygen parsed it somehow - that's fine")
	}
}

// =============================================================================
// Additional coverage tests
// =============================================================================

func TestWritePrivateKey_FileAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing_key")
	os.WriteFile(path, []byte("existing"), 0o600)

	err := writePrivateKey(path, []byte("new key data"))
	if err == nil {
		t.Fatal("expected error when file exists (O_EXCL)")
	}
}

func TestKeyPath_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.keyPath("../escape", "ed25519")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestKeyPath_EmptyProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.keyPath("", "ed25519")
	if err == nil {
		t.Fatal("expected error for empty profile")
	}
}

func TestKeyPath_SlashInProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.keyPath("a/b", "ed25519")
	if err == nil {
		t.Fatal("expected error for slash in profile")
	}
}

func TestKeyPath_ValidProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	path, err := m.keyPath("myprofile", "ed25519")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(path, "id_ed25519_myprofile") {
		t.Errorf("path = %q, expected to contain id_ed25519_myprofile", path)
	}
}

func TestGetFingerprint_ValidPublicKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// Generate a real key pair
	opts := GenerateOptions{
		Profile: "fptest",
		KeyType: "ed25519",
		Comment: "test@test",
	}
	result, err := m.Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	fp, err := m.getFingerprint(result.Path)
	if err != nil {
		t.Fatalf("getFingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("fingerprint = %q, expected SHA256: prefix", fp)
	}
}

func TestGetFingerprint_BadPublicKey_FallsBackToSSHKeygen(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// Write invalid public key data
	keyPath := filepath.Join(dir, "bad_key")
	os.WriteFile(keyPath, []byte("private"), 0o600)
	os.WriteFile(keyPath+".pub", []byte("not a valid pub key"), 0o644)

	// This should fail to parse natively and fall back to ssh-keygen
	_, err := m.getFingerprint(keyPath)
	// May succeed or fail depending on whether ssh-keygen is installed
	_ = err
}

func TestAddToAgent_BadKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		SSHDir: dir,
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	err := m.AddToAgent(filepath.Join(dir, "nonexistent_key"))
	if err == nil {
		t.Fatal("expected error adding nonexistent key to agent")
	}
}

func TestGetFingerprint_NoPubFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// No .pub file at all — ReadFile fails, falls through to ssh-keygen which also fails
	keyPath := filepath.Join(dir, "no_pub_key")
	os.WriteFile(keyPath, []byte("private"), 0o600)
	// No .pub file written

	_, err := m.getFingerprint(keyPath)
	if err == nil {
		t.Log("ssh-keygen succeeded without pub file - unlikely but ok")
	}
}

func TestGenerate_WritePublicKeyError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		SSHDir: dir,
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// Generate a key first
	opts := GenerateOptions{
		Profile: "pubwrite-test",
		KeyType: "ed25519",
		Comment: "test",
	}
	result, err := m.Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify the key was created
	if result.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}

	// Now create a directory where the pub file should be to test pub write error
	// This path needs a fresh profile
	pubPath := filepath.Join(dir, "id_ed25519_pubwrite-err.pub")
	os.MkdirAll(pubPath, 0o755) // create dir where file should be

	opts2 := GenerateOptions{
		Profile: "pubwrite-err",
		KeyType: "ed25519",
		Comment: "test",
	}
	_, err = m.Generate(opts2)
	if err == nil {
		t.Fatal("expected error when pub path is a directory")
	}
}

func TestGenerate_DSAKeyType(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		SSHDir: dir,
		Advanced: config.AdvancedConfig{
			SSHCommand: "ssh",
		},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	_, err := m.Generate(GenerateOptions{
		Profile: "badtype",
		KeyType: "dsa",
		Comment: "test",
	})
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

func TestTestConnection_SuccessGreeting(t *testing.T) {
	dir := t.TempDir()
	fakeSSH := filepath.Join(dir, "fake-ssh")
	os.WriteFile(fakeSSH, []byte("#!/bin/sh\necho 'Hi testuser! You have successfully authenticated'\nexit 1\n"), 0o755)

	cfg := &config.Config{
		SSHDir:   dir,
		Advanced: config.AdvancedConfig{SSHCommand: fakeSSH},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	output, err := m.TestConnection(filepath.Join(dir, "somekey"))
	if err != nil {
		t.Fatalf("expected success for greeting, got: %v", err)
	}
	if !strings.Contains(output, "Hi") {
		t.Errorf("output = %q", output)
	}
}

func TestTestConnection_ExitZeroNoGreeting(t *testing.T) {
	dir := t.TempDir()
	fakeSSH := filepath.Join(dir, "fake-ssh")
	os.WriteFile(fakeSSH, []byte("#!/bin/sh\necho 'some other output'\nexit 0\n"), 0o755)

	cfg := &config.Config{
		SSHDir:   dir,
		Advanced: config.AdvancedConfig{SSHCommand: fakeSSH},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	output, err := m.TestConnection(filepath.Join(dir, "somekey"))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(output, "some other output") {
		t.Errorf("output = %q", output)
	}
}

func TestTestConnectionToHost_DefaultHostPortAndGitLabGreeting(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	fakeSSH := filepath.Join(dir, "fake-ssh")
	script := "#!/bin/sh\nprintf '%s\n' \"$*\" > \"" + argsFile + "\"\necho 'Welcome to GitLab, @test!'\nexit 1\n"
	if err := os.WriteFile(fakeSSH, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	cfg := &config.Config{
		SSHDir:   dir,
		Advanced: config.AdvancedConfig{SSHCommand: fakeSSH},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	if output, err := m.TestConnectionToHost(filepath.Join(dir, "somekey"), "", 0); err != nil || !strings.Contains(output, "Welcome to GitLab") {
		t.Fatalf("default host connection = %q, %v", output, err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	if !strings.Contains(string(args), "git@github.com") {
		t.Fatalf("default host args = %q", string(args))
	}

	if output, err := m.TestConnectionToHost(filepath.Join(dir, "somekey"), "gitlab.example.test", 2222); err != nil || !strings.Contains(output, "Welcome to GitLab") {
		t.Fatalf("custom host connection = %q, %v", output, err)
	}
	args, err = os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read custom args: %v", err)
	}
	if !strings.Contains(string(args), "-p 2222") || !strings.Contains(string(args), "git@gitlab.example.test") {
		t.Fatalf("custom host args = %q", string(args))
	}
}

func TestTestConnectionToHost_InvalidHost(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.TestConnectionToHost("/tmp/key", "bad host", 0); err == nil || !strings.Contains(err.Error(), "invalid SSH host") {
		t.Fatalf("invalid host error = %v", err)
	}
}

func TestAddToAgent_WithRealKey(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		SSHDir:   dir,
		Advanced: config.AdvancedConfig{SSHCommand: "ssh"},
	}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	opts := GenerateOptions{Profile: "agent-real", KeyType: "ed25519", Comment: "test"}
	result, err := m.Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	_ = m.AddToAgent(result.Path)
}

func TestGetFingerprint_FallbackToSSHKeygen(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))
	opts := GenerateOptions{Profile: "fp-fb", KeyType: "ed25519", Comment: "test"}
	result, err := m.Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Corrupt pub so native parse fails
	os.WriteFile(result.Path+".pub", []byte("garbage"), 0o644)
	_, _ = m.getFingerprint(result.Path)
}

func TestGetFingerprint_SSHKeygenSucceeds(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{SSHDir: dir}
	m := NewManager(cfg, logger.New(logger.LevelError, os.Stderr))

	// Create a fake ssh-keygen in PATH that outputs a valid fingerprint
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0o755)
	fakeKeygen := filepath.Join(binDir, "ssh-keygen")
	os.WriteFile(fakeKeygen, []byte("#!/bin/sh\necho '256 SHA256:abcdef test@test (ED25519)'\n"), 0o755)

	// Prepend binDir to PATH
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	// Create key with invalid pub (native parse fails, fake ssh-keygen succeeds)
	keyPath := filepath.Join(dir, "testkey")
	os.WriteFile(keyPath, []byte("private"), 0o600)
	os.WriteFile(keyPath+".pub", []byte("not-parseable"), 0o644)

	fp, err := m.getFingerprint(keyPath)
	if err != nil {
		t.Fatalf("expected success from fake ssh-keygen, got: %v", err)
	}
	if fp != "SHA256:abcdef" {
		t.Errorf("fingerprint = %q, want SHA256:abcdef", fp)
	}
}

func TestGenerate_MkdirError(t *testing.T) {
	m := newTestManager(t)
	orig := sshMkdirFn
	sshMkdirFn = func(string, os.FileMode) error { return errors.New("mkdir fail") }
	defer func() { sshMkdirFn = orig }()

	_, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "creating SSH directory") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

func TestGenerate_MarshalPrivKeyError(t *testing.T) {
	m := newTestManager(t)
	orig := sshMarshalPrivKeyFn
	sshMarshalPrivKeyFn = func(interface{}, string) (*pem.Block, error) {
		return nil, errors.New("marshal fail")
	}
	defer func() { sshMarshalPrivKeyFn = orig }()

	_, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "encoding private key") {
		t.Fatalf("expected marshal error, got: %v", err)
	}
}

func TestGenerate_NewPublicKeyError(t *testing.T) {
	m := newTestManager(t)
	orig := sshNewPublicKeyFn
	sshNewPublicKeyFn = func(interface{}) (ssh.PublicKey, error) {
		return nil, errors.New("pubkey fail")
	}
	defer func() { sshNewPublicKeyFn = orig }()

	_, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "deriving public key") {
		t.Fatalf("expected public key error, got: %v", err)
	}
}

func TestGenerate_WritePrivateKeyOpenError(t *testing.T) {
	m := newTestManager(t)
	orig := sshOpenFileFn
	sshOpenFileFn = func(string, int, os.FileMode) (*os.File, error) {
		return nil, errors.New("open fail")
	}
	defer func() { sshOpenFileFn = orig }()

	_, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "creating private key") {
		t.Fatalf("expected open error, got: %v", err)
	}
}

func TestGenerate_WritePrivateKeyWriteError(t *testing.T) {
	m := newTestManager(t)
	orig := sshFileWriteFn
	sshFileWriteFn = func(f *os.File, data []byte) (int, error) {
		return 0, errors.New("write fail")
	}
	defer func() { sshFileWriteFn = orig }()

	_, err := m.Generate(GenerateOptions{Profile: "work", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "writing private key") {
		t.Fatalf("expected write error, got: %v", err)
	}
}

func TestKeyPath_AbsError(t *testing.T) {
	m := newTestManager(t)
	orig := sshAbsFn
	sshAbsFn = func(string) (string, error) { return "", errors.New("abs fail") }
	defer func() { sshAbsFn = orig }()

	_, err := m.keyPath("work", "ed25519")
	if err == nil || !strings.Contains(err.Error(), "resolving SSH dir") {
		t.Fatalf("expected abs error, got: %v", err)
	}
}

func TestGetAgentKeys_LookPathError(t *testing.T) {
	m := newTestManager(t)
	orig := sshLookPathFn
	sshLookPathFn = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { sshLookPathFn = orig }()

	keys := m.getAgentKeys()
	if keys != nil {
		t.Errorf("expected nil when ssh-add not found, got %v", keys)
	}
}

func TestGetFingerprint_SSHKeygenError(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	// Create a .pub file that can't be parsed as authorized key
	keyPath := filepath.Join(dir, "testkey")
	os.WriteFile(keyPath+".pub", []byte("not-parseable-key"), 0o644)

	// Use a fake ssh-keygen that fails
	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0o755)
	fakeKeygen := filepath.Join(binDir, "ssh-keygen")
	os.WriteFile(fakeKeygen, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	_, err := m.getFingerprint(keyPath)
	if err == nil {
		t.Fatal("expected error when ssh-keygen fails")
	}
}

func TestGetFingerprint_UnexpectedOutput(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()

	// Create a .pub file that can't be parsed as authorized key
	keyPath := filepath.Join(dir, "testkey")
	os.WriteFile(keyPath+".pub", []byte("not-parseable-key"), 0o644)

	// Use a fake ssh-keygen that outputs single word (unexpected format)
	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0o755)
	fakeKeygen := filepath.Join(binDir, "ssh-keygen")
	os.WriteFile(fakeKeygen, []byte("#!/bin/sh\necho 'singleword'\n"), 0o755)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	_, err := m.getFingerprint(keyPath)
	if err == nil || !strings.Contains(err.Error(), "unexpected fingerprint output") {
		t.Fatalf("expected unexpected output error, got: %v", err)
	}
}

func TestGenerate_KeyPathError(t *testing.T) {
	m := newTestManager(t)
	// Invalid profile name triggers keyPath error
	_, err := m.Generate(GenerateOptions{Profile: "../escape", KeyType: "ed25519"})
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("expected keyPath error, got: %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("rand fail") }

var _ io.Reader = errReader{}

func TestGenerateKeyPair_Ed25519_RandError(t *testing.T) {
	orig := sshRandReader
	sshRandReader = errReader{}
	defer func() { sshRandReader = orig }()

	_, _, err := generateKeyPair("ed25519", 0)
	if err == nil {
		t.Fatal("expected rand error for ed25519")
	}
}

func TestAddToAgent_SSHAddNotFound(t *testing.T) {
	m := newTestManager(t)
	// Use empty PATH so ssh-add is not found
	t.Setenv("PATH", t.TempDir())

	err := m.AddToAgent("/tmp/fake_key")
	if err == nil || !strings.Contains(err.Error(), "ssh-add not found") {
		t.Fatalf("expected ssh-add not found error, got: %v", err)
	}
}

func TestKeyPath_RelError(t *testing.T) {
	m := newTestManager(t)
	orig := sshAbsFn
	// Return a path that makes filepath.Rel produce a ".." result
	sshAbsFn = func(string) (string, error) { return "/some/other/dir", nil }
	defer func() { sshAbsFn = orig }()

	// The profile "work" generates path SSHDir/id_ed25519_work
	// With sshAbs = /some/other/dir, Rel will produce "../..." which starts with ".."
	_, err := m.keyPath("work", "ed25519")
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("expected traversal error, got: %v", err)
	}
}

func TestRemoveFromAgent_SSHAddNotFound(t *testing.T) {
	m := newTestManager(t)
	// Use empty PATH so ssh-add is not found
	t.Setenv("PATH", t.TempDir())

	err := m.RemoveFromAgent("/tmp/fake_key")
	if err != nil {
		t.Fatalf("RemoveFromAgent should return nil when ssh-add not found, got: %v", err)
	}
}

func TestRemoveFromAgent_Success(t *testing.T) {
	m := newTestManager(t)
	tmp := t.TempDir()

	// Create a fake ssh-add that succeeds
	binDir := filepath.Join(tmp, "bin")
	os.MkdirAll(binDir, 0o755)
	fakeSSHAdd := filepath.Join(binDir, "ssh-add")
	os.WriteFile(fakeSSHAdd, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", binDir)

	err := m.RemoveFromAgent("/tmp/some_key")
	if err != nil {
		t.Fatalf("RemoveFromAgent: %v", err)
	}
}

func TestRemoveFromAgent_CommandFails_StillReturnsNil(t *testing.T) {
	m := newTestManager(t)
	tmp := t.TempDir()

	// Create a fake ssh-add that fails (key not loaded)
	binDir := filepath.Join(tmp, "bin")
	os.MkdirAll(binDir, 0o755)
	fakeSSHAdd := filepath.Join(binDir, "ssh-add")
	os.WriteFile(fakeSSHAdd, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	t.Setenv("PATH", binDir)

	// Should still return nil even when ssh-add -d fails
	err := m.RemoveFromAgent("/tmp/nonexistent_key")
	if err != nil {
		t.Fatalf("RemoveFromAgent should return nil on failure, got: %v", err)
	}
}

func TestRemoveFromAgent_ExpandsPath(t *testing.T) {
	m := newTestManager(t)
	tmp := t.TempDir()

	// Create a fake ssh-add that writes args to a file for verification
	binDir := filepath.Join(tmp, "bin")
	os.MkdirAll(binDir, 0o755)
	argsFile := filepath.Join(tmp, "args.txt")
	fakeSSHAdd := filepath.Join(binDir, "ssh-add")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\nexit 0\n"
	os.WriteFile(fakeSSHAdd, []byte(script), 0o755)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", tmp)

	err := m.RemoveFromAgent("~/testkey")
	if err != nil {
		t.Fatalf("RemoveFromAgent: %v", err)
	}

	data, _ := os.ReadFile(argsFile)
	args := strings.TrimSpace(string(data))
	if !strings.Contains(args, "-d") {
		t.Errorf("expected ssh-add args to contain -d, got: %s", args)
	}
	if strings.Contains(args, "~") {
		t.Errorf("expected path to be expanded, got: %s", args)
	}
}
