// Package audit provides audit logging for GCM operations.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
)

// marshalFn is used by write to serialize entries. Tests may override it.
var marshalFn = json.Marshal

// Action represents an auditable event type.
type Action string

const (
	ActionProfileCreate   Action = "profile.create"
	ActionProfileUpdate   Action = "profile.update"
	ActionProfileDelete   Action = "profile.delete"
	ActionProfileActivate Action = "profile.activate"
	ActionSSHGenerate     Action = "ssh.generate"
	ActionGPGGenerate     Action = "gpg.generate"
	ActionGitHubLogin     Action = "github.login"
	ActionGitHubLogout    Action = "github.logout"
	ActionProviderLogin   Action = "provider.login"
	ActionProviderLogout  Action = "provider.logout"
	ActionBackupCreate    Action = "backup.create"
	ActionBackupRestore   Action = "backup.restore"
	ActionShellInit       Action = "shell.init"
)

// Entry represents a single audit log entry.
type Entry struct {
	Timestamp time.Time         `json:"timestamp"`
	Action    Action            `json:"action"`
	Profile   string            `json:"profile,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	Success   bool              `json:"success"`
	Error     string            `json:"error,omitempty"`
}

// Logger writes audit entries to a JSONL log file.
type Logger struct {
	mu      sync.Mutex
	enabled bool
	logDir  string
}

// NewLogger creates an audit logger.
func NewLogger(cfg *config.Config) *Logger {
	return &Logger{
		enabled: cfg.Security.AuditLog,
		logDir:  filepath.Join(config.GCMDir(), "logs"),
	}
}

// Log records an audit event.
func (l *Logger) Log(action Action, profileName string, details map[string]string, err error) {
	if !l.enabled {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Profile:   profileName,
		Details:   details,
		Success:   err == nil,
	}
	if err != nil {
		entry.Error = err.Error()
	}

	_ = l.write(entry)
}

func (l *Logger) write(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(l.logDir, 0700); err != nil {
		return err
	}

	logFile := filepath.Join(l.logDir, time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := marshalFn(entry)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(f, string(data))
	return err
}

// ReadLog reads audit entries from a specific date.
func ReadLog(date string) ([]Entry, error) {
	// Validate date to prevent path traversal.
	if strings.ContainsAny(date, `/\.`) {
		return nil, fmt.Errorf("invalid date %q", date)
	}
	logFile := filepath.Join(config.GCMDir(), "logs", date+".jsonl")
	data, err := os.ReadFile(logFile)
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
