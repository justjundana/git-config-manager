package version

import (
	"os"
	"runtime"
	"strings"
	"testing"

	_testutil "github.com/justjundana/git-config-manager/pkg/testutil"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Version should not be empty")
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %s, want %s", info.OS, runtime.GOOS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %s, want %s", info.Arch, runtime.GOARCH)
	}
}

func TestInfoString(t *testing.T) {
	info := Get()
	s := info.String()

	if !strings.Contains(s, "gcm") {
		t.Errorf("String() should contain 'gcm', got: %s", s)
	}
	if !strings.Contains(s, info.Version) {
		t.Errorf("String() should contain version, got: %s", s)
	}
}

func TestInfoShort(t *testing.T) {
	info := Get()
	s := info.Short()

	expected := "gcm " + info.Version
	if s != expected {
		t.Errorf("Short() = %s, want %s", s, expected)
	}
}

// TestMain sandboxes the whole test binary before any test in this package
// runs: HOME, the three git config scopes, the OS keychain, the ssh-agent and
// the GPG keyring are redirected to a throwaway directory. Without it these
// tests rewrite the developer's real ~/.gcm, ~/.gitconfig and login keychain.
func TestMain(m *testing.M) {
	os.Exit(_testutil.RunIsolated(m))
}
