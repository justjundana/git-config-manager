package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_keyledger "github.com/justjundana/git-config-manager/internal/keyledger"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"
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

// The rename only applies to files already following GCM's own naming
// convention. A key the user created and attached to a profile keeps its name
// and its location — activating a profile must not move the user's files.
func TestMigrateSSHKeyPath_LeavesAdoptedKeyNamesAlone(t *testing.T) {
	ctr := withRepairTestContainer(t)

	for i, name := range []string{"id_rsa", "my-personal-key", "work-laptop.pem"} {
		t.Run(name, func(t *testing.T) {
			keyPath := filepath.Join(ctr.Config.SSHDir, name)
			writeFakeSSHKey(t, keyPath)

			// Profile names allow only letters, digits, "-" and "_", so it is
			// derived from the index rather than the key file name.
			p := repairTestProfile(fmt.Sprintf("adopted%d", i))
			p.SSH = &_profile.SSHConfig{KeyPath: keyPath, KeyType: "ed25519"}
			p.Providers = map[string]_profile.ProviderAccountConfig{
				string(_provider.GitHubID): {Username: "someone"},
			}
			if err := ctr.ProfileManager.Create(p); err != nil {
				t.Fatalf("create profile: %v", err)
			}

			if _, ok := providerSSHKeyMigrationTarget(p.Name, p); ok {
				t.Fatalf("a key named %q must not be scheduled for renaming", name)
			}

			migrated, err := migrateProfileSSHKeyPathToProvider(p.Name, p)
			if err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if migrated {
				t.Errorf("adopted key %q was renamed", name)
			}
			if _, err := os.Stat(keyPath); err != nil {
				t.Errorf("adopted key must stay where the user put it: %v", err)
			}
		})
	}
}

// Renaming a key is a side effect of activating a profile, not the thing the
// user asked for, so it is offered rather than performed.
func TestMigrateSSHKeyPath_AsksBeforeRenaming(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		answer      string
		wantRenamed bool
	}{
		{"declined", true, "n\n", false},
		{"accepted", true, "y\n", true},
		// A scripted run may well have something on stdin for its own
		// purposes. Without the interactivity check that "y" would be read as
		// consent and the key moved, so this deliberately supplies one.
		{"no one to ask, stdin says yes", false, "y\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctr := withRepairTestContainer(t)
			defer _ui.SetInteractiveForTesting(tc.interactive)()
			if tc.answer != "" {
				setUIPromptInput(t, tc.answer)
			}

			p, keyPath := profileNeedingSSHRename(t, "work")
			target, ok := providerSSHKeyMigrationTarget(p.Name, p)
			if !ok || target == keyPath {
				t.Skip("provider layout does not want this key renamed")
			}

			migrated, err := migrateProfileSSHKeyPathWithConsent(p.Name, p)
			if err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if migrated != tc.wantRenamed {
				t.Errorf("migrated = %v, want %v", migrated, tc.wantRenamed)
			}

			movedAway := false
			if _, statErr := os.Stat(keyPath); statErr != nil {
				movedAway = true
			}
			if movedAway != tc.wantRenamed {
				t.Errorf("key moved = %v, want %v", movedAway, tc.wantRenamed)
			}

			reloaded, err := ctr.ProfileManager.Get(p.Name)
			if err != nil {
				t.Fatalf("get profile: %v", err)
			}
			wantPath := keyPath
			if tc.wantRenamed {
				wantPath = target
			}
			if reloaded.SSH.KeyPath != wantPath {
				t.Errorf("profile KeyPath = %q, want %q", reloaded.SSH.KeyPath, wantPath)
			}
		})
	}
}

// gcm repair --fix renames without asking: there the rename is precisely what
// the user requested.
func TestMigrateSSHKeyPath_RepairRenamesWithoutAsking(t *testing.T) {
	withRepairTestContainer(t)
	defer _ui.SetInteractiveForTesting(false)()

	p, keyPath := profileNeedingSSHRename(t, "work")
	target, ok := providerSSHKeyMigrationTarget(p.Name, p)
	if !ok || target == keyPath {
		t.Skip("provider layout does not want this key renamed")
	}

	migrated, err := migrateProfileSSHKeyPathToProvider(p.Name, p)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrated {
		t.Fatal("repair must rename without a prompt")
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("key should be at the new path: %v", err)
	}
}
