package provider

import (
	"net/url"
	"sort"
	"strings"

	"github.com/justjundana/git-config-manager/internal/config"
)

const defaultGitHubAPIURL = "https://api.github.com"

// Definition is the runtime description of one configured provider.
type Definition struct {
	ID                ProviderID
	Type              string
	DisplayName       string
	APIURL            string
	WebURL            string
	GitHosts          []string
	SSHHost           string
	SSHPort           int
	UploadKeys        bool
	DefaultAuthMethod string
	Scopes            []string
	Capabilities      CapabilitySet
}

// Registry resolves providers by ID and by Git credential host.
type Registry struct {
	providers map[ProviderID]Definition
	hostIndex map[string]ProviderID
}

// NewRegistry builds a registry from config, preserving legacy github config.
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		providers: make(map[ProviderID]Definition),
		hostIndex: make(map[string]ProviderID),
	}

	for id, providerCfg := range cfg.Providers {
		r.Register(definitionFromConfig(ProviderID(id), providerCfg))
	}

	// Backward compatibility: keep a customized legacy github block authoritative
	// when providers.github is still the default entry inherited from defaults.
	githubDef := r.providers[GitHubID]
	if legacyGitHubConfigOverridesProvider(cfg) {
		githubDef.ID = GitHubID
		githubDef.Type = "github"
		githubDef.DisplayName = "GitHub"
		githubDef.APIURL = cfg.GitHub.APIURL
		githubDef.UploadKeys = cfg.GitHub.UploadKeys
		githubDef.DefaultAuthMethod = AuthMethodPAT
		githubDef.Scopes = append([]string(nil), cfg.GitHub.OAuth.Scopes...)
		githubDef.Capabilities = GitHubCapabilities()
		if cfg.GitHub.APIURL != defaultGitHubAPIURL || githubDef.WebURL == "" {
			githubDef.WebURL = githubWebURLFromAPI(cfg.GitHub.APIURL)
			githubDef.GitHosts = []string{NormalizeHost(githubDef.WebURL)}
			githubDef.SSHHost = NormalizeHost(githubDef.WebURL)
		} else {
			if len(githubDef.GitHosts) == 0 {
				githubDef.GitHosts = []string{NormalizeHost(githubDef.WebURL)}
			}
			if githubDef.SSHHost == "" {
				githubDef.SSHHost = NormalizeHost(githubDef.WebURL)
			}
		}
		r.Register(githubDef)
	}

	if _, ok := r.providers[GitLabID]; !ok {
		r.Register(definitionFromConfig(GitLabID, defaultGitLabConfig()))
	}

	return r
}

func legacyGitHubConfigOverridesProvider(cfg *config.Config) bool {
	if cfg == nil || cfg.GitHub.APIURL == "" {
		return false
	}
	providerCfg, hasProviderConfig := cfg.Providers[string(GitHubID)]
	if !hasProviderConfig || providerCfg.APIURL == "" {
		return true
	}
	legacyAPI := strings.TrimRight(cfg.GitHub.APIURL, "/")
	providerAPI := strings.TrimRight(providerCfg.APIURL, "/")
	return legacyAPI != defaultGitHubAPIURL && providerAPI == defaultGitHubAPIURL
}

// Register adds or replaces a provider definition.
func (r *Registry) Register(def Definition) {
	if def.ID == "" {
		return
	}
	if def.DisplayName == "" {
		def.DisplayName = strings.Title(string(def.ID))
	}
	def.APIURL = strings.TrimRight(def.APIURL, "/")
	def.WebURL = strings.TrimRight(def.WebURL, "/")
	for i, host := range def.GitHosts {
		def.GitHosts[i] = NormalizeHost(host)
	}
	for host, id := range r.hostIndex {
		if id == def.ID {
			delete(r.hostIndex, host)
		}
	}
	r.providers[def.ID] = def
	for _, host := range def.GitHosts {
		if host != "" {
			r.hostIndex[host] = def.ID
		}
	}
}

// Get returns a provider definition by ID.
func (r *Registry) Get(id ProviderID) (Definition, bool) {
	def, ok := r.providers[id]
	return def, ok
}

// ResolveHost returns the provider responsible for a Git credential host.
func (r *Registry) ResolveHost(host string) (Definition, bool) {
	id, ok := r.hostIndex[NormalizeHost(host)]
	if !ok {
		return Definition{}, false
	}
	return r.Get(id)
}

// All returns all registered definitions.
func (r *Registry) All() []Definition {
	defs := make([]Definition, 0, len(r.providers))
	for _, def := range r.providers {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].ID < defs[j].ID
	})
	return defs
}

// CredentialServer returns the git credential config URL for this provider.
func (d Definition) CredentialServer() string {
	if len(d.GitHosts) > 0 && d.GitHosts[0] != "" {
		return "https://" + d.GitHosts[0]
	}
	if d.WebURL != "" {
		return d.WebURL
	}
	return d.APIURL
}

// CredentialUsername returns the username git should pair with the token.
func (d Definition) CredentialUsername(profileName, username string, token TokenSet) string {
	switch ProviderID(d.ID) {
	case GitLabID:
		if token.Bearer() {
			return "oauth2"
		}
		if username != "" {
			return username
		}
		return "oauth2"
	case GitHubID:
		if username != "" {
			return username
		}
		return profileName
	default:
		if username != "" {
			return username
		}
		return profileName
	}
}

// NormalizeHost normalizes a Git host or URL into the credential host key.
func NormalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	}
	return strings.ToLower(strings.TrimSuffix(value, "/"))
}

// GitHubCapabilities returns the capability set for the GitHub adapter.
func GitHubCapabilities() CapabilitySet {
	return CapabilitySet{
		CapabilityPATAuth:          true,
		CapabilityOAuthDeviceAuth:  true,
		CapabilityCredentialHelper: true,
		CapabilitySSHKeys:          true,
		CapabilityGPGKeys:          true,
	}
}

// GitLabCapabilities returns the capability set for the GitLab adapter.
func GitLabCapabilities() CapabilitySet {
	return CapabilitySet{
		CapabilityPATAuth:          true,
		CapabilityCredentialHelper: true,
		CapabilitySSHKeys:          true,
		CapabilityGPGKeys:          true,
	}
}

func definitionFromConfig(id ProviderID, providerCfg config.ProviderConfig) Definition {
	def := Definition{
		ID:                id,
		Type:              providerCfg.Type,
		APIURL:            providerCfg.APIURL,
		WebURL:            providerCfg.WebURL,
		GitHosts:          append([]string(nil), providerCfg.GitHosts...),
		SSHHost:           providerCfg.SSHHost,
		SSHPort:           providerCfg.SSHPort,
		UploadKeys:        providerCfg.UploadKeys,
		DefaultAuthMethod: providerCfg.Auth.DefaultMethod,
		Scopes:            append([]string(nil), providerCfg.Auth.Scopes...),
	}
	if def.Type == "" {
		def.Type = string(id)
	}
	switch id {
	case GitHubID:
		def.DisplayName = "GitHub"
		def.Capabilities = GitHubCapabilities()
	case GitLabID:
		def.DisplayName = "GitLab"
		def.Capabilities = GitLabCapabilities()
	}
	return def
}

func defaultGitLabConfig() config.ProviderConfig {
	return config.ProviderConfig{
		Type:       "gitlab",
		APIURL:     "https://gitlab.com/api/v4",
		WebURL:     "https://gitlab.com",
		GitHosts:   []string{"gitlab.com"},
		SSHHost:    "gitlab.com",
		UploadKeys: true,
		Auth: config.ProviderAuthConfig{
			DefaultMethod: AuthMethodPAT,
			Scopes:        []string{"api", "read_user", "read_repository", "write_repository"},
		},
	}
}

func githubWebURLFromAPI(apiURL string) string {
	if apiURL == "" || apiURL == "https://api.github.com" {
		return "https://github.com"
	}
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Host == "" {
		return apiURL
	}
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/api/v3"), "/api")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}
