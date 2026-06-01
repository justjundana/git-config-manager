package profile

import (
	"testing"

	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

func TestSetProviderAccountEnforcesSingleProviderAndLegacyGitHubSync(t *testing.T) {
	p := &Profile{
		GitHub: &GitHubConfig{Username: "old-gh", TokenPath: "legacy-token"},
		Providers: map[string]ProviderAccountConfig{
			string(providerpkg.GitHubID): {Username: "old-gh", TokenPath: "provider-token"},
			string(providerpkg.GitLabID): {Username: "old-gl"},
		},
	}

	SetProviderAccount(p, providerpkg.GitLabID, "new-gl", providerpkg.AuthMethodPAT)

	if HasMultipleProviders(p) {
		t.Fatal("profile should have exactly one provider after SetProviderAccount")
	}
	if p.GitHub != nil {
		t.Fatal("legacy GitHub block should be cleared when switching to GitLab")
	}
	if got, ok := ProviderID(p); !ok || got != providerpkg.GitLabID {
		t.Fatalf("ProviderID = %q, %v; want gitlab, true", got, ok)
	}
	account := ProviderAccount(p, providerpkg.GitLabID)
	if account.Username != "new-gl" || account.AuthMethod != providerpkg.AuthMethodPAT {
		t.Fatalf("GitLab account = %+v", account)
	}
}

func TestSetProviderAccountNilAndGitHubCreation(t *testing.T) {
	SetProviderAccount(nil, providerpkg.GitHubID, "ignored", providerpkg.AuthMethodPAT)

	p := &Profile{}
	SetProviderAccount(p, providerpkg.GitHubID, "octo", providerpkg.AuthMethodPAT)
	if p.GitHub == nil || p.GitHub.Username != "octo" {
		t.Fatalf("GitHub config = %+v", p.GitHub)
	}
	if got, ok := ProviderID(p); !ok || got != providerpkg.GitHubID {
		t.Fatalf("ProviderID = %q, %v; want github, true", got, ok)
	}
}

func TestSetProviderAccountPreservesSameProviderMetadata(t *testing.T) {
	uploadKeys := true
	p := &Profile{Providers: map[string]ProviderAccountConfig{
		string(providerpkg.GitLabID): {
			Username:   "old-gl",
			Account:    "self-managed",
			TokenPath:  "tokens/work/gitlab",
			UploadKeys: &uploadKeys,
		},
	}}

	SetProviderAccount(p, providerpkg.GitLabID, "new-gl", providerpkg.AuthMethodPAT)

	account := ProviderAccount(p, providerpkg.GitLabID)
	if account.Username != "new-gl" {
		t.Fatalf("Username = %q, want updated username", account.Username)
	}
	if account.Account != "self-managed" || account.TokenPath != "tokens/work/gitlab" || account.UploadKeys != &uploadKeys {
		t.Fatalf("provider metadata was not preserved: %+v", account)
	}
}

func TestProviderAccountFallsBackToLegacyGitHubBlock(t *testing.T) {
	uploadKeys := false
	p := &Profile{GitHub: &GitHubConfig{Username: "octo", TokenPath: "legacy", UploadKeys: &uploadKeys}}

	account := ProviderAccount(p, providerpkg.GitHubID)

	if account.Username != "octo" || account.TokenPath != "legacy" || account.AuthMethod != providerpkg.AuthMethodLegacy {
		t.Fatalf("legacy account = %+v", account)
	}
	if account.UploadKeys != &uploadKeys {
		t.Fatal("legacy upload_keys pointer should be preserved")
	}
}

func TestProviderAccountEmptyBranches(t *testing.T) {
	if got := ProviderAccount(nil, providerpkg.GitHubID); got != (ProviderAccountConfig{}) {
		t.Fatalf("ProviderAccount(nil) = %+v", got)
	}
	if got := ProviderAccount(&Profile{}, providerpkg.GitLabID); got != (ProviderAccountConfig{}) {
		t.Fatalf("ProviderAccount(empty) = %+v", got)
	}
}

func TestHasMultipleProvidersDetectsMixedLegacyAndProviderState(t *testing.T) {
	cases := []struct {
		name string
		p    *Profile
		want bool
	}{
		{
			name: "two provider map entries",
			p: &Profile{Providers: map[string]ProviderAccountConfig{
				string(providerpkg.GitHubID): {},
				string(providerpkg.GitLabID): {},
			}},
			want: true,
		},
		{
			name: "gitlab provider plus legacy github",
			p: &Profile{
				Providers: map[string]ProviderAccountConfig{string(providerpkg.GitLabID): {}},
				GitHub:    &GitHubConfig{Username: "octo"},
			},
			want: true,
		},
		{
			name: "github provider plus legacy github compatibility",
			p: &Profile{
				Providers: map[string]ProviderAccountConfig{string(providerpkg.GitHubID): {}},
				GitHub:    &GitHubConfig{Username: "octo"},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasMultipleProviders(tc.p); got != tc.want {
				t.Fatalf("HasMultipleProviders = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClearProviderAccounts(t *testing.T) {
	ClearProviderAccounts(nil)

	p := &Profile{
		Providers: map[string]ProviderAccountConfig{string(providerpkg.GitLabID): {Username: "gl"}},
		GitHub:    &GitHubConfig{Username: "gh"},
	}

	ClearProviderAccounts(p)

	if p.Providers != nil || p.GitHub != nil {
		t.Fatalf("provider accounts not cleared: providers=%v github=%v", p.Providers, p.GitHub)
	}
}

func TestClearProviderAccountRemovesOnlyRequestedProvider(t *testing.T) {
	ClearProviderAccount(nil, providerpkg.GitHubID)

	p := &Profile{Providers: map[string]ProviderAccountConfig{
		string(providerpkg.GitHubID): {Username: "gh"},
		string(providerpkg.GitLabID): {Username: "gl"},
	}}

	ClearProviderAccount(p, providerpkg.GitLabID)

	if _, ok := p.Providers[string(providerpkg.GitLabID)]; ok {
		t.Fatal("GitLab provider should be removed")
	}
	if got := p.Providers[string(providerpkg.GitHubID)].Username; got != "gh" {
		t.Fatalf("GitHub provider = %q, want preserved", got)
	}

	ClearProviderAccount(p, providerpkg.GitHubID)
	if p.Providers != nil {
		t.Fatalf("providers map = %v, want nil after last provider removal", p.Providers)
	}

	p = &Profile{GitHub: &GitHubConfig{Username: "gh"}}
	ClearProviderAccount(p, providerpkg.GitHubID)
	if p.GitHub != nil {
		t.Fatalf("legacy GitHub config should be cleared: %+v", p.GitHub)
	}
}

func TestProviderIDAndUsesProviderEdgeCases(t *testing.T) {
	if id, ok := ProviderID(nil); ok || id != "" {
		t.Fatalf("ProviderID(nil) = %q, %v; want empty, false", id, ok)
	}
	if id, ok := ProviderID(&Profile{}); ok || id != "" {
		t.Fatalf("ProviderID(empty) = %q, %v; want empty, false", id, ok)
	}
	if HasMultipleProviders(nil) {
		t.Fatal("nil profile should not have multiple providers")
	}
	if HasMultipleProviders(&Profile{}) {
		t.Fatal("empty profile should not have multiple providers")
	}

	p := &Profile{Providers: map[string]ProviderAccountConfig{string(providerpkg.GitLabID): {Username: "gl"}}}
	if !UsesProvider(p, providerpkg.GitLabID) {
		t.Fatal("profile should use GitLab")
	}
	if UsesProvider(p, providerpkg.GitHubID) {
		t.Fatal("profile should not use GitHub")
	}

	p.Providers[string(providerpkg.GitHubID)] = ProviderAccountConfig{Username: "gh"}
	if id, ok := ProviderID(p); ok || id != "" {
		t.Fatalf("ProviderID(multiple) = %q, %v; want empty, false", id, ok)
	}

	legacy := &Profile{GitHub: &GitHubConfig{Username: "octo"}}
	if id, ok := ProviderID(legacy); !ok || id != providerpkg.GitHubID {
		t.Fatalf("ProviderID(legacy) = %q, %v; want github, true", id, ok)
	}
}
