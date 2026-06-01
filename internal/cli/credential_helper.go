package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	githubpkg "github.com/justjundana/git-config-manager/internal/github"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"

	"github.com/spf13/cobra"
)

// credentialHelperTimeout bounds git-config operations to prevent hangs.
const credentialHelperTimeout = 10 * time.Second

func newCredentialHelperCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential-helper",
		Short: "Git credential helper (used internally by git)",
		Long: `GCM's built-in credential helper for git.

This command is called automatically by git when it needs provider credentials.
It reads the active profile, resolves the provider by host, loads the encrypted
token, and returns it to git — bypassing the system keychain entirely.

You should not need to call this manually. It is registered during "gcm init"
and works transparently with git push/pull/clone.`,
		Hidden: true, // Don't clutter normal help output
	}

	cmd.AddCommand(&cobra.Command{
		Use:    "get",
		Short:  "Provide credentials to git",
		Hidden: true,
		RunE:   credentialHelperGet,
	})

	cmd.AddCommand(&cobra.Command{
		Use:    "store",
		Short:  "Store credentials (no-op, GCM manages its own store)",
		Hidden: true,
		RunE:   func(_ *cobra.Command, _ []string) error { return nil },
	})

	cmd.AddCommand(&cobra.Command{
		Use:    "erase",
		Short:  "Erase credentials (no-op, GCM manages its own store)",
		Hidden: true,
		RunE:   func(_ *cobra.Command, _ []string) error { return nil },
	})

	return cmd
}

// credentialHelperGet is called by git when it needs credentials.
// It determines the active profile, loads the token, and outputs credentials.
func credentialHelperGet(_ *cobra.Command, _ []string) error {
	// When git clone/fetch/push invokes this credential helper, it sets GIT_DIR
	// to the target repository (e.g., the clone destination). This causes
	// ProfileSwitcher.Current() → findGitDir() → git rev-parse --git-dir to
	// return the clone target instead of the user's actual working directory.
	// As a result, the session marker (.git/gcm-session) cannot be found and
	// the wrong profile's credentials are returned.
	//
	// Fix: clear GIT_DIR so that profile resolution uses the real cwd.
	// This is safe because the credential helper is a short-lived subprocess.
	os.Unsetenv("GIT_DIR")

	input := parseCredentialInput()

	host := input["host"]
	protocol := input["protocol"]
	if protocol == "" {
		protocol = "https"
	}

	def, ok := ctr.ProviderRegistry.ResolveHost(host)
	if !ok || !def.Capabilities.Has(providerpkg.CapabilityCredentialHelper) {
		return nil // Not our domain — let other helpers handle it
	}

	// Determine current active profile
	currentProfile, _, err := ctr.ProfileSwitcher.Current()
	if err != nil || currentProfile == "" {
		return nil // No active profile — cannot provide credentials
	}

	p, err := ctr.ProfileManager.Get(currentProfile)
	if err != nil || p == nil {
		return nil
	}
	if !profileUsesProvider(p, def.ID) {
		return nil
	}

	// Load token for this profile/provider from GCM's encrypted store.
	token, err := loadProviderToken(currentProfile, def, p)
	if err != nil || token.AccessToken == "" {
		return nil // No token available
	}

	account := providerAccountForProfile(p, def.ID)
	username := def.CredentialUsername(currentProfile, account.Username, token)

	// Sanitize all output values to prevent credential protocol injection
	// (newlines could inject additional key=value pairs).
	protocol = githubpkg.SanitizeCredentialField(protocol)
	host = githubpkg.SanitizeCredentialField(host)
	username = githubpkg.SanitizeCredentialField(username)
	password := githubpkg.SanitizeCredentialField(token.AccessToken)

	// Output credentials in git credential protocol format
	fmt.Fprintf(os.Stdout, "protocol=%s\n", protocol)
	fmt.Fprintf(os.Stdout, "host=%s\n", host)
	fmt.Fprintf(os.Stdout, "username=%s\n", username)
	fmt.Fprintf(os.Stdout, "password=%s\n", password)

	return nil
}

// parseCredentialInput reads git credential protocol key=value pairs from stdin.
func parseCredentialInput() map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			result[k] = v
		}
	}
	return result
}

// IsCredentialHelperConfigured checks whether GCM is registered for GitHub hosts.
func IsCredentialHelperConfigured() bool {
	servers := []string{"https://github.com"}
	if def, ok := ctr.ProviderRegistry.Get(providerpkg.GitHubID); ok {
		servers = credentialHelperServersFor(def)
	}
	for _, server := range servers {
		if !IsCredentialHelperConfiguredFor(server) {
			return false
		}
	}
	return len(servers) > 0
}

// IsCredentialHelperConfiguredFor checks whether GCM is registered for server.
func IsCredentialHelperConfiguredFor(server string) bool {
	key := fmt.Sprintf("credential.%s.helper", server)
	ctx, cancel := context.WithTimeout(context.Background(), credentialHelperTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "--get-all", key).Output()
	if err != nil {
		return false
	}

	return credentialHelperConfigContainsGCM(string(out))
}

// RegisterCredentialHelper configures git to use GCM as the credential helper
// for all configured provider hosts.
func RegisterCredentialHelper() error {
	// Find the GCM binary path
	gcmPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine GCM binary path: %w", err)
	}

	// Resolve symlinks to get the real path
	gcmPath, err = resolveExecutablePath(gcmPath)
	if err != nil {
		return fmt.Errorf("cannot resolve GCM binary path: %w", err)
	}

	helperValue := credentialHelperCommand(gcmPath)

	for _, server := range credentialHelperServers() {
		key := fmt.Sprintf("credential.%s.helper", server)
		ctx, cancel := context.WithTimeout(context.Background(), credentialHelperTimeout)

		// Remove any existing GCM credential helper entries for this server
		_ = exec.CommandContext(ctx, "git", "config", "--global", "--unset-all", key).Run()

		// Set empty value first to reset/override global credential helpers for this host
		if err := exec.CommandContext(ctx, "git", "config", "--global", key, "").Run(); err != nil {
			cancel()
			return fmt.Errorf("failed to reset credential helper for %s: %w", server, err)
		}

		// Add GCM as the credential helper
		if err := exec.CommandContext(ctx, "git", "config", "--global", "--add", key, helperValue).Run(); err != nil {
			cancel()
			return fmt.Errorf("failed to register credential helper for %s: %w", server, err)
		}
		cancel()
	}

	return nil
}

// UnregisterCredentialHelper removes GCM from git's credential helper
// configuration and restores the default system behavior.
func UnregisterCredentialHelper() error {
	for _, server := range credentialHelperServers() {
		key := fmt.Sprintf("credential.%s.helper", server)
		ctx, cancel := context.WithTimeout(context.Background(), credentialHelperTimeout)
		_ = exec.CommandContext(ctx, "git", "config", "--global", "--unset-all", key).Run()
		cancel()
	}
	return nil
}

func credentialHelperServers() []string {
	seen := make(map[string]bool)
	servers := make([]string, 0)
	for _, def := range ctr.ProviderRegistry.All() {
		if !def.Capabilities.Has(providerpkg.CapabilityCredentialHelper) {
			continue
		}
		for _, server := range credentialHelperServersFor(def) {
			if server == "" || seen[server] {
				continue
			}
			seen[server] = true
			servers = append(servers, server)
		}
	}
	return servers
}

func credentialHelperServersFor(def providerpkg.Definition) []string {
	servers := make([]string, 0, len(def.GitHosts))
	for _, host := range def.GitHosts {
		if normalized := providerpkg.NormalizeHost(host); normalized != "" {
			servers = append(servers, "https://"+normalized)
		}
	}
	if len(servers) == 0 {
		server := strings.TrimRight(def.CredentialServer(), "/")
		if server != "" {
			servers = append(servers, server)
		}
	}
	return servers
}

func credentialHelperCommand(gcmPath string) string {
	return fmt.Sprintf("!%s credential-helper", shellQuote(gcmPath))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func credentialHelperConfigContainsGCM(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "gcm") && strings.Contains(line, "credential-helper") {
			return true
		}
	}
	return false
}

// resolveExecutablePath resolves the real path of the executable,
// following symlinks. Uses filepath.EvalSymlinks which is portable across
// all platforms (unlike the external "realpath" command).
func resolveExecutablePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If EvalSymlinks fails (e.g. broken symlink), return path as-is.
		return path, nil
	}
	return resolved, nil
}

// sanitizeCredField removed; using githubpkg.SanitizeCredentialField instead.
