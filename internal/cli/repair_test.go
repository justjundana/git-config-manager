package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/container"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/logger"
	"github.com/justjundana/git-config-manager/pkg/ui"
)

func newRepairTestContainer(t *testing.T) *container.Container {
	t.Helper()

	tmp := t.TempDir()

	// Isolate git environment to prevent tests from touching real config
	t.Setenv("HOME", tmp)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, ".gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("GNUPGHOME", filepath.Join(tmp, "gpg"))
	t.Setenv("SSH_AUTH_SOCK", "")

	cfg := config.DefaultConfig()
	cfg.ProfilesDir = filepath.Join(tmp, "profiles")
	cfg.TemplatesDir = filepath.Join(tmp, "templates")
	cfg.CacheDir = filepath.Join(tmp, "cache")
	cfg.SSHDir = filepath.Join(tmp, "ssh")
	cfg.GPGHome = filepath.Join(tmp, "gpg")
	cfg.Security.UseKeychain = false
	cfg.Security.EncryptTokens = false
	cfg.Security.MasterPassword = false
	cfg.Security.AllowPlaintextTokens = true

	for _, dir := range []string{cfg.ProfilesDir, cfg.TemplatesDir, cfg.CacheDir, cfg.SSHDir, cfg.GPGHome} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	return container.New(cfg, logger.New(logger.LevelError, io.Discard))
}

func withRepairTestContainer(t *testing.T) *container.Container {
	t.Helper()
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	return ctr
}

func repairTestProfile(name string) *profile.Profile {
	return &profile.Profile{
		Name: name,
		Git:  profile.GitConfig{User: profile.GitUser{Name: "Test User", Email: name + "@example.test"}},
	}
}

func TestAppendProfileRepairIssuesReportsProviderMissing(t *testing.T) {
	withRepairTestContainer(t)
	var issues []repairIssue

	appendProfileRepairIssues(nil, func(issue repairIssue) {
		issues = append(issues, issue)
	}, repairTestProfile("work"), false)

	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	issue := issues[0]
	if issue.Code != "provider_missing" || issue.Fixable {
		t.Fatalf("issue = %+v, want non-fixable provider_missing", issue)
	}
}

func TestRepairStaleLegacyGitHubBlockReportAndFix(t *testing.T) {
	ctr := withRepairTestContainer(t)
	p := repairTestProfile("work")
	p.GitHub = &profile.GitHubConfig{Username: "stale-gh"}
	p.Providers = map[string]profile.ProviderAccountConfig{
		string(providerpkg.GitLabID): {Username: "lab"},
	}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	fixed, err := repairStaleLegacyGitHubBlock(p, false)
	if err != nil || !fixed {
		t.Fatalf("dry-run stale repair = %v, %v; want true, nil", fixed, err)
	}
	if p.GitHub == nil {
		t.Fatal("dry-run should not mutate legacy GitHub block")
	}

	fixed, err = repairStaleLegacyGitHubBlock(p, true)
	if err != nil || !fixed {
		t.Fatalf("apply stale repair = %v, %v; want true, nil", fixed, err)
	}
	updated, err := ctr.ProfileManager.Get("work")
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if updated.GitHub != nil {
		t.Fatalf("legacy GitHub block should be removed: %+v", updated.GitHub)
	}
	if !profile.UsesProvider(updated, providerpkg.GitLabID) {
		t.Fatal("GitLab provider should remain selected")
	}
}

func TestAppendProfileRepairIssuesReportsMultipleProviders(t *testing.T) {
	withRepairTestContainer(t)
	p := repairTestProfile("mixed")
	p.Providers = map[string]profile.ProviderAccountConfig{
		string(providerpkg.GitHubID): {Username: "gh"},
		string(providerpkg.GitLabID): {Username: "gl"},
	}
	var issues []repairIssue

	appendProfileRepairIssues(nil, func(issue repairIssue) {
		issues = append(issues, issue)
	}, p, false)

	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if issues[0].Code != "multiple_providers" || issues[0].Fixable {
		t.Fatalf("issue = %+v, want non-fixable multiple_providers", issues[0])
	}
}

func TestAppendProfileRepairIssuesReportsStaleLegacyGitHubBlock(t *testing.T) {
	withRepairTestContainer(t)
	p := repairTestProfile("work")
	p.GitHub = &profile.GitHubConfig{Username: "stale-gh"}
	p.Providers = map[string]profile.ProviderAccountConfig{
		string(providerpkg.GitLabID): {Username: "lab"},
	}
	var issues []repairIssue

	appendProfileRepairIssues(nil, func(issue repairIssue) {
		issues = append(issues, issue)
	}, p, false)

	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if issues[0].Code != "stale_legacy_github_block" || !issues[0].Fixable {
		t.Fatalf("issue = %+v, want fixable stale_legacy_github_block", issues[0])
	}
}

func TestAppendProfileRepairIssuesReportsLegacySSHKeyName(t *testing.T) {
	withRepairTestContainer(t)
	p := repairTestProfile("work")
	profile.SetProviderAccount(p, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	p.SSH = &profile.SSHConfig{KeyPath: filepath.Join(t.TempDir(), "id_ed25519_work"), KeyType: "ed25519"}
	var issues []repairIssue

	appendProfileRepairIssues(nil, func(issue repairIssue) {
		issues = append(issues, issue)
	}, p, false)

	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	issue := issues[0]
	if issue.Code != "ssh_key_name_legacy" || !issue.Fixable || issue.Provider != string(providerpkg.GitLabID) {
		t.Fatalf("issue = %+v, want GitLab ssh_key_name_legacy", issue)
	}
	if !strings.Contains(issue.Action, "id_ed25519_work_gitlab") {
		t.Fatalf("action = %q, want provider-scoped target", issue.Action)
	}
}

func TestAppendLegacyGitHubTokenIssueMigratesProviderAwareToken(t *testing.T) {
	ctr := withRepairTestContainer(t)
	p := repairTestProfile("work")
	profile.SetProviderAccount(p, providerpkg.GitHubID, "octo", providerpkg.AuthMethodPAT)
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := ctr.GitHubClient.SaveToken("work", "legacy-token"); err != nil {
		t.Fatalf("save legacy token: %v", err)
	}
	def, ok := ctr.ProviderRegistry.Get(providerpkg.GitHubID)
	if !ok {
		t.Fatal("GitHub provider missing")
	}

	var dryRun []repairIssue
	appendLegacyGitHubTokenIssue(func(issue repairIssue) {
		dryRun = append(dryRun, issue)
	}, p, def, false)
	if len(dryRun) != 1 || dryRun[0].Code != "legacy_github_token" || !dryRun[0].Fixable || dryRun[0].Fixed {
		t.Fatalf("dry-run issue = %+v", dryRun)
	}

	var applied []repairIssue
	appendLegacyGitHubTokenIssue(func(issue repairIssue) {
		applied = append(applied, issue)
	}, p, def, true)
	if len(applied) != 1 || !applied[0].Fixed || applied[0].Error != "" {
		t.Fatalf("applied issue = %+v", applied)
	}

	token, err := loadProviderToken("work", def, p)
	if err != nil {
		t.Fatalf("load provider-aware token: %v", err)
	}
	if token.AccessToken != "legacy-token" || token.AuthMethod != providerpkg.AuthMethodLegacy {
		t.Fatalf("migrated token = %+v", token)
	}
	if legacyToken, err := ctr.GitHubClient.LoadToken("work"); err == nil && legacyToken != "" {
		t.Fatalf("legacy token should be deleted, got %q", legacyToken)
	}
}

func TestRunRepairRejectsFixJSONWithoutYes(t *testing.T) {
	withRepairTestContainer(t)
	err := runRepair(repairOptions{fix: true, json: true})
	if err == nil {
		t.Fatal("expected --fix --json without --yes to fail")
	}
}

func TestBuildRepairReportReportsProfileIssuesWithoutCredentialHelperChecks(t *testing.T) {
	ctr := withRepairTestContainer(t)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"})
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"})
	if err := ctr.ProfileManager.Create(repairTestProfile("work")); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	report, err := buildRepairReport(false)
	if err != nil {
		t.Fatalf("buildRepairReport: %v", err)
	}
	if report.IssueCount != 1 || report.ManualCount != 1 {
		t.Fatalf("report counts = %+v", report)
	}
	if report.Issues[0].Code != "provider_missing" {
		t.Fatalf("issue = %+v, want provider_missing", report.Issues[0])
	}
}

func TestAppendCredentialHelperIssuesReportsMissingProviderHostReadOnly(t *testing.T) {
	ctr := withRepairTestContainer(t)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"})
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"})
	fakeProviderID := providerpkg.ProviderID("repair-test")
	fakeHost := fmt.Sprintf("repair-%s.invalid", strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-")))
	ctr.ProviderRegistry.Register(providerpkg.Definition{
		ID:           fakeProviderID,
		DisplayName:  "Repair Test",
		WebURL:       "https://" + fakeHost,
		GitHosts:     []string{fakeHost},
		Capabilities: providerpkg.CapabilitySet{providerpkg.CapabilityCredentialHelper: true},
	})
	var issues []repairIssue

	appendCredentialHelperIssues(nil, func(issue repairIssue) {
		issues = append(issues, issue)
	}, false)

	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(issues))
	}
	if issues[0].Code != "credential_helper_missing" || issues[0].Provider != string(fakeProviderID) || !issues[0].Fixable {
		t.Fatalf("issue = %+v", issues[0])
	}
}

func TestNewRepairCmdFlags(t *testing.T) {
	cmd := newRepairCmd()
	if cmd.Use != "repair" {
		t.Fatalf("Use = %q, want repair", cmd.Use)
	}
	for _, name := range []string{"fix", "yes", "json"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing --%s flag", name)
		}
	}
}

func TestOutputRepairReportJSON(t *testing.T) {
	report := repairReport{
		GeneratedAt:  "2026-05-25T00:00:00Z",
		Issues:       []repairIssue{{Code: "provider_missing", Severity: "warning", Profile: "work", Message: "missing", Action: "connect", Fixable: false}},
		IssueCount:   1,
		FixableCount: 0,
		ManualCount:  1,
	}
	output := captureStdout(t, func() {
		if err := outputRepairReport(report, true); err != nil {
			t.Fatalf("outputRepairReport: %v", err)
		}
	})

	var decoded repairReport
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("invalid JSON output %q: %v", output, err)
	}
	if decoded.IssueCount != 1 || decoded.Issues[0].Code != "provider_missing" {
		t.Fatalf("decoded report = %+v", decoded)
	}
}

func TestOutputRepairReportText(t *testing.T) {
	noIssueOutput := captureStdout(t, func() {
		outputRepairReport(repairReport{}, false) //nolint:errcheck
	})
	if !strings.Contains(noIssueOutput, "No repair issues found") {
		t.Fatalf("no-issue output = %q", noIssueOutput)
	}

	withIssueOutput := captureStdout(t, func() {
		outputRepairReport(repairReport{
			IssueCount:   1,
			FixableCount: 1,
			ManualCount:  1,
			Issues: []repairIssue{{
				Code:     "credential_helper_missing",
				Severity: "error",
				Profile:  "work",
				Message:  "missing helper",
				Action:   "register helper",
				Fixable:  true,
				Error:    "boom",
			}},
		}, false) //nolint:errcheck
	})
	for _, want := range []string{"work: credential_helper_missing", "missing helper", "register helper", "repair failed:", "boom"} {
		if !strings.Contains(withIssueOutput, want) {
			t.Fatalf("output %q does not contain %q", withIssueOutput, want)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	originalUIOut := ui.Out
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writeEnd
	ui.Out = writeEnd
	defer func() {
		os.Stdout = original
		ui.Out = originalUIOut
	}()

	fn()
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, readEnd); err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return buf.String()
}
