package providerclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/github"
	"github.com/justjundana/git-config-manager/internal/gitlab"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func TestRouterVerifyPATRoutesToProviderClient(t *testing.T) {
	log := logger.New(logger.LevelError, io.Discard)
	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("GitHub path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token gh-token" {
			t.Fatalf("GitHub Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "octo", "name": "Octo Cat"})
	}))
	defer githubServer.Close()

	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("GitLab path = %s", r.URL.Path)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "gl-token" {
			t.Fatalf("GitLab PRIVATE-TOKEN = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "lab", "name": "Lab User"})
	}))
	defer gitlabServer.Close()

	cfg := config.DefaultConfig()
	cfg.GitHub.APIURL = githubServer.URL
	router := NewRouter(
		github.NewClient(cfg, log, nil),
		gitlab.NewClient(config.ProviderConfig{APIURL: gitlabServer.URL}, log),
	)

	githubUser, err := router.VerifyPAT(context.Background(), providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}, "gh-token")
	if err != nil {
		t.Fatalf("VerifyPAT GitHub: %v", err)
	}
	if githubUser.Username != "octo" || githubUser.Name != "Octo Cat" {
		t.Fatalf("GitHub user = %+v", githubUser)
	}

	gitlabUser, err := router.VerifyPAT(context.Background(), providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}, "gl-token")
	if err != nil {
		t.Fatalf("VerifyPAT GitLab: %v", err)
	}
	if gitlabUser.Username != "lab" || gitlabUser.Name != "Lab User" {
		t.Fatalf("GitLab user = %+v", gitlabUser)
	}
}

func TestRouterGitHubOperationsUseTokenFromEachCall(t *testing.T) {
	const publicKey = "ssh-ed25519 AAAA comment"
	const keyID = "ABC123"
	assertGitHubToken := func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.Header.Get("Authorization"); got != "token current-token" {
			t.Fatalf("Authorization = %q, want current token", got)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertGitHubToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/keys":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 11, "key": publicKey, "title": "old"}})
		case r.Method == http.MethodPost && r.URL.Path == "/user/keys":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/user/keys/11":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/user/gpg_keys":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 22, "key_id": keyID}})
		case r.Method == http.MethodPost && r.URL.Path == "/user/gpg_keys":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/user/gpg_keys/22":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.GitHub.APIURL = server.URL
	router := NewRouter(github.NewClient(cfg, logger.New(logger.LevelError, io.Discard), nil), nil)
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	oldToken := providerpkg.TokenSet{AccessToken: "old-token", AuthMethod: providerpkg.AuthMethodPAT}
	currentToken := providerpkg.TokenSet{AccessToken: "current-token", AuthMethod: providerpkg.AuthMethodPAT}

	if err := router.SetToken(def, oldToken); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if exists, err := router.SSHKeyExists(context.Background(), def, currentToken, publicKey); err != nil || !exists {
		t.Fatalf("SSHKeyExists = %v, %v; want true, nil", exists, err)
	}
	if err := router.UploadSSHKey(context.Background(), def, currentToken, "title", publicKey); err != nil {
		t.Fatalf("UploadSSHKey: %v", err)
	}
	if deleted, err := router.DeleteSSHKey(context.Background(), def, currentToken, publicKey); err != nil || !deleted {
		t.Fatalf("DeleteSSHKey = %v, %v; want true, nil", deleted, err)
	}
	if exists, err := router.GPGKeyExists(context.Background(), def, currentToken, keyID); err != nil || !exists {
		t.Fatalf("GPGKeyExists = %v, %v; want true, nil", exists, err)
	}
	if err := router.UploadGPGKey(context.Background(), def, currentToken, "armored"); err != nil {
		t.Fatalf("UploadGPGKey: %v", err)
	}
	if deleted, err := router.DeleteGPGKey(context.Background(), def, currentToken, keyID); err != nil || !deleted {
		t.Fatalf("DeleteGPGKey = %v, %v; want true, nil", deleted, err)
	}
}

func TestRouterGitLabOperationsUseStructuredTokenAuth(t *testing.T) {
	const publicKey = "ssh-ed25519 BBBB comment"
	const keyID = "DEF456"
	assertBearer := func(t *testing.T, r *http.Request) {
		t.Helper()
		if got := r.Header.Get("Authorization"); got != "Bearer gl-bearer" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "" {
			t.Fatalf("PRIVATE-TOKEN = %q, want empty for bearer token", got)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearer(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/keys":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 31, "key": publicKey, "title": "old"}})
		case r.Method == http.MethodPost && r.URL.Path == "/user/keys":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/user/keys/31":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/user/gpg_keys":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 41, "key_id": keyID, "fingerprint": "FFFF"}})
		case r.Method == http.MethodPost && r.URL.Path == "/user/gpg_keys":
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/user/gpg_keys/41":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	router := NewRouter(nil, gitlab.NewClient(config.ProviderConfig{APIURL: server.URL}, logger.New(logger.LevelError, io.Discard)))
	def := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}
	token := providerpkg.TokenSet{AccessToken: "gl-bearer", AuthMethod: providerpkg.AuthMethodOAuthDevice}

	if exists, err := router.SSHKeyExists(context.Background(), def, token, publicKey); err != nil || !exists {
		t.Fatalf("SSHKeyExists = %v, %v; want true, nil", exists, err)
	}
	if err := router.UploadSSHKey(context.Background(), def, token, "title", publicKey); err != nil {
		t.Fatalf("UploadSSHKey: %v", err)
	}
	if deleted, err := router.DeleteSSHKey(context.Background(), def, token, publicKey); err != nil || !deleted {
		t.Fatalf("DeleteSSHKey = %v, %v; want true, nil", deleted, err)
	}
	if exists, err := router.GPGKeyExists(context.Background(), def, token, keyID); err != nil || !exists {
		t.Fatalf("GPGKeyExists = %v, %v; want true, nil", exists, err)
	}
	if err := router.UploadGPGKey(context.Background(), def, token, "armored"); err != nil {
		t.Fatalf("UploadGPGKey: %v", err)
	}
	if deleted, err := router.DeleteGPGKey(context.Background(), def, token, keyID); err != nil || !deleted {
		t.Fatalf("DeleteGPGKey = %v, %v; want true, nil", deleted, err)
	}
}

func TestRouterRejectsUnsupportedProviderAndEmptyToken(t *testing.T) {
	router := NewRouter(nil, nil)
	unknownDef := providerpkg.Definition{ID: providerpkg.BitbucketID, DisplayName: "Bitbucket"}
	if _, err := router.VerifyPAT(context.Background(), unknownDef, "token"); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unsupported VerifyPAT error = %v", err)
	}
	if err := router.SetToken(providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}, providerpkg.TokenSet{}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty token error = %v", err)
	}
	if err := router.SetToken(unknownDef, providerpkg.TokenSet{AccessToken: "token"}); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unsupported SetToken error = %v", err)
	}
}

func TestRouterRoutesRegisteredAdapter(t *testing.T) {
	adapter := &stubAdapter{}
	router := NewRouterWithAdapters(map[providerpkg.ProviderID]Adapter{
		providerpkg.BitbucketID: adapter,
	})
	def := providerpkg.Definition{ID: providerpkg.BitbucketID, DisplayName: "Bitbucket"}
	token := providerpkg.TokenSet{AccessToken: "bb-token", AuthMethod: providerpkg.AuthMethodPAT}

	user, err := router.VerifyPAT(context.Background(), def, "bb-pat")
	if err != nil {
		t.Fatalf("VerifyPAT: %v", err)
	}
	if user.Username != "bitbucket-user" || adapter.verifyToken != "bb-pat" {
		t.Fatalf("registered VerifyPAT user=%+v token=%q", user, adapter.verifyToken)
	}
	if exists, err := router.SSHKeyExists(context.Background(), def, token, "ssh-ed25519 AAAA"); err != nil || !exists {
		t.Fatalf("SSHKeyExists = %v, %v; want true, nil", exists, err)
	}
	if adapter.sshToken != "bb-token" {
		t.Fatalf("registered adapter token = %q", adapter.sshToken)
	}

	router.Register(providerpkg.BitbucketID, nil)
	if _, err := router.VerifyPAT(context.Background(), def, "bb-pat"); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("removed adapter error = %v", err)
	}
}

func TestRouterVerifyPATWrapsProviderErrors(t *testing.T) {
	log := logger.New(logger.LevelError, io.Discard)
	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer githubServer.Close()
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer gitlabServer.Close()

	cfg := config.DefaultConfig()
	cfg.GitHub.APIURL = githubServer.URL
	router := NewRouter(
		github.NewClient(cfg, log, nil),
		gitlab.NewClient(config.ProviderConfig{APIURL: gitlabServer.URL}, log),
	)

	if _, err := router.VerifyPAT(context.Background(), providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}, "bad"); err == nil || !strings.Contains(err.Error(), "GitHub token verification failed") {
		t.Fatalf("GitHub VerifyPAT error = %v", err)
	}
	if _, err := router.VerifyPAT(context.Background(), providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}, "bad"); err == nil || !strings.Contains(err.Error(), "GitLab token verification failed") {
		t.Fatalf("GitLab VerifyPAT error = %v", err)
	}
}

func TestRouterReturnsErrorWhenConcreteClientMissing(t *testing.T) {
	router := NewRouter(nil, nil)
	token := providerpkg.TokenSet{AccessToken: "token", AuthMethod: providerpkg.AuthMethodPAT}

	if _, err := router.SSHKeyExists(context.Background(), providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing GitHub client error = %v", err)
	}
	if _, err := router.GPGKeyExists(context.Background(), providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing GitLab client error = %v", err)
	}
}

func TestRouterUnsupportedOperationErrors(t *testing.T) {
	router := NewRouter(nil, nil)
	def := providerpkg.Definition{ID: providerpkg.BitbucketID, DisplayName: "Bitbucket"}
	token := providerpkg.TokenSet{AccessToken: "token", AuthMethod: providerpkg.AuthMethodPAT}
	ctx := context.Background()

	if _, err := router.SSHKeyExists(ctx, def, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "SSH key upload") {
		t.Fatalf("SSHKeyExists unsupported error = %v", err)
	}
	if err := router.UploadSSHKey(ctx, def, token, "title", "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "SSH key upload") {
		t.Fatalf("UploadSSHKey unsupported error = %v", err)
	}
	if _, err := router.DeleteSSHKey(ctx, def, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "SSH key deletion") {
		t.Fatalf("DeleteSSHKey unsupported error = %v", err)
	}
	if _, err := router.GPGKeyExists(ctx, def, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "GPG key upload") {
		t.Fatalf("GPGKeyExists unsupported error = %v", err)
	}
	if err := router.UploadGPGKey(ctx, def, token, "armored"); err == nil || !strings.Contains(err.Error(), "GPG key upload") {
		t.Fatalf("UploadGPGKey unsupported error = %v", err)
	}
	if _, err := router.DeleteGPGKey(ctx, def, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "GPG key deletion") {
		t.Fatalf("DeleteGPGKey unsupported error = %v", err)
	}
}

func TestRouterOperationSetTokenErrors(t *testing.T) {
	router := NewRouter(nil, nil)
	ctx := context.Background()
	githubDef := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	gitlabDef := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}
	token := providerpkg.TokenSet{AccessToken: "token", AuthMethod: providerpkg.AuthMethodPAT}

	if _, err := router.VerifyPAT(ctx, githubDef, "token"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("VerifyPAT missing client error = %v", err)
	}
	if _, err := router.VerifyPAT(ctx, gitlabDef, "token"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("VerifyPAT missing GitLab client error = %v", err)
	}
	if _, err := router.SSHKeyExists(ctx, gitlabDef, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("SSHKeyExists missing GitLab client error = %v", err)
	}
	if err := router.UploadSSHKey(ctx, githubDef, token, "title", "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("UploadSSHKey missing client error = %v", err)
	}
	if err := router.UploadSSHKey(ctx, gitlabDef, token, "title", "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("UploadSSHKey missing GitLab client error = %v", err)
	}
	if _, err := router.DeleteSSHKey(ctx, githubDef, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("DeleteSSHKey missing client error = %v", err)
	}
	if _, err := router.DeleteSSHKey(ctx, gitlabDef, token, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("DeleteSSHKey missing GitLab client error = %v", err)
	}
	if _, err := router.GPGKeyExists(ctx, githubDef, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("GPGKeyExists missing GitHub client error = %v", err)
	}
	if err := router.UploadGPGKey(ctx, githubDef, token, "armored"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("UploadGPGKey missing GitHub client error = %v", err)
	}
	if err := router.UploadGPGKey(ctx, gitlabDef, token, "armored"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("UploadGPGKey missing client error = %v", err)
	}
	if _, err := router.DeleteGPGKey(ctx, githubDef, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("DeleteGPGKey missing GitHub client error = %v", err)
	}
	if _, err := router.DeleteGPGKey(ctx, gitlabDef, token, "ABC123"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("DeleteGPGKey missing client error = %v", err)
	}
}

func TestRouterNilAndAdapterTokenErrors(t *testing.T) {
	ctx := context.Background()
	githubDef := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	gitlabDef := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}
	emptyToken := providerpkg.TokenSet{}

	var nilRouter *Router
	if _, err := nilRouter.VerifyPAT(ctx, githubDef, "token"); err == nil || !strings.Contains(err.Error(), "router is not configured") {
		t.Fatalf("nil router error = %v", err)
	}

	githubAdapter := gitHubAdapter{client: github.NewClient(config.DefaultConfig(), logger.New(logger.LevelError, io.Discard), nil)}
	if _, err := githubAdapter.VerifyPAT(ctx, ""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub VerifyPAT empty token error = %v", err)
	}
	if _, err := githubAdapter.SSHKeyExists(ctx, emptyToken, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub SSHKeyExists empty token error = %v", err)
	}
	if err := githubAdapter.UploadSSHKey(ctx, emptyToken, "title", "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub UploadSSHKey empty token error = %v", err)
	}
	if _, err := githubAdapter.DeleteSSHKey(ctx, emptyToken, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub DeleteSSHKey empty token error = %v", err)
	}
	if _, err := githubAdapter.GPGKeyExists(ctx, emptyToken, "ABC123"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub GPGKeyExists empty token error = %v", err)
	}
	if err := githubAdapter.UploadGPGKey(ctx, emptyToken, "armored"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub UploadGPGKey empty token error = %v", err)
	}
	if _, err := githubAdapter.DeleteGPGKey(ctx, emptyToken, "ABC123"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitHub DeleteGPGKey empty token error = %v", err)
	}
	if _, err := (gitHubAdapter{}).clientWithToken(providerpkg.TokenSet{AccessToken: "token"}); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("GitHub nil client error = %v", err)
	}

	gitlabAdapter := gitLabAdapter{client: gitlab.NewClient(config.ProviderConfig{APIURL: "https://gitlab.example/api/v4"}, logger.New(logger.LevelError, io.Discard))}
	if _, err := gitlabAdapter.VerifyPAT(ctx, ""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab VerifyPAT empty token error = %v", err)
	}
	if _, err := gitlabAdapter.SSHKeyExists(ctx, emptyToken, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab SSHKeyExists empty token error = %v", err)
	}
	if err := gitlabAdapter.UploadSSHKey(ctx, emptyToken, "title", "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab UploadSSHKey empty token error = %v", err)
	}
	if _, err := gitlabAdapter.DeleteSSHKey(ctx, emptyToken, "ssh-ed25519 AAAA"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab DeleteSSHKey empty token error = %v", err)
	}
	if _, err := gitlabAdapter.GPGKeyExists(ctx, emptyToken, "ABC123"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab GPGKeyExists empty token error = %v", err)
	}
	if err := gitlabAdapter.UploadGPGKey(ctx, emptyToken, "armored"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab UploadGPGKey empty token error = %v", err)
	}
	if _, err := gitlabAdapter.DeleteGPGKey(ctx, emptyToken, "ABC123"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("GitLab DeleteGPGKey empty token error = %v", err)
	}
	if _, err := (gitLabAdapter{}).clientWithToken(providerpkg.TokenSet{AccessToken: "token"}); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("GitLab nil client error = %v", err)
	}

	_ = gitlabDef
}

type stubAdapter struct {
	verifyToken string
	sshToken    string
}

func (s *stubAdapter) VerifyPAT(_ context.Context, token string) (AuthenticatedUser, error) {
	s.verifyToken = token
	return AuthenticatedUser{Username: "bitbucket-user", Name: "Bitbucket User"}, nil
}

func (s *stubAdapter) SSHKeyExists(_ context.Context, token providerpkg.TokenSet, _ string) (bool, error) {
	s.sshToken = token.AccessToken
	return true, nil
}

func (s *stubAdapter) UploadSSHKey(context.Context, providerpkg.TokenSet, string, string) error {
	return nil
}

func (s *stubAdapter) DeleteSSHKey(context.Context, providerpkg.TokenSet, string) (bool, error) {
	return true, nil
}

func (s *stubAdapter) GPGKeyExists(context.Context, providerpkg.TokenSet, string) (bool, error) {
	return true, nil
}

func (s *stubAdapter) UploadGPGKey(context.Context, providerpkg.TokenSet, string) error {
	return nil
}

func (s *stubAdapter) DeleteGPGKey(context.Context, providerpkg.TokenSet, string) (bool, error) {
	return true, nil
}
