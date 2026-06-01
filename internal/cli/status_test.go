package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/container"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

func TestQuickVerifyProviderTokenUsesProviderAuthHeaders(t *testing.T) {
	t.Run("github token header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/user" {
				t.Fatalf("path = %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "token gh-token" {
				t.Fatalf("Authorization = %q", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := quickVerifyProviderToken(
			providerpkg.Definition{ID: providerpkg.GitHubID, APIURL: server.URL},
			providerpkg.TokenSet{AccessToken: "gh-token", AuthMethod: providerpkg.AuthMethodPAT},
		)
		if err != nil {
			t.Fatalf("quickVerifyProviderToken GitHub: %v", err)
		}
	})

	t.Run("gitlab pat header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("PRIVATE-TOKEN"); got != "gl-token" {
				t.Fatalf("PRIVATE-TOKEN = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("Authorization = %q, want empty for PAT", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := quickVerifyProviderToken(
			providerpkg.Definition{ID: providerpkg.GitLabID, APIURL: server.URL},
			providerpkg.TokenSet{AccessToken: "gl-token", AuthMethod: providerpkg.AuthMethodPAT},
		)
		if err != nil {
			t.Fatalf("quickVerifyProviderToken GitLab PAT: %v", err)
		}
	})

	t.Run("gitlab bearer header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer gl-bearer" {
				t.Fatalf("Authorization = %q", got)
			}
			if got := r.Header.Get("PRIVATE-TOKEN"); got != "" {
				t.Fatalf("PRIVATE-TOKEN = %q, want empty for bearer", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := quickVerifyProviderToken(
			providerpkg.Definition{ID: providerpkg.GitLabID, APIURL: server.URL},
			providerpkg.TokenSet{AccessToken: "gl-bearer", AuthMethod: providerpkg.AuthMethodOAuthDevice},
		)
		if err != nil {
			t.Fatalf("quickVerifyProviderToken GitLab bearer: %v", err)
		}
	})
}

func TestQuickVerifyProviderTokenReportsInvalidHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	err := quickVerifyProviderToken(
		providerpkg.Definition{ID: providerpkg.GitHubID, APIURL: server.URL},
		providerpkg.TokenSet{AccessToken: "bad-token", AuthMethod: providerpkg.AuthMethodPAT},
	)
	if err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestStatusVerifyConcurrencyFollowsConfig(t *testing.T) {
	original := ctr
	defer func() { ctr = original }()

	ctr = nil
	if got := statusVerifyConcurrency(); got != 1 {
		t.Fatalf("statusVerifyConcurrency without container = %d, want 1", got)
	}

	ctr = &container.Container{Config: &config.Config{Advanced: config.AdvancedConfig{ParallelOperations: false}}}
	if got := statusVerifyConcurrency(); got != 1 {
		t.Fatalf("statusVerifyConcurrency disabled = %d, want 1", got)
	}

	ctr = &container.Container{Config: &config.Config{Advanced: config.AdvancedConfig{ParallelOperations: true}}}
	if got := statusVerifyConcurrency(); got != 4 {
		t.Fatalf("statusVerifyConcurrency enabled = %d, want 4", got)
	}
}
