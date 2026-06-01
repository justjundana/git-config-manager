package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Version should not be empty")
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %s, want %s", info.OS, runtime.GOOS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %s, want %s", info.Arch, runtime.GOARCH)
	}
}

func TestInfoString(t *testing.T) {
	info := Get()
	s := info.String()

	if !strings.Contains(s, "gcm") {
		t.Errorf("String() should contain 'gcm', got: %s", s)
	}
	if !strings.Contains(s, info.Version) {
		t.Errorf("String() should contain version, got: %s", s)
	}
}

func TestInfoShort(t *testing.T) {
	info := Get()
	s := info.Short()

	expected := "gcm " + info.Version
	if s != expected {
		t.Errorf("Short() = %s, want %s", s, expected)
	}
}
