package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justjundana/git-config-manager/internal/keyledger"
	"github.com/justjundana/git-config-manager/internal/profile"
)

// writeFakeSSHKey creates an empty private + public key file pair at path.
func writeFakeSSHKey(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("PRIVATE"), 0o600); err != nil {
		t.Fatalf("write priv: %v", err)
	}
	if err := os.WriteFile(path+".pub", []byte("ssh-ed25519 AAAA test"), 0o644); err != nil {
		t.Fatalf("write pub: %v", err)
	}
}

func TestSSHClean_RemovesOrphanKeepsReferencedAndPreexisting(t *testing.T) {
	ctr := withRepairTestContainer(t)

	usedPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_used")
	orphanPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_orphan")
	preexistingPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_preexisting")

	writeFakeSSHKey(t, usedPath)
	writeFakeSSHKey(t, orphanPath)
	writeFakeSSHKey(t, preexistingPath)

	// A profile references the "used" key.
	p := repairTestProfile("work")
	p.SSH = &profile.SSHConfig{KeyPath: usedPath, KeyType: "ed25519"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	// Ledger records the used + orphan keys as GCM-generated. The preexisting
	// key is intentionally NOT in the ledger.
	if err := ctr.KeyLedger.AddSSH(keyledger.SSHEntry{Profile: "work", KeyPath: usedPath}); err != nil {
		t.Fatalf("ledger add used: %v", err)
	}
	if err := ctr.KeyLedger.AddSSH(keyledger.SSHEntry{Profile: "gone", KeyPath: orphanPath}); err != nil {
		t.Fatalf("ledger add orphan: %v", err)
	}

	cmd := newSSHCleanCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ssh clean: %v", err)
	}

	// Orphan removed.
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan key should have been removed, stat err = %v", err)
	}
	// Referenced key kept.
	if _, err := os.Stat(usedPath); err != nil {
		t.Fatalf("used key should be kept: %v", err)
	}
	// Pre-existing (not in ledger) key kept.
	if _, err := os.Stat(preexistingPath); err != nil {
		t.Fatalf("pre-existing key should be ignored and kept: %v", err)
	}

	// Ledger should no longer reference the orphan.
	data, _ := ctr.KeyLedger.Load()
	for _, e := range data.SSH {
		if e.KeyPath == orphanPath {
			t.Fatalf("orphan still in ledger: %+v", data.SSH)
		}
	}
}

func TestSSHClean_DryRunRemovesNothing(t *testing.T) {
	ctr := withRepairTestContainer(t)
	orphanPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_orphan")
	writeFakeSSHKey(t, orphanPath)
	if err := ctr.KeyLedger.AddSSH(keyledger.SSHEntry{Profile: "gone", KeyPath: orphanPath}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	cmd := newSSHCleanCmd()
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ssh clean dry-run: %v", err)
	}

	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("dry-run should not delete key: %v", err)
	}
	data, _ := ctr.KeyLedger.Load()
	if len(data.SSH) != 1 {
		t.Fatalf("dry-run should not modify ledger, got %+v", data.SSH)
	}
}

func TestSSHClean_NoLedgerEntries(t *testing.T) {
	ctr := withRepairTestContainer(t)
	_ = ctr
	cmd := newSSHCleanCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ssh clean: %v", err)
	}
}

func TestGPGKeyReferenced_SuffixMatching(t *testing.T) {
	ref := map[string]struct{}{"ABCDEF12": {}}

	if !gpgKeyReferenced(keyledger.GPGEntry{KeyID: "ABCDEF12"}, ref) {
		t.Fatal("exact key ID should match")
	}
	if !gpgKeyReferenced(keyledger.GPGEntry{KeyID: "1122ABCDEF12"}, ref) {
		t.Fatal("longer ledger key ID with matching suffix should match")
	}
	if !gpgKeyReferenced(keyledger.GPGEntry{KeyID: "EF12", Fingerprint: "0011223344ABCDEF12"}, ref) {
		t.Fatal("fingerprint suffix should match")
	}
	if !gpgKeyReferenced(keyledger.GPGEntry{KeyID: "ZZZZ", Fingerprint: "0000ABCDEF12"}, ref) {
		t.Fatal("fingerprint-only suffix should match when key ID differs")
	}
	if gpgKeyReferenced(keyledger.GPGEntry{KeyID: "99999999"}, ref) {
		t.Fatal("unrelated key should not match")
	}
}

func TestNormalizeKeyPath_ExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	got := normalizeKeyPath("~/.ssh/id_ed25519")
	want := filepath.Clean(filepath.Join(home, ".ssh/id_ed25519"))
	if got != want {
		t.Fatalf("normalizeKeyPath = %q, want %q", got, want)
	}
}

func TestSSHClean_CancelViaPrompt(t *testing.T) {
	ctr := withRepairTestContainer(t)
	orphanPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_orphan")
	writeFakeSSHKey(t, orphanPath)
	if err := ctr.KeyLedger.AddSSH(keyledger.SSHEntry{Profile: "gone", KeyPath: orphanPath}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	// Answer "n" to the confirmation prompt.
	setUIPromptInput(t, "n\n")

	cmd := newSSHCleanCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ssh clean cancel: %v", err)
	}

	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("cancel should keep key: %v", err)
	}
	data, _ := ctr.KeyLedger.Load()
	if len(data.SSH) != 1 {
		t.Fatalf("cancel should not modify ledger, got %+v", data.SSH)
	}
}

func TestGPGClean_NoLedgerEntries(t *testing.T) {
	ctr := withRepairTestContainer(t)
	_ = ctr
	cmd := newGPGCleanCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("gpg clean: %v", err)
	}
}

func TestGPGClean_DryRunListsOrphans(t *testing.T) {
	ctr := withRepairTestContainer(t)

	// One referenced key, one orphan.
	p := repairTestProfile("work")
	p.GPG = &profile.GPGConfig{KeyID: "USEDKEY1"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := ctr.KeyLedger.AddGPG(keyledger.GPGEntry{Profile: "work", KeyID: "USEDKEY1"}); err != nil {
		t.Fatalf("ledger add used: %v", err)
	}
	if err := ctr.KeyLedger.AddGPG(keyledger.GPGEntry{Profile: "gone", KeyID: "ORPHANKEY"}); err != nil {
		t.Fatalf("ledger add orphan: %v", err)
	}

	cmd := newGPGCleanCmd()
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("set dry-run: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("gpg clean dry-run: %v", err)
	}

	// Dry run must not modify the ledger.
	data, _ := ctr.KeyLedger.Load()
	if len(data.GPG) != 2 {
		t.Fatalf("dry-run should not modify ledger, got %+v", data.GPG)
	}
}

func TestGPGClean_GPGNotInstalledStopsBeforeDeleting(t *testing.T) {
	ctr := withRepairTestContainer(t)
	// GPG is not installed in the test environment, so an orphaned entry must
	// be reported but not deleted from the ledger.
	if err := ctr.KeyLedger.AddGPG(keyledger.GPGEntry{Profile: "gone", KeyID: "ORPHANKEY"}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	if ctr.GPGManager.IsInstalled() {
		t.Skip("gpg is installed; this test asserts the not-installed branch")
	}

	cmd := newGPGCleanCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("gpg clean: %v", err)
	}

	data, _ := ctr.KeyLedger.Load()
	if len(data.GPG) != 1 {
		t.Fatalf("not-installed branch should not modify ledger, got %+v", data.GPG)
	}
}

func TestReferencedGPGKeyIDs(t *testing.T) {
	ctr := withRepairTestContainer(t)

	p1 := repairTestProfile("a")
	p1.GPG = &profile.GPGConfig{KeyID: "KEYA"}
	if err := ctr.ProfileManager.Create(p1); err != nil {
		t.Fatalf("create a: %v", err)
	}
	p2 := repairTestProfile("b") // no GPG
	if err := ctr.ProfileManager.Create(p2); err != nil {
		t.Fatalf("create b: %v", err)
	}

	refs, err := referencedGPGKeyIDs()
	if err != nil {
		t.Fatalf("referencedGPGKeyIDs: %v", err)
	}
	if _, ok := refs["KEYA"]; !ok {
		t.Fatalf("expected KEYA referenced, got %+v", refs)
	}
	if len(refs) != 1 {
		t.Fatalf("expected exactly 1 referenced key, got %+v", refs)
	}
}

func TestRemoveSSHKeyFiles_MissingIsNoError(t *testing.T) {
	if err := removeSSHKeyFiles(filepath.Join(t.TempDir(), "does-not-exist")); err != nil {
		t.Fatalf("removing missing key should not error: %v", err)
	}
}

func TestSSHClean_RemoveFailureKeepsLedger(t *testing.T) {
	ctr := withRepairTestContainer(t)

	// Ledger references a path whose private key file is actually a directory,
	// so os.Remove fails and the entry should remain in the ledger.
	dirPath := filepath.Join(ctr.Config.SSHDir, "id_ed25519_dir")
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Put a file inside so the directory is non-empty and cannot be removed.
	if err := os.WriteFile(filepath.Join(dirPath, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := ctr.KeyLedger.AddSSH(keyledger.SSHEntry{Profile: "gone", KeyPath: dirPath}); err != nil {
		t.Fatalf("ledger add: %v", err)
	}

	cmd := newSSHCleanCmd()
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("ssh clean: %v", err)
	}

	data, _ := ctr.KeyLedger.Load()
	if len(data.SSH) != 1 {
		t.Fatalf("failed removal should keep ledger entry, got %+v", data.SSH)
	}
}
