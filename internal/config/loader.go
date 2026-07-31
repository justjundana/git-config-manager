package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Test hooks for deterministic error-path testing.
var (
	mkdirAllFn    = os.MkdirAll
	chmodPathFn   = os.Chmod
	yamlMarshalFn = yaml.Marshal
	createTempFn  = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
	statFn        = os.Stat
	removeFn      = os.Remove
	renameFn      = os.Rename
	configPathFn  = func() string { return filepath.Join(GCMDir(), "config.yaml") }
	tempDirFn     = os.TempDir
)

type tempFile interface {
	Name() string
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// Load reads the GCM configuration from disk.
// If no config file exists, it returns defaults.
func Load() (*Config, error) {
	cfg := DefaultConfig()
	configPath := ConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to disk atomically. A temp file + rename
// pattern prevents corruption if the process is interrupted mid-write.
func Save(cfg *Config) error {
	configPath := ConfigPath()

	// Guard: refuse to save config that contains temp/test paths to the real
	// user config. This prevents test runs from corrupting production data.
	if err := validateConfigPaths(cfg, configPath); err != nil {
		return err
	}

	dir := filepath.Dir(configPath)
	if err := mkdirAllFn(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yamlMarshalFn(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	tmp, err := createTempFn(dir, ".gcm-config-*")
	if err != nil {
		return fmt.Errorf("creating temp config file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if _, statErr := statFn(tmpPath); statErr == nil {
			_ = removeFn(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing config file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting config permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing config file: %w", err)
	}

	if err := renameFn(tmpPath, configPath); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// ConfigPath returns the full path to the config file.
func ConfigPath() string {
	return configPathFn()
}

// SetConfigPathForTesting overrides ConfigPath to return the given path.
// It returns a restore function that must be deferred by the caller.
func SetConfigPathForTesting(path string) func() {
	orig := configPathFn
	configPathFn = func() string { return path }
	return func() { configPathFn = orig }
}

// validateConfigPaths is a safety check that prevents test data from leaking
// into the production config. It rejects obviously incorrect git_command
// values, and — more importantly — refuses to relocate GCM's data
// directories into the OS temporary area.
//
// The second check exists because of a real failure mode: a Config built by a
// test points ProfilesDir at t.TempDir(), and if that Config reaches Save()
// while configPath still resolves to the user's real ~/.gcm/config.yaml, every
// profile created from then on is written into a directory the OS purges
// periodically. To the user the profiles simply vanish, with no delete having
// ever been issued.
//
// Saves whose destination is itself inside the temp area are left alone: that
// is a properly sandboxed test, not a leak.
func validateConfigPaths(cfg *Config, configPath string) error {
	gitCmd := cfg.Advanced.GitCommand
	if gitCmd != "" && gitCmd != "git" && filepath.IsAbs(gitCmd) {
		// If git_command is an absolute path, verify it exists.
		if _, err := os.Stat(gitCmd); err != nil {
			return fmt.Errorf("refusing to save: git_command %q does not exist", gitCmd)
		}
	}

	temps := tempDirCandidates()
	if isUnderAnyDir(configPath, temps) {
		return nil
	}

	for _, dir := range []struct {
		field string
		path  string
	}{
		{"profiles_dir", cfg.ProfilesDir},
		{"templates_dir", cfg.TemplatesDir},
		{"cache_dir", cfg.CacheDir},
	} {
		if isUnderAnyDir(dir.path, temps) {
			return fmt.Errorf(
				"refusing to save %s: %s points into the temporary directory (%q), "+
					"which the operating system deletes periodically",
				configPath, dir.field, dir.path)
		}
	}

	return nil
}

// EnsureRemovable reports whether path may be recursively deleted by GCM.
//
// "gcm clean" runs os.RemoveAll on directories taken straight from the config
// file, and that file is user-editable — and, as this project has learned,
// corruptible. Without a check, a cache_dir of "/" or "$HOME" turns a cache
// purge into wiping the home directory.
//
// The rule is deliberately narrow: it blocks the catastrophic targets (empty,
// relative, filesystem root, the home directory, or any ancestor of the home
// or GCM data directory) while still allowing a cache directory the user has
// legitimately relocated elsewhere.
func EnsureRemovable(path string) error {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return fmt.Errorf("refusing to delete: no path configured")
	}
	if !filepath.IsAbs(trimmed) {
		return fmt.Errorf("refusing to delete %q: path is not absolute", path)
	}

	abs := filepath.Clean(trimmed)
	if filepath.Dir(abs) == abs {
		return fmt.Errorf("refusing to delete %q: that is the filesystem root", abs)
	}

	// Compare through resolved forms as well: on macOS the temp and home trees
	// reach the same directory via /var and /private/var, and a raw string
	// comparison would miss the match.
	candidates := pathCandidates(abs)

	// Derive both checks from one home lookup. GCMDir terminates the process
	// when the home directory is unknown, and a validation helper must never
	// do that — with no home there is simply nothing further to protect.
	home, err := userHomeDirFn()
	if err != nil || home == "" {
		return nil
	}

	if samePath(abs, home) {
		return fmt.Errorf("refusing to delete %q: that is the home directory", abs)
	}
	if isUnderAnyDir(home, candidates) {
		return fmt.Errorf("refusing to delete %q: it contains the home directory", abs)
	}

	// Only equality is checked here: the data directory lives inside the home
	// directory, so anything that *contains* it also contains home and was
	// already rejected above.
	if samePath(abs, filepath.Join(home, ".gcm")) {
		return fmt.Errorf("refusing to delete %q: that is the GCM data directory", abs)
	}

	return nil
}

// pathCandidates returns p cleaned, plus its symlink-resolved form when that
// differs and the path exists.
func pathCandidates(p string) []string {
	out := []string{filepath.Clean(p)}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		if cleaned := filepath.Clean(resolved); cleaned != out[0] {
			out = append(out, cleaned)
		}
	}
	return out
}

// samePath reports whether a and b denote the same location, comparing both
// their literal and symlink-resolved forms.
func samePath(a, b string) bool {
	for _, x := range pathCandidates(a) {
		for _, y := range pathCandidates(b) {
			if x == y {
				return true
			}
		}
	}
	return false
}

// tempDirCandidates returns the OS temp directory both as reported and with
// symlinks resolved. On macOS TMPDIR is /var/folders/..., which resolves to
// /private/var/folders/...; a config may record either spelling, so both are
// needed for a reliable containment test.
func tempDirCandidates() []string {
	dir := tempDirFn()
	if dir == "" {
		return nil
	}
	dirs := []string{filepath.Clean(dir)}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		if cleaned := filepath.Clean(resolved); cleaned != dirs[0] {
			dirs = append(dirs, cleaned)
		}
	}
	return dirs
}

// isUnderAnyDir reports whether path lies inside any of dirs.
func isUnderAnyDir(path string, dirs []string) bool {
	if path == "" {
		return false
	}
	// The path may no longer exist (a purged temp dir is exactly the case we
	// care about), so fall back to lexical resolution when EvalSymlinks fails.
	resolved := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolved = r
	} else if abs, err := filepath.Abs(path); err == nil {
		resolved = abs
	}
	resolved = filepath.Clean(resolved)

	for _, dir := range dirs {
		rel, err := filepath.Rel(dir, resolved)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// EnsureDirs creates all required GCM directories.
//
// Directories holding secrets (tokens, audit logs, backups) are created with
// 0700 so only the owner can access them. The main data and cache directories
// use 0755.
func EnsureDirs(cfg *Config) error {
	// path + permissions. Ordered so parent directories are created before
	// children (important when ~/.gcm doesn't exist yet).
	type dirEntry struct {
		path string
		perm os.FileMode
	}
	dirs := []dirEntry{
		{GCMDir(), 0o755},
		{cfg.ProfilesDir, 0o755},
		{cfg.TemplatesDir, 0o755},
		{cfg.CacheDir, 0o755},
		{filepath.Join(GCMDir(), "tokens"), 0o700},  // contains encrypted tokens + keys
		{filepath.Join(GCMDir(), "backups"), 0o700}, // may contain sensitive config
		{filepath.Join(GCMDir(), "logs"), 0o700},    // audit trail
	}

	for _, d := range dirs {
		if err := mkdirAllFn(d.path, d.perm); err != nil {
			return fmt.Errorf("creating directory %s: %w", d.path, err)
		}
		// os.MkdirAll respects the umask and skips pre-existing directories,
		// so explicitly tighten permissions for sensitive locations.
		if d.perm == 0o700 {
			if err := chmodPathFn(d.path, d.perm); err != nil {
				return fmt.Errorf("tightening permissions on %s: %w", d.path, err)
			}
		}
	}

	return nil
}
