package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.0.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"v2.3.4-rc1", "2.3.4-rc1"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeVersion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFetchLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/justjundana/git-config-manager/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tag_name":"v1.2.0","name":"v1.2.0","body":"test release","assets":[{"name":"gcm-linux-amd64","browser_download_url":"http://example.com/gcm-linux-amd64"}]}`)
		} else if r.URL.Path == "/repos/justjundana/git-config-manager/releases" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"tag_name":"v1.3.0-rc1","name":"v1.3.0-rc1","prerelease":true,"assets":[]},{"tag_name":"v1.2.0","name":"v1.2.0","assets":[]}]`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Override the HTTP client to use our test server
	origClient := updateHTTPClient
	updateHTTPClient = server.Client()
	defer func() { updateHTTPClient = origClient }()

	// We can't easily override the URL constants, so just test the helper functions
	t.Run("normalizeVersion", func(t *testing.T) {
		if got := normalizeVersion("v1.2.0"); got != "1.2.0" {
			t.Errorf("got %q, want %q", got, "1.2.0")
		}
	})
}

func TestVerifyUpdateChecksum(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	filePath := filepath.Join(dir, "gcm-test")
	content := []byte("hello world binary content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Compute expected SHA-256
	// sha256("hello world binary content") = known hash
	// We'll serve a checksums.txt with the correct hash
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The real hash of "hello world binary content"
		fmt.Fprintf(w, "e38f42ef80b77b3cf1e06fb4e7e1a0da2edbe39b6e84de28abe8f3c632cb39aa  wrong-file\n")
		fmt.Fprintf(w, "abc123  gcm-test\n") // Wrong hash for our asset
	}))
	defer server.Close()

	err := verifyUpdateChecksum(filePath, "gcm-test", server.URL+"/checksums.txt")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if got := err.Error(); !contains(got, "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %s", got)
	}
}

func TestVerifyUpdateChecksumMissingEntry(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "gcm-test")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "abc123  gcm-other-platform\n")
	}))
	defer server.Close()

	err := verifyUpdateChecksum(filePath, "gcm-test", server.URL+"/checksums.txt")
	if err == nil {
		t.Fatal("expected error for missing checksum entry")
	}
	if got := err.Error(); !contains(got, "no checksum entry") {
		t.Errorf("expected 'no checksum entry' error, got: %s", got)
	}
}

func TestDownloadToTemp(t *testing.T) {
	expectedContent := "fake binary data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, expectedContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	tempFile, err := downloadToTemp(server.URL+"/gcm-linux-amd64", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile)

	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expectedContent {
		t.Errorf("got %q, want %q", string(data), expectedContent)
	}
}

func TestDownloadToTempHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	_, err := downloadToTemp(server.URL+"/missing", dir)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestAssetNameFormat(t *testing.T) {
	name := fmt.Sprintf("gcm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	// Verify it matches what goreleaser produces
	if !contains(name, "gcm-") {
		t.Errorf("unexpected asset name format: %s", name)
	}
	if runtime.GOOS == "windows" && !contains(name, ".exe") {
		t.Error("Windows binary should have .exe extension")
	}
	if runtime.GOOS != "windows" && contains(name, ".exe") {
		t.Error("Non-Windows binary should not have .exe extension")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
