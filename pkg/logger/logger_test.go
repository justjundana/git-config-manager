package logger

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    Level
		logLevel Level
		msg      string
		want     bool // Should message appear in output
	}{
		{"debug at debug level", LevelDebug, LevelDebug, "test debug", true},
		{"info at debug level", LevelInfo, LevelDebug, "test info", true},
		{"debug at info level", LevelDebug, LevelInfo, "test debug", false},
		{"error at info level", LevelError, LevelInfo, "test error", true},
		{"info at error level", LevelInfo, LevelError, "test info", false},
		{"warn at warn level", LevelWarn, LevelWarn, "test warn", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := New(tt.logLevel, buf)

			log.log(tt.level, tt.msg)

			got := buf.String()
			if tt.want && !strings.Contains(got, tt.msg) {
				t.Errorf("expected message %q in output, got: %q", tt.msg, got)
			}
			if !tt.want && strings.Contains(got, tt.msg) {
				t.Errorf("did not expect message %q in output, got: %q", tt.msg, got)
			}
		})
	}
}

func TestLogOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelInfo, buf)

	log.Info("hello world")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("output should contain [INFO], got: %s", output)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("output should contain 'hello world', got: %s", output)
	}
}

func TestLogFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelDebug, buf)

	log.Debug("operation", F("key", "value"), F("count", 42))

	output := buf.String()
	if !strings.Contains(output, "key=value") {
		t.Errorf("output should contain key=value, got: %s", output)
	}
	if !strings.Contains(output, "count=42") {
		t.Errorf("output should contain count=42, got: %s", output)
	}
}

func TestSetVerbose(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelInfo, buf)

	log.Debug("should not appear")
	if buf.Len() > 0 {
		t.Error("debug messages should be suppressed at info level")
	}

	log.SetVerbose(true)
	log.Debug("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("debug messages should appear after setting verbose")
	}
}

func TestSetQuiet(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelInfo, buf)

	log.SetQuiet(true)
	log.Info("should not appear")
	log.Warn("should not appear")
	if buf.Len() > 0 {
		t.Error("info/warn should be suppressed in quiet mode")
	}

	log.Error("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("errors should still appear in quiet mode")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %s, want %s", tt.level, got, tt.want)
		}
	}
}

func TestDefault(t *testing.T) {
	log1 := Default()
	log2 := Default()
	if log1 != log2 {
		t.Error("Default() should return the same instance")
	}
}

func TestSetLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelInfo, buf)

	log.SetLevel(LevelError)
	log.Info("suppressed")
	if buf.Len() > 0 {
		t.Error("info should be suppressed at error level")
	}

	log.Error("visible")
	if !strings.Contains(buf.String(), "visible") {
		t.Error("error should be visible at error level")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	// Just verify they don't panic
	buf := &bytes.Buffer{}
	oldDefault := defaultLogger
	defaultLogger = New(LevelDebug, buf)
	defer func() { defaultLogger = oldDefault }()

	Debug("debug msg", F("k", "v"))
	Info("info msg")
	Warn("warn msg")
	Error("error msg")

	out := buf.String()
	if !strings.Contains(out, "debug msg") {
		t.Error("expected debug msg")
	}
	if !strings.Contains(out, "info msg") {
		t.Error("expected info msg")
	}
	if !strings.Contains(out, "warn msg") {
		t.Error("expected warn msg")
	}
	if !strings.Contains(out, "error msg") {
		t.Error("expected error msg")
	}
}

func TestF(t *testing.T) {
	f := F("key", 42)
	if f.Key != "key" {
		t.Errorf("Key = %q", f.Key)
	}
	if f.Value != 42 {
		t.Errorf("Value = %v", f.Value)
	}
}

func TestNew(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(LevelWarn, buf)
	if log == nil {
		t.Fatal("expected non-nil logger")
	}
	log.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("info should be suppressed at warn level")
	}
	log.Warn("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("warn should appear at warn level")
	}
}

func TestFatal_ExitsWithCode1(t *testing.T) {
	if os.Getenv("TEST_FATAL_LOGGER") == "1" {
		log := New(LevelDebug, os.Stderr)
		log.Fatal("fatal message")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatal_ExitsWithCode1")
	cmd.Env = append(os.Environ(), "TEST_FATAL_LOGGER=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected process to exit with non-zero code")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(string(output), "fatal message") {
		t.Errorf("output should contain 'fatal message', got: %s", output)
	}
}

func TestFatal_PackageLevel_ExitsWithCode1(t *testing.T) {
	if os.Getenv("TEST_FATAL_PKG") == "1" {
		defaultLogger = New(LevelDebug, os.Stderr)
		Fatal("pkg fatal message")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatal_PackageLevel_ExitsWithCode1")
	cmd.Env = append(os.Environ(), "TEST_FATAL_PKG=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected process to exit with non-zero code")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(string(output), "pkg fatal message") {
		t.Errorf("output should contain 'pkg fatal message', got: %s", output)
	}
}
