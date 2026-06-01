package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeTempFile struct {
	name     string
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (f *fakeTempFile) Name() string                { return f.name }
func (f *fakeTempFile) Write(_ []byte) (int, error) { return 0, f.writeErr }
func (f *fakeTempFile) Chmod(_ os.FileMode) error   { return f.chmodErr }
func (f *fakeTempFile) Sync() error                 { return f.syncErr }
func (f *fakeTempFile) Close() error                { return f.closeErr }

func TestEnsureDirs_TightenSensitivePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions do not apply on Windows")
	}

	// Work inside a temp HOME so we don't touch the user's real ~/.gcm.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// DefaultConfig reads HOME at call time, so this picks up the override.
	cfg := DefaultConfig()

	if err := EnsureDirs(cfg); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	sensitive := []string{
		filepath.Join(GCMDir(), "tokens"),
		filepath.Join(GCMDir(), "backups"),
		filepath.Join(GCMDir(), "logs"),
	}
	for _, dir := range sensitive {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := fi.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s perm = %o, want 0700", dir, perm)
		}
	}
}

func TestEnsureDirs_TightensExistingDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions do not apply on Windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Simulate an older install where tokens/ was created world-readable.
	oldTokens := filepath.Join(home, ".gcm", "tokens")
	if err := os.MkdirAll(oldTokens, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// MkdirAll respects umask so the on-disk bits may differ - force it.
	if err := os.Chmod(oldTokens, 0o755); err != nil {
		t.Fatalf("chmod seed: %v", err)
	}

	cfg := DefaultConfig()
	if err := EnsureDirs(cfg); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	fi, err := os.Stat(oldTokens)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("tokens perm after EnsureDirs = %o, want 0700 (existing dirs must be tightened)", perm)
	}
}

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Advanced.GitCommand != "git" {
		t.Errorf("GitCommand = %q, want %q", cfg.Advanced.GitCommand, "git")
	}
	if cfg.AutoSwitch.DetectionStrategy != "project_file" {
		t.Errorf("DetectionStrategy = %q, want %q", cfg.AutoSwitch.DetectionStrategy, "project_file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":::invalid"), 0o600)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSave_And_Load(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = "custom-git"

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Advanced.GitCommand != "custom-git" {
		t.Errorf("GitCommand = %q, want %q", loaded.Advanced.GitCommand, "custom-git")
	}
}

func TestDefaultConfig_Defaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if cfg.ProfilesDir == "" {
		t.Error("expected non-empty ProfilesDir")
	}
	if cfg.Security.AuditLog != true {
		t.Error("expected audit log enabled by default")
	}
}

func TestSetConfigPathForTesting(t *testing.T) {
	origPath := ConfigPath()
	customPath := "/tmp/test-gcm/config.yaml"

	restore := SetConfigPathForTesting(customPath)
	if got := ConfigPath(); got != customPath {
		t.Errorf("ConfigPath() = %q, want %q", got, customPath)
	}

	restore()
	if got := ConfigPath(); got != origPath {
		t.Errorf("after restore: ConfigPath() = %q, want %q", got, origPath)
	}
}

func TestValidateConfigPaths_AbsoluteGitCommand_NotExist(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = "/nonexistent/path/to/fakegit"

	err := validateConfigPaths(cfg, "/some/config.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent absolute git_command path")
	}
	if !strings.Contains(err.Error(), "refusing to save") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfigPaths_AbsoluteGitCommand_Exists(t *testing.T) {
	tmp := t.TempDir()
	fakeGit := filepath.Join(tmp, "git")
	os.WriteFile(fakeGit, []byte("#!/bin/sh\n"), 0755)

	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = fakeGit

	err := validateConfigPaths(cfg, "/some/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigPaths_RelativeGitCommand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = "custom-git"

	err := validateConfigPaths(cfg, "/some/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error for relative git command: %v", err)
	}
}

func TestSave_ValidateConfigPathsRejectsInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".gcm"), 0755)

	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = "/nonexistent/fakegit/binary"

	err := Save(cfg)
	if err == nil {
		t.Fatal("expected Save to fail when git_command is invalid absolute path")
	}
	if !strings.Contains(err.Error(), "refusing to save") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigPath_Format(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := ConfigPath()
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected config.yaml, got %q", filepath.Base(path))
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify config file exists
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestSave_OverwritesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	cfg.Advanced.GitCommand = "first"
	if err := Save(cfg); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	cfg.Advanced.GitCommand = "second"
	if err := Save(cfg); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Advanced.GitCommand != "second" {
		t.Errorf("GitCommand = %q, want %q", loaded.Advanced.GitCommand, "second")
	}
}

func TestLoad_ReadableYAMLConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	yamlContent := `default_profile: work
ui:
  color: false
  verbose: true
`
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yamlContent), 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProfile != "work" {
		t.Errorf("DefaultProfile = %q, want %q", cfg.DefaultProfile, "work")
	}
	if cfg.UI.Color != false {
		t.Error("expected Color=false")
	}
	if cfg.UI.Verbose != true {
		t.Error("expected Verbose=true")
	}
}

func TestEnsureDirs_CreatesAllDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if err := EnsureDirs(cfg); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Verify all dirs exist
	for _, dir := range []string{
		cfg.ProfilesDir,
		cfg.TemplatesDir,
		cfg.CacheDir,
		filepath.Join(GCMDir(), "tokens"),
		filepath.Join(GCMDir(), "backups"),
		filepath.Join(GCMDir(), "logs"),
	} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("directory %s not created: %v", dir, err)
		}
	}
}

func TestGCMDir_WithHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := GCMDir()
	if dir != filepath.Join(home, ".gcm") {
		t.Errorf("GCMDir = %q, want %q", dir, filepath.Join(home, ".gcm"))
	}
}

func TestDefaultConfig_SecurityDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if !cfg.Security.EncryptTokens {
		t.Error("expected EncryptTokens=true")
	}
	if !cfg.Security.UseKeychain {
		t.Error("expected UseKeychain=true")
	}
	if cfg.Security.MasterPassword {
		t.Error("expected MasterPassword=false")
	}
	if cfg.Security.AllowPlaintextTokens {
		t.Error("expected AllowPlaintextTokens=false")
	}
}

func TestDefaultConfig_BackupDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if cfg.Backup.AutoBackup {
		t.Error("expected AutoBackup=false")
	}
	if cfg.Backup.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d", cfg.Backup.RetentionDays)
	}
	if cfg.Backup.MaxBackups != 10 {
		t.Errorf("MaxBackups = %d", cfg.Backup.MaxBackups)
	}
	if cfg.Backup.Encryption {
		t.Error("expected Backup.Encryption=false")
	}
	if cfg.Backup.IncludeKeys {
		t.Error("expected IncludeKeys=false")
	}
}

func TestDefaultConfig_AdvancedDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	if cfg.Advanced.GitCommand != "git" {
		t.Errorf("GitCommand = %q", cfg.Advanced.GitCommand)
	}
	if cfg.Advanced.SSHCommand != "ssh" {
		t.Errorf("SSHCommand = %q", cfg.Advanced.SSHCommand)
	}
	if cfg.Advanced.GPGCommand != "gpg" {
		t.Errorf("GPGCommand = %q", cfg.Advanced.GPGCommand)
	}
}

func TestSave_UnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create the .gcm dir as read-only
	gcmDir := filepath.Join(home, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.Chmod(gcmDir, 0o444)
	t.Cleanup(func() { os.Chmod(gcmDir, 0o755) })

	cfg := DefaultConfig()
	err := Save(cfg)
	if err == nil {
		t.Fatal("expected error saving to read-only dir")
	}
}

func TestLoad_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	configFile := filepath.Join(configDir, "config.yaml")
	os.WriteFile(configFile, []byte("default_profile: test\n"), 0o000)
	t.Cleanup(func() { os.Chmod(configFile, 0o644) })

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unreadable config")
	}
}

func TestLoad_MalformedYAMLVariants(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"tabs and colons", "\t::\t::"},
		{"invalid structure", "- - - invalid"},
		{"binary garbage", "\x00\x01\x02\x03"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			configDir := filepath.Join(home, ".gcm")
			os.MkdirAll(configDir, 0o755)
			os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(tt.content), 0o600)

			_, err := Load()
			if err == nil {
				t.Errorf("expected error for malformed YAML: %q", tt.content)
			}
		})
	}
}

func TestEnsureDirs_ImpossiblePath(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	// Set a path that can't be created
	cfg.ProfilesDir = "/dev/null/impossible/profiles"

	err := EnsureDirs(cfg)
	if err == nil {
		t.Fatal("expected error for impossible directory path")
	}
}

func TestLoad_PartialYAMLConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	// Only set some fields - rest should keep defaults
	yamlContent := `default_profile: partial
`
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yamlContent), 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProfile != "partial" {
		t.Errorf("DefaultProfile = %q, want partial", cfg.DefaultProfile)
	}
}

func TestSave_HookErrors(t *testing.T) {
	origMarshal := yamlMarshalFn
	origCreate := createTempFn
	origRename := renameFn
	origMkdir := mkdirAllFn
	origStat := statFn
	defer func() {
		yamlMarshalFn = origMarshal
		createTempFn = origCreate
		renameFn = origRename
		mkdirAllFn = origMkdir
		statFn = origStat
	}()

	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := DefaultConfig()

	t.Run("marshal error", func(t *testing.T) {
		yamlMarshalFn = func(any) ([]byte, error) { return nil, fmt.Errorf("marshal failed") }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected marshaling error")
		}
		yamlMarshalFn = origMarshal
	})

	t.Run("create temp error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) { return nil, fmt.Errorf("temp fail") }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected create temp error")
		}
		createTempFn = origCreate
	})

	t.Run("write error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(home, "fake-write"), writeErr: fmt.Errorf("write fail")}, nil
		}
		statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected write error")
		}
	})

	t.Run("chmod error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(home, "fake-chmod"), chmodErr: fmt.Errorf("chmod fail")}, nil
		}
		statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected chmod error")
		}
	})

	t.Run("sync error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(home, "fake-sync"), syncErr: fmt.Errorf("sync fail")}, nil
		}
		statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected sync error")
		}
	})

	t.Run("close error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(home, "fake-close"), closeErr: fmt.Errorf("close fail")}, nil
		}
		statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected close error")
		}
	})

	t.Run("rename error", func(t *testing.T) {
		createTempFn = func(string, string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(home, "fake-rename")}, nil
		}
		renameFn = func(string, string) error { return fmt.Errorf("rename fail") }
		statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := Save(cfg)
		if err == nil {
			t.Fatal("expected rename error")
		}
		renameFn = origRename
		createTempFn = origCreate
		statFn = origStat
	})
}

func TestEnsureDirs_ChmodErrorHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := DefaultConfig()

	origChmod := chmodPathFn
	defer func() { chmodPathFn = origChmod }()

	chmodPathFn = func(path string, perm os.FileMode) error {
		if strings.HasSuffix(path, string(filepath.Separator)+"tokens") {
			return fmt.Errorf("chmod blocked")
		}
		return origChmod(path, perm)
	}

	err := EnsureDirs(cfg)
	if err == nil {
		t.Fatal("expected chmod tightening error")
	}
}

func TestEnsureDirs_PartialFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	// Create a file where a directory is expected so MkdirAll fails
	cfg.TemplatesDir = "/dev/null/impossible/templates"

	err := EnsureDirs(cfg)
	if err == nil {
		t.Fatal("expected error for impossible path in EnsureDirs")
	}
}

func TestSave_EmptyConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save empty config: %v", err)
	}

	// Should be loadable
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after empty save: %v", err)
	}
	_ = loaded
}

func TestGCMDir_Consistent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir1 := GCMDir()
	dir2 := GCMDir()
	if dir1 != dir2 {
		t.Errorf("GCMDir() not consistent: %q vs %q", dir1, dir2)
	}
}

func TestConfigPath_ContainsGCMDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := ConfigPath()
	gcmDir := GCMDir()
	if !strings.HasPrefix(path, gcmDir) {
		t.Errorf("ConfigPath %q not under GCMDir %q", path, gcmDir)
	}
}

func TestSave_UnwritableConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .gcm as a file to block MkdirAll
	gcmDir := filepath.Join(home, ".gcm")
	os.WriteFile(gcmDir, []byte("blocker"), 0o644)

	cfg := DefaultConfig()
	err := Save(cfg)
	if err == nil {
		t.Fatal("expected error when config directory creation fails")
	}
}

func TestEnsureDirs_BlockedByFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := DefaultConfig()
	// Create a file where tokens dir should be
	os.MkdirAll(GCMDir(), 0o755)
	os.WriteFile(filepath.Join(GCMDir(), "tokens"), []byte("blocker"), 0o644)

	err := EnsureDirs(cfg)
	if err == nil {
		t.Fatal("expected error when dir creation is blocked")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("profiles_dir:\n  - invalid\n  nested: bad\n\x00"), 0o600)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestSave_WriteFileError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .gcm directory but make config.yaml a directory
	configDir := filepath.Join(home, ".gcm")
	os.MkdirAll(configDir, 0o755)
	os.MkdirAll(filepath.Join(configDir, "config.yaml"), 0o755) // dir where file should be

	cfg := DefaultConfig()
	err := Save(cfg)
	if err == nil {
		t.Fatal("expected error when config path is a directory")
	}
}

// =============================================================================
// GCMDir / DefaultConfig error paths (using test hooks)
// =============================================================================

func TestGCMDir_HomeError(t *testing.T) {
	old := userHomeDirFn
	oldExit := exitFn
	defer func() { userHomeDirFn = old; exitFn = oldExit }()

	userHomeDirFn = func() (string, error) {
		return "", fmt.Errorf("no home")
	}
	exitCalled := false
	exitFn = func(code int) {
		exitCalled = true
		panic("exit") // prevent further execution
	}

	defer func() { recover() }()
	_ = GCMDir()
	if !exitCalled {
		t.Fatal("expected exit to be called")
	}
}

func TestGCMDir_EmptyHome(t *testing.T) {
	old := userHomeDirFn
	oldExit := exitFn
	defer func() { userHomeDirFn = old; exitFn = oldExit }()

	userHomeDirFn = func() (string, error) {
		return "", nil
	}
	exitCalled := false
	exitFn = func(code int) {
		exitCalled = true
		panic("exit")
	}

	defer func() { recover() }()
	_ = GCMDir()
	if !exitCalled {
		t.Fatal("expected exit to be called")
	}
}

func TestDefaultConfig_HomeError(t *testing.T) {
	old := userHomeDirFn
	oldExit := exitFn
	defer func() { userHomeDirFn = old; exitFn = oldExit }()

	userHomeDirFn = func() (string, error) {
		return "", fmt.Errorf("no home")
	}
	exitCalled := false
	exitFn = func(code int) {
		exitCalled = true
		panic("exit")
	}

	defer func() { recover() }()
	_ = DefaultConfig()
	if !exitCalled {
		t.Fatal("expected exit to be called")
	}
}

func TestDefaultConfig_EmptyHome(t *testing.T) {
	old := userHomeDirFn
	oldExit := exitFn
	defer func() { userHomeDirFn = old; exitFn = oldExit }()

	userHomeDirFn = func() (string, error) {
		return "", nil
	}
	exitCalled := false
	exitFn = func(code int) {
		exitCalled = true
		panic("exit")
	}

	defer func() { recover() }()
	_ = DefaultConfig()
	if !exitCalled {
		t.Fatal("expected exit to be called")
	}
}

func TestEnsureDirs_ChmodError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test chmod failure as root")
	}
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := DefaultConfig()
	// Create tokens dir as a regular file to block MkdirAll
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.MkdirAll(cfg.ProfilesDir, 0o755)
	os.MkdirAll(cfg.TemplatesDir, 0o755)
	os.MkdirAll(cfg.CacheDir, 0o755)

	// Block the tokens dir creation by putting a file in its place
	tokensDir := filepath.Join(gcmDir, "tokens")
	os.WriteFile(tokensDir, []byte("not-a-dir"), 0o644)

	err := EnsureDirs(cfg)
	if err == nil {
		t.Fatal("expected error when tokens dir is blocked by a file")
	}
}
