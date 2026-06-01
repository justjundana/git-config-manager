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

	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
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

func quickVerifyGitLabToken(token providerpkg.TokenSet, apiURL string) error {
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

func quickVerifyProviderToken(def providerpkg.Definition, token providerpkg.TokenSet) error {
	switch def.ID {
	case providerpkg.GitHubID:
		return quickVerifyToken(token.AccessToken, def.APIURL)
	case providerpkg.GitLabID:
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
	ui.Header("%s GCM Status", ui.IconRocket)
	ui.Blank()

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
	ui.Print("  %s", ui.Bold("Active Profile"))
	if currentName == "" {
		ui.Print("    %s No active profile", ui.Red(ui.IconError))
		ui.Print("      Activate one: %s", ui.Cyan("gcm use <profile>"))
	} else {
		p, _ := ctr.ProfileManager.Get(currentName)
		if p != nil {
			ui.Print("    %s %s (%s)", ui.Green(ui.IconSuccess), ui.Bold(currentName), scope.String())
			ui.Print("      %s <%s>", p.Git.User.Name, p.Git.User.Email)
		} else {
			ui.Print("    %s %s (%s)", ui.Green(ui.IconSuccess), ui.Bold(currentName), scope.String())
		}
	}

	ui.Blank()
	ui.Divider()

	// ─── Profiles ───
	ui.Blank()
	ui.Print("  %s %s", ui.Bold("Profiles"), ui.Dim(fmt.Sprintf("(%d total)", len(profiles))))

	if len(profiles) == 0 {
		ui.Print("    %s No profiles yet", ui.Yellow(ui.IconWarning))
		ui.Print("      Create one: %s", ui.Cyan("gcm profile create work -i"))
	} else {
		for _, p := range profiles {
			marker := ui.Dim("•")
			if p.Name == currentName {
				marker = ui.Green("●")
			}
			extras := ""
			if p.SSH != nil {
				extras += " " + ui.IconKey
			}
			if p.GPG != nil {
				extras += " 🔏"
			}
			ui.Print("    %s %-*s %s%s", marker, maxNameLen, p.Name, ui.Dim(p.Git.User.Email), extras)
		}
	}

	ui.Blank()
	ui.Divider()

	// ─── Provider Auth ───
	ui.Blank()
	ui.Print("  %s", ui.Bold("Provider Auth"))

	var issues []string
	issues = append(issues, migrationIssues...)

	if len(profiles) == 0 {
		ui.Print("    %s No profiles configured", ui.Dim("—"))
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
					icon:     ui.Yellow(ui.IconWarning),
					name:     p.Name,
					provider: "multiple",
					status:   ui.Yellow("choose one provider"),
					hint:     fmt.Sprintf("gcm profile edit %s -i", p.Name),
				}
				continue
			}

			def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper)
			if !ok {
				entries[i] = providerAuthEntry{
					icon:     ui.Yellow(ui.IconWarning),
					name:     p.Name,
					provider: "—",
					status:   ui.Dim("no provider"),
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
					icon:     ui.Red(ui.IconError),
					name:     p.Name,
					provider: providerName,
					username: username,
					status:   ui.Dim("not authenticated"),
					hint:     fmt.Sprintf("gcm connect %s --provider %s", p.Name, def.ID),
				}
				continue
			}

			entries[i] = providerAuthEntry{
				icon:     ui.Yellow(ui.IconWarning),
				name:     p.Name,
				provider: providerName,
				username: username,
				status:   ui.Dim("checking"),
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(idx int, profileName string, def providerpkg.Definition, token providerpkg.TokenSet) {
				defer wg.Done()
				defer func() { <-sem }()

				status := ui.Green("valid")
				icon := ui.Green(ui.IconSuccess)
				if verifyErr := quickVerifyProviderToken(def, token); verifyErr != nil {
					if strings.Contains(verifyErr.Error(), "context deadline exceeded") ||
						strings.Contains(verifyErr.Error(), "timeout") {
						status = ui.Yellow("timeout")
						icon = ui.Yellow(ui.IconWarning)
					} else {
						status = ui.Red("expired/invalid")
						icon = ui.Red(ui.IconError)
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
				ui.Print("    %s %-*s %s %s %s", e.icon, maxNameLen, e.name, providerName, ui.Dim(username), e.status)
				ui.Print("      %s %s", ui.Dim("└─"), ui.Cyan(e.hint))
			} else {
				ui.Print("    %s %-*s %s %s %s", e.icon, maxNameLen, e.name, providerName, ui.Dim(username), e.status)
			}
		}
	}

	ui.Blank()
	ui.Divider()

	// ─── SSH Keys ───
	ui.Blank()
	ui.Print("  %s", ui.Bold("SSH Keys"))

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
			icon := ui.Green(ui.IconSuccess)
			if _, statErr := os.Stat(p.SSH.KeyPath); statErr != nil {
				icon = ui.Red(ui.IconError)
				issues = append(issues, fmt.Sprintf("SSH key for %q missing at %s", p.Name, p.SSH.KeyPath))
			}
			keyName := padRight(filepath.Base(p.SSH.KeyPath), maxKeyLen)
			ui.Print("    %s %-*s %s %s", icon, maxNameLen, p.Name, keyName, ui.Dim(string(p.SSH.KeyType)))
		} else {
			ui.Print("    %s %-*s %s", ui.Dim("—"), maxNameLen, p.Name, ui.Dim("not configured"))
		}
	}

	if !hasKeys && len(profiles) == 0 {
		ui.Print("    %s No SSH keys configured", ui.Dim("—"))
		ui.Print("      Generate: %s", ui.Cyan("gcm ssh generate <profile>"))
	}

	// ─── Issues / Suggestions ───
	if len(issues) > 0 {
		ui.Blank()
		ui.Divider()
		ui.Blank()
		ui.Print("  %s %s", ui.Bold("Issues"), ui.Red(fmt.Sprintf("(%d)", len(issues))))
		for _, issue := range issues {
			ui.Print("    %s %s", ui.Red(ui.IconArrow), issue)
		}
	}

	// ─── Quick Tips (if new user) ───
	if len(profiles) == 0 {
		ui.Blank()
		ui.Divider()
		ui.Blank()
		ui.Print("  %s", ui.Bold("Quick Start"))
		ui.Print("    Run %s for a guided setup wizard", ui.Cyan("gcm setup"))
	}

	ui.Blank()
	return nil
}

func statusVerifyConcurrency() int {
	if ctr == nil || ctr.Config == nil || !ctr.Config.Advanced.ParallelOperations {
		return 1
	}
	return 4
}
