package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// cache_dir comes from the config file, which is user-editable and — as this
// project has learned — corruptible. gcm clean must not turn into "delete my
// home directory".
func TestClean_RefusesToDeleteHome(t *testing.T) {
	ctr := withRepairTestContainer(t)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	canary := filepath.Join(home, "canary.txt")
	if err := os.WriteFile(canary, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("seed canary: %v", err)
	}

	ctr.Config.CacheDir = home

	cmd := newCleanCmd()
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("clean must refuse when cache_dir is the home directory")
	}
	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("home contents must survive: %v", err)
	}
}

func TestClean_RemovesTheConfiguredCacheDir(t *testing.T) {
	ctr := withRepairTestContainer(t)

	stale := filepath.Join(ctr.Config.CacheDir, "stale.bin")
	if err := os.MkdirAll(ctr.Config.CacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cmd := newCleanCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("cache contents should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(ctr.Config.CacheDir); err != nil {
		t.Errorf("cache dir should be recreated: %v", err)
	}
}
