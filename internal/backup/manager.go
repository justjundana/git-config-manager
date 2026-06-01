// Package backup provides backup and restore capabilities for GCM.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// Test hooks for unreachable OS/IO error paths.
var (
	osOpenFileFn       = os.OpenFile
	backupAbsFn        = filepath.Abs
	backupRelFn        = filepath.Rel
	backupMkdirFn      = os.MkdirAll
	backupRemoveFn     = os.Remove
	backupReadDirFn    = os.ReadDir
	tarCloseFn         = func(tw *tar.Writer) error { return tw.Close() }
	gzipCloseFn        = func(gzw *gzip.Writer) error { return gzw.Close() }
	fileStatFn         = func(f *os.File) (os.FileInfo, error) { return f.Stat() }
	tarWriteHeaderFn   = func(tw *tar.Writer, hdr *tar.Header) error { return tw.WriteHeader(hdr) }
	restoreMkdirTempFn = os.MkdirTemp
	restoreMkdirFn     = os.MkdirAll
	restoreRemoveFn    = os.Remove
	restoreRenameFn    = os.Rename
	restoreChmodFn     = os.Chmod
	restoreLstatFn     = os.Lstat
	restoreStatFn      = os.Stat
	restoreRelFn       = filepath.Rel
	restoreEntryInfoFn = func(entry fs.DirEntry) (fs.FileInfo, error) { return entry.Info() }
)

// Manager handles backup and restore operations.
type Manager struct {
	cfg *config.Config
	log *logger.Logger
}

// NewManager creates a new backup manager.
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{cfg: cfg, log: log}
}

// BackupInfo holds metadata about a backup.
type BackupInfo struct {
	Path      string
	Size      int64
	Created   time.Time
	Profiles  int
	Templates int
}

type restoreChange struct {
	rel         string
	hadOriginal bool
}

// Create creates a backup of all GCM data.
func (m *Manager) Create() (*BackupInfo, error) {
	if err := m.validateCreateOptions(); err != nil {
		return nil, err
	}

	backupDir := filepath.Join(config.GCMDir(), "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02-150405.000000000")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("gcm-backup-%s.tar.gz", timestamp))

	f, err := osOpenFileFn(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("creating backup file: %w", err)
	}
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(backupPath)
		}
	}()

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	gcmDir := config.GCMDir()
	profileCount := 0
	templateCount := 0

	// Backup config file
	configPath := filepath.Join(gcmDir, "config.yaml")
	if err := m.addToArchive(tw, configPath, "config.yaml"); err != nil {
		m.log.Debug("No config to backup", logger.F("error", err))
	}

	// Backup profiles
	profilesDir := m.cfg.ProfilesDir
	if entries, err := os.ReadDir(profilesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
				src := filepath.Join(profilesDir, entry.Name())
				dst := filepath.Join("profiles", entry.Name())
				if err := m.addToArchive(tw, src, dst); err != nil {
					m.log.Warn("Failed to backup profile", logger.F("file", entry.Name()))
					continue
				}
				profileCount++
			}
		}
	}

	// Backup templates
	templatesDir := m.cfg.TemplatesDir
	if entries, err := os.ReadDir(templatesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
				src := filepath.Join(templatesDir, entry.Name())
				dst := filepath.Join("templates", entry.Name())
				if err := m.addToArchive(tw, src, dst); err != nil {
					m.log.Warn("Failed to backup template", logger.F("file", entry.Name()))
					continue
				}
				templateCount++
			}
		}
	}

	// Close archive writers before measuring file size so all data is flushed.
	if err := tarCloseFn(tw); err != nil {
		return nil, fmt.Errorf("finalizing tar: %w", err)
	}
	if err := gzipCloseFn(gzw); err != nil {
		return nil, fmt.Errorf("finalizing gzip: %w", err)
	}

	// Get file size (now accurate after writers are closed)
	fInfo, err := fileStatFn(f)
	if err != nil {
		return nil, fmt.Errorf("getting backup file size: %w", err)
	}

	m.log.Debug("Backup created",
		logger.F("path", backupPath),
		logger.F("profiles", profileCount),
		logger.F("templates", templateCount))

	success = true
	info := &BackupInfo{
		Path:      backupPath,
		Size:      fInfo.Size(),
		Created:   time.Now(),
		Profiles:  profileCount,
		Templates: templateCount,
	}
	m.enforceRetention()
	return info, nil
}

func (m *Manager) validateCreateOptions() error {
	if m.cfg.Backup.Encryption {
		return fmt.Errorf("encrypted backups are not implemented; set backup.encryption=false or do not create backups until encryption support is available")
	}
	if m.cfg.Backup.IncludeKeys {
		return fmt.Errorf("backup.include_keys requires encrypted backup support and is disabled")
	}
	return nil
}

func (m *Manager) enforceRetention() {
	if m.cfg.Backup.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -m.cfg.Backup.RetentionDays)
		if removed, err := m.PruneOlderThan(cutoff); err != nil {
			m.log.Warn("Failed to prune backups by age", logger.F("error", err))
		} else if removed > 0 {
			m.log.Debug("Pruned backups by age", logger.F("removed", removed))
		}
	}
	if m.cfg.Backup.MaxBackups > 0 {
		if removed, err := m.Prune(m.cfg.Backup.MaxBackups); err != nil {
			m.log.Warn("Failed to prune backups by count", logger.F("error", err))
		} else if removed > 0 {
			m.log.Debug("Pruned backups by count", logger.F("removed", removed))
		}
	}
}

// Restore restores from a backup file. It refuses to extract entries whose
// sanitized path would escape the GCM data directory (zip-slip protection).
//
// Individual files are capped at maxExtractSize to guard against
// decompression bombs; GCM config files should never approach this limit.
const maxExtractSize = 10 << 20 // 10 MiB per file

func (m *Manager) Restore(backupPath string) error {
	gcmDir := config.GCMDir()
	gcmDirAbs, err := backupAbsFn(gcmDir)
	if err != nil {
		return fmt.Errorf("resolving GCM dir: %w", err)
	}
	if err := backupMkdirFn(gcmDirAbs, 0o700); err != nil {
		return fmt.Errorf("creating GCM directory: %w", err)
	}

	stagingDir, err := restoreMkdirTempFn(gcmDirAbs, ".restore-*")
	if err != nil {
		return fmt.Errorf("creating restore staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	if err := m.extractArchive(backupPath, stagingDir); err != nil {
		return err
	}
	if err := m.applyStagedRestore(stagingDir, gcmDirAbs); err != nil {
		return err
	}

	m.log.Debug("Backup restored", logger.F("path", backupPath))
	return nil
}

func (m *Manager) extractArchive(backupPath, stagingDir string) error {
	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("opening backup: %w", err)
	}
	defer f.Close()

	// Limit decompressed data to prevent decompression bombs and mitigate
	// GO-2026-4869 (unbounded allocation from old GNU sparse headers).
	// GCM backups are small config files; 50 MiB is generous.
	const maxArchiveSize = 50 << 20
	gzr, err := gzip.NewReader(io.LimitReader(f, maxArchiveSize))
	if err != nil {
		return fmt.Errorf("decompressing backup: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	stagingAbs, err := backupAbsFn(stagingDir)
	if err != nil {
		return fmt.Errorf("resolving restore staging dir: %w", err)
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading backup entry: %w", err)
		}

		// Only support regular files and directories. Skip symlinks and other
		// special entries; they could otherwise point outside the data dir.
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			m.log.Warn("Skipping unsupported archive entry",
				logger.F("name", header.Name),
				logger.F("type", header.Typeflag))
			continue
		}

		// Reject absolute paths and any entry whose cleaned path escapes the
		// staging directory (zip-slip / tar-slip).
		cleanName := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "..") || strings.Contains(header.Name, `\`) {
			return fmt.Errorf("refusing to extract unsafe path %q from backup", header.Name)
		}

		target := filepath.Join(stagingAbs, cleanName)
		rel, err := backupRelFn(stagingAbs, target)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("refusing to extract unsafe path %q from backup", header.Name)
		}

		if header.Typeflag == tar.TypeDir {
			if err := restoreMkdirFn(target, 0o700); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}
			continue
		}

		if err := restoreMkdirFn(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		// Clamp extracted file permissions to the owner. Backups never need
		// group/other bits set.
		if header.Size > maxExtractSize {
			return fmt.Errorf("file %q in backup exceeds maximum extract size (%d bytes)", header.Name, maxExtractSize)
		}

		mode := os.FileMode(header.Mode).Perm() & 0o700
		if mode == 0 {
			mode = 0o600
		}

		outFile, err := osOpenFileFn(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("creating file %s: %w", target, err)
		}
		if err := func() error {
			defer outFile.Close()
			if _, err := io.Copy(outFile, io.LimitReader(tr, maxExtractSize)); err != nil {
				return fmt.Errorf("writing file %s: %w", target, err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) applyStagedRestore(stagingDir, gcmDirAbs string) error {
	rollbackDir, err := restoreMkdirTempFn(gcmDirAbs, ".restore-rollback-*")
	if err != nil {
		return fmt.Errorf("creating restore rollback directory: %w", err)
	}
	rollbackComplete := false
	defer func() {
		if rollbackComplete {
			os.RemoveAll(rollbackDir)
		}
	}()

	var changes []restoreChange
	var createdDirs []string
	rollback := func() {
		for i := len(changes) - 1; i >= 0; i-- {
			change := changes[i]
			target := filepath.Join(gcmDirAbs, change.rel)
			_ = os.RemoveAll(target)
			if change.hadOriginal {
				backupPath := filepath.Join(rollbackDir, change.rel)
				_ = os.MkdirAll(filepath.Dir(target), 0o700)
				_ = os.Rename(backupPath, target)
			}
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			_ = restoreRemoveFn(createdDirs[i])
		}
	}

	err = filepath.WalkDir(stagingDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == stagingDir {
			return nil
		}

		rel, err := restoreRelFn(stagingDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(gcmDirAbs, rel)

		if entry.IsDir() {
			if info, err := restoreStatFn(target); err == nil {
				if !info.IsDir() {
					return fmt.Errorf("restore target %s is not a directory", target)
				}
				return nil
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("checking restore directory %s: %w", target, err)
			}
			if err := restoreMkdirFn(target, 0o700); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}
			createdDirs = append(createdDirs, target)
			return nil
		}

		info, err := restoreEntryInfoFn(entry)
		if err != nil {
			return fmt.Errorf("reading staged file info: %w", err)
		}
		if err := restoreMkdirFn(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		change := restoreChange{rel: rel}
		if _, err := restoreLstatFn(target); err == nil {
			backupPath := filepath.Join(rollbackDir, rel)
			if err := restoreMkdirFn(filepath.Dir(backupPath), 0o700); err != nil {
				return fmt.Errorf("creating rollback directory: %w", err)
			}
			if err := restoreRenameFn(target, backupPath); err != nil {
				return fmt.Errorf("staging existing file for rollback: %w", err)
			}
			change.hadOriginal = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking restore target %s: %w", target, err)
		}

		if err := restoreRenameFn(path, target); err != nil {
			if change.hadOriginal {
				_ = restoreRenameFn(filepath.Join(rollbackDir, rel), target)
			}
			return fmt.Errorf("applying restored file %s: %w", target, err)
		}
		if err := restoreChmodFn(target, info.Mode().Perm()&0o700); err != nil {
			changes = append(changes, change)
			return fmt.Errorf("setting restored file permissions: %w", err)
		}
		changes = append(changes, change)
		return nil
	})
	if err != nil {
		rollback()
		rollbackComplete = true
		return err
	}

	rollbackComplete = true
	return nil
}

// List returns all available backups sorted by date (newest first).
func (m *Manager) List() ([]BackupInfo, error) {
	backupDir := filepath.Join(config.GCMDir(), "backups")
	entries, err := backupReadDirFn(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backups directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			m.log.Warn("Failed to stat backup file", logger.F("name", entry.Name()), logger.F("error", err))
			continue
		}
		backups = append(backups, BackupInfo{
			Path:    filepath.Join(backupDir, entry.Name()),
			Size:    info.Size(),
			Created: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Created.After(backups[j].Created)
	})

	return backups, nil
}

// Prune removes old backups keeping only the latest N.
func (m *Manager) Prune(keep int) (int, error) {
	if keep < 1 {
		return 0, fmt.Errorf("keep must be at least 1, got %d", keep)
	}

	backups, err := m.List()
	if err != nil {
		return 0, err
	}

	if len(backups) <= keep {
		return 0, nil
	}

	removed := 0
	for _, b := range backups[keep:] {
		if err := backupRemoveFn(b.Path); err != nil {
			m.log.Warn("Failed to remove backup", logger.F("path", b.Path))
			continue
		}
		removed++
	}

	m.log.Debug("Backups pruned", logger.F("removed", removed), logger.F("kept", keep))
	return removed, nil
}

// PruneOlderThan removes backups older than the provided cutoff time.
func (m *Manager) PruneOlderThan(cutoff time.Time) (int, error) {
	backups, err := m.List()
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, backup := range backups {
		if !backup.Created.Before(cutoff) {
			continue
		}
		if err := backupRemoveFn(backup.Path); err != nil {
			m.log.Warn("Failed to remove expired backup", logger.F("path", backup.Path), logger.F("error", err))
			continue
		}
		removed++
	}
	return removed, nil
}

func (m *Manager) addToArchive(tw *tar.Writer, srcPath, archivePath string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     archivePath,
		Size:     info.Size(),
		Mode:     int64(info.Mode()),
		ModTime:  info.ModTime(),
	}

	if err := tarWriteHeaderFn(tw, header); err != nil {
		return err
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
	return err
}
