package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/providerclient"
)

const defaultResolveTimeout = 8 * time.Second

// TokenStore is the token persistence surface needed by the auth resolver.
type TokenStore interface {
	LoadTokenSet(providerpkg.TokenKey) (providerpkg.TokenSet, error)
	SaveTokenSet(providerpkg.TokenKey, providerpkg.TokenSet) error
	DeleteTokenSet(providerpkg.TokenKey) error
}

// Verifier verifies provider tokens without mutating shared clients.
type Verifier interface {
	VerifyPAT(ctx context.Context, def providerpkg.Definition, token string) (providerclient.AuthenticatedUser, error)
}

// Manager resolves and mutates source-aware auth state.
type Manager struct {
	TokenStore        TokenStore
	Verifier          Verifier
	ExternalInspector ExternalInspector
	Now               func() time.Time
}

// NewManager creates a source-aware auth manager.
func NewManager(tokenStore TokenStore, verifier Verifier) *Manager {
	return &Manager{TokenStore: tokenStore, Verifier: verifier, ExternalInspector: NewGitCredentialInspector(), Now: time.Now}
}

// Resolve returns the source-aware auth status for one profile/provider pair.
func (m *Manager) Resolve(ctx context.Context, req ResolveRequest) (ProfileAuthStatus, error) {
	if req.ProfileName == "" && req.Profile != nil {
		req.ProfileName = req.Profile.Name
	}
	if req.ProfileName == "" {
		return ProfileAuthStatus{}, fmt.Errorf("profile name cannot be empty")
	}
	if req.Provider.ID == "" {
		return ProfileAuthStatus{}, fmt.Errorf("provider cannot be empty")
	}

	resolveCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = defaultResolveTimeout
		}
		resolveCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	now := time.Now
	if m != nil && m.Now != nil {
		now = m.Now
	}

	status := ProfileAuthStatus{
		GeneratedAt:  now().UTC(),
		Profile:      req.ProfileName,
		Provider:     req.Provider.ID,
		ProviderName: req.Provider.DisplayName,
		Host:         PrimaryHost(req.Provider),
		State:        StateUnauthenticated,
		Ownership:    OwnershipUnknown,
	}

	account := profile.ProviderAccount(req.Profile, req.Provider.ID)
	status.GCMCredential = m.resolveGCMCredential(resolveCtx, req, account)
	if status.GCMCredential.Present && status.GCMCredential.Username != "" {
		status.Username = status.GCMCredential.Username
	}

	if req.InspectExternal && m != nil && m.ExternalInspector != nil {
		inspection, err := m.ExternalInspector.InspectGitCredential(resolveCtx, req.Provider, account.Username)
		if err != nil {
			status.ExternalCredential = CredentialStatus{Type: "https", Source: SourceUnknown, Ownership: OwnershipUnknown, State: StateUnknown, Error: err.Error(), Host: status.Host}
		} else {
			status.ExternalCredential = inspection.Credential
			status.CredentialHelpers = inspection.Helpers
			status.GitCredentialUsername = inspection.ConfiguredUsername
			status.ExternalCredential.Helpers = inspection.Helpers
			if status.ExternalCredential.Source == SourceGCMStore && credentialsReferToSameSecret(status.GCMCredential, status.ExternalCredential) {
				status.ExternalCredential.Verified = status.GCMCredential.Verified
				status.ExternalCredential.AuthMethod = firstNonEmpty(status.ExternalCredential.AuthMethod, status.GCMCredential.AuthMethod)
			}
			if inspection.Error != "" {
				status.addFinding("external_inspection_error", "warning", fmt.Sprintf("External Git credential inspection failed: %s", inspection.Error), "Run: gcm auth inspect "+req.ProfileName+" --verbose")
			}
			if !inspection.GCMConfigured {
				status.addFinding("credential_helper_missing", "warning", fmt.Sprintf("GCM is not registered as git credential helper for %s", req.Provider.CredentialServer()), "Run: gcm auth repair --yes")
			}
		}
		if req.Verify && status.ExternalCredential.Present && status.ExternalCredential.Secret != "" && status.ExternalCredential.Source != SourceGCMStore {
			m.verifyCredential(resolveCtx, req.Provider, &status.ExternalCredential, StateAuthenticatedExternal)
		}
	}

	if req.InspectSSH {
		status.SSHCredential = inspectSSHCredential(req.Profile)
	}

	status.applyAccountFindings(account)
	status.finalize(req.Profile, req.Provider)
	return status, nil
}

func (m *Manager) resolveGCMCredential(ctx context.Context, req ResolveRequest, account profile.ProviderAccountConfig) CredentialStatus {
	credential := CredentialStatus{
		Type:      "https",
		Source:    SourceGCMStore,
		Ownership: OwnershipGCM,
		State:     StateUnauthenticated,
		Host:      PrimaryHost(req.Provider),
	}
	if m == nil || m.TokenStore == nil {
		credential.Error = "token store is not configured"
		return credential
	}

	token, err := m.TokenStore.LoadTokenSet(TokenKey(req.ProfileName, req.Provider, req.Profile))
	if err != nil || token.AccessToken == "" {
		if err != nil && !isTokenMissingError(err) {
			credential.Error = err.Error()
		}
		return credential
	}

	credential.Present = true
	credential.Exportable = true
	credential.Token = token
	credential.Secret = token.AccessToken
	credential.AuthMethod = token.AuthMethod
	credential.Username = account.Username
	credential.State = StateAuthenticatedGCM

	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	if token.Expired(now()) {
		credential.State = StateExpired
		credential.Error = "token is expired"
		return credential
	}
	if req.Verify {
		m.verifyCredential(ctx, req.Provider, &credential, StateAuthenticatedGCM)
	}
	return credential
}

func (m *Manager) verifyCredential(ctx context.Context, def providerpkg.Definition, credential *CredentialStatus, authenticatedState State) {
	if credential == nil || credential.Secret == "" || m == nil || m.Verifier == nil {
		return
	}
	user, err := m.Verifier.VerifyPAT(ctx, def, credential.Secret)
	if err != nil {
		credential.Verified = false
		credential.State = StateRevoked
		credential.Error = err.Error()
		return
	}
	credential.Verified = true
	credential.State = authenticatedState
	if user.Username != "" {
		credential.Username = user.Username
	}
}

func inspectSSHCredential(p *profile.Profile) CredentialStatus {
	credential := CredentialStatus{Type: "ssh", Source: SourceProfileSSHKey, Ownership: OwnershipUnknown, State: StateUnauthenticated}
	if p == nil || p.SSH == nil || p.SSH.KeyPath == "" {
		return credential
	}
	credential.Present = true
	credential.Host = filepath.Base(p.SSH.KeyPath)
	if _, err := os.Stat(p.SSH.KeyPath); err != nil {
		credential.State = StateStale
		credential.Error = err.Error()
		return credential
	}
	credential.State = StatePartial
	return credential
}

func (s *ProfileAuthStatus) applyAccountFindings(account profile.ProviderAccountConfig) {
	if account.Username == "" {
		return
	}
	for _, credential := range []CredentialStatus{s.GCMCredential, s.ExternalCredential} {
		if credential.Source == SourceGCMStore && credential.Ownership == OwnershipGCM && credential.Type == "https" && credential.Secret != s.GCMCredential.Secret {
			continue
		}
		if credential.Present && credential.Username != "" && !strings.EqualFold(credential.Username, account.Username) {
			s.addFinding("account_mismatch", "error", fmt.Sprintf("Credential user %q does not match profile account %q", credential.Username, account.Username), "Run: gcm auth inspect "+s.Profile)
		}
	}
}

func (s *ProfileAuthStatus) finalize(p *profile.Profile, def providerpkg.Definition) {
	gcmReady := s.GCMCredential.Present && s.GCMCredential.State == StateAuthenticatedGCM
	externalReady := s.ExternalCredential.Present && s.ExternalCredential.State == StateAuthenticatedExternal
	gcmBad := s.GCMCredential.Present && (s.GCMCredential.State == StateExpired || s.GCMCredential.State == StateRevoked || s.GCMCredential.State == StateStale)
	sshReady := s.SSHCredential.Present && s.SSHCredential.State == StatePartial

	if gcmBad {
		s.addFinding("gcm_token_invalid", "error", "GCM-managed token is expired, revoked, or stale", fmt.Sprintf("Run: gcm connect %s --provider %s", s.Profile, def.ID))
	}
	if externalReady && !gcmReady {
		s.addFinding("external_credential_present", "info", "Git can authenticate through an external credential that GCM does not own", fmt.Sprintf("Run: gcm auth adopt %s --provider %s", s.Profile, def.ID))
		s.Recommendations = append(s.Recommendations, Recommendation{Command: fmt.Sprintf("gcm auth adopt %s --provider %s", s.Profile, def.ID), Reason: "Adopt the external credential into GCM-managed storage"})
	}
	if s.GitCredentialUsername != "" && !gcmReady && !externalReady {
		key := fmt.Sprintf("credential.https://%s.username", s.Host)
		s.addFinding("credential_username_pinned", "warning", fmt.Sprintf("Git has HTTPS username %q pinned for %s, but no HTTPS credential is available", s.GitCredentialUsername, s.Host), "Run: git config --global --unset "+key)
	}

	switch {
	case gcmReady && externalReady:
		if credentialsReferToSameSecret(s.GCMCredential, s.ExternalCredential) || s.ExternalCredential.Source == SourceGCMStore {
			s.State = StateAuthenticatedGCM
			s.Ownership = OwnershipGCM
			s.Username = firstNonEmpty(s.GCMCredential.Username, s.ExternalCredential.Username, s.Username)
		} else if credentialsUserConflict(s.GCMCredential, s.ExternalCredential) || s.hasFinding("account_mismatch") {
			s.State = StateConflicted
			s.Ownership = OwnershipMixed
			s.Username = firstNonEmpty(s.GCMCredential.Username, s.ExternalCredential.Username, s.Username)
			s.addFinding("credential_conflict", "error", "GCM-managed and external credentials resolve to different accounts", "Run: gcm auth inspect "+s.Profile)
		} else {
			s.State = StateAuthenticatedMixed
			s.Ownership = OwnershipMixed
			s.Username = firstNonEmpty(s.GCMCredential.Username, s.ExternalCredential.Username, s.Username)
			s.addFinding("mixed_credentials", "warning", "Both GCM-managed and external credentials are available", "Run: gcm auth logout "+s.Profile+" --scope external --dry-run")
		}
	case gcmReady:
		s.State = StateAuthenticatedGCM
		s.Ownership = OwnershipGCM
		s.Username = firstNonEmpty(s.GCMCredential.Username, s.Username)
	case externalReady:
		s.State = StateAuthenticatedExternal
		s.Ownership = OwnershipExternal
		s.Username = firstNonEmpty(s.ExternalCredential.Username, s.Username)
	case gcmBad:
		s.State = s.GCMCredential.State
		s.Ownership = OwnershipGCM
		s.Username = firstNonEmpty(s.GCMCredential.Username, s.Username)
	case sshReady:
		s.State = StateUnauthenticated
		s.Ownership = OwnershipUnknown
		s.addFinding("ssh_only", "info", "SSH key is available but no HTTPS credential was found", fmt.Sprintf("Run: gcm connect %s --provider %s", s.Profile, def.ID))
		s.Recommendations = append(s.Recommendations, Recommendation{Command: fmt.Sprintf("gcm connect %s --provider %s", s.Profile, def.ID), Reason: "Create a GCM-managed provider credential for HTTPS operations"})
	default:
		s.State = StateUnauthenticated
		s.Ownership = OwnershipUnknown
		s.addFinding("not_authenticated", "warning", "No GCM-managed or external HTTPS credential was found", fmt.Sprintf("Run: gcm connect %s --provider %s", s.Profile, def.ID))
		s.Recommendations = append(s.Recommendations, Recommendation{Command: fmt.Sprintf("gcm connect %s --provider %s", s.Profile, def.ID), Reason: "Create a GCM-managed provider credential"})
	}

	if s.hasFinding("account_mismatch") && (s.State == StateAuthenticatedGCM || s.State == StateAuthenticatedExternal || s.State == StateAuthenticatedMixed) {
		s.State = StateConflicted
	}
	s.Capabilities = buildCapabilities(p, s)
}

func buildCapabilities(p *profile.Profile, s *ProfileAuthStatus) []CapabilityStatus {
	httpsSource := SourceUnknown
	if s.GCMCredential.Present && s.GCMCredential.State == StateAuthenticatedGCM {
		httpsSource = SourceGCMStore
	} else if s.ExternalCredential.Present && s.ExternalCredential.State == StateAuthenticatedExternal {
		httpsSource = s.ExternalCredential.Source
	}
	httpsState := StateUnauthenticated
	if httpsSource != SourceUnknown {
		httpsState = s.State
	}

	sshState := StateUnauthenticated
	sshSource := SourceUnknown
	if s.SSHCredential.Present {
		sshState = s.SSHCredential.State
		sshSource = s.SSHCredential.Source
	}

	gpgState := StateUnauthenticated
	if p != nil && p.GPG != nil && p.GPG.KeyID != "" {
		gpgState = StatePartial
	}

	return []CapabilityStatus{
		{Name: "https_git", State: httpsState, Source: httpsSource},
		{Name: "provider_api", State: httpsState, Source: httpsSource},
		{Name: "ssh_git", State: sshState, Source: sshSource},
		{Name: "gpg_signing", State: gpgState, Source: SourceUnknown},
	}
}

func (s *ProfileAuthStatus) addFinding(code, severity, message, action string) {
	if s.hasFinding(code) {
		return
	}
	s.Findings = append(s.Findings, Finding{Code: code, Severity: severity, Message: message, Action: action})
}

func (s *ProfileAuthStatus) hasFinding(code string) bool {
	for _, finding := range s.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func credentialsReferToSameSecret(a, b CredentialStatus) bool {
	return a.Secret != "" && b.Secret != "" && a.Secret == b.Secret
}

func credentialsUserConflict(a, b CredentialStatus) bool {
	return a.Username != "" && b.Username != "" && !strings.EqualFold(a.Username, b.Username)
}

func isTokenMissingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "does not exist")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// PrimaryHost returns the provider host used in provider-aware token keys.
func PrimaryHost(def providerpkg.Definition) string {
	if len(def.GitHosts) > 0 && def.GitHosts[0] != "" {
		return providerpkg.NormalizeHost(def.GitHosts[0])
	}
	if def.WebURL != "" {
		return providerpkg.NormalizeHost(def.WebURL)
	}
	return providerpkg.NormalizeHost(def.APIURL)
}

// TokenKey returns the provider-aware token key used by GCM-managed credentials.
func TokenKey(profileName string, def providerpkg.Definition, p *profile.Profile) providerpkg.TokenKey {
	account := profile.ProviderAccount(p, def.ID)
	return providerpkg.TokenKey{Profile: profileName, Provider: def.ID, Host: PrimaryHost(def), Account: account.Account}
}
