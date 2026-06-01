package tokenstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/provider"
)

func TestTokenStore_ProviderTokenSet_SaveLoadDelete(t *testing.T) {
	ts := newPlainStore(t)
	key := provider.TokenKey{Profile: "work", Provider: provider.GitLabID, Host: "gitlab.com"}
	token := provider.TokenSet{AccessToken: "glpat-secret", AuthMethod: provider.AuthMethodPAT, Scopes: []string{"api"}}

	if err := ts.SaveTokenSet(key, token); err != nil {
		t.Fatalf("SaveTokenSet: %v", err)
	}
	loaded, err := ts.LoadTokenSet(key)
	if err != nil {
		t.Fatalf("LoadTokenSet: %v", err)
	}
	if loaded.AccessToken != token.AccessToken {
		t.Fatalf("AccessToken = %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.AuthMethod != provider.AuthMethodPAT {
		t.Fatalf("AuthMethod = %q", loaded.AuthMethod)
	}
	if len(loaded.Scopes) != 1 || loaded.Scopes[0] != "api" {
		t.Fatalf("Scopes = %#v", loaded.Scopes)
	}

	if err := ts.DeleteTokenSet(key); err != nil {
		t.Fatalf("DeleteTokenSet: %v", err)
	}
	if _, err := ts.LoadTokenSet(key); err == nil {
		t.Fatal("expected error after DeleteTokenSet")
	}
}

func TestTokenStore_ProviderTokenSet_DoesNotCollideAcrossProviders(t *testing.T) {
	ts := newPlainStore(t)
	githubKey := provider.TokenKey{Profile: "work", Provider: provider.GitHubID, Host: "github.com"}
	gitlabKey := provider.TokenKey{Profile: "work", Provider: provider.GitLabID, Host: "gitlab.com"}

	if err := ts.SaveTokenSet(githubKey, provider.TokenSet{AccessToken: "gh-token", AuthMethod: provider.AuthMethodPAT}); err != nil {
		t.Fatalf("SaveTokenSet github: %v", err)
	}
	if err := ts.SaveTokenSet(gitlabKey, provider.TokenSet{AccessToken: "gl-token", AuthMethod: provider.AuthMethodPAT}); err != nil {
		t.Fatalf("SaveTokenSet gitlab: %v", err)
	}

	githubToken, err := ts.LoadTokenSet(githubKey)
	if err != nil {
		t.Fatalf("LoadTokenSet github: %v", err)
	}
	gitlabToken, err := ts.LoadTokenSet(gitlabKey)
	if err != nil {
		t.Fatalf("LoadTokenSet gitlab: %v", err)
	}
	if githubToken.AccessToken != "gh-token" || gitlabToken.AccessToken != "gl-token" {
		t.Fatalf("tokens collided: github=%q gitlab=%q", githubToken.AccessToken, gitlabToken.AccessToken)
	}
}

func TestTokenStore_ProviderTokenSet_FallsBackToLegacyGitHubToken(t *testing.T) {
	ts := newPlainStore(t)
	if err := ts.Save("work", "legacy-gh-token"); err != nil {
		t.Fatalf("Save legacy token: %v", err)
	}

	loaded, err := ts.LoadTokenSet(provider.TokenKey{Profile: "work", Provider: provider.GitHubID, Host: "github.com"})
	if err != nil {
		t.Fatalf("LoadTokenSet fallback: %v", err)
	}
	if loaded.AccessToken != "legacy-gh-token" {
		t.Fatalf("AccessToken = %q", loaded.AccessToken)
	}
	if loaded.AuthMethod != provider.AuthMethodLegacy {
		t.Fatalf("AuthMethod = %q", loaded.AuthMethod)
	}
}

func TestTokenStore_ProviderTokenSet_RequiresProvider(t *testing.T) {
	ts := newPlainStore(t)
	err := ts.SaveTokenSet(provider.TokenKey{Profile: "work"}, provider.TokenSet{AccessToken: "tok"})
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestTokenStore_ProviderTokenSet_SaveValidationAndMarshalError(t *testing.T) {
	ts := newPlainStore(t)
	key := provider.TokenKey{Profile: "work", Provider: provider.GitLabID}
	if err := ts.SaveTokenSet(key, provider.TokenSet{}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty access token error = %v", err)
	}
	if err := ts.SaveTokenSet(provider.TokenKey{Provider: provider.GitLabID}, provider.TokenSet{AccessToken: "tok"}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("empty profile error = %v", err)
	}

	originalMarshal := marshalTokenSet
	marshalTokenSet = func(v any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { marshalTokenSet = originalMarshal })
	if err := ts.SaveTokenSet(key, provider.TokenSet{AccessToken: "tok", CreatedAt: time.Now().UTC()}); err == nil || !strings.Contains(err.Error(), "marshaling") {
		t.Fatalf("marshal error = %v", err)
	}
}

func TestTokenStore_ProviderTokenSet_LoadBranches(t *testing.T) {
	ts := newPlainStore(t)
	key := provider.TokenKey{Profile: "work", Provider: provider.GitLabID, Host: "gitlab.com"}
	storageKey, err := providerTokenStorageKey(key)
	if err != nil {
		t.Fatalf("providerTokenStorageKey: %v", err)
	}

	if _, err := ts.LoadTokenSet(provider.TokenKey{Provider: provider.GitLabID}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("empty profile load error = %v", err)
	}
	if _, err := ts.LoadTokenSet(key); err == nil {
		t.Fatal("expected missing provider token error")
	}
	if err := ts.saveTokenValue(storageKey, "raw-token"); err != nil {
		t.Fatalf("save raw token: %v", err)
	}
	loaded, err := ts.LoadTokenSet(key)
	if err != nil {
		t.Fatalf("LoadTokenSet raw: %v", err)
	}
	if loaded.AccessToken != "raw-token" || loaded.AuthMethod != provider.AuthMethodLegacy {
		t.Fatalf("raw loaded token = %+v", loaded)
	}
	if err := ts.saveTokenValue(storageKey, "{"); err != nil {
		t.Fatalf("save malformed token: %v", err)
	}
	if _, err := ts.LoadTokenSet(key); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("malformed token error = %v", err)
	}
	if err := ts.saveTokenValue(storageKey, `{"access_token":""}`); err != nil {
		t.Fatalf("save empty token payload: %v", err)
	}
	if _, err := ts.LoadTokenSet(key); err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("empty token payload error = %v", err)
	}
}

func TestTokenStore_ProviderTokenSet_DeleteBranches(t *testing.T) {
	ts := newPlainStore(t)
	if err := ts.DeleteTokenSet(provider.TokenKey{Provider: provider.GitLabID}); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatalf("empty profile delete error = %v", err)
	}

	key := provider.TokenKey{Profile: "work", Provider: provider.GitLabID, Host: "gitlab.com"}
	storageKey, err := providerTokenStorageKey(key)
	if err != nil {
		t.Fatalf("providerTokenStorageKey: %v", err)
	}
	tokenPath, err := sanitizeTokenPath(storageKey)
	if err != nil {
		t.Fatalf("sanitizeTokenPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tokenPath, "nested"), 0o700); err != nil {
		t.Fatalf("mkdir token path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tokenPath, "nested", "file"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write nested token file: %v", err)
	}
	if err := ts.DeleteTokenSet(key); err == nil || !strings.Contains(err.Error(), "deleting") {
		t.Fatalf("delete provider token error = %v", err)
	}

	githubKey := provider.TokenKey{Profile: "work", Provider: provider.GitHubID, Host: "github.com"}
	if err := ts.SaveTokenSet(githubKey, provider.TokenSet{AccessToken: "provider-token"}); err != nil {
		t.Fatalf("SaveTokenSet github: %v", err)
	}
	if err := ts.Save("work", "legacy-token"); err != nil {
		t.Fatalf("Save legacy: %v", err)
	}
	if err := ts.DeleteTokenSet(githubKey); err != nil {
		t.Fatalf("DeleteTokenSet github: %v", err)
	}
	if _, err := ts.Load("work"); err == nil {
		t.Fatal("expected legacy token to be deleted")
	}

	if err := ts.DeleteTokenSet(provider.TokenKey{Profile: "work/slash", Provider: provider.GitHubID, Host: "github.com"}); err == nil || !strings.Contains(err.Error(), "invalid profile") {
		t.Fatalf("legacy delete error = %v", err)
	}
}

func TestProviderTokenStorageKeyAndSafeComponentBranches(t *testing.T) {
	key, err := providerTokenStorageKey(provider.TokenKey{Profile: " Work ", Provider: provider.GitLabID})
	if err != nil {
		t.Fatalf("providerTokenStorageKey default host/account: %v", err)
	}
	if key != "work__gitlab__default__default" {
		t.Fatalf("storage key = %q", key)
	}
	key, err = providerTokenStorageKey(provider.TokenKey{Profile: "Wörk", Provider: provider.GitLabID, Host: "GitLab.COM", Account: "Team_1"})
	if err != nil {
		t.Fatalf("providerTokenStorageKey unicode: %v", err)
	}
	if key != "wörk__gitlab__gitlab_com__team_1" {
		t.Fatalf("unicode storage key = %q", key)
	}
	if _, err := providerTokenStorageKey(provider.TokenKey{Profile: "$$$", Provider: provider.GitLabID}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid component error = %v", err)
	}
	if safeTokenComponent(" --A_b.9-- ") != "--a_b_9--" {
		t.Fatalf("safeTokenComponent punctuation branch failed")
	}
}
