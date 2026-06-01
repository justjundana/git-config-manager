// Package file provides safe file system operations for GCM.
package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Test hooks for deterministic WriteAtomic error-path tests.
var (
	writeAtomicMkdirAllFn   = os.MkdirAll
	writeAtomicCreateTempFn = func(dir, pattern string) (atomicTempFile, error) { return os.CreateTemp(dir, pattern) }
	writeAtomicStatFn       = os.Stat
	writeAtomicRemoveFn     = os.Remove
	writeAtomicRenameFn     = os.Rename
)

// Test hooks for CopyFile error-path tests.
var (
	copyStatFn   = func(f *os.File) (os.FileInfo, error) { return f.Stat() }
	copyIOCopyFn = io.Copy
	runtimeGOOS  = runtime.GOOS
)

type atomicTempFile interface {
	Name() string
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// Service provides file system operations.
type Service struct{}

// NewService creates a new file service.
func NewService() *Service {
	return &Service{}
}

// Read reads a file completely.
func (s *Service) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// Write writes data to a file, creating parent directories if needed.
func (s *Service) Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WriteAtomic writes data atomically using a temp file + rename.
func (s *Service) WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := writeAtomicMkdirAllFn(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp, err := writeAtomicCreateTempFn(dir, ".gcm-tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	defer func() {
		// Clean up temp file on error
		if _, statErr := writeAtomicStatFn(tmpPath); statErr == nil {
			_ = writeAtomicRemoveFn(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("setting permissions: %w", err)
	}

	// Sync ensures data is flushed to stable storage before the rename.
	// Without this, a crash after rename could leave a zero-length file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	// On Windows, os.Rename fails if the target already exists.
	// Remove the target first to ensure atomic replacement.
	if runtimeGOOS == "windows" {
		_ = os.Remove(path)
	}

	if err := writeAtomicRenameFn(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// Exists checks if a file or directory exists.
func (s *Service) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes a file.
func (s *Service) Delete(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting %s: %w", path, err)
	}
	return nil
}

// List returns files in dir matching a glob pattern.
func (s *Service) List(dir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(dir, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", fullPattern, err)
	}
	return matches, nil
}

// CopyFile copies a file from src to dst.
func (s *Service) CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %s: %w", src, err)
	}
	defer srcFile.Close()

	info, err := copyStatFn(srcFile)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", src, err)
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("creating destination %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := copyIOCopyFn(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	return nil
}

// EnsurePermissions validates file permissions.
func (s *Service) EnsurePermissions(path string, expected os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	current := info.Mode().Perm()
	if current != expected {
		if err := os.Chmod(path, expected); err != nil {
			return fmt.Errorf("setting permissions on %s: %w", path, err)
		}
	}
	return nil
}

// ExpandPath expands a leading ~ in a file path to the user's home directory.
func ExpandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	// Handle both Unix (~/) and Windows (~\) path separators
	if len(path) > 1 && (path[:2] == "~/" || path[:2] == "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			rest := path[2:]
			if rest == "" {
				return home + string(filepath.Separator)
			}
			return filepath.Join(home, rest)
		}
	}
	return path
}
