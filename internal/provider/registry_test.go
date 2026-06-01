package provider

import (
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
)

func TestRegistry_DefaultProvidersResolveHosts(t *testing.T) {
	cfg := config.DefaultConfig()
	registry := NewRegistry(cfg)

	githubDef, ok := registry.ResolveHost("github.com")
	if !ok {
		t.Fatal("expected github.com to resolve")
	}
	if githubDef.ID != GitHubID {
		t.Fatalf("github.com resolved to %q, want %q", githubDef.ID, GitHubID)
	}

	gitlabDef, ok := registry.ResolveHost("https://gitlab.com")
	if !ok {
		t.Fatal("expected gitlab.com to resolve")
	}
	if gitlabDef.ID != GitLabID {
		t.Fatalf("gitlab.com resolved to %q, want %q", gitlabDef.ID, GitLabID)
	}
}

func TestRegistry_LegacyGitHubCustomAPIURLOverridesDefaultProviderHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.GitHub.APIURL = "https://github.company.test/api/v3"

	registry := NewRegistry(cfg)
	def, ok := registry.Get(GitHubID)
	if !ok {
		t.Fatal("expected GitHub provider")
	}
	if def.WebURL != "https://github.company.test" {
		t.Fatalf("WebURL = %q, want custom enterprise web URL", def.WebURL)
	}
	if def.CredentialServer() != "https://github.company.test" {
		t.Fatalf("CredentialServer = %q", def.CredentialServer())
	}
	resolved, ok := registry.ResolveHost("github.company.test")
	if !ok || resolved.ID != GitHubID {
		t.Fatalf("custom host did not resolve to GitHub")
	}
	if _, ok := registry.ResolveHost("github.com"); ok {
		t.Fatal("default github.com host should not resolve after legacy custom API override")
	}
}

func TestRegistry_ProviderGitHubConfigWinsOverDefaultLegacyBlock(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers["github"] = config.ProviderConfig{
		Type:       "github",
		APIURL:     "https://github.enterprise.test/api/v3",
		WebURL:     "https://github.enterprise.test",
		GitHosts:   []string{"github.enterprise.test"},
		SSHHost:    "github.enterprise.test",
		UploadKeys: true,
	}

	registry := NewRegistry(cfg)
	def, ok := registry.Get(GitHubID)
	if !ok {
		t.Fatal("expected GitHub provider")
	}
	if def.APIURL != "https://github.enterprise.test/api/v3" {
		t.Fatalf("APIURL = %q, want providers.github API URL", def.APIURL)
	}
	if _, ok := registry.ResolveHost("github.enterprise.test"); !ok {
		t.Fatal("providers.github host should resolve")
	}
	if _, ok := registry.ResolveHost("github.com"); ok {
		t.Fatal("default github.com host should not resolve when providers.github is customized")
	}
}

func TestRegistry_AllIsDeterministic(t *testing.T) {
	registry := &Registry{
		providers: make(map[ProviderID]Definition),
		hostIndex: make(map[string]ProviderID),
	}
	registry.Register(Definition{ID: GitLabID})
	registry.Register(Definition{ID: GitHubID})

	defs := registry.All()
	if len(defs) != 2 {
		t.Fatalf("len(All()) = %d, want 2", len(defs))
	}
	if defs[0].ID != GitHubID || defs[1].ID != GitLabID {
		t.Fatalf("All() order = %q, %q", defs[0].ID, defs[1].ID)
	}
}

func TestDefinition_CredentialUsernameStrategies(t *testing.T) {
	gitlab := Definition{ID: GitLabID}
	if got := gitlab.CredentialUsername("work", "jane", TokenSet{AuthMethod: AuthMethodPAT}); got != "jane" {
		t.Fatalf("GitLab PAT username = %q", got)
	}
	if got := gitlab.CredentialUsername("work", "", TokenSet{AuthMethod: AuthMethodPAT}); got != "oauth2" {
		t.Fatalf("GitLab PAT fallback username = %q", got)
	}
	if got := gitlab.CredentialUsername("work", "jane", TokenSet{AuthMethod: AuthMethodOAuthDevice}); got != "oauth2" {
		t.Fatalf("GitLab OAuth username = %q", got)
	}

	github := Definition{ID: GitHubID}
	if got := github.CredentialUsername("work", "octo", TokenSet{}); got != "octo" {
		t.Fatalf("GitHub configured username = %q", got)
	}
	if got := github.CredentialUsername("work", "", TokenSet{}); got != "work" {
		t.Fatalf("GitHub fallback username = %q", got)
	}

	other := Definition{ID: ProviderID("forgejo")}
	if got := other.CredentialUsername("work", "alice", TokenSet{}); got != "alice" {
		t.Fatalf("generic configured username = %q", got)
	}
	if got := other.CredentialUsername("work", "", TokenSet{}); got != "work" {
		t.Fatalf("generic fallback username = %q", got)
	}
}

func TestRegistry_DefaultGitLabIsRegisteredWhenMissing(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Providers, "gitlab")

	registry := NewRegistry(cfg)
	def, ok := registry.Get(GitLabID)
	if !ok {
		t.Fatal("expected default GitLab provider")
	}
	if def.APIURL != "https://gitlab.com/api/v4" || def.WebURL != "https://gitlab.com" {
		t.Fatalf("default GitLab definition = %+v", def)
	}
}

func TestRegistry_LegacyDefaultGitHubFillsMissingHostsFromProviderWebURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers["github"] = config.ProviderConfig{
		Type:       "github",
		APIURL:     "",
		WebURL:     "https://github.example.test",
		UploadKeys: true,
	}

	registry := NewRegistry(cfg)
	def, ok := registry.Get(GitHubID)
	if !ok {
		t.Fatal("expected GitHub provider")
	}
	if len(def.GitHosts) != 1 || def.GitHosts[0] != "github.example.test" {
		t.Fatalf("GitHosts = %#v, want provider web host", def.GitHosts)
	}
	if def.SSHHost != "github.example.test" {
		t.Fatalf("SSHHost = %q, want provider web host", def.SSHHost)
	}
}

func TestRegistry_RegisterEdgeCases(t *testing.T) {
	registry := &Registry{providers: map[ProviderID]Definition{}, hostIndex: map[string]ProviderID{}}
	registry.Register(Definition{})
	if len(registry.providers) != 0 {
		t.Fatal("empty provider ID should be ignored")
	}

	registry.Register(Definition{ID: ProviderID("forgejo"), GitHosts: []string{"old.example.test"}})
	if def, ok := registry.ResolveHost("old.example.test"); !ok || def.DisplayName != "Forgejo" {
		t.Fatalf("default display name not applied: %+v, %v", def, ok)
	}
	registry.Register(Definition{ID: ProviderID("forgejo"), DisplayName: "Forge", GitHosts: []string{"new.example.test"}})
	if _, ok := registry.ResolveHost("old.example.test"); ok {
		t.Fatal("old host index should be removed when provider is replaced")
	}
	if def, ok := registry.ResolveHost("new.example.test"); !ok || def.DisplayName != "Forge" {
		t.Fatalf("new host did not resolve: %+v, %v", def, ok)
	}
}

func TestDefinitionCredentialServerBranches(t *testing.T) {
	if got := (Definition{GitHosts: []string{"git.example.test"}}).CredentialServer(); got != "https://git.example.test" {
		t.Fatalf("CredentialServer host = %q", got)
	}
	if got := (Definition{WebURL: "https://web.example.test"}).CredentialServer(); got != "https://web.example.test" {
		t.Fatalf("CredentialServer web = %q", got)
	}
	if got := (Definition{APIURL: "https://api.example.test"}).CredentialServer(); got != "https://api.example.test" {
		t.Fatalf("CredentialServer api = %q", got)
	}
}

func TestNormalizeHostBranches(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		" HTTPS://GitHub.COM/path ": "github.com",
		"GitLab.COM/":               "gitlab.com",
	}
	for input, want := range cases {
		if got := NormalizeHost(input); got != want {
			t.Fatalf("NormalizeHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDefinitionFromConfigUnknownProviderAndGitHubWebURLFallbacks(t *testing.T) {
	def := definitionFromConfig(ProviderID("forgejo"), config.ProviderConfig{APIURL: "https://code.example.test/api"})
	if def.DisplayName != "" || def.Type != "forgejo" || def.Capabilities != nil {
		t.Fatalf("unknown provider def = %+v", def)
	}
	if got := githubWebURLFromAPI(""); got != "https://github.com" {
		t.Fatalf("empty GitHub API web URL = %q", got)
	}
	if got := githubWebURLFromAPI("https://github.company.test/api"); got != "https://github.company.test" {
		t.Fatalf("enterprise /api web URL = %q", got)
	}
	if got := githubWebURLFromAPI("://bad-url"); got != "://bad-url" {
		t.Fatalf("invalid URL fallback = %q", got)
	}
}

func TestLegacyGitHubConfigOverrideBranches(t *testing.T) {
	if legacyGitHubConfigOverridesProvider(nil) {
		t.Fatal("nil config should not override")
	}
	cfg := config.DefaultConfig()
	cfg.GitHub.APIURL = ""
	if legacyGitHubConfigOverridesProvider(cfg) {
		t.Fatal("empty legacy API should not override")
	}
	cfg = config.DefaultConfig()
	delete(cfg.Providers, "github")
	if !legacyGitHubConfigOverridesProvider(cfg) {
		t.Fatal("missing providers.github should preserve legacy")
	}
	cfg = config.DefaultConfig()
	cfg.GitHub.APIURL = "https://github.company.test/api/v3/"
	cfg.Providers["github"] = config.ProviderConfig{APIURL: "https://api.github.com/"}
	if !legacyGitHubConfigOverridesProvider(cfg) {
		t.Fatal("custom legacy plus default provider should override")
	}
}

func TestTokenSetAndCapabilityHelpers(t *testing.T) {
	now := time.Now()
	if !((TokenSet{ExpiresAt: now.Add(-time.Second)}).Expired(now)) {
		t.Fatal("expired token should report expired")
	}
	if (TokenSet{}).Expired(now) {
		t.Fatal("token without expiry should not expire")
	}
	if (CapabilitySet(nil)).Has(CapabilityPATAuth) {
		t.Fatal("nil capability set should not have capability")
	}
	if !(CapabilitySet{CapabilityPATAuth: true}).Has(CapabilityPATAuth) {
		t.Fatal("capability set should report present capability")
	}
}
