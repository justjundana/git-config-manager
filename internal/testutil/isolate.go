// Package testutil provides helpers that keep the test suite from touching
// real user state.
//
// GCM's job is to rewrite the machine's git identity, credential helpers and
// SSH/GPG keys, so its tests exercise code that writes to $HOME, runs
// "git config --global", and talks to the OS keychain. Without an explicit
// sandbox those writes land on the developer's own machine: profiles get
// relocated into a temp directory the OS later purges, ~/.gitconfig loses its
// credential username, and "git credential approve" stores a fake token in the
// login keychain.
//
// Isolate (per test) and RunIsolated (per package, from TestMain) redirect
// every one of those lookups into a throwaway directory.
package testutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Indirection hooks, overridden in this package's own tests to exercise the
// failure branches. They mirror the pattern used in internal/config and
// internal/tokenstore.
var (
	mkdirAllFn  = os.MkdirAll
	writeFileFn = os.WriteFile
	mkdirTempFn = os.MkdirTemp
	setenvFn    = os.Setenv
	// Method expression rather than a wrapper closure: same signature, and no
	// function body that only ever runs when a sandbox genuinely fails.
	fatalFn           = (*testing.T).Fatalf
	errOut  io.Writer = os.Stderr
)

// isolationEnv returns the environment overrides that redirect all host-state
// lookups into home.
//
// Three git config scopes matter here, not one. HOME only covers the global
// scope; on macOS the Xcode-provided system config at
// /Applications/Xcode.app/Contents/Developer/usr/share/git-core/gitconfig sets
// credential.helper=osxkeychain, which survives a HOME override. A test that
// runs "git credential approve" would therefore still write to the real login
// keychain unless the system scope is disabled as well.
func isolationEnv(home string) map[string]string {
	return map[string]string{
		// config.GCMDir resolves ~/.gcm via os.UserHomeDir, which reads HOME.
		"HOME":            home,
		"USERPROFILE":     home, // Windows equivalent of HOME
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),

		// git config scopes: global, system, and the system opt-out.
		"GIT_CONFIG_GLOBAL":   filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",

		// Never let a git subprocess block on an interactive prompt.
		"GIT_TERMINAL_PROMPT": "0",

		// Hard-refuse OS keychain access; see tokenstore.keychainDisabled.
		"GCM_NO_KEYCHAIN": "1",

		// ssh-add talks to whatever agent SSH_AUTH_SOCK points at, regardless
		// of HOME. Without this, tests that generate a throwaway key and add
		// it to "the agent" load it into the developer's real ssh-agent, where
		// it lingers after the key file itself has been deleted.
		"SSH_AUTH_SOCK": filepath.Join(home, "no-ssh-agent.sock"),

		// gpg resolves its keyring through GNUPGHOME, not HOME, so deleting a
		// "test" key would otherwise hit the real secret keyring.
		"GNUPGHOME": filepath.Join(home, ".gnupg"),
	}
}

// seedHome creates the sandbox layout that git, gpg and GCM expect to find.
func seedHome(home string) error {
	if err := mkdirAllFn(filepath.Join(home, ".config"), 0o755); err != nil {
		return fmt.Errorf("creating XDG config dir: %w", err)
	}
	// gpg refuses to run when GNUPGHOME exists with loose permissions, and
	// creates it with a warning when absent; make it correct up front.
	if err := mkdirAllFn(filepath.Join(home, ".gnupg"), 0o700); err != nil {
		return fmt.Errorf("creating sandbox GNUPGHOME: %w", err)
	}
	// git errors out on --get against a non-existent global config file, so
	// create it empty rather than letting every read fail.
	if err := writeFileFn(filepath.Join(home, ".gitconfig"), nil, 0o600); err != nil {
		return fmt.Errorf("seeding gitconfig: %w", err)
	}
	return nil
}

// Isolate points HOME, the git config scopes, the OS keychain, the ssh-agent
// and the GPG keyring at a throwaway directory for the duration of t, and
// returns that directory. Every override is restored automatically when t
// finishes.
//
// Isolate uses t.Setenv, so it cannot be called from a parallel test.
func Isolate(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	for key, value := range isolationEnv(home) {
		t.Setenv(key, value)
	}
	if err := seedHome(home); err != nil {
		fatalFn(t, "testutil.Isolate: %v", err)
	}
	return home
}

// RunIsolated applies the same redirection to the whole test binary and then
// runs m. Call it from TestMain so every test in the package is sandboxed by
// default — including tests added later that forget to call Isolate:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunIsolated(m)) }
//
// Individual tests may still call Isolate to get their own directory; the
// process-wide sandbox is what guarantees that forgetting to do so is safe.
func RunIsolated(m *testing.M) int {
	home, err := mkdirTempFn("", "gcm-test-home-")
	if err != nil {
		fmt.Fprintf(errOut, "testutil.RunIsolated: creating sandbox home: %v\n", err)
		return 1
	}
	defer os.RemoveAll(home)

	// Registered before the first Setenv so a failure part-way through the
	// loop still restores whatever was already overwritten.
	restore := make(map[string]*string)
	defer func() {
		for key, prev := range restore {
			if prev == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *prev)
		}
	}()

	for key, value := range isolationEnv(home) {
		if prev, ok := os.LookupEnv(key); ok {
			saved := prev
			restore[key] = &saved
		} else {
			restore[key] = nil
		}
		if err := setenvFn(key, value); err != nil {
			fmt.Fprintf(errOut, "testutil.RunIsolated: setting %s: %v\n", key, err)
			return 1
		}
	}

	if err := seedHome(home); err != nil {
		fmt.Fprintf(errOut, "testutil.RunIsolated: %v\n", err)
		return 1
	}

	return m.Run()
}
