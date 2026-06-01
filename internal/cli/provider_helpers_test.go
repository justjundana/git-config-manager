package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	sshpkg "github.com/justjundana/git-config-manager/internal/ssh"
)

func TestCloneRestoreProfileProviderStatePreservesProviderAdjacentState(t *testing.T) {
	p := &profile.Profile{
		Providers: map[string]profile.ProviderAccountConfig{string(providerpkg.GitHubID): {Username: "octo"}},
		GitHub:    &profile.GitHubConfig{Username: "octo"},
		SSH:       &profile.SSHConfig{KeyPath: "/tmp/id_ed25519_work_github", KeyType: "ed25519"},
		GPG:       &profile.GPGConfig{KeyID: "ABC123"},
	}
	snapshot := cloneProfileProviderState(p)

	p.Providers = map[string]profile.ProviderAccountConfig{string(providerpkg.GitLabID): {Username: "lab"}}
	p.GitHub = nil
	p.SSH.KeyPath = "/tmp/id_ed25519_work_gitlab"
	p.GPG.KeyID = "DEF456"

	restoreProfileProviderState(p, snapshot)

	if got := p.Providers[string(providerpkg.GitHubID)].Username; got != "octo" {
		t.Fatalf("provider username = %q, want octo", got)
	}
	if p.GitHub == nil || p.GitHub.Username != "octo" {
		t.Fatalf("legacy GitHub config not restored: %+v", p.GitHub)
	}
	if p.SSH == nil || p.SSH.KeyPath != "/tmp/id_ed25519_work_github" {
		t.Fatalf("SSH config not restored: %+v", p.SSH)
	}
	if p.GPG == nil || p.GPG.KeyID != "ABC123" {
		t.Fatalf("GPG config not restored: %+v", p.GPG)
	}

	p.SSH.KeyPath = "mutated-again"
	if snapshot.SSH.KeyPath != "/tmp/id_ed25519_work_github" {
		t.Fatal("snapshot SSH config should be a deep copy")
	}
}

func TestProviderSSHKeyMigrationTarget(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		profile  string
		keyPath  string
		provider providerpkg.ProviderID
		wantBase string
		wantOK   bool
	}{
		{
			name:     "legacy filename gains provider suffix",
			profile:  "work",
			keyPath:  filepath.Join(dir, "id_ed25519_work"),
			provider: providerpkg.GitLabID,
			wantBase: "id_ed25519_work_gitlab",
			wantOK:   true,
		},
		{
			name:     "old provider suffix migrates to selected provider",
			profile:  "work",
			keyPath:  filepath.Join(dir, "id_ed25519_work_github"),
			provider: providerpkg.GitLabID,
			wantBase: "id_ed25519_work_gitlab",
			wantOK:   true,
		},
		{
			name:     "already provider scoped",
			profile:  "work",
			keyPath:  filepath.Join(dir, "id_ed25519_work_gitlab"),
			provider: providerpkg.GitLabID,
			wantOK:   false,
		},
		{
			name:     "custom filename is left alone",
			profile:  "work",
			keyPath:  filepath.Join(dir, "custom_work_key"),
			provider: providerpkg.GitLabID,
			wantOK:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &profile.Profile{
				Providers: map[string]profile.ProviderAccountConfig{string(tc.provider): {Username: "user"}},
				SSH:       &profile.SSHConfig{KeyPath: tc.keyPath, KeyType: "ed25519"},
			}
			got, ok := providerSSHKeyMigrationTarget(tc.profile, p)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && filepath.Base(got) != tc.wantBase {
				t.Fatalf("target base = %q, want %q", filepath.Base(got), tc.wantBase)
			}
		})
	}
}

func TestNormalizeProviderSelectionAliases(t *testing.T) {
	cases := map[string]providerpkg.ProviderID{
		"gh":        providerpkg.GitHubID,
		" GitHub ":  providerpkg.GitHubID,
		"gl":        providerpkg.GitLabID,
		"GITLAB":    providerpkg.GitLabID,
		"bb":        providerpkg.BitbucketID,
		"Bitbucket": providerpkg.BitbucketID,
		"forgejo":   providerpkg.ProviderID("forgejo"),
	}
	for input, want := range cases {
		if got := normalizeProviderSelection(input); got != want {
			t.Fatalf("normalizeProviderSelection(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProviderResourceNameSanitizesComponents(t *testing.T) {
	def := providerpkg.Definition{ID: providerpkg.GitLabID}
	got := providerResourceName("Work/Profile", def, "SSH Key", "ED25519")
	if got != "gcm-work-profile-gitlab-ssh-key-ed25519" {
		t.Fatalf("providerResourceName = %q", got)
	}
}

func TestProviderSSHKeyAlreadyInUse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "github key in use", err: fmt.Errorf(`API error 422: {"message":"Validation Failed","errors":[{"message":"key is already in use"}]}`), want: true},
		{name: "gitlab fingerprint taken", err: fmt.Errorf(`GitLab API error 400: {"message":{"fingerprint":["has already been taken"]}}`), want: true},
		{name: "other API error", err: fmt.Errorf(`API error 500: unavailable`), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := providerSSHKeyAlreadyInUse(tc.err); got != tc.want {
				t.Fatalf("providerSSHKeyAlreadyInUse(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestAdoptExistingSSHKeyForProfile(t *testing.T) {
	original := ctr
	t.Cleanup(func() { ctr = original })
	ctr = newRepairTestContainer(t)

	p := repairTestProfile("personal")
	profile.SetProviderAccount(p, providerpkg.GitHubID, "octo", providerpkg.AuthMethodPAT)
	if err := ctr.ProfileManager.Create(p); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	keyProfileName := sshKeyProfileName("personal", p)
	generated, err := ctr.SSHManager.Generate(sshpkg.GenerateOptions{Profile: keyProfileName, KeyType: "ed25519", Comment: "gcm-personal-github"})
	if err != nil {
		t.Fatalf("generate stale key: %v", err)
	}
	p, err = ctr.ProfileManager.Get("personal")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if p.SSH != nil {
		t.Fatalf("test setup should leave SSH unconfigured: %+v", p.SSH)
	}

	adoptedInfo, adopted, err := adoptExistingSSHKeyForProfile("personal", p, []string{"ed25519"})
	if err != nil {
		t.Fatalf("adoptExistingSSHKeyForProfile: %v", err)
	}
	if !adopted || adoptedInfo.Path != generated.Path {
		t.Fatalf("adopted = %v, info = %+v, generated = %+v", adopted, adoptedInfo, generated)
	}

	stored, err := ctr.ProfileManager.Get("personal")
	if err != nil {
		t.Fatalf("get stored profile: %v", err)
	}
	if stored.SSH == nil || stored.SSH.KeyPath != generated.Path || stored.SSH.Fingerprint == "" {
		t.Fatalf("stored SSH config = %+v", stored.SSH)
	}
	if _, err := os.Stat(generated.Path); err != nil {
		t.Fatalf("adoption should not remove key: %v", err)
	}
}
