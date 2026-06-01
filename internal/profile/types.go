// Package profile provides Git profile data types for GCM.
package profile

import (
	"time"
)

// Profile represents a complete Git identity configuration.
type Profile struct {
	Name   string        `yaml:"name" json:"name"`
	Git    GitConfig     `yaml:"git" json:"git"`
	SSH    *SSHConfig    `yaml:"ssh,omitempty" json:"ssh,omitempty"`
	GPG    *GPGConfig    `yaml:"gpg,omitempty" json:"gpg,omitempty"`
	GitHub *GitHubConfig `yaml:"github,omitempty" json:"github,omitempty"`
	// Providers stores the single provider account selected for this profile.
	Providers map[string]ProviderAccountConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
	Metadata  Metadata                         `yaml:"metadata" json:"metadata"`
}

// GitConfig holds all git configuration for a profile.
type GitConfig struct {
	User    GitUser           `yaml:"user" json:"user"`
	Core    GitCore           `yaml:"core,omitempty" json:"core,omitempty"`
	Commit  GitCommit         `yaml:"commit,omitempty" json:"commit,omitempty"`
	Pull    GitPull           `yaml:"pull,omitempty" json:"pull,omitempty"`
	Push    GitPush           `yaml:"push,omitempty" json:"push,omitempty"`
	Aliases map[string]string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Custom  map[string]string `yaml:"custom,omitempty" json:"custom,omitempty"`
}

// GitUser holds user identity.
type GitUser struct {
	Name       string `yaml:"name" json:"name"`
	Email      string `yaml:"email" json:"email"`
	SigningKey string `yaml:"signingkey,omitempty" json:"signingkey,omitempty"`
}

// GitCore holds core git settings.
type GitCore struct {
	Editor            string `yaml:"editor,omitempty" json:"editor,omitempty"`
	AutoCRLF          string `yaml:"autocrlf,omitempty" json:"autocrlf,omitempty"`
	EOL               string `yaml:"eol,omitempty" json:"eol,omitempty"`
	FileMode          *bool  `yaml:"filemode,omitempty" json:"filemode,omitempty"`
	IgnoreCase        *bool  `yaml:"ignorecase,omitempty" json:"ignorecase,omitempty"`
	PrecomposeUnicode *bool  `yaml:"precomposeunicode,omitempty" json:"precomposeunicode,omitempty"`
}

// GitCommit holds commit-related settings.
type GitCommit struct {
	GPGSign  *bool  `yaml:"gpgsign,omitempty" json:"gpgsign,omitempty"`
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
	Verbose  *bool  `yaml:"verbose,omitempty" json:"verbose,omitempty"`
}

// GitPull holds pull-related settings.
type GitPull struct {
	Rebase string `yaml:"rebase,omitempty" json:"rebase,omitempty"`
	FF     string `yaml:"ff,omitempty" json:"ff,omitempty"`
}

// GitPush holds push-related settings.
type GitPush struct {
	Default         string `yaml:"default,omitempty" json:"default,omitempty"`
	FollowTags      *bool  `yaml:"followtags,omitempty" json:"followtags,omitempty"`
	AutoSetupRemote *bool  `yaml:"autosetupremote,omitempty" json:"autosetupremote,omitempty"`
}

// SSHConfig holds SSH key configuration.
type SSHConfig struct {
	KeyPath     string  `yaml:"key_path" json:"key_path"`
	KeyType     KeyType `yaml:"key_type,omitempty" json:"key_type,omitempty"`
	Comment     string  `yaml:"comment,omitempty" json:"comment,omitempty"`
	Fingerprint string  `yaml:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	LoadToAgent *bool   `yaml:"load_to_agent,omitempty" json:"load_to_agent,omitempty"`
}

// KeyType represents the type of SSH key.
type KeyType string

// GPGConfig holds GPG key configuration.
type GPGConfig struct {
	KeyID     string     `yaml:"key_id" json:"key_id"`
	Program   string     `yaml:"program,omitempty" json:"program,omitempty"`
	Format    string     `yaml:"format,omitempty" json:"format,omitempty"`
	ExpiresAt *time.Time `yaml:"expires_at,omitempty" json:"expires_at,omitempty"`
}

// GitHubConfig holds GitHub account configuration.
type GitHubConfig struct {
	Username   string `yaml:"username" json:"username"`
	TokenPath  string `yaml:"token_path,omitempty" json:"token_path,omitempty"`
	UploadKeys *bool  `yaml:"upload_keys,omitempty" json:"upload_keys,omitempty"`
}

// ProviderAccountConfig holds per-profile account settings for any provider.
type ProviderAccountConfig struct {
	Username   string `yaml:"username" json:"username"`
	Account    string `yaml:"account,omitempty" json:"account,omitempty"`
	TokenPath  string `yaml:"token_path,omitempty" json:"token_path,omitempty"`
	AuthMethod string `yaml:"auth_method,omitempty" json:"auth_method,omitempty"`
	UploadKeys *bool  `yaml:"upload_keys,omitempty" json:"upload_keys,omitempty"`
}

// Metadata holds profile lifecycle data.
type Metadata struct {
	Created    time.Time  `yaml:"created" json:"created"`
	Updated    time.Time  `yaml:"updated" json:"updated"`
	UsageCount int64      `yaml:"usage_count" json:"usage_count"`
	LastUsed   *time.Time `yaml:"last_used,omitempty" json:"last_used,omitempty"`
	Version    string     `yaml:"version" json:"version"`
}

// ActivationScope represents where a profile is activated.
type ActivationScope int

const (
	ScopeSession ActivationScope = iota
	ScopeGlobal
	ScopeLocal
)

func (s ActivationScope) String() string {
	switch s {
	case ScopeSession:
		return "session"
	case ScopeGlobal:
		return "global"
	case ScopeLocal:
		return "local"
	default:
		return "unknown"
	}
}

// BoolPtr returns a pointer to a bool.
func BoolPtr(b bool) *bool {
	return &b
}
