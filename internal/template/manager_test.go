package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	fileSvc "github.com/justjundana/git-config-manager/internal/service/file"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		TemplatesDir: filepath.Join(dir, "templates"),
	}
	if err := os.MkdirAll(cfg.TemplatesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	return NewManager(cfg, fs, log)
}

func TestCreate(t *testing.T) {
	m := newTestManager(t)
	tmpl := &Template{Name: "standard", Description: "Standard dev config"}

	if err := m.Create(tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := m.Get("standard")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "standard" {
		t.Errorf("Name = %q, want %q", got.Name, "standard")
	}
	if got.Metadata.Version != "1.0" {
		t.Errorf("Version = %q, want %q", got.Metadata.Version, "1.0")
	}
	if got.Metadata.Created.IsZero() {
		t.Error("Created should not be zero")
	}
}

func TestCreateDuplicate(t *testing.T) {
	m := newTestManager(t)
	_ = m.Create(&Template{Name: "dup"})
	if err := m.Create(&Template{Name: "dup"}); err == nil {
		t.Error("expected error for duplicate template")
	}
}

func TestCreateEmptyName(t *testing.T) {
	m := newTestManager(t)
	if err := m.Create(&Template{Name: ""}); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestGetNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Get("nonexistent"); err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestList(t *testing.T) {
	m := newTestManager(t)
	_ = m.Create(&Template{Name: "beta"})
	_ = m.Create(&Template{Name: "alpha"})

	templates, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("List = %d templates, want 2", len(templates))
	}
	// Should be sorted
	if templates[0].Name != "alpha" {
		t.Errorf("first = %q, want alpha", templates[0].Name)
	}
}

func TestDelete(t *testing.T) {
	m := newTestManager(t)
	_ = m.Create(&Template{Name: "gone"})

	if err := m.Delete("gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get("gone"); err == nil {
		t.Error("template should be deleted")
	}
}

func TestDeleteNotFound(t *testing.T) {
	m := newTestManager(t)
	if err := m.Delete("nonexistent"); err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestExportImport(t *testing.T) {
	m := newTestManager(t)
	_ = m.Create(&Template{Name: "exportme", Description: "exported"})

	data, err := m.Export("exportme")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	_ = m.Delete("exportme")

	imported, err := m.Import(data)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if imported.Description != "exported" {
		t.Errorf("Description = %q, want %q", imported.Description, "exported")
	}
}

func TestTemplatePath_RejectsTraversal(t *testing.T) {
	m := newTestManager(t)

	cases := []string{
		"../etc/passwd",
		"..",
		".",
		"foo/bar",
		"foo\\bar",
		"..\\evil",
		"good/../bad",
		"",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := m.templatePath(name); err == nil {
				t.Fatalf("templatePath(%q) should have failed", name)
			}
		})
	}
}

func TestTemplatePath_ValidNames(t *testing.T) {
	m := newTestManager(t)

	cases := []string{"standard", "my-template", "work_2024"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			path, err := m.templatePath(name)
			if err != nil {
				t.Fatalf("templatePath(%q): %v", name, err)
			}
			if filepath.Ext(path) != ".yaml" {
				t.Errorf("expected .yaml extension, got %q", path)
			}
		})
	}
}

func TestSavePermissions(t *testing.T) {
	m := newTestManager(t)
	_ = m.Create(&Template{Name: "perms"})

	path, _ := m.templatePath("perms")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("template perm = %o, want 0600", perm)
	}
}

func TestImport_InvalidYAML(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Import([]byte(":::invalid yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestImport_EmptyName(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Import([]byte("description: no name\n"))
	if err == nil {
		t.Error("expected error for import with empty name")
	}
}

func TestExport_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Export("nonexistent")
	if err == nil {
		t.Error("expected error for exporting nonexistent template")
	}
}

func TestCreateWithGitConfig(t *testing.T) {
	m := newTestManager(t)
	tmpl := &Template{
		Name:        "full",
		Description: "Full config template",
		Git: GitConfigTemplate{
			Core:    map[string]interface{}{"editor": "vim"},
			Commit:  map[string]interface{}{"gpgsign": true},
			Pull:    map[string]interface{}{"rebase": true},
			Push:    map[string]interface{}{"default": "current"},
			Aliases: map[string]string{"st": "status", "co": "checkout"},
		},
		Required: []string{"user.name", "user.email"},
	}

	if err := m.Create(tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := m.Get("full")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Git.Aliases["st"] != "status" {
		t.Errorf("alias st = %q, want status", got.Git.Aliases["st"])
	}
	if len(got.Required) != 2 {
		t.Errorf("required = %d, want 2", len(got.Required))
	}
}

func TestList_Empty(t *testing.T) {
	m := newTestManager(t)
	templates, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(templates))
	}
}

func TestImport_TraversalName(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Import([]byte("name: \"../evil\"\ndescription: bad\n"))
	if err == nil {
		t.Error("expected error for import with traversal name")
	}
}

func TestGet_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestGet_Traversal(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Get("../evil")
	if err == nil {
		t.Fatal("expected error for traversal name")
	}
}

func TestDelete_Traversal(t *testing.T) {
	m := newTestManager(t)
	err := m.Delete("../evil")
	if err == nil {
		t.Fatal("expected error for traversal delete")
	}
}

func TestDelete_Success(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{
		Name:        "tobedeleted",
		Description: "test",
	})

	if err := m.Delete("tobedeleted"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := m.Get("tobedeleted")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestList_WithTemplates(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{Name: "t1", Description: "first"})
	m.Create(&Template{Name: "t2", Description: "second"})

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 templates, got %d", len(list))
	}
}

func TestSave_ErrorPath(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denied as root")
	}
	dir := t.TempDir()
	cfg := &config.Config{
		TemplatesDir: filepath.Join(dir, "templates"),
	}
	os.MkdirAll(cfg.TemplatesDir, 0o755)
	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, fs, log)

	// Create a template first
	m.Create(&Template{Name: "test"})

	// Make the templates dir read-only
	os.Chmod(cfg.TemplatesDir, 0o444)
	t.Cleanup(func() { os.Chmod(cfg.TemplatesDir, 0o755) })

	// Trying to create a new template should fail
	err := m.Create(&Template{Name: "newone"})
	if err == nil {
		t.Fatal("expected error saving to read-only dir")
	}
}

func TestList_SkipsInvalidTemplates(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{Name: "valid", Description: "ok"})

	// Write invalid YAML
	path, _ := m.templatePath("invalid")
	os.WriteFile(path, []byte(":::bad yaml"), 0o600)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should skip the invalid one
	if len(list) != 1 {
		t.Errorf("expected 1 valid template, got %d", len(list))
	}
}

func TestDelete_Traversal_Error(t *testing.T) {
	m := newTestManager(t)
	err := m.Delete("../evil")
	if err == nil {
		t.Fatal("expected error for traversal delete")
	}
}

func TestGet_EmptyName(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Get("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestExport_MarshalSuccess(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{
		Name:        "exportable",
		Description: "test export",
		Git: GitConfigTemplate{
			Aliases: map[string]string{"co": "checkout"},
		},
	})

	data, err := m.Export("exportable")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty export data")
	}
}

func TestExport_Success(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{Name: "exportme", Description: "export test"})

	data, err := m.Export("exportme")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty export data")
	}
}

func TestTemplatePath_EmptyName(t *testing.T) {
	m := newTestManager(t)
	_, err := m.templatePath("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestTemplatePath_Traversal(t *testing.T) {
	m := newTestManager(t)
	cases := []string{"../evil", "a/b", `a\b`, "..", "."}
	for _, name := range cases {
		_, err := m.templatePath(name)
		if err == nil {
			t.Errorf("templatePath(%q) should fail", name)
		}
	}
}

func TestList_CorruptYAML(t *testing.T) {
	m := newTestManager(t)
	// Create a valid template and a corrupt one
	m.Create(&Template{Name: "good", Description: "ok"})
	os.WriteFile(filepath.Join(m.cfg.TemplatesDir, "bad.yaml"), []byte(":::invalid"), 0o600)

	templates, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should return at least the good one (corrupt may be skipped or error)
	if len(templates) < 1 {
		t.Errorf("expected at least 1 template, got %d", len(templates))
	}
}

func TestDelete_NonExistentTemplate(t *testing.T) {
	m := newTestManager(t)
	err := m.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestSave_UnwritableDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		TemplatesDir: filepath.Join(dir, "templates"),
	}
	os.MkdirAll(cfg.TemplatesDir, 0o755)
	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, fs, log)

	// Create template first
	m.Create(&Template{Name: "testwrite", Description: "ok"})

	// Make dir unwritable
	os.Chmod(cfg.TemplatesDir, 0o000)
	defer os.Chmod(cfg.TemplatesDir, 0o755)

	err := m.Create(&Template{Name: "fail", Description: "should fail"})
	if err == nil {
		t.Fatal("expected error when templates dir is unwritable")
	}
}

func TestDelete_AlreadyRemoved(t *testing.T) {
	m := newTestManager(t)

	// Create a template so it passes the Exists check
	tmpl := &Template{Name: "ephemeral", Description: "will vanish"}
	if err := m.Create(tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually remove the file so Delete's os.Remove fails
	path := filepath.Join(m.cfg.TemplatesDir, "ephemeral.yaml")
	os.Remove(path)

	err := m.Delete("ephemeral")
	if err == nil {
		t.Fatal("expected error when file already deleted")
	}
}

func TestUpdate_Success(t *testing.T) {
	m := newTestManager(t)
	m.Create(&Template{Name: "updatable", Description: "original"})

	tmpl, _ := m.Get("updatable")
	tmpl.Description = "updated"
	tmpl.Git.Core = map[string]interface{}{"editor": "vim"}

	if err := m.Update(tmpl); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := m.Get("updatable")
	if got.Description != "updated" {
		t.Errorf("Description = %q, want %q", got.Description, "updated")
	}
	if got.Git.Core["editor"] != "vim" {
		t.Errorf("Core editor = %v, want vim", got.Git.Core["editor"])
	}
	if got.Metadata.Updated.IsZero() {
		t.Error("Updated timestamp should be set")
	}
}

func TestUpdate_EmptyName(t *testing.T) {
	m := newTestManager(t)
	err := m.Update(&Template{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	m := newTestManager(t)
	err := m.Update(&Template{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestUpdate_TraversalName(t *testing.T) {
	m := newTestManager(t)
	err := m.Update(&Template{Name: "../evil"})
	if err == nil {
		t.Fatal("expected error for traversal name")
	}
}

func TestGet_ReadErrorNotExist(t *testing.T) {
	// Non-NotExist read error: make the template file unreadable
	m := newTestManager(t)
	path := filepath.Join(m.cfg.TemplatesDir, "broken.yaml")
	os.WriteFile(path, []byte("data"), 0o644)
	os.Chmod(path, 0o000)
	t.Cleanup(func() { os.Chmod(path, 0o644) })

	_, err := m.Get("broken")
	if err == nil {
		t.Fatal("expected error reading unreadable template")
	}
	if !strings.Contains(err.Error(), "reading template") {
		t.Errorf("error = %q, expected 'reading template'", err.Error())
	}
}

func TestList_InaccessibleDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		TemplatesDir: filepath.Join(dir, "templates"),
	}
	os.MkdirAll(cfg.TemplatesDir, 0o755)
	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, fs, log)

	// Place an invalid YAML file to trigger the "skipping invalid template" path
	os.WriteFile(filepath.Join(cfg.TemplatesDir, "bad.yaml"), []byte("{{invalid"), 0o644)

	templates, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Bad template should be skipped
	if len(templates) != 0 {
		t.Errorf("expected 0 valid templates, got %d", len(templates))
	}
}

func TestList_DirListError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		// Use a glob pattern that's invalid inside the TemplatesDir
		TemplatesDir: filepath.Join(dir, "templates["),
	}
	os.MkdirAll(cfg.TemplatesDir, 0o755)
	fs := fileSvc.NewService()
	log := logger.New(logger.LevelError, os.Stderr)
	m := NewManager(cfg, fs, log)

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error listing templates with bad glob pattern in path")
	}
}

func TestSave_TemplatePathError(t *testing.T) {
	m := newTestManager(t)
	err := m.save(&Template{Name: "../escape"})
	if err == nil {
		t.Fatal("expected error from save with invalid name")
	}
}

func TestTemplatePath_AbsError(t *testing.T) {
	m := newTestManager(t)

	old := filepathAbsFn
	filepathAbsFn = func(path string) (string, error) {
		return "", fmt.Errorf("abs failure")
	}
	defer func() { filepathAbsFn = old }()

	_, err := m.templatePath("valid")
	if err == nil {
		t.Fatal("expected error when Abs fails")
	}
	if !strings.Contains(err.Error(), "resolving templates dir") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestTemplatePath_RelError(t *testing.T) {
	m := newTestManager(t)

	old := filepathRelFn
	filepathRelFn = func(basepath, targpath string) (string, error) {
		return "", fmt.Errorf("rel failure")
	}
	defer func() { filepathRelFn = old }()

	_, err := m.templatePath("valid")
	if err == nil {
		t.Fatal("expected error when Rel fails")
	}
	if !strings.Contains(err.Error(), "invalid template name") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestSave_MarshalError(t *testing.T) {
	m := newTestManager(t)

	old := yamlMarshalFn
	yamlMarshalFn = func(v interface{}) ([]byte, error) {
		return nil, fmt.Errorf("marshal failure")
	}
	defer func() { yamlMarshalFn = old }()

	err := m.save(&Template{Name: "test"})
	if err == nil {
		t.Fatal("expected error when marshal fails")
	}
	if !strings.Contains(err.Error(), "marshaling template") {
		t.Errorf("error = %q", err.Error())
	}
}
