package profile

import providerpkg "github.com/justjundana/git-config-manager/internal/provider"

// ProviderAccount returns the account metadata for a provider, including the
// legacy GitHub field for backward-compatible profiles.
func ProviderAccount(p *Profile, id providerpkg.ProviderID) ProviderAccountConfig {
	if p != nil && p.Providers != nil {
		if account, ok := p.Providers[string(id)]; ok {
			return account
		}
	}
	if id == providerpkg.GitHubID && p != nil && p.GitHub != nil {
		return ProviderAccountConfig{
			Username:   p.GitHub.Username,
			TokenPath:  p.GitHub.TokenPath,
			AuthMethod: providerpkg.AuthMethodLegacy,
			UploadKeys: p.GitHub.UploadKeys,
		}
	}
	return ProviderAccountConfig{}
}

// SetProviderAccount enforces the one-provider-per-profile invariant while
// preserving backward compatibility for the legacy GitHub profile block.
func SetProviderAccount(p *Profile, id providerpkg.ProviderID, username, authMethod string) {
	if p == nil {
		return
	}
	previous := ProviderAccount(p, id)
	p.Providers = make(map[string]ProviderAccountConfig)
	account := previous
	account.Username = username
	account.AuthMethod = authMethod
	p.Providers[string(id)] = account

	if id == providerpkg.GitHubID {
		if p.GitHub == nil {
			p.GitHub = &GitHubConfig{}
		}
		p.GitHub.Username = username
	} else {
		p.GitHub = nil
	}
}

// ClearProviderAccount removes one provider account from the profile.
func ClearProviderAccount(p *Profile, id providerpkg.ProviderID) {
	if p == nil {
		return
	}
	if p.Providers != nil {
		delete(p.Providers, string(id))
		if len(p.Providers) == 0 {
			p.Providers = nil
		}
	}
	if id == providerpkg.GitHubID {
		p.GitHub = nil
	}
}

// ClearProviderAccounts removes all provider account metadata from the profile.
func ClearProviderAccounts(p *Profile) {
	if p == nil {
		return
	}
	p.Providers = nil
	p.GitHub = nil
}

// ProviderID returns the single provider selected for the profile.
func ProviderID(p *Profile) (providerpkg.ProviderID, bool) {
	if p == nil || HasMultipleProviders(p) {
		return "", false
	}
	if len(p.Providers) == 1 {
		for id := range p.Providers {
			return providerpkg.ProviderID(id), true
		}
	}
	if p.GitHub != nil {
		return providerpkg.GitHubID, true
	}
	return "", false
}

// UsesProvider reports whether the profile is scoped to the given provider.
func UsesProvider(p *Profile, id providerpkg.ProviderID) bool {
	profileProviderID, ok := ProviderID(p)
	return ok && profileProviderID == id
}

// HasMultipleProviders reports invalid mixed provider metadata.
func HasMultipleProviders(p *Profile) bool {
	if p == nil {
		return false
	}
	if len(p.Providers) > 1 {
		return true
	}
	if len(p.Providers) == 1 && p.GitHub != nil {
		_, hasGitHubProvider := p.Providers[string(providerpkg.GitHubID)]
		return !hasGitHubProvider
	}
	return false
}
