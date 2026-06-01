package auth

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/providerclient"
)

type fakeTokenStore struct {
	tokens map[providerpkg.TokenKey]providerpkg.TokenSet
}

func (f fakeTokenStore) LoadTokenSet(key providerpkg.TokenKey) (providerpkg.TokenSet, error) {
	if token, ok := f.tokens[key]; ok {
		return token, nil
	}
	return providerpkg.TokenSet{}, fmt.Errorf("not found")
}

func (f fakeTokenStore) SaveTokenSet(key providerpkg.TokenKey, token providerpkg.TokenSet) error {
	f.tokens[key] = token
	return nil
}

func (f fakeTokenStore) DeleteTokenSet(key providerpkg.TokenKey) error {
	delete(f.tokens, key)
	return nil
}

type failingTokenStore struct {
	err error
}

func (f failingTokenStore) LoadTokenSet(providerpkg.TokenKey) (providerpkg.TokenSet, error) {
	return providerpkg.TokenSet{}, f.err
}

func (f failingTokenStore) SaveTokenSet(providerpkg.TokenKey, providerpkg.TokenSet) error {
	return nil
}

func (f failingTokenStore) DeleteTokenSet(providerpkg.TokenKey) error {
	return nil
}

type fakeVerifier map[string]providerclient.AuthenticatedUser

func (f fakeVerifier) VerifyPAT(_ context.Context, _ providerpkg.Definition, token string) (providerclient.AuthenticatedUser, error) {
	if user, ok := f[token]; ok {
		return user, nil
	}
	return providerclient.AuthenticatedUser{}, fmt.Errorf("revoked")
}

type fakeExternalInspector struct {
	inspection GitCredentialInspection
	err        error
}

func (f fakeExternalInspector) InspectGitCredential(context.Context, providerpkg.Definition, string) (GitCredentialInspection, error) {
	return f.inspection, f.err
}

func (f fakeExternalInspector) RejectGitCredential(context.Context, providerpkg.Definition, string) error {
	return nil
}

func testDefinition() providerpkg.Definition {
	return providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", APIURL: "https://api.github.com", WebURL: "https://github.com", GitHosts: []string{"github.com"}}
}

func testProfile() *profile.Profile {
	return &profile.Profile{Name: "work", Providers: map[string]profile.ProviderAccountConfig{"github": {Username: "octo"}}}
}

func TestResolveGCMTokenVerified(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	key := TokenKey("work", def, p)
	mgr := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "gcm-token", AuthMethod: providerpkg.AuthMethodPAT}}},
		Verifier:   fakeVerifier{"gcm-token": {Username: "octo"}},
		Now:        func() time.Time { return time.Unix(100, 0) },
	}

	status, err := mgr.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateAuthenticatedGCM || status.Ownership != OwnershipGCM {
		t.Fatalf("state=%s ownership=%s", status.State, status.Ownership)
	}
	if !status.GCMCredential.Verified || status.Username != "octo" {
		t.Fatalf("expected verified octo credential: %+v", status.GCMCredential)
	}
}

func TestResolveExternalOnly(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	mgr := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}},
		Verifier:   fakeVerifier{"external-token": {Username: "octo"}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo",
		}}},
	}

	status, err := mgr.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateAuthenticatedExternal || status.Ownership != OwnershipExternal {
		t.Fatalf("state=%s ownership=%s", status.State, status.Ownership)
	}
	if !status.ExternalCredential.Verified || len(status.Recommendations) == 0 {
		t.Fatalf("expected verified external credential and adoption recommendation: %+v", status)
	}
}

func TestResolveMissingGCMTokenHasNoError(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	manager := &Manager{TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}}}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.GCMCredential.Present || status.GCMCredential.Error != "" {
		t.Fatalf("missing token should be absent without error: %+v", status.GCMCredential)
	}
	if !isTokenMissingError(fmt.Errorf("secret does not exist")) || isTokenMissingError(fmt.Errorf("permission denied")) || isTokenMissingError(nil) {
		t.Fatal("unexpected missing-token error classification")
	}

	manager.TokenStore = failingTokenStore{err: fmt.Errorf("permission denied")}
	status, err = manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def})
	if err != nil {
		t.Fatalf("Resolve with failing store: %v", err)
	}
	if status.GCMCredential.Error != "permission denied" {
		t.Fatalf("expected non-missing token error to be preserved: %+v", status.GCMCredential)
	}
}

func TestResolvePinnedGitCredentialUsernameWithoutHTTPSCredential(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{
			Credential:         CredentialStatus{Type: "https", Source: SourceUnknown, Ownership: OwnershipUnknown, State: StateUnauthenticated, Host: "github.com", Username: "octo"},
			ConfiguredUsername: "octo",
			GCMConfigured:      true,
		}},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.GitCredentialUsername != "octo" || !status.hasFinding("credential_username_pinned") {
		t.Fatalf("expected pinned username finding: %+v", status)
	}
}

func TestResolveConflictingCredentials(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	key := TokenKey("work", def, p)
	mgr := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "gcm-token"}}},
		Verifier: fakeVerifier{
			"gcm-token":      {Username: "octo"},
			"external-token": {Username: "other"},
		},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "other",
		}}},
	}

	status, err := mgr.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateConflicted || status.Ownership != OwnershipMixed {
		t.Fatalf("state=%s ownership=%s findings=%+v", status.State, status.Ownership, status.Findings)
	}
	if !status.hasFinding("credential_conflict") || !status.hasFinding("account_mismatch") {
		t.Fatalf("expected conflict findings: %+v", status.Findings)
	}
}

func TestResolveGCMHelperCredentialForDifferentActiveProfileDoesNotConflict(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	p.Providers["github"] = profile.ProviderAccountConfig{Username: "justjundana"}
	key := TokenKey("work", def, p)
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "profile-token"}}},
		Verifier:   fakeVerifier{"profile-token": {Username: "justjundana"}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceGCMStore, Ownership: OwnershipGCM, State: StateAuthenticatedGCM, Present: true, Secret: "active-profile-token", Username: "justjundana",
		}}},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateAuthenticatedGCM || status.Ownership != OwnershipGCM || status.Username != "justjundana" {
		t.Fatalf("expected GCM-owned status for profile token: %+v", status)
	}
	if status.ExternalCredential.State != StateAuthenticatedGCM || status.ExternalCredential.Ownership != OwnershipGCM || status.ExternalCredential.Verified {
		t.Fatalf("GCM helper credential should remain non-external and unverified by resolver: %+v", status.ExternalCredential)
	}
	if status.hasFinding("account_mismatch") || status.hasFinding("credential_conflict") || status.hasFinding("external_credential_present") {
		t.Fatalf("GCM helper credential should not create external findings: %+v", status.Findings)
	}
}

func TestResolveGCMHelperCredentialMirrorsProfileTokenVerification(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	key := TokenKey("work", def, p)
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "profile-token", AuthMethod: providerpkg.AuthMethodPAT}}},
		Verifier:   fakeVerifier{"profile-token": {Username: "octo"}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceGCMStore, Ownership: OwnershipGCM, State: StateAuthenticatedGCM, Present: true, Secret: "profile-token", Username: "octo",
		}}},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !status.ExternalCredential.Verified || status.ExternalCredential.AuthMethod != providerpkg.AuthMethodPAT {
		t.Fatalf("expected helper credential to mirror GCM verification: %+v", status.ExternalCredential)
	}
}

func TestResolveGCMHelperCredentialWithoutProfileTokenIsNotExternal(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}},
		Verifier:   fakeVerifier{"active-profile-token": {Username: "justjundana"}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceGCMStore, Ownership: OwnershipGCM, State: StateAuthenticatedGCM, Present: true, Secret: "active-profile-token", Username: "justjundana",
		}}},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateUnauthenticated || status.Ownership != OwnershipUnknown {
		t.Fatalf("GCM helper credential without profile token should not authenticate the profile: %+v", status)
	}
	if status.ExternalCredential.Verified || status.hasFinding("external_credential_present") {
		t.Fatalf("GCM helper credential should not be treated as adoptable external auth: %+v", status)
	}
	for _, recommendation := range status.Recommendations {
		if recommendation.Command == "gcm auth adopt work --provider github" {
			t.Fatalf("unexpected external adoption recommendation: %+v", status.Recommendations)
		}
	}
}

func TestResolveExpiredTokenFallsBackToSSHPartial(t *testing.T) {
	def := testDefinition()
	tmp := t.TempDir()
	keyPath := tmp + "/id_gcm_github_work"
	if err := os.WriteFile(keyPath, []byte("private"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	p := testProfile()
	p.SSH = &profile.SSHConfig{KeyPath: keyPath}
	key := TokenKey("work", def, p)
	mgr := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "expired", ExpiresAt: time.Unix(90, 0)}}},
		Now:        func() time.Time { return time.Unix(100, 0) },
	}

	status, err := mgr.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectSSH: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateExpired || status.SSHCredential.State != StatePartial {
		t.Fatalf("state=%s ssh=%s", status.State, status.SSHCredential.State)
	}
}

func TestTokenKeyUsesPrimaryHostAndAccount(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	p.Providers["github"] = profile.ProviderAccountConfig{Username: "octo", Account: "enterprise"}
	key := TokenKey("work", def, p)
	if key.Profile != "work" || key.Provider != providerpkg.GitHubID || key.Host != "github.com" || key.Account != "enterprise" {
		t.Fatalf("unexpected key: %+v", key)
	}
}

func TestResolveErrorBranches(t *testing.T) {
	manager := NewManager(fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}}, fakeVerifier{})
	if manager.TokenStore == nil || manager.Verifier == nil || manager.ExternalInspector == nil || manager.Now == nil {
		t.Fatalf("NewManager did not initialize dependencies: %+v", manager)
	}
	if _, err := manager.Resolve(context.Background(), ResolveRequest{Provider: testDefinition()}); err == nil {
		t.Fatal("expected empty profile name error")
	}
	if _, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work"}); err == nil {
		t.Fatal("expected empty provider error")
	}
	status, err := manager.Resolve(context.Background(), ResolveRequest{Profile: &profile.Profile{Name: "work"}, Provider: testDefinition(), Timeout: time.Millisecond})
	if err != nil {
		t.Fatalf("Resolve with profile-derived name: %v", err)
	}
	if status.Profile != "work" {
		t.Fatalf("profile-derived name not applied: %+v", status)
	}
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()
	if _, err := manager.Resolve(deadlineCtx, ResolveRequest{ProfileName: "work", Provider: testDefinition()}); err != nil {
		t.Fatalf("Resolve with deadline context: %v", err)
	}
}

func TestResolveTokenStoreAndExternalInspectorBranches(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	manager := &Manager{
		Verifier:          fakeVerifier{"external-token": {Username: "octo"}},
		ExternalInspector: fakeExternalInspector{err: fmt.Errorf("inspector failed")},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.GCMCredential.Error != "token store is not configured" || status.ExternalCredential.State != StateUnknown {
		t.Fatalf("unexpected status: %+v", status)
	}

	manager.ExternalInspector = fakeExternalInspector{inspection: GitCredentialInspection{
		Credential: CredentialStatus{Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateRevoked, Present: true, Secret: "bad-token", Username: "octo"},
		Error:      "fill failed",
	}}
	status, err = manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve external error: %v", err)
	}
	if !status.hasFinding("external_inspection_error") || !status.hasFinding("not_authenticated") {
		t.Fatalf("expected external and unauthenticated findings: %+v", status.Findings)
	}
}

func TestResolveAggregationBranches(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	key := TokenKey("work", def, p)
	tests := []struct {
		name        string
		gcmToken    providerpkg.TokenSet
		external    CredentialStatus
		verifier    fakeVerifier
		wantState   State
		wantOwner   Ownership
		wantFinding string
	}{
		{
			name:      "same secret remains gcm owned",
			gcmToken:  providerpkg.TokenSet{AccessToken: "same-token"},
			external:  CredentialStatus{Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "same-token", Username: "octo"},
			verifier:  fakeVerifier{"same-token": {Username: "octo"}},
			wantState: StateAuthenticatedGCM,
			wantOwner: OwnershipGCM,
		},
		{
			name:        "different tokens same user become mixed",
			gcmToken:    providerpkg.TokenSet{AccessToken: "gcm-token"},
			external:    CredentialStatus{Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo"},
			verifier:    fakeVerifier{"gcm-token": {Username: "octo"}, "external-token": {Username: "octo"}},
			wantState:   StateAuthenticatedMixed,
			wantOwner:   OwnershipMixed,
			wantFinding: "mixed_credentials",
		},
		{
			name:        "bad gcm token with good external becomes external",
			gcmToken:    providerpkg.TokenSet{AccessToken: "bad-token"},
			external:    CredentialStatus{Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo"},
			verifier:    fakeVerifier{"external-token": {Username: "octo"}},
			wantState:   StateAuthenticatedExternal,
			wantOwner:   OwnershipExternal,
			wantFinding: "gcm_token_invalid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{
				TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: tc.gcmToken}},
				Verifier:   tc.verifier,
				ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{
					Credential: tc.external,
				}},
			}
			status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if status.State != tc.wantState || status.Ownership != tc.wantOwner {
				t.Fatalf("state=%s ownership=%s findings=%+v", status.State, status.Ownership, status.Findings)
			}
			if tc.wantFinding != "" && !status.hasFinding(tc.wantFinding) {
				t.Fatalf("missing finding %q: %+v", tc.wantFinding, status.Findings)
			}
		})
	}
}

func TestResolveSSHAndGPGCapabilities(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	p.SSH = &profile.SSHConfig{KeyPath: "/missing/key"}
	p.GPG = &profile.GPGConfig{KeyID: "ABC123"}
	manager := &Manager{TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}}}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, InspectSSH: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.SSHCredential.State != StateStale {
		t.Fatalf("expected stale ssh credential: %+v", status.SSHCredential)
	}
	foundGPG := false
	for _, capability := range status.Capabilities {
		if capability.Name == "gpg_signing" && capability.State == StatePartial {
			foundGPG = true
		}
	}
	if !foundGPG {
		t.Fatalf("expected partial gpg capability: %+v", status.Capabilities)
	}
}

func TestResolveSSHOnlyUnauthenticatedAndNilSSH(t *testing.T) {
	def := testDefinition()
	tmp := t.TempDir()
	keyPath := tmp + "/id_gcm_github_work"
	if err := os.WriteFile(keyPath, []byte("private"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	p := testProfile()
	p.SSH = &profile.SSHConfig{KeyPath: keyPath}
	manager := &Manager{TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{}}}
	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, InspectSSH: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateUnauthenticated || status.SSHCredential.State != StatePartial {
		t.Fatalf("expected unauthenticated overall with ssh partial: state=%s ssh=%s", status.State, status.SSHCredential.State)
	}
	if !status.hasFinding("ssh_only") {
		t.Fatalf("expected ssh_only finding: %+v", status.Findings)
	}
	if status.Username != "" {
		t.Fatalf("expected empty username when no HTTPS credential: %q", status.Username)
	}
	if got := inspectSSHCredential(nil); got.State != StateUnauthenticated {
		t.Fatalf("nil ssh status = %+v", got)
	}
}

func TestResolveVerifierAndAccountMismatchBranches(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	key := TokenKey("work", def, p)
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "gcm-token"}}},
		Verifier:   fakeVerifier{"gcm-token": {Username: "different"}},
	}

	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateConflicted || !status.hasFinding("account_mismatch") {
		t.Fatalf("expected account mismatch conflict: %+v", status)
	}

	credential := CredentialStatus{}
	manager.verifyCredential(context.Background(), def, &credential, StateAuthenticatedGCM)
	if credential.State != "" {
		t.Fatalf("empty credential should not be mutated: %+v", credential)
	}
}

func TestResolveMixedAccountMismatchKeepsMixedOwnership(t *testing.T) {
	def := testDefinition()
	p := testProfile()
	p.Providers["github"] = profile.ProviderAccountConfig{Username: "expected"}
	key := TokenKey("work", def, p)
	manager := &Manager{
		TokenStore: fakeTokenStore{tokens: map[providerpkg.TokenKey]providerpkg.TokenSet{key: {AccessToken: "gcm-token"}}},
		Verifier:   fakeVerifier{"gcm-token": {Username: "octo"}, "external-token": {Username: "octo"}},
		ExternalInspector: fakeExternalInspector{inspection: GitCredentialInspection{Credential: CredentialStatus{
			Type: "https", Source: SourceOSXKeychain, Ownership: OwnershipExternal, State: StateAuthenticatedExternal, Present: true, Secret: "external-token", Username: "octo",
		}}},
	}
	status, err := manager.Resolve(context.Background(), ResolveRequest{ProfileName: "work", Profile: p, Provider: def, Verify: true, InspectExternal: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if status.State != StateConflicted || status.Ownership != OwnershipMixed {
		t.Fatalf("expected mixed ownership conflict: %+v", status)
	}

	status.addFinding("duplicate", "warning", "one", "")
	status.addFinding("duplicate", "warning", "two", "")
	count := 0
	for _, finding := range status.Findings {
		if finding.Code == "duplicate" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate finding was appended %d times", count)
	}
	status.applyAccountFindings(profile.ProviderAccountConfig{})
}

func TestPrimaryHostFallbacksAndFirstNonEmpty(t *testing.T) {
	if got := PrimaryHost(providerpkg.Definition{WebURL: "https://example.com/team"}); got != "example.com" {
		t.Fatalf("web fallback = %q", got)
	}
	if got := PrimaryHost(providerpkg.Definition{APIURL: "https://api.example.com/v1"}); got != "api.example.com" {
		t.Fatalf("api fallback = %q", got)
	}
	if got := firstNonEmpty("", "value"); got != "value" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty(""); got != "" {
		t.Fatalf("firstNonEmpty empty = %q", got)
	}
}
