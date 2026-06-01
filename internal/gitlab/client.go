// Package gitlab provides GitLab API integration for GCM.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

const maxResponseSize = 5 << 20 // 5 MiB

// Client handles GitLab API operations.
type Client struct {
	cfg        config.ProviderConfig
	log        *logger.Logger
	apiURL     string
	token      provider.TokenSet
	httpClient *http.Client
}

// NewClient creates a GitLab API client from provider config.
func NewClient(cfg config.ProviderConfig, log *logger.Logger) *Client {
	apiURL := strings.TrimRight(cfg.APIURL, "/")
	if apiURL == "" {
		apiURL = "https://gitlab.com/api/v4"
	}
	return &Client{
		cfg:        cfg,
		log:        log,
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// WithTokenSet returns a shallow client clone configured with the provided token.
// It lets callers perform token-scoped operations without mutating a shared
// client instance.
func (c *Client) WithTokenSet(token provider.TokenSet) *Client {
	clone := *c
	clone.token = token
	return &clone
}

// User represents a GitLab user.
type User struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	PublicEmail string `json:"public_email"`
	AvatarURL   string `json:"avatar_url"`
	WebURL      string `json:"web_url"`
}

// SSHKeyResponse from GitLab keys API.
type SSHKeyResponse struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// GPGKeyResponse from GitLab GPG keys API.
type GPGKeyResponse struct {
	ID          int    `json:"id"`
	KeyID       string `json:"key_id"`
	Fingerprint string `json:"fingerprint"`
	Key         string `json:"key"`
}

// SetToken configures a PAT token for API calls.
func (c *Client) SetToken(token string) {
	c.token = provider.TokenSet{AccessToken: token, AuthMethod: provider.AuthMethodPAT}
}

// SetTokenSet configures a structured token for API calls.
func (c *Client) SetTokenSet(token provider.TokenSet) {
	c.token = token
}

// GetUser returns the authenticated user's info.
func (c *Client) GetUser(ctx context.Context) (*User, error) {
	var user User
	if err := c.apiGet(ctx, "/user", &user); err != nil {
		return nil, fmt.Errorf("getting GitLab user: %w", err)
	}
	return &user, nil
}

// VerifyToken checks if the current token is valid.
func (c *Client) VerifyToken(ctx context.Context) (*User, error) {
	return c.GetUser(ctx)
}

// UploadSSHKey uploads a public SSH key to GitLab.
func (c *Client) UploadSSHKey(ctx context.Context, title, publicKey string) error {
	payload := map[string]string{"title": title, "key": publicKey}
	return c.apiPost(ctx, "/user/keys", payload, nil)
}

// ListSSHKeys returns the user's SSH keys from GitLab.
func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKeyResponse, error) {
	var keys []SSHKeyResponse
	if err := c.apiGet(ctx, "/user/keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// SSHKeyExists checks if a public key is already uploaded to GitLab.
func (c *Client) SSHKeyExists(ctx context.Context, publicKey string) (bool, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return false, err
	}
	localKey := normalizeSSHKey(publicKey)
	for _, key := range keys {
		if normalizeSSHKey(key.Key) == localKey {
			return true, nil
		}
	}
	return false, nil
}

// DeleteSSHKey removes an SSH key from GitLab by matching the public key content.
func (c *Client) DeleteSSHKey(ctx context.Context, publicKey string) (bool, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return false, err
	}
	localKey := normalizeSSHKey(publicKey)
	for _, key := range keys {
		if normalizeSSHKey(key.Key) == localKey {
			if err := c.apiDelete(ctx, fmt.Sprintf("/user/keys/%d", key.ID)); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// UploadGPGKey uploads a public GPG key to GitLab.
func (c *Client) UploadGPGKey(ctx context.Context, armoredKey string) error {
	payload := map[string]string{"key": armoredKey}
	return c.apiPost(ctx, "/user/gpg_keys", payload, nil)
}

// ListGPGKeys returns the user's GPG keys from GitLab.
func (c *Client) ListGPGKeys(ctx context.Context) ([]GPGKeyResponse, error) {
	var keys []GPGKeyResponse
	if err := c.apiGet(ctx, "/user/gpg_keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// GPGKeyExists checks if a GPG key ID or fingerprint is already uploaded.
func (c *Client) GPGKeyExists(ctx context.Context, keyID string) (bool, error) {
	keys, err := c.ListGPGKeys(ctx)
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if strings.EqualFold(key.KeyID, keyID) || strings.EqualFold(key.Fingerprint, keyID) {
			return true, nil
		}
	}
	return false, nil
}

// DeleteGPGKey removes a GPG key from GitLab by matching key ID or fingerprint.
func (c *Client) DeleteGPGKey(ctx context.Context, keyID string) (bool, error) {
	keys, err := c.ListGPGKeys(ctx)
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if strings.EqualFold(key.KeyID, keyID) || strings.EqualFold(key.Fingerprint, keyID) {
			if err := c.apiDelete(ctx, fmt.Sprintf("/user/gpg_keys/%d", key.ID)); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func normalizeSSHKey(key string) string {
	parts := strings.Fields(strings.TrimSpace(key))
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return strings.TrimSpace(key)
}

func (c *Client) apiGet(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(result)
}

func (c *Client) apiPost(ctx context.Context, path string, payload, result interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(result)
	}
	return nil
}

func (c *Client) apiDelete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.apiURL+path, nil)
	if err != nil {
		return err
	}
	c.authorize(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("GitLab API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.token.Bearer() {
		req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	} else {
		req.Header.Set("PRIVATE-TOKEN", c.token.AccessToken)
	}
	req.Header.Set("Accept", "application/json")
}
