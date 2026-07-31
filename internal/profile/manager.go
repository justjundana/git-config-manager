package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_config "github.com/justjundana/git-config-manager/internal/config"
	_file "github.com/justjundana/git-config-manager/internal/service/file"
	_logger "github.com/justjundana/git-config-manager/pkg/logger"

	"gopkg.in/yaml.v3"
)

// Test hooks for unreachable OS/IO error paths.
var (
	profileAbsFn      = filepath.Abs
	profileRelFn      = filepath.Rel
	yamlMarshalProfFn = yaml.Marshal
	mgrGetwdFn        = os.Getwd
	mgrStatFn         = os.Stat
)

// Manager handles profile CRUD operations.
type Manager struct {
	cfg     *_config.Config
	fileSvc *_file.Service
	log     *_logger.Logger
}

// NewManager creates a new profile manager.
func NewManager(cfg *_config.Config, fs *_file.Service, log *_logger.Logger) *Manager {
	return &Manager{
		cfg:     cfg,
		fileSvc: fs,
		log:     log,
	}
}

// Create creates a new profile.
func (m *Manager) Create(p *Profile) error {
	if err := ValidateProfile(p); err != nil {
		return err
	}

	if _, err := m.profilePath(p.Name); err != nil {
		return err
	}

	// Check exact duplicate (case-sensitive).
	if _, found := m.findExactProfile(p.Name); found {
		return errAlreadyExists(p.Name)
	}

	// Reject case-insensitive duplicates for cross-platform consistency.
	if existing, conflict := m.hasCaseInsensitiveDuplicate(p.Name); conflict {
		return &ProfileError{
			Code:       ErrCodeAlreadyExists,
			Message:    fmt.Sprintf("profile %q conflicts with existing profile %q (case-insensitive duplicate)", p.Name, existing),
			Profile:    p.Name,
			Suggestion: fmt.Sprintf("Use the existing profile %q or choose a different name", existing),
		}
	}

	now := time.Now().UTC()
	p.Metadata = Metadata{
		Created: now,
		Updated: now,
		Version: "1.0",
	}

	if err := m.save(p); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}

	m.log.Debug("Profile created", _logger.F("profile", p.Name))
	return nil
}

// Get retrieves a profile by name (case-sensitive exact match).
// On case-insensitive filesystems, this ensures consistent behavior
// by scanning the directory for an exact filename match.
func (m *Manager) Get(name string) (*Profile, error) {
	if _, err := m.profilePath(name); err != nil {
		return nil, err
	}

	// Use directory scan for strict case-sensitive lookup.
	path, found := m.findExactProfile(name)
	if !found {
		return nil, errNotFound(name)
	}

	data, err := m.fileSvc.Read(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %s: %w", name, err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile %s: %w", name, err)
	}

	p.Name = name
	return &p, nil
}

// Update modifies an existing profile.
func (m *Manager) Update(p *Profile) error {
	if err := ValidateProfile(p); err != nil {
		return err
	}

	if _, err := m.profilePath(p.Name); err != nil {
		return err
	}
	if _, found := m.findExactProfile(p.Name); !found {
		return errNotFound(p.Name)
	}

	p.Metadata.Updated = time.Now().UTC()

	if err := m.save(p); err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}

	m.log.Debug("Profile updated", _logger.F("profile", p.Name))
	return nil
}

// Delete removes a profile.
func (m *Manager) Delete(name string, force ...bool) error {
	path, err := m.profilePath(name)
	if err != nil {
		return err
	}
	if _, found := m.findExactProfile(name); !found {
		return errNotFound(name)
	}

	skipSafety := len(force) > 0 && force[0]

	if !skipSafety {
		// Safety: cannot delete active profile
		current, _ := m.currentLocalProfile()
		if current == name {
			return errCannotDeleteActive(name)
		}

		// Safety: cannot delete default profile
		if m.cfg.DefaultProfile == name {
			return errCannotDeleteDefault(name)
		}
	}

	if err := m.fileSvc.Delete(path); err != nil {
		return fmt.Errorf("deleting profile: %w", err)
	}

	// Clear the default profile reference if we just deleted it, and persist
	// that. Clearing it only in memory left config.yaml naming a profile that
	// no longer exists, so the next process start resolved a default that
	// cannot be loaded.
	if m.cfg.DefaultProfile == name {
		m.cfg.DefaultProfile = ""
		if err := configSaveFn(m.cfg); err != nil {
			return fmt.Errorf("profile %q was deleted but clearing default_profile failed: %w", name, err)
		}
	}

	m.log.Debug("Profile deleted", _logger.F("profile", name))
	return nil
}

// List returns all profiles sorted by name. Profile files that cannot be read
// are skipped, so the result is a best-effort view suitable for display.
// Callers that need to know the view is complete must use ListWithUnreadable.
func (m *Manager) List() ([]*Profile, error) {
	profiles, _, err := m.ListWithUnreadable()
	return profiles, err
}

// ListWithUnreadable returns all readable profiles sorted by name, plus the
// paths of profile files that could not be parsed.
//
// The distinction matters for anything that reasons about what is *not* in the
// list. A profile whose YAML has a typo silently vanishes from List, and an
// inventory built from that list then reports its SSH key and GPG key as
// referenced by nobody — which is enough for "gcm ssh clean" to delete key
// material belonging to a profile that still exists.
func (m *Manager) ListWithUnreadable() ([]*Profile, []string, error) {
	files, err := m.fileSvc.List(m.cfg.ProfilesDir, "*.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("listing profiles: %w", err)
	}

	profiles := make([]*Profile, 0, len(files))
	var unreadable []string
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		p, err := m.Get(name)
		if err != nil {
			m.log.Warn("Skipping invalid profile",
				_logger.F("file", f), _logger.F("error", err))
			unreadable = append(unreadable, f)
			continue
		}
		profiles = append(profiles, p)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	sort.Strings(unreadable)

	return profiles, unreadable, nil
}

// EnsureStore verifies that the profile store directory exists and is readable.
//
// List is built on filepath.Glob, which reports no matches and no error when
// the directory is absent — so "the store is gone" and "the store is empty"
// are indistinguishable in its result. Read paths can live with that: showing
// "0 profiles" is harmless.
//
// Operations that treat the profile list as an authoritative inventory cannot.
// For them an absent store silently turns "no profile references this key"
// into "every key is orphaned", which is a licence to delete all of them.
// Such callers must call EnsureStore first and refuse to proceed on error.
func (m *Manager) EnsureStore() error {
	info, err := mgrStatFn(m.cfg.ProfilesDir)
	if err != nil {
		return errStoreMissing(m.cfg.ProfilesDir, err)
	}
	if !info.IsDir() {
		return errStoreMissing(m.cfg.ProfilesDir, fmt.Errorf("not a directory"))
	}
	return nil
}

// Exists checks if a profile exists by name (case-sensitive exact match).
func (m *Manager) Exists(name string) bool {
	if _, err := m.profilePath(name); err != nil {
		return false
	}
	_, found := m.findExactProfile(name)
	return found
}

// Export serializes a profile to YAML bytes.
func (m *Manager) Export(name string) ([]byte, error) {
	p, err := m.Get(name)
	if err != nil {
		return nil, err
	}

	data, err := yamlMarshalProfFn(p)
	if err != nil {
		return nil, fmt.Errorf("marshaling profile: %w", err)
	}
	return data, nil
}

// Import creates a profile from YAML bytes.
func (m *Manager) Import(data []byte) (*Profile, error) {
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile data: %w", err)
	}

	if err := m.Create(&p); err != nil {
		return nil, err
	}

	return &p, nil
}

// IncrementUsage updates the usage counter.
func (m *Manager) IncrementUsage(name string) error {
	p, err := m.Get(name)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	p.Metadata.UsageCount++
	p.Metadata.LastUsed = &now

	return m.save(p)
}

func (m *Manager) save(p *Profile) error {
	data, err := yamlMarshalProfFn(p)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	path, err := m.profilePath(p.Name)
	if err != nil {
		return err
	}
	return m.fileSvc.WriteAtomic(path, data, 0o600)
}

// profilePath returns the on-disk path for a profile, refusing names that
// would traverse outside the profiles directory.
//
// The regex validator already rejects slashes and dots at create time, but we
// defend in depth here because Import() accepts names embedded in a YAML
// payload and callers like Get/Delete may be invoked with arbitrary user
// input (file listings, CLI args, config values).
func (m *Manager) profilePath(name string) (string, error) {
	if name == "" {
		return "", ErrProfileNameEmpty()
	}
	// Reject anything containing path separators or ".." components.
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." ||
		strings.Contains(name, "..") {
		return "", &ProfileError{
			Code:       ErrCodeInvalid,
			Message:    fmt.Sprintf("invalid profile name %q", name),
			Profile:    name,
			Suggestion: "Profile names may not contain path separators or '..'",
		}
	}

	baseAbs, err := profileAbsFn(m.cfg.ProfilesDir)
	if err != nil {
		return "", fmt.Errorf("resolving profiles dir: %w", err)
	}
	full := filepath.Join(baseAbs, name+".yaml")
	cleaned := filepath.Clean(full)
	rel, err := profileRelFn(baseAbs, cleaned)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", &ProfileError{
			Code:       ErrCodeInvalid,
			Message:    fmt.Sprintf("invalid profile name %q", name),
			Profile:    name,
			Suggestion: "Profile names must stay inside the profiles directory",
		}
	}
	return cleaned, nil
}

// findExactProfile scans the profiles directory for a file whose base name
// (minus extension) matches 'name' exactly (case-sensitive). This ensures
// consistent behavior across all operating systems regardless of filesystem
// case-sensitivity. Returns true and the path if found.
func (m *Manager) findExactProfile(name string) (string, bool) {
	entries, err := os.ReadDir(m.cfg.ProfilesDir)
	if err != nil {
		return "", false
	}
	target := name + ".yaml"
	for _, e := range entries {
		if e.Name() == target {
			baseAbs, _ := filepath.Abs(m.cfg.ProfilesDir)
			return filepath.Join(baseAbs, e.Name()), true
		}
	}
	return "", false
}

// hasCaseInsensitiveDuplicate checks if a profile with the same name
// (case-insensitive) but different casing already exists. Returns the
// conflicting name if found.
func (m *Manager) hasCaseInsensitiveDuplicate(name string) (string, bool) {
	entries, err := os.ReadDir(m.cfg.ProfilesDir)
	if err != nil {
		return "", false
	}
	target := strings.ToLower(name + ".yaml")
	for _, e := range entries {
		if strings.ToLower(e.Name()) == target && e.Name() != name+".yaml" {
			return strings.TrimSuffix(e.Name(), ".yaml"), true
		}
	}
	return "", false
}

func (m *Manager) currentLocalProfile() (string, error) {
	cwd, err := mgrGetwdFn()
	if err != nil {
		return "", err
	}

	profileFile := filepath.Join(cwd, m.cfg.AutoSwitch.ProjectFile)
	data, err := os.ReadFile(profileFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}
