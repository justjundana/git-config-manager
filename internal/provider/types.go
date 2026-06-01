// Package provider defines provider-neutral contracts and value types.
package provider

import "time"

const (
	GitHubID    ProviderID = "github"
	GitLabID    ProviderID = "gitlab"
	BitbucketID ProviderID = "bitbucket"
)

const (
	AuthMethodPAT         = "pat"
	AuthMethodOAuthDevice = "oauth_device"
	AuthMethodOAuthPKCE   = "oauth_pkce"
	AuthMethodLegacy      = "legacy"
)

// ProviderID is the stable identifier used in config, profiles, and token keys.
type ProviderID string

// TokenKey uniquely identifies one stored credential.
type TokenKey struct {
	Profile  string     `json:"profile"`
	Provider ProviderID `json:"provider"`
	Host     string     `json:"host,omitempty"`
	Account  string     `json:"account,omitempty"`
}

// TokenSet stores both present-day PAT credentials and future OAuth tokens.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	AuthMethod   string    `json:"auth_method,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

// Expired reports whether the access token is known to be expired.
func (t TokenSet) Expired(now time.Time) bool {
	return !t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt)
}

// Bearer reports whether this token should be sent as an OAuth bearer token.
func (t TokenSet) Bearer() bool {
	return t.TokenType == "bearer" || t.TokenType == "Bearer" ||
		t.AuthMethod == AuthMethodOAuthDevice || t.AuthMethod == AuthMethodOAuthPKCE
}

// Capability names a provider feature that callers can test before invoking.
type Capability string

const (
	CapabilityPATAuth          Capability = "pat_auth"
	CapabilityOAuthDeviceAuth  Capability = "oauth_device_auth"
	CapabilityOAuthPKCEAuth    Capability = "oauth_pkce_auth"
	CapabilityCredentialHelper Capability = "credential_helper"
	CapabilitySSHKeys          Capability = "ssh_keys"
	CapabilityGPGKeys          Capability = "gpg_keys"
	CapabilityRepositories     Capability = "repositories"
	CapabilityGroups           Capability = "groups"
	CapabilityWebhooks         Capability = "webhooks"
	CapabilityCICD             Capability = "ci_cd"
)

// CapabilitySet is a simple provider feature set.
type CapabilitySet map[Capability]bool

// Has reports whether a capability is available.
func (s CapabilitySet) Has(capability Capability) bool {
	return s != nil && s[capability]
}
