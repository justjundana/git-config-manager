// Package template provides configuration template management for GCM.
package template

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	fileSvc "github.com/justjundana/git-config-manager/internal/service/file"
	"github.com/justjundana/git-config-manager/pkg/logger"

	"gopkg.in/yaml.v3"
)

// Test hooks for deterministic error-path tests.
var (
	yamlMarshalFn = yaml.Marshal
	filepathAbsFn = filepath.Abs
	filepathRelFn = filepath.Rel
)

// Template represents a reusable configuration template.
type Template struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Git         GitConfigTemplate `yaml:"git" json:"git"`
	Required    []string          `yaml:"required,omitempty" json:"required,omitempty"`
	Metadata    TemplateMetadata  `yaml:"metadata" json:"metadata"`
}

// GitConfigTemplate holds template git settings.
type GitConfigTemplate struct {
	Core    map[string]interface{} `yaml:"core,omitempty" json:"core,omitempty"`
	Commit  map[string]interface{} `yaml:"commit,omitempty" json:"commit,omitempty"`
	Pull    map[string]interface{} `yaml:"pull,omitempty" json:"pull,omitempty"`
	Push    map[string]interface{} `yaml:"push,omitempty" json:"push,omitempty"`
	Aliases map[string]string      `yaml:"aliases,omitempty" json:"aliases,omitempty"`
}

// TemplateMetadata holds template lifecycle info.
type TemplateMetadata struct {
	Author  string    `yaml:"author,omitempty" json:"author,omitempty"`
	Version string    `yaml:"version" json:"version"`
	Created time.Time `yaml:"created" json:"created"`
	Updated time.Time `yaml:"updated" json:"updated"`
}

// Manager handles template operations.
type Manager struct {
	cfg     *config.Config
	fileSvc *fileSvc.Service
	log     *logger.Logger
}

// NewManager creates a new template manager.
func NewManager(cfg *config.Config, fs *fileSvc.Service, log *logger.Logger) *Manager {
	return &Manager{cfg: cfg, fileSvc: fs, log: log}
}

// Create creates a new template.
func (m *Manager) Create(t *Template) error {
	if t.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	path, err := m.templatePath(t.Name)
	if err != nil {
		return err
	}
	if m.fileSvc.Exists(path) {
		return fmt.Errorf("template %q already exists", t.Name)
	}

	now := time.Now().UTC()
	if t.Metadata.Version == "" {
		t.Metadata.Version = "1.0"
	}
	if t.Metadata.Created.IsZero() {
		t.Metadata.Created = now
	}
	if t.Metadata.Updated.IsZero() {
		t.Metadata.Updated = now
	}

	return m.save(t)
}

// Update updates an existing template.
func (m *Manager) Update(t *Template) error {
	if t.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	path, err := m.templatePath(t.Name)
	if err != nil {
		return err
	}
	if !m.fileSvc.Exists(path) {
		return fmt.Errorf("template %q not found", t.Name)
	}

	t.Metadata.Updated = time.Now().UTC()
	return m.save(t)
}

// Get retrieves a template by name.
func (m *Manager) Get(name string) (*Template, error) {
	path, err := m.templatePath(name)
	if err != nil {
		return nil, err
	}
	data, err := m.fileSvc.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("template %q not found", name)
		}
		return nil, fmt.Errorf("reading template: %w", err)
	}

	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}
	t.Name = name
	return &t, nil
}

// List returns all templates sorted by name.
func (m *Manager) List() ([]*Template, error) {
	files, err := m.fileSvc.List(m.cfg.TemplatesDir, "*.yaml")
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	var templates []*Template
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		t, err := m.Get(name)
		if err != nil {
			m.log.Warn("Skipping invalid template", logger.F("file", f))
			continue
		}
		templates = append(templates, t)
	}

	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})

	return templates, nil
}

// Delete removes a template.
func (m *Manager) Delete(name string) error {
	path, err := m.templatePath(name)
	if err != nil {
		return err
	}
	if !m.fileSvc.Exists(path) {
		return fmt.Errorf("template %q not found", name)
	}
	return m.fileSvc.Delete(path)
}

// Export serializes a template to YAML.
func (m *Manager) Export(name string) ([]byte, error) {
	t, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(t)
}

// Import creates a template from YAML data.
func (m *Manager) Import(data []byte) (*Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing template data: %w", err)
	}
	if err := m.Create(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (m *Manager) save(t *Template) error {
	data, err := yamlMarshalFn(t)
	if err != nil {
		return fmt.Errorf("marshaling template: %w", err)
	}
	path, err := m.templatePath(t.Name)
	if err != nil {
		return err
	}
	return m.fileSvc.WriteAtomic(path, data, 0600)
}

// templatePath returns the on-disk path for a template, refusing names that
// would traverse outside the templates directory (same defence as profile's
// profilePath).
func (m *Manager) templatePath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("template name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." ||
		strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid template name %q", name)
	}

	baseAbs, err := filepathAbsFn(m.cfg.TemplatesDir)
	if err != nil {
		return "", fmt.Errorf("resolving templates dir: %w", err)
	}
	full := filepath.Join(baseAbs, name+".yaml")
	cleaned := filepath.Clean(full)
	rel, err := filepathRelFn(baseAbs, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	return cleaned, nil
}
