package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/pkg/ui"
	"github.com/justjundana/git-config-manager/pkg/version"

	"github.com/spf13/cobra"
)

const (
	githubRepoOwner = "justjundana"
	githubRepoName  = "git-config-manager"
	githubAPIURL    = "https://api.github.com/repos/" + githubRepoOwner + "/" + githubRepoName
)

// githubRelease represents a GitHub release.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var updateHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

func newUpdateCmd() *cobra.Command {
	var (
		checkOnly  bool
		force      bool
		prerelease bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update GCM to the latest version",
		Long: `Automatically check for and install the latest version of GCM.

Features:
  • Automatic platform detection and binary selection
  • Safe backup and rollback on failure
  • SHA-256 checksum verification
  • Support for stable and pre-release versions

Examples:
  gcm update                  # Update to latest stable
  gcm update --check          # Check without installing
  gcm update --prerelease     # Include pre-releases
  gcm update --force          # Force reinstall even if on latest`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUpdate(checkOnly, force, prerelease)
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "Check for updates without installing")
	cmd.Flags().BoolVar(&force, "force", false, "Force update even if already on latest version")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "Include pre-release versions")

	return cmd
}

func runUpdate(checkOnly, force, prerelease bool) error {
	ui.Info("Checking for updates...")

	latest, err := fetchLatestRelease(prerelease)
	if err != nil {
		ui.Error("Failed to check for updates: %v", err)
		return err
	}

	current := version.Version
	if current == "dev" {
		ui.Warning("Development build detected — updates are not available")
		ui.Print("  Build from source or use the install script to get a release version.")
		return nil
	}

	ui.Blank()
	ui.Detail("Current", current)
	ui.Detail("Latest", latest.TagName)

	if latest.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, latest.PublishedAt); err == nil {
			ui.Detail("Released", t.Format("January 2, 2006"))
		}
	}
	ui.Blank()

	if !force && normalizeVersion(latest.TagName) == normalizeVersion(current) {
		ui.Success("Already on the latest version!")
		ui.Print("  Use --force to reinstall the current version.")
		return nil
	}

	if checkOnly {
		if normalizeVersion(latest.TagName) != normalizeVersion(current) {
			ui.Info("Update available: %s → %s", current, latest.TagName)
			if latest.Body != "" {
				ui.Blank()
				ui.Print("  %s", ui.Bold("Release Notes:"))
				ui.Print("  %s", strings.Repeat("─", 40))
				for _, line := range strings.Split(latest.Body, "\n") {
					ui.Print("  %s", line)
				}
				ui.Print("  %s", strings.Repeat("─", 40))
			}
			ui.Blank()
			ui.Print("  Run 'gcm update' to install this version.")
		}
		return nil
	}

	// Find asset for current platform
	assetName := fmt.Sprintf("gcm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	downloadURL := ""
	for _, asset := range latest.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latest.TagName)
	}

	// Find checksums asset
	checksumsURL := ""
	for _, asset := range latest.Assets {
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
			break
		}
	}

	ui.Info("Downloading %s for %s/%s...", latest.TagName, runtime.GOOS, runtime.GOARCH)

	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine current binary path: %w", err)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return fmt.Errorf("failed to resolve binary path: %w", err)
	}

	binaryDir := filepath.Dir(currentBinary)

	// Download binary to temp file
	tempFile, err := downloadToTemp(downloadURL, binaryDir)
	if err != nil {
		ui.Error("Download failed: %v", err)
		return err
	}
	defer func() {
		if _, statErr := os.Stat(tempFile); statErr == nil {
			os.Remove(tempFile)
		}
	}()

	// Verify checksum if available
	if checksumsURL != "" {
		if err := verifyUpdateChecksum(tempFile, assetName, checksumsURL); err != nil {
			ui.Error("Checksum verification failed: %v", err)
			os.Remove(tempFile)
			return err
		}
		ui.Success("Checksum verified")
	} else {
		ui.Warning("Checksums not available for this release — skipping verification")
	}

	// Replace binary with backup
	backupPath := currentBinary + ".bak"
	if err := os.Rename(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	if err := os.Rename(tempFile, currentBinary); err != nil {
		// Rollback
		ui.Warning("Install failed, restoring previous version...")
		if restoreErr := os.Rename(backupPath, currentBinary); restoreErr != nil {
			ui.Error("Failed to restore backup — manually restore from %s", backupPath)
			return restoreErr
		}
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	if err := os.Chmod(currentBinary, 0755); err != nil {
		ui.Warning("Failed to set executable permissions: %v", err)
	}

	// Clean up backup
	os.Remove(backupPath)

	ui.Blank()
	ui.Success("Updated to %s!", latest.TagName)
	return nil
}

func fetchLatestRelease(includePrerelease bool) (*githubRelease, error) {
	var url string
	if includePrerelease {
		url = githubAPIURL + "/releases?per_page=10"
	} else {
		url = githubAPIURL + "/releases/latest"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "gcm/"+version.Version)

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if includePrerelease {
		var releases []githubRelease
		if err := json.Unmarshal(body, &releases); err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("no releases found")
		}
		return &releases[0], nil
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadToTemp(url, dir string) (string, error) {
	resp, err := updateHTTPClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tempFile, err := os.CreateTemp(dir, "gcm-update-*.bin")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write binary: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

func verifyUpdateChecksum(filePath, assetName, checksumsURL string) error {
	resp, err := updateHTTPClient.Get(checksumsURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch checksums: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse checksums.txt format: "<hash>  <filename>"
	var expectedHash string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			expectedHash = parts[0]
			break
		}
	}
	if expectedHash == "" {
		return fmt.Errorf("no checksum entry for %s", assetName)
	}

	// Compute SHA-256 of downloaded file
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}
