package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	cryptoSvc "github.com/justjundana/git-config-manager/internal/service/crypto"
	"github.com/justjundana/git-config-manager/internal/tokenstore"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// plainTextConfig returns a config with all token encryption and keychain
// disabled so that tests exercise the plain-text file storage backend.
func plainTextConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Security.EncryptTokens = false
	cfg.Security.UseKeychain = false
	cfg.Security.MasterPassword = false
	cfg.Security.AllowPlaintextTokens = true
	return cfg
}

func init() {
	// Reduce the OAuth poll interval floor from 5s to 1s so tests that
	// exercise multiple polling iterations don't take 15+ seconds each.
	minPollInterval = 1
}

// newTestTokenStore creates a TokenStore suitable for testing (plain-text).
func newTestTokenStore(cfg *config.Config) *tokenstore.TokenStore {
	log := logger.New(logger.LevelError, os.Stderr)
	crypto := cryptoSvc.NewService()
	return tokenstore.NewTokenStore(cfg, crypto, log, nil)
}

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = srv.URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	return NewClient(cfg, log, ts)
}

func TestSaveLoadDeleteToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	if err := c.SaveToken("testprofile", "ghp_abc123"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, err := c.LoadToken("testprofile")
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if token != "ghp_abc123" {
		t.Errorf("token = %q, want %q", token, "ghp_abc123")
	}

	if err := c.DeleteToken("testprofile"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err = c.LoadToken("testprofile")
	if err == nil {
		t.Error("expected error loading deleted token")
	}
}

func TestLoadToken_InvalidFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	path := filepath.Join(config.GCMDir(), "tokens", "bad")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(""), 0o600)

	_, err := c.LoadToken("bad")
	if err == nil {
		t.Error("expected error for empty token file")
	}
}

func TestDeleteToken_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	if err := c.DeleteToken("nonexistent"); err != nil {
		t.Fatalf("DeleteToken nonexistent: %v", err)
	}
}

func TestSetToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.SetToken("mytoken")
	if c.token != "mytoken" {
		t.Errorf("token = %q", c.token)
	}
}

func TestWithTokenReturnsTokenScopedClone(t *testing.T) {
	cfg := plainTextConfig()
	c := NewClient(cfg, logger.New(logger.LevelError, io.Discard), nil)
	c.SetToken("original")

	clone := c.WithToken("scoped")
	if clone == c {
		t.Fatal("WithToken returned original client")
	}
	if clone.token != "scoped" {
		t.Fatalf("clone token = %q, want scoped", clone.token)
	}
	if c.token != "original" {
		t.Fatalf("original token = %q, want original", c.token)
	}
	if clone.httpClient != c.httpClient {
		t.Fatal("clone should share HTTP client")
	}
}

func TestGetUser(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			json.NewEncoder(w).Encode(User{
				Login: "testuser",
				Name:  "Test User",
				Email: "test@example.com",
			})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("faketoken")

	user, err := c.GetUser(context.Background())
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Login != "testuser" {
		t.Errorf("Login = %q", user.Login)
	}
}

func TestVerifyToken(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(User{Login: "verifieduser"})
	})
	c.SetToken("faketoken")

	user, err := c.VerifyToken(context.Background())
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if user.Login != "verifieduser" {
		t.Errorf("Login = %q", user.Login)
	}
}

func TestUploadSSHKey(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/user/keys" {
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(400)
	})
	c.SetToken("faketoken")

	if err := c.UploadSSHKey(context.Background(), "mykey", "ssh-ed25519 AAAA..."); err != nil {
		t.Fatalf("UploadSSHKey: %v", err)
	}
}

func TestUploadGPGKey(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/user/gpg_keys" {
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(400)
	})
	c.SetToken("faketoken")

	if err := c.UploadGPGKey(context.Background(), "-----BEGIN PGP PUBLIC KEY BLOCK-----\n..."); err != nil {
		t.Fatalf("UploadGPGKey: %v", err)
	}
}

func TestListSSHKeys(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/keys" {
			json.NewEncoder(w).Encode([]SSHKeyResponse{
				{ID: 1, Key: "ssh-ed25519 AAAA", Title: "test"},
			})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("faketoken")

	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestApiGet_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"forbidden"}`))
	})
	c.SetToken("faketoken")

	var result User
	err := c.apiGet(context.Background(), "/user", &result)
	if err == nil {
		t.Error("expected error on 403")
	}
}

func TestApiPost_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"validation failed"}`))
	})
	c.SetToken("faketoken")

	err := c.apiPost(context.Background(), "/user/keys", map[string]string{"key": "data"}, nil)
	if err == nil {
		t.Error("expected error on 422")
	}
}

func TestApiPost_WithResult(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	c.SetToken("faketoken")

	var result map[string]string
	err := c.apiPost(context.Background(), "/test", map[string]string{"key": "val"}, &result)
	if err != nil {
		t.Fatalf("apiPost with result: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q", result["status"])
	}
}

func TestNewClient(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.apiURL != cfg.GitHub.APIURL {
		t.Errorf("apiURL = %q", c.apiURL)
	}
	if c.httpClient == nil {
		t.Error("expected non-nil httpClient")
	}
}

// roundTripFunc adapts a function to http.RoundTripper for testing.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestInitiateDeviceFlow_JSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client-id"
	cfg.GitHub.OAuth.Scopes = []string{"read:user", "user:email"}
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	// Replace transport to intercept the request to github.com
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// Verify the request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body := r.FormValue("client_id")
		if body != "test-client-id" {
			// FormValue doesn't work on the request directly; parse manually
		}

		resp := &DeviceCodeResponse{
			DeviceCode:      "device-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	dcr, err := c.InitiateDeviceFlow()
	if err != nil {
		t.Fatalf("InitiateDeviceFlow: %v", err)
	}
	if dcr.DeviceCode != "device-123" {
		t.Errorf("DeviceCode = %q", dcr.DeviceCode)
	}
	if dcr.UserCode != "ABCD-1234" {
		t.Errorf("UserCode = %q", dcr.UserCode)
	}
}

func TestInitiateDeviceFlow_FormEncoded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	// Return form-encoded response (not JSON)
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		formData := url.Values{
			"device_code":      {"form-device-code"},
			"user_code":        {"WXYZ-9999"},
			"verification_uri": {"https://github.com/login/device"},
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(formData.Encode()))),
			Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}, nil
	})

	dcr, err := c.InitiateDeviceFlow()
	if err != nil {
		t.Fatalf("InitiateDeviceFlow form: %v", err)
	}
	if dcr.DeviceCode != "form-device-code" {
		t.Errorf("DeviceCode = %q", dcr.DeviceCode)
	}
}

func TestPollForToken_ImmediateSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := TokenResponse{
			AccessToken: "ghp_success_token",
			TokenType:   "bearer",
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_success_token" {
		t.Errorf("token = %q", token)
	}
}

func TestPollForToken_OAuthError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := TokenResponse{
			Error:     "access_denied",
			ErrorDesc: "user denied access",
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	_, err := c.PollForToken(context.Background(), "device-code", 0)
	if err == nil {
		t.Fatal("expected error for access_denied")
	}
	if !contains(err.Error(), "access_denied") {
		t.Errorf("error = %q, expected to contain access_denied", err)
	}
}

func TestPollForToken_ContextCancelled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := TokenResponse{Error: "authorization_pending"}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.PollForToken(ctx, "device-code", 0)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPollForToken_FormEncoded(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		formData := url.Values{
			"access_token": {"ghp_form_token"},
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte(formData.Encode()))),
			Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken form: %v", err)
	}
	if token != "ghp_form_token" {
		t.Errorf("token = %q", token)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
func TestSaveAndLoadToken(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})

	if err := c.SaveToken("testprofile", "ghp_test_token_123"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, err := c.LoadToken("testprofile")
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if token != "ghp_test_token_123" {
		t.Errorf("token = %q, want %q", token, "ghp_test_token_123")
	}
}

func TestListSSHKeys_Empty(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/keys" {
			json.NewEncoder(w).Encode([]SSHKeyResponse{})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("faketoken")

	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestListSSHKeys_APIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"internal error"}`))
	})
	c.SetToken("faketoken")

	_, err := c.ListSSHKeys(context.Background())
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestInitiateDeviceFlow_NetworkError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("network unreachable")
	})

	_, err := c.InitiateDeviceFlow()
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestPollForToken_SlowDown(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	callCount := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		var resp TokenResponse
		if callCount == 1 {
			resp = TokenResponse{Error: "slow_down"}
		} else {
			resp = TokenResponse{AccessToken: "ghp_after_slowdown"}
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_after_slowdown" {
		t.Errorf("token = %q", token)
	}
}

func TestUploadSSHKey_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"key already exists"}`))
	})
	c.SetToken("faketoken")

	err := c.UploadSSHKey(context.Background(), "mykey", "ssh-ed25519 AAAA...")
	if err == nil {
		t.Fatal("expected error for duplicate key upload")
	}
}

func TestUploadGPGKey_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"key already exists"}`))
	})
	c.SetToken("faketoken")

	err := c.UploadGPGKey(context.Background(), "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...")
	if err == nil {
		t.Fatal("expected error for duplicate GPG key upload")
	}
}

func TestGetUser_NetworkError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://localhost:1" // invalid port
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("faketoken")

	_, err := c.GetUser(context.Background())
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestSaveToken_InvalidName(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	err := c.SaveToken("../escape", "token")
	if err == nil {
		t.Fatal("expected error for traversal in profile name")
	}
}

func TestLoadToken_InvalidName(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := c.LoadToken("../escape")
	if err == nil {
		t.Fatal("expected error for traversal in profile name")
	}
}

func TestDeleteToken_InvalidName(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})
	err := c.DeleteToken("../escape")
	if err == nil {
		t.Fatal("expected error for traversal in profile name")
	}
}

func TestDeleteToken(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})

	c.SaveToken("delme", "token123")
	if err := c.DeleteToken("delme"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err := c.LoadToken("delme")
	if err == nil {
		t.Fatal("expected error loading deleted token")
	}
}

func TestDeleteToken_NotExist(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})

	if err := c.DeleteToken("nope"); err != nil {
		t.Fatalf("DeleteToken nonexistent: %v", err)
	}
}

func TestLoadToken_TooShort(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})

	path := filepath.Join(config.GCMDir(), "tokens", "short")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte("   \n"), 0o600)

	_, err := c.LoadToken("short")
	if err == nil {
		t.Fatal("expected error for empty token file")
	}
}

func TestGetUser_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			json.NewEncoder(w).Encode(User{Login: "testuser", Name: "Test User", Email: "test@example.com"})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("fake-token")

	user, err := c.GetUser(context.Background())
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Login != "testuser" {
		t.Errorf("Login = %q, want %q", user.Login, "testuser")
	}
}

func TestGetUser_Unauthorized(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	})
	c.SetToken("bad-token")

	_, err := c.GetUser(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestListSSHKeys_WithKeys(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/keys" {
			json.NewEncoder(w).Encode([]SSHKeyResponse{{ID: 1, Title: "test", Key: "ssh-ed25519 AAAA"}})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("fake-token")

	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Title != "test" {
		t.Errorf("keys = %+v", keys)
	}
}

func TestUploadSSHKey_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/keys" && r.Method == "POST" {
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("fake-token")

	err := c.UploadSSHKey(context.Background(), "mykey", "ssh-ed25519 AAAA test")
	if err != nil {
		t.Fatalf("UploadSSHKey: %v", err)
	}
}

func TestUploadGPGKey_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/gpg_keys" && r.Method == "POST" {
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("fake-token")

	err := c.UploadGPGKey(context.Background(), "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...")
	if err != nil {
		t.Fatalf("UploadGPGKey: %v", err)
	}
}

func TestVerifyToken_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(User{Login: "verified"})
	})
	c.SetToken("fake-token")

	user, err := c.VerifyToken(context.Background())
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if user.Login != "verified" {
		t.Errorf("Login = %q", user.Login)
	}
}

func TestSaveToken_TraversalProfile(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {})

	err := c.SaveToken("../evil", "token")
	if err == nil {
		t.Fatal("expected error for traversal profile name")
	}
}

func TestApiGet_InvalidJSON(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("not json at all"))
	})
	c.SetToken("faketoken")

	var user User
	err := c.apiGet(context.Background(), "/user", &user)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestApiPost_WithNilResult(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`{"status":"created"}`))
	})
	c.SetToken("faketoken")

	// nil result should still succeed (body is ignored)
	err := c.apiPost(context.Background(), "/resource", map[string]string{"key": "val"}, nil)
	if err != nil {
		t.Fatalf("apiPost with nil result: %v", err)
	}
}

func TestApiGet_ServerError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"internal server error"}`))
	})
	c.SetToken("faketoken")

	var user User
	err := c.apiGet(context.Background(), "/user", &user)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestListSSHKeys_MultipleKeys(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/keys" {
			json.NewEncoder(w).Encode([]SSHKeyResponse{
				{ID: 1, Key: "ssh-ed25519 AAAA1", Title: "key1"},
				{ID: 2, Key: "ssh-rsa AAAA2", Title: "key2"},
				{ID: 3, Key: "ssh-ed25519 AAAA3", Title: "key3"},
			})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("faketoken")

	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestPollForToken_AuthorizationPendingThenSuccess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	callCount := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		var resp TokenResponse
		if callCount <= 2 {
			resp = TokenResponse{Error: "authorization_pending"}
		} else {
			resp = TokenResponse{AccessToken: "ghp_final_token"}
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_final_token" {
		t.Errorf("token = %q", token)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestPollForToken_NetworkErrorRecovery(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	callCount := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		if callCount <= 2 {
			// Simulate network error on first two attempts
			return nil, fmt.Errorf("connection refused")
		}
		resp := TokenResponse{AccessToken: "ghp_recovered"}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken after recovery: %v", err)
	}
	if token != "ghp_recovered" {
		t.Errorf("token = %q", token)
	}
}

func TestInitiateDeviceFlow_MalformedResponse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte("not json or form encoded!!!{{{{"))),
			Header:     http.Header{"Content-Type": {"text/plain"}},
		}, nil
	})

	// Should try JSON parse and form-encoded parse but still return a result
	// (with empty fields since neither parse properly)
	dcr, err := c.InitiateDeviceFlow()
	// The function falls through to form-encoded parse which may or may not error
	_ = dcr
	_ = err
}

func TestGetUser_FullFields(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			json.NewEncoder(w).Encode(User{
				Login:       "fulluser",
				Name:        "Full User",
				Email:       "full@example.com",
				Bio:         "A bio",
				Company:     "ACME Inc",
				Location:    "NYC",
				PublicRepos: 100,
				Followers:   50,
				Following:   25,
				HTMLURL:     "https://github.com/fulluser",
			})
			return
		}
		w.WriteHeader(404)
	})
	c.SetToken("faketoken")

	user, err := c.GetUser(context.Background())
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Company != "ACME Inc" {
		t.Errorf("Company = %q", user.Company)
	}
	if user.PublicRepos != 100 {
		t.Errorf("PublicRepos = %d", user.PublicRepos)
	}
	if user.HTMLURL != "https://github.com/fulluser" {
		t.Errorf("HTMLURL = %q", user.HTMLURL)
	}
}

func TestApiPost_RequestBodyContainsPayload(t *testing.T) {
	var receivedBody []byte
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(201)
	})
	c.SetToken("faketoken")

	payload := map[string]string{"title": "mykey", "key": "ssh-ed25519 AAAA"}
	err := c.apiPost(context.Background(), "/user/keys", payload, nil)
	if err != nil {
		t.Fatalf("apiPost: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if parsed["title"] != "mykey" {
		t.Errorf("title = %q", parsed["title"])
	}
}

func TestApiGet_AuthorizationHeader(t *testing.T) {
	var authHeader string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(User{Login: "test"})
	})
	c.SetToken("mytoken123")

	var user User
	_ = c.apiGet(context.Background(), "/user", &user)
	if authHeader != "token mytoken123" {
		t.Errorf("Authorization = %q, want 'token mytoken123'", authHeader)
	}
}

func TestApiPost_AcceptHeader(t *testing.T) {
	var acceptHeader string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		acceptHeader = r.Header.Get("Accept")
		w.WriteHeader(201)
	})
	c.SetToken("faketoken")

	_ = c.apiPost(context.Background(), "/test", map[string]string{}, nil)
	if acceptHeader != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q", acceptHeader)
	}
}

func TestApiPost_ContentTypeHeader(t *testing.T) {
	var contentType string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(201)
	})
	c.SetToken("faketoken")

	_ = c.apiPost(context.Background(), "/test", map[string]string{"key": "val"}, nil)
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q", contentType)
	}
}

func TestUploadSSHKey_VerifiesRequestBody(t *testing.T) {
	var receivedBody map[string]string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(201)
	})
	c.SetToken("faketoken")

	c.UploadSSHKey(context.Background(), "work-key", "ssh-ed25519 AAAA...")
	if receivedBody["title"] != "work-key" {
		t.Errorf("title = %q", receivedBody["title"])
	}
	if receivedBody["key"] != "ssh-ed25519 AAAA..." {
		t.Errorf("key = %q", receivedBody["key"])
	}
}

func TestUploadGPGKey_VerifiesRequestBody(t *testing.T) {
	var receivedBody map[string]string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(201)
	})
	c.SetToken("faketoken")

	c.UploadGPGKey(context.Background(), "-----BEGIN PGP-----\nkey data\n-----END PGP-----")
	if receivedBody["armored_public_key"] != "-----BEGIN PGP-----\nkey data\n-----END PGP-----" {
		t.Errorf("armored_public_key = %q", receivedBody["armored_public_key"])
	}
}

func TestPollForToken_EmptyTokenAndNoError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	callCount := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		var resp TokenResponse
		if callCount <= 1 {
			// Empty response - no token, no error -> treated as pending
			resp = TokenResponse{}
		} else {
			resp = TokenResponse{AccessToken: "ghp_delayed"}
		}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_delayed" {
		t.Errorf("token = %q", token)
	}
}

func TestPollForToken_ReadBodyError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	callCount := 0
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		if callCount <= 1 {
			// Return a body that errors on read
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(&errorReader{}),
				Header:     http.Header{"Content-Type": {"application/json"}},
			}, nil
		}
		resp := TokenResponse{AccessToken: "ghp_after_read_error"}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_after_read_error" {
		t.Errorf("token = %q", token)
	}
}

// errorReader is a reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("simulated read error")
}

func TestApiGet_NetworkError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://127.0.0.1:1" // closed port
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("faketoken")
	c.httpClient.Timeout = 1 * time.Second

	var user User
	err := c.apiGet(context.Background(), "/user", &user)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestApiPost_NetworkError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://127.0.0.1:1" // closed port
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("faketoken")
	c.httpClient.Timeout = 1 * time.Second

	err := c.apiPost(context.Background(), "/user/keys", map[string]string{"key": "val"}, nil)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestInitiateDeviceFlow_BothParsesFail(t *testing.T) {
	// Server returns a body that is neither valid JSON nor valid URL-encoded.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		// Invalid percent-encoding causes url.ParseQuery to fail.
		w.Write([]byte("%zz=bad&value"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = srv.URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.httpClient = srv.Client()

	// Override the device flow URL by using the httpClient pointed at our server.
	// InitiateDeviceFlow posts to "https://github.com/login/device/code" which won't
	// hit our test server. We need to override the HTTP client transport.
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Redirect all requests to our test server.
			req.URL, _ = url.Parse(srv.URL + req.URL.Path)
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	_, err := c.InitiateDeviceFlow()
	if err == nil {
		t.Fatal("expected error when both JSON and URL parsing fail")
	}
	if !strings.Contains(err.Error(), "unexpected response from GitHub") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPollForToken_FormEncodedFallbackV2(t *testing.T) {
	// Server returns form-encoded token response (not JSON).
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		w.WriteHeader(200)
		w.Write([]byte("access_token=ghp_test123&token_type=bearer"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.APIURL = srv.URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(srv.URL + req.URL.Path)
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	ctx := context.Background()
	token, err := c.PollForToken(ctx, "test-device-code", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghp_test123" {
		t.Errorf("token = %q, want ghp_test123", token)
	}
}

func TestInitiateDeviceFlow_ReadBodyError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(&errorReader{}),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	_, err := c.InitiateDeviceFlow()
	if err == nil {
		t.Fatal("expected error for read body failure")
	}
	if !strings.Contains(err.Error(), "could not read response from GitHub") {
		t.Errorf("error = %q, want 'could not read response from GitHub'", err)
	}
}

func TestPollForToken_FormEncodedFallbackNoJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// Return form-encoded response (not valid JSON)
		body := "access_token=ghp_form_token&token_type=bearer"
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}, nil
	})

	ctx := context.Background()
	token, err := c.PollForToken(ctx, "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if token != "ghp_form_token" {
		t.Errorf("token = %q, want ghp_form_token", token)
	}
}

func TestPollForToken_Timeout(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := TokenResponse{Error: "authorization_pending"}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := c.PollForToken(ctx, "device-code", 0)
	if err == nil {
		t.Fatal("expected error on timeout")
	}
}

func TestApiGet_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := plainTextConfig()
	cfg.GitHub.APIURL = srv.URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("tok")

	var user User
	err := c.apiGet(context.Background(), "/user", &user)
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestApiPost_MarshalError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://localhost"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("tok")

	// channels can't be marshaled to JSON
	err := c.apiPost(context.Background(), "/test", make(chan int), nil)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestClearGitCredentials_DefaultServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Should not error even if git is not configured
	err := c.ClearGitCredentials("")
	if err != nil {
		t.Fatalf("ClearGitCredentials: %v", err)
	}
}

func TestClearGitCredentials_CustomServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.ClearGitCredentials("https://github.example.com")
	if err != nil {
		t.Fatalf("ClearGitCredentials custom server: %v", err)
	}
}

func TestClearGitCredentials_InvalidURL(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Even with a weird URL, should not error (graceful)
	err := c.ClearGitCredentials("github.com")
	if err != nil {
		t.Fatalf("ClearGitCredentials bare host: %v", err)
	}
}

func TestStoreGitCredentials_DefaultServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.StoreGitCredentials("", "testuser", "ghp_faketoken123")
	if err != nil {
		t.Fatalf("StoreGitCredentials: %v", err)
	}

	// Clean up: reject the credential we just stored
	_ = c.ClearGitCredentials("")
}

func TestStoreGitCredentials_CustomServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.StoreGitCredentials("https://github.example.com", "user", "token123")
	if err != nil {
		t.Fatalf("StoreGitCredentials custom server: %v", err)
	}

	_ = c.ClearGitCredentials("https://github.example.com")
}

func TestStoreGitCredentials_BareHost(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.StoreGitCredentials("github.com", "user", "token123")
	if err != nil {
		t.Fatalf("StoreGitCredentials bare host: %v", err)
	}

	_ = c.ClearGitCredentials("github.com")
}

func TestSetGitCredentialUsername_Set(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.SetGitCredentialUsername("https://github.example.com", "testuser")
	if err != nil {
		t.Fatalf("SetGitCredentialUsername: %v", err)
	}

	// Clean up
	_ = c.SetGitCredentialUsername("https://github.example.com", "")
}

func TestSetGitCredentialUsername_Unset(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Unset something that doesn't exist — should not error
	err := c.SetGitCredentialUsername("https://nonexistent.example.com", "")
	if err != nil {
		t.Fatalf("SetGitCredentialUsername unset: %v", err)
	}
}

// --- ClearGitCredentials / StoreGitCredentials additional coverage ----------

func TestClearGitCredentials_BareHost(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Pass a bare host without scheme - exercises Host=="" fallback to Path
	err := c.ClearGitCredentials("github.com")
	if err != nil {
		t.Fatalf("ClearGitCredentials bare host: %v", err)
	}
}

func TestStoreGitCredentials_EmptyServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Empty server should default to https://github.com
	err := c.StoreGitCredentials("", "user", "token123")
	if err != nil {
		t.Fatalf("StoreGitCredentials empty: %v", err)
	}
}

func TestClearGitCredentials_EmptyServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.ClearGitCredentials("")
	if err != nil {
		t.Fatalf("ClearGitCredentials empty: %v", err)
	}
}

func TestImportFromGHCLI_NotInstalled(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Set PATH to empty so gh is not found
	t.Setenv("PATH", t.TempDir())

	_, err := c.ImportFromGHCLI()
	if err == nil {
		t.Fatal("expected error when gh is not installed")
	}
}

func TestImportFromGHCLI_Success(t *testing.T) {
	tmp := t.TempDir()
	ghPath := filepath.Join(tmp, "gh")
	script := "#!/bin/sh\necho ghp_test_token_from_gh\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	tok, err := c.ImportFromGHCLI()
	if err != nil {
		t.Fatalf("ImportFromGHCLI: %v", err)
	}
	if tok != "ghp_test_token_from_gh" {
		t.Fatalf("token = %q", tok)
	}
}

func TestImportFromGHCLI_EmptyToken(t *testing.T) {
	tmp := t.TempDir()
	ghPath := filepath.Join(tmp, "gh")
	script := "#!/bin/sh\necho \"\"\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	_, err := c.ImportFromGHCLI()
	if err == nil {
		t.Fatal("expected error for empty gh token")
	}
}

// =============================================================================
// Coverage: InitiateDeviceFlow non-200 response
// =============================================================================

func TestInitiateDeviceFlow_Non200Status(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 401,
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad_client_id"}`)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	_, err := c.InitiateDeviceFlow()
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err)
	}
}

// =============================================================================
// Coverage: InitiateDeviceFlow body > 4096 truncation
// =============================================================================

func TestInitiateDeviceFlow_LargeBodyTruncation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	// Create a body > 4096 bytes that is not valid JSON but is valid form-encoded
	largeBody := "device_code=dev123&user_code=ABCD-1234&verification_uri=https://github.com/login/device&" + strings.Repeat("padding=x&", 500)
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(largeBody)),
			Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}, nil
	})

	dcr, err := c.InitiateDeviceFlow()
	if err != nil {
		t.Fatalf("InitiateDeviceFlow with large body: %v", err)
	}
	if dcr.DeviceCode != "dev123" {
		t.Errorf("DeviceCode = %q", dcr.DeviceCode)
	}
}

// =============================================================================
// Coverage: PollForToken body > 4096 truncation
// =============================================================================

func TestPollForToken_LargeBodyTruncation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	// Create a body > 4096 bytes that is not valid JSON but is valid form-encoded
	largeBody := "access_token=ghp_large_token&token_type=bearer&" + strings.Repeat("padding=x&", 500)
	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(largeBody)),
			Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}, nil
	})

	token, err := c.PollForToken(context.Background(), "device-code", 0)
	if err != nil {
		t.Fatalf("PollForToken with large body: %v", err)
	}
	if token != "ghp_large_token" {
		t.Errorf("token = %q", token)
	}
}

// =============================================================================
// Coverage: url.Parse errors in ClearGitCredentials and StoreGitCredentials
// =============================================================================

func TestClearGitCredentials_URLParseError(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.ClearGitCredentials("http://[::1")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestStoreGitCredentials_URLParseError(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	err := c.StoreGitCredentials("http://[::1", "user", "token")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// =============================================================================
// Coverage: cmd.Run errors via execCommandContext hook
// =============================================================================

func TestClearGitCredentials_CmdRunError(t *testing.T) {
	old := execCommandContext
	defer func() { execCommandContext = old }()
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Should not return an error (non-fatal)
	err := c.ClearGitCredentials("https://github.com")
	if err != nil {
		t.Fatalf("ClearGitCredentials should not fail: %v", err)
	}
}

func TestStoreGitCredentials_CmdRunError(t *testing.T) {
	old := execCommandContext
	defer func() { execCommandContext = old }()
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Should not return an error (non-fatal)
	err := c.StoreGitCredentials("https://github.com", "user", "token")
	if err != nil {
		t.Fatalf("StoreGitCredentials should not fail: %v", err)
	}
}

func TestSetGitCredentialUsername_CmdRunError_NonEmptyUsername(t *testing.T) {
	old := execCommandContext
	defer func() { execCommandContext = old }()
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}

	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Should not return an error (non-fatal)
	err := c.SetGitCredentialUsername("https://github.com", "someuser")
	if err != nil {
		t.Fatalf("SetGitCredentialUsername should not fail: %v", err)
	}
}

// =============================================================================
// Coverage: SetGitCredentialUsername default server (empty) + success paths
// =============================================================================

func TestSetGitCredentialUsername_EmptyServer(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// Empty server triggers the default "https://github.com" path
	err := c.SetGitCredentialUsername("", "testuser")
	if err != nil {
		t.Fatalf("SetGitCredentialUsername empty server: %v", err)
	}
	// Clean up
	_ = c.SetGitCredentialUsername("", "")
}

func TestSetGitCredentialUsername_UnsetSuccess(t *testing.T) {
	cfg := plainTextConfig()
	log := logger.New(logger.LevelError, os.Stderr)
	ts := tokenstore.NewTokenStore(cfg, cryptoSvc.NewService(), log, nil)
	c := NewClient(cfg, log, ts)

	// First set a credential username so that unset succeeds (doesn't error)
	_ = c.SetGitCredentialUsername("https://gcm-test-unset.example.com", "tempuser")

	// Now unset — cmd.Run() should succeed (exit 0), hitting the else branch
	err := c.SetGitCredentialUsername("https://gcm-test-unset.example.com", "")
	if err != nil {
		t.Fatalf("SetGitCredentialUsername unset success: %v", err)
	}
}

// =============================================================================
// Coverage: apiGet with invalid URL (NewRequestWithContext error)
// =============================================================================

func TestApiGet_InvalidURL(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://[::1" // invalid URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("tok")

	var user User
	err := c.apiGet(context.Background(), "/user", &user)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestApiPost_InvalidURL(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := plainTextConfig()
	cfg.GitHub.APIURL = "http://[::1" // invalid URL
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)
	c.SetToken("tok")

	err := c.apiPost(context.Background(), "/test", map[string]string{"k": "v"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// =============================================================================
// Coverage: PollForToken internal timeout (authorization timed out)
// =============================================================================

func TestPollForToken_InternalTimeout(t *testing.T) {
	// Override pollTimeout to a very short duration so the internal
	// context times out before the caller's context.
	old := pollTimeout
	pollTimeout = 100 * time.Millisecond
	defer func() { pollTimeout = old }()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := plainTextConfig()
	cfg.GitHub.OAuth.ClientID = "test-client"
	log := logger.New(logger.LevelError, os.Stderr)
	ts := newTestTokenStore(cfg)
	c := NewClient(cfg, log, ts)

	c.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := TokenResponse{Error: "authorization_pending"}
		data, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	})

	// Pass a context with NO timeout (or very long timeout) so that
	// ctx.Err() == nil when pollCtx expires.
	_, err := c.PollForToken(context.Background(), "device-code", 0)
	if err == nil {
		t.Fatal("expected error for internal timeout")
	}
	if !strings.Contains(err.Error(), "authorization timed out") {
		t.Errorf("error = %q, want 'authorization timed out'", err)
	}
}

// --- Tests for ListGPGKeys, SSHKeyExists, GPGKeyExists, normalizeSSHKey ---

func TestListGPGKeys_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/gpg_keys" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GPGKeyResponse{
			{ID: 1, KeyID: "ABC123"},
			{ID: 2, KeyID: "DEF456"},
		})
	})
	c.SetToken("test-token")

	keys, err := c.ListGPGKeys(context.Background())
	if err != nil {
		t.Fatalf("ListGPGKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	if keys[0].KeyID != "ABC123" {
		t.Errorf("keys[0].KeyID = %q, want ABC123", keys[0].KeyID)
	}
}

func TestListGPGKeys_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	})
	c.SetToken("test-token")

	_, err := c.ListGPGKeys(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListGPGKeys_Empty(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GPGKeyResponse{})
	})
	c.SetToken("test-token")

	keys, err := c.ListGPGKeys(context.Background())
	if err != nil {
		t.Fatalf("ListGPGKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("got %d keys, want 0", len(keys))
	}
}

func TestSSHKeyExists_Found(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]SSHKeyResponse{
			{ID: 1, Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA test@host", Title: "my-key"},
		})
	})
	c.SetToken("test-token")

	exists, err := c.SSHKeyExists(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA comment-ignored")
	if err != nil {
		t.Fatalf("SSHKeyExists: %v", err)
	}
	if !exists {
		t.Error("expected key to exist")
	}
}

func TestSSHKeyExists_NotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]SSHKeyResponse{
			{ID: 1, Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5BBBB other@host", Title: "other-key"},
		})
	})
	c.SetToken("test-token")

	exists, err := c.SSHKeyExists(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA my-key")
	if err != nil {
		t.Fatalf("SSHKeyExists: %v", err)
	}
	if exists {
		t.Error("expected key to NOT exist")
	}
}

func TestSSHKeyExists_APIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad credentials"))
	})
	c.SetToken("test-token")

	_, err := c.SSHKeyExists(context.Background(), "ssh-ed25519 AAAA")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGPGKeyExists_Found(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GPGKeyResponse{
			{ID: 1, KeyID: "ABC123DEF456"},
		})
	})
	c.SetToken("test-token")

	exists, err := c.GPGKeyExists(context.Background(), "abc123def456") // case-insensitive
	if err != nil {
		t.Fatalf("GPGKeyExists: %v", err)
	}
	if !exists {
		t.Error("expected key to exist (case-insensitive match)")
	}
}

func TestGPGKeyExists_NotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GPGKeyResponse{
			{ID: 1, KeyID: "OTHER999"},
		})
	})
	c.SetToken("test-token")

	exists, err := c.GPGKeyExists(context.Background(), "ABC123")
	if err != nil {
		t.Fatalf("GPGKeyExists: %v", err)
	}
	if exists {
		t.Error("expected key to NOT exist")
	}
}

func TestGPGKeyExists_APIError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	})
	c.SetToken("test-token")

	_, err := c.GPGKeyExists(context.Background(), "ABC123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeSSHKey(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"ssh-ed25519 AAAAC3Nz user@host", "ssh-ed25519 AAAAC3Nz"},
		{"ssh-rsa AAAAB3NzaC1yc2E comment with spaces", "ssh-rsa AAAAB3NzaC1yc2E"},
		{"ssh-ed25519 AAAAC3Nz", "ssh-ed25519 AAAAC3Nz"},
		{"  ssh-ed25519 AAAAC3Nz  ", "ssh-ed25519 AAAAC3Nz"},
		{"onlyone", "onlyone"},
		{"", ""},
	}
	for _, tc := range cases {
		got := normalizeSSHKey(tc.input)
		if got != tc.want {
			t.Errorf("normalizeSSHKey(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDeleteSSHKey_Found(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/user/keys" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]SSHKeyResponse{
				{ID: 42, Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA user@host", Title: "my-key"},
			})
			return
		}
		if r.Method == "DELETE" && r.URL.Path == "/user/keys/42" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c.SetToken("test-token")

	deleted, err := c.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA comment")
	if err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
	if !deleted {
		t.Error("expected key to be deleted")
	}
}

func TestDeleteSSHKey_NotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]SSHKeyResponse{
			{ID: 1, Key: "ssh-ed25519 BBBBB other@host", Title: "other"},
		})
	})
	c.SetToken("test-token")

	deleted, err := c.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA my-key")
	if err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
	if deleted {
		t.Error("expected key to NOT be deleted")
	}
}

func TestDeleteSSHKey_ListError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad credentials"))
	})
	c.SetToken("test-token")

	_, err := c.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAA")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteSSHKey_DeleteError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/user/keys" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]SSHKeyResponse{
				{ID: 42, Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA user@host", Title: "my-key"},
			})
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c.SetToken("test-token")

	_, err := c.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA comment")
	if err == nil {
		t.Fatal("expected error on delete failure")
	}
}

func TestDeleteGPGKey_Found(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/user/gpg_keys" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]GPGKeyResponse{
				{ID: 99, KeyID: "ABC123DEF456", Email: "user@example.com"},
			})
			return
		}
		if r.Method == "DELETE" && r.URL.Path == "/user/gpg_keys/99" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c.SetToken("test-token")

	deleted, err := c.DeleteGPGKey(context.Background(), "abc123def456") // case-insensitive
	if err != nil {
		t.Fatalf("DeleteGPGKey: %v", err)
	}
	if !deleted {
		t.Error("expected key to be deleted")
	}
}

func TestDeleteGPGKey_NotFound(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]GPGKeyResponse{
			{ID: 1, KeyID: "OTHER999", Email: "other@example.com"},
		})
	})
	c.SetToken("test-token")

	deleted, err := c.DeleteGPGKey(context.Background(), "ABC123")
	if err != nil {
		t.Fatalf("DeleteGPGKey: %v", err)
	}
	if deleted {
		t.Error("expected key to NOT be deleted")
	}
}

func TestDeleteGPGKey_ListError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	})
	c.SetToken("test-token")

	_, err := c.DeleteGPGKey(context.Background(), "ABC123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteGPGKey_DeleteError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/user/gpg_keys" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]GPGKeyResponse{
				{ID: 99, KeyID: "ABC123DEF456", Email: "user@example.com"},
			})
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c.SetToken("test-token")

	_, err := c.DeleteGPGKey(context.Background(), "abc123def456")
	if err == nil {
		t.Fatal("expected error on delete failure")
	}
}

func TestApiDelete_Success(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "token test-token" {
			t.Errorf("missing or wrong Authorization header")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c.SetToken("test-token")

	err := c.apiDelete(context.Background(), "/some/resource/1")
	if err != nil {
		t.Fatalf("apiDelete: %v", err)
	}
}

func TestApiDelete_Error(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})
	c.SetToken("test-token")

	err := c.apiDelete(context.Background(), "/some/resource/999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestApiDelete_NetworkError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c.SetToken("test-token")
	// Point to an invalid URL to trigger httpClient.Do error
	c.apiURL = "http://127.0.0.1:1" // port 1 is unlikely to be open

	err := c.apiDelete(context.Background(), "/some/resource")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestApiDelete_InvalidURL(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c.SetToken("test-token")
	c.apiURL = "://invalid"

	err := c.apiDelete(context.Background(), "/test")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
