package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.ProfilesDir = filepath.Join(tmp, ".gcm", "profiles")
	cfg.TemplatesDir = filepath.Join(tmp, ".gcm", "templates")

	os.MkdirAll(cfg.ProfilesDir, 0o755)
	os.MkdirAll(cfg.TemplatesDir, 0o755)

	log := logger.New(logger.LevelError, os.Stderr)
	return NewManager(cfg, log), tmp
}

func isolateBackupHooks(t *testing.T) {
	t.Helper()
	oldOpenFileFn := osOpenFileFn
	oldAbsFn := backupAbsFn
	oldRelFn := backupRelFn
	oldMkdirFn := backupMkdirFn
	oldRemoveFn := backupRemoveFn
	oldReadDirFn := backupReadDirFn
	oldRestoreMkdirTempFn := restoreMkdirTempFn
	oldRestoreMkdirFn := restoreMkdirFn
	oldRestoreRemoveFn := restoreRemoveFn
	oldRestoreRenameFn := restoreRenameFn
	oldRestoreChmodFn := restoreChmodFn
	oldRestoreLstatFn := restoreLstatFn
	oldRestoreStatFn := restoreStatFn
	oldRestoreRelFn := restoreRelFn
	oldRestoreEntryInfoFn := restoreEntryInfoFn
	t.Cleanup(func() {
		osOpenFileFn = oldOpenFileFn
		backupAbsFn = oldAbsFn
		backupRelFn = oldRelFn
		backupMkdirFn = oldMkdirFn
		backupRemoveFn = oldRemoveFn
		backupReadDirFn = oldReadDirFn
		restoreMkdirTempFn = oldRestoreMkdirTempFn
		restoreMkdirFn = oldRestoreMkdirFn
		restoreRemoveFn = oldRestoreRemoveFn
		restoreRenameFn = oldRestoreRenameFn
		restoreChmodFn = oldRestoreChmodFn
		restoreLstatFn = oldRestoreLstatFn
		restoreStatFn = oldRestoreStatFn
		restoreRelFn = oldRestoreRelFn
		restoreEntryInfoFn = oldRestoreEntryInfoFn
	})
}

type testArchiveEntry struct {
	name string
	typ  byte
	mode int64
	data []byte
}

func writeTestArchive(t *testing.T, entries ...testArchiveEntry) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	for _, entry := range entries {
		typ := entry.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o600
		}
		header := &tar.Header{Name: entry.name, Typeflag: typ, Mode: mode, Size: int64(len(entry.data))}
		if typ == tar.TypeDir {
			header.Size = 0
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := tw.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

func TestCreateAndList(t *testing.T) {
	m, _ := testManager(t)

	// Write a dummy profile so backup has content
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "test.yaml"), []byte("name: test\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Size == 0 {
		t.Error("expected non-zero backup size")
	}
	if info.Profiles != 1 {
		t.Errorf("expected 1 profile, got %d", info.Profiles)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(list))
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	m, tmp := testManager(t)

	profileData := []byte("name: roundtrip\n")
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "roundtrip.yaml"), profileData, 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete the profile and restore
	os.Remove(filepath.Join(m.cfg.ProfilesDir, "roundtrip.yaml"))
	if err := m.Restore(info.Path); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(tmp, ".gcm", "profiles", "roundtrip.yaml"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(restored) != string(profileData) {
		t.Errorf("restored data mismatch: got %q", string(restored))
	}
}

func TestPrune(t *testing.T) {
	m, _ := testManager(t)

	for i := 0; i < 3; i++ {
		if _, err := m.Create(); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	removed, err := m.Prune(1)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	list, _ := m.List()
	if len(list) != 1 {
		t.Errorf("expected 1 backup after prune, got %d", len(list))
	}
}

func TestBackupFilePermissions(t *testing.T) {
	m, _ := testManager(t)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stat, err := os.Stat(info.Path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := stat.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected 0600 permissions, got %04o", perm)
	}
}

func TestCreate_EncryptedBackupFailsClosed(t *testing.T) {
	m, _ := testManager(t)
	m.cfg.Backup.Encryption = true

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "encrypted backups are not implemented") {
		t.Fatalf("expected encrypted backup fail-closed error, got: %v", err)
	}
}

func TestCreate_IncludeKeysFailsClosed(t *testing.T) {
	m, _ := testManager(t)
	m.cfg.Backup.IncludeKeys = true

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "backup.include_keys") {
		t.Fatalf("expected include_keys fail-closed error, got: %v", err)
	}
}

func TestListEmpty(t *testing.T) {
	m, _ := testManager(t)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestRestore_InvalidPath(t *testing.T) {
	m, _ := testManager(t)
	err := m.Restore("/nonexistent/path.tar.gz")
	if err == nil {
		t.Fatal("expected error for nonexistent backup path")
	}
}

func TestRestore_InvalidGzip(t *testing.T) {
	m, _ := testManager(t)

	// Write a non-gzip file
	badPath := filepath.Join(t.TempDir(), "bad.tar.gz")
	os.WriteFile(badPath, []byte("not gzip data"), 0o600)

	err := m.Restore(badPath)
	if err == nil {
		t.Fatal("expected error for non-gzip file")
	}
}

func TestRestore_ZipSlipRejected(t *testing.T) {
	m, _ := testManager(t)

	// Create a malicious tar.gz with a traversal path
	malPath := filepath.Join(t.TempDir(), "evil.tar.gz")
	f, err := os.Create(malPath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Write a file with path traversal
	header := &tar.Header{
		Name: "../../etc/evil",
		Mode: 0o600,
		Size: 4,
	}
	tw.WriteHeader(header)
	tw.Write([]byte("evil"))
	tw.Close()
	gzw.Close()
	f.Close()

	err = m.Restore(malPath)
	if err == nil {
		t.Fatal("expected error for zip-slip path")
	}
}

func TestRestore_ExtractionFailureLeavesExistingDataUnchanged(t *testing.T) {
	m, tmp := testManager(t)

	profilePath := filepath.Join(tmp, ".gcm", "profiles", "current.yaml")
	if err := os.WriteFile(profilePath, []byte("name: original\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "partial-fail.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	changed := []byte("name: changed\n")
	if err := tw.WriteHeader(&tar.Header{Name: "profiles/current.yaml", Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(changed))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(changed); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "../evil", Typeflag: tar.TypeReg, Mode: 0o600, Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gzw.Close()
	f.Close()

	if err := m.Restore(archivePath); err == nil {
		t.Fatal("expected restore extraction failure")
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "name: original\n" {
		t.Fatalf("profile changed despite failed restore: %q", string(data))
	}
}

func TestRestore_DirectoryEntry(t *testing.T) {
	m, _ := testManager(t)

	// Create a tar.gz with a directory entry and a file inside it
	archivePath := filepath.Join(t.TempDir(), "withdir.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a directory entry
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})

	// Add a file in that directory
	data := []byte("name: dirtest\n")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/dirtest.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
	})
	tw.Write(data)

	tw.Close()
	gzw.Close()
	f.Close()

	err = m.Restore(archivePath)
	if err != nil {
		t.Fatalf("Restore with dir entry: %v", err)
	}
}

func TestRestore_SymlinkSkipped(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "symlink.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a symlink entry - should be skipped
	tw.WriteHeader(&tar.Header{
		Name:     "badlink",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	})

	tw.Close()
	gzw.Close()
	f.Close()

	// Should succeed (skips the symlink without error)
	err = m.Restore(archivePath)
	if err != nil {
		t.Fatalf("Restore with symlink: %v", err)
	}
}

func TestPrune_KeepMoreThanExisting(t *testing.T) {
	m, _ := testManager(t)
	if _, err := m.Create(); err != nil {
		t.Fatal(err)
	}

	removed, err := m.Prune(5)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed when keep > count, got %d", removed)
	}
}

func TestPrune_KeepLessThanOne(t *testing.T) {
	m, _ := testManager(t)
	_, err := m.Prune(0)
	if err == nil {
		t.Fatal("expected error for keep < 1")
	}
}

func TestPruneOlderThan(t *testing.T) {
	m, tmp := testManager(t)
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(backupDir, "gcm-backup-old.tar.gz")
	newPath := filepath.Join(backupDir, "gcm-backup-new.tar.gz")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().AddDate(0, 0, -45)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	removed, err := m.PruneOlderThan(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old backup still exists or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new backup removed unexpectedly: %v", err)
	}
}

func TestEnforceRetentionHandlesPruneErrors(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	m.cfg.Backup.RetentionDays = 30
	m.cfg.Backup.MaxBackups = 1
	backupReadDirFn = func(string) ([]os.DirEntry, error) {
		return nil, fmt.Errorf("read boom")
	}

	m.enforceRetention()
}

func TestEnforceRetentionPrunesByAgeAndCount(t *testing.T) {
	m, tmp := testManager(t)
	m.cfg.Backup.RetentionDays = 30
	m.cfg.Backup.MaxBackups = 1
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(backupDir, "gcm-backup-old.tar.gz")
	newPath := filepath.Join(backupDir, "gcm-backup-new.tar.gz")
	newerPath := filepath.Join(backupDir, "gcm-backup-newer.tar.gz")
	for _, path := range []string{oldPath, newPath, newerPath} {
		if err := os.WriteFile(path, []byte("backup"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().AddDate(0, 0, -45)
	newTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	m.enforceRetention()

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old backup still exists or stat failed: %v", err)
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Path != newerPath {
		t.Fatalf("remaining backups = %+v, want only %s", list, newerPath)
	}
}

func TestCreate_UnwritableBackupDir(t *testing.T) {
	m, tmp := testManager(t)

	// Create the .gcm dir as read-only so MkdirAll for backups fails
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	// Place a file where the backups directory should be
	os.WriteFile(filepath.Join(gcmDir, "backups"), []byte("blocker"), 0o644)

	_, err := m.Create()
	if err == nil {
		t.Fatal("expected error when backup directory creation fails")
	}
}

func TestCreate_WithTemplates(t *testing.T) {
	m, _ := testManager(t)

	// Write a profile AND a template
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p1.yaml"), []byte("name: p1\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "t1.yaml"), []byte("name: t1\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 1 {
		t.Errorf("Profiles = %d, want 1", info.Profiles)
	}
	if info.Templates != 1 {
		t.Errorf("Templates = %d, want 1", info.Templates)
	}
}

func TestCreate_SkipsNonYAMLFiles(t *testing.T) {
	m, _ := testManager(t)

	// Write non-yaml files
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "readme.txt"), []byte("ignore"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "valid.yaml"), []byte("name: valid\n"), 0o600)
	os.MkdirAll(filepath.Join(m.cfg.ProfilesDir, "subdir"), 0o755) // directories skipped

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 1 {
		t.Errorf("Profiles = %d, want 1 (should skip .txt and dirs)", info.Profiles)
	}
}

func TestList_UnreadableBackupDir(t *testing.T) {
	m, tmp := testManager(t)

	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o755)
	os.Chmod(backupDir, 0o000)
	t.Cleanup(func() { os.Chmod(backupDir, 0o755) })

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error listing unreadable backup directory")
	}
}

func TestPrune_RemoveFailure(t *testing.T) {
	m, tmp := testManager(t)

	// Create some backups
	for i := 0; i < 3; i++ {
		if _, err := m.Create(); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Make the backup directory read-only so os.Remove fails
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.Chmod(backupDir, 0o555)
	t.Cleanup(func() { os.Chmod(backupDir, 0o755) })

	removed, err := m.Prune(1)
	if err != nil {
		t.Fatalf("Prune unexpected error: %v", err)
	}
	// Remove should fail (dir is read-only), so removed should be 0
	if removed != 0 {
		t.Errorf("expected 0 removed (dir is read-only), got %d", removed)
	}
}

func TestRestore_AbsolutePathEntry(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "abspath.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	tw.WriteHeader(&tar.Header{
		Name:     "/etc/evil",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     4,
	})
	tw.Write([]byte("evil"))
	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for absolute path entry")
	}
}

func TestRestore_DotDotEntry(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "dotdot.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	tw.WriteHeader(&tar.Header{
		Name:     "../outside",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     4,
	})
	tw.Write([]byte("evil"))
	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for .. path entry")
	}
}

func TestRestore_CorruptTarEntry(t *testing.T) {
	m, _ := testManager(t)

	// Create a gzip-valid but tar-corrupt archive
	archivePath := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	// Write garbage that isn't valid tar
	gzw.Write([]byte("this is not valid tar data at all!"))
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for corrupt tar")
	}
}

func TestCreateWithTemplates(t *testing.T) {
	m, _ := testManager(t)

	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "tmpl.yaml"), []byte("name: tmpl\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "prof.yaml"), []byte("name: prof\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Templates != 1 {
		t.Errorf("expected 1 template, got %d", info.Templates)
	}
	if info.Profiles != 1 {
		t.Errorf("expected 1 profile, got %d", info.Profiles)
	}
}

func TestList_SkipsNonTarGz(t *testing.T) {
	m, tmp := testManager(t)
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)

	os.WriteFile(filepath.Join(backupDir, "notes.txt"), []byte("not a backup"), 0o600)
	os.WriteFile(filepath.Join(backupDir, "gcm-backup-2024.tar.gz"), []byte("fake"), 0o600)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 backup (skipping .txt), got %d", len(list))
	}
}

func TestList_SkipsDirectories(t *testing.T) {
	m, tmp := testManager(t)
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)
	os.MkdirAll(filepath.Join(backupDir, "subdir"), 0o700)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 backups (skipping dirs), got %d", len(list))
	}
}

func TestRestore_AbsolutePathRejected(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "abspath.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	header := &tar.Header{
		Name: "/etc/evil",
		Mode: 0o600,
		Size: 4,
	}
	tw.WriteHeader(header)
	tw.Write([]byte("evil"))
	tw.Close()
	gzw.Close()
	f.Close()

	err = m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for absolute path in archive")
	}
}

func TestPrune_NoBackups(t *testing.T) {
	m, _ := testManager(t)
	removed, err := m.Prune(3)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

func TestCreate_EmptyDirs(t *testing.T) {
	m, _ := testManager(t)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 0 {
		t.Errorf("expected 0 profiles, got %d", info.Profiles)
	}
	if info.Templates != 0 {
		t.Errorf("expected 0 templates, got %d", info.Templates)
	}
	if info.Size == 0 {
		t.Error("expected non-zero size even for empty backup")
	}
}

func TestList_SortedNewestFirst(t *testing.T) {
	m, _ := testManager(t)

	for i := 0; i < 3; i++ {
		if _, err := m.Create(); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 backups, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].Created.After(list[i-1].Created) {
			t.Errorf("backup %d (%v) is newer than %d (%v) — not sorted newest first",
				i, list[i].Created, i-1, list[i-1].Created)
		}
	}
}

func TestPrune_KeepLessThanOneV2(t *testing.T) {
	m, _ := testManager(t)
	_, err := m.Prune(0)
	if err == nil {
		t.Fatal("expected error for keep=0")
	}
	_, err = m.Prune(-1)
	if err == nil {
		t.Fatal("expected error for keep=-1")
	}
}

func TestCreate_BackupDirCreation(t *testing.T) {
	m, tmp := testManager(t)
	// Remove the backups dir to test that Create() creates it
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.RemoveAll(backupDir)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Path == "" {
		t.Error("expected non-empty backup path")
	}
}

func TestRestore_RoundTripWithMultipleFiles(t *testing.T) {
	m, tmp := testManager(t)

	// Create multiple profiles and templates
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p1.yaml"), []byte("name: p1\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p2.yaml"), []byte("name: p2\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "t1.yaml"), []byte("name: t1\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 2 {
		t.Errorf("expected 2 profiles, got %d", info.Profiles)
	}
	if info.Templates != 1 {
		t.Errorf("expected 1 template, got %d", info.Templates)
	}

	// Delete all files
	os.RemoveAll(filepath.Join(tmp, ".gcm", "profiles"))
	os.RemoveAll(filepath.Join(tmp, ".gcm", "templates"))

	// Restore
	if err := m.Restore(info.Path); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify files are restored
	data, err := os.ReadFile(filepath.Join(tmp, ".gcm", "profiles", "p1.yaml"))
	if err != nil {
		t.Fatalf("reading restored p1: %v", err)
	}
	if string(data) != "name: p1\n" {
		t.Errorf("p1 content = %q", string(data))
	}
}

func TestRestore_DotDotInNameRejected(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "dotdot.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	header := &tar.Header{
		Name:     "profiles/../../etc/evil",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     4,
	}
	tw.WriteHeader(header)
	tw.Write([]byte("evil"))
	tw.Close()
	gzw.Close()
	f.Close()

	err = m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for dotdot traversal in archive name")
	}
}

func TestPrune_ExactlyKeep(t *testing.T) {
	m, _ := testManager(t)

	for i := 0; i < 3; i++ {
		m.Create()
	}

	removed, err := m.Prune(3)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed when count == keep, got %d", removed)
	}
}

func TestAddToArchive_NonExistentFile(t *testing.T) {
	m, _ := testManager(t)

	// Create a tar writer to test addToArchive directly
	f, err := os.CreateTemp(t.TempDir(), "test.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()

	err = m.addToArchive(tw, "/nonexistent/file.yaml", "file.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent source file")
	}
}

func TestRestore_InvalidTarContent(t *testing.T) {
	m, _ := testManager(t)

	// Create a valid gzip file containing invalid tar content
	archivePath := filepath.Join(t.TempDir(), "badtar.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzw := gzip.NewWriter(f)
	gzw.Write([]byte("this is not a valid tar"))
	gzw.Close()
	f.Close()

	err = m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for invalid tar content")
	}
}

func TestCreate_EmptyProfilesAndTemplates(t *testing.T) {
	m, _ := testManager(t)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 0 {
		t.Errorf("expected 0 profiles, got %d", info.Profiles)
	}
	if info.Templates != 0 {
		t.Errorf("expected 0 templates, got %d", info.Templates)
	}
}

func TestPrune_RemovesOldest(t *testing.T) {
	m, _ := testManager(t)

	// Create 5 backups
	for i := 0; i < 5; i++ {
		if _, err := m.Create(); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	removed, err := m.Prune(2)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 3 {
		t.Errorf("expected 3 removed, got %d", removed)
	}

	list, _ := m.List()
	if len(list) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(list))
	}
}

func TestList_NonExistentBackupDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Don't create the backups dir
	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

func TestRestore_FilePermissionsClamped(t *testing.T) {
	m, tmp := testManager(t)

	// Create a tar with a file that has overly permissive mode
	archivePath := filepath.Join(t.TempDir(), "perms.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("sensitive data\n")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/test.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o777,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	if err := m.Restore(archivePath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredPath := filepath.Join(tmp, ".gcm", "profiles", "test.yaml")
	fi, err := os.Stat(restoredPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Permissions should be clamped to owner-only (0o700 mask)
	perm := fi.Mode().Perm()
	if perm > 0o700 {
		t.Errorf("permissions = %o, should be clamped to owner-only", perm)
	}
}

func TestCreate_UnreadableProfileDir(t *testing.T) {
	m, _ := testManager(t)

	// Write a valid profile
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "good.yaml"), []byte("name: good\n"), 0o600)
	// Make profiles dir unreadable so os.ReadDir fails
	os.Chmod(m.cfg.ProfilesDir, 0o000)
	defer os.Chmod(m.cfg.ProfilesDir, 0o755)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Profiles should be 0 since dir was unreadable
	if info.Profiles != 0 {
		t.Errorf("Profiles = %d, want 0", info.Profiles)
	}
}

func TestCreate_UnreadableTemplateDir(t *testing.T) {
	m, _ := testManager(t)

	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "good.yaml"), []byte("name: good\n"), 0o600)
	os.Chmod(m.cfg.TemplatesDir, 0o000)
	defer os.Chmod(m.cfg.TemplatesDir, 0o755)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Templates != 0 {
		t.Errorf("Templates = %d, want 0", info.Templates)
	}
}

func TestRestore_MkdirAllFails(t *testing.T) {
	m, tmp := testManager(t)

	// Create a valid backup first
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "test.yaml"), []byte("name: test\n"), 0o600)
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make .gcm directory read-only so Restore's MkdirAll fails
	gcmDir := filepath.Join(tmp, ".gcm")
	os.Chmod(gcmDir, 0o000)
	defer os.Chmod(gcmDir, 0o755)

	err = m.Restore(info.Path)
	if err == nil {
		t.Fatal("expected error when restore dir is unwritable")
	}
}

func TestPrune_ListError(t *testing.T) {
	m, tmp := testManager(t)

	// Create backups directory, then make it unreadable for List
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o755)
	os.WriteFile(filepath.Join(backupDir, "gcm-backup-test.tar.gz"), []byte("fake"), 0o600)

	os.Chmod(backupDir, 0o000)
	defer os.Chmod(backupDir, 0o755)

	_, err := m.Prune(1)
	if err == nil {
		t.Fatal("expected error when backup dir is unreadable")
	}
}

func TestList_ReadDirError(t *testing.T) {
	m, tmp := testManager(t)

	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o755)

	// Make backup dir unreadable (but it exists, so not IsNotExist)
	os.Chmod(backupDir, 0o000)
	defer os.Chmod(backupDir, 0o755)

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error when backup dir is unreadable")
	}
}

func TestRestore_FileWithZeroMode(t *testing.T) {
	m, tmp := testManager(t)

	// Create a tar with file that has 0 mode - should be set to 0600
	archivePath := filepath.Join(t.TempDir(), "zeromode.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("zero mode file\n")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/zero.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o000,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	if err := m.Restore(archivePath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredPath := filepath.Join(tmp, ".gcm", "profiles", "zero.yaml")
	fi, err := os.Stat(restoredPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Zero mode should be replaced with 0600
	perm := fi.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("permissions = %o, want 0600 for zero mode input", perm)
	}
}

func TestRestore_NestedDirectoryCreation(t *testing.T) {
	m, tmp := testManager(t)

	// Create a tar with deeply nested file
	archivePath := filepath.Join(t.TempDir(), "nested.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("nested file\n")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/subdir/deep.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	if err := m.Restore(archivePath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restoredPath := filepath.Join(tmp, ".gcm", "profiles", "subdir", "deep.yaml")
	content, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatalf("reading nested file: %v", err)
	}
	if string(content) != "nested file\n" {
		t.Errorf("content = %q", string(content))
	}
}

func TestRestore_CorruptArchive(t *testing.T) {
	m, tmp := testManager(t)

	// Create a corrupt backup file
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o755)
	corruptPath := filepath.Join(backupDir, "corrupt.tar.gz")
	os.WriteFile(corruptPath, []byte("not a tar.gz file"), 0o600)

	err := m.Restore(corruptPath)
	if err == nil {
		t.Fatal("expected error for corrupt archive")
	}
}

func TestAddToArchive_UnreadableFile(t *testing.T) {
	m, _ := testManager(t)

	// Create a file and make it unreadable
	srcPath := filepath.Join(t.TempDir(), "unreadable.yaml")
	os.WriteFile(srcPath, []byte("data"), 0o600)
	os.Chmod(srcPath, 0o000)
	defer os.Chmod(srcPath, 0o644)

	archivePath := filepath.Join(t.TempDir(), "test.tar")
	f, _ := os.Create(archivePath)
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()

	err := m.addToArchive(tw, srcPath, "unreadable.yaml")
	if err == nil {
		t.Fatal("expected error for unreadable source file")
	}
}

func TestCreate_ConfigFileBackup(t *testing.T) {
	m, tmp := testManager(t)

	// Write a config file to be included in backup
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.WriteFile(filepath.Join(gcmDir, "config.yaml"), []byte("default_profile: work\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Restore and verify config was backed up
	os.Remove(filepath.Join(gcmDir, "config.yaml"))
	if err := m.Restore(info.Path); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(gcmDir, "config.yaml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if string(data) != "default_profile: work\n" {
		t.Errorf("config content = %q", string(data))
	}
}

// =============================================================================
// Additional coverage: Restore error paths
// =============================================================================

func TestRestore_CorruptTarEntryV2(t *testing.T) {
	m, _ := testManager(t)

	// Create a valid gzip file with corrupted tar content
	archivePath := filepath.Join(t.TempDir(), "corrupt_tar.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gzw := gzip.NewWriter(f)
	// Write some garbage that's not valid tar
	gzw.Write([]byte("this is not valid tar content at all"))
	gzw.Close()
	f.Close()

	err = m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for corrupted tar")
	}
}

func TestRestore_CreateGCMDirError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	backupMkdirFn = func(string, os.FileMode) error { return fmt.Errorf("mkdir boom") }

	if err := m.Restore("missing.tar.gz"); err == nil || !strings.Contains(err.Error(), "creating GCM directory") {
		t.Fatalf("Restore error = %v", err)
	}
}

func TestExtractArchive_StagingAbsError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	archivePath := writeTestArchive(t)
	backupAbsFn = func(string) (string, error) { return "", fmt.Errorf("abs boom") }

	if err := m.extractArchive(archivePath, t.TempDir()); err == nil || !strings.Contains(err.Error(), "resolving restore staging dir") {
		t.Fatalf("extractArchive error = %v", err)
	}
}

func TestExtractArchive_CreateDirectoryError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	archivePath := writeTestArchive(t, testArchiveEntry{name: "profiles/test.yaml", data: []byte("name: test\n")})
	restoreMkdirFn = func(string, os.FileMode) error { return fmt.Errorf("mkdir boom") }

	if err := m.extractArchive(archivePath, t.TempDir()); err == nil || !strings.Contains(err.Error(), "creating directory") {
		t.Fatalf("extractArchive error = %v", err)
	}
}

func TestExtractArchive_CreateFileError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	archivePath := writeTestArchive(t, testArchiveEntry{name: "profiles/test.yaml", data: []byte("name: test\n")})
	osOpenFileFn = func(string, int, os.FileMode) (*os.File, error) { return nil, fmt.Errorf("open boom") }

	if err := m.extractArchive(archivePath, t.TempDir()); err == nil || !strings.Contains(err.Error(), "creating file") {
		t.Fatalf("extractArchive error = %v", err)
	}
}

func TestApplyStagedRestore_RollbackTempError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	restoreMkdirTempFn = func(string, string) (string, error) { return "", fmt.Errorf("mkdtemp boom") }

	if err := m.applyStagedRestore(t.TempDir(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "creating restore rollback directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_WalkError(t *testing.T) {
	m, _ := testManager(t)

	if err := m.applyStagedRestore(filepath.Join(t.TempDir(), "missing"), t.TempDir()); err == nil {
		t.Fatal("expected walk error for missing staging dir")
	}
}

func TestApplyStagedRestore_RelError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("name: rel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreRelFn = func(string, string) (string, error) { return "", fmt.Errorf("rel boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "rel boom") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_RollbackRestoresChangesAndCreatedDirs(t *testing.T) {
	m, _ := testManager(t)
	stagingDir := t.TempDir()
	gcmDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(stagingDir, "a_created"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "b_existing.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(stagingDir, "c_block"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gcmDir, "b_existing.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gcmDir, "c_block"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := m.applyStagedRestore(stagingDir, gcmDir); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(gcmDir, "b_existing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("rollback did not restore original file: %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(gcmDir, "a_created")); !os.IsNotExist(err) {
		t.Fatalf("rollback did not remove created dir: %v", err)
	}
}

func TestApplyStagedRestore_DirectoryStatError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(stagingDir, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	restoreStatFn = func(string) (os.FileInfo, error) { return nil, fmt.Errorf("stat boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "checking restore directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_DirectoryMkdirError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(stagingDir, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	restoreMkdirFn = func(string, os.FileMode) error { return fmt.Errorf("mkdir boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "creating directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_TargetCheckError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("name: target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreLstatFn = func(string) (os.FileInfo, error) { return nil, fmt.Errorf("lstat boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "checking restore target") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_EntryInfoError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("name: info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreEntryInfoFn = func(os.DirEntry) (os.FileInfo, error) { return nil, fmt.Errorf("info boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "reading staged file info") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_FileParentMkdirError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	gcmDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("name: mkdir\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreMkdirFn = func(path string, _ os.FileMode) error {
		if path == gcmDir {
			return fmt.Errorf("mkdir boom")
		}
		return os.MkdirAll(path, 0o700)
	}

	if err := m.applyStagedRestore(stagingDir, gcmDir); err == nil || !strings.Contains(err.Error(), "creating directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_RollbackDirectoryError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	gcmDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gcmDir, "file.yaml"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreMkdirFn = func(path string, mode os.FileMode) error {
		if strings.Contains(path, ".restore-rollback-") {
			return fmt.Errorf("rollback mkdir boom")
		}
		return os.MkdirAll(path, mode)
	}

	if err := m.applyStagedRestore(stagingDir, gcmDir); err == nil || !strings.Contains(err.Error(), "creating rollback directory") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_StageExistingRenameError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	gcmDir := t.TempDir()
	target := filepath.Join(gcmDir, "file.yaml")
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreRenameFn = func(oldPath, newPath string) error {
		if oldPath == target {
			return fmt.Errorf("rename boom")
		}
		return os.Rename(oldPath, newPath)
	}

	if err := m.applyStagedRestore(stagingDir, gcmDir); err == nil || !strings.Contains(err.Error(), "staging existing file for rollback") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestApplyStagedRestore_ApplyRenameErrorRollsBackOriginal(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	gcmDir := t.TempDir()
	staged := filepath.Join(stagingDir, "file.yaml")
	target := filepath.Join(gcmDir, "file.yaml")
	if err := os.WriteFile(staged, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreRenameFn = func(oldPath, newPath string) error {
		if oldPath == staged {
			return fmt.Errorf("apply boom")
		}
		return os.Rename(oldPath, newPath)
	}

	if err := m.applyStagedRestore(stagingDir, gcmDir); err == nil || !strings.Contains(err.Error(), "applying restored file") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("original file not restored: %q", string(data))
	}
}

func TestApplyStagedRestore_ChmodError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	stagingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stagingDir, "file.yaml"), []byte("name: chmod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreChmodFn = func(string, os.FileMode) error { return fmt.Errorf("chmod boom") }

	if err := m.applyStagedRestore(stagingDir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "setting restored file permissions") {
		t.Fatalf("applyStagedRestore error = %v", err)
	}
}

func TestPruneOlderThanListError(t *testing.T) {
	m, _ := testManager(t)
	isolateBackupHooks(t)
	backupReadDirFn = func(string) ([]os.DirEntry, error) { return nil, fmt.Errorf("read boom") }

	if _, err := m.PruneOlderThan(time.Now()); err == nil || !strings.Contains(err.Error(), "reading backups directory") {
		t.Fatalf("PruneOlderThan error = %v", err)
	}
}

func TestPruneOlderThanRemoveError(t *testing.T) {
	m, tmp := testManager(t)
	isolateBackupHooks(t)
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(backupDir, "gcm-backup-old.tar.gz")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -45)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	backupRemoveFn = func(string) error { return fmt.Errorf("remove boom") }

	removed, err := m.PruneOlderThan(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old backup should remain after remove failure: %v", err)
	}
}

func TestRestore_AbsolutePath(t *testing.T) {
	m, _ := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "abspath.tar.gz")
	f, _ := os.Create(archivePath)

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Write a file with absolute path
	data := []byte("evil data")
	tw.WriteHeader(&tar.Header{
		Name:     "/etc/evil",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error for absolute path in archive")
	}
}

func TestRestore_MultipleFiles(t *testing.T) {
	m, tmp := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "multi.tar.gz")
	f, _ := os.Create(archivePath)

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add multiple files
	for _, name := range []string{"profiles/a.yaml", "profiles/b.yaml", "templates/t.yaml"} {
		data := []byte("name: " + name + "\n")
		tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o600,
			Size:     int64(len(data)),
		})
		tw.Write(data)
	}

	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify files exist
	gcmDir := filepath.Join(tmp, ".gcm")
	for _, name := range []string{"profiles/a.yaml", "profiles/b.yaml", "templates/t.yaml"} {
		if _, err := os.Stat(filepath.Join(gcmDir, name)); err != nil {
			t.Errorf("file %q not restored: %v", name, err)
		}
	}
}

// =============================================================================
// Additional coverage: addToArchive - file open error
// =============================================================================

func TestCreate_SkipsUnreadableFiles(t *testing.T) {
	m, _ := testManager(t)

	// Write a profile that's readable
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "good.yaml"), []byte("name: good\n"), 0o600)

	// Create backup should succeed even if some files are problematic
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 1 {
		t.Errorf("expected 1 profile, got %d", info.Profiles)
	}
}

func TestCreate_MultipleProfilesAndTemplates(t *testing.T) {
	m, _ := testManager(t)

	// Write multiple profiles
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p1.yaml"), []byte("name: p1\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p2.yaml"), []byte("name: p2\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p3.yaml"), []byte("name: p3\n"), 0o600)
	// Write templates
	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "t1.yaml"), []byte("name: t1\n"), 0o600)
	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "t2.yaml"), []byte("name: t2\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Profiles != 3 {
		t.Errorf("expected 3 profiles, got %d", info.Profiles)
	}
	if info.Templates != 2 {
		t.Errorf("expected 2 templates, got %d", info.Templates)
	}
}

// =============================================================================
// Additional coverage: List - ignores non-tar.gz files and directories
// =============================================================================

func TestList_IgnoresNonTarGz(t *testing.T) {
	m, tmp := testManager(t)

	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)

	// Write some non-tar.gz files
	os.WriteFile(filepath.Join(backupDir, "readme.txt"), []byte("not a backup"), 0o644)
	os.MkdirAll(filepath.Join(backupDir, "subdir"), 0o755)

	// Write a valid tar.gz file
	validPath := filepath.Join(backupDir, "gcm-backup-2024-01-01-120000.tar.gz")
	f, _ := os.Create(validPath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)
	tw.Close()
	gzw.Close()
	f.Close()

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 backup, got %d", len(list))
	}
}

// --- Filesystem error injection tests for backup ---

func TestRestore_MkdirAllError(t *testing.T) {
	m, tmp := testManager(t)

	// Create archive with a file whose parent dir cannot be created
	archivePath := filepath.Join(t.TempDir(), "mkdirerr.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("hello")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/test.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	// Block "profiles" dir creation by putting a file there
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	blocker := filepath.Join(gcmDir, "profiles")
	os.WriteFile(blocker, []byte("blocker"), 0o644)
	// Make it immutable so MkdirAll can't remove it
	os.Chmod(blocker, 0o444)
	t.Cleanup(func() { os.Chmod(blocker, 0o644) })

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails during restore")
	}
}

func TestRestore_CreateFileError(t *testing.T) {
	m, tmp := testManager(t)

	// Create archive with a regular file
	archivePath := filepath.Join(t.TempDir(), "createerr.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("content")
	tw.WriteHeader(&tar.Header{
		Name:     "config.yaml",
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     int64(len(data)),
	})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	// Make GCM dir read-only so file creation fails
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.Chmod(gcmDir, 0o555)
	t.Cleanup(func() { os.Chmod(gcmDir, 0o755) })

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error when file creation fails during restore")
	}
}

func TestCreate_AddToArchiveWriteHeaderError(t *testing.T) {
	m, _ := testManager(t)

	// Create a profile that is a dangling symlink (Stat will fail)
	profPath := filepath.Join(m.cfg.ProfilesDir, "dangling.yaml")
	os.Symlink("/nonexistent/target", profPath)

	// Also create a valid profile so we can verify partial success
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "good.yaml"), []byte("name: good\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The dangling symlink profile should have been skipped
	if info.Profiles != 1 {
		t.Errorf("expected 1 profile (dangling skipped), got %d", info.Profiles)
	}
}

func TestCreate_AddToArchiveTemplateError(t *testing.T) {
	m, _ := testManager(t)

	// Create a template that is a dangling symlink
	tmplPath := filepath.Join(m.cfg.TemplatesDir, "dangling.yaml")
	os.Symlink("/nonexistent/target", tmplPath)

	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "good.yaml"), []byte("name: t\n"), 0o600)

	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info.Templates != 1 {
		t.Errorf("expected 1 template (dangling skipped), got %d", info.Templates)
	}
}

func TestRestore_DirEntryMkdirError(t *testing.T) {
	m, tmp := testManager(t)

	archivePath := filepath.Join(t.TempDir(), "direrr.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	tw.WriteHeader(&tar.Header{
		Name:     "newdir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})
	tw.Close()
	gzw.Close()
	f.Close()

	// Make gcm dir read-only so MkdirAll for the dir entry fails
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o755)
	os.Chmod(gcmDir, 0o555)
	t.Cleanup(func() { os.Chmod(gcmDir, 0o755) })

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error when dir creation fails in restore")
	}
}

func TestCreate_BackupDirNotWritable(t *testing.T) {
	m, tmp := testManager(t)
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p.yaml"), []byte("name: p\n"), 0o600)

	// Create the backup dir as read-only so file creation fails.
	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)
	os.Chmod(backupDir, 0o500)
	t.Cleanup(func() { os.Chmod(backupDir, 0o700) })

	_, err := m.Create()
	if err == nil {
		t.Fatal("expected error when backup dir is not writable")
	}
	if !strings.Contains(err.Error(), "creating backup file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestore_SymlinkEntrySkipped(t *testing.T) {
	m, tmp := testManager(t)

	// Build an archive containing a symlink entry.
	archivePath := filepath.Join(tmp, "test-symlink.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a regular file.
	data := []byte("hello")
	tw.WriteHeader(&tar.Header{Name: "valid.txt", Size: int64(len(data)), Mode: 0o600, Typeflag: tar.TypeReg})
	tw.Write(data)

	// Add a symlink entry (should be skipped).
	tw.WriteHeader(&tar.Header{Name: "link.txt", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})

	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(archivePath)
	if err != nil {
		t.Fatalf("Restore should succeed skipping symlinks: %v", err)
	}

	// Verify the regular file was extracted.
	gcmDir := filepath.Join(tmp, ".gcm")
	if _, err := os.Stat(filepath.Join(gcmDir, "valid.txt")); err != nil {
		t.Fatal("expected valid.txt to be restored")
	}
	// Verify symlink was NOT extracted.
	if _, err := os.Stat(filepath.Join(gcmDir, "link.txt")); err == nil {
		t.Fatal("symlink should not have been extracted")
	}
}

func TestRestore_FileParentDirCreateFails(t *testing.T) {
	m, tmp := testManager(t)

	// Build archive with a file under a subdirectory.
	archivePath := filepath.Join(tmp, "test-parentfail.tar.gz")
	f, _ := os.Create(archivePath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	data := []byte("content")
	tw.WriteHeader(&tar.Header{Name: "sub/deep/file.txt", Size: int64(len(data)), Mode: 0o600, Typeflag: tar.TypeReg})
	tw.Write(data)
	tw.Close()
	gzw.Close()
	f.Close()

	// Place a FILE at the "sub" path so MkdirAll for "sub/deep" fails.
	gcmDir := filepath.Join(tmp, ".gcm")
	os.MkdirAll(gcmDir, 0o700)
	os.WriteFile(filepath.Join(gcmDir, "sub"), []byte("blocker"), 0o644)

	err := m.Restore(archivePath)
	if err == nil {
		t.Fatal("expected error when parent dir creation is blocked by file")
	}
}

func TestList_DirUnreadable(t *testing.T) {
	_, tmp := testManager(t)

	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)
	os.WriteFile(filepath.Join(backupDir, "a.tar.gz"), []byte("x"), 0o600)
	os.Chmod(backupDir, 0o000)
	t.Cleanup(func() { os.Chmod(backupDir, 0o700) })

	cfg := config.DefaultConfig()
	cfg.ProfilesDir = filepath.Join(tmp, ".gcm", "profiles")
	cfg.TemplatesDir = filepath.Join(tmp, ".gcm", "templates")
	log := logger.New(logger.LevelError, os.Stderr)
	mgr := NewManager(cfg, log)

	_, err := mgr.List()
	if err == nil {
		t.Fatal("expected error when backup dir is unreadable")
	}
}

func TestList_NonTarGzFilesSkipped(t *testing.T) {
	m, tmp := testManager(t)

	backupDir := filepath.Join(tmp, ".gcm", "backups")
	os.MkdirAll(backupDir, 0o700)

	// Create various non-.tar.gz files that should be skipped
	os.WriteFile(filepath.Join(backupDir, "notes.txt"), []byte("hi"), 0o600)
	os.WriteFile(filepath.Join(backupDir, "backup.zip"), []byte("pk"), 0o600)
	os.MkdirAll(filepath.Join(backupDir, "subdir"), 0o700)

	// Also create a valid .tar.gz file
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "p.yaml"), []byte("name: p\n"), 0o600)
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only find the one valid tar.gz backup
	if len(list) != 1 {
		t.Errorf("expected 1 backup, got %d", len(list))
	}
	if list[0].Path != info.Path {
		t.Errorf("path mismatch: got %q, want %q", list[0].Path, info.Path)
	}
}

func TestRestore_DirectoryEntryV2(t *testing.T) {
	m, tmp := testManager(t)

	// Create a tar.gz with a directory entry
	backupPath := filepath.Join(tmp, "test-dir-entry.tar.gz")
	f, _ := os.Create(backupPath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a directory entry
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/",
		Typeflag: tar.TypeDir,
		Mode:     0o700,
	})

	// Add a regular file entry
	content := []byte("name: test\n")
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/test.yaml",
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
		Mode:     0o600,
	})
	tw.Write(content)

	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify the directory was created
	gcmDir := filepath.Join(tmp, ".gcm")
	info, err := os.Stat(filepath.Join(gcmDir, "profiles"))
	if err != nil {
		t.Fatalf("profiles dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("profiles should be a directory")
	}
}

func TestRestore_RejectsOversizedFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, log)

	// Create a backup archive with a file whose header declares a size
	// larger than maxExtractSize.
	backupPath := filepath.Join(tmp, "oversize.tar.gz")
	f, err := os.Create(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Declare a size greater than maxExtractSize (10 MiB) but write only a
	// small body — we just need the header to trigger the check.
	tw.WriteHeader(&tar.Header{
		Name:     "profiles/huge.yaml",
		Size:     maxExtractSize + 1,
		Typeflag: tar.TypeReg,
		Mode:     0o600,
	})
	// Write one byte; the archive is technically malformed but the size
	// check fires before any read.
	tw.Write([]byte("x"))
	tw.Close()
	gzw.Close()
	f.Close()

	err = m.Restore(backupPath)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum extract size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate_OpenFileError(t *testing.T) {
	m, _ := testManager(t)
	orig := osOpenFileFn
	osOpenFileFn = func(string, int, os.FileMode) (*os.File, error) {
		return nil, os.ErrPermission
	}
	defer func() { osOpenFileFn = orig }()

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "creating backup file") {
		t.Fatalf("expected open file error, got: %v", err)
	}
}

func TestCreate_TarCloseError(t *testing.T) {
	m, _ := testManager(t)
	orig := tarCloseFn
	tarCloseFn = func(tw *tar.Writer) error { return os.ErrClosed }
	defer func() { tarCloseFn = orig }()

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "finalizing tar") {
		t.Fatalf("expected tar close error, got: %v", err)
	}
}

func TestCreate_GzipCloseError(t *testing.T) {
	m, _ := testManager(t)
	orig := gzipCloseFn
	gzipCloseFn = func(gzw *gzip.Writer) error { return os.ErrClosed }
	defer func() { gzipCloseFn = orig }()

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "finalizing gzip") {
		t.Fatalf("expected gzip close error, got: %v", err)
	}
}

func TestCreate_FileStatError(t *testing.T) {
	m, _ := testManager(t)
	orig := fileStatFn
	fileStatFn = func(f *os.File) (os.FileInfo, error) { return nil, os.ErrInvalid }
	defer func() { fileStatFn = orig }()

	_, err := m.Create()
	if err == nil || !strings.Contains(err.Error(), "getting backup file size") {
		t.Fatalf("expected stat error, got: %v", err)
	}
}

func TestRestore_AbsError(t *testing.T) {
	m, _ := testManager(t)
	// Create a valid backup first
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "test.yaml"), []byte("name: test\n"), 0o600)
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	orig := backupAbsFn
	backupAbsFn = func(string) (string, error) { return "", os.ErrInvalid }
	defer func() { backupAbsFn = orig }()

	err = m.Restore(info.Path)
	if err == nil || !strings.Contains(err.Error(), "resolving GCM dir") {
		t.Fatalf("expected abs error, got: %v", err)
	}
}

func TestRestore_MkdirError(t *testing.T) {
	m, tmp := testManager(t)
	// Create a backup with a directory entry to hit the TypeDir mkdir path
	backupPath := filepath.Join(tmp, ".gcm", "backups", "test-dir.tar.gz")
	os.MkdirAll(filepath.Dir(backupPath), 0o700)

	f, _ := os.Create(backupPath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a directory entry
	hdr := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "profiles/",
		Mode:     0o700,
	}
	tw.WriteHeader(hdr)
	tw.Close()
	gzw.Close()
	f.Close()

	orig := restoreMkdirFn
	restoreMkdirFn = func(string, os.FileMode) error { return os.ErrPermission }
	defer func() { restoreMkdirFn = orig }()

	err := m.Restore(backupPath)
	if err == nil || !strings.Contains(err.Error(), "creating directory") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

func TestRestore_CopyError(t *testing.T) {
	m, tmp := testManager(t)
	// Create a backup with a file that will trigger io.Copy failure.
	// We'll create a corrupted tar entry to trigger reading error.
	backupPath := filepath.Join(tmp, ".gcm", "backups", "test-corrupt.tar.gz")
	os.MkdirAll(filepath.Dir(backupPath), 0o700)

	f, _ := os.Create(backupPath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Write a header claiming more content than we provide.
	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "profiles/broken.yaml",
		Size:     1000, // claim 1000 bytes
		Mode:     0o600,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte("short")) // only 5 bytes
	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(backupPath)
	if err == nil || !strings.Contains(err.Error(), "writing file") {
		t.Fatalf("expected copy/write error, got: %v", err)
	}
}

func TestList_ReadDirNonNotExistError(t *testing.T) {
	m, _ := testManager(t)
	orig := backupReadDirFn
	backupReadDirFn = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	defer func() { backupReadDirFn = orig }()

	_, err := m.List()
	if err == nil || !strings.Contains(err.Error(), "reading backups directory") {
		t.Fatalf("expected readdir error, got: %v", err)
	}
}

type fakeDirEntry struct {
	name string
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() os.FileMode          { return 0 }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return nil, os.ErrPermission }

func TestList_EntryInfoError(t *testing.T) {
	m, _ := testManager(t)
	orig := backupReadDirFn
	backupReadDirFn = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "gcm-backup-2024.tar.gz"}}, nil
	}
	defer func() { backupReadDirFn = orig }()

	list, err := m.List()
	if err != nil {
		t.Fatalf("List should not error on individual Info() failures, got: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 backups (Info failed), got %d", len(list))
	}
}

func TestCreate_AddToArchiveWriteHeaderHookError(t *testing.T) {
	m, _ := testManager(t)
	// Write a profile so addToArchive is called
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "test.yaml"), []byte("name: test\n"), 0o600)

	orig := tarWriteHeaderFn
	tarWriteHeaderFn = func(tw *tar.Writer, hdr *tar.Header) error { return os.ErrClosed }
	defer func() { tarWriteHeaderFn = orig }()

	// Create should still succeed (addToArchive errors are logged and skipped)
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create should succeed (addToArchive errors are non-fatal), got: %v", err)
	}
	if info.Profiles != 0 {
		t.Errorf("expected 0 profiles (all failed), got %d", info.Profiles)
	}
}

func TestRestore_OversizedHeader(t *testing.T) {
	m, tmp := testManager(t)
	// Create a backup with a header claiming > maxExtractSize
	backupPath := filepath.Join(tmp, ".gcm", "backups", "test-oversize.tar.gz")
	os.MkdirAll(filepath.Dir(backupPath), 0o700)

	f, _ := os.Create(backupPath)
	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "profiles/big.yaml",
		Size:     11 << 20, // 11 MiB > maxExtractSize (10 MiB)
		Mode:     0o600,
	}
	tw.WriteHeader(hdr)
	tw.Close()
	gzw.Close()
	f.Close()

	err := m.Restore(backupPath)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum extract size") {
		t.Fatalf("expected oversize error, got: %v", err)
	}
}

func TestRestore_RelError(t *testing.T) {
	m, _ := testManager(t)
	// Create a valid backup
	os.WriteFile(filepath.Join(m.cfg.ProfilesDir, "test.yaml"), []byte("name: test\n"), 0o600)
	info, err := m.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	orig := backupRelFn
	backupRelFn = func(string, string) (string, error) { return "", os.ErrInvalid }
	defer func() { backupRelFn = orig }()

	err = m.Restore(info.Path)
	if err == nil || !strings.Contains(err.Error(), "refusing to extract unsafe path") {
		t.Fatalf("expected rel error, got: %v", err)
	}
}
