package cli

import (
	"os"
	"path/filepath"
	"testing"

	_keyledger "github.com/justjundana/git-config-manager/internal/keyledger"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
)

// profileNeedingSSHRename builds a profile whose SSH key sits at a path the
// provider-aware layout wants renamed, and returns that profile plus the path.
func profileNeedingSSHRename(t *testing.T, name string) (*_profile.Profile, string) {
	t.Helper()

	keyPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_"+name)
	writeFakeSSHKey(t, keyPath)

	p := repairTestProfile(name)
	p.SSH = &_profile.SSHConfig{KeyPath: keyPath, KeyType: "ed25519"}
	p.Providers = map[string]_profile.ProviderAccountConfig{
		string(_provider.GitHubID): {Username: name},
	}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return p, keyPath
}

// gcm status is a dashboard. It must not rename key files or rewrite profile
// YAML as a side effect of being looked at.
func TestStatus_DoesNotRenameSSHKeys(t *testing.T) {
	ctr := withRepairTestContainer(t)

	p, keyPath := profileNeedingSSHRename(t, "work")
	target, ok := providerSSHKeyMigrationTarget(p.Name, p)
	if !ok || target == keyPath {
		t.Skip("provider layout does not want this key renamed; nothing to assert")
	}

	if err := runStatus(); err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("status must leave the key where it was: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("status renamed the key to %s", target)
	}
	reloaded, err := ctr.ProfileManager.Get("work")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if reloaded.SSH.KeyPath != keyPath {
		t.Errorf("status rewrote the profile: KeyPath = %q, want %q", reloaded.SSH.KeyPath, keyPath)
	}
}

// When GCM does rename a generated key, the ledger must follow it. Otherwise
// the key loses its provenance and the old path stays marked as GCM's, so
// anything the user later puts there becomes eligible for deletion.
func TestMigrateSSHKeyPath_MovesLedgerEntry(t *testing.T) {
	ctr := withRepairTestContainer(t)

	p, keyPath := profileNeedingSSHRename(t, "work")
	target, ok := providerSSHKeyMigrationTarget(p.Name, p)
	if !ok || target == keyPath {
		t.Skip("provider layout does not want this key renamed; nothing to assert")
	}
	if err := ctr.KeyLedger.AddSSH(_keyledger.SSHEntry{Profile: "work", KeyPath: keyPath}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	migrated, err := migrateProfileSSHKeyPathToProvider(p.Name, p)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrated {
		t.Fatal("expected the key to be migrated")
	}

	atNew, err := ctr.KeyLedger.HasSSH(target)
	if err != nil {
		t.Fatalf("HasSSH new: %v", err)
	}
	if !atNew {
		t.Error("ledger must record the key at its new path")
	}
	atOld, err := ctr.KeyLedger.HasSSH(keyPath)
	if err != nil {
		t.Fatalf("HasSSH old: %v", err)
	}
	if atOld {
		t.Error("ledger must not keep claiming the old path is GCM-generated")
	}
}
