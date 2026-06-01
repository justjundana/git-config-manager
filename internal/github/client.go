// Package github provides GitHub API integration for GCM.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/tokenstore"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// maxResponseSize caps how much data we read from GitHub API responses to
// prevent unbounded memory consumption from malicious or broken servers.
const maxResponseSize = 5 << 20 // 5 MiB

// minPollInterval is the floor applied to the OAuth device-flow polling
// interval. GitHub requires at least 5 seconds between requests. Tests may
// override this to avoid sleeping 5+ seconds per iteration.
var minPollInterval = 5

// execCommandContext is a hook for exec.CommandContext, overridden in tests.
var execCommandContext = exec.CommandContext

// pollTimeout is the maximum duration PollForToken will wait before giving up.
// Tests may override this to avoid long waits.
var pollTimeout = 15 * time.Minute

// Client handles GitHub API operations.
type Client struct {
	cfg        *config.Config
	log        *logger.Logger
	apiURL     string
	token      string
	httpClient *http.Client
	tokenStore *tokenstore.TokenStore
}

// NewClient creates a new GitHub client.
func NewClient(cfg *config.Config, log *logger.Logger, tokenStore *tokenstore.TokenStore) *Client {
	return &Client{
		cfg:    cfg,
		log:    log,
		apiURL: cfg.GitHub.APIURL,
		// 60s is more forgiving than 30s on slow connections without leaving
		// HTTP requests hanging indefinitely.
		httpClient: &http.Client{Timeout: 60 * time.Second},
		tokenStore: tokenStore,
	}
}

// WithToken returns a shallow client clone configured with the provided token.
// It lets callers perform token-scoped operations without mutating a shared
// client instance.
func (c *Client) WithToken(token string) *Client {
	clone := *c
	clone.token = token
	return &clone
}

// User represents a GitHub user.
type User struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Bio         string `json:"bio"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	PublicRepos int    `json:"public_repos"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	HTMLURL     string `json:"html_url"`
}

// DeviceCodeResponse from the device flow initiation.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse from the token exchange.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// SSHKeyResponse from the keys API.
type SSHKeyResponse struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// InitiateDeviceFlow starts the OAuth device flow.
func (c *Client) InitiateDeviceFlow() (*DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {c.cfg.GitHub.OAuth.ClientID},
		"scope":     {strings.Join(c.cfg.GitHub.OAuth.Scopes, " ")},
	}

	req, _ := http.NewRequest("POST", "https://github.com/login/device/code", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to GitHub — check your internet connection")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("could not read response from GitHub")
	}

	// If we got a non-2xx response or HTML back, the client_id is likely invalid
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub rejected the login request (HTTP %d) — the client_id may be invalid", resp.StatusCode)
	}

	// GitHub returns JSON when Accept header is set
	var dcr DeviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		// Try parsing as form-encoded. Cap input size to mitigate
		// GO-2026-4341 (memory exhaustion in url.ParseQuery).
		queryInput := string(body)
		if len(queryInput) > 4096 {
			queryInput = queryInput[:4096]
		}
		values, parseErr := url.ParseQuery(queryInput)
		if parseErr != nil {
			return nil, fmt.Errorf("unexpected response from GitHub — the client_id may be invalid")
		}
		dcr = DeviceCodeResponse{
			DeviceCode:      values.Get("device_code"),
			UserCode:        values.Get("user_code"),
			VerificationURI: values.Get("verification_uri"),
		}
	}

	// Check for error in JSON response
	if dcr.DeviceCode == "" {
		return nil, fmt.Errorf("GitHub did not return a device code — the client_id may be invalid")
	}

	return &dcr, nil
}

// PollForToken polls GitHub for the access token using the OAuth device flow.
//
// GitHub may respond with:
//   - authorization_pending: user hasn't finished authorizing yet
//   - slow_down: we're polling too fast; the retry interval must be increased
//     by (at minimum) the value returned in the next interval, per RFC 8628.
//
// We respect the server-provided interval, enforce a minimum 5s floor, and
// cap the total polling window at the device-code lifetime (typically 15
// minutes for GitHub). The caller's context can cancel the wait early.
func (c *Client) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
	if interval < minPollInterval {
		interval = minPollInterval
	}

	// Overall polling deadline: 15 minutes is the GitHub device code TTL.
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	// consecutiveErrors tracks transient network failures so we can apply
	// exponential backoff instead of hammering a potentially overloaded
	// network at the base interval.
	consecutiveErrors := 0
	const maxBackoffErrors = 5 // cap at 2^5 = 32x the base interval

	for {
		// Apply exponential backoff on consecutive transient errors.
		waitInterval := interval
		if consecutiveErrors > 0 {
			backoff := 1 << min(consecutiveErrors, maxBackoffErrors)
			waitInterval = interval * backoff
		}

		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("authorization timed out")
		case <-time.After(time.Duration(waitInterval) * time.Second):
		}

		data := url.Values{
			"client_id":   {c.cfg.GitHub.OAuth.ClientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}

		req, _ := http.NewRequestWithContext(pollCtx, "POST",
			"https://github.com/login/oauth/access_token",
			strings.NewReader(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Transient network error — apply exponential backoff.
			consecutiveErrors++
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
		if readErr != nil {
			consecutiveErrors++
			continue // transient read error, retry with backoff
		}

		// Successful network round-trip — reset backoff counter.
		consecutiveErrors = 0

		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			// Fallback: GitHub may return form-encoded response. Cap the
			// input size to mitigate GO-2026-4341 (memory exhaustion in
			// url.ParseQuery with pathological inputs).
			queryInput := string(body)
			if len(queryInput) > 4096 {
				queryInput = queryInput[:4096]
			}
			values, _ := url.ParseQuery(queryInput)
			tokenResp = TokenResponse{
				AccessToken: values.Get("access_token"),
				Error:       values.Get("error"),
				ErrorDesc:   values.Get("error_description"),
			}
		}

		switch tokenResp.Error {
		case "authorization_pending":
			// Continue at the current interval.
		case "slow_down":
			// RFC 8628: bump the interval on slow_down responses.
			interval += 5
		case "":
			if tokenResp.AccessToken != "" {
				return tokenResp.AccessToken, nil
			}
			// No token and no error - treat as pending.
		default:
			return "", fmt.Errorf("OAuth error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
		}
	}
}

// SaveToken stores the token for a profile using the configured storage
// backend (OS keychain, encrypted file, or plain-text file).
func (c *Client) SaveToken(profile, token string) error {
	return c.tokenStore.Save(profile, token)
}

// LoadToken reads the stored token for a profile.
func (c *Client) LoadToken(profile string) (string, error) {
	return c.tokenStore.Load(profile)
}

// DeleteToken removes the stored token for a profile.
func (c *Client) DeleteToken(profile string) error {
	return c.tokenStore.Delete(profile)
}

// ClearGitCredentials removes cached git credentials for GitHub from the
// system's git credential helper. This works cross-platform:
//   - macOS: clears from osxkeychain / Keychain Access
//   - Linux: clears from libsecret, gnome-keyring, or credential-cache
//   - Windows: clears from Windows Credential Manager
//
// It uses `git credential reject` which is the standard interface all
// credential helpers implement.
func (c *Client) ClearGitCredentials(server string) error {
	if server == "" {
		server = "https://github.com"
	}

	parsed, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}

	host := parsed.Host
	if host == "" {
		host = parsed.Path // handle case where server is just "github.com"
	}
	protocol := parsed.Scheme
	if protocol == "" {
		protocol = "https"
	}

	// Sanitize values to prevent newline injection into the credential protocol.
	protocol = SanitizeCredentialField(protocol)
	host = SanitizeCredentialField(host)

	// The credential protocol format that git credential helpers expect
	credInput := fmt.Sprintf("protocol=%s\nhost=%s\n\n", protocol, host)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "git", "credential", "reject")
	cmd.Stdin = strings.NewReader(credInput)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Don't fail hard — git credential reject may not be available
		c.log.Debug("git credential reject returned error (non-fatal)",
			logger.F("error", err.Error()),
			logger.F("stderr", stderr.String()))
		return nil
	}

	c.log.Debug("Git credentials cleared", logger.F("host", host))
	return nil
}

// StoreGitCredentials saves credentials into git's credential helper so that
// git clone/push/pull operations work without prompting. Uses `git credential approve`.
func (c *Client) StoreGitCredentials(server, username, token string) error {
	if server == "" {
		server = "https://github.com"
	}

	parsed, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("parsing server URL: %w", err)
	}

	host := parsed.Host
	if host == "" {
		host = parsed.Path
	}
	protocol := parsed.Scheme
	if protocol == "" {
		protocol = "https"
	}

	// Sanitize all values to prevent newline injection into the credential protocol.
	protocol = SanitizeCredentialField(protocol)
	host = SanitizeCredentialField(host)
	username = SanitizeCredentialField(username)
	token = SanitizeCredentialField(token)

	credInput := fmt.Sprintf("protocol=%s\nhost=%s\nusername=%s\npassword=%s\n\n",
		protocol, host, username, token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, "git", "credential", "approve")
	cmd.Stdin = strings.NewReader(credInput)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.log.Debug("git credential approve returned error (non-fatal)",
			logger.F("error", err.Error()),
			logger.F("stderr", stderr.String()))
		return nil
	}

	c.log.Debug("Git credentials stored", logger.F("host", host), logger.F("username", username))
	return nil
}

// SetGitCredentialUsername sets or unsets git's credential.username for the
// given server so that git only uses credentials belonging to a specific user.
// This prevents credential bleed when switching between accounts.
//
// If username is empty, the setting is unset (git will use any available credential).
func (c *Client) SetGitCredentialUsername(server, username string) error {
	if server == "" {
		server = "https://github.com"
	}

	key := fmt.Sprintf("credential.%s.username", server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if username == "" {
		cmd = execCommandContext(ctx, "git", "config", "--global", "--unset", key)
	} else {
		cmd = execCommandContext(ctx, "git", "config", "--global", key, username)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// --unset returns exit code 5 if the key doesn't exist — not an error
		if username == "" {
			c.log.Debug("git config --unset (non-fatal)", logger.F("key", key))
			return nil
		}
		c.log.Debug("git config set credential.username failed (non-fatal)",
			logger.F("error", err.Error()),
			logger.F("stderr", stderr.String()))
		return nil
	}

	if username != "" {
		c.log.Debug("Git credential username set", logger.F("server", server), logger.F("username", username))
	} else {
		c.log.Debug("Git credential username unset", logger.F("server", server))
	}
	return nil
}

// SetToken sets the token for API calls.
func (c *Client) SetToken(token string) {
	c.token = token
}

// GetUser returns the authenticated user's info.
func (c *Client) GetUser(ctx context.Context) (*User, error) {
	var user User
	if err := c.apiGet(ctx, "/user", &user); err != nil {
		return nil, fmt.Errorf("getting user: %w", err)
	}
	return &user, nil
}

// VerifyToken checks if the current token is valid.
func (c *Client) VerifyToken(ctx context.Context) (*User, error) {
	return c.GetUser(ctx)
}

// UploadSSHKey uploads a public SSH key to GitHub.
func (c *Client) UploadSSHKey(ctx context.Context, title, publicKey string) error {
	payload := map[string]string{
		"title": title,
		"key":   publicKey,
	}

	return c.apiPost(ctx, "/user/keys", payload, nil)
}

// UploadGPGKey uploads a public GPG key to GitHub.
func (c *Client) UploadGPGKey(ctx context.Context, armoredKey string) error {
	payload := map[string]string{
		"armored_public_key": armoredKey,
	}

	return c.apiPost(ctx, "/user/gpg_keys", payload, nil)
}

// ListSSHKeys returns the user's SSH keys from GitHub.
func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKeyResponse, error) {
	var keys []SSHKeyResponse
	if err := c.apiGet(ctx, "/user/keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// GPGKeyResponse from the gpg_keys API.
type GPGKeyResponse struct {
	ID    int    `json:"id"`
	KeyID string `json:"key_id"`
	Email string `json:"primary_key_id"`
}

// ListGPGKeys returns the user's GPG keys from GitHub.
func (c *Client) ListGPGKeys(ctx context.Context) ([]GPGKeyResponse, error) {
	var keys []GPGKeyResponse
	if err := c.apiGet(ctx, "/user/gpg_keys", &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// SSHKeyExists checks if a public key is already uploaded to GitHub.
// It compares the key material (type + base64 data), ignoring comments.
func (c *Client) SSHKeyExists(ctx context.Context, publicKey string) (bool, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return false, err
	}
	localKey := normalizeSSHKey(publicKey)
	for _, k := range keys {
		if normalizeSSHKey(k.Key) == localKey {
			return true, nil
		}
	}
	return false, nil
}

// DeleteSSHKey removes an SSH key from GitHub by matching the public key content.
// Returns true if a key was found and deleted, false if not found.
func (c *Client) DeleteSSHKey(ctx context.Context, publicKey string) (bool, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return false, err
	}
	localKey := normalizeSSHKey(publicKey)
	for _, k := range keys {
		if normalizeSSHKey(k.Key) == localKey {
			path := fmt.Sprintf("/user/keys/%d", k.ID)
			if err := c.apiDelete(ctx, path); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// DeleteGPGKey removes a GPG key from GitHub by matching the key ID.
// Returns true if a key was found and deleted, false if not found.
func (c *Client) DeleteGPGKey(ctx context.Context, keyID string) (bool, error) {
	keys, err := c.ListGPGKeys(ctx)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if strings.EqualFold(k.KeyID, keyID) {
			path := fmt.Sprintf("/user/gpg_keys/%d", k.ID)
			if err := c.apiDelete(ctx, path); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// GPGKeyExists checks if a GPG key ID is already uploaded to GitHub.
func (c *Client) GPGKeyExists(ctx context.Context, keyID string) (bool, error) {
	keys, err := c.ListGPGKeys(ctx)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if strings.EqualFold(k.KeyID, keyID) {
			return true, nil
		}
	}
	return false, nil
}

// normalizeSSHKey extracts the key type and base64 data, stripping any trailing comment.
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
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
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
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(result)
	}

	return nil
}

func (c *Client) apiDelete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.apiURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SanitizeCredentialField removes newlines, carriage returns, and null bytes
// from a value to prevent injection of additional fields into the git credential
// protocol. Null bytes are stripped because C-based credential helpers may
// interpret them as string terminators, causing field truncation.
func SanitizeCredentialField(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// ImportFromGHCLI retrieves the GitHub token from the gh CLI tool.
func (c *Client) ImportFromGHCLI() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not run 'gh auth token'")
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("GitHub CLI returned an empty token")
	}
	return token, nil
}
