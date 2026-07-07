package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_auth "github.com/justjundana/git-config-manager/internal/auth"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_providerclient "github.com/justjundana/git-config-manager/internal/providerclient"
)

type cliAuthFakeVerifier map[string]_providerclient.AuthenticatedUser

func (f cliAuthFakeVerifier) VerifyPAT(_ context.Context, _ _provider.Definition, token string) (_providerclient.AuthenticatedUser, error) {
	if user, ok := f[token]; ok {
		return user, nil
	}
	return _providerclient.AuthenticatedUser{}, fmt.Errorf("invalid token")
}

type cliAuthFakeInspector struct {
	inspection GitCredentialInspection
	rejected   bool
}

type GitCredentialInspection = _auth.GitCredentialInspection

func (f *cliAuthFakeInspector) InspectGitCredential(context.Context, _provider.Definition, string) (_auth.GitCredentialInspection, error) {
	return f.inspection, nil
}

func (f *cliAuthFakeInspector) RejectGitCredential(context.Context, _provider.Definition, string) error {
	f.rejected = true
	return nil
}

func withAuthManagerFactory(t *testing.T, manager *_auth.Manager) {
	t.Helper()
	original := authManagerFactory
	authManagerFactory = func() *_auth.Manager { return manager }
	t.Cleanup(func() { authManagerFactory = original })
}

func createAuthTestProfile(t *testing.T, name string, providerID _provider.ProviderID, username string) *_profile.Profile {
	t.Helper()
	p := repairTestProfile(name)
	if providerID != "" {
		p.Providers = map[string]_profile.ProviderAccountConfig{string(providerID): {Username: username}}
	}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return p
}

func TestAuthCommandRegistration(t *testing.T) {
	root := NewRootCmd()
	for _, path := range [][]string{{"auth"}, {"auth", "status"}, {"auth", "inspect"}, {"auth", "adopt"}, {"auth", "logout"}, {"auth", "doctor"}, {"auth", "repair"}} {
		if _, _, err := root.Find(path); err != nil {
			t.Fatalf("missing command %v: %v", path, err)
		}
	}
}

func TestRunAuthStatusUsesSourceAwareResolver(t *testing.T) {
	c := withRepairTestContainer(t)
	p := createAuthTestProfile(t, "work", _provider.GitHubID, "octo")
	def, ok := c.ProviderRegistry.Get(_provider.GitHubID)
	if !ok {
		t.Fatal("github provider missing")
	}
	if err := saveProviderToken("work", def, p, _provider.TokenSet{AccessToken: "gcm-token", AuthMethod: _provider.AuthMethodPAT}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{"gcm-token": {Username: "octo"}})
	manager.ExternalInspector = &cliAuthFakeInspector{}
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthStatus(context.Background(), "work", authStatusOptions{}); err != nil {
			t.Fatalf("runAuthStatus: %v", err)
		}
	})
	if !strings.Contains(output, "authenticated:gcm") || !strings.Contains(output, "gcm-store") {
		t.Fatalf("status output missing source-aware state:\n%s", output)
	}
}

func TestRunAuthStatusUnauthenticatedHidesUsername(t *testing.T) {
	c := withRepairTestContainer(t)
	createAuthTestProfile(t, "logged-out", _provider.GitHubID, "octo")
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{})
	manager.ExternalInspector = &cliAuthFakeInspector{}
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthStatus(context.Background(), "logged-out", authStatusOptions{verbose: true}); err != nil {
			t.Fatalf("runAuthStatus: %v", err)
		}
	})
	if strings.Contains(output, "octo") {
		t.Fatalf("unauthenticated profile should not display username:\n%s", output)
	}
	if !strings.Contains(output, "unauthenticated") {
		t.Fatalf("expected unauthenticated state:\n%s", output)
	}
}

func TestRunAuthAdoptDryRunDoesNotMutateProfile(t *testing.T) {
	c := withRepairTestContainer(t)
	createAuthTestProfile(t, "work", "", "")
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{"external-token": {Username: "octo"}})
	manager.ExternalInspector = &cliAuthFakeInspector{inspection: _auth.GitCredentialInspection{Credential: _auth.CredentialStatus{
		Type: "https", Source: _auth.SourceOSXKeychain, Ownership: _auth.OwnershipExternal, State: _auth.StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo",
	}}}
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthAdopt(context.Background(), "work", authAdoptOptions{provider: "github", dryRun: true}); err != nil {
			t.Fatalf("runAuthAdopt: %v", err)
		}
	})
	updated, err := c.ProfileManager.Get("work")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if _profile.UsesProvider(updated, _provider.GitHubID) {
		t.Fatal("dry-run adopt should not set provider")
	}
	if !strings.Contains(output, "Auth Adopt Dry Run") {
		t.Fatalf("missing dry-run output:\n%s", output)
	}
}

func TestRunAuthLogoutDryRunDoesNotRejectExternalCredential(t *testing.T) {
	c := withRepairTestContainer(t)
	createAuthTestProfile(t, "work", _provider.GitHubID, "octo")
	inspector := &cliAuthFakeInspector{inspection: _auth.GitCredentialInspection{Credential: _auth.CredentialStatus{
		Type: "https", Source: _auth.SourceOSXKeychain, Ownership: _auth.OwnershipExternal, State: _auth.StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo",
	}}}
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{"external-token": {Username: "octo"}})
	manager.ExternalInspector = inspector
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthLogout(context.Background(), "work", authLogoutOptions{scope: "external", dryRun: true}); err != nil {
			t.Fatalf("runAuthLogout: %v", err)
		}
	})
	if inspector.rejected {
		t.Fatal("dry-run logout should not reject external credential")
	}
	if !strings.Contains(output, "Auth Logout Dry Run") || !strings.Contains(output, "osxkeychain") {
		t.Fatalf("missing logout dry-run details:\n%s", output)
	}
}

func TestRunAuthLogoutIgnoresGCMHelperCredentialAsExternal(t *testing.T) {
	c := withRepairTestContainer(t)
	createAuthTestProfile(t, "work", _provider.GitHubID, "octo")
	inspector := &cliAuthFakeInspector{inspection: _auth.GitCredentialInspection{Credential: _auth.CredentialStatus{
		Type: "https", Source: _auth.SourceGCMStore, Ownership: _auth.OwnershipGCM, State: _auth.StateAuthenticatedGCM, Present: true, Secret: "active-profile-token", Username: "other",
	}}}
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{})
	manager.ExternalInspector = inspector
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthLogout(context.Background(), "work", authLogoutOptions{scope: "external"}); err != nil {
			t.Fatalf("runAuthLogout: %v", err)
		}
	})
	if inspector.rejected {
		t.Fatal("GCM helper credential should not be rejected as external")
	}
	if !strings.Contains(output, "No external credential owned by another tool was found.") {
		t.Fatalf("missing no-external message:\n%s", output)
	}
}

func TestRunAuthLogoutScopeAllReportsMissingGCMToken(t *testing.T) {
	c := withRepairTestContainer(t)
	profileName := "absent-auth-token"
	createAuthTestProfile(t, profileName, _provider.GitHubID, "octo")
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{})
	manager.ExternalInspector = &cliAuthFakeInspector{}
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthLogout(context.Background(), profileName, authLogoutOptions{scope: "all", yes: true}); err != nil {
			t.Fatalf("runAuthLogout: %v", err)
		}
	})
	if !strings.Contains(output, "No GCM-managed GitHub token was found for \""+profileName+"\".") {
		t.Fatalf("missing absent GCM token message:\n%s", output)
	}
}

func TestRunAuthLogoutActiveProfileClearsCredentialsAndRejects(t *testing.T) {
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	c := withRepairTestContainer(t)
	profileName := "active-logout"
	p := createAuthTestProfile(t, profileName, _provider.GitHubID, "octo")
	if err := c.ProfileSwitcher.Activate(profileName, _profile.ScopeGlobal); err != nil {
		t.Fatalf("activate profile: %v", err)
	}
	def, ok := c.ProviderRegistry.Get(_provider.GitHubID)
	if !ok {
		t.Fatal("github provider missing")
	}
	if err := saveProviderToken(profileName, def, p, _provider.TokenSet{AccessToken: "token", AuthMethod: _provider.AuthMethodPAT}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if err := c.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "octo"); err != nil {
		t.Fatalf("set credential username: %v", err)
	}
	inspector := &cliAuthFakeInspector{}
	manager := _auth.NewManager(c.TokenStore, cliAuthFakeVerifier{"token": {Username: "octo"}})
	manager.ExternalInspector = inspector
	withAuthManagerFactory(t, manager)

	output := captureStdout(t, func() {
		if err := runAuthLogout(context.Background(), profileName, authLogoutOptions{scope: "gcm"}); err != nil {
			t.Fatalf("runAuthLogout: %v", err)
		}
	})
	if !strings.Contains(output, "GCM-managed GitHub token removed") {
		t.Fatalf("missing token removal message:\n%s", output)
	}
	if !inspector.rejected {
		t.Fatal("expected RejectGitCredential to be called for active profile")
	}
	if out, err := exec.Command("git", "config", "--global", "--get", "credential.https://github.com.username").CombinedOutput(); err == nil {
		t.Fatalf("credential username still configured: %q", out)
	}
}

func TestGitHubLogoutReportsMissingToken(t *testing.T) {
	withRepairTestContainer(t)
	profileName := "absent-github-token"
	createAuthTestProfile(t, profileName, _provider.GitHubID, "octo")

	output := captureStdout(t, func() {
		if err := runRootCommand(t, "github", "logout", profileName, "--force", "--clear-credentials=false"); err != nil {
			t.Fatalf("github logout: %v", err)
		}
	})
	if !strings.Contains(output, "No GitHub token was stored for profile \""+profileName+"\".") {
		t.Fatalf("missing absent GitHub token message:\n%s", output)
	}
	if strings.Contains(output, "GitHub token removed") {
		t.Fatalf("logout should not claim removal when token is absent:\n%s", output)
	}
}

func TestGitHubLogoutClearsCredentialUsernamePin(t *testing.T) {
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	c := withRepairTestContainer(t)
	profileName := "logout-pin"
	p := createAuthTestProfile(t, profileName, _provider.GitHubID, "octo")
	if err := c.ProfileSwitcher.Activate(profileName, _profile.ScopeGlobal); err != nil {
		t.Fatalf("activate profile: %v", err)
	}
	def, ok := c.ProviderRegistry.Get(_provider.GitHubID)
	if !ok {
		t.Fatal("github provider missing")
	}
	if err := saveProviderToken(profileName, def, p, _provider.TokenSet{AccessToken: "token", AuthMethod: _provider.AuthMethodPAT}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if err := c.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "octo"); err != nil {
		t.Fatalf("set credential username: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runRootCommand(t, "github", "logout", profileName); err != nil {
			t.Fatalf("github logout: %v", err)
		}
	})
	if !strings.Contains(output, "username pin cleared") {
		t.Fatalf("missing username pin cleanup output:\n%s", output)
	}
	if out, err := exec.Command("git", "config", "--global", "--get", "credential.https://github.com.username").CombinedOutput(); err == nil {
		t.Fatalf("credential username still configured: %q", out)
	}
}

func TestBuildAuthDoctorReportSkipsProviderlessProfiles(t *testing.T) {
	report := buildAuthDoctorReport([]_auth.ProfileAuthStatus{
		{
			Profile: "local-only",
			State:   _auth.StateUnauthenticated,
			Findings: []_auth.Finding{{
				Code:     "profile_provider_unresolved",
				Severity: "warning",
				Message:  "no provider",
			}},
		},
		{
			Profile:  "work",
			Provider: _provider.GitHubID,
			State:    _auth.StateUnauthenticated,
			Findings: []_auth.Finding{{
				Code:     "not_authenticated",
				Severity: "warning",
				Message:  "No GCM-managed or external HTTPS credential was found",
			}},
		},
	})

	if report.IssueCount != 1 || len(report.Findings) != 1 || report.Findings[0].Profile != "work" {
		t.Fatalf("unexpected doctor report: %+v", report)
	}
}

func TestGitCredentialDetailLabel(t *testing.T) {
	if got := gitCredentialDetailLabel(_auth.CredentialStatus{Source: _auth.SourceGCMStore}); got != "Git via GCM Helper" {
		t.Fatalf("GCM label = %q", got)
	}
	if got := gitCredentialDetailLabel(_auth.CredentialStatus{Source: _auth.SourceOSXKeychain}); got != "External Git" {
		t.Fatalf("external label = %q", got)
	}
}
