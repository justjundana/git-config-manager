package profile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// defaultGitTimeout bounds external git command invocations to prevent hangs
// (e.g. a credential helper waiting for input).
const defaultGitTimeout = 30 * time.Second

// Test hooks for unreachable OS/IO error paths.
var (
	configSaveFn  = config.Save
	swGetwdFn     = os.Getwd
	swWriteFileFn = os.WriteFile
	swReadFileFn  = os.ReadFile
)

// Switcher handles profile activation across different scopes.
type Switcher struct {
	cfg     *config.Config
	manager *Manager
	log     *logger.Logger
}

// NewSwitcher creates a new profile switcher.
func NewSwitcher(cfg *config.Config, manager *Manager, log *logger.Logger) *Switcher {
	return &Switcher{
		cfg:     cfg,
		manager: manager,
		log:     log,
	}
}

// Activate applies a profile at the given scope.
func (s *Switcher) Activate(name string, scope ActivationScope) error {
	p, err := s.manager.Get(name)
	if err != nil {
		return err
	}

	switch scope {
	case ScopeGlobal:
		if err := s.activateGlobal(p); err != nil {
			return fmt.Errorf("activating global: %w", err)
		}
		s.cfg.DefaultProfile = name
		if err := configSaveFn(s.cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
	case ScopeLocal:
		if err := s.activateLocal(p); err != nil {
			return fmt.Errorf("activating local: %w", err)
		}
	case ScopeSession:
		if err := s.activateSession(p); err != nil {
			return fmt.Errorf("activating session: %w", err)
		}
	}

	if err := s.manager.IncrementUsage(name); err != nil {
		s.log.Warn("Failed to increment usage", logger.F("error", err))
	}

	s.log.Debug("Profile activated",
		logger.F("profile", name),
		logger.F("scope", scope.String()))
	return nil
}

// Current returns the currently active profile info.
// Priority: session > local > global (session is an explicit user action via
// "gcm use" and should override directory-level pinning).
func (s *Switcher) Current() (name string, scope ActivationScope, err error) {
	// Check session first: the direct marker file (.git/gcm-session) is written
	// by "gcm use <profile>" and represents the user's most recent intent.
	name = s.readSessionMarker()
	if name != "" && s.manager.Exists(name) {
		return name, ScopeSession, nil
	}
	name = s.detectSessionProfile()
	if name != "" {
		return name, ScopeSession, nil
	}

	// Check local (.gcm-profile in current directory)
	cwd, err := os.Getwd()
	if err == nil {
		profileFile := filepath.Join(cwd, s.cfg.AutoSwitch.ProjectFile)
		data, readErr := os.ReadFile(profileFile)
		if readErr == nil {
			name = strings.TrimSpace(string(data))
			if name != "" && s.manager.Exists(name) {
				return name, ScopeLocal, nil
			}
		}
	}

	// Check global default
	if s.cfg.DefaultProfile != "" && s.manager.Exists(s.cfg.DefaultProfile) {
		return s.cfg.DefaultProfile, ScopeGlobal, nil
	}

	return "", ScopeSession, fmt.Errorf("no active profile")
}

// readSessionMarker reads the profile name from .git/gcm-session if it exists.
func (s *Switcher) readSessionMarker() string {
	gitDir := s.findGitDir()
	if gitDir == "" {
		return ""
	}
	data, err := swReadFileFn(filepath.Join(gitDir, "gcm-session"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// detectSessionProfile checks the local git config for a user.email that matches a known profile.
func (s *Switcher) detectSessionProfile() string {
	gitCmd := s.cfg.Advanced.GitCommand
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitCmd, "config", "--local", "user.email")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	email := strings.TrimSpace(string(out))
	if email == "" {
		return ""
	}

	profiles, err := s.manager.List()
	if err != nil {
		return ""
	}
	for _, p := range profiles {
		if p.Git.User.Email == email {
			return p.Name
		}
	}
	return ""
}

// Refresh re-evaluates the current directory and activates the appropriate profile.
func (s *Switcher) Refresh() error {
	name, _, err := s.Current()
	if err != nil {
		return nil // No profile to activate, not an error
	}

	return s.Activate(name, ScopeLocal)
}

// ClearGlobalIdentity unsets git global user.name, user.email, and
// user.signingkey so that no identity is configured. This ensures git will
// refuse to commit until a profile is activated through GCM.
func (s *Switcher) ClearGlobalIdentity() error {
	gitCmd := s.cfg.Advanced.GitCommand

	for _, key := range identityGitConfigKeys {
		ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
		cmd := exec.CommandContext(ctx, gitCmd, "config", "--global", "--unset-all", key)
		_ = cmd.Run() // ignore errors (key may not exist)
		cancel()
	}
	return nil
}

// ClearLocalIdentity removes profile identity keys from the current git repo.
// Outside a git repository it is a no-op so project markers can still be used
// in directories that are not repositories yet.
func (s *Switcher) ClearLocalIdentity() error {
	cwd, err := swGetwdFn()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}
	gitDir, err := s.findGitDirInCwd(cwd)
	if err != nil {
		return err
	}
	if gitDir == "" {
		return nil
	}

	gitCmd := s.cfg.Advanced.GitCommand
	for _, key := range identityGitConfigKeys {
		ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
		cmd := exec.CommandContext(ctx, gitCmd, "config", "--local", "--unset-all", key)
		cmd.Dir = cwd
		_ = cmd.Run() // ignore errors (key may not exist)
		cancel()
	}
	return nil
}

// Deactivate removes git config and markers for a profile that is being deleted.
// It detects the scope where the profile is active and clears accordingly.
func (s *Switcher) Deactivate(name string) {
	current, scope, err := s.Current()
	if err != nil || current != name {
		return // profile is not active, nothing to do
	}

	gitCmd := s.cfg.Advanced.GitCommand

	switch scope {
	case ScopeGlobal:
		for _, key := range identityGitConfigKeys {
			ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
			cmd := exec.CommandContext(ctx, gitCmd, "config", "--global", "--unset-all", key)
			_ = cmd.Run()
			cancel()
		}
		// Clear default profile reference
		s.cfg.DefaultProfile = ""

	case ScopeLocal:
		for _, key := range identityGitConfigKeys {
			ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
			cmd := exec.CommandContext(ctx, gitCmd, "config", "--local", "--unset-all", key)
			_ = cmd.Run()
			cancel()
		}
		// Remove .gcm-profile marker
		cwd, cwdErr := swGetwdFn()
		if cwdErr == nil {
			os.Remove(filepath.Join(cwd, s.cfg.AutoSwitch.ProjectFile))
		}

	case ScopeSession:
		for _, key := range identityGitConfigKeys {
			ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
			cmd := exec.CommandContext(ctx, gitCmd, "config", "--local", "--unset-all", key)
			_ = cmd.Run()
			cancel()
		}
		// Remove session marker
		s.clearSessionMarker()
	}
}

func (s *Switcher) activateGlobal(p *Profile) error {
	return s.applyGitConfig(p, "--global")
}

func (s *Switcher) activateLocal(p *Profile) error {
	cwd, err := swGetwdFn()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	// Apply to local git config. Critical fields (user.name, user.email) must
	// succeed in git repositories; outside a repo, the .gcm-profile marker is
	// still useful for shell integration and future git repos.
	gitDir, err := s.findGitDirInCwd(cwd)
	if err != nil {
		return err
	}
	if gitDir != "" {
		if err := s.applyGitConfig(p, "--local"); err != nil {
			return fmt.Errorf("applying git config: %w", err)
		}
	}

	// Write .gcm-profile marker file
	profileFile := filepath.Join(cwd, s.cfg.AutoSwitch.ProjectFile)
	if err := swWriteFileFn(profileFile, []byte(p.Name+"\n"), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", s.cfg.AutoSwitch.ProjectFile, err)
	}

	return nil
}

func (s *Switcher) activateSession(p *Profile) error {
	// Write a session marker inside .git/ so detection doesn't depend on
	// git-config plumbing (which can silently fail on some git versions).
	if err := s.writeSessionMarker(p.Name); err != nil {
		return fmt.Errorf("writing session marker: %w", err)
	}

	// Apply git config --local so git commands use the correct identity.
	// Critical fields (user.name, user.email) must succeed; if they fail the
	// user's commits would silently use the wrong identity.
	if err := s.applyGitConfig(p, "--local"); err != nil {
		return fmt.Errorf("applying git config: %w", err)
	}
	return nil
}

// writeSessionMarker writes the profile name into .git/gcm-session.
func (s *Switcher) writeSessionMarker(name string) error {
	gitDir := s.findGitDir()
	if gitDir == "" {
		return fmt.Errorf("not in a git repository")
	}
	markerPath := filepath.Join(gitDir, "gcm-session")
	return swWriteFileFn(markerPath, []byte(name+"\n"), 0644)
}

// clearSessionMarker removes the .git/gcm-session file.
func (s *Switcher) clearSessionMarker() {
	gitDir := s.findGitDir()
	if gitDir == "" {
		return
	}
	os.Remove(filepath.Join(gitDir, "gcm-session"))
}

// ClearSession removes the session marker if present.
func (s *Switcher) ClearSession() {
	s.clearSessionMarker()
}

// findGitDir returns the path to the .git directory for the current working directory.
func (s *Switcher) findGitDir() string {
	cwd, err := swGetwdFn()
	if err != nil {
		return ""
	}
	dir, err := s.findGitDirInCwd(cwd)
	if err != nil {
		return ""
	}
	return dir
}

func (s *Switcher) findGitDirInCwd(cwd string) (string, error) {
	gitCmd := s.cfg.Advanced.GitCommand
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitCmd, "rev-parse", "--git-dir")
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if strings.Contains(output, "not a git repository") {
			return "", nil
		}
		return "", fmt.Errorf("detecting git repository: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", nil
	}
	// git rev-parse --git-dir may return a relative path
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	return dir, nil
}

// criticalGitConfigKeys are the fields that MUST be applied successfully for
// a profile activation to be meaningful. If these fail, the user's commits
// will use the wrong identity — a silent correctness bug.
var criticalGitConfigKeys = map[string]bool{
	"user.name":  true,
	"user.email": true,
}

var identityGitConfigKeys = []string{"user.name", "user.email", "user.signingkey", "commit.gpgsign"}

func (s *Switcher) applyGitConfig(p *Profile, scope string) error {
	gitCmd := s.cfg.Advanced.GitCommand

	// Core identity
	configs := map[string]string{
		"user.name":  p.Git.User.Name,
		"user.email": p.Git.User.Email,
	}

	if p.Git.User.SigningKey != "" {
		configs["user.signingkey"] = p.Git.User.SigningKey
	}

	// Core settings
	if p.Git.Core.Editor != "" {
		configs["core.editor"] = p.Git.Core.Editor
	}
	if p.Git.Core.AutoCRLF != "" {
		configs["core.autocrlf"] = p.Git.Core.AutoCRLF
	}
	if p.Git.Core.EOL != "" {
		configs["core.eol"] = p.Git.Core.EOL
	}

	// Commit settings
	if p.Git.Commit.GPGSign != nil {
		configs["commit.gpgsign"] = fmt.Sprintf("%t", *p.Git.Commit.GPGSign)
	}
	if p.Git.Commit.Template != "" {
		configs["commit.template"] = p.Git.Commit.Template
	}

	// Pull settings
	if p.Git.Pull.Rebase != "" {
		configs["pull.rebase"] = p.Git.Pull.Rebase
	}
	if p.Git.Pull.FF != "" {
		configs["pull.ff"] = p.Git.Pull.FF
	}

	// Push settings
	if p.Git.Push.Default != "" {
		configs["push.default"] = p.Git.Push.Default
	}
	if p.Git.Push.FollowTags != nil {
		configs["push.followTags"] = fmt.Sprintf("%t", *p.Git.Push.FollowTags)
	}
	if p.Git.Push.AutoSetupRemote != nil {
		configs["push.autoSetupRemote"] = fmt.Sprintf("%t", *p.Git.Push.AutoSetupRemote)
	}

	// Aliases
	for alias, cmd := range p.Git.Aliases {
		configs[fmt.Sprintf("alias.%s", alias)] = cmd
	}

	// Custom settings — validate at activation time as a defense-in-depth
	// measure. Profiles loaded from disk (e.g., after a backup restore or
	// manual YAML edit) may contain dangerous keys that weren't checked at
	// creation time.
	if err := validateCustomKeys(p.Git.Custom); err != nil {
		return fmt.Errorf("profile %q contains unsafe custom git config: %w", p.Name, err)
	}
	for key, val := range p.Git.Custom {
		configs[key] = val
	}

	// Apply all settings in sorted key order for deterministic behavior
	keys := make([]string, 0, len(configs))
	for k := range configs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var criticalErrors []string

	for _, key := range keys {
		val := configs[key]
		args := []string{"config", scope, key, val}
		ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
		cmd := exec.CommandContext(ctx, gitCmd, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			cancel()
			output := strings.TrimSpace(string(out))

			if criticalGitConfigKeys[key] {
				// Critical field failed — this will cause wrong identity on commits.
				s.log.Warn("Failed to set git config (critical)",
					logger.F("key", key),
					logger.F("scope", scope),
					logger.F("output", output))
				criticalErrors = append(criticalErrors, fmt.Sprintf("%s: %v", key, err))
			} else if scope == "--local" {
				// Non-critical field in local scope — log and continue.
				s.log.Debug("git config failed (non-critical, skipping)",
					logger.F("key", key),
					logger.F("output", output))
				continue
			} else {
				return fmt.Errorf("setting %s: %w", key, err)
			}
		}
		cancel()
	}

	if len(criticalErrors) > 0 {
		return fmt.Errorf("failed to apply critical git config: %s", strings.Join(criticalErrors, "; "))
	}

	// Load SSH key to agent if configured
	if p.SSH != nil && p.SSH.KeyPath != "" {
		expanded := expandPath(p.SSH.KeyPath)
		if _, err := os.Stat(expanded); err == nil {
			loadToAgent := p.SSH.LoadToAgent == nil || *p.SSH.LoadToAgent
			if loadToAgent {
				ctx, cancel := context.WithTimeout(context.Background(), defaultGitTimeout)
				defer cancel()
				cmd := exec.CommandContext(ctx, "ssh-add", expanded)
				if out, err := cmd.CombinedOutput(); err != nil {
					s.log.Warn("Failed to load SSH key to agent",
						logger.F("key", p.SSH.KeyPath),
						logger.F("error", strings.TrimSpace(string(out))))
				}
			}
		}
	}

	return nil
}
