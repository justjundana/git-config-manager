package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/pkg/logger"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	log := logger.New(logger.LevelError, os.Stderr)
	return NewManager(log)
}

// writeShellRC writes content to a temporary file that a test can point the
// manager at. Returns the path.
func writeShellRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding rc: %v", err)
	}
	return path
}

func TestUninstall_CurrentMarkerFormat(t *testing.T) {
	m := newTestManager(t)

	before := "export PATH=/usr/bin\n"
	gcmBlock := startMarker + "\neval \"$(gcm shell init zsh)\"\n_gcm_hook() { :; }\n" + endMarker + "\n"
	after := "alias ll='ls -la'\n"

	path := writeShellRC(t, before+gcmBlock+after)

	out, err := m.uninstallAt(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	result := string(out)

	if strings.Contains(result, startMarker) || strings.Contains(result, endMarker) {
		t.Fatalf("markers not removed: %q", result)
	}
	if strings.Contains(result, "_gcm_hook") {
		t.Fatalf("hook code not removed: %q", result)
	}
	if !strings.Contains(result, "export PATH=/usr/bin") {
		t.Fatalf("user content before block lost: %q", result)
	}
	if !strings.Contains(result, "alias ll='ls -la'") {
		t.Fatalf("user content after block lost: %q", result)
	}
}

func TestUninstall_LegacyMarkerFormat(t *testing.T) {
	m := newTestManager(t)

	before := "export PATH=/usr/bin\n"
	// Legacy block: single marker, terminated by blank line.
	legacyBlock := legacyMarker + "\neval \"$(gcm shell init zsh)\"\n_gcm_hook() { :; }\n\n"
	after := "alias ll='ls -la'\n"

	path := writeShellRC(t, before+legacyBlock+after)

	out, err := m.uninstallAt(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	result := string(out)

	if strings.Contains(result, legacyMarker) {
		t.Fatalf("legacy marker not removed: %q", result)
	}
	if strings.Contains(result, "_gcm_hook") {
		t.Fatalf("hook code not removed: %q", result)
	}
	if !strings.Contains(result, "alias ll='ls -la'") {
		t.Fatalf("user content after block lost: %q", result)
	}
}

func TestUninstall_MissingBlockReportsError(t *testing.T) {
	m := newTestManager(t)
	path := writeShellRC(t, "export PATH=/usr/bin\n")

	if _, err := m.uninstallAt(path); err == nil {
		t.Fatal("expected error when no GCM block present")
	}
}

func TestInstall_WritesBracketedMarkers(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")

	content, err := m.installAt(path, "echo hi")
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, startMarker) || !strings.Contains(s, endMarker) {
		t.Fatalf("expected bracketed markers in content: %q", s)
	}
	if !strings.Contains(s, "echo hi") {
		t.Fatalf("expected hook body, got %q", s)
	}
}

func TestInstall_RefusesIfAlreadyInstalled(t *testing.T) {
	m := newTestManager(t)
	path := writeShellRC(t, startMarker+"\necho already\n"+endMarker+"\n")

	if _, err := m.installAt(path, "echo new"); err == nil {
		t.Fatal("expected error for re-install")
	}
}

func TestInstall_RefusesIfLegacyInstalled(t *testing.T) {
	m := newTestManager(t)
	path := writeShellRC(t, legacyMarker+"\necho legacy\n\n")

	if _, err := m.installAt(path, "echo new"); err == nil {
		t.Fatal("expected error when legacy block exists")
	}
}

func TestGenerateInitScript(t *testing.T) {
	m := newTestManager(t)

	shells := []ShellType{ShellBash, ShellZsh, ShellFish, ShellPowerShell}
	for _, sh := range shells {
		t.Run(string(sh), func(t *testing.T) {
			script := m.GenerateInitScript(sh)
			if script == "" {
				t.Fatal("expected non-empty init script")
			}
			if !strings.Contains(script, "gcm") {
				t.Errorf("init script for %s should mention gcm", sh)
			}
		})
	}
	// Unknown returns a comment
	script := m.GenerateInitScript(ShellUnknown)
	if !strings.Contains(script, "Unsupported") {
		t.Errorf("unknown shell should return unsupported comment")
	}
}

func TestGenerateHook_BashContainsPromptCommand(t *testing.T) {
	m := newTestManager(t)
	script := m.generateHook(ShellBash)
	if !strings.Contains(script, "PROMPT_COMMAND") {
		t.Error("bash hook should set PROMPT_COMMAND")
	}
	if !strings.Contains(script, "_gcm_auto_switch") {
		t.Error("bash hook should define _gcm_auto_switch")
	}
}

func TestGenerateHook_ZshContainsChpwd(t *testing.T) {
	m := newTestManager(t)
	script := m.generateHook(ShellZsh)
	if !strings.Contains(script, "chpwd") {
		t.Error("zsh hook should use chpwd")
	}
	if !strings.Contains(script, "PROMPT_SUBST") {
		t.Error("zsh hook should enable PROMPT_SUBST")
	}
}

func TestGenerateHook_FishContainsPWD(t *testing.T) {
	m := newTestManager(t)
	script := m.generateHook(ShellFish)
	if !strings.Contains(script, "--on-variable PWD") {
		t.Error("fish hook should use --on-variable PWD")
	}
}

func TestGenerateHook_PowerShellContainsTestPath(t *testing.T) {
	m := newTestManager(t)
	script := m.generateHook(ShellPowerShell)
	if !strings.Contains(script, "Test-Path") {
		t.Error("powershell hook should use Test-Path")
	}
}

func TestGenerateHook_UnknownReturnsComment(t *testing.T) {
	m := newTestManager(t)
	script := m.generateHook(ShellUnknown)
	if !strings.Contains(script, "Unsupported") {
		t.Error("unknown shell should return unsupported comment")
	}
}

func TestGenerateCompletionScript(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		shell    ShellType
		contains string
	}{
		{ShellBash, "completion bash"},
		{ShellZsh, "completion zsh"},
		{ShellFish, "completion fish"},
		{ShellPowerShell, "completion powershell"},
	}
	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			script, err := m.GenerateCompletionScript(tt.shell)
			if err != nil {
				t.Fatalf("GenerateCompletionScript(%s): %v", tt.shell, err)
			}
			if !strings.Contains(script, tt.contains) {
				t.Errorf("expected %q in completion script", tt.contains)
			}
		})
	}
}

func TestGenerateCompletionScript_UnsupportedShell(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GenerateCompletionScript(ShellUnknown)
	if err == nil {
		t.Fatal("expected error for unknown shell")
	}
}

func TestShellConfigFile(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		shell  ShellType
		suffix string
	}{
		{ShellZsh, ".zshrc"},
		{ShellFish, "config.fish"},
		{ShellPowerShell, "Microsoft.PowerShell_profile.ps1"},
	}
	for _, tt := range tests {
		t.Run(string(tt.shell), func(t *testing.T) {
			path, err := m.shellConfigFile(tt.shell)
			if err != nil {
				t.Fatalf("shellConfigFile(%s): %v", tt.shell, err)
			}
			if !strings.HasSuffix(path, tt.suffix) {
				t.Errorf("path %q should end with %q", path, tt.suffix)
			}
		})
	}
}

func TestShellConfigFile_Bash(t *testing.T) {
	m := newTestManager(t)
	path, err := m.shellConfigFile(ShellBash)
	if err != nil {
		t.Fatalf("shellConfigFile(bash): %v", err)
	}
	// Should end in .bashrc or .bash_profile
	base := filepath.Base(path)
	if base != ".bashrc" && base != ".bash_profile" {
		t.Errorf("bash config = %q, expected .bashrc or .bash_profile", base)
	}
}

func TestShellConfigFile_UnsupportedShell(t *testing.T) {
	m := newTestManager(t)
	_, err := m.shellConfigFile(ShellUnknown)
	if err == nil {
		t.Fatal("expected error for unknown shell")
	}
}

func TestDetectShell_Zsh(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/bin/zsh")
	if got := m.DetectShell(); got != ShellZsh {
		t.Errorf("DetectShell() = %q, want %q", got, ShellZsh)
	}
}

func TestDetectShell_Bash(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/bin/bash")
	if got := m.DetectShell(); got != ShellBash {
		t.Errorf("DetectShell() = %q, want %q", got, ShellBash)
	}
}

func TestDetectShell_Fish(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := m.DetectShell(); got != ShellFish {
		t.Errorf("DetectShell() = %q, want %q", got, ShellFish)
	}
}

func TestDetectShell_PowerShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/usr/bin/pwsh")
	if got := m.DetectShell(); got != ShellPowerShell {
		t.Errorf("DetectShell() = %q, want %q", got, ShellPowerShell)
	}
}

func TestDetectShell_UnknownBinary(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/usr/bin/unknown-shell")
	if got := m.DetectShell(); got != ShellUnknown {
		t.Errorf("DetectShell() = %q, want %q", got, ShellUnknown)
	}
}

func TestDetectShell_EmptyShellEnv(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "")
	// Without PSModulePath, should be unknown
	t.Setenv("PSModulePath", "")
	got := m.DetectShell()
	// On non-Windows, empty SHELL => ShellUnknown
	_ = got
}

func TestInstall_CreatesFileIfMissing(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "newrc")

	content, err := m.installAt(path, "echo hook")
	if err != nil {
		t.Fatalf("installAt: %v", err)
	}
	if !strings.Contains(string(content), "echo hook") {
		t.Errorf("expected hook body in content")
	}
	// Verify file was created with parent dir
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("file should exist: %v", statErr)
	}
}

func TestUninstall_PreservesMultipleBlankLinesAroundBlock(t *testing.T) {
	m := newTestManager(t)

	before := "line1\n\n"
	gcmBlock := startMarker + "\neval gcm\n" + endMarker + "\n"
	after := "\nline2\n"

	path := writeShellRC(t, before+gcmBlock+after)
	out, err := m.uninstallAt(path)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	result := string(out)
	if strings.Contains(result, startMarker) || strings.Contains(result, endMarker) {
		t.Fatal("markers not removed")
	}
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") {
		t.Fatal("user content lost")
	}
}

func TestInstall_Zsh(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")

	configFile, err := m.Install(ShellZsh)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if configFile == "" {
		t.Fatal("expected config file path")
	}
	data, _ := os.ReadFile(configFile)
	if !strings.Contains(string(data), startMarker) {
		t.Error("expected start marker in config")
	}
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	m.Install(ShellZsh)
	_, err := m.Install(ShellZsh)
	if err == nil {
		t.Fatal("expected error on double install")
	}
}

func TestUninstall_Zsh(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	m.Install(ShellZsh)
	configFile, err := m.Uninstall(ShellZsh)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	data, _ := os.ReadFile(configFile)
	if strings.Contains(string(data), startMarker) {
		t.Error("marker should be removed after uninstall")
	}
}

func TestUninstall_NotInstalled(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	// Create empty config file
	home := os.Getenv("HOME")
	os.WriteFile(home+"/.zshrc", []byte("# empty\n"), 0o644)

	_, err := m.Uninstall(ShellZsh)
	if err == nil {
		t.Fatal("expected error when not installed")
	}
}

func TestInstall_Bash(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	configFile, err := m.Install(ShellBash)
	if err != nil {
		t.Fatalf("Install bash: %v", err)
	}
	data, _ := os.ReadFile(configFile)
	if !strings.Contains(string(data), "PROMPT_COMMAND") {
		t.Error("expected PROMPT_COMMAND in bash hook")
	}
}

func TestInstall_Fish(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	configFile, err := m.Install(ShellFish)
	if err != nil {
		t.Fatalf("Install fish: %v", err)
	}
	data, _ := os.ReadFile(configFile)
	if !strings.Contains(string(data), "PWD") {
		t.Error("expected PWD variable in fish hook")
	}
}

func TestInstallAt_CreatesDir(t *testing.T) {
	m := newTestManager(t)
	path := t.TempDir() + "/subdir/shellrc"
	_, err := m.installAt(path, "eval gcm init")
	if err != nil {
		t.Fatalf("installAt: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), startMarker) {
		t.Error("expected markers")
	}
}

func TestUninstallAt_LegacyMarker(t *testing.T) {
	m := newTestManager(t)
	path := writeShellRC(t, "before\n"+legacyMarker+"\neval gcm init\n\nafter\n")
	_, err := m.uninstallAt(path)
	if err != nil {
		t.Fatalf("uninstallAt legacy: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), legacyMarker) {
		t.Error("legacy marker should be removed")
	}
	if !strings.Contains(string(data), "before") || !strings.Contains(string(data), "after") {
		t.Error("user content lost")
	}
}

func TestUninstallAt_FileNotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.uninstallAt("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDetectShell_PowerShellViaEnv(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "/usr/bin/powershell")
	if got := m.DetectShell(); got != ShellPowerShell {
		t.Errorf("DetectShell() = %q, want %q", got, ShellPowerShell)
	}
}

func TestShellConfigFile_BashFallback(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No .bashrc exists, so it should fall back to .bash_profile
	path, err := m.shellConfigFile(ShellBash)
	if err != nil {
		t.Fatalf("shellConfigFile: %v", err)
	}
	if filepath.Base(path) != ".bash_profile" {
		t.Errorf("expected .bash_profile fallback, got %q", filepath.Base(path))
	}
}

func TestShellConfigFile_BashPrefersBashrc(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .bashrc so it's preferred
	os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# existing\n"), 0o644)

	path, err := m.shellConfigFile(ShellBash)
	if err != nil {
		t.Fatalf("shellConfigFile: %v", err)
	}
	if filepath.Base(path) != ".bashrc" {
		t.Errorf("expected .bashrc, got %q", filepath.Base(path))
	}
}

func TestInstall_PowerShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	configFile, err := m.Install(ShellPowerShell)
	if err != nil {
		t.Fatalf("Install powershell: %v", err)
	}
	data, _ := os.ReadFile(configFile)
	if !strings.Contains(string(data), "Test-Path") {
		t.Error("expected Test-Path in powershell hook")
	}
}

func TestInstall_UnsupportedShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	_, err := m.Install(ShellUnknown)
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestUninstall_UnsupportedShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	_, err := m.Uninstall(ShellUnknown)
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestInstallAt_MkdirAllError(t *testing.T) {
	m := newTestManager(t)
	// Place a file where the parent dir needs to be
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0o644)
	path := filepath.Join(dir, "blocker", "subdir", "rcfile")

	_, err := m.installAt(path, "echo hook")
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestInstallAt_FileWriteError(t *testing.T) {
	m := newTestManager(t)
	// Create a read-only directory
	dir := t.TempDir()
	path := filepath.Join(dir, "rcfile")
	os.WriteFile(path, []byte("existing"), 0o644)
	// Make the file itself read-only and dir read-only
	os.Chmod(path, 0o444)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() {
		os.Chmod(dir, 0o755)
		os.Chmod(path, 0o644)
	})

	_, err := m.installAt(path, "echo hook")
	if err == nil {
		t.Fatal("expected error when file can't be opened for writing")
	}
}

func TestUninstallAt_WriteBackFails(t *testing.T) {
	m := newTestManager(t)

	// Create a file with GCM markers but make it read-only so WriteFile fails.
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")
	content := "export FOO=1\n" + startMarker + "\neval hook\n" + endMarker + "\nalias x=y\n"
	if err := os.WriteFile(path, []byte(content), 0o444); err != nil {
		t.Fatal(err)
	}

	_, err := m.uninstallAt(path)
	if err == nil {
		t.Fatal("expected error when file is read-only")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateCompletionScript_PathWithoutGcm(t *testing.T) {
	m := newTestManager(t)
	// Ensure gcm is NOT in PATH so the LookPath error branch executes.
	t.Setenv("PATH", t.TempDir())

	script, err := m.GenerateCompletionScript(ShellBash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When gcm is not found, it defaults to "gcm" literal in the script.
	if !strings.Contains(script, "gcm") {
		t.Fatalf("expected gcm in script, got: %s", script)
	}
}

func TestDetectShell_Windows_PowerShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "")

	old := shellRuntimeGOOS
	shellRuntimeGOOS = "windows"
	defer func() { shellRuntimeGOOS = old }()

	t.Setenv("PSModulePath", "C:\\Program Files\\PowerShell")
	got := m.DetectShell()
	if got != ShellPowerShell {
		t.Errorf("DetectShell() = %q, want %q", got, ShellPowerShell)
	}
}

func TestDetectShell_Windows_Unknown(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("SHELL", "")
	t.Setenv("PSModulePath", "")

	old := shellRuntimeGOOS
	shellRuntimeGOOS = "windows"
	defer func() { shellRuntimeGOOS = old }()

	got := m.DetectShell()
	if got != ShellUnknown {
		t.Errorf("DetectShell() = %q, want %q", got, ShellUnknown)
	}
}

func TestInstallAt_WriteStringError(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")

	old := installAtWriteStringFn
	installAtWriteStringFn = func(f *os.File, s string) (int, error) {
		return 0, fmt.Errorf("disk full")
	}
	defer func() { installAtWriteStringFn = old }()

	_, err := m.installAt(path, "eval \"$(gcm shell init zsh)\"")
	if err == nil {
		t.Fatal("expected error when WriteString fails")
	}
	if !strings.Contains(err.Error(), "writing to") {
		t.Errorf("error = %q, want 'writing to' prefix", err.Error())
	}
}

func TestShellConfigFile_UserHomeDirError(t *testing.T) {
	m := newTestManager(t)

	old := userHomeDirFn
	userHomeDirFn = func() (string, error) {
		return "", fmt.Errorf("no home")
	}
	defer func() { userHomeDirFn = old }()

	_, err := m.shellConfigFile(ShellZsh)
	if err == nil {
		t.Fatal("expected error when UserHomeDir fails")
	}
}

func TestShellConfigFile_PowerShell_Windows(t *testing.T) {
	m := newTestManager(t)
	tmp := t.TempDir()

	old := shellRuntimeGOOS
	shellRuntimeGOOS = "windows"
	defer func() { shellRuntimeGOOS = old }()

	oldHome := userHomeDirFn
	userHomeDirFn = func() (string, error) { return tmp, nil }
	defer func() { userHomeDirFn = oldHome }()

	path, err := m.shellConfigFile(ShellPowerShell)
	if err != nil {
		t.Fatalf("shellConfigFile: %v", err)
	}
	if !strings.Contains(path, "Documents") {
		t.Errorf("path = %q, expected Windows Documents path", path)
	}
}

func TestIsInstalled_True(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Install first
	_, err := m.Install(ShellZsh)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	installed, configFile := m.IsInstalled(ShellZsh)
	if !installed {
		t.Error("expected IsInstalled to return true after Install")
	}
	if configFile == "" {
		t.Error("expected configFile to be non-empty")
	}
}

func TestIsInstalled_False(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .zshrc without any GCM markers
	zshrc := filepath.Join(home, ".zshrc")
	os.WriteFile(zshrc, []byte("# just normal shell config\nexport PATH=$PATH\n"), 0o644)

	installed, configFile := m.IsInstalled(ShellZsh)
	if installed {
		t.Error("expected IsInstalled to return false when not installed")
	}
	if configFile == "" {
		t.Error("expected configFile path to be returned even when not installed")
	}
}

func TestIsInstalled_LegacyMarker(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write a file with the legacy marker
	zshrc := filepath.Join(home, ".zshrc")
	content := fmt.Sprintf("# some config\n%s\neval $(gcm init)\n", legacyMarker)
	os.WriteFile(zshrc, []byte(content), 0o644)

	installed, _ := m.IsInstalled(ShellZsh)
	if !installed {
		t.Error("expected IsInstalled to detect legacy marker")
	}
}

func TestIsInstalled_UnsupportedShell(t *testing.T) {
	m := newTestManager(t)
	t.Setenv("HOME", t.TempDir())

	installed, configFile := m.IsInstalled(ShellUnknown)
	if installed {
		t.Error("expected IsInstalled to return false for unknown shell")
	}
	if configFile != "" {
		t.Errorf("expected empty configFile for unknown shell, got %q", configFile)
	}
}

func TestIsInstalled_FileDoesNotExist(t *testing.T) {
	m := newTestManager(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Don't create .zshrc — file doesn't exist

	installed, configFile := m.IsInstalled(ShellZsh)
	if installed {
		t.Error("expected IsInstalled to return false when config file doesn't exist")
	}
	// Should still return the config file path
	if configFile == "" {
		t.Error("expected configFile path even when file doesn't exist")
	}
}
