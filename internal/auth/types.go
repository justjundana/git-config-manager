// Package auth resolves provider authentication state across GCM-managed and external credentials.
package auth

import (
	"time"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

// State describes the resolved authentication state for a profile/provider pair.
type State string

const (
	StateAuthenticatedGCM      State = "authenticated:gcm"
	StateAuthenticatedExternal State = "authenticated:external"
	StateAuthenticatedMixed    State = "authenticated:mixed"
	StatePartial               State = "partial"
	StateStale                 State = "stale"
	StateExpired               State = "expired"
	StateRevoked               State = "revoked"
	StateConflicted            State = "conflicted"
	StateUnknown               State = "unknown"
	StateUnauthenticated       State = "unauthenticated"
)

// Ownership identifies who owns the effective credential.
type Ownership string

const (
	OwnershipGCM      Ownership = "gcm"
	OwnershipExternal Ownership = "external"
	OwnershipMixed    Ownership = "mixed"
	OwnershipUnknown  Ownership = "unknown"
)

// CredentialSource identifies where a credential was discovered.
type CredentialSource string

const (
	SourceGCMStore             CredentialSource = "gcm-store"
	SourceOSXKeychain          CredentialSource = "osxkeychain"
	SourceWinCred              CredentialSource = "wincred"
	SourceLibsecret            CredentialSource = "libsecret"
	SourceGitCredential        CredentialSource = "git-credential"
	SourceGitCredentialManager CredentialSource = "git-credential-manager"
	SourceGitCredentialCache   CredentialSource = "git-credential-cache"
	SourceGitCredentialStore   CredentialSource = "git-credential-store"
	SourceGHCLI                CredentialSource = "gh-cli"
	SourceSSHAgent             CredentialSource = "ssh-agent"
	SourceProfileSSHKey        CredentialSource = "profile-ssh-key"
	SourceUnknown              CredentialSource = "unknown"
)

// CredentialStatus is a source-aware credential snapshot. Secrets are never serialized.
type CredentialStatus struct {
	Type       string                   `json:"type"`
	Source     CredentialSource         `json:"source"`
	Ownership  Ownership                `json:"ownership"`
	State      State                    `json:"state"`
	Present    bool                     `json:"present"`
	Verified   bool                     `json:"verified"`
	Exportable bool                     `json:"exportable"`
	Host       string                   `json:"host,omitempty"`
	Username   string                   `json:"username,omitempty"`
	AuthMethod string                   `json:"auth_method,omitempty"`
	Error      string                   `json:"error,omitempty"`
	Helpers    []CredentialHelperStatus `json:"helpers,omitempty"`
	Token      providerpkg.TokenSet     `json:"-"`
	Secret     string                   `json:"-"`
}

// CredentialHelperStatus describes one configured Git credential helper entry.
type CredentialHelperStatus struct {
	Scope  string           `json:"scope"`
	Key    string           `json:"key"`
	Value  string           `json:"value"`
	Source CredentialSource `json:"source"`
	GCM    bool             `json:"gcm"`
}

// CapabilityStatus reports whether a workflow capability is ready.
type CapabilityStatus struct {
	Name    string           `json:"name"`
	State   State            `json:"state"`
	Source  CredentialSource `json:"source"`
	Details string           `json:"details,omitempty"`
}

// Finding is a diagnostic emitted by the resolver.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

// Recommendation is a next action suggested by the resolver.
type Recommendation struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

// ProfileAuthStatus is the full source-aware status for one profile/provider pair.
type ProfileAuthStatus struct {
	GeneratedAt           time.Time                `json:"generated_at"`
	Profile               string                   `json:"profile"`
	Provider              providerpkg.ProviderID   `json:"provider"`
	ProviderName          string                   `json:"provider_name"`
	Host                  string                   `json:"host"`
	State                 State                    `json:"state"`
	Ownership             Ownership                `json:"ownership"`
	Username              string                   `json:"username,omitempty"`
	GCMCredential         CredentialStatus         `json:"gcm_credential"`
	ExternalCredential    CredentialStatus         `json:"external_credential"`
	SSHCredential         CredentialStatus         `json:"ssh_credential"`
	Capabilities          []CapabilityStatus       `json:"capabilities"`
	CredentialHelpers     []CredentialHelperStatus `json:"credential_helpers,omitempty"`
	GitCredentialUsername string                   `json:"git_credential_username,omitempty"`
	Findings              []Finding                `json:"findings,omitempty"`
	Recommendations       []Recommendation         `json:"recommendations,omitempty"`
}

// ResolveRequest controls a single auth resolution.
type ResolveRequest struct {
	ProfileName     string
	Profile         *profile.Profile
	Provider        providerpkg.Definition
	Verify          bool
	InspectExternal bool
	InspectSSH      bool
	Timeout         time.Duration
}
