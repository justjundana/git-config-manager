package auth

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
)

const defaultGitCredentialTimeout = 5 * time.Second

// CommandRunner runs a command with stdin and returns stdout/stderr.
type CommandRunner func(ctx context.Context, stdin string, name string, args ...string) (string, string, error)

// ExternalInspector inspects and mutates non-GCM Git credentials.
type ExternalInspector interface {
	InspectGitCredential(ctx context.Context, def providerpkg.Definition, username string) (GitCredentialInspection, error)
	RejectGitCredential(ctx context.Context, def providerpkg.Definition, username string) error
}

// GitCredentialInspection is the raw inspection result for Git credential helpers.
type GitCredentialInspection struct {
	Credential         CredentialStatus         `json:"credential"`
	Helpers            []CredentialHelperStatus `json:"helpers,omitempty"`
	ConfiguredUsername string                   `json:"configured_username,omitempty"`
	GCMConfigured      bool                     `json:"gcm_configured"`
	Error              string                   `json:"error,omitempty"`
}

// GitCredentialInspector queries Git's credential helper chain without persisting credentials.
type GitCredentialInspector struct {
	Timeout time.Duration
	Runner  CommandRunner
}

// NewGitCredentialInspector creates a Git credential inspector with the default command runner.
func NewGitCredentialInspector() *GitCredentialInspector {
	return &GitCredentialInspector{Timeout: defaultGitCredentialTimeout, Runner: runCommand}
}

// InspectGitCredential asks Git what credential would be used for the provider host.
func (i *GitCredentialInspector) InspectGitCredential(ctx context.Context, def providerpkg.Definition, username string) (GitCredentialInspection, error) {
	server := strings.TrimRight(def.CredentialServer(), "/")
	protocol, host, err := credentialProtocolHost(server)
	if err != nil {
		return GitCredentialInspection{}, err
	}

	helpers := i.collectHelpers(ctx, server)
	configuredUsername := i.configuredUsername(ctx, server)
	if username == "" {
		username = configuredUsername
	}

	input := fmt.Sprintf("protocol=%s\nhost=%s\n", sanitizeCredentialValue(protocol), sanitizeCredentialValue(host))
	if username != "" {
		input += fmt.Sprintf("username=%s\n", sanitizeCredentialValue(username))
	}
	input += "\n"

	out, _, _ := i.run(ctx, input, "git", "credential", "fill")
	fields := parseCredentialOutput(out)
	credentialUsername := fields["username"]
	if credentialUsername == "" {
		credentialUsername = username
	}
	secret := fields["password"]

	credential := CredentialStatus{
		Type:      "https",
		Source:    SourceUnknown,
		Ownership: OwnershipUnknown,
		State:     StateUnauthenticated,
		Host:      host,
		Username:  credentialUsername,
		Helpers:   helpers,
	}
	inspection := GitCredentialInspection{
		Credential:         credential,
		Helpers:            helpers,
		ConfiguredUsername: configuredUsername,
		GCMConfigured:      helpersContainGCM(helpers),
	}

	if secret == "" {
		return inspection, nil
	}

	inspection.Credential.Present = true
	inspection.Credential.Secret = secret
	inspection.Credential.Source = classifyCredentialSource(helpers)
	if inspection.Credential.Source == SourceGCMStore {
		inspection.Credential.Exportable = false
		inspection.Credential.Ownership = OwnershipGCM
		inspection.Credential.State = StateAuthenticatedGCM
		return inspection, nil
	}
	inspection.Credential.Exportable = true
	inspection.Credential.Ownership = OwnershipExternal
	inspection.Credential.State = StateAuthenticatedExternal
	return inspection, nil
}

// RejectGitCredential asks Git's credential chain to erase a credential for the provider host.
func (i *GitCredentialInspector) RejectGitCredential(ctx context.Context, def providerpkg.Definition, username string) error {
	server := strings.TrimRight(def.CredentialServer(), "/")
	protocol, host, err := credentialProtocolHost(server)
	if err != nil {
		return err
	}

	input := fmt.Sprintf("protocol=%s\nhost=%s\n", sanitizeCredentialValue(protocol), sanitizeCredentialValue(host))
	if username != "" {
		input += fmt.Sprintf("username=%s\n", sanitizeCredentialValue(username))
	}
	input += "\n"
	_, stderr, err := i.run(ctx, input, "git", "credential", "reject")
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return fmt.Errorf("git credential reject failed: %s", strings.TrimSpace(stderr))
		}
		return fmt.Errorf("git credential reject failed: %w", err)
	}
	return nil
}

func (i *GitCredentialInspector) collectHelpers(ctx context.Context, server string) []CredentialHelperStatus {
	var helpers []CredentialHelperStatus
	globalKey := "credential.helper"
	scopedKey := fmt.Sprintf("credential.%s.helper", server)
	helpers = append(helpers, i.collectHelperScope(ctx, "global", globalKey)...)
	helpers = append(helpers, i.collectHelperScope(ctx, "host", scopedKey)...)
	return helpers
}

func (i *GitCredentialInspector) collectHelperScope(ctx context.Context, scope, key string) []CredentialHelperStatus {
	out, _, err := i.run(ctx, "", "git", "config", "--global", "--get-all", key)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil
	}
	var helpers []CredentialHelperStatus
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" && strings.TrimSpace(out) == "" {
			continue
		}
		source := ClassifyHelper(line)
		helpers = append(helpers, CredentialHelperStatus{
			Scope:  scope,
			Key:    key,
			Value:  line,
			Source: source,
			GCM:    source == SourceGCMStore,
		})
	}
	return helpers
}

func (i *GitCredentialInspector) configuredUsername(ctx context.Context, server string) string {
	key := fmt.Sprintf("credential.%s.username", server)
	out, _, err := i.run(ctx, "", "git", "config", "--global", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (i *GitCredentialInspector) run(ctx context.Context, stdin string, name string, args ...string) (string, string, error) {
	runner := i.Runner
	if runner == nil {
		runner = runCommand
	}
	timeout := i.Timeout
	if timeout <= 0 {
		timeout = defaultGitCredentialTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runner(runCtx, stdin, name, args...)
}

func runCommand(ctx context.Context, stdin string, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func credentialProtocolHost(server string) (string, string, error) {
	if server == "" {
		return "https", "", fmt.Errorf("credential server cannot be empty")
	}
	parsed, err := url.Parse(server)
	if err != nil {
		return "", "", fmt.Errorf("parsing credential server: %w", err)
	}
	protocol := parsed.Scheme
	if protocol == "" {
		protocol = "https"
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Path
	}
	if host == "" {
		return "", "", fmt.Errorf("credential server %q has no host", server)
	}
	return protocol, providerpkg.NormalizeHost(host), nil
}

func parseCredentialOutput(output string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	return fields
}

// ClassifyHelper maps a Git credential helper command to a stable source name.
func ClassifyHelper(value string) CredentialSource {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case lower == "":
		return SourceUnknown
	case strings.Contains(lower, "gcm") && strings.Contains(lower, "credential-helper"):
		return SourceGCMStore
	case strings.Contains(lower, "gh auth git-credential") || strings.Contains(lower, "github-cli"):
		return SourceGHCLI
	case strings.Contains(lower, "osxkeychain"):
		return SourceOSXKeychain
	case strings.Contains(lower, "wincred"):
		return SourceWinCred
	case strings.Contains(lower, "libsecret") || strings.Contains(lower, "gnome-keyring"):
		return SourceLibsecret
	case strings.Contains(lower, "manager-core") || strings.Contains(lower, "manager"):
		return SourceGitCredentialManager
	case strings.Contains(lower, "cache"):
		return SourceGitCredentialCache
	case strings.Contains(lower, "store"):
		return SourceGitCredentialStore
	case strings.Contains(lower, "credential"):
		return SourceGitCredential
	default:
		return SourceUnknown
	}
}

func classifyCredentialSource(helpers []CredentialHelperStatus) CredentialSource {
	for i := len(helpers) - 1; i >= 0; i-- {
		if helpers[i].Source != SourceUnknown {
			return helpers[i].Source
		}
	}
	return SourceGitCredential
}

func helpersContainGCM(helpers []CredentialHelperStatus) bool {
	for _, helper := range helpers {
		if helper.GCM {
			return true
		}
	}
	return false
}

func sanitizeCredentialValue(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}
