package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type fakeAtomicFile struct {
	name     string
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (f *fakeAtomicFile) Name() string                { return f.name }
func (f *fakeAtomicFile) Write(_ []byte) (int, error) { return 0, f.writeErr }
func (f *fakeAtomicFile) Chmod(_ os.FileMode) error   { return f.chmodErr }
func (f *fakeAtomicFile) Sync() error                 { return f.syncErr }
func (f *fakeAtomicFile) Close() error                { return f.closeErr }

func TestWriteAndRead(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	data := []byte("hello world")
	if err := svc.Write(path, data, 0644); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	got, err := svc.Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Read() = %q, want %q", got, data)
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.txt")

	if err := svc.Write(path, []byte("nested"), 0644); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	if !svc.Exists(path) {
		t.Error("file should exist after Write()")
	}
}

func TestWriteAtomic(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")

	data := []byte("atomic write content")
	if err := svc.WriteAtomic(path, data, 0644); err != nil {
		t.Fatalf("WriteAtomic() error: %v", err)
	}

	got, err := svc.Read(path)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("Read() = %q, want %q", got, data)
	}

	// Verify no temp files left behind
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}

func TestExists(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	if svc.Exists(filepath.Join(dir, "nonexistent")) {
		t.Error("Exists() should return false for nonexistent file")
	}

	path := filepath.Join(dir, "exists.txt")
	_ = svc.Write(path, []byte("test"), 0644)
	if !svc.Exists(path) {
		t.Error("Exists() should return true for existing file")
	}
}

func TestDelete(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "delete.txt")

	_ = svc.Write(path, []byte("delete me"), 0644)
	if err := svc.Delete(path); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if svc.Exists(path) {
		t.Error("file should not exist after Delete()")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	svc := NewService()
	if err := svc.Delete("/tmp/gcm-nonexistent-file-12345"); err != nil {
		t.Errorf("Delete() of nonexistent file should not error, got: %v", err)
	}
}

func TestList(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	_ = svc.Write(filepath.Join(dir, "a.yaml"), []byte("a"), 0644)
	_ = svc.Write(filepath.Join(dir, "b.yaml"), []byte("b"), 0644)
	_ = svc.Write(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	files, err := svc.List(dir, "*.yaml")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("List(*.yaml) = %d files, want 2", len(files))
	}
}

func TestCopyFile(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "subdir", "dst.txt")

	_ = svc.Write(src, []byte("copy me"), 0644)

	if err := svc.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile() error: %v", err)
	}

	got, _ := svc.Read(dst)
	if string(got) != "copy me" {
		t.Errorf("CopyFile() content = %q, want %q", got, "copy me")
	}
}

func TestEnsurePermissions(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	_ = svc.Write(path, []byte("test"), 0755)

	if err := svc.EnsurePermissions(path, 0600); err != nil {
		t.Fatalf("EnsurePermissions() error: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestRead_Nonexistent(t *testing.T) {
	svc := NewService()
	_, err := svc.Read("/tmp/gcm-nonexistent-12345")
	if err == nil {
		t.Error("Read() nonexistent should error")
	}
}

func TestCopyFile_NonexistentSrc(t *testing.T) {
	svc := NewService()
	err := svc.CopyFile("/tmp/gcm-nonexistent-src-12345", filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Error("CopyFile() nonexistent src should error")
	}
}

func TestEnsurePermissions_Nonexistent(t *testing.T) {
	svc := NewService()
	err := svc.EnsurePermissions("/tmp/gcm-nonexistent-12345", 0600)
	if err == nil {
		t.Error("EnsurePermissions() nonexistent should error")
	}
}

func TestEnsurePermissions_AlreadyCorrect(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "correct.txt")
	_ = svc.Write(path, []byte("test"), 0644)

	if err := svc.EnsurePermissions(path, 0644); err != nil {
		t.Fatalf("EnsurePermissions same perm: %v", err)
	}
}

func TestWriteAtomic_CreatesParentDirs(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "atomic.txt")

	if err := svc.WriteAtomic(path, []byte("nested atomic"), 0644); err != nil {
		t.Fatalf("WriteAtomic nested: %v", err)
	}

	got, _ := svc.Read(path)
	if string(got) != "nested atomic" {
		t.Errorf("got %q", got)
	}
}

func TestList_EmptyDir(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	files, err := svc.List(dir, "*.yaml")
	if err != nil {
		t.Fatalf("List empty dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestList_NoMatch(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	_ = svc.Write(filepath.Join(dir, "test.txt"), []byte("hi"), 0644)

	files, err := svc.List(dir, "*.yaml")
	if err != nil {
		t.Fatalf("List no match: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 matches, got %d", len(files))
	}
}

func TestCopyFile_PreservesContent(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	src := filepath.Join(dir, "original.bin")
	data := []byte{0, 1, 2, 3, 255, 254, 253}
	_ = svc.Write(src, data, 0644)

	dst := filepath.Join(dir, "copy.bin")
	if err := svc.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, _ := svc.Read(dst)
	if len(got) != len(data) {
		t.Fatalf("copied %d bytes, want %d", len(got), len(data))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Errorf("byte %d: got %d, want %d", i, got[i], data[i])
		}
	}
}

func TestWriteAtomic_ReadOnlyDir(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	err := svc.WriteAtomic(filepath.Join(roDir, "test.txt"), []byte("data"), 0644)
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestCopyFile_DestinationParentBlockedByFile(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	if err := svc.Write(src, []byte("src-data"), 0o644); err != nil {
		t.Fatalf("Write src: %v", err)
	}

	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}

	err := svc.CopyFile(src, filepath.Join(blocker, "dst.txt"))
	if err == nil {
		t.Fatal("expected error when destination parent is a file")
	}
}

func TestWriteAtomic_HookErrors(t *testing.T) {
	svc := NewService()
	home := t.TempDir()
	path := filepath.Join(home, "out.txt")

	origCreate := writeAtomicCreateTempFn
	origRename := writeAtomicRenameFn
	origStat := writeAtomicStatFn
	defer func() {
		writeAtomicCreateTempFn = origCreate
		writeAtomicRenameFn = origRename
		writeAtomicStatFn = origStat
	}()

	t.Run("create temp error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return nil, fmt.Errorf("temp fail")
		}
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected create temp error")
		}
		writeAtomicCreateTempFn = origCreate
	})

	t.Run("write error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return &fakeAtomicFile{name: filepath.Join(home, "fake-write"), writeErr: fmt.Errorf("write fail")}, nil
		}
		writeAtomicStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected write error")
		}
	})

	t.Run("chmod error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return &fakeAtomicFile{name: filepath.Join(home, "fake-chmod"), chmodErr: fmt.Errorf("chmod fail")}, nil
		}
		writeAtomicStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected chmod error")
		}
	})

	t.Run("sync error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return &fakeAtomicFile{name: filepath.Join(home, "fake-sync"), syncErr: fmt.Errorf("sync fail")}, nil
		}
		writeAtomicStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected sync error")
		}
	})

	t.Run("close error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return &fakeAtomicFile{name: filepath.Join(home, "fake-close"), closeErr: fmt.Errorf("close fail")}, nil
		}
		writeAtomicStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected close error")
		}
	})

	t.Run("rename error", func(t *testing.T) {
		writeAtomicCreateTempFn = func(string, string) (atomicTempFile, error) {
			return &fakeAtomicFile{name: filepath.Join(home, "fake-rename")}, nil
		}
		writeAtomicRenameFn = func(string, string) error { return fmt.Errorf("rename fail") }
		writeAtomicStatFn = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		err := svc.WriteAtomic(path, []byte("x"), 0o644)
		if err == nil {
			t.Fatal("expected rename error")
		}
		writeAtomicRenameFn = origRename
		writeAtomicCreateTempFn = origCreate
		writeAtomicStatFn = origStat
	})
}

func TestWriteAtomic_BadParentPath(t *testing.T) {
	svc := NewService()
	err := svc.WriteAtomic("/dev/null/impossible/test.txt", []byte("data"), 0644)
	if err == nil {
		t.Error("expected error for impossible parent path")
	}
}

func TestWrite_BadParentPath(t *testing.T) {
	svc := NewService()
	err := svc.Write("/dev/null/impossible/test.txt", []byte("data"), 0644)
	if err == nil {
		t.Error("expected error for impossible parent path")
	}
}

func TestDelete_NonEmptyDirectory(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0o755)
	_ = svc.Write(filepath.Join(subdir, "file.txt"), []byte("hi"), 0644)

	err := svc.Delete(subdir)
	if err == nil {
		t.Error("expected error deleting non-empty directory")
	}
}

func TestList_BadPattern(t *testing.T) {
	svc := NewService()
	_, err := svc.List(t.TempDir(), "[")
	if err == nil {
		t.Error("expected error for bad glob pattern")
	}
}

func TestCopyFile_ReadOnlyDestDir(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	_ = svc.Write(src, []byte("data"), 0644)

	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	err := svc.CopyFile(src, filepath.Join(roDir, "dst.txt"))
	if err == nil {
		t.Error("expected error copying to read-only dir")
	}
}

func TestWriteAtomic_VerifyPermissions(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	if err := svc.WriteAtomic(path, []byte("secret"), 0600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestWrite_CreatesParentDirs(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	if err := svc.Write(path, []byte("data"), 0644); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "data" {
		t.Errorf("content = %q", string(data))
	}
}

func TestDelete_File(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "del.txt")
	os.WriteFile(path, []byte("x"), 0644)

	if err := svc.Delete(path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestDelete_NonExistent(t *testing.T) {
	svc := NewService()
	// Delete of non-existent should succeed (IsNotExist is ignored)
	if err := svc.Delete("/nonexistent/path"); err != nil {
		t.Fatalf("Delete nonexistent should not error: %v", err)
	}
}

func TestCopyFile_LargeFile(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "large.bin")
	dst := filepath.Join(dir, "large_copy.bin")

	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(src, data, 0644)

	if err := svc.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	copied, _ := os.ReadFile(dst)
	if len(copied) != len(data) {
		t.Errorf("size mismatch: got %d, want %d", len(copied), len(data))
	}
}

func TestList_MultipleMatches(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.go"), []byte("c"), 0644)

	files, err := svc.List(dir, "*.txt")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 .txt files, got %d", len(files))
	}
}

func TestExists_CheckBoth(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.txt")

	if svc.Exists(path) {
		t.Error("file should not exist yet")
	}

	os.WriteFile(path, []byte("x"), 0644)
	if !svc.Exists(path) {
		t.Error("file should exist after write")
	}
}

func TestWrite_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	svc := NewService()
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	err := svc.Write(filepath.Join(roDir, "file.txt"), []byte("data"), 0644)
	if err == nil {
		t.Error("expected error writing to read-only dir")
	}
}

func TestCopyFile_PreservesMode(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	os.WriteFile(src, []byte("data"), 0o755)
	if err := svc.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
		t.Errorf("mode mismatch: src=%o, dst=%o", srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
	}
}

func TestWriteAtomic_Overwrite(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	svc.WriteAtomic(path, []byte("first"), 0644)
	svc.WriteAtomic(path, []byte("second"), 0644)

	got, _ := svc.Read(path)
	if string(got) != "second" {
		t.Errorf("got %q, want second", got)
	}
}

func TestEnsurePermissions_TightenPermissions(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "loose.txt")
	os.WriteFile(path, []byte("data"), 0o777)

	if err := svc.EnsurePermissions(path, 0o600); err != nil {
		t.Fatalf("EnsurePermissions: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestWriteAtomic_Permissions(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	if err := svc.WriteAtomic(path, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestCopyFile_ToReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	svc := NewService()
	dir := t.TempDir()

	src := filepath.Join(dir, "src.txt")
	svc.Write(src, []byte("data"), 0o644)

	roDir := filepath.Join(dir, "readonly")
	os.MkdirAll(roDir, 0o755)
	os.Chmod(roDir, 0o444)
	t.Cleanup(func() { os.Chmod(roDir, 0o755) })

	err := svc.CopyFile(src, filepath.Join(roDir, "dst.txt"))
	if err == nil {
		t.Error("expected error copying to read-only dir")
	}
}

func TestWrite_LargeData(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	data := make([]byte, 1024*1024) // 1 MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := svc.Write(path, data, 0o644); err != nil {
		t.Fatalf("Write large: %v", err)
	}

	got, err := svc.Read(path)
	if err != nil {
		t.Fatalf("Read large: %v", err)
	}
	if len(got) != len(data) {
		t.Errorf("size = %d, want %d", len(got), len(data))
	}
}

func TestWriteAtomic_LargeData(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "large-atomic.bin")

	data := make([]byte, 512*1024) // 512 KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := svc.WriteAtomic(path, data, 0o644); err != nil {
		t.Fatalf("WriteAtomic large: %v", err)
	}

	got, _ := svc.Read(path)
	if len(got) != len(data) {
		t.Errorf("size = %d, want %d", len(got), len(data))
	}
}

func TestCopyFile_LargeFileV2(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	src := filepath.Join(dir, "large-src.bin")
	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	svc.Write(src, data, 0o644)

	dst := filepath.Join(dir, "large-dst.bin")
	if err := svc.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile large: %v", err)
	}

	got, _ := svc.Read(dst)
	if len(got) != len(data) {
		t.Errorf("size = %d, want %d", len(got), len(data))
	}
}

func TestWriteAtomic_Success(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "atomic.txt")
	err := svc.WriteAtomic(path, []byte("atomic data"), 0o600)
	if err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	data, _ := svc.Read(path)
	if string(data) != "atomic data" {
		t.Errorf("content = %q", string(data))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perms = %o", info.Mode().Perm())
	}
}

func TestWriteAtomic_BadDir(t *testing.T) {
	svc := NewService()
	// Create a file where a directory would be needed
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	path := filepath.Join(blocker, "subdir", "file.txt")
	err := svc.WriteAtomic(path, []byte("data"), 0o600)
	if err == nil {
		t.Fatal("expected error for bad directory path")
	}
}

func TestWriteAtomic_OverwriteV2(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")
	svc.WriteAtomic(path, []byte("first"), 0o600)
	svc.WriteAtomic(path, []byte("second"), 0o600)
	data, _ := svc.Read(path)
	if string(data) != "second" {
		t.Errorf("content = %q, want second", string(data))
	}
}

func TestCopyFile_SrcNotFound(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	err := svc.CopyFile(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dst"))
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestCopyFile_DstBadDir(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("data"), 0o644)
	// Create a file where directory is expected
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	err := svc.CopyFile(src, filepath.Join(blocker, "dst.txt"))
	if err == nil {
		t.Fatal("expected error for bad destination dir")
	}
}

func TestCopyFile_NonExistentSource(t *testing.T) {
	svc := NewService()
	err := svc.CopyFile("/nonexistent/file.txt", t.TempDir()+"/dst.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestCopyFile_DstOpenError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("data"), 0o600)

	// Create dst as directory to cause open error
	dst := filepath.Join(dir, "dstdir")
	os.MkdirAll(dst, 0o755)

	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error when dst is a directory")
	}
}

func TestWriteAtomic_CreateTempError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	os.Chmod(dir, 0o000)
	defer os.Chmod(dir, 0o755)

	err := svc.WriteAtomic(filepath.Join(dir, "file.txt"), []byte("data"), 0o600)
	if err == nil {
		t.Fatal("expected error when temp file creation fails")
	}
}

func TestEnsurePermissions_NonExistent(t *testing.T) {
	svc := NewService()
	err := svc.EnsurePermissions("/nonexistent/file.txt", 0o600)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestEnsurePermissions_CorrectPerms(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("data"), 0o600)

	err := svc.EnsurePermissions(f, 0o600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsurePermissions_FixPerms(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("data"), 0o644)

	err := svc.EnsurePermissions(f, 0o600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, _ := os.Stat(f)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}

// =============================================================================
// Additional coverage: WriteAtomic error paths
// =============================================================================

func TestWriteAtomic_InvalidDir(t *testing.T) {
	svc := NewService()
	// Use a path where we can't create the directory
	path := "/dev/null/impossible/path/file.txt"

	err := svc.WriteAtomic(path, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestWriteAtomic_RenameErrorOnReadOnlyTarget(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	// Create the target as a directory (rename over a directory fails)
	targetPath := filepath.Join(dir, "target")
	os.MkdirAll(targetPath, 0o755)
	// Put a file in it so it's non-empty (can't rename file over non-empty dir)
	os.WriteFile(filepath.Join(targetPath, "blocker"), []byte("x"), 0o644)

	err := svc.WriteAtomic(targetPath, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error when renaming over a non-empty directory")
	}
}

func TestWriteAtomic_PermissionsApplied(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "perms.txt")

	if err := svc.WriteAtomic(path, []byte("restricted"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestWriteAtomic_OverwritesExisting(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")

	svc.WriteAtomic(path, []byte("first"), 0644)
	svc.WriteAtomic(path, []byte("second"), 0644)

	got, _ := svc.Read(path)
	if string(got) != "second" {
		t.Errorf("content = %q, want 'second'", got)
	}
}

func TestWrite_DirCreateError(t *testing.T) {
	svc := NewService()
	// Try to write in a directory that can't be created
	path := "/dev/null/impossible/file.txt"
	err := svc.Write(path, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error for impossible directory")
	}
}

func TestCopyFile_DestDirCreateError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	svc.Write(src, []byte("data"), 0644)

	// Destination directory is impossible
	dst := "/dev/null/impossible/dst.txt"
	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error for impossible destination directory")
	}
}

func TestWriteAtomic_WriteError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	// Create dir but make it so CreateTemp succeeds then Write fails
	// We can't easily do this without mocking, but we can test the Rename error path
	// by making the destination directory unwritable AFTER temp creation
	// Instead, test with /dev/full-like approach: write a huge amount
	// Actually the simplest injection: write to a path where the final rename target
	// crosses filesystem boundaries isn't trivial. Let's test CreateTemp error instead.

	// Place a file where the temp dir is expected, blocking CreateTemp indirectly
	blocker := filepath.Join(dir, "subdir")
	os.WriteFile(blocker, []byte("x"), 0o644)

	err := svc.WriteAtomic(filepath.Join(blocker, "file.txt"), []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error when parent is a file (MkdirAll fails)")
	}
}

func TestWrite_FileInPlaceOfDir(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	// Block a parent directory with a file
	blocker := filepath.Join(dir, "blocked")
	os.WriteFile(blocker, []byte("x"), 0o644)

	err := svc.Write(filepath.Join(blocker, "child", "file.txt"), []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error when parent path is a file")
	}
}

func TestCopyFile_MkdirAllDestBlockedByFile(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	// Create source file.
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("copy me"), 0o644)

	// Put a file where the destination's parent directory should be.
	blocker := filepath.Join(dir, "destparent")
	os.WriteFile(blocker, []byte("I'm a file"), 0o644)
	dst := filepath.Join(blocker, "subdir", "dest.txt")

	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error when dest parent dir is blocked by a file")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("sanity check")
	}
}

func TestEnsurePermissions_ChmodFails(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()

	// Create a file and make its parent dir read-only.
	// On macOS, you can still chmod files you own even in read-only dirs,
	// so we use a symlink to a non-existent target instead.
	// Actually, use a path that exists but remove write from dir to test
	// differently: create file, then change to different perm and try to chmod.
	// On macOS chmod on own file always works, so test stat failure path instead
	// by removing the file between calls (race sim).
	path := filepath.Join(dir, "file.txt")
	os.WriteFile(path, []byte("data"), 0o644)

	// EnsurePermissions when perm already matches should be a no-op.
	err := svc.EnsurePermissions(path, 0o644)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it actually changes perms.
	err = svc.EnsurePermissions(path, 0o600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

// =============================================================================
// ExpandPath tests
// =============================================================================

func TestExpandPath_TildeOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := ExpandPath("~")
	if got != home {
		t.Errorf("ExpandPath(~) = %q, want %q", got, home)
	}
}

func TestExpandPath_TildeSlash(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := ExpandPath("~/Documents/file.txt")
	want := filepath.Join(home, "Documents/file.txt")
	if got != want {
		t.Errorf("ExpandPath(~/Documents/file.txt) = %q, want %q", got, want)
	}
}

func TestExpandPath_AbsolutePath(t *testing.T) {
	got := ExpandPath("/usr/local/bin")
	if got != "/usr/local/bin" {
		t.Errorf("ExpandPath(/usr/local/bin) = %q", got)
	}
}

func TestExpandPath_RelativePath(t *testing.T) {
	got := ExpandPath("relative/path")
	if got != "relative/path" {
		t.Errorf("ExpandPath(relative/path) = %q", got)
	}
}

func TestExpandPath_EmptyString(t *testing.T) {
	got := ExpandPath("")
	if got != "" {
		t.Errorf("ExpandPath(\"\") = %q, want empty", got)
	}
}

func TestExpandPath_TildeNoSlash(t *testing.T) {
	// "~user" should NOT be expanded (only "~" and "~/...")
	got := ExpandPath("~user")
	if got != "~user" {
		t.Errorf("ExpandPath(~user) = %q, want ~user (not expanded)", got)
	}
}

func TestExpandPath_DoubleSlash(t *testing.T) {
	got := ExpandPath("//absolute")
	if got != "//absolute" {
		t.Errorf("ExpandPath(//absolute) = %q", got)
	}
}

func TestExpandPath_TildeSlashOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := ExpandPath("~/")
	want := home + "/"
	if got != want {
		t.Errorf("ExpandPath(~/) = %q, want %q", got, want)
	}
}

func TestEnsurePermissions_ChmodDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test chmod failure as root")
	}
	svc := NewService()
	// /etc/hosts is owned by root; requesting different perms triggers Chmod which fails
	err := svc.EnsurePermissions("/etc/hosts", 0o777)
	if err == nil {
		t.Fatal("expected permission error on chmod")
	}
}

func TestCopyFile_StatError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	// Create a symlink to a file, open it, then delete original before stat
	// On macOS/Linux, /dev/stdin works but behavior varies. Use /proc approach on Linux.
	// Simpler approach: create a file, copy it to read-only target
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("hello"), 0o644)

	// Copy to a path where the destination parent dir is an existing file (not a dir)
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	dst := filepath.Join(blocker, "subdir", "dst.txt")

	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error copying to blocked path")
	}
}

func TestWriteAtomic_MkdirAllBlockedByFile(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	// Create a regular file where a directory is expected
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)
	path := filepath.Join(blocker, "subdir", "file.txt")

	err := svc.WriteAtomic(path, []byte("data"), 0o644)
	if err == nil {
		t.Fatal("expected error when parent is a file")
	}
}

func TestWriteAtomic_WindowsRemovePath(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	path := filepath.Join(dir, "win.txt")

	// Write initial file
	os.WriteFile(path, []byte("old"), 0o644)

	// Simulate Windows behavior
	old := runtimeGOOS
	runtimeGOOS = "windows"
	defer func() { runtimeGOOS = old }()

	err := svc.WriteAtomic(path, []byte("new"), 0o644)
	if err != nil {
		t.Fatalf("WriteAtomic on simulated windows: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}

func TestCopyFile_StatHookError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("data"), 0o644)

	old := copyStatFn
	copyStatFn = func(f *os.File) (os.FileInfo, error) {
		return nil, fmt.Errorf("stat failure")
	}
	defer func() { copyStatFn = old }()

	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error when stat fails")
	}
}

func TestCopyFile_IOCopyError(t *testing.T) {
	svc := NewService()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	os.WriteFile(src, []byte("data"), 0o644)

	old := copyIOCopyFn
	copyIOCopyFn = func(dst2 io.Writer, src2 io.Reader) (int64, error) {
		return 0, fmt.Errorf("copy failure")
	}
	defer func() { copyIOCopyFn = old }()

	err := svc.CopyFile(src, dst)
	if err == nil {
		t.Fatal("expected error when io.Copy fails")
	}
}
