// Package config provides configuration management for GCM.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Test hooks – overridden only in tests to simulate failures in
// os.UserHomeDir and os.Exit.
var (
	userHomeDirFn = os.UserHomeDir
	exitFn        = os.Exit
)

// Config is the root configuration structure for GCM.
type Config struct {
	DefaultProfile string                    `yaml:"default_profile,omitempty" json:"default_profile,omitempty"`
	ProfilesDir    string                    `yaml:"profiles_dir" json:"profiles_dir"`
	TemplatesDir   string                    `yaml:"templates_dir" json:"templates_dir"`
	CacheDir       string                    `yaml:"cache_dir" json:"cache_dir"`
	SSHDir         string                    `yaml:"ssh_dir" json:"ssh_dir"`
	GPGHome        string                    `yaml:"gpg_home" json:"gpg_home"`
	AutoSwitch     AutoSwitchConfig          `yaml:"auto_switch" json:"auto_switch"`
	DetectionRules []DetectionRule           `yaml:"detection_rules,omitempty" json:"detection_rules,omitempty"`
	Shell          ShellConfig               `yaml:"shell" json:"shell"`
	GitHub         GitHubAppConfig           `yaml:"github" json:"github"`
	Providers      map[string]ProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
	Backup         BackupConfig              `yaml:"backup" json:"backup"`
	Security       SecurityConfig            `yaml:"security" json:"security"`
	UI             UIConfig                  `yaml:"ui" json:"ui"`
	Advanced       AdvancedConfig            `yaml:"advanced" json:"advanced"`
}

// AutoSwitchConfig controls auto-switching behavior.
type AutoSwitchConfig struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	ProjectFile       string `yaml:"project_file" json:"project_file"`
	DetectionStrategy string `yaml:"detection_strategy" json:"detection_strategy"`
}

// DetectionRule maps a URL pattern to a profile.
type DetectionRule struct {
	Pattern  string `yaml:"pattern" json:"pattern"`
	Profile  string `yaml:"profile" json:"profile"`
	Priority int    `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// ShellConfig controls shell integration.
type ShellConfig struct {
	Integration     bool   `yaml:"integration" json:"integration"`
	PromptIndicator bool   `yaml:"prompt_indicator" json:"prompt_indicator"`
	PromptFormat    string `yaml:"prompt_format" json:"prompt_format"`
	Completion      bool   `yaml:"completion" json:"completion"`
	AutoDetect      bool   `yaml:"auto_detect" json:"auto_detect"`
}

// GitHubAppConfig controls GitHub integration.
type GitHubAppConfig struct {
	APIURL     string      `yaml:"api_url" json:"api_url"`
	UploadKeys bool        `yaml:"upload_keys" json:"upload_keys"`
	OAuth      OAuthConfig `yaml:"oauth" json:"oauth"`
}

// ProviderConfig controls a single Git hosting provider integration.
type ProviderConfig struct {
	Type       string             `yaml:"type" json:"type"`
	APIURL     string             `yaml:"api_url" json:"api_url"`
	WebURL     string             `yaml:"web_url" json:"web_url"`
	GitHosts   []string           `yaml:"git_hosts" json:"git_hosts"`
	SSHHost    string             `yaml:"ssh_host,omitempty" json:"ssh_host,omitempty"`
	SSHPort    int                `yaml:"ssh_port,omitempty" json:"ssh_port,omitempty"`
	UploadKeys bool               `yaml:"upload_keys" json:"upload_keys"`
	Auth       ProviderAuthConfig `yaml:"auth" json:"auth"`
	OAuth      OAuthConfig        `yaml:"oauth" json:"oauth"`
}

// ProviderAuthConfig controls provider auth defaults.
type ProviderAuthConfig struct {
	DefaultMethod string   `yaml:"default_method" json:"default_method"`
	Scopes        []string `yaml:"scopes" json:"scopes"`
}

// OAuthConfig holds OAuth application details.
type OAuthConfig struct {
	ClientID string   `yaml:"client_id" json:"client_id"`
	Scopes   []string `yaml:"scopes" json:"scopes"`
}

// BackupConfig controls backup behavior.
type BackupConfig struct {
	AutoBackup    bool   `yaml:"auto_backup" json:"auto_backup"`
	Interval      string `yaml:"interval" json:"interval"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
	MaxBackups    int    `yaml:"max_backups" json:"max_backups"`
	IncludeKeys   bool   `yaml:"include_keys" json:"include_keys"`
	Encryption    bool   `yaml:"encryption" json:"encryption"`
}

// SecurityConfig controls security behavior.
type SecurityConfig struct {
	EncryptTokens        bool `yaml:"encrypt_tokens" json:"encrypt_tokens"`
	UseKeychain          bool `yaml:"use_keychain" json:"use_keychain"`
	MasterPassword       bool `yaml:"master_password" json:"master_password"`
	AllowPlaintextTokens bool `yaml:"allow_plaintext_tokens" json:"allow_plaintext_tokens"`
	AuditLog             bool `yaml:"audit_log" json:"audit_log"`
}

// UIConfig controls CLI output.
type UIConfig struct {
	Color   bool `yaml:"color" json:"color"`
	Emoji   bool `yaml:"emoji" json:"emoji"`
	Verbose bool `yaml:"verbose" json:"verbose"`
	Quiet   bool `yaml:"quiet" json:"quiet"`
}

// AdvancedConfig holds advanced options.
type AdvancedConfig struct {
	GitCommand         string `yaml:"git_command" json:"git_command"`
	SSHCommand         string `yaml:"ssh_command" json:"ssh_command"`
	GPGCommand         string `yaml:"gpg_command" json:"gpg_command"`
	ParallelOperations bool   `yaml:"parallel_operations" json:"parallel_operations"`
}

// GCMDir returns the GCM data directory (~/.gcm).
// It terminates the process if the home directory cannot be determined,
// because continuing would create sensitive files in the current directory.
func GCMDir() string {
	homeDir, err := userHomeDirFn()
	if err != nil || homeDir == "" {
		fmt.Fprintf(os.Stderr, "fatal: cannot determine home directory: %v\n", err)
		exitFn(1)
	}
	return filepath.Join(homeDir, ".gcm")
}

// DefaultConfig returns the default GCM configuration.
func DefaultConfig() *Config {
	homeDir, err := userHomeDirFn()
	if err != nil || homeDir == "" {
		fmt.Fprintf(os.Stderr, "fatal: cannot determine home directory: %v\n", err)
		exitFn(1)
	}
	gcmDir := filepath.Join(homeDir, ".gcm")

	return &Config{
		ProfilesDir:  filepath.Join(gcmDir, "profiles"),
		TemplatesDir: filepath.Join(gcmDir, "templates"),
		CacheDir:     filepath.Join(gcmDir, "cache"),
		SSHDir:       filepath.Join(homeDir, ".ssh"),
		GPGHome:      filepath.Join(homeDir, ".gnupg"),
		AutoSwitch: AutoSwitchConfig{
			Enabled:           true,
			ProjectFile:       ".gcm-profile",
			DetectionStrategy: "project_file",
		},
		Shell: ShellConfig{
			Integration:     true,
			PromptIndicator: true,
			PromptFormat:    "(%s)",
			Completion:      true,
			AutoDetect:      true,
		},
		GitHub: GitHubAppConfig{
			APIURL:     "https://api.github.com",
			UploadKeys: true,
			OAuth: OAuthConfig{
				ClientID: "gcm-oauth-app",
				Scopes:   []string{"repo", "admin:public_key", "admin:gpg_key"},
			},
		},
		Providers: map[string]ProviderConfig{
			"github": {
				Type:       "github",
				APIURL:     "https://api.github.com",
				WebURL:     "https://github.com",
				GitHosts:   []string{"github.com"},
				SSHHost:    "github.com",
				UploadKeys: true,
				Auth: ProviderAuthConfig{
					DefaultMethod: "pat",
					Scopes:        []string{"repo", "admin:public_key", "admin:gpg_key"},
				},
				OAuth: OAuthConfig{
					ClientID: "gcm-oauth-app",
					Scopes:   []string{"repo", "admin:public_key", "admin:gpg_key"},
				},
			},
			"gitlab": {
				Type:       "gitlab",
				APIURL:     "https://gitlab.com/api/v4",
				WebURL:     "https://gitlab.com",
				GitHosts:   []string{"gitlab.com"},
				SSHHost:    "gitlab.com",
				UploadKeys: true,
				Auth: ProviderAuthConfig{
					DefaultMethod: "pat",
					Scopes:        []string{"api", "read_user", "read_repository", "write_repository"},
				},
			},
		},
		Backup: BackupConfig{
			AutoBackup:    false,
			Interval:      "daily",
			RetentionDays: 30,
			MaxBackups:    10,
			IncludeKeys:   false,
			Encryption:    false,
		},
		Security: SecurityConfig{
			EncryptTokens:        true,
			UseKeychain:          true,
			MasterPassword:       false,
			AllowPlaintextTokens: false,
			AuditLog:             true,
		},
		UI: UIConfig{
			Color:   true,
			Emoji:   true,
			Verbose: false,
			Quiet:   false,
		},
		Advanced: AdvancedConfig{
			GitCommand:         "git",
			SSHCommand:         "ssh",
			GPGCommand:         "gpg",
			ParallelOperations: true,
		},
	}
}
