// feat: refactor main function for better exit handling and add comprehensive tests

package main

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/ui"
)

// mockDeps replaces configLoad and configEnsureDirs with no-op stubs that
// return the given cfg. Returns a restore function to call in defer.
func mockDeps(t *testing.T, cfg *config.Config) func() {
	t.Helper()
	origLoad := configLoad
	origEnsure := configEnsureDirs
	configLoad = func() (*config.Config, error) { return cfg, nil }
	configEnsureDirs = func(*config.Config) error { return nil }
	return func() {
		configLoad = origLoad
		configEnsureDirs = origEnsure
	}
}

func TestMain_CallsOsExit(t *testing.T) {
	restore := mockDeps(t, config.DefaultConfig())
	defer restore()

	orig := osExit
	var captured int
	osExit = func(code int) { captured = code }
	defer func() { osExit = orig }()

	origArgs := os.Args
	os.Args = []string{"gcm", "version"}
	defer func() { os.Args = origArgs }()

	main()

	if captured != 0 {
		t.Errorf("main() called osExit(%d), want 0", captured)
	}
}

func TestRun_Success(t *testing.T) {
	restore := mockDeps(t, config.DefaultConfig())
	defer restore()

	if code := run([]string{"version"}); code != 0 {
		t.Errorf("run(version) = %d, want 0", code)
	}
}

func TestRun_UIFlagsFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UI.Color = false  // triggers ui.DisableColor()
	cfg.UI.Verbose = true // triggers log.SetVerbose(true)
	cfg.UI.Quiet = true   // triggers log.SetQuiet(true)
	restore := mockDeps(t, cfg)
	defer restore()

	if code := run([]string{"version"}); code != 0 {
		t.Errorf("run with UI config flags = %d, want 0", code)
	}
}

func TestRun_PersistentPreRun(t *testing.T) {
	restore := mockDeps(t, config.DefaultConfig())
	defer restore()

	// These flags exercise the PersistentPreRun closure
	code := run([]string{"version", "--verbose", "--no-color", "--quiet"})
	if code != 0 {
		t.Errorf("run(version --verbose --no-color --quiet) = %d, want 0", code)
	}
}

func TestRun_ConfigLoadError(t *testing.T) {
	orig := configLoad
	configLoad = func() (*config.Config, error) { return nil, errors.New("load error") }
	defer func() { configLoad = orig }()

	if code := run(nil); code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRun_EnsureDirsError(t *testing.T) {
	origLoad := configLoad
	configLoad = func() (*config.Config, error) { return config.DefaultConfig(), nil }
	defer func() { configLoad = origLoad }()

	origEnsure := configEnsureDirs
	configEnsureDirs = func(*config.Config) error { return errors.New("dirs error") }
	defer func() { configEnsureDirs = origEnsure }()

	if code := run(nil); code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}

func TestRun_ExecuteError(t *testing.T) {
	restore := mockDeps(t, config.DefaultConfig())
	defer restore()

	// An unknown flag causes cobra to return an error (exit code 1)
	if code := run([]string{"--unknown-flag-xyz"}); code != 1 {
		t.Errorf("expected exit code 1 for unknown flag, got %d", code)
	}
}

func TestMasterPasswordPrompt_DefaultIsCallable(t *testing.T) {
	// Feed input via PromptIn so the default prompt reads without blocking.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origIn := ui.PromptIn
	ui.PromptIn = r
	defer func() { ui.PromptIn = origIn; r.Close() }()

	fmt.Fprintln(w, "testpassword")
	w.Close()

	// Call the actual default function (not the test-replaced one).
	orig := masterPasswordPrompt
	pw, err := orig("Enter password:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pw != "testpassword" {
		t.Errorf("got %q, want %q", pw, "testpassword")
	}
}
