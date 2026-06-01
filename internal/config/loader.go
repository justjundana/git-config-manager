package config

import (
	"fmt"
	"os"
	"path/filepath"

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

// validateConfigPaths is a safety check that prevents saving config with
// obviously incorrect git_command values (e.g. paths to non-existent files).
// This catches scenarios where test data accidentally leaks into production config.
func validateConfigPaths(cfg *Config, _ string) error {
	gitCmd := cfg.Advanced.GitCommand
	if gitCmd == "" || gitCmd == "git" {
		return nil
	}
	// If git_command is an absolute path, verify it exists.
	if filepath.IsAbs(gitCmd) {
		if _, err := os.Stat(gitCmd); err != nil {
			return fmt.Errorf("refusing to save: git_command %q does not exist", gitCmd)
		}
	}
	return nil
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
