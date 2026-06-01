package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"

	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

func TestClassifyHelper(t *testing.T) {
	tests := map[string]CredentialSource{
		"!/usr/local/bin/gcm credential-helper": SourceGCMStore,
		"!gh auth git-credential":               SourceGHCLI,
		"osxkeychain":                           SourceOSXKeychain,
		"wincred":                               SourceWinCred,
		"libsecret":                             SourceLibsecret,
		"manager-core":                          SourceGitCredentialManager,
		"cache --timeout 900":                   SourceGitCredentialCache,
		"store --file ~/.git-credentials":       SourceGitCredentialStore,
		"custom-credential":                     SourceGitCredential,
		"something else":                        SourceUnknown,
		"":                                      SourceUnknown,
	}
	for input, want := range tests {
		if got := ClassifyHelper(input); got != want {
			t.Fatalf("ClassifyHelper(%q)=%s want %s", input, got, want)
		}
	}
}

func TestNewInspectorAndDefaultRunner(t *testing.T) {
	inspector := NewGitCredentialInspector()
	if inspector.Runner == nil || inspector.Timeout != defaultGitCredentialTimeout {
		t.Fatalf("unexpected inspector defaults: %+v", inspector)
	}
	if out, stderr, err := (&GitCredentialInspector{}).run(context.Background(), "ignored", "go", "version"); err != nil || !strings.Contains(out, "go version") {
		t.Fatalf("default inspector run = %q %q %v", out, stderr, err)
	}
	out, stderr, err := runCommand(context.Background(), "ignored", "go", "version")
	if err != nil {
		t.Fatalf("runCommand: %v stderr=%s", err, stderr)
	}
	if !strings.Contains(out, "go version") {
		t.Fatalf("unexpected go version output: %q", out)
	}
}

func TestInspectGitCredential(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", WebURL: "https://github.com", GitHosts: []string{"github.com"}}
	inspector := &GitCredentialInspector{Runner: func(_ context.Context, stdin string, _ string, args ...string) (string, string, error) {
		joined := strings.Join(args, " ")
		switch {
		case joined == "config --global --get-all credential.helper":
			return "osxkeychain\n", "", nil
		case joined == "config --global --get-all credential.https://github.com.helper":
			return "\n!/usr/local/bin/gcm credential-helper\n", "", nil
		case joined == "config --global --get credential.https://github.com.username":
			return "octo\n", "", nil
		case joined == "credential fill":
			if !strings.Contains(stdin, "username=octo") {
				return "", "", fmt.Errorf("missing username in stdin: %q", stdin)
			}
			return "protocol=https\nhost=github.com\nusername=octo\npassword=secret\n", "", nil
		default:
			return "", "", fmt.Errorf("unexpected args: %s", joined)
		}
	}}

	inspection, err := inspector.InspectGitCredential(context.Background(), def, "")
	if err != nil {
		t.Fatalf("InspectGitCredential: %v", err)
	}
	if !inspection.Credential.Present || inspection.Credential.Secret != "secret" || inspection.Credential.Source != SourceGCMStore {
		t.Fatalf("unexpected credential: %+v", inspection.Credential)
	}
	if inspection.Credential.Ownership != OwnershipGCM || inspection.Credential.State != StateAuthenticatedGCM || inspection.Credential.Exportable {
		t.Fatalf("GCM helper credential should be GCM-owned and non-exportable: %+v", inspection.Credential)
	}
	if !inspection.GCMConfigured || len(inspection.Helpers) != 3 {
		t.Fatalf("unexpected helpers: %+v", inspection.Helpers)
	}
}

func TestInspectGitCredentialNoCredential(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", WebURL: "https://github.com", GitHosts: []string{"github.com"}}
	inspector := &GitCredentialInspector{Runner: func(_ context.Context, _ string, _ string, args ...string) (string, string, error) {
		if strings.Join(args, " ") == "credential fill" {
			return "", "fatal: could not read Username", fmt.Errorf("exit status 128")
		}
		return "", "", fmt.Errorf("not found")
	}}

	inspection, err := inspector.InspectGitCredential(context.Background(), def, "")
	if err != nil {
		t.Fatalf("InspectGitCredential: %v", err)
	}
	if inspection.Credential.Present || inspection.Error != "" || inspection.Credential.Error != "" {
		t.Fatalf("expected absent credential without inspection error: %+v", inspection)
	}
}

func TestInspectGitCredentialEmptySuccessfulFill(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", WebURL: "https://github.com", GitHosts: []string{"github.com"}}
	inspector := &GitCredentialInspector{Runner: func(_ context.Context, _ string, _ string, args ...string) (string, string, error) {
		if strings.Join(args, " ") == "credential fill" {
			return "username=octo\n", "", nil
		}
		return "", "", nil
	}}
	inspection, err := inspector.InspectGitCredential(context.Background(), def, "")
	if err != nil {
		t.Fatalf("InspectGitCredential: %v", err)
	}
	if inspection.Credential.Present || inspection.Credential.State != StateUnauthenticated {
		t.Fatalf("expected absent credential: %+v", inspection.Credential)
	}
}

func TestInspectGitCredentialErrorAndHelperBranches(t *testing.T) {
	inspector := &GitCredentialInspector{Runner: func(_ context.Context, _ string, _ string, args ...string) (string, string, error) {
		switch strings.Join(args, " ") {
		case "config --global --get-all credential.helper":
			return "\n", "", fmt.Errorf("empty reset")
		case "config --global --get-all credential.example.com.helper":
			return "custom\n", "", nil
		case "config --global --get credential.example.com.username":
			return "", "", fmt.Errorf("missing")
		case "credential fill":
			return "username=me\npassword=secret\n", "", nil
		default:
			return "", "", fmt.Errorf("unexpected args: %s", strings.Join(args, " "))
		}
	}}
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", WebURL: "example.com"}
	inspection, err := inspector.InspectGitCredential(context.Background(), def, "me")
	if err != nil {
		t.Fatalf("InspectGitCredential: %v", err)
	}
	if inspection.Credential.Source != SourceGitCredential || inspection.GCMConfigured {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
	if len(inspection.Helpers) != 1 {
		t.Fatalf("expected empty reset helper to be skipped: %+v", inspection.Helpers)
	}

	badDef := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	if _, err := inspector.InspectGitCredential(context.Background(), badDef, ""); err == nil {
		t.Fatal("expected empty credential server error")
	}
}

func TestRejectGitCredential(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab", WebURL: "https://gitlab.com", GitHosts: []string{"gitlab.com"}}
	called := false
	inspector := &GitCredentialInspector{Runner: func(_ context.Context, stdin string, _ string, args ...string) (string, string, error) {
		if strings.Join(args, " ") != "credential reject" {
			return "", "", fmt.Errorf("unexpected args")
		}
		called = strings.Contains(stdin, "host=gitlab.com") && strings.Contains(stdin, "username=lab")
		return "", "", nil
	}}

	if err := inspector.RejectGitCredential(context.Background(), def, "lab"); err != nil {
		t.Fatalf("RejectGitCredential: %v", err)
	}
	if !called {
		t.Fatalf("runner was not called with expected credential input")
	}
}

func TestRejectGitCredentialErrors(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", WebURL: "https://github.com", GitHosts: []string{"github.com"}}
	stderrInspector := &GitCredentialInspector{Runner: func(context.Context, string, string, ...string) (string, string, error) {
		return "", "denied", fmt.Errorf("exit status 1")
	}}
	if err := stderrInspector.RejectGitCredential(context.Background(), def, "octo"); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected stderr error, got %v", err)
	}

	plainInspector := &GitCredentialInspector{Runner: func(context.Context, string, string, ...string) (string, string, error) {
		return "", "", fmt.Errorf("exit status 1")
	}}
	if err := plainInspector.RejectGitCredential(context.Background(), def, "octo"); err == nil || !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("expected wrapped command error, got %v", err)
	}

	badDef := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub"}
	if err := plainInspector.RejectGitCredential(context.Background(), badDef, "octo"); err == nil {
		t.Fatal("expected empty credential server error")
	}
}

func TestCredentialProtocolHostAndSourceFallbacks(t *testing.T) {
	protocol, host, err := credentialProtocolHost("gitlab.com")
	if err != nil || protocol != "https" || host != "gitlab.com" {
		t.Fatalf("bare host = %s %s %v", protocol, host, err)
	}
	if _, _, err := credentialProtocolHost("http://[::1"); err == nil {
		t.Fatal("expected URL parse error")
	}
	if _, _, err := credentialProtocolHost("https://"); err == nil {
		t.Fatal("expected missing host error")
	}
	inspector := &GitCredentialInspector{Runner: func(context.Context, string, string, ...string) (string, string, error) {
		return "\n", "", nil
	}}
	if helpers := inspector.collectHelperScope(context.Background(), "global", "credential.helper"); len(helpers) != 0 {
		t.Fatalf("empty helper reset should be skipped: %+v", helpers)
	}
	if got := classifyCredentialSource(nil); got != SourceGitCredential {
		t.Fatalf("empty helper source = %s", got)
	}
	if got := sanitizeCredentialValue("a\r\nb\x00c"); got != "abc" {
		t.Fatalf("sanitizeCredentialValue = %q", got)
	}
}
