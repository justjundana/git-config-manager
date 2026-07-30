package cli

import (
	"os"
	"testing"

	_testutil "github.com/justjundana/git-config-manager/pkg/testutil"
)

// TestMain sandboxes the whole test binary before any test in this package
// runs: HOME, the three git config scopes, the OS keychain, the ssh-agent and
// the GPG keyring are redirected to a throwaway directory. Without it these
// tests rewrite the developer's real ~/.gcm, ~/.gitconfig and login keychain.
func TestMain(m *testing.M) {
	os.Exit(_testutil.RunIsolated(m))
}

func TestNewRootCmd_HasExpectedSubcommands(t *testing.T) {
	root := NewRootCmd()

	want := []string{"init", "use", "profile", "doctor", "repair", "backup", "clean"}
	got := make(map[string]bool, len(root.Commands()))
	for _, sub := range root.Commands() {
		got[sub.Name()] = true
	}

	for _, name := range want {
		if !got[name] {
			t.Errorf("root command is missing subcommand %q", name)
		}
	}
}
