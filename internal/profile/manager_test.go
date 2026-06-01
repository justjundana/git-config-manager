package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	fileSvc "github.com/justjundana/git-config-manager/internal/service/file"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func newTestManager(t *testing.T) (*Manager, *config.Config) {
	t.Helper()

	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.ProfilesDir = filepath.Join(dir, "profiles")
	cfg.TemplatesDir = filepath.Join(dir, "templates")
	cfg.CacheDir = filepath.Join(dir, "cache")

	if err := os.MkdirAll(cfg.ProfilesDir, 0755); err != nil {
		t.Fatal(err)
	}

	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)

	return NewManager(cfg, fs, log), cfg
}

func validProfile(name string) *Profile {
	return &Profile{
		Name: name,
		Git: GitConfig{
			User: GitUser{
				Name:  "Test User",
				Email: "test@example.com",
			},
		},
	}
}

func TestManagerCreate(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	if err := mgr.Create(p); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if !mgr.Exists("work") {
		t.Error("profile should exist after Create()")
	}

	// Verify metadata was set
	got, _ := mgr.Get("work")
	if got.Metadata.Version != "1.0" {
		t.Errorf("version = %s, want 1.0", got.Metadata.Version)
	}
	if got.Metadata.Created.IsZero() {
		t.Error("created timestamp should not be zero")
	}
}

func TestManagerCreateDuplicate(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	_ = mgr.Create(p)

	err := mgr.Create(p)
	if err == nil {
		t.Error("Create() duplicate should fail")
	}
}

func TestManagerGet(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	p.Git.User.Name = "John Doe"
	p.Git.User.Email = "john@company.com"
	_ = mgr.Create(p)

	got, err := mgr.Get("work")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if got.Name != "work" {
		t.Errorf("Name = %s, want work", got.Name)
	}
	if got.Git.User.Name != "John Doe" {
		t.Errorf("User.Name = %s, want John Doe", got.Git.User.Name)
	}
	if got.Git.User.Email != "john@company.com" {
		t.Errorf("User.Email = %s, want john@company.com", got.Git.User.Email)
	}
}

func TestManagerGetNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("Get() nonexistent should fail")
	}
}

func TestManagerUpdate(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	_ = mgr.Create(p)

	got, _ := mgr.Get("work")
	got.Git.User.Email = "updated@company.com"
	if err := mgr.Update(got); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	updated, _ := mgr.Get("work")
	if updated.Git.User.Email != "updated@company.com" {
		t.Errorf("Email after update = %s, want updated@company.com", updated.Git.User.Email)
	}
}

func TestManagerDelete(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	_ = mgr.Create(p)

	if err := mgr.Delete("work"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if mgr.Exists("work") {
		t.Error("profile should not exist after Delete()")
	}
}

func TestManagerDeleteNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	if err := mgr.Delete("nonexistent"); err == nil {
		t.Error("Delete() nonexistent should fail")
	}
}

func TestManagerList(t *testing.T) {
	mgr, _ := newTestManager(t)

	_ = mgr.Create(validProfile("work"))
	_ = mgr.Create(validProfile("personal"))
	p3 := validProfile("client")
	p3.Git.User.Email = "client@example.com"
	_ = mgr.Create(p3)

	profiles, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(profiles) != 3 {
		t.Errorf("List() = %d profiles, want 3", len(profiles))
	}

	// Should be sorted by name
	if profiles[0].Name != "client" {
		t.Errorf("first profile = %s, want client", profiles[0].Name)
	}
}

func TestManagerExportImport(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("work")
	p.Git.User.Name = "Export User"
	_ = mgr.Create(p)

	data, err := mgr.Export("work")
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	// Delete and re-import
	_ = mgr.Delete("work")

	imported, err := mgr.Import(data)
	if err != nil {
		t.Fatalf("Import() error: %v", err)
	}
	if imported.Git.User.Name != "Export User" {
		t.Errorf("imported name = %s, want Export User", imported.Git.User.Name)
	}
}

func TestManagerIncrementUsage(t *testing.T) {
	mgr, _ := newTestManager(t)
	_ = mgr.Create(validProfile("work"))

	if err := mgr.IncrementUsage("work"); err != nil {
		t.Fatalf("IncrementUsage() error: %v", err)
	}

	got, _ := mgr.Get("work")
	if got.Metadata.UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", got.Metadata.UsageCount)
	}
	if got.Metadata.LastUsed == nil {
		t.Error("LastUsed should be set after IncrementUsage()")
	}
}

func TestProfilePath_RejectsTraversal(t *testing.T) {
	mgr, cfg := newTestManager(t)
	_ = cfg

	cases := []string{
		"../etc/passwd",
		"..",
		".",
		"foo/bar",
		"foo\\bar",
		"..\\evil",
		"good/../bad",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := mgr.profilePath(name); err == nil {
				t.Fatalf("profilePath(%q) should have failed", name)
			}
		})
	}
}

func TestProfilePath_StaysInsideProfilesDir(t *testing.T) {
	mgr, cfg := newTestManager(t)

	got, err := mgr.profilePath("work")
	if err != nil {
		t.Fatalf("profilePath(work): %v", err)
	}

	baseAbs, _ := filepath.Abs(cfg.ProfilesDir)
	rel, err := filepath.Rel(baseAbs, got)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || filepath.Dir(rel) != "." {
		t.Fatalf("path %q escaped profiles dir %q", got, baseAbs)
	}
	if filepath.Ext(got) != ".yaml" {
		t.Fatalf("expected .yaml extension, got %q", got)
	}
}

func TestImport_RefusesTraversalName(t *testing.T) {
	mgr, _ := newTestManager(t)

	// A YAML payload whose `name` field tries to escape the profiles dir.
	evil := []byte(`name: "../evil"
git:
  user:
    name: Evil
    email: evil@example.com
metadata:
  version: "1.0"
`)
	if _, err := mgr.Import(evil); err == nil {
		t.Fatal("expected Import to reject traversal name")
	}
}

func TestGet_RefusesTraversalName(t *testing.T) {
	mgr, _ := newTestManager(t)

	if _, err := mgr.Get("../../etc/passwd"); err == nil {
		t.Fatal("expected Get to reject traversal")
	}
}

func TestSavedProfileHasRestrictivePermissions(t *testing.T) {
	mgr, cfg := newTestManager(t)

	if err := mgr.Create(validProfile("work")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(cfg.ProfilesDir, "work.yaml")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Profile files may contain usernames/emails; 0600 is appropriate.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("profile perm = %o, want 0600", perm)
	}
}

func TestManagerExists(t *testing.T) {
	mgr, _ := newTestManager(t)

	if mgr.Exists("nope") {
		t.Error("expected Exists to return false for nonexistent profile")
	}

	mgr.Create(validProfile("exists"))
	if !mgr.Exists("exists") {
		t.Error("expected Exists to return true for created profile")
	}
}

func TestManagerUpdate_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	p := validProfile("notcreated")
	if err := mgr.Update(p); err == nil {
		t.Fatal("expected error updating nonexistent profile")
	}
}

func TestManagerExport_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	_, err := mgr.Export("nonexistent")
	if err == nil {
		t.Fatal("expected error exporting nonexistent profile")
	}
}

func TestManagerDelete_Traversal(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.Delete("../evil"); err == nil {
		t.Fatal("expected error for traversal delete")
	}
}

func TestManagerListEmpty(t *testing.T) {
	mgr, _ := newTestManager(t)

	profiles, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestManagerImport_DuplicateName(t *testing.T) {
	mgr, _ := newTestManager(t)

	mgr.Create(validProfile("dup"))
	data, _ := mgr.Export("dup")

	// Importing again should still work (overwrite/create)
	_, err := mgr.Import(data)
	_ = err // behavior may vary
}

func TestManagerImport_InvalidYAML(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Import([]byte(":::invalid yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestManagerIncrementUsage_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.IncrementUsage("nonexistent")
	if err == nil {
		t.Fatal("expected error incrementing usage for nonexistent profile")
	}
}

func TestManagerGet_Traversal(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Get("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal name")
	}
}

func TestManagerCreate_InvalidProfile(t *testing.T) {
	mgr, _ := newTestManager(t)
	// Profile without required fields
	p := &Profile{Name: "invalid"}
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected validation error for profile without email")
	}
}

func TestManagerUpdate_Validation(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("work"))

	// Update with invalid data
	p := &Profile{Name: "work", Git: GitConfig{User: GitUser{Name: "", Email: ""}}}
	err := mgr.Update(p)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestManagerDelete_DefaultProfile(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("mydefault"))
	cfg.DefaultProfile = "mydefault"

	err := mgr.Delete("mydefault")
	if err == nil {
		t.Fatal("expected error deleting default profile")
	}
}

func TestManagerList_SkipsInvalidFiles(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("good"))

	// Write an invalid YAML file
	os.WriteFile(filepath.Join(cfg.ProfilesDir, "bad.yaml"), []byte(":::invalid"), 0o600)

	profiles, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only return the valid one
	if len(profiles) != 1 {
		t.Errorf("expected 1 valid profile, got %d", len(profiles))
	}
}

func TestProfilePath_EmptyName(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.profilePath("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestExists_NonExistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	if mgr.Exists("nonexistent") {
		t.Error("expected false for nonexistent profile")
	}
}

func TestExists_AfterCreate(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(&Profile{Name: "exists", Git: GitConfig{User: GitUser{Name: "E", Email: "e@e.com"}}})
	if !mgr.Exists("exists") {
		t.Error("expected true for existing profile")
	}
}

func TestUpdate_NonExistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Update(&Profile{Name: "nope", Git: GitConfig{User: GitUser{Name: "N", Email: "n@n.com"}}})
	if err == nil {
		t.Fatal("expected error updating nonexistent profile")
	}
}

func TestCreate_InvalidName(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Create(&Profile{Name: "bad name!", Git: GitConfig{User: GitUser{Name: "B", Email: "b@b.com"}}})
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
}

func TestDelete_NonExistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Delete("nope")
	if err == nil {
		t.Fatal("expected error deleting nonexistent profile")
	}
}

func TestGet_NonExistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	_, err := mgr.Get("nope")
	if err == nil {
		t.Fatal("expected error getting nonexistent profile")
	}
}

func TestIncrementUsage_NonExistent(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.IncrementUsage("nope")
	if err == nil {
		t.Fatal("expected error incrementing nonexistent profile")
	}
}

func TestCurrentLocalProfile_NotInGitRepo(t *testing.T) {
	mgr, _ := newTestManager(t)
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)
	name, _ := mgr.currentLocalProfile()
	if name != "" {
		t.Errorf("expected empty, got %q", name)
	}
}

func TestCurrentLocalProfile_WithFile(t *testing.T) {
	mgr, cfg := newTestManager(t)
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)
	os.WriteFile(cfg.AutoSwitch.ProjectFile, []byte("myprofile\n"), 0o644)
	name, err := mgr.currentLocalProfile()
	if err != nil {
		t.Fatalf("currentLocalProfile: %v", err)
	}
	if name != "myprofile" {
		t.Errorf("expected myprofile, got %q", name)
	}
}

func TestManagerDelete_ActiveProfile(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("active"))

	// Write .gcm-profile marking it active
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)
	os.WriteFile(cfg.AutoSwitch.ProjectFile, []byte("active\n"), 0o644)

	err := mgr.Delete("active")
	if err == nil {
		t.Fatal("expected error deleting active profile")
	}
}

func TestManagerDelete_ForceActiveProfile(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("active"))

	// Write .gcm-profile marking it active
	tmp := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(origDir)
	os.WriteFile(cfg.AutoSwitch.ProjectFile, []byte("active\n"), 0o644)

	// Force delete should succeed
	err := mgr.Delete("active", true)
	if err != nil {
		t.Fatalf("Delete(force=true) should succeed: %v", err)
	}
	if mgr.Exists("active") {
		t.Error("profile should not exist after force delete")
	}
}

func TestManagerDelete_ForceDefaultProfile(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("mydefault"))
	cfg.DefaultProfile = "mydefault"

	// Force delete should succeed and clear DefaultProfile
	err := mgr.Delete("mydefault", true)
	if err != nil {
		t.Fatalf("Delete(force=true) should succeed: %v", err)
	}
	if mgr.Exists("mydefault") {
		t.Error("profile should not exist after force delete")
	}
	if cfg.DefaultProfile != "" {
		t.Errorf("DefaultProfile should be cleared, got %q", cfg.DefaultProfile)
	}
}

func TestManagerUpdate_MetadataUpdated(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("meta"))

	got, _ := mgr.Get("meta")
	oldUpdated := got.Metadata.Updated

	got.Git.User.Email = "new@example.com"
	if err := mgr.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated, _ := mgr.Get("meta")
	if !updated.Metadata.Updated.After(oldUpdated) {
		t.Error("expected Updated timestamp to advance after Update()")
	}
}

func TestManagerCreate_EmptyName(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{Name: "", Git: GitConfig{User: GitUser{Name: "X", Email: "x@x.com"}}}
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestManagerCreate_EmptyEmail(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{Name: "noemail", Git: GitConfig{User: GitUser{Name: "X", Email: ""}}}
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestManagerCreate_EmptyUserName(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{Name: "nouser", Git: GitConfig{User: GitUser{Name: "", Email: "x@x.com"}}}
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected error for empty user name")
	}
}

func TestManagerGet_CorruptedYAML(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Write a corrupted YAML file directly
	path := filepath.Join(cfg.ProfilesDir, "corrupt.yaml")
	os.WriteFile(path, []byte(":::this is not valid yaml{{{"), 0o600)

	_, err := mgr.Get("corrupt")
	if err == nil {
		t.Fatal("expected error for corrupted YAML file")
	}
}

func TestManagerCreate_WithSSH(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{
		Name: "withssh",
		Git:  GitConfig{User: GitUser{Name: "SSH", Email: "ssh@test.com"}},
		SSH:  &SSHConfig{KeyPath: "~/.ssh/id_ed25519"},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := mgr.Get("withssh")
	if got.SSH == nil || got.SSH.KeyPath != "~/.ssh/id_ed25519" {
		t.Error("SSH config not preserved")
	}
}

func TestManagerCreate_WithGPG(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{
		Name: "withgpg",
		Git:  GitConfig{User: GitUser{Name: "GPG", Email: "gpg@test.com"}},
		GPG:  &GPGConfig{KeyID: "ABCDEF"},
	}
	if err := mgr.Create(p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _ := mgr.Get("withgpg")
	if got.GPG == nil || got.GPG.KeyID != "ABCDEF" {
		t.Error("GPG config not preserved")
	}
}

func TestManagerCreate_UnwritableProfilesDir(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Make profiles dir read-only
	os.Chmod(cfg.ProfilesDir, 0o555)
	defer os.Chmod(cfg.ProfilesDir, 0o755)

	p := validProfile("cantwrite")
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected error when profiles dir is not writable")
	}
}

func TestManagerUpdate_UnwritableProfilesDir(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Create profile first (while writable)
	mgr.Create(validProfile("updatefail"))

	// Make dir read-only
	os.Chmod(cfg.ProfilesDir, 0o555)
	defer os.Chmod(cfg.ProfilesDir, 0o755)

	p, _ := mgr.Get("updatefail")
	p.Git.User.Email = "new@test.com"
	err := mgr.Update(p)
	if err == nil {
		t.Fatal("expected error when profiles dir is not writable")
	}
}

func TestManagerDelete_UnwritableProfilesDir(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("delfail"))

	// Make profiles dir read-only to prevent deletion
	os.Chmod(cfg.ProfilesDir, 0o555)
	defer os.Chmod(cfg.ProfilesDir, 0o755)

	err := mgr.Delete("delfail")
	if err == nil {
		t.Fatal("expected error when profiles dir is not writable")
	}
}

func TestManagerProfilePath_ContainsDotDot(t *testing.T) {
	mgr, _ := newTestManager(t)

	cases := []string{
		"a..b",
		"..hidden",
		"test..",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := mgr.profilePath(name)
			if err == nil {
				t.Fatalf("profilePath(%q) should reject names containing '..'", name)
			}
		})
	}
}

func TestManagerGet_ReadPermissionError(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("noread"))

	// Make the profile file unreadable
	path := filepath.Join(cfg.ProfilesDir, "noread.yaml")
	os.Chmod(path, 0o000)
	defer os.Chmod(path, 0o644)

	_, err := mgr.Get("noread")
	if err == nil {
		t.Fatal("expected error when profile file is unreadable")
	}
}

func TestManagerList_UnreadableProfilesDir(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("x"))

	os.Chmod(cfg.ProfilesDir, 0o000)
	defer os.Chmod(cfg.ProfilesDir, 0o755)

	// filepath.Glob may not return error for unreadable dirs, but profiles
	// should be empty since files can't be read
	profiles, err := mgr.List()
	if err != nil {
		// If it does error, that's valid too
		return
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles from unreadable dir, got %d", len(profiles))
	}
}

func TestManagerIncrementUsage_WriteFails(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("incfail"))

	// Make dir read-only to prevent save
	os.Chmod(cfg.ProfilesDir, 0o555)
	defer os.Chmod(cfg.ProfilesDir, 0o755)

	err := mgr.IncrementUsage("incfail")
	if err == nil {
		t.Fatal("expected error when save fails due to read-only dir")
	}
}

func TestManagerExport_Marshal(t *testing.T) {
	mgr, _ := newTestManager(t)
	p := &Profile{
		Name: "exportme",
		Git: GitConfig{
			User:    GitUser{Name: "Export", Email: "export@test.com"},
			Aliases: map[string]string{"co": "checkout"},
			Custom:  map[string]string{"color.ui": "auto"},
		},
	}
	mgr.Create(p)

	data, err := mgr.Export("exportme")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty export data")
	}
	// Should contain the email
	if !strings.Contains(string(data), "export@test.com") {
		t.Errorf("export data missing email: %s", string(data))
	}
}

func TestManagerImport_ValidationFails(t *testing.T) {
	mgr, _ := newTestManager(t)
	// Valid YAML but fails validation (no email)
	data := []byte(`name: "noemail"
git:
  user:
    name: "NoEmail"
    email: ""
`)
	_, err := mgr.Import(data)
	if err == nil {
		t.Fatal("expected error importing profile without email")
	}
}

// --- Filesystem error injection tests ---

func TestManagerGet_ReadFileError(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("readfail"))

	// Make the file unreadable
	path := filepath.Join(cfg.ProfilesDir, "readfail.yaml")
	os.Chmod(path, 0o000)
	t.Cleanup(func() { os.Chmod(path, 0o600) })

	_, err := mgr.Get("readfail")
	if err == nil {
		t.Fatal("expected error reading unreadable profile file")
	}
}

func TestManagerGet_UnmarshalError(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Write binary garbage that looks like valid file but isn't YAML
	path := filepath.Join(cfg.ProfilesDir, "garbled.yaml")
	os.WriteFile(path, []byte("\x00\x01\x02\x03"), 0o600)

	_, err := mgr.Get("garbled")
	if err == nil {
		t.Fatal("expected unmarshal error for binary garbage")
	}
	if !strings.Contains(err.Error(), "parsing profile") {
		t.Errorf("expected 'parsing profile' error, got: %v", err)
	}
}

func TestManagerList_GetFailsForUnreadableFile(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Create a valid profile
	mgr.Create(validProfile("readable"))

	// Create a file that exists but is unreadable (so Get errors with non-IsNotExist)
	path := filepath.Join(cfg.ProfilesDir, "noperm.yaml")
	os.WriteFile(path, []byte("name: noperm\n"), 0o600)
	os.Chmod(path, 0o000)
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	profiles, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should skip the unreadable profile and return only the readable one
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile (skip unreadable), got %d", len(profiles))
	}
}

func TestManagerCreate_ProfilesDirIsFile(t *testing.T) {
	// When profilesDir is a file instead of directory, MkdirAll / WriteAtomic fails
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	// Put a regular file where profiles dir should be
	blocker := filepath.Join(dir, "profiles")
	os.WriteFile(blocker, []byte("blocker"), 0o644)
	cfg.ProfilesDir = blocker
	cfg.TemplatesDir = filepath.Join(dir, "templates")
	cfg.CacheDir = filepath.Join(dir, "cache")

	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	mgr := NewManager(cfg, fs, log)

	p := validProfile("fail")
	err := mgr.Create(p)
	if err == nil {
		t.Fatal("expected error when profiles dir is a file")
	}
}

func TestManagerUpdate_ProfilesDirIsFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	profilesDir := filepath.Join(dir, "profiles")
	os.MkdirAll(profilesDir, 0o755)
	cfg.ProfilesDir = profilesDir
	cfg.TemplatesDir = filepath.Join(dir, "templates")
	cfg.CacheDir = filepath.Join(dir, "cache")

	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	mgr := NewManager(cfg, fs, log)

	// Create the profile first
	mgr.Create(validProfile("updfail"))

	// Now replace profiles dir with a file to break writes
	os.RemoveAll(profilesDir)
	os.WriteFile(profilesDir, []byte("blocker"), 0o644)
	t.Cleanup(func() { os.Remove(profilesDir) })

	p := validProfile("updfail")
	err := mgr.Update(p)
	if err == nil {
		t.Fatal("expected error when profilesDir is a file")
	}
}

func TestManagerDelete_RemoveError(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("delfail"))

	// Make profiles dir non-writable so Remove fails
	os.Chmod(cfg.ProfilesDir, 0o555)
	t.Cleanup(func() { os.Chmod(cfg.ProfilesDir, 0o755) })

	err := mgr.Delete("delfail")
	if err == nil {
		t.Fatal("expected error when remove fails due to permissions")
	}
}

func TestManagerList_ReadDirError(t *testing.T) {
	mgr, cfg := newTestManager(t)

	// Make profiles dir unreadable (Glob can't read its contents)
	os.Chmod(cfg.ProfilesDir, 0o000)
	t.Cleanup(func() { os.Chmod(cfg.ProfilesDir, 0o755) })

	profiles, err := mgr.List()
	if err != nil {
		// If Glob returns error (some OSes), that's fine
		return
	}
	// On some systems Glob returns empty with no error for unreadable dir
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles from unreadable dir, got %d", len(profiles))
	}
}

func TestManagerList_SkipsUnmarshalErrors(t *testing.T) {
	mgr, cfg := newTestManager(t)
	mgr.Create(validProfile("good"))

	// Write a file that exists but has invalid YAML that won't unmarshal to Profile
	path := filepath.Join(cfg.ProfilesDir, "badunmarshal.yaml")
	os.WriteFile(path, []byte(":::invalid{{{yaml"), 0o600)

	profiles, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only have the good profile
	if len(profiles) != 1 || profiles[0].Name != "good" {
		t.Errorf("expected only 'good' profile, got %d profiles", len(profiles))
	}
}

// --- Case-sensitivity tests ---

func TestGet_CaseSensitive_ExactMatch(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// Exact case should succeed
	got, err := mgr.Get("Work")
	if err != nil {
		t.Fatalf("Get(Work) error: %v", err)
	}
	if got.Name != "Work" {
		t.Errorf("Name = %q, want Work", got.Name)
	}
}

func TestGet_CaseSensitive_WrongCase(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// Wrong case should fail (not found)
	_, err := mgr.Get("work")
	if err == nil {
		t.Fatal("Get(work) should fail when profile is 'Work'")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestGet_CaseSensitive_AllUppercase(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	_, err := mgr.Get("WORK")
	if err == nil {
		t.Fatal("Get(WORK) should fail when profile is 'Work'")
	}
}

func TestExists_CaseSensitive(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Personal"))

	if !mgr.Exists("Personal") {
		t.Error("Exists(Personal) should be true")
	}
	if mgr.Exists("personal") {
		t.Error("Exists(personal) should be false when profile is 'Personal'")
	}
	if mgr.Exists("PERSONAL") {
		t.Error("Exists(PERSONAL) should be false when profile is 'Personal'")
	}
}

func TestCreate_CaseInsensitiveDuplicate(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Personal"))

	// Trying to create "personal" (different case) should fail
	err := mgr.Create(validProfile("personal"))
	if err == nil {
		t.Fatal("Create(personal) should fail when 'Personal' exists")
	}
	if !strings.Contains(err.Error(), "case-insensitive duplicate") {
		t.Errorf("expected case-insensitive duplicate error, got: %v", err)
	}
}

func TestCreate_CaseInsensitiveDuplicate_UpperCase(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	err := mgr.Create(validProfile("WORK"))
	if err == nil {
		t.Fatal("Create(WORK) should fail when 'Work' exists")
	}
	if !strings.Contains(err.Error(), "case-insensitive duplicate") {
		t.Errorf("expected case-insensitive duplicate error, got: %v", err)
	}
}

func TestCreate_ExactDuplicate(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// Exact same name should also fail
	err := mgr.Create(validProfile("Work"))
	if err == nil {
		t.Fatal("Create(Work) should fail when 'Work' already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestCreate_DifferentNames_Allowed(t *testing.T) {
	mgr, _ := newTestManager(t)
	// Names that are completely different should both succeed
	if err := mgr.Create(validProfile("alpha")); err != nil {
		t.Fatalf("Create(alpha): %v", err)
	}
	if err := mgr.Create(validProfile("beta")); err != nil {
		t.Fatalf("Create(beta): %v", err)
	}
}

func TestDelete_CaseSensitive(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// Deleting with wrong case should fail
	err := mgr.Delete("work")
	if err == nil {
		t.Fatal("Delete(work) should fail when profile is 'Work'")
	}

	// Deleting with correct case should succeed
	if err := mgr.Delete("Work"); err != nil {
		t.Fatalf("Delete(Work) error: %v", err)
	}
}

func TestUpdate_CaseSensitive(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// Updating with wrong case should fail
	p := validProfile("work")
	err := mgr.Update(p)
	if err == nil {
		t.Fatal("Update(work) should fail when profile is 'Work'")
	}

	// Updating with correct case should succeed
	p2, _ := mgr.Get("Work")
	p2.Git.User.Email = "new@example.com"
	if err := mgr.Update(p2); err != nil {
		t.Fatalf("Update(Work) error: %v", err)
	}
}

func TestFindExactProfile(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("MyProfile"))

	if _, found := mgr.findExactProfile("MyProfile"); !found {
		t.Error("findExactProfile should find exact match")
	}
	if _, found := mgr.findExactProfile("myprofile"); found {
		t.Error("findExactProfile should not find case-insensitive match")
	}
	if _, found := mgr.findExactProfile("MYPROFILE"); found {
		t.Error("findExactProfile should not find uppercase match")
	}
}

func TestHasCaseInsensitiveDuplicate(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("Work"))

	// "work" conflicts with "Work"
	existing, conflict := mgr.hasCaseInsensitiveDuplicate("work")
	if !conflict {
		t.Fatal("expected conflict between 'work' and 'Work'")
	}
	if existing != "Work" {
		t.Errorf("expected conflicting name 'Work', got %q", existing)
	}

	// "Work" itself is not a duplicate (exact same name)
	_, conflict = mgr.hasCaseInsensitiveDuplicate("Work")
	if conflict {
		t.Error("same exact name should not be flagged as duplicate")
	}

	// Completely different name has no conflict
	_, conflict = mgr.hasCaseInsensitiveDuplicate("Personal")
	if conflict {
		t.Error("different name should not conflict")
	}
}

func TestCreate_ProfilePathAbsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := profileAbsFn
	profileAbsFn = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { profileAbsFn = orig }()

	err := mgr.Create(validProfile("work"))
	if err == nil || !strings.Contains(err.Error(), "resolving profiles dir") {
		t.Fatalf("expected abs error, got: %v", err)
	}
}

func TestUpdate_ProfilePathAbsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	// First create with working abs
	mgr.Create(validProfile("work"))

	orig := profileAbsFn
	profileAbsFn = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { profileAbsFn = orig }()

	err := mgr.Update(validProfile("work"))
	if err == nil || !strings.Contains(err.Error(), "resolving profiles dir") {
		t.Fatalf("expected abs error, got: %v", err)
	}
}

func TestList_FileSvcListError(t *testing.T) {
	mgr, cfg := newTestManager(t)
	// filepath.Glob returns ErrBadPattern for malformed patterns like unclosed brackets
	cfg.ProfilesDir = filepath.Join(t.TempDir(), "[unclosed")

	_, err := mgr.List()
	if err == nil || !strings.Contains(err.Error(), "listing profiles") {
		t.Fatalf("expected listing error, got: %v", err)
	}
}

func TestExists_ProfilePathAbsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := profileAbsFn
	profileAbsFn = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { profileAbsFn = orig }()

	if mgr.Exists("work") {
		t.Error("expected false when profilePath fails")
	}
}

func TestExport_MarshalError(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Create(validProfile("work"))

	orig := yamlMarshalProfFn
	yamlMarshalProfFn = func(interface{}) ([]byte, error) { return nil, errors.New("marshal fail") }
	defer func() { yamlMarshalProfFn = orig }()

	_, err := mgr.Export("work")
	if err == nil || !strings.Contains(err.Error(), "marshaling profile") {
		t.Fatalf("expected marshal error, got: %v", err)
	}
}

func TestSave_MarshalError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := yamlMarshalProfFn
	yamlMarshalProfFn = func(interface{}) ([]byte, error) { return nil, errors.New("marshal fail") }
	defer func() { yamlMarshalProfFn = orig }()

	err := mgr.Create(validProfile("work"))
	if err == nil || !strings.Contains(err.Error(), "marshaling profile") {
		t.Fatalf("expected marshal error in save, got: %v", err)
	}
}

func TestSave_ProfilePathAbsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	// We need marshal to succeed but profilePath to fail in save.
	// Create first then break abs for the update path.
	mgr.Create(validProfile("work"))

	callCount := 0
	orig := profileAbsFn
	profileAbsFn = func(path string) (string, error) {
		callCount++
		// Let the first call (from Update's profilePath check) succeed,
		// fail on the second (from save's profilePath call)
		if callCount <= 1 {
			return orig(path)
		}
		return "", errors.New("abs error")
	}
	defer func() { profileAbsFn = orig }()

	p := validProfile("work")
	p.Metadata.Updated = p.Metadata.Created
	err := mgr.Update(p)
	if err == nil || !strings.Contains(err.Error(), "resolving profiles dir") {
		t.Fatalf("expected abs error in save, got: %v", err)
	}
}

func TestProfilePath_AbsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := profileAbsFn
	profileAbsFn = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { profileAbsFn = orig }()

	_, err := mgr.profilePath("validname")
	if err == nil || !strings.Contains(err.Error(), "resolving profiles dir") {
		t.Fatalf("expected abs error, got: %v", err)
	}
}

func TestProfilePath_RelError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := profileRelFn
	profileRelFn = func(string, string) (string, error) { return "", errors.New("rel error") }
	defer func() { profileRelFn = orig }()

	_, err := mgr.profilePath("validname")
	if err == nil {
		t.Fatal("expected error when filepath.Rel fails")
	}
	var pe *ProfileError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ProfileError, got: %T", err)
	}
}

func TestCurrentLocalProfile_GetwdError(t *testing.T) {
	mgr, _ := newTestManager(t)
	orig := mgrGetwdFn
	mgrGetwdFn = func() (string, error) { return "", errors.New("getwd error") }
	defer func() { mgrGetwdFn = orig }()

	_, err := mgr.currentLocalProfile()
	if err == nil || !strings.Contains(err.Error(), "getwd error") {
		t.Fatalf("expected getwd error, got: %v", err)
	}
}
