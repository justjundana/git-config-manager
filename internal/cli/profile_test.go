package cli

import (
	"os"
	"path/filepath"
	"testing"

	_keyledger "github.com/justjundana/git-config-manager/internal/keyledger"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
)

// deleteProfile runs "gcm profile delete <name> --yes".
func deleteProfile(t *testing.T, name string) error {
	t.Helper()
	cmd := newProfileDeleteCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	return cmd.RunE(cmd, []string{name})
}

// A profile may point at a key the user already had; deleting the profile must
// not delete that key. The generated-keys ledger is the only thing that
// distinguishes the two, and package keyledger documents it as the source of
// truth for exactly this decision.
func TestProfileDelete_KeepsSSHKeyGCMDidNotGenerate(t *testing.T) {
	ctr := withRepairTestContainer(t)

	keyPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_preexisting")
	writeFakeSSHKey(t, keyPath)

	p := repairTestProfile("work")
	p.SSH = &_profile.SSHConfig{KeyPath: keyPath, KeyType: "ed25519"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	// Deliberately NOT recorded in the ledger: GCM adopted this key.

	if err := deleteProfile(t, "work"); err != nil {
		t.Fatalf("profile delete: %v", err)
	}

	if ctr.ProfileManager.Exists("work") {
		t.Error("profile should have been deleted")
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("adopted private key must survive profile deletion: %v", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Fatalf("adopted public key must survive profile deletion: %v", err)
	}
}

func TestProfileDelete_RemovesSSHKeyGCMGenerated(t *testing.T) {
	ctr := withRepairTestContainer(t)

	keyPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_generated")
	writeFakeSSHKey(t, keyPath)

	p := repairTestProfile("work")
	p.SSH = &_profile.SSHConfig{KeyPath: keyPath, KeyType: "ed25519"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := ctr.KeyLedger.AddSSH(_keyledger.SSHEntry{Profile: "work", KeyPath: keyPath}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	if err := deleteProfile(t, "work"); err != nil {
		t.Fatalf("profile delete: %v", err)
	}

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("generated private key should have been removed, stat err = %v", err)
	}
	// The ledger must not keep pointing at a file that no longer exists.
	data, err := ctr.KeyLedger.Load()
	if err != nil {
		t.Fatalf("ledger load: %v", err)
	}
	for _, e := range data.SSH {
		if e.KeyPath == keyPath {
			t.Errorf("ledger still references the deleted key: %+v", data.SSH)
		}
	}
}

// Deleting a GPG secret key is irreversible, so the same rule applies: a key
// GCM did not generate is left in the keyring.
func TestProfileDelete_KeepsGPGKeyGCMDidNotGenerate(t *testing.T) {
	ctr := withRepairTestContainer(t)

	p := repairTestProfile("work")
	p.GPG = &_profile.GPGConfig{KeyID: "USERSOWNKEY12345"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	if err := deleteProfile(t, "work"); err != nil {
		t.Fatalf("profile delete: %v", err)
	}

	if ctr.ProfileManager.Exists("work") {
		t.Error("profile should have been deleted")
	}
	// The ledger never recorded it, and deletion must not have added anything.
	data, err := ctr.KeyLedger.Load()
	if err != nil {
		t.Fatalf("ledger load: %v", err)
	}
	if len(data.GPG) != 0 {
		t.Errorf("ledger should still be empty, got %+v", data.GPG)
	}
}
