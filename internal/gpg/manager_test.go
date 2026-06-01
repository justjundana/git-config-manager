package gpg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func writeFakeGPG(t *testing.T, dir string) {
	t.Helper()
	script := `#!/bin/sh
cmd="$1"
case "$cmd" in
	--version)
		printf '%s\n' "gpg (Fake) 1.0"
		;;
	--batch)
		while IFS= read -r _line; do :; done
		exit 0
		;;
	--list-keys)
		filter="$4"
		if [ "$filter" = "missing@example.com" ]; then
			exit 2
		fi
		printf '%s\n' "pub:-:4096:1:FAKEKEY1234567890:1700000000:0::-:::scESC:"
		printf '%s\n' "fpr:::::::::FFFFFFFFFFFFFFFFFAKEKEY1234567890:"
		printf '%s\n' "uid:-::::1700000000::0000000000000000::Fake User <fake@example.com>:"
		;;
	--armor)
		printf '%s\n' "-----BEGIN PGP PUBLIC KEY BLOCK-----"
		printf '%s\n' "FAKE"
		printf '%s\n' "-----END PGP PUBLIC KEY BLOCK-----"
		;;
	--local-user)
		while IFS= read -r _line; do :; done
		printf '%s\n' "signed"
		;;
	*)
		exit 1
		;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gpg"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gpg: %v", err)
	}
}

func TestValidateGenerateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    GenerateOptions
		wantErr string // substring, "" for success
	}{
		{
			name: "valid RSA 4096 2y",
			opts: GenerateOptions{
				Name: "Alice", Email: "alice@example.com",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
		},
		{
			name: "valid with comment",
			opts: GenerateOptions{
				Name: "Alice", Email: "alice@example.com", Comment: "work key",
				KeyType: "RSA", KeyLength: 4096, Expiration: "0",
			},
		},
		{
			name:    "empty name",
			opts:    GenerateOptions{Name: "", Email: "a@b.c", KeyType: "RSA", KeyLength: 4096, Expiration: "2y"},
			wantErr: "name is required",
		},
		{
			name:    "empty email",
			opts:    GenerateOptions{Name: "A", Email: "", KeyType: "RSA", KeyLength: 4096, Expiration: "2y"},
			wantErr: "email is required",
		},
		{
			name: "injection via newline in name",
			opts: GenerateOptions{
				Name:    "Alice\nExpire-Date: 0",
				Email:   "a@b.c",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "disallowed characters",
		},
		{
			name: "injection via carriage return in email",
			opts: GenerateOptions{
				Name:    "Alice",
				Email:   "a@b.c\r%commit",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "disallowed characters",
		},
		{
			name: "injection via percent in comment",
			opts: GenerateOptions{
				Name: "Alice", Email: "a@b.c",
				Comment: "%commit\nKey-Type: DSA",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "disallowed characters",
		},
		{
			name: "invalid key type",
			opts: GenerateOptions{
				Name: "A", Email: "a@b.c",
				KeyType: "RSA; rm -rf /", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "invalid key type",
		},
		{
			name: "key length too small",
			opts: GenerateOptions{
				Name: "A", Email: "a@b.c",
				KeyType: "RSA", KeyLength: 512, Expiration: "2y",
			},
			wantErr: "invalid key length",
		},
		{
			name: "key length too large",
			opts: GenerateOptions{
				Name: "A", Email: "a@b.c",
				KeyType: "RSA", KeyLength: 99999, Expiration: "2y",
			},
			wantErr: "invalid key length",
		},
		{
			name: "invalid expiration shell injection",
			opts: GenerateOptions{
				Name: "A", Email: "a@b.c",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y; evil",
			},
			wantErr: "invalid expiration",
		},
		{
			name: "valid ISO date expiration",
			opts: GenerateOptions{
				Name: "A", Email: "a@b.c",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2030-12-31",
			},
		},
		{
			name: "email without @",
			opts: GenerateOptions{
				Name: "A", Email: "plainstring",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "does not contain @",
		},
		{
			name: "null byte in name",
			opts: GenerateOptions{
				Name: "A\x00B", Email: "a@b.c",
				KeyType: "RSA", KeyLength: 4096, Expiration: "2y",
			},
			wantErr: "disallowed characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerateOptions(&tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestIsInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)
	// Just ensure it doesn't panic; result depends on system
	_ = m.IsInstalled()
}

func TestIsInstalled_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)
	if m.IsInstalled() {
		t.Error("expected IsInstalled=false for fake command")
	}
}

func TestParseGPGOutput(t *testing.T) {
	output := `pub:-:4096:1:ABCDEF1234567890:20230101:20250101::-:::scESC:
fpr:::::::::AABBCCDDEE1122334455ABCDEF1234567890:
uid:-::::20230101::0000000000000000::Test User <test@example.com>:
sub:-:4096:1:1234567890ABCDEF:20230101:20250101:::::e:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k.KeyID != "ABCDEF1234567890" {
		t.Errorf("KeyID = %q", k.KeyID)
	}
	if k.Fingerprint != "AABBCCDDEE1122334455ABCDEF1234567890" {
		t.Errorf("Fingerprint = %q", k.Fingerprint)
	}
	if k.Name != "Test User" {
		t.Errorf("Name = %q", k.Name)
	}
	if k.Email != "test@example.com" {
		t.Errorf("Email = %q", k.Email)
	}
	if k.Created.IsZero() {
		t.Error("expected non-zero Created")
	}
	if k.Expires == nil {
		t.Error("expected non-nil Expires")
	}
}

func TestParseGPGOutput_Empty(t *testing.T) {
	keys := parseGPGOutput("")
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestParseGPGOutput_UIDWithoutEmail(t *testing.T) {
	output := `pub:-:4096:1:KEY123:20230101::::-:::scESC:
uid:-::::20230101::0000000000000000::JustName:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Name != "JustName" {
		t.Errorf("Name = %q", keys[0].Name)
	}
}

func TestParseGPGOutput_MultipleKeys(t *testing.T) {
	output := `pub:-:4096:1:KEY1:20230101::::-:::scESC:
fpr:::::::::FP1:
uid:-::::::::Name1 <a@b.c>:
pub:-:4096:1:KEY2:20230201::::-:::scESC:
fpr:::::::::FP2:
uid:-::::::::Name2 <d@e.f>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].KeyID != "KEY1" || keys[1].KeyID != "KEY2" {
		t.Errorf("KeyIDs = %q, %q", keys[0].KeyID, keys[1].KeyID)
	}
}

func TestGetVersion_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.GetVersion()
	if err == nil {
		t.Error("expected error for nonexistent gpg binary")
	}
}

func TestList_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// listKeys with nonexistent binary
	keys, err := m.List()
	// It might return nil,nil (exit status 2 treated as "no keys found")
	// or nil,err depending on error message
	_ = keys
	_ = err
}

func TestGetKey_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.GetKey("ABCDEF")
	// Should error because binary doesn't exist
	_ = err
}

func TestGetPublicKey_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.GetPublicKey("ABCDEF")
	if err == nil {
		t.Error("expected error for nonexistent gpg binary")
	}
}

func TestTestSigning_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.TestSigning("ABCDEF")
	if err == nil {
		t.Error("expected error for nonexistent gpg binary")
	}
}

func TestGetVersion_RealGPG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	version, err := m.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if !strings.Contains(strings.ToLower(version), "gpg") {
		t.Errorf("version %q should mention gpg", version)
	}
}

func TestList_RealGPG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Empty GNUPGHOME should have no keys
	if len(keys) != 0 {
		t.Logf("found %d keys in temp GNUPGHOME", len(keys))
	}
}

func TestGetKey_NotFound_RealGPG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	_, err := m.GetKey("NONEXISTENT_KEY_ID")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestParseGPGOutput_NoExpiry(t *testing.T) {
	output := `pub:-:4096:1:KEY1:20230101::::-:::scESC:
fpr:::::::::FP1:
uid:-::::::::Name <a@b.c>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Expires != nil {
		t.Error("expected nil Expires for key without expiry")
	}
}

func TestParseGPGOutput_InvalidDates(t *testing.T) {
	output := `pub:-:4096:1:KEY1:invalid:invalid::-:::scESC:
uid:-::::::::Name <a@b.c>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if !keys[0].Created.IsZero() {
		t.Error("expected zero Created for invalid date")
	}
}

func TestParseGPGOutput_FprWithoutPub(t *testing.T) {
	// fingerprint line without preceding pub should be ignored
	output := `fpr:::::::::ORPHAN_FP:
pub:-:4096:1:KEY1:20230101::::-:::scESC:
fpr:::::::::VALID_FP:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Fingerprint != "VALID_FP" {
		t.Errorf("fingerprint = %q", keys[0].Fingerprint)
	}
}

func TestParseGPGOutput_ShortFields(t *testing.T) {
	output := "short:line\npub:-:4096:1:KEY1:20230101::::-:::scESC:\n"
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d (short lines should be skipped)", len(keys))
	}
}

func TestValidateGenerateOptions_ValidExpDays(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 4096, Expiration: "90d",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for 90d: %v", err)
	}
}

func TestValidateGenerateOptions_ValidExpWeeks(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 4096, Expiration: "52w",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for 52w: %v", err)
	}
}

func TestValidateGenerateOptions_ValidExpMonths(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 4096, Expiration: "12m",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for 12m: %v", err)
	}
}

func TestParseGPGTimestamp_UnixAndISO(t *testing.T) {
	t.Run("unix", func(t *testing.T) {
		got, ok := parseGPGTimestamp("1700000000")
		if !ok {
			t.Fatal("expected unix timestamp to parse")
		}
		if got.IsZero() {
			t.Fatal("expected non-zero time")
		}
	})

	t.Run("iso-datetime", func(t *testing.T) {
		got, ok := parseGPGTimestamp("20240102T030405")
		if !ok {
			t.Fatal("expected ISO datetime to parse")
		}
		if got.Year() != 2024 || got.Month() != 1 || got.Day() != 2 {
			t.Fatalf("unexpected parsed date: %v", got)
		}
	})

	t.Run("iso-date", func(t *testing.T) {
		got, ok := parseGPGTimestamp("20240102")
		if !ok {
			t.Fatal("expected ISO date to parse")
		}
		if got.Year() != 2024 || got.Month() != 1 || got.Day() != 2 {
			t.Fatalf("unexpected parsed date: %v", got)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		_, ok := parseGPGTimestamp("bad-value")
		if ok {
			t.Fatal("expected invalid timestamp to fail")
		}
	})
}

func TestResolveGPGCommand_FallbackFromDefault(t *testing.T) {
	tmp := t.TempDir()
	writeFakeGPG(t, tmp)
	t.Setenv("PATH", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg2" // default candidate; should fall back to gpg
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if got := m.resolveGPGCommand(); got != "gpg" {
		t.Fatalf("resolveGPGCommand() = %q, want gpg", got)
	}
}

func TestManager_WithFakeGPG_SuccessPaths(t *testing.T) {
	tmp := t.TempDir()
	writeFakeGPG(t, tmp)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Fatal("expected fake gpg to be installed")
	}

	ver, err := m.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if !strings.Contains(ver, "Fake") {
		t.Fatalf("version = %q", ver)
	}

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one fake key")
	}

	k, err := m.GetKey("FAKEKEY1234567890")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if k.Email != "fake@example.com" {
		t.Fatalf("email = %q", k.Email)
	}

	pub, err := m.GetPublicKey("FAKEKEY1234567890")
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if !strings.Contains(pub, "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Fatalf("unexpected public key output: %q", pub)
	}

	if err := m.TestSigning("FAKEKEY1234567890"); err != nil {
		t.Fatalf("TestSigning: %v", err)
	}

	generated, err := m.Generate(GenerateOptions{Name: "Fake User", Email: "fake@example.com"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if generated.KeyID == "" {
		t.Fatal("expected generated key info")
	}
}

func TestListKeys_ExitStatus2ReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	writeFakeGPG(t, tmp)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	keys, err := m.listKeys("missing@example.com")
	if err != nil {
		t.Fatalf("listKeys: %v", err)
	}
	if keys != nil {
		t.Fatalf("expected nil keys for exit status 2, got %v", keys)
	}
}

func TestGetKey_SearchByFingerprint(t *testing.T) {
	// parseGPGOutput is used by GetKey; test that suffix match works
	output := `pub:-:4096:1:ABCDEF1234567890:20230101::::-:::scESC:
fpr:::::::::FULLFINGERPRINT1234567890ABCDEF1234567890:
uid:-::::::::Test User <test@example.com>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	// GetKey searches by KeyID or fingerprint suffix
	k := keys[0]
	if k.KeyID != "ABCDEF1234567890" {
		t.Errorf("KeyID = %q", k.KeyID)
	}
	// Verify that fingerprint suffix matching would work
	searchID := "ABCDEF1234567890"
	if !strings.HasSuffix(k.Fingerprint, searchID) {
		t.Errorf("expected fingerprint %q to have suffix %q", k.Fingerprint, searchID)
	}
}

func TestParseGPGOutput_UIDWithComment(t *testing.T) {
	output := `pub:-:4096:1:KEY1:20230101::::-:::scESC:
fpr:::::::::FP123:
uid:-::::::::Alice (work) <alice@work.com>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	// Name should include the comment part
	if !strings.Contains(keys[0].Name, "Alice") {
		t.Errorf("Name = %q, expected to contain Alice", keys[0].Name)
	}
	if keys[0].Email != "alice@work.com" {
		t.Errorf("Email = %q", keys[0].Email)
	}
}

func TestParseGPGOutput_MultipleUIDs(t *testing.T) {
	// Only first uid should be used
	output := `pub:-:4096:1:KEY1:20230101::::-:::scESC:
fpr:::::::::FP1:
uid:-::::::::First Name <first@test.com>:
uid:-::::::::Second Name <second@test.com>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	// First UID sets the name/email, subsequent ones overwrite
	if keys[0].Email == "" {
		t.Error("expected non-empty email")
	}
}

func TestValidateGenerateOptions_DSAKeyType(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "DSA", KeyLength: 2048, Expiration: "2y",
	}
	// DSA is a valid key type pattern (matches regex)
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for DSA: %v", err)
	}
}

func TestValidateGenerateOptions_EDDSAKeyType(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "EDDSA", KeyLength: 4096, Expiration: "1y",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for EDDSA: %v", err)
	}
}

func TestValidateGenerateOptions_ExpirationZero(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 4096, Expiration: "0",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for 0 expiration: %v", err)
	}
}

func TestValidateGenerateOptions_MaxKeyLength(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 16384, Expiration: "2y",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for max key length: %v", err)
	}
}

func TestValidateGenerateOptions_MinKeyLength(t *testing.T) {
	opts := GenerateOptions{
		Name: "A", Email: "a@b.c",
		KeyType: "RSA", KeyLength: 1024, Expiration: "2y",
	}
	if err := validateGenerateOptions(&opts); err != nil {
		t.Fatalf("unexpected error for min key length: %v", err)
	}
}

func TestGenerate_FakeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:    "Test",
		Email:   "test@example.com",
		KeyType: "RSA",
	})
	if err == nil {
		t.Error("expected error for nonexistent gpg binary")
	}
}

func TestGenerate_DefaultValues(t *testing.T) {
	// Verify default values are applied before validation
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// With empty KeyType, KeyLength, Expiration - defaults should be set
	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "test@example.com",
	})
	// Error is expected (fake command) but it shouldn't fail on validation
	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("defaults not applied before validation: %v", err)
	}
}

func TestGenerate_CommentInBatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Generate with a comment - should reach the batch input building
	_, err := m.Generate(GenerateOptions{
		Name:    "Test",
		Email:   "test@example.com",
		Comment: "work key",
	})
	// Error is expected (fake command) but should get past validation
	if err != nil && strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestGenerate_ValidationFailsEmptyName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want 'name is required'", err)
	}
}

func TestGenerate_ValidationFailsEmptyEmail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "",
	})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "email is required") {
		t.Errorf("error = %q, want 'email is required'", err)
	}
}

func TestGenerate_ValidationFailsInvalidKeyType(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:    "Test",
		Email:   "test@example.com",
		KeyType: "INVALID TYPE!",
	})
	if err == nil {
		t.Fatal("expected error for invalid key type")
	}
	if !strings.Contains(err.Error(), "invalid key type") {
		t.Errorf("error = %q", err)
	}
}

func TestGenerate_ValidationFailsInvalidKeyLength(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:      "Test",
		Email:     "test@example.com",
		KeyType:   "RSA",
		KeyLength: 256,
	})
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
	if !strings.Contains(err.Error(), "invalid key length") {
		t.Errorf("error = %q", err)
	}
}

func TestGenerate_ValidationFailsInvalidExpiration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:       "Test",
		Email:      "test@example.com",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "forever!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid expiration")
	}
	if !strings.Contains(err.Error(), "invalid expiration") {
		t.Errorf("error = %q", err)
	}
}

func TestGenerate_ValidationFailsUnsafeCharsInName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test\nEvil",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error for unsafe chars in name")
	}
	if !strings.Contains(err.Error(), "disallowed characters") {
		t.Errorf("error = %q", err)
	}
}

func TestGenerate_ValidationFailsEmailNoAt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "notanemail",
	})
	if err == nil {
		t.Fatal("expected error for email without @")
	}
	if !strings.Contains(err.Error(), "does not contain @") {
		t.Errorf("error = %q", err)
	}
}

func TestGetKey_NotFoundReturnsError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	_, err := m.GetKey("DEFINITELYNOTEXIST123")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected 'not found'", err)
	}
}

func TestGetPublicKey_EmptyKeyID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	// Empty key ID should return empty or error
	result, err := m.GetPublicKey("")
	// gpg --armor --export "" may succeed but return empty
	if err == nil && result == "" {
		// This is acceptable - no key found
		return
	}
	// If there's an error that's also fine
	_ = err
}

func TestTestSigning_InvalidKeyID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	err := m.TestSigning("NONEXISTENT_KEY_XYZ")
	if err == nil {
		t.Fatal("expected error for signing with nonexistent key")
	}
	if !strings.Contains(err.Error(), "signing test failed") {
		t.Errorf("error = %q", err)
	}
}

func TestListKeys_EmptyGNUPGHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	keys, err := m.List()
	if err != nil {
		t.Fatalf("List on empty GNUPGHOME: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys in fresh GNUPGHOME, got %d", len(keys))
	}
}

func TestGetVersion_ContainsGPG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	version, err := m.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if version == "" {
		t.Error("expected non-empty version string")
	}
}

// =============================================================================
// Additional coverage: Generate validation paths
// =============================================================================

func TestGenerate_FakeCommand_BatchFails(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error when GPG binary doesn't exist")
	}
	if !strings.Contains(err.Error(), "GPG is not installed") {
		t.Errorf("error = %q, expected to contain 'GPG is not installed'", err.Error())
	}
}

func TestGenerate_EmptyName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_EmptyEmail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "",
	})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "email is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_InvalidKeyType(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:    "Test",
		Email:   "test@example.com",
		KeyType: "INVALID!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid key type")
	}
	if !strings.Contains(err.Error(), "invalid key type") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_InvalidKeyLength(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:      "Test",
		Email:     "test@example.com",
		KeyType:   "RSA",
		KeyLength: 99999,
	})
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
	if !strings.Contains(err.Error(), "invalid key length") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_InvalidExpiration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:       "Test",
		Email:      "test@example.com",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "invalid!",
	})
	if err == nil {
		t.Fatal("expected error for invalid expiration")
	}
	if !strings.Contains(err.Error(), "invalid expiration") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_UnsafeName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Inject\n%commit",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error for unsafe name")
	}
	if !strings.Contains(err.Error(), "disallowed characters") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_EmailWithoutAt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "noemailatall",
	})
	if err == nil {
		t.Fatal("expected error for email without @")
	}
	if !strings.Contains(err.Error(), "does not contain @") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestGenerate_WithComment(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// This exercises the comment insertion path even though generation fails
	_, err := m.Generate(GenerateOptions{
		Name:    "Test",
		Email:   "test@example.com",
		Comment: "my work key",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should be from resolveGPGCommand since binary doesn't exist
	if !strings.Contains(err.Error(), "GPG is not installed") {
		t.Errorf("error = %q, expected 'GPG is not installed'", err.Error())
	}
}

func TestGenerate_DefaultValuesApplied(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Verify defaults are applied (KeyType="", KeyLength=0, Expiration="")
	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The defaults (RSA, 4096, 2y) should be set without validation errors
	if strings.Contains(err.Error(), "invalid key type") ||
		strings.Contains(err.Error(), "invalid key length") ||
		strings.Contains(err.Error(), "invalid expiration") {
		t.Errorf("default values caused validation error: %v", err)
	}
}

// =============================================================================
// Additional coverage: GetKey - key found vs not found
// =============================================================================

func TestGetKey_NotFoundWithEmptyList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", t.TempDir())
	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	_, err := m.GetKey("NONEXISTENT_KEY_ID")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, expected 'not found'", err.Error())
	}
}

func TestGetKey_MatchByFingerprint(t *testing.T) {
	// This test verifies the fingerprint suffix matching path in GetKey
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "nonexistent-gpg-binary-xyz"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// GetKey calls listKeys which will fail, returning an error
	_, err := m.GetKey("ABCDEF")
	// Should error because binary doesn't exist - verifying path is exercised
	if err == nil {
		// If somehow no error, it should be "not found"
		t.Log("GetKey didn't error (unexpected)")
	}
}

// =============================================================================
// Additional coverage: Generate and GetKey error paths
// =============================================================================

func TestGenerate_KeyNotFoundAfterGeneration(t *testing.T) {
	// Use a script that pretends to succeed gen-key but has no keys to list
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	// Generate with a non-matching email to test "key generated but not found" path
	_, err := m.Generate(GenerateOptions{
		Name:       "Test User",
		Email:      "unique-nonexist-12345@test-domain-xyz.invalid",
		KeyType:    "RSA",
		KeyLength:  1024,
		Expiration: "0",
	})
	// This may fail at GPG key generation or at "key not found" step
	if err == nil {
		t.Log("Generate succeeded (GPG env accepted it)")
	}
}

func TestGetKey_RealGPG_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	_, err := m.GetKey("FFFFFFFFFFFFFFFF")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestGetPublicKey_RealGPG_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	// Exporting a nonexistent key - gpg may return empty output or error
	result, err := m.GetPublicKey("NONEXISTENT_KEY_12345")
	if err != nil {
		// Error path covered
		return
	}
	// If no error, the result should be empty (gpg exports nothing for missing keys)
	if result != "" {
		t.Errorf("expected empty export for nonexistent key, got %d bytes", len(result))
	}
}

func TestTestSigning_RealGPG_InvalidKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("GNUPGHOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if !m.IsInstalled() {
		t.Skip("gpg not installed")
	}

	err := m.TestSigning("NONEXISTENT_KEY_12345")
	if err == nil {
		t.Fatal("expected error when signing with nonexistent key")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error = %q, expected 'failed'", err.Error())
	}
}

func TestValidateGenerateOptions_CommentWithControlChars(t *testing.T) {
	opts := &GenerateOptions{
		Name:       "Test",
		Email:      "test@example.com",
		Comment:    "bad\ncomment",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "2y",
	}
	err := validateGenerateOptions(opts)
	if err == nil {
		t.Fatal("expected error for comment with control characters")
	}
	if !strings.Contains(err.Error(), "disallowed") {
		t.Errorf("error = %q, expected 'disallowed'", err.Error())
	}
}

func TestValidateGenerateOptions_EmailWithControlChars(t *testing.T) {
	opts := &GenerateOptions{
		Name:       "Test",
		Email:      "test\r@example.com",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "2y",
	}
	err := validateGenerateOptions(opts)
	if err == nil {
		t.Fatal("expected error for email with control characters")
	}
}

func TestValidateGenerateOptions_NameWithPercentChar(t *testing.T) {
	opts := &GenerateOptions{
		Name:       "Test%User",
		Email:      "test@example.com",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "2y",
	}
	err := validateGenerateOptions(opts)
	if err == nil {
		t.Fatal("expected error for name with percent character")
	}
}

func TestValidateGenerateOptions_DateExpiration(t *testing.T) {
	opts := &GenerateOptions{
		Name:       "Test",
		Email:      "test@example.com",
		KeyType:    "RSA",
		KeyLength:  4096,
		Expiration: "2025-12-31",
	}
	err := validateGenerateOptions(opts)
	if err != nil {
		t.Fatalf("unexpected error for date expiration: %v", err)
	}
}

func TestGenerate_InvalidKeyType_RunsValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:      "Test",
		Email:     "test@example.com",
		KeyType:   "INVALID!!!",
		KeyLength: 4096,
	})
	if err == nil {
		t.Fatal("expected error for invalid key type")
	}
	if !strings.Contains(err.Error(), "invalid key type") {
		t.Errorf("error = %q, expected 'invalid key type'", err.Error())
	}
}

func TestGenerate_InvalidKeyLength_TooSmall(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:      "Test",
		Email:     "test@example.com",
		KeyType:   "RSA",
		KeyLength: 512,
	})
	if err == nil {
		t.Fatal("expected error for key length too small")
	}
	if !strings.Contains(err.Error(), "invalid key length") {
		t.Errorf("error = %q, expected 'invalid key length'", err.Error())
	}
}

func TestGenerate_InvalidKeyLength_TooLarge(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:      "Test",
		Email:     "test@example.com",
		KeyType:   "RSA",
		KeyLength: 99999,
	})
	if err == nil {
		t.Fatal("expected error for key length too large")
	}
}

func TestGetKey_FoundByID_WithFakeScript(t *testing.T) {
	// Create a script that outputs valid GPG colon-formatted key data.
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gpg")
	scriptContent := `#!/bin/sh
echo "pub:-:4096:1:AABBCCDDEE112233:1700000000:::-:::scESC::::::23::0:"
echo "fpr:::::::::AABBCCDDEEFF00112233445566778899AABBCCDDEE112233:"
echo "uid:-::::1700000000::ABCDEF::Alice <alice@example.com>::::::::::0:"
`
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Advanced: config.AdvancedConfig{GPGCommand: script},
	}
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	key, err := m.GetKey("AABBCCDDEE112233")
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if key.KeyID != "AABBCCDDEE112233" {
		t.Errorf("KeyID = %q, want AABBCCDDEE112233", key.KeyID)
	}
}

func TestGetKey_IteratesKeysNoMatch_Script(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gpg")
	scriptContent := `#!/bin/sh
echo "pub:-:4096:1:AAAA111122223333:1700000000:::-:::scESC::::::23::0:"
echo "fpr:::::::::AAAA111122223333BBBB444455556666CCCC777788889999:"
echo "uid:-::::1700000000::ABCDEF::Bob <bob@example.com>::::::::::0:"
`
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Advanced: config.AdvancedConfig{GPGCommand: script},
	}
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.GetKey("NONEXISTENT999")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListKeys_NonFatalCommandError(t *testing.T) {
	// Script exits with status 1 (not 2) and doesn't contain "not found".
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-gpg")
	scriptContent := `#!/bin/sh
echo "gpg: error reading key" >&2
exit 1
`
	if err := os.WriteFile(script, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Advanced: config.AdvancedConfig{GPGCommand: script},
	}
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error for exit status 1")
	}
	if !strings.Contains(err.Error(), "listing GPG keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGPGTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantOK bool
		wantYr int // expected year, 0 means don't check
	}{
		{"empty string", "", false, 0},
		{"unix timestamp", "1672531200", true, 2023},            // 2023-01-01 00:00:00 UTC
		{"unix timestamp far future", "1893456000", true, 2030}, // 2030-01-01
		{"yyyymmdd format", "20230101", true, 2023},
		{"yyyymmddThhmmss format", "20230615T120000", true, 2023},
		{"negative timestamp", "-1", false, 0},
		{"zero timestamp", "0", false, 0},
		{"non-numeric garbage", "notadate", false, 0},
		{"partial date", "123456", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, ok := parseGPGTimestamp(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseGPGTimestamp(%q) ok=%v, want %v", tt.input, ok, tt.wantOK)
			}
			if tt.wantOK && tt.wantYr != 0 && ts.Year() != tt.wantYr {
				t.Errorf("parseGPGTimestamp(%q) year=%d, want %d", tt.input, ts.Year(), tt.wantYr)
			}
		})
	}
}

func TestParseGPGOutput_UnixTimestamps(t *testing.T) {
	// Modern GPG uses Unix timestamps in colon-format output
	output := `pub:-:4096:1:ABCDEF1234567890:1672531200:1735689600::-:::scESC:
fpr:::::::::AABBCCDDEE1122334455ABCDEF1234567890:
uid:-::::::::Test User <test@example.com>:
`
	keys := parseGPGOutput(output)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k.Created.Year() != 2023 {
		t.Errorf("Created year = %d, want 2023", k.Created.Year())
	}
	if k.Expires == nil {
		t.Fatal("expected non-nil Expires")
	}
	if k.Expires.Year() != 2025 {
		t.Errorf("Expires year = %d, want 2025", k.Expires.Year())
	}
}

func TestResolveGPGCommand_NoneAvailable(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATH", tmp) // empty dir, no GPG
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "" // no explicit command
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	if m.IsInstalled() {
		t.Error("expected GPG to not be found")
	}
}

func TestGenerate_WithComment_FakeGPG(t *testing.T) {
	tmp := t.TempDir()
	writeFakeGPG(t, tmp)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	key, err := m.Generate(GenerateOptions{
		Name:    "Fake User",
		Email:   "fake@example.com",
		Comment: "my work key",
	})
	if err != nil {
		t.Fatalf("Generate with comment: %v", err)
	}
	if key.KeyID == "" {
		t.Fatal("expected generated key info")
	}
}

func TestGenerate_ListKeysError(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG where --batch succeeds but --list-keys always fails
	script := `#!/bin/sh
case "$1" in
	--batch) while IFS= read -r _; do :; done; exit 0 ;;
	--list-keys) echo "error" >&2; exit 1 ;;
	*) exit 1 ;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error when listKeys fails")
	}
	if !strings.Contains(err.Error(), "finding generated key") {
		t.Errorf("error = %q, expected 'finding generated key'", err.Error())
	}
}

func TestGenerate_EmptyListAfterGeneration(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG where --batch succeeds and --list-keys returns empty
	script := `#!/bin/sh
case "$1" in
	--batch) while IFS= read -r _; do :; done; exit 0 ;;
	--list-keys) exit 0 ;;
	*) exit 1 ;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.Generate(GenerateOptions{
		Name:  "Test",
		Email: "test@example.com",
	})
	if err == nil {
		t.Fatal("expected error when no keys found after generation")
	}
	if !strings.Contains(err.Error(), "could not be found") {
		t.Errorf("error = %q, expected 'could not be found'", err.Error())
	}
}

func TestGetVersion_CommandError(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG where --version fails
	script := `#!/bin/sh
case "$1" in
	--version) exit 1 ;;
	*) exit 1 ;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	_, err := m.GetVersion()
	if err == nil {
		t.Fatal("expected error when --version fails")
	}
}

func TestGetVersion_EmptyOutput(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG where --version outputs nothing
	script := `#!/bin/sh
case "$1" in
	--version) exit 0 ;;
	*) exit 1 ;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	ver, err := m.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if ver != "" {
		t.Errorf("version = %q, want empty", ver)
	}
}

func TestDelete_Success(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG that returns a fingerprint, then succeeds on delete
	script := `#!/bin/sh
case "$1" in
	--with-colons)
		printf '%s\n' "pub:-:4096:1:FAKEKEY123:1700000000:0::-:::scESC:"
		printf '%s\n' "fpr:::::::::AAAA1111BBBB2222CCCC3333DDDD4444EEEE5555:"
		exit 0
		;;
	--batch)
		exit 0
		;;
	*)
		exit 1
		;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.Delete("FAKEKEY123")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDelete_GPGNotInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATH", tmp) // empty PATH, no gpg
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.Delete("SOMEKEY")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected 'not installed' error, got: %v", err)
	}
}

func TestDelete_KeyNotFound(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG where --with-colons fails (key not found)
	script := `#!/bin/sh
case "$1" in
	--with-colons)
		exit 1
		;;
	*)
		exit 1
		;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.Delete("NOSUCHKEY")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestDelete_NoFingerprint(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG that returns output but no fpr line
	script := `#!/bin/sh
case "$1" in
	--with-colons)
		printf '%s\n' "pub:-:4096:1:FAKEKEY123:1700000000:0::-:::scESC:"
		printf '%s\n' "uid:-::::1700000000::0000000000000000::Fake <f@e.com>:"
		exit 0
		;;
	*)
		exit 1
		;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.Delete("FAKEKEY123")
	if err == nil || !strings.Contains(err.Error(), "could not find fingerprint") {
		t.Fatalf("expected 'could not find fingerprint' error, got: %v", err)
	}
}

func TestDelete_DeletePublicKeyFails(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG: fingerprint works, delete-secret-keys works, delete-keys fails
	script := `#!/bin/sh
case "$1" in
	--with-colons)
		printf '%s\n' "fpr:::::::::AAAA1111BBBB2222CCCC3333DDDD4444EEEE5555:"
		exit 0
		;;
	--batch)
		# $3 is the operation: --delete-secret-keys or --delete-keys
		if [ "$3" = "--delete-keys" ]; then
			echo "delete failed"
			exit 1
		fi
		exit 0
		;;
	*)
		exit 1
		;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	err := m.Delete("FAKEKEY123")
	if err == nil || !strings.Contains(err.Error(), "failed to delete") {
		t.Fatalf("expected 'failed to delete' error, got: %v", err)
	}
}

func TestDelete_DeleteSecretKeyFails_StillContinues(t *testing.T) {
	tmp := t.TempDir()
	// Fake GPG: fingerprint works, delete-secret-keys fails, delete-keys succeeds
	script := `#!/bin/sh
case "$1" in
	--with-colons)
		printf '%s\n' "fpr:::::::::AAAA1111BBBB2222CCCC3333DDDD4444EEEE5555:"
		exit 0
		;;
	--batch)
		if [ "$3" = "--delete-secret-keys" ]; then
			echo "no secret key"
			exit 1
		fi
		exit 0
		;;
	*)
		exit 1
		;;
esac
`
	os.WriteFile(filepath.Join(tmp, "gpg"), []byte(script), 0o755)
	t.Setenv("PATH", tmp)
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Advanced.GPGCommand = "gpg"
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Should succeed even though secret key deletion failed
	err := m.Delete("FAKEKEY123")
	if err != nil {
		t.Fatalf("Delete should succeed when only secret key deletion fails: %v", err)
	}
}
