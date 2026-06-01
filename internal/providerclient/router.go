package providerclient

import (
	"context"
	"fmt"

	"github.com/justjundana/git-config-manager/internal/github"
	"github.com/justjundana/git-config-manager/internal/gitlab"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

// AuthenticatedUser is the provider-neutral identity returned after token verification.
type AuthenticatedUser struct {
	Username string
	Name     string
}

// Adapter implements provider-specific authenticated operations.
type Adapter interface {
	VerifyPAT(ctx context.Context, token string) (AuthenticatedUser, error)
	SSHKeyExists(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error)
	UploadSSHKey(ctx context.Context, token providerpkg.TokenSet, title, publicKey string) error
	DeleteSSHKey(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error)
	GPGKeyExists(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error)
	UploadGPGKey(ctx context.Context, token providerpkg.TokenSet, armoredKey string) error
	DeleteGPGKey(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error)
}

// Router dispatches provider operations through registered provider adapters.
type Router struct {
	adapters map[providerpkg.ProviderID]Adapter
}

// NewRouter creates a provider operation router for configured clients.
func NewRouter(githubClient *github.Client, gitlabClient *gitlab.Client) *Router {
	r := NewRouterWithAdapters(nil)
	if githubClient != nil {
		r.Register(providerpkg.GitHubID, newGitHubAdapter(githubClient))
	}
	if gitlabClient != nil {
		r.Register(providerpkg.GitLabID, newGitLabAdapter(gitlabClient))
	}
	return r
}

// NewRouterWithAdapters creates a router from prebuilt adapters.
func NewRouterWithAdapters(adapters map[providerpkg.ProviderID]Adapter) *Router {
	r := &Router{adapters: make(map[providerpkg.ProviderID]Adapter)}
	for id, adapter := range adapters {
		r.Register(id, adapter)
	}
	return r
}

// Register adds or replaces the adapter for a provider ID.
func (r *Router) Register(id providerpkg.ProviderID, adapter Adapter) {
	if adapter == nil {
		delete(r.adapters, id)
		return
	}
	r.adapters[id] = adapter
}

// SetToken validates token routing compatibility. Provider operations are
// token-scoped, so this method intentionally does not mutate shared clients.
func (r *Router) SetToken(def providerpkg.Definition, token providerpkg.TokenSet) error {
	if token.AccessToken == "" {
		return fmt.Errorf("%s token is empty", def.DisplayName)
	}
	_, err := r.adapter(def, "operation")
	return err
}

// VerifyPAT validates a Personal Access Token and returns the authenticated user.
func (r *Router) VerifyPAT(ctx context.Context, def providerpkg.Definition, token string) (AuthenticatedUser, error) {
	adapter, err := r.adapter(def, "token verification")
	if err != nil {
		return AuthenticatedUser{}, err
	}
	return adapter.VerifyPAT(ctx, token)
}

// SSHKeyExists reports whether a provider already has the given public key.
func (r *Router) SSHKeyExists(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, publicKey string) (bool, error) {
	adapter, err := r.adapter(def, "SSH key upload")
	if err != nil {
		return false, err
	}
	return adapter.SSHKeyExists(ctx, token, publicKey)
}

// UploadSSHKey uploads a public SSH key to the provider account.
func (r *Router) UploadSSHKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, title, publicKey string) error {
	adapter, err := r.adapter(def, "SSH key upload")
	if err != nil {
		return err
	}
	return adapter.UploadSSHKey(ctx, token, title, publicKey)
}

// DeleteSSHKey removes a public SSH key from the provider account when found.
func (r *Router) DeleteSSHKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, publicKey string) (bool, error) {
	adapter, err := r.adapter(def, "SSH key deletion")
	if err != nil {
		return false, err
	}
	return adapter.DeleteSSHKey(ctx, token, publicKey)
}

// GPGKeyExists reports whether a provider already has the given GPG key.
func (r *Router) GPGKeyExists(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, keyID string) (bool, error) {
	adapter, err := r.adapter(def, "GPG key upload")
	if err != nil {
		return false, err
	}
	return adapter.GPGKeyExists(ctx, token, keyID)
}

// UploadGPGKey uploads an armored GPG public key to the provider account.
func (r *Router) UploadGPGKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, armoredKey string) error {
	adapter, err := r.adapter(def, "GPG key upload")
	if err != nil {
		return err
	}
	return adapter.UploadGPGKey(ctx, token, armoredKey)
}

// DeleteGPGKey removes a GPG key from the provider account when found.
func (r *Router) DeleteGPGKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, keyID string) (bool, error) {
	adapter, err := r.adapter(def, "GPG key deletion")
	if err != nil {
		return false, err
	}
	return adapter.DeleteGPGKey(ctx, token, keyID)
}

func (r *Router) adapter(def providerpkg.Definition, operation string) (Adapter, error) {
	if r == nil {
		return nil, fmt.Errorf("provider router is not configured")
	}
	adapter := r.adapters[def.ID]
	if adapter != nil {
		return adapter, nil
	}
	switch def.ID {
	case providerpkg.GitHubID, providerpkg.GitLabID:
		return nil, fmt.Errorf("%s client is not configured", def.DisplayName)
	default:
		if operation == "token verification" || operation == "operation" {
			return nil, fmt.Errorf("provider %q is not implemented yet", def.ID)
		}
		return nil, fmt.Errorf("provider %q does not support %s yet", def.ID, operation)
	}
}

func validateToken(def providerpkg.Definition, token providerpkg.TokenSet) error {
	if token.AccessToken == "" {
		return fmt.Errorf("%s token is empty", def.DisplayName)
	}
	return nil
}

type gitHubAdapter struct {
	client *github.Client
}

func newGitHubAdapter(client *github.Client) Adapter {
	return gitHubAdapter{client: client}
}

func (a gitHubAdapter) clientWithToken(token providerpkg.TokenSet) (*github.Client, error) {
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	if err := validateToken(def, token); err != nil {
		return nil, err
	}
	if a.client == nil {
		return nil, fmt.Errorf("GitHub client is not configured")
	}
	return a.client.WithToken(token.AccessToken), nil
}

func (a gitHubAdapter) VerifyPAT(ctx context.Context, token string) (AuthenticatedUser, error) {
	client, err := a.clientWithToken(providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"})
	if err != nil {
		return AuthenticatedUser{}, err
	}
	user, err := client.GetUser(ctx)
	if err != nil {
		return AuthenticatedUser{}, fmt.Errorf("GitHub token verification failed: %w", err)
	}
	return AuthenticatedUser{Username: user.Login, Name: user.Name}, nil
}

func (a gitHubAdapter) SSHKeyExists(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.SSHKeyExists(ctx, publicKey)
}

func (a gitHubAdapter) UploadSSHKey(ctx context.Context, token providerpkg.TokenSet, title, publicKey string) error {
	client, err := a.clientWithToken(token)
	if err != nil {
		return err
	}
	return client.UploadSSHKey(ctx, title, publicKey)
}

func (a gitHubAdapter) DeleteSSHKey(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.DeleteSSHKey(ctx, publicKey)
}

func (a gitHubAdapter) GPGKeyExists(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.GPGKeyExists(ctx, keyID)
}

func (a gitHubAdapter) UploadGPGKey(ctx context.Context, token providerpkg.TokenSet, armoredKey string) error {
	client, err := a.clientWithToken(token)
	if err != nil {
		return err
	}
	return client.UploadGPGKey(ctx, armoredKey)
}

func (a gitHubAdapter) DeleteGPGKey(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.DeleteGPGKey(ctx, keyID)
}

type gitLabAdapter struct {
	client *gitlab.Client
}

func newGitLabAdapter(client *gitlab.Client) Adapter {
	return gitLabAdapter{client: client}
}

func (a gitLabAdapter) clientWithToken(token providerpkg.TokenSet) (*gitlab.Client, error) {
	def := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab"}
	if err := validateToken(def, token); err != nil {
		return nil, err
	}
	if a.client == nil {
		return nil, fmt.Errorf("GitLab client is not configured")
	}
	return a.client.WithTokenSet(token), nil
}

func (a gitLabAdapter) VerifyPAT(ctx context.Context, token string) (AuthenticatedUser, error) {
	client, err := a.clientWithToken(providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"})
	if err != nil {
		return AuthenticatedUser{}, err
	}
	user, err := client.GetUser(ctx)
	if err != nil {
		return AuthenticatedUser{}, fmt.Errorf("GitLab token verification failed: %w", err)
	}
	return AuthenticatedUser{Username: user.Username, Name: user.Name}, nil
}

func (a gitLabAdapter) SSHKeyExists(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.SSHKeyExists(ctx, publicKey)
}

func (a gitLabAdapter) UploadSSHKey(ctx context.Context, token providerpkg.TokenSet, title, publicKey string) error {
	client, err := a.clientWithToken(token)
	if err != nil {
		return err
	}
	return client.UploadSSHKey(ctx, title, publicKey)
}

func (a gitLabAdapter) DeleteSSHKey(ctx context.Context, token providerpkg.TokenSet, publicKey string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.DeleteSSHKey(ctx, publicKey)
}

func (a gitLabAdapter) GPGKeyExists(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.GPGKeyExists(ctx, keyID)
}

func (a gitLabAdapter) UploadGPGKey(ctx context.Context, token providerpkg.TokenSet, armoredKey string) error {
	client, err := a.clientWithToken(token)
	if err != nil {
		return err
	}
	return client.UploadGPGKey(ctx, armoredKey)
}

func (a gitLabAdapter) DeleteGPGKey(ctx context.Context, token providerpkg.TokenSet, keyID string) (bool, error) {
	client, err := a.clientWithToken(token)
	if err != nil {
		return false, err
	}
	return client.DeleteGPGKey(ctx, keyID)
}
