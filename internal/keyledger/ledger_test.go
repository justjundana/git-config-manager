package keyledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	_testutil "github.com/justjundana/git-config-manager/pkg/testutil"
)

func newTestLedger(t *testing.T) *Ledger {
	t.Helper()
	return NewWithPath(filepath.Join(t.TempDir(), "generated-keys.json"))
}

func TestLoad_MissingFileReturnsEmpty(t *testing.T) {
	l := newTestLedger(t)
	d, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.SSH) != 0 || len(d.GPG) != 0 {
		t.Fatalf("expected empty ledger, got %+v", d)
	}
}

func TestHasSSH(t *testing.T) {
	l := newTestLedger(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	generated := filepath.Join(home, ".ssh", "id_ed25519_work")
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: generated}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}

	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{"exact path", generated, true},
		{"tilde spelling of the same key", "~/.ssh/id_ed25519_work", true},
		{"unclean spelling of the same key", filepath.Join(home, ".ssh", "..", ".ssh", "id_ed25519_work"), true},
		{"surrounding whitespace", "  " + generated + "  ", true},
		{"a key the user brought themselves", filepath.Join(home, ".ssh", "id_rsa"), false},
		{"empty path", "", false},
		{"whitespace only", "   ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := l.HasSSH(tc.path)
			if err != nil {
				t.Fatalf("HasSSH: %v", err)
			}
			if got != tc.want {
				t.Errorf("HasSSH(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestHasGPG(t *testing.T) {
	l := newTestLedger(t)
	if err := l.AddGPG(GPGEntry{Profile: "work", KeyID: "ABCDEF0123456789"}); err != nil {
		t.Fatalf("AddGPG: %v", err)
	}
	if err := l.AddGPG(GPGEntry{Profile: "blank", KeyID: "  "}); err != nil {
		t.Fatalf("AddGPG blank: %v", err)
	}

	for _, tc := range []struct {
		name  string
		keyID string
		want  bool
	}{
		{"exact", "ABCDEF0123456789", true},
		{"lowercase", "abcdef0123456789", true},
		{"surrounding whitespace", " ABCDEF0123456789 ", true},
		// Deliberately NOT a match: a suffix match here would authorise
		// deleting a secret key the user generated themselves.
		{"short id that is only a suffix", "23456789", false},
		{"longer fingerprint ending with it", "FFFFABCDEF0123456789", false},
		{"unknown key", "0000000000000000", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := l.HasGPG(tc.keyID)
			if err != nil {
				t.Fatalf("HasGPG: %v", err)
			}
			if got != tc.want {
				t.Errorf("HasGPG(%q) = %v, want %v", tc.keyID, got, tc.want)
			}
		})
	}
}

func TestRenameSSH(t *testing.T) {
	l := newTestLedger(t)
	old := "/keys/id_ed25519_work"
	fresh := "/keys/id_ed25519_work_github"
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: old, Fingerprint: "SHA256:abc"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}
	if err := l.AddSSH(SSHEntry{Profile: "other", KeyPath: "/keys/id_ed25519_other"}); err != nil {
		t.Fatalf("AddSSH other: %v", err)
	}

	if err := l.RenameSSH(old, fresh); err != nil {
		t.Fatalf("RenameSSH: %v", err)
	}

	moved, err := l.HasSSH(fresh)
	if err != nil {
		t.Fatalf("HasSSH: %v", err)
	}
	if !moved {
		t.Error("renamed key should be recorded at the new path")
	}
	stale, err := l.HasSSH(old)
	if err != nil {
		t.Fatalf("HasSSH old: %v", err)
	}
	if stale {
		t.Error("old path must no longer be recorded as GCM-generated")
	}

	d, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.SSH) != 2 {
		t.Fatalf("entry count = %d, want 2", len(d.SSH))
	}
	for _, e := range d.SSH {
		if e.KeyPath == fresh {
			if e.Profile != "work" || e.Fingerprint != "SHA256:abc" {
				t.Errorf("rename lost metadata: %+v", e)
			}
		}
	}
}

func TestRenameSSH_NoMatchingEntryIsANoOp(t *testing.T) {
	l := newTestLedger(t)
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: "/keys/a"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}

	if err := l.RenameSSH("/keys/does-not-exist", "/keys/b"); err != nil {
		t.Fatalf("RenameSSH: %v", err)
	}

	d, _ := l.Load()
	if len(d.SSH) != 1 || d.SSH[0].KeyPath != "/keys/a" {
		t.Errorf("ledger should be untouched, got %+v", d.SSH)
	}
}

func TestRenameSSH_IgnoresEmptyPaths(t *testing.T) {
	l := newTestLedger(t)
	if err := l.RenameSSH("", "/keys/b"); err != nil {
		t.Errorf("RenameSSH with empty old path: %v", err)
	}
	if err := l.RenameSSH("/keys/a", "  "); err != nil {
		t.Errorf("RenameSSH with blank new path: %v", err)
	}
}

func TestRenameSSH_PropagatesLoadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated-keys.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := NewWithPath(path).RenameSSH("/a", "/b"); err == nil {
		t.Error("RenameSSH should surface a corrupt ledger")
	}
}

func TestHasSSHAndHasGPG_PropagateLoadErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "generated-keys.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	l := NewWithPath(path)

	if _, err := l.HasSSH("/some/key"); err == nil {
		t.Error("HasSSH should surface a corrupt ledger")
	}
	if _, err := l.HasGPG("ABCDEF"); err == nil {
		t.Error("HasGPG should surface a corrupt ledger")
	}
}

func TestAddSSH_AndLoad(t *testing.T) {
	l := newTestLedger(t)
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: "/home/u/.ssh/id_ed25519_work", Fingerprint: "SHA256:abc"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}

	d, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.SSH) != 1 {
		t.Fatalf("expected 1 SSH entry, got %d", len(d.SSH))
	}
	got := d.SSH[0]
	if got.Profile != "work" || got.KeyPath != "/home/u/.ssh/id_ed25519_work" || got.Fingerprint != "SHA256:abc" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestAddSSH_ReplacesSamePath(t *testing.T) {
	l := newTestLedger(t)
	path := "/home/u/.ssh/id_ed25519_work"
	if err := l.AddSSH(SSHEntry{Profile: "old", KeyPath: path, Fingerprint: "SHA256:old"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}
	if err := l.AddSSH(SSHEntry{Profile: "new", KeyPath: path, Fingerprint: "SHA256:new"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}

	d, _ := l.Load()
	if len(d.SSH) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(d.SSH))
	}
	if d.SSH[0].Profile != "new" || d.SSH[0].Fingerprint != "SHA256:new" {
		t.Fatalf("expected replacement entry, got %+v", d.SSH[0])
	}
}

func TestAddGPG_AndReplace(t *testing.T) {
	l := newTestLedger(t)
	if err := l.AddGPG(GPGEntry{Profile: "work", KeyID: "DEADBEEF", Fingerprint: "FPR1"}); err != nil {
		t.Fatalf("AddGPG: %v", err)
	}
	if err := l.AddGPG(GPGEntry{Profile: "work2", KeyID: "DEADBEEF", Fingerprint: "FPR2"}); err != nil {
		t.Fatalf("AddGPG: %v", err)
	}

	d, _ := l.Load()
	if len(d.GPG) != 1 {
		t.Fatalf("expected 1 GPG entry, got %d", len(d.GPG))
	}
	if d.GPG[0].Profile != "work2" || d.GPG[0].Fingerprint != "FPR2" {
		t.Fatalf("unexpected entry: %+v", d.GPG[0])
	}
}

func TestRemoveSSH(t *testing.T) {
	l := newTestLedger(t)
	_ = l.AddSSH(SSHEntry{Profile: "a", KeyPath: "/k/a"})
	_ = l.AddSSH(SSHEntry{Profile: "b", KeyPath: "/k/b"})

	if err := l.RemoveSSH("/k/a"); err != nil {
		t.Fatalf("RemoveSSH: %v", err)
	}
	d, _ := l.Load()
	if len(d.SSH) != 1 || d.SSH[0].KeyPath != "/k/b" {
		t.Fatalf("unexpected state after remove: %+v", d.SSH)
	}

	// Removing a non-existent path is a no-op.
	if err := l.RemoveSSH("/k/missing"); err != nil {
		t.Fatalf("RemoveSSH no-op: %v", err)
	}
	d, _ = l.Load()
	if len(d.SSH) != 1 {
		t.Fatalf("expected unchanged ledger, got %+v", d.SSH)
	}
}

func TestRemoveGPG(t *testing.T) {
	l := newTestLedger(t)
	_ = l.AddGPG(GPGEntry{Profile: "a", KeyID: "AAAA"})
	_ = l.AddGPG(GPGEntry{Profile: "b", KeyID: "BBBB"})

	if err := l.RemoveGPG("AAAA"); err != nil {
		t.Fatalf("RemoveGPG: %v", err)
	}
	d, _ := l.Load()
	if len(d.GPG) != 1 || d.GPG[0].KeyID != "BBBB" {
		t.Fatalf("unexpected state after remove: %+v", d.GPG)
	}

	// Removing a non-existent key ID is a no-op.
	if err := l.RemoveGPG("MISSING"); err != nil {
		t.Fatalf("RemoveGPG no-op: %v", err)
	}
	d, _ = l.Load()
	if len(d.GPG) != 1 {
		t.Fatalf("expected unchanged ledger, got %+v", d.GPG)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	l := newTestLedger(t)
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: "/k/a"}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}
	info, err := os.Stat(l.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}
}

func TestAddSSH_PreservedCreatedAt(t *testing.T) {
	l := newTestLedger(t)
	ts := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := l.AddSSH(SSHEntry{Profile: "work", KeyPath: "/k/a", CreatedAt: ts}); err != nil {
		t.Fatalf("AddSSH: %v", err)
	}
	d, _ := l.Load()
	if !d.SSH[0].CreatedAt.Equal(ts) {
		t.Fatalf("expected CreatedAt preserved, got %v", d.SSH[0].CreatedAt)
	}
}

func TestNew_UsesGCMDir(t *testing.T) {
	home := t.TempDir()
	_testutil.SetHome(t, home)
	t.Setenv("USERPROFILE", home) // Windows

	l := New()
	want := filepath.Join(home, ".gcm", fileName)
	if l.path != want {
		t.Fatalf("New path = %q, want %q", l.path, want)
	}
}

func TestLoad_ReadError(t *testing.T) {
	l := newTestLedger(t)
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }

	if _, err := l.Load(); err == nil {
		t.Fatal("expected read error")
	}
}

func TestLoad_EmptyFileReturnsEmpty(t *testing.T) {
	l := newTestLedger(t)
	if err := os.WriteFile(l.path, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	d, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(d.SSH) != 0 || len(d.GPG) != 0 {
		t.Fatalf("expected empty ledger, got %+v", d)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	l := newTestLedger(t)
	if err := os.WriteFile(l.path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := l.Load(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAddSSH_LoadErrorPropagates(t *testing.T) {
	l := newTestLedger(t)
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected error from AddSSH")
	}
}

func TestAddGPG_LoadErrorPropagates(t *testing.T) {
	l := newTestLedger(t)
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }

	if err := l.AddGPG(GPGEntry{KeyID: "AAAA"}); err == nil {
		t.Fatal("expected error from AddGPG")
	}
}

func TestRemoveSSH_LoadErrorPropagates(t *testing.T) {
	l := newTestLedger(t)
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }

	if err := l.RemoveSSH("/k/a"); err == nil {
		t.Fatal("expected error from RemoveSSH")
	}
}

func TestRemoveGPG_LoadErrorPropagates(t *testing.T) {
	l := newTestLedger(t)
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }

	if err := l.RemoveGPG("AAAA"); err == nil {
		t.Fatal("expected error from RemoveGPG")
	}
}

// fakeTempFile lets tests inject failures at individual save steps.
type fakeTempFile struct {
	name     string
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
	closed   bool
}

func (f *fakeTempFile) Name() string                { return f.name }
func (f *fakeTempFile) Write(p []byte) (int, error) { return len(p), f.writeErr }
func (f *fakeTempFile) Chmod(os.FileMode) error     { return f.chmodErr }
func (f *fakeTempFile) Sync() error                 { return f.syncErr }
func (f *fakeTempFile) Close() error                { f.closed = true; return f.closeErr }

func restoreSaveHooks(t *testing.T) {
	origMkdir, origMarshal, origCreate := mkdirAllFn, marshalFn, createTempFn
	origStat, origRemove, origRename := statFn, removeFn, renameFn
	t.Cleanup(func() {
		mkdirAllFn, marshalFn, createTempFn = origMkdir, origMarshal, origCreate
		statFn, removeFn, renameFn = origStat, origRemove, origRename
	})
}

func TestSave_MkdirError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	mkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir boom") }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestSave_MarshalError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	marshalFn = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal boom") }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestSave_CreateTempError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	createTempFn = func(string, string) (tempFile, error) { return nil, errors.New("temp boom") }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected create temp error")
	}
}

func TestSave_WriteError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	fake := &fakeTempFile{name: filepath.Join(t.TempDir(), "tmp"), writeErr: errors.New("write boom")}
	createTempFn = func(string, string) (tempFile, error) { return fake, nil }
	statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected write error")
	}
	if !fake.closed {
		t.Fatal("temp file should be closed on write error")
	}
}

func TestSave_ChmodError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	fake := &fakeTempFile{name: filepath.Join(t.TempDir(), "tmp"), chmodErr: errors.New("chmod boom")}
	createTempFn = func(string, string) (tempFile, error) { return fake, nil }
	statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected chmod error")
	}
}

func TestSave_SyncError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	fake := &fakeTempFile{name: filepath.Join(t.TempDir(), "tmp"), syncErr: errors.New("sync boom")}
	createTempFn = func(string, string) (tempFile, error) { return fake, nil }
	statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected sync error")
	}
}

func TestSave_CloseError(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	fake := &fakeTempFile{name: filepath.Join(t.TempDir(), "tmp"), closeErr: errors.New("close boom")}
	createTempFn = func(string, string) (tempFile, error) { return fake, nil }
	statFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected close error")
	}
}

func TestSave_RenameErrorAndTempCleanup(t *testing.T) {
	l := newTestLedger(t)
	restoreSaveHooks(t)
	tmpName := filepath.Join(t.TempDir(), "tmp")
	fake := &fakeTempFile{name: tmpName}
	createTempFn = func(string, string) (tempFile, error) { return fake, nil }
	renameFn = func(string, string) error { return errors.New("rename boom") }

	removed := false
	statFn = func(string) (os.FileInfo, error) { return nil, nil } // temp "exists"
	removeFn = func(string) error { removed = true; return nil }

	if err := l.AddSSH(SSHEntry{KeyPath: "/k/a"}); err == nil {
		t.Fatal("expected rename error")
	}
	if !removed {
		t.Fatal("temp file should be cleaned up after rename failure")
	}
}

// TestMain sandboxes the whole test binary before any test in this package
// runs: HOME, the three git config scopes, the OS keychain, the ssh-agent and
// the GPG keyring are redirected to a throwaway directory. Without it these
// tests rewrite the developer's real ~/.gcm, ~/.gitconfig and login keychain.
func TestMain(m *testing.M) {
	os.Exit(_testutil.RunIsolated(m))
}
