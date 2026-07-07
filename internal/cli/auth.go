package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_auth "github.com/justjundana/git-config-manager/internal/auth"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

type authStatusOptions struct {
	provider string
	json     bool
	verbose  bool
}

type authAdoptOptions struct {
	provider string
	yes      bool
	dryRun   bool
}

type authLogoutOptions struct {
	provider string
	scope    string
	yes      bool
	dryRun   bool
	json     bool
}

type authRepairOptions struct {
	provider string
	yes      bool
	dryRun   bool
	json     bool
}

type authDoctorReport struct {
	GeneratedAt string                    `json:"generated_at"`
	Statuses    []_auth.ProfileAuthStatus `json:"statuses"`
	Findings    []authDoctorFinding       `json:"findings,omitempty"`
	IssueCount  int                       `json:"issue_count"`
}

type authDoctorFinding struct {
	Profile  string `json:"profile"`
	Provider string `json:"provider"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

var authManagerFactory = defaultAuthManager

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect and manage provider authentication ownership",
		Long: `Inspect authentication as a source-aware state instead of assuming every
working Git credential is owned by GCM. These commands distinguish GCM-managed
tokens from external Git credentials such as Keychain, Git Credential Manager,
GitHub CLI, and libsecret.`,
	}

	cmd.AddCommand(newAuthStatusCmd())
	cmd.AddCommand(newAuthInspectCmd())
	cmd.AddCommand(newAuthAdoptCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthDoctorCmd())
	cmd.AddCommand(newAuthRepairCmd())
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	opts := authStatusOptions{}
	cmd := &cobra.Command{
		Use:   "status [profile]",
		Short: "Show source-aware auth status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := ""
			if len(args) == 1 {
				profileName = args[0]
			}
			return runAuthStatus(cmd.Context(), profileName, opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to inspect (github, gitlab)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print machine-readable JSON")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Show credential helper and finding details")
	return cmd
}

func newAuthInspectCmd() *cobra.Command {
	opts := authStatusOptions{verbose: true}
	cmd := &cobra.Command{
		Use:   "inspect <profile>",
		Short: "Inspect auth sources for one profile",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthInspect(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to inspect (required when profile has no provider)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print machine-readable JSON")
	return cmd
}

func newAuthAdoptCmd() *cobra.Command {
	opts := authAdoptOptions{}
	cmd := &cobra.Command{
		Use:   "adopt <profile>",
		Short: "Adopt an external Git credential into GCM storage",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthAdopt(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to adopt (required when profile has no provider)")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Confirm adoption without prompting")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would be adopted without writing")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	opts := authLogoutOptions{scope: "gcm"}
	cmd := &cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove GCM-owned auth safely",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to log out (github, gitlab)")
	cmd.Flags().StringVar(&opts.scope, "scope", "gcm", "Credential scope to remove: gcm, external, all")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Confirm external credential deletion without prompting")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would be deleted without writing")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print post-logout status as JSON")
	return cmd
}

func newAuthDoctorCmd() *cobra.Command {
	opts := authStatusOptions{}
	cmd := &cobra.Command{
		Use:   "doctor [profile]",
		Short: "Diagnose auth ownership and helper issues",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := ""
			if len(args) == 1 {
				profileName = args[0]
			}
			return runAuthDoctor(cmd.Context(), profileName, opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to diagnose (github, gitlab)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print machine-readable JSON")
	return cmd
}

func newAuthRepairCmd() *cobra.Command {
	opts := authRepairOptions{}
	cmd := &cobra.Command{
		Use:   "repair [profile]",
		Short: "Repair safe local auth configuration issues",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := ""
			if len(args) == 1 {
				profileName = args[0]
			}
			return runAuthRepair(cmd.Context(), profileName, opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to repair (github, gitlab)")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Apply repairs without prompting")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show repairs without applying them")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print machine-readable JSON")
	return cmd
}

func runAuthStatus(ctx context.Context, profileName string, opts authStatusOptions) error {
	statuses, err := resolveAuthStatuses(ctx, profileName, opts.provider, true)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(statuses)
	}
	if profileName != "" && opts.verbose {
		printAuthInspect(statuses[0])
		return nil
	}
	_ui.Header("%s Authentication Status", _ui.IconKey)
	_ui.Blank()
	printAuthStatusTable(statuses)
	if opts.verbose {
		printAuthFindings(statuses)
	}
	return nil
}

func runAuthInspect(ctx context.Context, profileName string, opts authStatusOptions) error {
	statuses, err := resolveAuthStatuses(ctx, profileName, opts.provider, true)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(statuses[0])
	}
	printAuthInspect(statuses[0])
	return nil
}

func runAuthAdopt(ctx context.Context, profileName string, opts authAdoptOptions) error {
	p, def, err := resolveAuthTarget(profileName, opts.provider, true)
	if err != nil {
		return err
	}
	status, err := resolveAuthStatus(ctx, profileName, p, def, true)
	if err != nil {
		return err
	}
	external := status.ExternalCredential
	if !external.Present || external.Secret == "" {
		return fmt.Errorf("no exportable external credential found for %q on %s", profileName, def.DisplayName)
	}
	if external.Source == _auth.SourceGCMStore {
		return fmt.Errorf("the credential Git returned is already supplied by GCM; run: gcm auth inspect %s --provider %s", profileName, def.ID)
	}
	if external.State != _auth.StateAuthenticatedExternal {
		return fmt.Errorf("external credential could not be verified: %s", firstNonEmptyString(external.Error, "unknown verification error"))
	}

	if status.GCMCredential.Present && status.GCMCredential.Secret != external.Secret && !opts.yes && !opts.dryRun {
		_ui.Warning("Profile %q already has a different GCM-managed token.", profileName)
		ok, askErr := _ui.AskConfirm("Replace the GCM-managed token with the external credential?", false)
		if askErr != nil || !ok {
			_ui.Info("Cancelled.")
			return askErr
		}
	}

	username := firstNonEmptyString(external.Username, status.Username)
	if username == "" {
		return fmt.Errorf("external credential did not resolve to a provider username")
	}

	if opts.dryRun {
		_ui.Header("Auth Adopt Dry Run")
		_ui.Detail("Profile", profileName)
		_ui.Detail("Provider", def.DisplayName)
		_ui.Detail("External Source", string(external.Source))
		_ui.Detail("Username", username)
		_ui.Print("No files or credentials were changed.")
		return nil
	}

	tokenSet := _provider.TokenSet{AccessToken: external.Secret, AuthMethod: _provider.AuthMethodPAT, TokenType: "pat"}
	transitionOpts := providerTransitionOptions{AllowPrompt: true, AutoConfirm: opts.yes}
	ok, transitionErr := applyProfileProviderTransitionWithOptions(ctx, profileName, p, def, username, _provider.AuthMethodPAT, transitionOpts, func() error {
		return saveProviderToken(profileName, def, p, tokenSet)
	})
	if transitionErr != nil {
		ctr.AuditLogger.Log(_audit.ActionProviderLogin, profileName, map[string]string{"provider": string(def.ID), "method": "adopt"}, transitionErr)
		return transitionErr
	}
	if !ok {
		_ui.Info("Adoption cancelled")
		return nil
	}
	if err := ctr.ProfileManager.Update(p); err != nil {
		return fmt.Errorf("external credential was stored, but profile metadata could not be updated: %w", err)
	}
	if isActiveProfile(profileName) {
		configureGitCredentialsForProvider(profileName, p, def, tokenSet)
	}
	ctr.AuditLogger.Log(_audit.ActionProviderLogin, profileName, map[string]string{"provider": string(def.ID), "user": username, "method": "adopt"}, nil)
	_ui.Success("Adopted %s credential for %q as %s", def.DisplayName, profileName, _ui.Bold(username))
	return nil
}

func runAuthLogout(ctx context.Context, profileName string, opts authLogoutOptions) error {
	scope := strings.ToLower(strings.TrimSpace(opts.scope))
	if scope != "gcm" && scope != "external" && scope != "all" {
		return fmt.Errorf("invalid --scope %q; expected gcm, external, or all", opts.scope)
	}
	p, def, err := resolveAuthTarget(profileName, opts.provider, false)
	if err != nil {
		return err
	}
	status, err := resolveAuthStatus(ctx, profileName, p, def, true)
	if err != nil {
		return err
	}

	if opts.dryRun {
		printAuthLogoutPlan(profileName, def, scope, status)
		return nil
	}
	externalOwned := nonGCMExternalCredentialPresent(status.ExternalCredential)
	if (scope == "external" || scope == "all") && externalOwned && !opts.yes {
		_ui.Warning("This can delete credentials owned by another tool from Git's credential chain.")
		ok, askErr := _ui.AskConfirm(fmt.Sprintf("Delete %s external credential for %q?", def.DisplayName, profileName), false)
		if askErr != nil || !ok {
			_ui.Info("Cancelled.")
			return askErr
		}
	}

	if scope == "gcm" || scope == "all" {
		if status.GCMCredential.Present {
			if err := deleteProviderToken(profileName, def, p); err != nil {
				return fmt.Errorf("could not remove GCM-managed token for %q: %w", profileName, err)
			}
			if def.ID == _provider.GitHubID {
				_ = ctr.GitHubClient.DeleteToken(profileName)
			}
			ctr.AuditLogger.Log(_audit.ActionProviderLogout, profileName, map[string]string{"provider": string(def.ID), "scope": "gcm"}, nil)
			_ui.Success("GCM-managed %s token removed for %q", def.DisplayName, profileName)
		} else {
			_ui.Info("No GCM-managed %s token was found for %q.", def.DisplayName, profileName)
		}
		if isActiveProfile(profileName) {
			_ = ctr.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "")
			_ = authManager().ExternalInspector.RejectGitCredential(ctx, def, providerAccountForProfile(p, def.ID).Username)
		}
	}

	if scope == "external" || scope == "all" {
		if externalOwned {
			username := firstNonEmptyString(status.ExternalCredential.Username, providerAccountForProfile(p, def.ID).Username)
			if err := authManager().ExternalInspector.RejectGitCredential(ctx, def, username); err != nil {
				return err
			}
			ctr.AuditLogger.Log(_audit.ActionProviderLogout, profileName, map[string]string{"provider": string(def.ID), "scope": "external"}, nil)
			_ui.Success("External Git credential rejected for %s", def.CredentialServer())
		} else {
			_ui.Info("No external credential owned by another tool was found.")
		}
	}

	updated, err := resolveAuthStatus(ctx, profileName, p, def, true)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(updated)
	}
	_ui.Blank()
	printAuthInspect(updated)
	return nil
}

func runAuthDoctor(ctx context.Context, profileName string, opts authStatusOptions) error {
	statuses, err := resolveAuthStatuses(ctx, profileName, opts.provider, true)
	if err != nil {
		return err
	}
	report := buildAuthDoctorReport(statuses)
	if opts.json {
		return writeJSON(report)
	}
	_ui.Header("%s Auth Doctor", _ui.IconDoctor)
	_ui.Blank()
	if report.IssueCount == 0 {
		_ui.Success("No auth ownership issues found")
		return nil
	}
	for _, finding := range report.Findings {
		_ui.Print("  %s %s/%s: %s", authSeverityIcon(finding.Severity), finding.Profile, finding.Provider, finding.Message)
		if finding.Action != "" {
			_ui.Print("    %s %s", _ui.Dim("action:"), _ui.Cyan(finding.Action))
		}
	}
	return nil
}

func runAuthRepair(ctx context.Context, profileName string, opts authRepairOptions) error {
	statuses, err := resolveAuthStatuses(ctx, profileName, opts.provider, true)
	if err != nil {
		return err
	}
	report := buildAuthDoctorReport(statuses)
	missingHelper := false
	for _, finding := range report.Findings {
		if finding.Code == "credential_helper_missing" {
			missingHelper = true
			break
		}
	}
	if opts.json && opts.dryRun {
		return writeJSON(report)
	}
	if !missingHelper {
		if opts.json {
			return writeJSON(report)
		}
		_ui.Success("No safe auth repairs are needed")
		return nil
	}
	if opts.dryRun {
		_ui.Header("Auth Repair Dry Run")
		_ui.Print("  Would register GCM as git credential helper for configured provider hosts.")
		return nil
	}
	if !opts.yes {
		ok, askErr := _ui.AskConfirm("Register GCM as git credential helper for configured provider hosts?", false)
		if askErr != nil || !ok {
			_ui.Info("Cancelled.")
			return askErr
		}
	}
	if err := RegisterCredentialHelper(); err != nil {
		return err
	}
	_ui.Success("GCM credential helper registered for configured provider hosts")
	if opts.json {
		updated, updateErr := resolveAuthStatuses(ctx, profileName, opts.provider, true)
		if updateErr != nil {
			return updateErr
		}
		return writeJSON(buildAuthDoctorReport(updated))
	}
	return nil
}

func runProviderSpecificAuthStatus(ctx context.Context, def _provider.Definition) error {
	profiles, err := ctr.ProfileManager.List()
	if err != nil {
		return err
	}
	var statuses []_auth.ProfileAuthStatus
	for _, p := range profiles {
		if !profileUsesProvider(p, def.ID) {
			continue
		}
		status, resolveErr := resolveAuthStatus(ctx, p.Name, p, def, true)
		if resolveErr != nil {
			return resolveErr
		}
		statuses = append(statuses, status)
	}
	if len(statuses) == 0 {
		_ui.Info("No %s-scoped profiles found", def.DisplayName)
		return nil
	}
	_ui.Header("%s %s Authentication Status", _ui.IconGlobe, def.DisplayName)
	_ui.Blank()
	printAuthStatusTable(statuses)
	return nil
}

func resolveAuthStatuses(ctx context.Context, profileName, providerName string, allowProviderFlagForMissing bool) ([]_auth.ProfileAuthStatus, error) {
	if profileName != "" {
		p, def, err := resolveAuthTarget(profileName, providerName, allowProviderFlagForMissing)
		if err != nil {
			return nil, err
		}
		status, err := resolveAuthStatus(ctx, profileName, p, def, true)
		if err != nil {
			return nil, err
		}
		return []_auth.ProfileAuthStatus{status}, nil
	}

	profiles, err := ctr.ProfileManager.List()
	if err != nil {
		return nil, err
	}
	var requestedDef _provider.Definition
	if providerName != "" {
		requestedDef, err = providerDefinitionFromSelection(providerName)
		if err != nil {
			return nil, err
		}
	}
	statuses := make([]_auth.ProfileAuthStatus, 0, len(profiles))
	for _, p := range profiles {
		def := requestedDef
		if providerName == "" {
			if profileHasMultipleProviders(p) {
				statuses = append(statuses, unresolvedAuthStatus(p.Name, "multiple providers", _auth.StateConflicted))
				continue
			}
			profileDef, ok := profileProviderDefinition(p, _provider.CapabilityCredentialHelper)
			if !ok {
				statuses = append(statuses, unresolvedAuthStatus(p.Name, "no provider", _auth.StateUnauthenticated))
				continue
			}
			def = profileDef
		} else if profileHasMultipleProviders(p) || (profileUsesAnyProvider(p) && !profileUsesProvider(p, def.ID)) {
			statuses = append(statuses, unresolvedAuthStatus(p.Name, fmt.Sprintf("configured for another provider, not %s", def.DisplayName), _auth.StateConflicted))
			continue
		}
		status, resolveErr := resolveAuthStatus(ctx, p.Name, p, def, true)
		if resolveErr != nil {
			return nil, resolveErr
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func resolveAuthTarget(profileName, providerName string, allowMissingProvider bool) (*_profile.Profile, _provider.Definition, error) {
	p, err := ctr.ProfileManager.Get(profileName)
	if err != nil {
		return nil, _provider.Definition{}, fmt.Errorf("profile %q not found", profileName)
	}
	if profileHasMultipleProviders(p) {
		return nil, _provider.Definition{}, fmt.Errorf("profile %q has multiple providers configured; choose exactly one with: gcm profile edit %s -i", profileName, profileName)
	}
	if providerName != "" {
		def, err := providerDefinitionFromSelection(providerName)
		if err != nil {
			return nil, _provider.Definition{}, err
		}
		if profileUsesAnyProvider(p) && !profileUsesProvider(p, def.ID) {
			return nil, _provider.Definition{}, fmt.Errorf("profile %q is configured for another provider; switch providers explicitly with: gcm switch-provider %s %s", profileName, profileName, def.ID)
		}
		return p, def, nil
	}
	def, ok := profileProviderDefinition(p, _provider.CapabilityCredentialHelper)
	if !ok {
		if allowMissingProvider {
			return nil, _provider.Definition{}, fmt.Errorf("profile %q has no provider; pass --provider github or --provider gitlab", profileName)
		}
		return nil, _provider.Definition{}, fmt.Errorf("profile %q has no provider configured; run: gcm connect %s --provider <github|gitlab>", profileName, profileName)
	}
	return p, def, nil
}

func providerDefinitionFromSelection(value string) (_provider.Definition, error) {
	id := normalizeProviderSelection(value)
	def, ok := ctr.ProviderRegistry.Get(id)
	if !ok {
		return _provider.Definition{}, fmt.Errorf("provider %q is not configured", value)
	}
	if !def.Capabilities.Has(_provider.CapabilityCredentialHelper) {
		return _provider.Definition{}, fmt.Errorf("provider %q does not support Git credential authentication yet", value)
	}
	return def, nil
}

func resolveAuthStatus(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition, verify bool) (_auth.ProfileAuthStatus, error) {
	return authManager().Resolve(ctx, _auth.ResolveRequest{
		ProfileName:     profileName,
		Profile:         p,
		Provider:        def,
		Verify:          verify,
		InspectExternal: true,
		InspectSSH:      true,
	})
}

func authManager() *_auth.Manager {
	return authManagerFactory()
}

func defaultAuthManager() *_auth.Manager {
	return _auth.NewManager(ctr.TokenStore, ctr.ProviderClient)
}

func unresolvedAuthStatus(profileName, reason string, state _auth.State) _auth.ProfileAuthStatus {
	return _auth.ProfileAuthStatus{
		GeneratedAt:  time.Now().UTC(),
		Profile:      profileName,
		State:        state,
		Ownership:    _auth.OwnershipUnknown,
		Findings:     []_auth.Finding{{Code: "profile_provider_unresolved", Severity: "warning", Message: reason}},
		Capabilities: []_auth.CapabilityStatus{{Name: "https_git", State: state, Source: _auth.SourceUnknown}},
	}
}

func profileUsesAnyProvider(p *_profile.Profile) bool {
	_, ok := profileProviderID(p)
	return ok
}

func printAuthStatusTable(statuses []_auth.ProfileAuthStatus) {
	headers := []string{"Profile", "Provider", "State", "Owner", "Source", "Username", "Findings"}
	rows := make([][]string, 0, len(statuses))
	for _, status := range statuses {
		rows = append(rows, []string{
			status.Profile,
			firstNonEmptyString(status.ProviderName, string(status.Provider), "-"),
			renderAuthState(status.State),
			string(status.Ownership),
			string(effectiveAuthSource(status)),
			firstNonEmptyString(status.Username, "-"),
			fmt.Sprintf("%d", len(status.Findings)),
		})
	}
	if len(rows) == 0 {
		_ui.Info("No profiles found")
		return
	}
	_ui.SimpleTable(headers, rows)
}

func printAuthInspect(status _auth.ProfileAuthStatus) {
	_ui.Header("%s Auth Inspect: %s", _ui.IconDoctor, status.Profile)
	_ui.Blank()
	_ui.Detail("Provider", firstNonEmptyString(status.ProviderName, string(status.Provider), "-"))
	_ui.Detail("Host", firstNonEmptyString(status.Host, "-"))
	if status.GitCredentialUsername != "" {
		_ui.Detail("Git Credential Username", status.GitCredentialUsername)
	}
	_ui.Detail("State", string(status.State))
	_ui.Detail("Ownership", string(status.Ownership))
	_ui.Detail("Username", firstNonEmptyString(status.Username, "-"))
	_ui.Blank()
	printCredentialDetail("GCM", status.GCMCredential)
	printCredentialDetail(gitCredentialDetailLabel(status.ExternalCredential), status.ExternalCredential)
	printCredentialDetail("SSH", status.SSHCredential)
	if len(status.CredentialHelpers) > 0 {
		_ui.Blank()
		_ui.Print("  %s", _ui.Bold("Credential Helpers"))
		for _, helper := range status.CredentialHelpers {
			value := helper.Value
			if value == "" {
				value = "<reset>"
			}
			_ui.Print("    %s %-6s %s", authSourceLabel(helper.Source), helper.Scope, value)
		}
	}
	printAuthFindings([]_auth.ProfileAuthStatus{status})
}

func gitCredentialDetailLabel(credential _auth.CredentialStatus) string {
	if credential.Source == _auth.SourceGCMStore {
		return "Git via GCM Helper"
	}
	return "External Git"
}

func printCredentialDetail(label string, credential _auth.CredentialStatus) {
	state := string(credential.State)
	if credential.Present {
		state = fmt.Sprintf("%s via %s", credential.State, credential.Source)
	}
	_ui.Detail(label, state)
	if credential.Present {
		if credential.Username != "" {
			_ui.Print("    username: %s", credential.Username)
		}
		if credential.AuthMethod != "" {
			_ui.Print("    method:   %s", credential.AuthMethod)
		}
		_ui.Print("    verified: %t", credential.Verified)
	}
	if credential.Error != "" {
		_ui.Print("    error:    %s", credential.Error)
	}
}

func printAuthFindings(statuses []_auth.ProfileAuthStatus) {
	printedHeader := false
	for _, status := range statuses {
		for _, finding := range status.Findings {
			if !printedHeader {
				_ui.Blank()
				_ui.Print("  %s", _ui.Bold("Findings"))
				printedHeader = true
			}
			_ui.Print("    %s %s: %s", authSeverityIcon(finding.Severity), status.Profile, finding.Message)
			if finding.Action != "" {
				_ui.Print("      %s", _ui.Cyan(finding.Action))
			}
		}
	}
}

func printAuthLogoutPlan(profileName string, def _provider.Definition, scope string, status _auth.ProfileAuthStatus) {
	_ui.Header("Auth Logout Dry Run")
	_ui.Detail("Profile", profileName)
	_ui.Detail("Provider", def.DisplayName)
	_ui.Detail("Scope", scope)
	if scope == "gcm" || scope == "all" {
		_ui.Detail("GCM Token", presentLabel(status.GCMCredential.Present))
	}
	if scope == "external" || scope == "all" {
		_ui.Detail("External Credential", fmt.Sprintf("%s (%s)", presentLabel(nonGCMExternalCredentialPresent(status.ExternalCredential)), status.ExternalCredential.Source))
	}
	_ui.Print("No files or credentials were changed.")
}

func buildAuthDoctorReport(statuses []_auth.ProfileAuthStatus) authDoctorReport {
	report := authDoctorReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Statuses: statuses}
	for _, status := range statuses {
		for _, finding := range status.Findings {
			if !isActionableAuthDoctorFinding(status, finding) {
				continue
			}
			report.Findings = append(report.Findings, authDoctorFinding{
				Profile:  status.Profile,
				Provider: string(status.Provider),
				Code:     finding.Code,
				Severity: finding.Severity,
				Message:  finding.Message,
				Action:   finding.Action,
			})
		}
	}
	report.IssueCount = len(report.Findings)
	return report
}

func isActionableAuthDoctorFinding(status _auth.ProfileAuthStatus, finding _auth.Finding) bool {
	return !(finding.Code == "profile_provider_unresolved" && status.Provider == "")
}

func effectiveAuthSource(status _auth.ProfileAuthStatus) _auth.CredentialSource {
	switch status.Ownership {
	case _auth.OwnershipGCM:
		return _auth.SourceGCMStore
	case _auth.OwnershipExternal:
		return status.ExternalCredential.Source
	case _auth.OwnershipMixed:
		return _auth.SourceGitCredential
	default:
		if status.SSHCredential.Present {
			return status.SSHCredential.Source
		}
		return _auth.SourceUnknown
	}
}

func renderAuthState(state _auth.State) string {
	switch state {
	case _auth.StateAuthenticatedGCM, _auth.StateAuthenticatedExternal:
		return _ui.Green(string(state))
	case _auth.StateAuthenticatedMixed, _auth.StatePartial:
		return _ui.Yellow(string(state))
	case _auth.StateUnauthenticated:
		return _ui.Dim(string(state))
	default:
		return _ui.Red(string(state))
	}
}

func authSeverityIcon(severity string) string {
	switch severity {
	case "error":
		return _ui.Red(_ui.IconError)
	case "warning":
		return _ui.Yellow(_ui.IconWarning)
	default:
		return _ui.Dim(_ui.IconInfo)
	}
}

func authSourceLabel(source _auth.CredentialSource) string {
	if source == _auth.SourceGCMStore {
		return _ui.Green(string(source))
	}
	if source == _auth.SourceUnknown {
		return _ui.Dim(string(source))
	}
	return _ui.Yellow(string(source))
}

func presentLabel(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

func nonGCMExternalCredentialPresent(credential _auth.CredentialStatus) bool {
	return credential.Present && credential.Source != _auth.SourceGCMStore
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
