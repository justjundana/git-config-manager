package testutil

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain sandboxes this package's own tests, exactly as it does for every
// other package. The helpers under test then nest a second sandbox inside it.
func TestMain(m *testing.M) {
	os.Exit(RunIsolated(m))
}

var errInjected = errors.New("injected failure")

// swapHooks restores every indirection hook when the test ends, so a forced
// failure in one test cannot leak into the next.
func swapHooks(t *testing.T) {
	t.Helper()
	mkdirAll, writeFile, mkdirTemp, setenv, fatal, out := mkdirAllFn, writeFileFn, mkdirTempFn, setenvFn, fatalFn, errOut
	t.Cleanup(func() {
		mkdirAllFn, writeFileFn, mkdirTempFn, setenvFn, fatalFn, errOut = mkdirAll, writeFile, mkdirTemp, setenv, fatal, out
	})
}

// captureStderr redirects the package's error output and returns a func that
// yields what was written.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	buf := &bytes.Buffer{}
	errOut = buf
	return buf.String
}

func TestIsolationEnv_RedirectsEveryChannelIntoHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "sandbox")
	env := isolationEnv(home)

	// Values that must point somewhere inside the sandbox.
	inHome := map[string]string{
		"HOME":              home,
		"USERPROFILE":       home,
		"XDG_CONFIG_HOME":   filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL": filepath.Join(home, ".gitconfig"),
		"SSH_AUTH_SOCK":     filepath.Join(home, "no-ssh-agent.sock"),
		"GNUPGHOME":         filepath.Join(home, ".gnupg"),
	}
	for key, want := range inHome {
		if got := env[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	// Values that switch a capability off outright.
	switches := map[string]string{
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
		"GCM_NO_KEYCHAIN":     "1",
	}
	for key, want := range switches {
		if got := env[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestSeedHome_CreatesGitAndGpgLayout(t *testing.T) {
	home := t.TempDir()
	if err := seedHome(home); err != nil {
		t.Fatalf("seedHome: %v", err)
	}

	// git errors out reading a global config that does not exist.
	fi, err := os.Stat(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("stat .gitconfig: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf(".gitconfig perm = %o, want 0600", perm)
	}

	// gpg refuses to run when GNUPGHOME has loose permissions.
	fi, err = os.Stat(filepath.Join(home, ".gnupg"))
	if err != nil {
		t.Fatalf("stat .gnupg: %v", err)
	}
	if !fi.IsDir() {
		t.Error(".gnupg should be a directory")
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf(".gnupg perm = %o, want 0700", perm)
	}

	if _, err := os.Stat(filepath.Join(home, ".config")); err != nil {
		t.Errorf("stat .config: %v", err)
	}
}

func TestSeedHome_ReportsWhichStepFailed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func()
		want    string
	}{
		{
			name: "xdg config dir",
			arrange: func() {
				mkdirAllFn = func(string, os.FileMode) error { return errInjected }
			},
			want: "creating XDG config dir",
		},
		{
			name: "gnupg home",
			arrange: func() {
				mkdirAllFn = func(path string, perm os.FileMode) error {
					if filepath.Base(path) == ".gnupg" {
						return errInjected
					}
					return os.MkdirAll(path, perm)
				}
			},
			want: "creating sandbox GNUPGHOME",
		},
		{
			name: "gitconfig",
			arrange: func() {
				writeFileFn = func(string, []byte, os.FileMode) error { return errInjected }
			},
			want: "seeding gitconfig",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			swapHooks(t)
			tc.arrange()

			err := seedHome(t.TempDir())
			if err == nil {
				t.Fatalf("expected %s failure to surface", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			if !errors.Is(err, errInjected) {
				t.Errorf("error should wrap the cause, got %v", err)
			}
		})
	}
}

func TestIsolate_RedirectsUserHomeDir(t *testing.T) {
	home := Isolate(t)

	got, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if got != home {
		t.Errorf("UserHomeDir() = %q, want sandbox %q", got, home)
	}
}

func TestIsolate_RestoresEnvironmentAfterTest(t *testing.T) {
	outer := os.Getenv("HOME")

	var sandbox string
	t.Run("inside", func(t *testing.T) {
		sandbox = Isolate(t)
		if got := os.Getenv("HOME"); got != sandbox {
			t.Fatalf("HOME inside sandbox = %q, want %q", got, sandbox)
		}
	})

	if sandbox == outer {
		t.Error("sandbox HOME should differ from the surrounding HOME")
	}
	if got := os.Getenv("HOME"); got != outer {
		t.Errorf("HOME after subtest = %q, want it restored to %q", got, outer)
	}
}

func TestIsolate_FailsTheTestWhenSeedingFails(t *testing.T) {
	swapHooks(t)

	var gotFormat string
	var gotArgs []any
	fatalFn = func(_ *testing.T, format string, args ...any) {
		gotFormat, gotArgs = format, args
	}
	writeFileFn = func(string, []byte, os.FileMode) error { return errInjected }

	Isolate(t)

	if gotFormat == "" {
		t.Fatal("Isolate should have reported a fatal error")
	}
	if !strings.Contains(gotFormat, "testutil.Isolate") {
		t.Errorf("message = %q, want it to name the helper", gotFormat)
	}
	if len(gotArgs) != 1 {
		t.Fatalf("expected the cause to be passed, got %v", gotArgs)
	}
	if err, ok := gotArgs[0].(error); !ok || !errors.Is(err, errInjected) {
		t.Errorf("cause = %v, want the injected failure", gotArgs[0])
	}
}

// The property that actually matters: "git config --global" must land in the
// sandbox and leave the surrounding gitconfig untouched. This is the exact
// write that used to delete the developer's real
// credential.https://github.com.username.
func TestIsolate_GitGlobalWritesStayInSandbox(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	outerConfig := os.Getenv("GIT_CONFIG_GLOBAL")
	if outerConfig == "" {
		t.Skip("no surrounding GIT_CONFIG_GLOBAL to compare against")
	}
	before, err := os.ReadFile(outerConfig)
	if err != nil {
		t.Fatalf("reading surrounding gitconfig: %v", err)
	}

	home := Isolate(t)

	out, err := exec.Command("git", "config", "--global", "gcm.sandboxprobe", "written").CombinedOutput()
	if err != nil {
		t.Fatalf("git config --global: %v: %s", err, out)
	}

	written, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		t.Fatalf("reading sandbox gitconfig: %v", err)
	}
	if !strings.Contains(string(written), "sandboxprobe") {
		t.Errorf("write did not land in the sandbox gitconfig, got:\n%s", written)
	}

	after, err := os.ReadFile(outerConfig)
	if err != nil {
		t.Fatalf("re-reading surrounding gitconfig: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("surrounding gitconfig was modified:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// RunIsolated returns before touching m on every failure path, so a nil
// *testing.M is safe here and keeps the test from recursing into the suite.
func TestRunIsolated_FailurePaths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func()
		want    string
	}{
		{
			name: "sandbox home cannot be created",
			arrange: func() {
				mkdirTempFn = func(string, string) (string, error) { return "", errInjected }
			},
			want: "creating sandbox home",
		},
		{
			name: "environment cannot be set",
			arrange: func() {
				setenvFn = func(string, string) error { return errInjected }
			},
			want: "setting ",
		},
		{
			name: "sandbox home cannot be seeded",
			arrange: func() {
				writeFileFn = func(string, []byte, os.FileMode) error { return errInjected }
			},
			want: "seeding gitconfig",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			swapHooks(t)
			stderr := captureStderr(t)
			tc.arrange()

			if code := RunIsolated(nil); code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if msg := stderr(); !strings.Contains(msg, tc.want) {
				t.Errorf("stderr = %q, want it to mention %q", msg, tc.want)
			}
		})
	}
}

// A variable that did not exist before the run must be removed again, not left
// behind holding the sandbox's value.
func TestRunIsolated_UnsetsVariablesItIntroduced(t *testing.T) {
	swapHooks(t)
	captureStderr(t)

	const key = "GCM_NO_KEYCHAIN"
	prev, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetting %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		}
	})

	// Fail only after the environment has been fully applied.
	writeFileFn = func(string, []byte, os.FileMode) error { return errInjected }

	if code := RunIsolated(nil); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	if value, ok := os.LookupEnv(key); ok {
		t.Errorf("%s = %q, want it unset again", key, value)
	}
}

// A failure part-way through the environment loop must still restore whatever
// was already overwritten, otherwise the surrounding process is left polluted.
func TestRunIsolated_RestoresEnvironmentAfterPartialFailure(t *testing.T) {
	swapHooks(t)
	captureStderr(t)

	before := map[string]string{}
	for key := range isolationEnv("probe") {
		before[key] = os.Getenv(key)
	}

	// Let the first assignment through, then fail, leaving one key overwritten.
	calls := 0
	setenvFn = func(key, value string) error {
		calls++
		if calls > 1 {
			return errInjected
		}
		return os.Setenv(key, value)
	}

	if code := RunIsolated(nil); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	for key, want := range before {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q after failed run, want it restored to %q", key, got, want)
		}
	}
}
