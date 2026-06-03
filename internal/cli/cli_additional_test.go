package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/container"
	"github.com/justjundana/git-config-manager/internal/github"
	"github.com/justjundana/git-config-manager/internal/gitlab"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/providerclient"
	templatepkg "github.com/justjundana/git-config-manager/internal/template"
	"github.com/justjundana/git-config-manager/pkg/logger"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func runRootCommand(t *testing.T, args ...string) error {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func setTestStdin(t *testing.T, input string) {
	t.Helper()
	original := os.Stdin
	t.Cleanup(func() { os.Stdin = original })
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	os.Stdin = readEnd
	if _, err := writeEnd.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
}

func setGlobalGitConfig(t *testing.T, key, value string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	cmd := exec.Command("git", "config", "--global", key, value)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config %s: %v\n%s", key, err, output)
	}
}

func globalGitConfig(t *testing.T, key string) (string, bool) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	cmd := exec.Command("git", "config", "--global", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func setUIPromptInput(t *testing.T, input string) {
	t.Helper()
	originalIn := ui.PromptIn
	originalOut := ui.PromptOut
	originalStdin := os.Stdin
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe prompt stdin: %v", err)
	}
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("close prompt stdin writer: %v", err)
	}
	os.Stdin = readEnd
	ui.PromptIn = iotest.OneByteReader(strings.NewReader(input))
	ui.PromptOut = io.Discard
	t.Cleanup(func() {
		ui.PromptIn = originalIn
		ui.PromptOut = originalOut
		os.Stdin = originalStdin
		readEnd.Close()
	})
}

func TestSetContainerAndRootCommandShape(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })

	c := &container.Container{}
	SetContainer(c)
	if ctr != c {
		t.Fatal("SetContainer did not install container")
	}

	root := NewRootCmd()
	if root.Use != "gcm" || !root.SilenceUsage || !root.SilenceErrors {
		t.Fatalf("root command = %+v", root)
	}
	want := []string{"setup", "status", "use", "current", "refresh", "profile", "ssh", "gpg", "connect", "auth", "switch-provider", "github", "gitlab", "init", "template", "backup", "validate", "doctor", "repair", "version", "clean", "credential-helper"}
	for _, name := range want {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Fatalf("missing root command %q: %v", name, err)
		}
	}
}

func TestFormattingParsingAndArgumentHelpers(t *testing.T) {
	bytesCases := map[int64]string{
		12:              "12 B",
		1024:            "1.0 KB",
		5 * 1024 * 1024: "5.0 MB",
	}
	for input, want := range bytesCases {
		if got := formatBytes(input); got != want {
			t.Fatalf("formatBytes(%d) = %q, want %q", input, got, want)
		}
	}

	now := time.Now()
	timeCases := []struct {
		input time.Time
		want  string
	}{
		{now.Add(-10 * time.Second), "just now"},
		{now.Add(-2 * time.Minute), "2m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
		{now.Add(-14 * 24 * time.Hour), "2w ago"},
	}
	for _, tc := range timeCases {
		if got := formatTimeAgo(tc.input); got != tc.want {
			t.Fatalf("formatTimeAgo = %q, want %q", got, tc.want)
		}
	}
	old := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	if got := formatTimeAgo(old); got != "2024-01-02" {
		t.Fatalf("old formatTimeAgo = %q", got)
	}

	if got := padRight("é", 3); got != "é  " {
		t.Fatalf("padRight unicode = %q", got)
	}
	if got := padRight("long", 2); got != "long" {
		t.Fatalf("padRight long = %q", got)
	}

	aliases := parseAliases([]string{"co=checkout", " bad ", "st= status ", "=empty", "x="})
	if aliases["co"] != "checkout" || aliases["st"] != "status" || len(aliases) != 2 {
		t.Fatalf("parseAliases = %#v", aliases)
	}

	parent := &cobra.Command{Use: "gcm"}
	cmd := &cobra.Command{Use: "thing <name>"}
	parent.AddCommand(cmd)
	validator := requireArgs(1)
	if err := validator(cmd, []string{"work"}); err != nil {
		t.Fatalf("requireArgs valid: %v", err)
	}
	if err := validator(cmd, nil); err == nil || !strings.Contains(err.Error(), "missing required argument") {
		t.Fatalf("missing arg error = %v", err)
	}
	if err := validator(cmd, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("extra arg error = %v", err)
	}
}

func TestProviderHelperBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })

	if got := providerNames(nil); got != "none" {
		t.Fatalf("providerNames(nil) = %q", got)
	}
	if got := providerNames([]providerpkg.Definition{{DisplayName: "GitHub"}, {DisplayName: "GitLab"}}); got != "GitHub, GitLab" {
		t.Fatalf("providerNames = %q", got)
	}
	if got := firstProviderHost(providerpkg.Definition{GitHosts: []string{"HTTPS://GitLab.COM/path"}}); got != "gitlab.com" {
		t.Fatalf("firstProviderHost GitHosts = %q", got)
	}
	if got := firstProviderHost(providerpkg.Definition{WebURL: "https://github.example.test"}); got != "github.example.test" {
		t.Fatalf("firstProviderHost WebURL = %q", got)
	}
	if got := firstProviderHost(providerpkg.Definition{APIURL: "https://api.example.test"}); got != "api.example.test" {
		t.Fatalf("firstProviderHost APIURL = %q", got)
	}

	if def := providerOption(providerpkg.Definition{DisplayName: "Forge"}); def != "Forge" {
		t.Fatalf("providerOption no host = %q", def)
	}
	if def := providerOption(providerpkg.Definition{DisplayName: "Forge", WebURL: "https://forge.example.test"}); def != "Forge (forge.example.test)" {
		t.Fatalf("providerOption host = %q", def)
	}

	p := &profile.Profile{Providers: map[string]profile.ProviderAccountConfig{string(providerpkg.GitLabID): {Username: "lab"}}}
	if !profileUsesProvider(p, providerpkg.GitLabID) || profileUsesProvider(p, providerpkg.GitHubID) {
		t.Fatal("profileUsesProvider returned unexpected value")
	}
	if got := sshKeyProfileName("work", &profile.Profile{}); got != "work" {
		t.Fatalf("sshKeyProfileName no provider = %q", got)
	}
	if got := sshKeyProfileName("work", p); got != "work_gitlab" {
		t.Fatalf("sshKeyProfileName provider = %q", got)
	}
	if got := inferSSHKeyTypeFromPath("/tmp/id_ed25519_work_gitlab"); got != "ed25519" {
		t.Fatalf("inferSSHKeyTypeFromPath = %q", got)
	}
	for _, path := range []string{"", "custom", "id_"} {
		if got := inferSSHKeyTypeFromPath(path); got != "" {
			t.Fatalf("inferSSHKeyTypeFromPath(%q) = %q", path, got)
		}
	}

	if got := providerManualKeyURL(providerpkg.Definition{ID: providerpkg.GitHubID, WebURL: "https://github.example.test/"}, "ssh"); got != "https://github.example.test/settings/keys" {
		t.Fatalf("GitHub manual URL = %q", got)
	}
	if got := providerManualKeyURL(providerpkg.Definition{ID: providerpkg.GitLabID, WebURL: "https://gitlab.example.test"}, "ssh"); got != "https://gitlab.example.test/-/user_settings/ssh_keys" {
		t.Fatalf("GitLab SSH manual URL = %q", got)
	}
	if got := providerManualKeyURL(providerpkg.Definition{ID: providerpkg.GitLabID, WebURL: "https://gitlab.example.test"}, "gpg"); got != "https://gitlab.example.test/-/user_settings/gpg_keys" {
		t.Fatalf("GitLab GPG manual URL = %q", got)
	}
	if got := providerManualKeyURL(providerpkg.Definition{ID: providerpkg.ProviderID("forge"), WebURL: "https://forge.example.test/"}, "ssh"); got != "https://forge.example.test" {
		t.Fatalf("generic manual URL = %q", got)
	}

	ctr = nil
	if defs := providerDefinitionsWithCapability(providerpkg.CapabilityPATAuth); defs != nil {
		t.Fatalf("providerDefinitionsWithCapability(nil ctr) = %+v", defs)
	}
	if _, ok := profileProviderDefinition(p, providerpkg.CapabilityPATAuth); ok {
		t.Fatal("profileProviderDefinition should fail without container")
	}

	ctr = newRepairTestContainer(t)
	defs := providerDefinitionsWithCapability(providerpkg.CapabilityPATAuth)
	if len(defs) < 2 {
		t.Fatalf("providerDefinitionsWithCapability = %+v", defs)
	}
	if def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper); !ok || def.ID != providerpkg.GitLabID {
		t.Fatalf("profileProviderDefinition = %+v, %v", def, ok)
	}
	if _, ok := profileProviderDefinition(p, providerpkg.CapabilityOAuthDeviceAuth); ok {
		t.Fatal("GitLab profile should not resolve OAuth capability")
	}
}

func TestProviderTransitionAndSSHMigrationBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)

	p := repairTestProfile("work")
	profile.SetProviderAccount(p, providerpkg.GitHubID, "octo", providerpkg.AuthMethodPAT)
	oldState := cloneProfileProviderState(p)
	def, _ := ctr.ProviderRegistry.Get(providerpkg.GitLabID)

	setProfileProviderAccount(p, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	if !profileUsesProvider(p, providerpkg.GitLabID) {
		t.Fatal("setProfileProviderAccount did not switch provider")
	}
	restoreProfileProviderState(p, oldState)

	ok, err := applyProfileProviderTransitionWithOptions(context.Background(), "work", p, def, "lab", providerpkg.AuthMethodPAT, providerTransitionOptions{}, nil)
	if err == nil || ok || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("non-interactive transition = %v, %v", ok, err)
	}
	ok, err = applyProfileProviderTransitionWithOptions(context.Background(), "work", p, def, "lab", providerpkg.AuthMethodPAT, providerTransitionOptions{AutoConfirm: true}, func() error {
		return os.ErrPermission
	})
	if err == nil || ok {
		t.Fatalf("transition afterSet error = %v, %v", ok, err)
	}
	if !profileUsesProvider(p, providerpkg.GitHubID) {
		t.Fatal("provider state should be restored after transition error")
	}

	clearProfileProviderAccount(p, providerpkg.GitHubID)
	if profileUsesProvider(p, providerpkg.GitHubID) {
		t.Fatal("clearProfileProviderAccount did not remove GitHub")
	}
	clearAllProfileProviderAccounts(p)
	if profileProviderID, ok := profileProviderID(p); ok || profileProviderID != "" {
		t.Fatalf("clearAllProfileProviderAccounts left provider %q", profileProviderID)
	}

	if err := ctr.ProfileManager.Create(repairTestProfile("migrate")); err != nil {
		t.Fatalf("create migrate profile: %v", err)
	}
	dir := t.TempDir()
	legacyKey := filepath.Join(dir, "id_ed25519_migrate")
	if err := os.WriteFile(legacyKey, []byte("private"), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if err := os.WriteFile(legacyKey+".pub", []byte("public"), 0o600); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	mp := repairTestProfile("migrate")
	profile.SetProviderAccount(mp, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	mp.SSH = &profile.SSHConfig{KeyPath: legacyKey, KeyType: "ed25519"}
	if err := ctr.ProfileManager.Update(mp); err != nil {
		t.Fatalf("seed migrate profile: %v", err)
	}
	migrated, err := migrateProfileSSHKeyPathToProvider("migrate", mp)
	if err != nil || !migrated {
		t.Fatalf("migrateProfileSSHKeyPathToProvider = %v, %v", migrated, err)
	}
	if !strings.HasSuffix(mp.SSH.KeyPath, "id_ed25519_migrate_gitlab") {
		t.Fatalf("migrated key path = %q", mp.SSH.KeyPath)
	}
	if _, err := os.Stat(mp.SSH.KeyPath + ".pub"); err != nil {
		t.Fatalf("public key not migrated: %v", err)
	}
}

func TestSelectionRequirementAndTokenHelpers(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	p := repairTestProfile("work")
	profile.SetProviderAccount(p, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	def, _ := ctr.ProviderRegistry.Get(providerpkg.GitLabID)

	if selected, err := selectProfileProviderWithCapability("work", p, "", providerpkg.CapabilityPATAuth); err != nil || selected.ID != providerpkg.GitLabID {
		t.Fatalf("select default = %+v, %v", selected, err)
	}
	if _, err := selectProfileProviderWithCapability("work", p, "github", providerpkg.CapabilityPATAuth); err == nil || !strings.Contains(err.Error(), "not github") {
		t.Fatalf("select mismatch error = %v", err)
	}
	if err := requireProfileProvider("work", p, def); err != nil {
		t.Fatalf("requireProfileProvider: %v", err)
	}
	if err := requireProfileProvider("work", &profile.Profile{}, def); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("require missing provider error = %v", err)
	}
	mixed := repairTestProfile("mixed")
	mixed.Providers = map[string]profile.ProviderAccountConfig{string(providerpkg.GitHubID): {}, string(providerpkg.GitLabID): {}}
	if err := requireProfileProvider("mixed", mixed, def); err == nil || !strings.Contains(err.Error(), "multiple providers") {
		t.Fatalf("require multiple provider error = %v", err)
	}

	if err := saveProviderToken("work", def, p, providerpkg.TokenSet{AccessToken: "tok", AuthMethod: providerpkg.AuthMethodPAT}); err != nil {
		t.Fatalf("saveProviderToken: %v", err)
	}
	if loaded, err := loadProviderToken("work", def, p); err != nil || loaded.AccessToken != "tok" {
		t.Fatalf("loadProviderToken = %+v, %v", loaded, err)
	}
	if defs := authenticatedProvidersForProfile("work", p, providerpkg.CapabilityPATAuth); len(defs) != 1 || defs[0].ID != providerpkg.GitLabID {
		t.Fatalf("authenticatedProvidersForProfile = %+v", defs)
	}
	if err := deleteProviderToken("work", def, p); err != nil {
		t.Fatalf("deleteProviderToken: %v", err)
	}
}

func TestCredentialHelperPureHelpers(t *testing.T) {
	setTestStdin(t, "protocol=https\nhost=github.com\nignored\n\n")
	parsed := parseCredentialInput()
	if parsed["protocol"] != "https" || parsed["host"] != "github.com" || len(parsed) != 2 {
		t.Fatalf("parseCredentialInput = %#v", parsed)
	}

	missing := filepath.Join(t.TempDir(), "missing-binary")
	if got, err := resolveExecutablePath(missing); err != nil || got != missing {
		t.Fatalf("resolveExecutablePath missing = %q, %v", got, err)
	}

	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.ProviderID("docs"), DisplayName: "Docs", WebURL: "https://docs.example.test", GitHosts: []string{"docs.example.test"}})
	ctr.ProviderRegistry.Register(providerpkg.Definition{
		ID:           providerpkg.ProviderID("forge"),
		DisplayName:  "Forge",
		GitHosts:     []string{"forge.example.test", "https://alt-forge.example.test/"},
		Capabilities: providerpkg.CapabilitySet{providerpkg.CapabilityCredentialHelper: true},
	})
	servers := credentialHelperServers()
	seen := make(map[string]bool, len(servers))
	for _, server := range servers {
		seen[server] = true
	}
	for _, want := range []string{"https://github.com", "https://gitlab.com", "https://forge.example.test", "https://alt-forge.example.test"} {
		if !seen[want] {
			t.Fatalf("credentialHelperServers = %#v, missing %s", servers, want)
		}
	}

	if got := credentialHelperCommand("/tmp/Git Config Manager/gcm"); got != "!'/tmp/Git Config Manager/gcm' credential-helper" {
		t.Fatalf("credentialHelperCommand spaces = %q", got)
	}
	if got := credentialHelperCommand("/tmp/o'clock/gcm"); got != "!'/tmp/o'\\''clock/gcm' credential-helper" {
		t.Fatalf("credentialHelperCommand quote = %q", got)
	}
	if !credentialHelperConfigContainsGCM("!'/tmp/Git Config Manager/gcm' credential-helper\n") {
		t.Fatal("quoted helper command should be recognized as GCM")
	}
}

func TestCheckCommandUsesStderrVersionOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}

	out := captureStdout(t, func() {
		checkCommand("SSH", "sh", "-c", "echo OpenSSH_9.9p1 >&2")
	})
	if strings.Contains(out, "not installed") || !strings.Contains(out, "OpenSSH_9.9p1") {
		t.Fatalf("checkCommand output = %q", out)
	}
}

func TestCredentialHelperGetOutputsActiveProviderCredentials(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	p := repairTestProfile("work")
	profile.SetProviderAccount(p, providerpkg.GitHubID, "octo", providerpkg.AuthMethodPAT)
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	ctr.Config.DefaultProfile = "work"
	def, _ := ctr.ProviderRegistry.Get(providerpkg.GitHubID)
	if err := saveProviderToken("work", def, p, providerpkg.TokenSet{AccessToken: "gh-token", AuthMethod: providerpkg.AuthMethodPAT}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	t.Setenv("GIT_DIR", "/tmp/should-be-cleared")
	setTestStdin(t, "protocol=https\nhost=github.com\n\n")

	output := captureStdout(t, func() {
		if err := credentialHelperGet(nil, nil); err != nil {
			t.Fatalf("credentialHelperGet: %v", err)
		}
	})
	for _, want := range []string{"protocol=https", "host=github.com", "username=octo", "password=gh-token"} {
		if !strings.Contains(output, want) {
			t.Fatalf("credential output %q missing %q", output, want)
		}
	}
	if got := os.Getenv("GIT_DIR"); got != "" {
		t.Fatalf("GIT_DIR should be cleared, got %q", got)
	}
}

func TestConnectRunPaths(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	p := repairTestProfile("work")
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") == "bad" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"username":"lab","name":"Lab User"}`))
	}))
	t.Cleanup(server.Close)
	log := logger.New(logger.LevelError, io.Discard)
	ctr.GitLabClient = gitlab.NewClient(config.ProviderConfig{APIURL: server.URL}, log)
	ctr.ProviderClient = providerclient.NewRouter(ctr.GitHubClient, ctr.GitLabClient)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab", Type: "gitlab", APIURL: server.URL, WebURL: server.URL, GitHosts: []string{"gitlab.test"}, UploadKeys: true, Capabilities: providerpkg.GitLabCapabilities()})

	if _, err := resolveConnectProvider("work", p, "missing"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("resolve missing provider error = %v", err)
	}
	if got := providerPATURL(providerpkg.Definition{ID: providerpkg.GitLabID, WebURL: "https://gitlab.example.test/"}); got != "https://gitlab.example.test/-/user_settings/personal_access_tokens" {
		t.Fatalf("providerPATURL GitLab = %q", got)
	}
	if got := providerPATURL(providerpkg.Definition{ID: providerpkg.ProviderID("forge"), WebURL: "https://forge.example.test/"}); got != "https://forge.example.test" {
		t.Fatalf("providerPATURL generic = %q", got)
	}

	setTestStdin(t, "gl-token\n")
	if err := runConnect(context.Background(), "work", connectOptions{provider: "gitlab", tokenStdin: true, yes: true}); err != nil {
		t.Fatalf("runConnect success: %v", err)
	}
	updated, err := ctr.ProfileManager.Get("work")
	if err != nil {
		t.Fatalf("get updated profile: %v", err)
	}
	if !profileUsesProvider(updated, providerpkg.GitLabID) {
		t.Fatalf("profile provider not updated: %+v", updated.Providers)
	}
	if token, err := loadProviderToken("work", func() providerpkg.Definition { d, _ := ctr.ProviderRegistry.Get(providerpkg.GitLabID); return d }(), updated); err != nil || token.AccessToken != "gl-token" {
		t.Fatalf("stored token = %+v, %v", token, err)
	}

	setTestStdin(t, "bad\n")
	if err := runConnect(context.Background(), "work", connectOptions{provider: "gitlab", tokenStdin: true, yes: true}); err == nil {
		t.Fatal("expected invalid token error")
	}
	if err := runConnect(context.Background(), "missing", connectOptions{provider: "gitlab"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestStatusVerifierEdgeBranches(t *testing.T) {
	if err := quickVerifyProviderToken(providerpkg.Definition{ID: providerpkg.BitbucketID}, providerpkg.TokenSet{}); err != nil {
		t.Fatalf("unknown provider verifier = %v", err)
	}
	if err := quickVerifyToken("tok", "http://bad host"); err == nil {
		t.Fatal("expected GitHub request error")
	}
	if err := quickVerifyGitLabToken(providerpkg.TokenSet{AccessToken: "tok"}, "http://bad host"); err == nil {
		t.Fatal("expected GitLab request error")
	}
}

func TestProviderSpecificCommandRunPaths(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	ctr.Config.DefaultProfile = "already-set"
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("GitHub path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token gh-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"login":"octo"}`))
	}))
	t.Cleanup(githubServer.Close)
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("GitLab path = %s", r.URL.Path)
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "gl-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"username":"lab"}`))
	}))
	t.Cleanup(gitlabServer.Close)

	log := logger.New(logger.LevelError, io.Discard)
	ctr.Config.GitHub.APIURL = githubServer.URL
	ctr.GitHubClient = github.NewClient(ctr.Config, log, ctr.TokenStore)
	ctr.GitLabClient = gitlab.NewClient(config.ProviderConfig{APIURL: gitlabServer.URL}, log)
	ctr.ProviderClient = providerclient.NewRouter(ctr.GitHubClient, ctr.GitLabClient)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", Type: "github", APIURL: githubServer.URL, WebURL: githubServer.URL, GitHosts: []string{"github.test"}, Capabilities: providerpkg.GitHubCapabilities()})
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab", Type: "gitlab", APIURL: gitlabServer.URL, WebURL: gitlabServer.URL, GitHosts: []string{"gitlab.test"}, Capabilities: providerpkg.GitLabCapabilities()})

	for _, p := range []*profile.Profile{repairTestProfile("ghwork"), repairTestProfile("glwork")} {
		if err := ctr.ProfileManager.Create(p); err != nil {
			t.Fatalf("create profile %s: %v", p.Name, err)
		}
	}

	setTestStdin(t, "gh-token\n")
	captureStdout(t, func() {
		if err := runRootCommand(t, "github", "login", "ghwork"); err != nil {
			t.Fatalf("github login: %v", err)
		}
	})
	for _, args := range [][]string{{"github", "verify", "ghwork"}, {"github", "user", "ghwork"}, {"github", "status"}, {"github", "logout", "ghwork", "--force", "--clear-credentials=false"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}

	setTestStdin(t, "gl-token\n")
	captureStdout(t, func() {
		if err := runRootCommand(t, "gitlab", "login", "glwork"); err != nil {
			t.Fatalf("gitlab login: %v", err)
		}
	})
	for _, args := range [][]string{{"gitlab", "verify", "glwork"}, {"gitlab", "user", "glwork"}, {"gitlab", "status"}, {"gitlab", "logout", "glwork", "--force", "--clear-credentials=false"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}

	if server := gitServer(); server == "" {
		t.Fatal("gitServer returned empty server")
	}
	if _, err := githubProviderDefinition(); err != nil {
		t.Fatalf("githubProviderDefinition: %v", err)
	}
	if _, err := gitLabProviderDefinition(); err != nil {
		t.Fatalf("gitLabProviderDefinition: %v", err)
	}
}

func TestSSHAndGPGCommandRunPaths(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/user/keys":
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected provider request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	log := logger.New(logger.LevelError, io.Discard)
	ctr.GitLabClient = gitlab.NewClient(config.ProviderConfig{APIURL: server.URL}, log)
	ctr.ProviderClient = providerclient.NewRouter(ctr.GitHubClient, ctr.GitLabClient)
	ctr.ProviderRegistry.Register(providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab", Type: "gitlab", APIURL: server.URL, WebURL: server.URL, GitHosts: []string{"gitlab.test"}, SSHHost: "gitlab.test", UploadKeys: false, Capabilities: providerpkg.GitLabCapabilities()})

	sshProfile := repairTestProfile("sshwork")
	profile.SetProviderAccount(sshProfile, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	if err := ctr.ProfileManager.Create(sshProfile); err != nil {
		t.Fatalf("create ssh profile: %v", err)
	}
	def, _ := ctr.ProviderRegistry.Get(providerpkg.GitLabID)
	if err := saveProviderToken("sshwork", def, sshProfile, providerpkg.TokenSet{AccessToken: "gl-token", AuthMethod: providerpkg.AuthMethodPAT}); err != nil {
		t.Fatalf("save token: %v", err)
	}

	fakeSSH := filepath.Join(t.TempDir(), "fake-ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\necho 'Welcome to GitLab, @test!'\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	ctr.Config.Advanced.SSHCommand = fakeSSH

	sshCommands := [][]string{
		{"ssh", "generate", "sshwork", "--type", "ed25519", "--comment", "ssh-test"},
		{"ssh", "list"},
		{"ssh", "copy", "sshwork"},
		{"ssh", "test", "sshwork", "--provider", "gitlab"},
		{"ssh", "upload", "sshwork", "--provider", "gitlab", "--force"},
	}
	for _, args := range sshCommands {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}

	gpgProfile := repairTestProfile("gpgwork")
	if err := ctr.ProfileManager.Create(gpgProfile); err != nil {
		t.Fatalf("create gpg profile: %v", err)
	}
	for _, args := range [][]string{{"gpg"}, {"gpg", "list"}, {"gpg", "sign", "disable", "gpgwork"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}
	for _, args := range [][]string{{"gpg", "sign", "enable", "gpgwork"}, {"gpg", "test", "gpgwork"}, {"gpg", "upload", "gpgwork"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err == nil {
				t.Fatalf("expected %v to fail", args)
			}
		})
	}
}

func TestTemplateAndProfileHelperBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)

	fileMode := true
	ignoreCase := false
	gpgSign := true
	verbose := true
	followTags := true
	autoSetup := true
	p := repairTestProfile("templated")
	p.Git.Core.Editor = "vim"
	p.Git.Core.AutoCRLF = "input"
	p.Git.Core.EOL = "lf"
	p.Git.Core.FileMode = &fileMode
	p.Git.Core.IgnoreCase = &ignoreCase
	p.Git.Commit.GPGSign = &gpgSign
	p.Git.Commit.Template = "commit.txt"
	p.Git.Commit.Verbose = &verbose
	p.Git.Pull.Rebase = "true"
	p.Git.Pull.FF = "only"
	p.Git.Push.Default = "current"
	p.Git.Push.FollowTags = &followTags
	p.Git.Push.AutoSetupRemote = &autoSetup
	p.Git.Aliases = map[string]string{"co": "checkout"}
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	captureStdout(t, func() {
		if err := templateCreateFromProfile("from-profile", "templated", ""); err != nil {
			t.Fatalf("templateCreateFromProfile: %v", err)
		}
	})
	extracted, err := ctr.TemplateManager.Get("from-profile")
	if err != nil {
		t.Fatalf("get extracted template: %v", err)
	}
	if extracted.Description == "" || extracted.Git.Core["editor"] != "vim" || extracted.Git.Aliases["co"] != "checkout" {
		t.Fatalf("extracted template = %+v", extracted)
	}

	applyTemplate := &templatepkg.Template{
		Name:        "apply-all",
		Description: "All settings",
		Git: templatepkg.GitConfigTemplate{
			Core:    map[string]interface{}{"editor": "nano", "autocrlf": "false", "eol": "crlf", "filemode": false, "ignorecase": true},
			Commit:  map[string]interface{}{"gpgsign": false, "template": "new.txt", "verbose": false},
			Pull:    map[string]interface{}{"rebase": "merges", "ff": "false"},
			Push:    map[string]interface{}{"default": "simple", "followtags": false, "autosetupremote": false},
			Aliases: map[string]string{"st": "status"},
		},
	}
	captureStdout(t, func() {
		if changes := templateShowChanges(applyTemplate, p); changes != 14 {
			t.Fatalf("templateShowChanges = %d", changes)
		}
		templatePrintSummary(applyTemplate)
	})
	applyTemplateToProfile(applyTemplate, p)
	if p.Git.Core.Editor != "nano" || p.Git.Pull.Rebase != "merges" || p.Git.Aliases["st"] != "status" {
		t.Fatalf("profile after apply = %+v", p.Git)
	}
	if p.Git.Core.FileMode == nil || *p.Git.Core.FileMode || p.Git.Commit.GPGSign == nil || *p.Git.Commit.GPGSign {
		t.Fatalf("bool settings after apply = core=%v commit=%v", p.Git.Core.FileMode, p.Git.Commit.GPGSign)
	}

	providerProfile := repairTestProfile("provider")
	profile.SetProviderAccount(providerProfile, providerpkg.GitLabID, "lab", providerpkg.AuthMethodPAT)
	captureStdout(t, func() {
		diffField("Email", "a@example.test", "b@example.test", "a", "b")
		diffField("Same", "x", "x", "a", "b")
		printProfileProviderAccounts(providerProfile)
		printProfileProviderAccountSummary(providerProfile)
		printProfileProviderAccounts(&profile.Profile{Name: "none"})
		printProfileProviderAccountSummary(&profile.Profile{Name: "none"})
		mixed := &profile.Profile{Name: "mixed", Providers: map[string]profile.ProviderAccountConfig{string(providerpkg.GitHubID): {}, string(providerpkg.GitLabID): {}}}
		printProfileProviderAccounts(mixed)
		printProfileProviderAccountSummary(mixed)
	})
}

func TestInteractiveProfileAndTemplateFlows(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)

	setUIPromptInput(t, "Interactive User\ninteractive@example.test\nvim\nn\nn\n1\n")
	captureStdout(t, func() {
		if err := profileCreateInteractive("interactive", ""); err != nil {
			t.Fatalf("profileCreateInteractive: %v", err)
		}
	})
	p, err := ctr.ProfileManager.Get("interactive")
	if err != nil {
		t.Fatalf("get interactive profile: %v", err)
	}
	if p.Git.User.Name != "Interactive User" || p.Git.Core.Editor != "vim" {
		t.Fatalf("interactive profile = %+v", p)
	}

	setUIPromptInput(t, "Edited User\nedited@example.test\nnano\nSIGN123\nn\n1\n")
	captureStdout(t, func() {
		if err := profileEditInteractive(p); err != nil {
			t.Fatalf("profileEditInteractive: %v", err)
		}
	})
	p, _ = ctr.ProfileManager.Get("interactive")
	if p.Git.User.Name != "Edited User" || p.Git.User.SigningKey != "SIGN123" {
		t.Fatalf("edited profile = %+v", p)
	}

	providerProfile := repairTestProfile("prompt-provider")
	setUIPromptInput(t, "2\nocto\n")
	if err := promptProviderAccountUsernames(providerProfile); err != nil {
		t.Fatalf("promptProviderAccountUsernames provider: %v", err)
	}
	if !profileUsesProvider(providerProfile, providerpkg.GitHubID) {
		t.Fatalf("provider prompt did not set GitHub: %+v", providerProfile.Providers)
	}
	setUIPromptInput(t, "1\ny\n")
	if err := promptProviderAccountUsernames(providerProfile); err != nil {
		t.Fatalf("promptProviderAccountUsernames skip: %v", err)
	}
	if profileProvider, ok := profileProviderID(providerProfile); ok || profileProvider != "" {
		t.Fatalf("provider prompt did not clear provider: %q", profileProvider)
	}

	setUIPromptInput(t, "Template description\ncode --wait\n3\ny\ny\nbadalias\nco=checkout\n\n")
	captureStdout(t, func() {
		if err := templateCreateInteractive("interactive-template"); err != nil {
			t.Fatalf("templateCreateInteractive: %v", err)
		}
	})
	tmpl, err := ctr.TemplateManager.Get("interactive-template")
	if err != nil {
		t.Fatalf("get interactive template: %v", err)
	}
	if tmpl.Git.Core["editor"] != "code --wait" || tmpl.Git.Pull["ff"] != "only" || tmpl.Git.Aliases["co"] != "checkout" {
		t.Fatalf("interactive template = %+v", tmpl)
	}
}

func TestSetupSkipFlowAndProviderAuthenticationBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	ctr.Config.DefaultProfile = "already-set"
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	setUIPromptInput(t, "setupwork\nSetup User\nsetup@example.test\nn\nn\nn\n")
	captureStdout(t, func() {
		if err := runSetup(context.Background()); err != nil {
			t.Fatalf("runSetup skip flow: %v", err)
		}
	})
	if _, err := ctr.ProfileManager.Get("setupwork"); err != nil {
		t.Fatalf("setup profile not created: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"username":"setup-lab"}`))
	}))
	t.Cleanup(server.Close)
	log := logger.New(logger.LevelError, io.Discard)
	ctr.GitLabClient = gitlab.NewClient(config.ProviderConfig{APIURL: server.URL}, log)
	ctr.ProviderClient = providerclient.NewRouter(ctr.GitHubClient, ctr.GitLabClient)
	gitlabDef := providerpkg.Definition{ID: providerpkg.GitLabID, DisplayName: "GitLab", Type: "gitlab", APIURL: server.URL, WebURL: server.URL, GitHosts: []string{"gitlab.test"}, Capabilities: providerpkg.GitLabCapabilities()}
	ctr.ProviderRegistry.Register(gitlabDef)

	setUIPromptInput(t, "\n")
	captureStdout(t, func() {
		if err := runSetupGitLabAuthentication(context.Background(), "setupwork", gitlabDef); err != nil {
			t.Fatalf("empty GitLab setup auth: %v", err)
		}
	})
	setUIPromptInput(t, "gl-token\n")
	captureStdout(t, func() {
		if err := runSetupGitLabAuthentication(context.Background(), "setupwork", gitlabDef); err != nil {
			t.Fatalf("GitLab setup auth: %v", err)
		}
	})
	updated, _ := ctr.ProfileManager.Get("setupwork")
	if !profileUsesProvider(updated, providerpkg.GitLabID) {
		t.Fatalf("setup GitLab auth did not set provider: %+v", updated.Providers)
	}

	setUIPromptInput(t, "y\n1\n")
	captureStdout(t, func() {
		if err := runSetupProviderAuthentication(context.Background(), "setupwork"); err != nil {
			t.Fatalf("setup provider skip: %v", err)
		}
	})
}

func TestInitAndCredentialRegistrationRunPaths(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	for _, args := range [][]string{{"init"}, {"init"}, {"init", "--force"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}
	_ = IsCredentialHelperConfigured()
	if err := UnregisterCredentialHelper(); err != nil {
		t.Fatalf("UnregisterCredentialHelper: %v", err)
	}
	_ = IsCredentialHelperConfigured()
}

func TestInitDoesNotClearGlobalIdentityWithoutExplicitFlag(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	setGlobalGitConfig(t, "user.name", "Existing User")
	setGlobalGitConfig(t, "user.email", "existing@example.test")

	captureStdout(t, func() {
		if err := runRootCommand(t, "init"); err != nil {
			t.Fatalf("init: %v", err)
		}
	})

	if got, ok := globalGitConfig(t, "user.name"); !ok || got != "Existing User" {
		t.Fatalf("user.name = %q, %v", got, ok)
	}
	if got, ok := globalGitConfig(t, "user.email"); !ok || got != "existing@example.test" {
		t.Fatalf("user.email = %q, %v", got, ok)
	}
}

func TestInitClearsGlobalIdentityWithExplicitFlag(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	setGlobalGitConfig(t, "user.name", "Existing User")
	setGlobalGitConfig(t, "user.email", "existing@example.test")

	captureStdout(t, func() {
		if err := runRootCommand(t, "init", "--clear-global-identity"); err != nil {
			t.Fatalf("init --clear-global-identity: %v", err)
		}
	})

	if got, ok := globalGitConfig(t, "user.name"); ok {
		t.Fatalf("user.name still set to %q", got)
	}
	if got, ok := globalGitConfig(t, "user.email"); ok {
		t.Fatalf("user.email still set to %q", got)
	}
}

func TestSetupDoesNotClearGlobalIdentityBeforeFirstPrompt(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	setGlobalGitConfig(t, "user.name", "Existing User")
	setGlobalGitConfig(t, "user.email", "existing@example.test")
	setUIPromptInput(t, "")

	captureStdout(t, func() {
		if err := runSetup(context.Background()); err == nil {
			t.Fatal("expected setup prompt error")
		}
	})

	if got, ok := globalGitConfig(t, "user.name"); !ok || got != "Existing User" {
		t.Fatalf("user.name = %q, %v", got, ok)
	}
	if got, ok := globalGitConfig(t, "user.email"); !ok || got != "existing@example.test" {
		t.Fatalf("user.email = %q, %v", got, ok)
	}
}

func TestGitHubOAuthGHAndSetupAuthBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	ctr.Config.DefaultProfile = "already-set"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"login":"setup-octo"}`))
	}))
	t.Cleanup(server.Close)
	log := logger.New(logger.LevelError, io.Discard)
	ctr.Config.GitHub.APIURL = server.URL
	ctr.GitHubClient = github.NewClient(ctr.Config, log, ctr.TokenStore)
	ctr.ProviderClient = providerclient.NewRouter(ctr.GitHubClient, ctr.GitLabClient)
	githubDef := providerpkg.Definition{ID: providerpkg.GitHubID, DisplayName: "GitHub", Type: "github", APIURL: server.URL, WebURL: server.URL, GitHosts: []string{"github.test"}, Capabilities: providerpkg.GitHubCapabilities()}
	ctr.ProviderRegistry.Register(githubDef)

	profileConfig := repairTestProfile("oauthwork")
	if err := ctr.ProfileManager.Create(profileConfig); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	captureStdout(t, func() {
		if err := runRootCommand(t, "github", "login-oauth", "missing"); err == nil {
			t.Fatal("expected missing profile error")
		}
	})
	captureStdout(t, func() {
		if err := runRootCommand(t, "github", "login-oauth", "oauthwork"); err == nil {
			t.Fatal("expected OAuth setup error")
		}
	})
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)
	captureStdout(t, func() {
		if err := runRootCommand(t, "github", "login-gh", "oauthwork"); err == nil {
			t.Fatal("expected gh import error")
		}
	})
	ghPath := filepath.Join(pathDir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nif [ \"$1 $2\" = \"auth token\" ]; then echo gh-token; exit 0; fi\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	captureStdout(t, func() {
		if err := runRootCommand(t, "github", "login-gh", "oauthwork"); err != nil {
			t.Fatalf("github login-gh success: %v", err)
		}
	})

	setUIPromptInput(t, "1\n\n")
	captureStdout(t, func() {
		if err := runSetupGitHubAuthentication(context.Background(), "oauthwork", githubDef); err != nil {
			t.Fatalf("empty GitHub setup auth: %v", err)
		}
	})
	setUIPromptInput(t, "2\n")
	captureStdout(t, func() {
		if err := runSetupGitHubAuthentication(context.Background(), "oauthwork", githubDef); err == nil {
			t.Fatal("expected OAuth GitHub setup auth error")
		}
	})
	setUIPromptInput(t, "1\ngh-token\n")
	captureStdout(t, func() {
		if err := runSetupGitHubAuthentication(context.Background(), "oauthwork", githubDef); err != nil {
			t.Fatalf("GitHub setup auth: %v", err)
		}
	})
	updated, _ := ctr.ProfileManager.Get("oauthwork")
	if !profileUsesProvider(updated, providerpkg.GitHubID) {
		t.Fatalf("setup GitHub auth did not set provider: %+v", updated.Providers)
	}
}

func TestAdditionalCommandBranches(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	source := repairTestProfile("source")
	source.Git.Core.Editor = "vim"
	if err := ctr.ProfileManager.Create(source); err != nil {
		t.Fatalf("create source profile: %v", err)
	}
	captureStdout(t, func() {
		validateAndPrint(source)
		validateAndPrint(&profile.Profile{Name: "invalid"})
	})

	captureStdout(t, func() {
		if err := runRootCommand(t, "backup", "create"); err != nil {
			t.Fatalf("backup create: %v", err)
		}
	})
	backups, err := ctr.BackupManager.List()
	if err != nil || len(backups) == 0 {
		t.Fatalf("list backups = %+v, %v", backups, err)
	}
	setUIPromptInput(t, "y\n")
	captureStdout(t, func() {
		if err := runRootCommand(t, "backup", "restore", backups[0].Path); err != nil {
			t.Fatalf("backup restore: %v", err)
		}
	})

	exported, err := ctr.ProfileManager.Export("source")
	if err != nil {
		t.Fatalf("export profile: %v", err)
	}
	importPath := filepath.Join(t.TempDir(), "imported.yaml")
	importedData := strings.Replace(string(exported), "name: source", "name: imported", 1)
	if err := os.WriteFile(importPath, []byte(importedData), 0o600); err != nil {
		t.Fatalf("write profile import: %v", err)
	}
	for _, args := range [][]string{{"profile", "import", filepath.Join(t.TempDir(), "missing.yaml")}, {"profile", "import", importPath}} {
		captureStdout(t, func() {
			err := runRootCommand(t, args...)
			if strings.Contains(args[2], "missing") {
				if err == nil {
					t.Fatalf("expected %v to fail", args)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}

	for _, args := range [][]string{{"use", "source", "--dry-run"}, {"current"}} {
		captureStdout(t, func() {
			if err := runRootCommand(t, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}
	ctr.Config.DefaultProfile = "source"
	captureStdout(t, func() {
		if err := runRootCommand(t, "current"); err != nil {
			t.Fatalf("current default: %v", err)
		}
	})

	if err := runRootCommand(t, "template", "create", "exported", "--description", "Exported", "--editor", "vim"); err != nil {
		t.Fatalf("template create exported: %v", err)
	}
	templateData, err := ctr.TemplateManager.Export("exported")
	if err != nil {
		t.Fatalf("template export: %v", err)
	}
	templatePath := filepath.Join(t.TempDir(), "template.yaml")
	if err := os.WriteFile(templatePath, []byte(strings.Replace(string(templateData), "name: exported", "name: imported-template", 1)), 0o600); err != nil {
		t.Fatalf("write template import: %v", err)
	}
	for _, args := range [][]string{{"template", "import", filepath.Join(t.TempDir(), "missing.yaml")}, {"template", "import", templatePath}} {
		captureStdout(t, func() {
			err := runRootCommand(t, args...)
			if args[1] == "import" && strings.Contains(args[2], "missing") {
				if err == nil {
					t.Fatalf("expected %v to fail", args)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
	}
}

func TestActivateIfOnlyProfile(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	t.Run("first profile auto-activates", func(t *testing.T) {
		original := ctr
		t.Cleanup(func() { ctr = original })
		ctr = newRepairTestContainer(t)

		p := repairTestProfile("solo")
		if err := ctr.ProfileManager.Create(p); err != nil {
			t.Fatalf("create: %v", err)
		}
		captureStdout(t, func() {
			if !activateIfOnlyProfile("solo") {
				t.Fatal("expected activateIfOnlyProfile to return true")
			}
		})
	})

	t.Run("second profile does not auto-activate", func(t *testing.T) {
		original := ctr
		t.Cleanup(func() { ctr = original })
		ctr = newRepairTestContainer(t)

		if err := ctr.ProfileManager.Create(repairTestProfile("first")); err != nil {
			t.Fatalf("create first: %v", err)
		}
		if err := ctr.ProfileManager.Create(repairTestProfile("second")); err != nil {
			t.Fatalf("create second: %v", err)
		}
		if activateIfOnlyProfile("second") {
			t.Fatal("should not activate when multiple profiles exist")
		}
	})

	t.Run("does not activate if default already set", func(t *testing.T) {
		original := ctr
		t.Cleanup(func() { ctr = original })
		ctr = newRepairTestContainer(t)
		ctr.Config.DefaultProfile = "existing"

		if err := ctr.ProfileManager.Create(repairTestProfile("only")); err != nil {
			t.Fatalf("create: %v", err)
		}
		if activateIfOnlyProfile("only") {
			t.Fatal("should not activate when DefaultProfile is already set")
		}
	})
}

func TestNonInteractiveCommandRunPaths(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	// Isolate working directory into a temp git repo so that commands like
	// "refresh --silent" cannot read the real .git/gcm-session marker and
	// accidentally overwrite the real repo's .git/config with test data.
	isoDir := t.TempDir()
	cmd := exec.Command("git", "init", isoDir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init isolation dir: %v", err)
	}
	origDir, _ := os.Getwd()
	if err := os.Chdir(isoDir); err != nil {
		t.Fatalf("chdir isolation: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	missingBackup := filepath.Join(t.TempDir(), "missing-backup.tar.gz")
	commands := [][]string{
		{"version"},
		{"version", "--short"},
		{"clean"},
		{"clean", "--all"},
		{"backup", "list"},
		{"backup", "create"},
		{"backup", "list"},
		{"backup", "prune", "--keep", "5"},
		{"backup", "restore", missingBackup},
		{"validate"},
		{"validate", "missing"},
		{"profile"},
		{"profile", "create", "work", "--name", "Jane Doe", "--email", "jane@example.test", "--editor", "vim"},
		{"profile", "show", "work"},
		{"profile", "edit", "work", "--name", "Janet Doe", "--email", "janet@example.test", "--editor", "nano", "--signing-key", "ABC123"},
		{"profile", "create", "other", "--name", "Other User", "--email", "other@example.test"},
		{"profile", "list"},
		{"profile", "diff", "work", "other"},
		{"profile", "export", "work"},
		{"profile", "show", "missing"},
		{"profile", "delete", "other", "--yes"},
		{"template"},
		{"template", "create", "team", "--description", "Team defaults", "--editor", "vim", "--rebase", "true", "--gpg-sign", "yes", "--alias", "co=checkout", "--alias", "st=status"},
		{"template", "list"},
		{"template", "show", "team"},
		{"template", "export", "team"},
		{"template", "apply", "team", "work", "--force"},
		{"template", "delete", "team", "--yes"},
		{"current"},
		{"current", "--short"},
		{"refresh", "--silent"},
		{"ssh"},
		{"ssh", "copy", "work"},
		{"ssh", "test", "work"},
		{"ssh", "upload", "work"},
		{"status"},
		{"doctor"},
	}
	wantErr := map[string]bool{
		strings.Join([]string{"backup", "restore", missingBackup}, " "): true,
		"validate missing":     true,
		"profile show missing": true,
		"ssh copy work":        true,
		"ssh test work":        true,
		"ssh upload work":      true,
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			captureStdout(t, func() {
				err := runRootCommand(t, args...)
				if wantErr[strings.Join(args, " ")] {
					if err == nil {
						t.Fatalf("expected %v to fail", args)
					}
					return
				}
				if err != nil {
					t.Fatalf("%v: %v", args, err)
				}
			})
		})
	}
}
