package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(config.ProviderConfig{APIURL: server.URL}, logger.New(logger.LevelError, os.Stderr))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNewClientDefaultsAPIURL(t *testing.T) {
	client := NewClient(config.ProviderConfig{}, logger.New(logger.LevelError, io.Discard))
	if client.apiURL != "https://gitlab.com/api/v4" {
		t.Fatalf("apiURL = %q", client.apiURL)
	}
}

func TestWithTokenSetReturnsTokenScopedClone(t *testing.T) {
	client := NewClient(config.ProviderConfig{APIURL: "https://gitlab.example/api/v4"}, logger.New(logger.LevelError, io.Discard))
	client.SetToken("original")

	clone := client.WithTokenSet(provider.TokenSet{AccessToken: "scoped", AuthMethod: provider.AuthMethodOAuthDevice, TokenType: "bearer"})
	if clone == client {
		t.Fatal("WithTokenSet returned original client")
	}
	if clone.token.AccessToken != "scoped" || !clone.token.Bearer() {
		t.Fatalf("clone token = %+v, want scoped bearer", clone.token)
	}
	if client.token.AccessToken != "original" {
		t.Fatalf("original token = %q, want original", client.token.AccessToken)
	}
	if clone.httpClient != client.httpClient {
		t.Fatal("clone should share HTTP client")
	}
}

func TestGetUser_UsesPrivateTokenForPAT(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "glpat" {
			t.Fatalf("PRIVATE-TOKEN = %q", got)
		}
		json.NewEncoder(w).Encode(User{Username: "jane", Name: "Jane Doe"})
	})
	client.SetToken("glpat")

	user, err := client.GetUser(context.Background())
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Username != "jane" {
		t.Fatalf("Username = %q", user.Username)
	}
}

func TestGetUser_UsesBearerForOAuthToken(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
			t.Fatalf("Authorization = %q", got)
		}
		json.NewEncoder(w).Encode(User{Username: "oauth-user"})
	})
	client.SetTokenSet(provider.TokenSet{AccessToken: "oauth-token", AuthMethod: provider.AuthMethodOAuthDevice, TokenType: "bearer"})

	if _, err := client.GetUser(context.Background()); err != nil {
		t.Fatalf("GetUser: %v", err)
	}
}

func TestVerifyToken(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(User{Username: "verified"})
	})
	client.SetToken("glpat")

	user, err := client.VerifyToken(context.Background())
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if user.Username != "verified" {
		t.Fatalf("Username = %q", user.Username)
	}
}

func TestUploadSSHKey(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/user/keys" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["title"] != "work" || payload["key"] == "" {
			t.Fatalf("payload = %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
	})
	client.SetToken("glpat")

	if err := client.UploadSSHKey(context.Background(), "work", "ssh-ed25519 AAAA"); err != nil {
		t.Fatalf("UploadSSHKey: %v", err)
	}
}

func TestSSHKeyExists_NormalizesKeyComments(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SSHKeyResponse{{ID: 1, Key: "ssh-ed25519 AAAA old-comment"}})
	})
	client.SetToken("glpat")

	exists, err := client.SSHKeyExists(context.Background(), "ssh-ed25519 AAAA new-comment")
	if err != nil {
		t.Fatalf("SSHKeyExists: %v", err)
	}
	if !exists {
		t.Fatal("expected key to exist")
	}
}

func TestSSHKeyExistsFalseAndListErrors(t *testing.T) {
	mode := ""
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "badjson":
			w.Write([]byte("not-json"))
		case "status":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"nope"}`))
		default:
			json.NewEncoder(w).Encode([]SSHKeyResponse{{ID: 1, Key: "ssh-ed25519 OTHER"}})
		}
	})
	client.SetToken("glpat")

	exists, err := client.SSHKeyExists(context.Background(), "ssh-ed25519 AAAA comment")
	if err != nil {
		t.Fatalf("SSHKeyExists: %v", err)
	}
	if exists {
		t.Fatal("unexpected SSH key match")
	}
	mode = "badjson"
	if _, err := client.ListSSHKeys(context.Background()); err == nil {
		t.Fatal("expected ListSSHKeys decode error")
	}
	mode = "status"
	if _, err := client.SSHKeyExists(context.Background(), "ssh-ed25519 AAAA"); err == nil {
		t.Fatal("expected SSHKeyExists list error")
	}
	if _, err := client.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAA"); err == nil {
		t.Fatal("expected DeleteSSHKey list error")
	}
}

func TestDeleteSSHKey(t *testing.T) {
	deleteCalled := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/keys":
			json.NewEncoder(w).Encode([]SSHKeyResponse{{ID: 7, Key: "ssh-ed25519 AAAA old-comment"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/user/keys/7":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	client.SetToken("glpat")

	deleted, err := client.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAA new-comment")
	if err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
	if !deleted || !deleteCalled {
		t.Fatal("expected SSH key to be deleted")
	}
}

func TestDeleteSSHKeyFalseAndDeleteError(t *testing.T) {
	deleteError := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/keys":
			json.NewEncoder(w).Encode([]SSHKeyResponse{{ID: 7, Key: "ssh-ed25519 OTHER"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/user/keys/7":
			deleteError = true
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message":"boom"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	client.SetToken("glpat")

	deleted, err := client.DeleteSSHKey(context.Background(), "ssh-ed25519 AAAA")
	if err != nil {
		t.Fatalf("DeleteSSHKey false: %v", err)
	}
	if deleted {
		t.Fatal("unexpected SSH delete")
	}
	deleted, err = client.DeleteSSHKey(context.Background(), "ssh-ed25519 OTHER comment")
	if err == nil || !deleteError {
		t.Fatalf("expected SSH delete error, deleted=%v err=%v", deleted, err)
	}
}

func TestUploadGPGKey(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/user/gpg_keys" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["key"] == "" {
			t.Fatalf("payload = %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
	})
	client.SetToken("glpat")

	if err := client.UploadGPGKey(context.Background(), "-----BEGIN PGP PUBLIC KEY BLOCK-----"); err != nil {
		t.Fatalf("UploadGPGKey: %v", err)
	}
}

func TestDeleteGPGKey(t *testing.T) {
	deleteCalled := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/gpg_keys":
			json.NewEncoder(w).Encode([]GPGKeyResponse{{ID: 9, KeyID: "ABC123", Fingerprint: "FINGERPRINT"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/user/gpg_keys/9":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	client.SetToken("glpat")

	deleted, err := client.DeleteGPGKey(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("DeleteGPGKey: %v", err)
	}
	if !deleted || !deleteCalled {
		t.Fatal("expected GPG key to be deleted")
	}
}

func TestGPGKeyExistsAndDeleteBranches(t *testing.T) {
	deleteStatus := http.StatusNoContent
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/gpg_keys":
			json.NewEncoder(w).Encode([]GPGKeyResponse{{ID: 9, KeyID: "ABC123", Fingerprint: "FINGERPRINT"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/user/gpg_keys/9":
			w.WriteHeader(deleteStatus)
			if deleteStatus >= 400 {
				w.Write([]byte(`{"message":"boom"}`))
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	client.SetToken("glpat")

	if exists, err := client.GPGKeyExists(context.Background(), "abc123"); err != nil || !exists {
		t.Fatalf("GPGKeyExists key ID = %v, %v", exists, err)
	}
	if exists, err := client.GPGKeyExists(context.Background(), "fingerprint"); err != nil || !exists {
		t.Fatalf("GPGKeyExists fingerprint = %v, %v", exists, err)
	}
	if exists, err := client.GPGKeyExists(context.Background(), "missing"); err != nil || exists {
		t.Fatalf("GPGKeyExists missing = %v, %v", exists, err)
	}
	if deleted, err := client.DeleteGPGKey(context.Background(), "missing"); err != nil || deleted {
		t.Fatalf("DeleteGPGKey missing = %v, %v", deleted, err)
	}
	deleteStatus = http.StatusInternalServerError
	if deleted, err := client.DeleteGPGKey(context.Background(), "fingerprint"); err == nil || deleted {
		t.Fatalf("expected DeleteGPGKey error, deleted=%v err=%v", deleted, err)
	}
}

func TestGPGListErrors(t *testing.T) {
	mode := "badjson"
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "status":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"nope"}`))
		default:
			w.Write([]byte("not-json"))
		}
	})
	client.SetToken("glpat")

	if _, err := client.ListGPGKeys(context.Background()); err == nil {
		t.Fatal("expected ListGPGKeys decode error")
	}
	mode = "status"
	if _, err := client.GPGKeyExists(context.Background(), "ABC123"); err == nil {
		t.Fatal("expected GPGKeyExists list error")
	}
	if _, err := client.DeleteGPGKey(context.Background(), "ABC123"); err == nil {
		t.Fatal("expected DeleteGPGKey list error")
	}
}

func TestApiError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"forbidden"}`))
	})
	client.SetToken("bad")

	if _, err := client.GetUser(context.Background()); err == nil {
		t.Fatal("expected API error")
	}
}

func TestAPIHelpersErrorBranches(t *testing.T) {
	client := NewClient(config.ProviderConfig{APIURL: "http://gitlab.test"}, logger.New(logger.LevelError, io.Discard))
	client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	client.SetToken("glpat")

	if err := client.apiGet(context.Background(), "/user", &User{}); err == nil {
		t.Fatal("expected apiGet transport error")
	}
	if err := client.apiPost(context.Background(), "/user/keys", map[string]string{"key": "value"}, nil); err == nil {
		t.Fatal("expected apiPost transport error")
	}
	if err := client.apiDelete(context.Background(), "/user/keys/1"); err == nil {
		t.Fatal("expected apiDelete transport error")
	}

	client.apiURL = "http://bad host"
	if err := client.apiGet(context.Background(), "/user", &User{}); err == nil {
		t.Fatal("expected apiGet request error")
	}
	if err := client.apiPost(context.Background(), "/user/keys", map[string]string{"key": "value"}, nil); err == nil {
		t.Fatal("expected apiPost request error")
	}
	if err := client.apiDelete(context.Background(), "/user/keys/1"); err == nil {
		t.Fatal("expected apiDelete request error")
	}
	if err := client.apiPost(context.Background(), "/user/keys", map[string]any{"bad": func() {}}, nil); err == nil {
		t.Fatal("expected apiPost marshal error")
	}
}

func TestAPIPostDecodeResultAndStatusError(t *testing.T) {
	mode := ""
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "badjson":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("not-json"))
		case "status":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"bad"}`))
		default:
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(User{Username: "created"})
		}
	})
	client.SetToken("glpat")

	var user User
	if err := client.apiPost(context.Background(), "/user/keys", map[string]string{"key": "value"}, &user); err != nil {
		t.Fatalf("apiPost result: %v", err)
	}
	if user.Username != "created" {
		t.Fatalf("Username = %q", user.Username)
	}
	mode = "badjson"
	if err := client.apiPost(context.Background(), "/user/keys", map[string]string{"key": "value"}, &user); err == nil {
		t.Fatal("expected apiPost decode error")
	}
	mode = "status"
	if err := client.UploadSSHKey(context.Background(), "title", "ssh-ed25519 AAAA"); err == nil {
		t.Fatal("expected UploadSSHKey status error")
	}
}

func TestNormalizeSSHKeyShortInput(t *testing.T) {
	if got := normalizeSSHKey("  ssh-ed25519  "); got != "ssh-ed25519" {
		t.Fatalf("normalizeSSHKey short = %q", got)
	}
	if got := normalizeSSHKey("  "); got != "" {
		t.Fatalf("normalizeSSHKey blank = %q", got)
	}
}
