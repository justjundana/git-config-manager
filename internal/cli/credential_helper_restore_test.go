package cli

import (
	"os/exec"
	"strings"
	"testing"
)

func globalHelpers(t *testing.T, server string) []string {
	t.Helper()
	out, err := exec.Command("git", "config", "--global", "--get-all", "credential."+server+".helper").Output()
	if err != nil {
		return nil
	}
	var got []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			got = append(got, l)
		}
	}
	return got
}

// Registering resets credential.<host>.helper so GCM's helper wins. That
// previously discarded whatever the user already had, with no way back.
func TestCredentialHelper_RestoresPreExistingHelperOnUnregister(t *testing.T) {
	withRepairTestContainer(t)

	const server = "https://github.com"
	if err := exec.Command("git", "config", "--global", "--add", "credential."+server+".helper", "osxkeychain").Run(); err != nil {
		t.Fatalf("seed helper: %v", err)
	}

	if err := RegisterCredentialHelper(); err != nil {
		t.Fatalf("RegisterCredentialHelper: %v", err)
	}

	after := globalHelpers(t, server)
	joined := strings.Join(after, "|")
	if !strings.Contains(joined, "credential-helper") {
		t.Fatalf("GCM helper should be registered, got %v", after)
	}
	if strings.Contains(joined, "osxkeychain") {
		t.Fatalf("the pre-existing helper should be reset while GCM is active, got %v", after)
	}

	if err := UnregisterCredentialHelper(); err != nil {
		t.Fatalf("UnregisterCredentialHelper: %v", err)
	}

	restored := globalHelpers(t, server)
	if strings.Join(restored, "|") != "osxkeychain" {
		t.Fatalf("pre-existing helper must be restored, got %v", restored)
	}

	saved, _ := exec.Command("git", "config", "--global", "--get-all", savedHelperKey(server)).Output()
	if strings.TrimSpace(string(saved)) != "" {
		t.Errorf("the saved record should be cleared after restore, got %q", saved)
	}
}

// Running init twice must not overwrite the original record with GCM's own.
func TestCredentialHelper_SecondRegisterKeepsOriginalRecord(t *testing.T) {
	withRepairTestContainer(t)

	const server = "https://github.com"
	if err := exec.Command("git", "config", "--global", "--add", "credential."+server+".helper", "manager-core").Run(); err != nil {
		t.Fatalf("seed helper: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := RegisterCredentialHelper(); err != nil {
			t.Fatalf("RegisterCredentialHelper #%d: %v", i+1, err)
		}
	}
	if err := UnregisterCredentialHelper(); err != nil {
		t.Fatalf("UnregisterCredentialHelper: %v", err)
	}

	restored := globalHelpers(t, server)
	if strings.Join(restored, "|") != "manager-core" {
		t.Fatalf("original helper must survive a repeated register, got %v", restored)
	}
}
