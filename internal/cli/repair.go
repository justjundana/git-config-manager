package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

type repairOptions struct {
	fix  bool
	yes  bool
	json bool
}

type repairReport struct {
	GeneratedAt  string        `json:"generated_at"`
	Issues       []repairIssue `json:"issues"`
	IssueCount   int           `json:"issue_count"`
	FixableCount int           `json:"fixable_count"`
	FixedCount   int           `json:"fixed_count"`
	ManualCount  int           `json:"manual_count"`
}

type repairIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Profile  string `json:"profile,omitempty"`
	Provider string `json:"provider,omitempty"`
	Message  string `json:"message"`
	Action   string `json:"action"`
	Fixable  bool   `json:"fixable"`
	Fixed    bool   `json:"fixed,omitempty"`
	Error    string `json:"error,omitempty"`
}

func newRepairCmd() *cobra.Command {
	opts := repairOptions{}
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Inspect and repair GCM provider/profile state",
		Long: `Inspect GCM's local provider/profile state and optionally repair safe issues.

Repair checks provider-scoped profile consistency, credential helper
registration, legacy GitHub token migration, and provider-aware SSH key names.
By default it only reports. Use --fix to apply safe local repairs.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runRepair(opts)
		},
	}
	cmd.Flags().BoolVar(&opts.fix, "fix", false, "Apply safe local repairs")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Skip confirmation when used with --fix")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print a machine-readable JSON report")
	return cmd
}

func runRepair(opts repairOptions) error {
	if opts.fix && opts.json && !opts.yes {
		return fmt.Errorf("--fix with --json requires --yes to avoid interactive prompts")
	}

	plan, err := buildRepairReport(false)
	if err != nil {
		return err
	}

	if !opts.fix {
		return outputRepairReport(plan, opts.json)
	}

	if plan.FixableCount == 0 {
		return outputRepairReport(plan, opts.json)
	}

	if !opts.yes {
		printRepairReport(plan)
		ui.Blank()
		ok, askErr := ui.AskConfirm(fmt.Sprintf("Apply %d safe repair action(s)?", plan.FixableCount), false)
		if askErr != nil || !ok {
			return askErr
		}
	}

	applied, err := buildRepairReport(true)
	if err != nil {
		return err
	}
	return outputRepairReport(applied, opts.json)
}

func buildRepairReport(apply bool) (repairReport, error) {
	report := repairReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}

	appendIssue := func(issue repairIssue) {
		report.Issues = append(report.Issues, issue)
		report.IssueCount++
		if issue.Fixable {
			report.FixableCount++
		}
		if issue.Fixed {
			report.FixedCount++
		}
		if !issue.Fixable || issue.Error != "" {
			report.ManualCount++
		}
	}

	appendCredentialHelperIssues(&report, appendIssue, apply)

	profiles, err := ctr.ProfileManager.List()
	if err != nil {
		return report, err
	}

	for _, p := range profiles {
		appendProfileRepairIssues(&report, appendIssue, p, apply)
	}

	return report, nil
}

func appendCredentialHelperIssues(_ *repairReport, appendIssue func(repairIssue), apply bool) {
	defs := providerDefinitionsWithCapability(providerpkg.CapabilityCredentialHelper)
	missing := make([]providerpkg.Definition, 0)
	for _, def := range defs {
		server := def.CredentialServer()
		if server != "" && !IsCredentialHelperConfiguredFor(server) {
			missing = append(missing, def)
		}
	}
	if len(missing) == 0 {
		return
	}

	var registerErr error
	if apply {
		registerErr = RegisterCredentialHelper()
	}

	for _, def := range missing {
		server := def.CredentialServer()
		issue := repairIssue{
			Code:     "credential_helper_missing",
			Severity: "warning",
			Provider: string(def.ID),
			Message:  fmt.Sprintf("GCM is not registered as git credential helper for %s", server),
			Action:   fmt.Sprintf("Register GCM credential helper for %s", server),
			Fixable:  true,
		}
		if apply {
			if registerErr != nil {
				issue.Error = registerErr.Error()
			} else if IsCredentialHelperConfiguredFor(server) {
				issue.Fixed = true
			} else {
				issue.Error = "credential helper was not registered after repair"
			}
		}
		appendIssue(issue)
	}
}

func appendProfileRepairIssues(_ *repairReport, appendIssue func(repairIssue), p *profile.Profile, apply bool) {
	if p == nil {
		return
	}

	if profileHasMultipleProviders(p) {
		if fixed, fixErr := repairStaleLegacyGitHubBlock(p, apply); fixed || fixErr != nil {
			issue := repairIssue{
				Code:     "stale_legacy_github_block",
				Severity: "warning",
				Profile:  p.Name,
				Message:  fmt.Sprintf("Profile %q has stale legacy GitHub metadata next to a non-GitHub provider", p.Name),
				Action:   "Remove the stale legacy GitHub profile block",
				Fixable:  true,
				Fixed:    fixed,
			}
			if fixErr != nil {
				issue.Error = fixErr.Error()
			}
			appendIssue(issue)
			return
		}

		appendIssue(repairIssue{
			Code:     "multiple_providers",
			Severity: "error",
			Profile:  p.Name,
			Message:  fmt.Sprintf("Profile %q has more than one provider configured", p.Name),
			Action:   fmt.Sprintf("Choose exactly one provider with: gcm profile edit %s -i", p.Name),
			Fixable:  false,
		})
		return
	}

	def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper)
	if !ok {
		appendIssue(repairIssue{
			Code:     "provider_missing",
			Severity: "warning",
			Profile:  p.Name,
			Message:  fmt.Sprintf("Profile %q has no provider selected", p.Name),
			Action:   fmt.Sprintf("Connect a provider with: gcm connect %s --provider <github|gitlab>", p.Name),
			Fixable:  false,
		})
		return
	}

	if target, needsRename := providerSSHKeyMigrationTarget(p.Name, p); needsRename {
		issue := repairIssue{
			Code:     "ssh_key_name_legacy",
			Severity: "warning",
			Profile:  p.Name,
			Provider: string(def.ID),
			Message:  fmt.Sprintf("Profile %q SSH key path does not include provider marker", p.Name),
			Action:   fmt.Sprintf("Rename SSH key to %s", target),
			Fixable:  true,
		}
		if apply {
			fixed, err := migrateProfileSSHKeyPathToProvider(p.Name, p)
			issue.Fixed = fixed && err == nil
			if err != nil {
				issue.Error = err.Error()
			}
		}
		appendIssue(issue)
	}

	if def.ID == providerpkg.GitHubID {
		appendLegacyGitHubTokenIssue(appendIssue, p, def, apply)
	}
}

func repairStaleLegacyGitHubBlock(p *profile.Profile, apply bool) (bool, error) {
	if p == nil || p.GitHub == nil || len(p.Providers) != 1 {
		return false, nil
	}
	if _, hasGitHubProvider := p.Providers[string(providerpkg.GitHubID)]; hasGitHubProvider {
		return false, nil
	}
	if !apply {
		return true, nil
	}
	p.GitHub = nil
	if err := ctr.ProfileManager.Update(p); err != nil {
		return false, err
	}
	return true, nil
}

func appendLegacyGitHubTokenIssue(appendIssue func(repairIssue), p *profile.Profile, def providerpkg.Definition, apply bool) {
	legacyToken, err := ctr.GitHubClient.LoadToken(p.Name)
	if err != nil || legacyToken == "" {
		return
	}

	issue := repairIssue{
		Code:     "legacy_github_token",
		Severity: "warning",
		Profile:  p.Name,
		Provider: string(def.ID),
		Message:  fmt.Sprintf("Profile %q still has a legacy GitHub token entry", p.Name),
		Action:   "Migrate token to provider-aware storage and remove the legacy token entry",
		Fixable:  true,
	}
	if apply {
		token := providerpkg.TokenSet{
			AccessToken: legacyToken,
			AuthMethod:  providerpkg.AuthMethodLegacy,
			CreatedAt:   time.Now().UTC(),
		}
		if err := saveProviderToken(p.Name, def, p, token); err != nil {
			issue.Error = err.Error()
		} else if err := ctr.GitHubClient.DeleteToken(p.Name); err != nil {
			issue.Error = err.Error()
		} else {
			issue.Fixed = true
		}
	}
	appendIssue(issue)
}

func outputRepairReport(report repairReport, jsonOutput bool) error {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printRepairReport(report)
	return nil
}

func printRepairReport(report repairReport) {
	ui.Header("%s GCM Repair", ui.IconDoctor)
	ui.Blank()

	if report.IssueCount == 0 {
		ui.Success("No repair issues found")
		return
	}

	ui.Print("  %s %d issue(s), %d fixable, %d fixed, %d manual", ui.Bold("Summary"), report.IssueCount, report.FixableCount, report.FixedCount, report.ManualCount)
	ui.Blank()

	for _, issue := range report.Issues {
		icon := ui.Yellow(ui.IconWarning)
		if issue.Severity == "error" {
			icon = ui.Red(ui.IconError)
		}
		if issue.Fixed {
			icon = ui.Green(ui.IconSuccess)
		}

		scope := issue.Code
		if issue.Profile != "" {
			scope = issue.Profile + ": " + scope
		}
		ui.Print("  %s %s", icon, ui.Bold(scope))
		ui.Print("    %s", issue.Message)
		ui.Print("    %s", ui.Dim(issue.Action))
		if issue.Error != "" {
			ui.Print("    %s %s", ui.Red("repair failed:"), issue.Error)
		}
	}
}
