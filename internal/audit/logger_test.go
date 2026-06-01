package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
)

func setupTestLogger(t *testing.T, enabled bool) *Logger {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Security.AuditLog = enabled
	l := NewLogger(cfg)
	l.logDir = filepath.Join(tmp, "logs")
	return l
}

func TestNewLogger(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.DefaultConfig()
	cfg.Security.AuditLog = true
	l := NewLogger(cfg)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if !l.enabled {
		t.Error("expected enabled=true")
	}
}

func TestLog_Disabled(t *testing.T) {
	l := setupTestLogger(t, false)
	l.Log(ActionProfileCreate, "work", nil, nil)
	// No file should be created
	_, err := os.Stat(l.logDir)
	if err == nil {
		t.Error("log dir should not exist when logging is disabled")
	}
}

func TestLog_Success(t *testing.T) {
	l := setupTestLogger(t, true)
	l.Log(ActionProfileCreate, "work", map[string]string{"key": "val"}, nil)

	entries := readEntries(t, l.logDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Action != ActionProfileCreate {
		t.Errorf("action = %q, want %q", e.Action, ActionProfileCreate)
	}
	if e.Profile != "work" {
		t.Errorf("profile = %q, want %q", e.Profile, "work")
	}
	if !e.Success {
		t.Error("expected success=true")
	}
	if e.Error != "" {
		t.Errorf("expected empty error, got %q", e.Error)
	}
	if e.Details["key"] != "val" {
		t.Errorf("details[key] = %q, want %q", e.Details["key"], "val")
	}
}

func TestLog_WithError(t *testing.T) {
	l := setupTestLogger(t, true)
	l.Log(ActionProfileDelete, "test", nil, fmt.Errorf("something failed"))

	entries := readEntries(t, l.logDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Success {
		t.Error("expected success=false")
	}
	if e.Error != "something failed" {
		t.Errorf("error = %q, want %q", e.Error, "something failed")
	}
}

func TestLog_MultipleEntries(t *testing.T) {
	l := setupTestLogger(t, true)
	l.Log(ActionProfileCreate, "a", nil, nil)
	l.Log(ActionProfileActivate, "b", nil, nil)
	l.Log(ActionSSHGenerate, "c", nil, nil)

	entries := readEntries(t, l.logDir)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestLog_ConcurrentWrites(t *testing.T) {
	l := setupTestLogger(t, true)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			l.Log(ActionProfileCreate, fmt.Sprintf("p%d", n), nil, nil)
		}(i)
	}
	wg.Wait()

	entries := readEntries(t, l.logDir)
	if len(entries) != 20 {
		t.Errorf("expected 20 entries, got %d (possible race corruption)", len(entries))
	}
}

func TestReadLog(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o700)

	date := time.Now().Format("2006-01-02")
	e := Entry{
		Timestamp: time.Now().UTC(),
		Action:    ActionBackupCreate,
		Profile:   "test",
		Success:   true,
	}
	data, _ := json.Marshal(e)
	os.WriteFile(filepath.Join(logDir, date+".jsonl"), append(data, '\n'), 0o600)

	entries, err := ReadLog(date)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != ActionBackupCreate {
		t.Errorf("action = %q", entries[0].Action)
	}
}

func TestReadLog_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	_, err := ReadLog("2099-01-01")
	if err == nil {
		t.Error("expected error for nonexistent log file")
	}
}

func TestReadLog_MalformedLines(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o700)

	content := "{\"action\":\"profile.create\",\"success\":true}\n{bad json}\n{\"action\":\"ssh.generate\",\"success\":true}\n"
	os.WriteFile(filepath.Join(logDir, "2024-01-01.jsonl"), []byte(content), 0o600)

	entries, err := ReadLog("2024-01-01")
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	// The bad JSON line should be skipped
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries (skipping malformed), got %d", len(entries))
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"a\nb\nc\n", 3},
		{"single", 1},
		{"", 0},
		{"\n", 1},
		{"a\n\nb", 3},
	}
	for _, tt := range tests {
		lines := splitLines([]byte(tt.input))
		if len(lines) != tt.want {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(lines), tt.want)
		}
	}
}

func TestLogFilePermissions(t *testing.T) {
	l := setupTestLogger(t, true)
	l.Log(ActionProfileCreate, "test", nil, nil)

	entries, _ := filepath.Glob(filepath.Join(l.logDir, "*.jsonl"))
	if len(entries) == 0 {
		t.Fatal("no log file created")
	}
	info, err := os.Stat(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("log file perm = %04o, want 0600", perm)
	}
}

func TestAllActions(t *testing.T) {
	l := setupTestLogger(t, true)
	actions := []Action{
		ActionProfileCreate, ActionProfileDelete,
		ActionProfileActivate, ActionSSHGenerate, ActionGPGGenerate,
		ActionGitHubLogin, ActionGitHubLogout, ActionBackupCreate,
		ActionBackupRestore, ActionShellInit,
	}
	for _, a := range actions {
		l.Log(a, "test", nil, nil)
	}

	entries := readEntries(t, l.logDir)
	if len(entries) != len(actions) {
		t.Errorf("expected %d entries, got %d", len(actions), len(entries))
	}
}

// readEntries reads all JSONL entries from the log directory.
func readEntries(t *testing.T, logDir string) []Entry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	if err != nil || len(files) == 0 {
		return nil
	}
	var all []Entry
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var e Entry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatalf("unmarshal entry: %v\nline: %s", err, line)
			}
			all = append(all, e)
		}
	}
	return all
}

func TestWrite_MarshalError(t *testing.T) {
	l := setupTestLogger(t, true)

	old := marshalFn
	marshalFn = func(v interface{}) ([]byte, error) {
		return nil, fmt.Errorf("marshal failure")
	}
	defer func() { marshalFn = old }()

	err := l.write(Entry{
		Timestamp: time.Now().UTC(),
		Action:    ActionProfileCreate,
		Profile:   "test",
		Success:   true,
	})
	if err == nil {
		t.Error("expected error when marshal fails")
	}
}

func TestWrite_MkdirAllError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := config.DefaultConfig()
	cfg.Security.AuditLog = true
	l := NewLogger(cfg)
	// Set logDir to an impossible path so MkdirAll fails.
	l.logDir = "/dev/null/impossible/logs"

	err := l.write(Entry{
		Timestamp: time.Now().UTC(),
		Action:    ActionProfileCreate,
		Profile:   "test",
		Success:   true,
	})
	if err == nil {
		t.Error("expected error when logDir is impossible")
	}
}

func TestReadLog_EmptyLinesBetweenEntries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o700)

	// File with empty lines between entries (exercises len(line) == 0 continue)
	content := "{\"action\":\"profile.create\",\"success\":true}\n\n\n{\"action\":\"ssh.generate\",\"success\":true}\n"
	os.WriteFile(filepath.Join(logDir, "2024-02-01.jsonl"), []byte(content), 0o600)

	entries, err := ReadLog("2024-02-01")
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (skipping empty lines), got %d", len(entries))
	}
}

func TestReadLog_PathTraversal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := ReadLog("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal in ReadLog")
	}
}

func TestReadLog_WithMultipleEntries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o700)

	var content string
	for i := 0; i < 5; i++ {
		e := Entry{
			Timestamp: time.Now().UTC(),
			Action:    ActionProfileCreate,
			Profile:   fmt.Sprintf("p%d", i),
			Success:   true,
		}
		data, _ := json.Marshal(e)
		content += string(data) + "\n"
	}
	os.WriteFile(filepath.Join(logDir, "2024-06-15.jsonl"), []byte(content), 0o600)

	entries, err := ReadLog("2024-06-15")
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestReadLog_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o700)
	os.WriteFile(filepath.Join(logDir, "2024-07-01.jsonl"), []byte(""), 0o600)

	entries, err := ReadLog("2024-07-01")
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestLog_WithDetails(t *testing.T) {
	l := setupTestLogger(t, true)
	details := map[string]string{
		"path":   "/tmp/backup.tar.gz",
		"key_id": "ABCDEF",
		"scope":  "global",
	}
	l.Log(ActionBackupCreate, "work", details, nil)

	entries := readEntries(t, l.logDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Details["path"] != "/tmp/backup.tar.gz" {
		t.Errorf("details[path] = %q", entries[0].Details["path"])
	}
	if entries[0].Details["key_id"] != "ABCDEF" {
		t.Errorf("details[key_id] = %q", entries[0].Details["key_id"])
	}
}

func TestLog_TimestampIsUTC(t *testing.T) {
	l := setupTestLogger(t, true)
	l.Log(ActionProfileCreate, "utctest", nil, nil)

	entries := readEntries(t, l.logDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.Location() != time.UTC {
		t.Errorf("timestamp location = %v, want UTC", entries[0].Timestamp.Location())
	}
}

func TestWrite_UnwritableLogDir(t *testing.T) {
	tmp := t.TempDir()
	// Create a file where log dir should be
	logDir := filepath.Join(tmp, "logs")
	os.WriteFile(logDir, []byte("blocker"), 0o644)

	l := &Logger{
		enabled: true,
		logDir:  logDir,
	}
	err := l.write(Entry{Action: "test"})
	if err == nil {
		t.Fatal("expected error when logDir creation blocked")
	}
}

func TestWrite_UnwritableLogFile(t *testing.T) {
	tmp := t.TempDir()
	logDir := filepath.Join(tmp, "logs")
	os.MkdirAll(logDir, 0o755)

	// Make log dir unwritable so file creation fails
	os.Chmod(logDir, 0o000)
	defer os.Chmod(logDir, 0o755)

	l := &Logger{
		enabled: true,
		logDir:  logDir,
	}
	err := l.write(Entry{Action: "test"})
	if err == nil {
		t.Fatal("expected error when log file cannot be created")
	}
}

func TestReadLog_PathTraversalBackslash(t *testing.T) {
	_, err := ReadLog(`2024\.01`)
	if err == nil {
		t.Fatal("expected error for backslash in date")
	}
}

func TestReadLog_NonExistentDate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	_, err := ReadLog("2099-01-01")
	if err == nil {
		t.Fatal("expected error for nonexistent log file")
	}
}

func TestReadLog_CorruptJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	logDir := filepath.Join(tmp, ".gcm", "logs")
	os.MkdirAll(logDir, 0o755)
	os.WriteFile(filepath.Join(logDir, "2024-01-01.jsonl"), []byte("not json\n"), 0o600)

	entries, err := ReadLog("2024-01-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Corrupt lines are skipped, so entries should be empty
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}
