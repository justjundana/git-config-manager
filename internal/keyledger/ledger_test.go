package keyledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	t.Setenv("HOME", home)
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
