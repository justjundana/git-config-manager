package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

// quickVerifyToken checks token validity with a short deadline without
// mutating the shared GitHubClient. This avoids a data race when verifying
// multiple profiles in sequence with goroutine-based timeouts.
func quickVerifyToken(token, apiURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL+"/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	// Drain body before close to allow connection reuse.
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func quickVerifyGitLabToken(token _provider.TokenSet, apiURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(apiURL, "/")+"/user", nil)
	if err != nil {
		return err
	}
	if token.Bearer() {
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	} else {
		req.Header.Set("PRIVATE-TOKEN", token.AccessToken)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096)) //nolint:errcheck
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func quickVerifyProviderToken(def _provider.Definition, token _provider.TokenSet) error {
	switch def.ID {
	case _provider.GitHubID:
		return quickVerifyToken(token.AccessToken, def.APIURL)
	case _provider.GitLabID:
		return quickVerifyGitLabToken(token, def.APIURL)
	default:
		return nil
	}
}

// padRight pads a string to the given visible width with spaces.
func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show a quick overview of your GCM setup",
		Long: `Display a dashboard of your current GCM state at a glance.

Shows: active profile, all profiles summary, provider auth status,
SSH keys, and any issues that need attention.`,
		Aliases: []string{"st"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	_ui.Header("%s GCM Status", _ui.IconRocket)
	_ui.Blank()

	profiles, _ := ctr.ProfileManager.List()
	currentName, scope, _ := ctr.ProfileSwitcher.Current()
	migrationIssues := make([]string, 0)
	for _, p := range profiles {
		if _, err := migrateProfileSSHKeyPathToProvider(p.Name, p); err != nil {
			migrationIssues = append(migrationIssues, fmt.Sprintf("SSH key rename for %q failed: %v", p.Name, err))
		}
	}

	// Calculate max profile name length for alignment
	maxNameLen := 0
	for _, p := range profiles {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}
	if maxNameLen < 6 {
		maxNameLen = 6
	}
	// Add 1 for breathing room
	maxNameLen++

	// ─── Active Profile ───
	_ui.Print("  %s", _ui.Bold("Active Profile"))
	if currentName == "" {
		_ui.Print("    %s No active profile", _ui.Red(_ui.IconError))
		_ui.Print("      Activate one: %s", _ui.Cyan("gcm use <profile>"))
	} else {
		p, _ := ctr.ProfileManager.Get(currentName)
		if p != nil {
			_ui.Print("    %s %s (%s)", _ui.Green(_ui.IconSuccess), _ui.Bold(currentName), scope.String())
			_ui.Print("      %s <%s>", p.Git.User.Name, p.Git.User.Email)
		} else {
			_ui.Print("    %s %s (%s)", _ui.Green(_ui.IconSuccess), _ui.Bold(currentName), scope.String())
		}
	}

	_ui.Blank()
	_ui.Divider()

	// ─── Profiles ───
	_ui.Blank()
	_ui.Print("  %s %s", _ui.Bold("Profiles"), _ui.Dim(fmt.Sprintf("(%d total)", len(profiles))))

	if len(profiles) == 0 {
		_ui.Print("    %s No profiles yet", _ui.Yellow(_ui.IconWarning))
		_ui.Print("      Create one: %s", _ui.Cyan("gcm profile create work -i"))
	} else {
		for _, p := range profiles {
			marker := _ui.Dim("•")
			if p.Name == currentName {
				marker = _ui.Green("●")
			}
			extras := ""
			if p.SSH != nil {
				extras += " " + _ui.IconKey
			}
			if p.GPG != nil {
				extras += " 🔏"
			}
			_ui.Print("    %s %-*s %s%s", marker, maxNameLen, p.Name, _ui.Dim(p.Git.User.Email), extras)
		}
	}

	_ui.Blank()
	_ui.Divider()

	// ─── Provider Auth ───
	_ui.Blank()
	_ui.Print("  %s", _ui.Bold("Provider Auth"))

	var issues []string
	issues = append(issues, migrationIssues...)

	if len(profiles) == 0 {
		_ui.Print("    %s No profiles configured", _ui.Dim("—"))
	} else {
		type providerAuthEntry struct {
			icon     string
			name     string
			provider string
			username string
			status   string
			hint     string
		}
		entries := make([]providerAuthEntry, len(profiles))
		authIssues := make([][]string, len(profiles))
		maxProviderLen := 0
		maxUserLen := 0
		var wg sync.WaitGroup
		sem := make(chan struct{}, statusVerifyConcurrency())

		for i, p := range profiles {
			if profileHasMultipleProviders(p) {
				entries[i] = providerAuthEntry{
					icon:     _ui.Yellow(_ui.IconWarning),
					name:     p.Name,
					provider: "multiple",
					status:   _ui.Yellow("choose one provider"),
					hint:     fmt.Sprintf("gcm profile edit %s -i", p.Name),
				}
				continue
			}

			def, ok := profileProviderDefinition(p, _provider.CapabilityCredentialHelper)
			if !ok {
				entries[i] = providerAuthEntry{
					icon:     _ui.Yellow(_ui.IconWarning),
					name:     p.Name,
					provider: "—",
					status:   _ui.Dim("no provider"),
					hint:     fmt.Sprintf("gcm connect %s --provider <github|gitlab>", p.Name),
				}
				continue
			}

			account := providerAccountForProfile(p, def.ID)
			providerName := def.DisplayName
			if len(providerName) > maxProviderLen {
				maxProviderLen = len(providerName)
			}

			username := ""
			if account.Username != "" {
				username = "@" + account.Username
			}
			if len(username) > maxUserLen {
				maxUserLen = len(username)
			}

			token, loadErr := loadProviderToken(p.Name, def, p)
			if loadErr != nil || token.AccessToken == "" {
				entries[i] = providerAuthEntry{
					icon:     _ui.Red(_ui.IconError),
					name:     p.Name,
					provider: providerName,
					username: username,
					status:   _ui.Dim("not authenticated"),
					hint:     fmt.Sprintf("gcm connect %s --provider %s", p.Name, def.ID),
				}
				continue
			}

			entries[i] = providerAuthEntry{
				icon:     _ui.Yellow(_ui.IconWarning),
				name:     p.Name,
				provider: providerName,
				username: username,
				status:   _ui.Dim("checking"),
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(idx int, profileName string, def _provider.Definition, token _provider.TokenSet) {
				defer wg.Done()
				defer func() { <-sem }()

				status := _ui.Green("valid")
				icon := _ui.Green(_ui.IconSuccess)
				if verifyErr := quickVerifyProviderToken(def, token); verifyErr != nil {
					if strings.Contains(verifyErr.Error(), "context deadline exceeded") ||
						strings.Contains(verifyErr.Error(), "timeout") {
						status = _ui.Yellow("timeout")
						icon = _ui.Yellow(_ui.IconWarning)
					} else {
						status = _ui.Red("expired/invalid")
						icon = _ui.Red(_ui.IconError)
						authIssues[idx] = append(authIssues[idx], fmt.Sprintf("%s token for %q expired — run: gcm connect %s --provider %s", def.DisplayName, profileName, profileName, def.ID))
					}
				}

				entries[idx].icon = icon
				entries[idx].status = status
			}(i, p.Name, def, token)
		}
		wg.Wait()

		for _, profileIssues := range authIssues {
			issues = append(issues, profileIssues...)
		}

		if maxProviderLen < 10 {
			maxProviderLen = 10
		}
		if maxUserLen < 12 {
			maxUserLen = 12
		}

		for _, e := range entries {
			providerName := padRight(e.provider, maxProviderLen)
			username := padRight(e.username, maxUserLen)
			if e.hint != "" {
				_ui.Print("    %s %-*s %s %s %s", e.icon, maxNameLen, e.name, providerName, _ui.Dim(username), e.status)
				_ui.Print("      %s %s", _ui.Dim("└─"), _ui.Cyan(e.hint))
			} else {
				_ui.Print("    %s %-*s %s %s %s", e.icon, maxNameLen, e.name, providerName, _ui.Dim(username), e.status)
			}
		}
	}

	_ui.Blank()
	_ui.Divider()

	// ─── SSH Keys ───
	_ui.Blank()
	_ui.Print("  %s", _ui.Bold("SSH Keys"))

	// Calculate max key filename length
	maxKeyLen := 0
	for _, p := range profiles {
		if p.SSH != nil && p.SSH.KeyPath != "" {
			kl := len(filepath.Base(p.SSH.KeyPath))
			if kl > maxKeyLen {
				maxKeyLen = kl
			}
		}
	}
	if maxKeyLen < 10 {
		maxKeyLen = 10
	}
	maxKeyLen += 2

	hasKeys := false
	for _, p := range profiles {
		if p.SSH != nil && p.SSH.KeyPath != "" {
			hasKeys = true
			icon := _ui.Green(_ui.IconSuccess)
			if _, statErr := os.Stat(p.SSH.KeyPath); statErr != nil {
				icon = _ui.Red(_ui.IconError)
				issues = append(issues, fmt.Sprintf("SSH key for %q missing at %s", p.Name, p.SSH.KeyPath))
			}
			keyName := padRight(filepath.Base(p.SSH.KeyPath), maxKeyLen)
			_ui.Print("    %s %-*s %s %s", icon, maxNameLen, p.Name, keyName, _ui.Dim(string(p.SSH.KeyType)))
		} else {
			_ui.Print("    %s %-*s %s", _ui.Dim("—"), maxNameLen, p.Name, _ui.Dim("not configured"))
		}
	}

	if !hasKeys && len(profiles) == 0 {
		_ui.Print("    %s No SSH keys configured", _ui.Dim("—"))
		_ui.Print("      Generate: %s", _ui.Cyan("gcm ssh generate <profile>"))
	}

	// ─── Issues / Suggestions ───
	if len(issues) > 0 {
		_ui.Blank()
		_ui.Divider()
		_ui.Blank()
		_ui.Print("  %s %s", _ui.Bold("Issues"), _ui.Red(fmt.Sprintf("(%d)", len(issues))))
		for _, issue := range issues {
			_ui.Print("    %s %s", _ui.Red(_ui.IconArrow), issue)
		}
	}

	// ─── Quick Tips (if new user) ───
	if len(profiles) == 0 {
		_ui.Blank()
		_ui.Divider()
		_ui.Blank()
		_ui.Print("  %s", _ui.Bold("Quick Start"))
		_ui.Print("    Run %s for a guided setup wizard", _ui.Cyan("gcm setup"))
	}

	_ui.Blank()
	return nil
}

func statusVerifyConcurrency() int {
	if ctr == nil || ctr.Config == nil || !ctr.Config.Advanced.ParallelOperations {
		return 1
	}
	return 4
}
