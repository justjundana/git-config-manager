package profile

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile *Profile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: &Profile{
				Name: "work",
				Git: GitConfig{
					User: GitUser{Name: "Test", Email: "test@example.com"},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty name",
			profile: &Profile{Name: ""},
			wantErr: true,
		},
		{
			name: "invalid name characters",
			profile: &Profile{
				Name: "has spaces",
				Git:  GitConfig{User: GitUser{Name: "T", Email: "t@e.com"}},
			},
			wantErr: true,
		},
		{
			name: "name starts with hyphen",
			profile: &Profile{
				Name: "-bad",
				Git:  GitConfig{User: GitUser{Name: "T", Email: "t@e.com"}},
			},
			wantErr: true,
		},
		{
			name: "empty user name",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Email: "t@e.com"}},
			},
			wantErr: true,
		},
		{
			name: "empty email",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Name: "Test"}},
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Name: "Test", Email: "not-an-email"}},
			},
			wantErr: true,
		},
		{
			name: "valid with SSH",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Name: "Test", Email: "t@e.com"}},
				SSH:  &SSHConfig{KeyPath: "~/.ssh/id_ed25519"},
			},
			wantErr: false,
		},
		{
			name: "SSH with empty path",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Name: "Test", Email: "t@e.com"}},
				SSH:  &SSHConfig{KeyPath: ""},
			},
			wantErr: true,
		},
		{
			name: "GPG with empty key ID",
			profile: &Profile{
				Name: "work",
				Git:  GitConfig{User: GitUser{Name: "Test", Email: "t@e.com"}},
				GPG:  &GPGConfig{KeyID: ""},
			},
			wantErr: true,
		},
		{
			name: "valid with underscores and numbers",
			profile: &Profile{
				Name: "my_work_2024",
				Git:  GitConfig{User: GitUser{Name: "Test", Email: "t@e.com"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDeep(t *testing.T) {
	p := &Profile{
		Name: "work",
		Git: GitConfig{
			User: GitUser{Name: "John", Email: "john@company.com"},
		},
	}

	result := ValidateDeep(p)
	if !result.IsValid() {
		t.Error("valid profile should pass deep validation")
	}
	if len(result.Info) == 0 {
		t.Error("should have info items for set fields")
	}
}

func TestValidateDeepMissingFields(t *testing.T) {
	p := &Profile{
		Name: "bad",
		Git:  GitConfig{User: GitUser{}},
	}

	result := ValidateDeep(p)
	if result.IsValid() {
		t.Error("profile with empty fields should fail deep validation")
	}
	if len(result.Errors) == 0 {
		t.Error("should have error items for missing fields")
	}
}

func TestIsValidProfileName(t *testing.T) {
	valid := []string{"work", "personal", "my-work", "client_a", "Work2024"}
	invalid := []string{"", " ", "-start", "_start", "has space", "has/slash", "a@b"}

	for _, name := range valid {
		if !isValidProfileName(name) {
			t.Errorf("isValidProfileName(%q) = false, want true", name)
		}
	}
	for _, name := range invalid {
		if isValidProfileName(name) {
			t.Errorf("isValidProfileName(%q) = true, want false", name)
		}
	}
}

func TestIsValidEmail(t *testing.T) {
	valid := []string{"a@b.com", "user@company.co.uk", "name+tag@example.com"}
	invalid := []string{"", "noat", "@", "a@", "@b", "a @b.com"}

	for _, email := range valid {
		if !isValidEmail(email) {
			t.Errorf("isValidEmail(%q) = false, want true", email)
		}
	}
	for _, email := range invalid {
		if isValidEmail(email) {
			t.Errorf("isValidEmail(%q) = true, want false", email)
		}
	}
}

func TestBoolPtr(t *testing.T) {
	trueP := BoolPtr(true)
	falseP := BoolPtr(false)

	if *trueP != true {
		t.Error("BoolPtr(true) should dereference to true")
	}
	if *falseP != false {
		t.Error("BoolPtr(false) should dereference to false")
	}
}

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}

	got := expandPath("~/foo/bar")
	want := home + "/foo/bar"
	if got != want {
		t.Errorf("expandPath(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandPath_TildeAlone(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}

	got := expandPath("~")
	if got != home {
		t.Errorf("expandPath(~) = %q, want %q", got, home)
	}
}

func TestExpandPath_NoTilde(t *testing.T) {
	got := expandPath("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandPath(/absolute/path) = %q", got)
	}
}

func TestExpandPath_RelativePath(t *testing.T) {
	got := expandPath("relative/path")
	if got != "relative/path" {
		t.Errorf("expandPath(relative/path) = %q", got)
	}
}

func TestValidateDeep_WithSSH(t *testing.T) {
	p := &Profile{
		Name: "ssh-user",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "test@example.com"},
		},
		SSH: &SSHConfig{
			KeyPath: "~/.ssh/nonexistent_test_key_gcm",
		},
	}

	result := ValidateDeep(p)
	// Should have an error about missing SSH key file
	hasSSHError := false
	for _, e := range result.Errors {
		if e.Category == "SSH" {
			hasSSHError = true
			break
		}
	}
	if !hasSSHError {
		t.Error("expected SSH error for nonexistent key")
	}
}

func TestValidateDeep_WithGPG(t *testing.T) {
	p := &Profile{
		Name: "gpg-user",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "test@example.com"},
		},
		GPG: &GPGConfig{
			KeyID: "ABCDEF1234567890",
		},
	}

	result := ValidateDeep(p)
	// Should have info about GPG key
	hasGPGInfo := false
	for _, info := range result.Info {
		if info.Category == "GPG" {
			hasGPGInfo = true
			break
		}
	}
	if !hasGPGInfo {
		t.Error("expected GPG info for configured key")
	}
}

func TestValidateDeep_MissingEmail(t *testing.T) {
	p := &Profile{
		Name: "noemail",
		Git: GitConfig{
			User: GitUser{Name: "Test"},
		},
	}

	result := ValidateDeep(p)
	if result.IsValid() {
		t.Error("should fail without email")
	}
}

func TestValidateDeep_InvalidEmail(t *testing.T) {
	p := &Profile{
		Name: "bademail",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "not-valid"},
		},
	}

	result := ValidateDeep(p)
	hasEmailError := false
	for _, e := range result.Errors {
		if e.Category == "Git" {
			hasEmailError = true
			break
		}
	}
	if !hasEmailError {
		t.Error("expected error for invalid email format")
	}
}

func TestValidateDeep_SSHKeyExistsWithBadPerms(t *testing.T) {
	dir := t.TempDir()
	keyPath := dir + "/test_key"
	os.WriteFile(keyPath, []byte("fake key"), 0o644) // Bad permissions

	p := &Profile{
		Name: "perms-test",
		Git:  GitConfig{User: GitUser{Name: "Test", Email: "test@example.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath},
	}

	result := ValidateDeep(p)
	hasPermWarning := false
	for _, w := range result.Warnings {
		if w.Category == "SSH" {
			hasPermWarning = true
			break
		}
	}
	if !hasPermWarning {
		t.Error("expected SSH warning for permissive key permissions")
	}
}

func TestValidateDeep_SSHKeyExistsCorrectPerms(t *testing.T) {
	dir := t.TempDir()
	keyPath := dir + "/test_key"
	os.WriteFile(keyPath, []byte("fake key"), 0o600)

	p := &Profile{
		Name: "goodperms",
		Git:  GitConfig{User: GitUser{Name: "Test", Email: "test@example.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath},
	}

	result := ValidateDeep(p)
	hasSSHInfo := false
	for _, info := range result.Info {
		if info.Category == "SSH" {
			hasSSHInfo = true
			break
		}
	}
	if !hasSSHInfo {
		t.Error("expected SSH info for existing key with correct permissions")
	}
}

func TestValidateDeep_GPGWithExpiration(t *testing.T) {
	expires := time.Date(2030, 12, 31, 0, 0, 0, 0, time.UTC)
	p := &Profile{
		Name: "gpg-expires",
		Git:  GitConfig{User: GitUser{Name: "Test", Email: "test@example.com"}},
		GPG:  &GPGConfig{KeyID: "ABCDEF", ExpiresAt: &expires},
	}

	result := ValidateDeep(p)
	hasExpiresInfo := false
	for _, info := range result.Info {
		if info.Category == "GPG" && info.Message != "" {
			hasExpiresInfo = true
		}
	}
	if !hasExpiresInfo {
		t.Error("expected GPG info with expiration date")
	}
}

func TestValidateProfile_GPGEmptyKeyID(t *testing.T) {
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		GPG:  &GPGConfig{KeyID: ""},
	}
	err := ValidateProfile(p)
	if err == nil {
		t.Fatal("expected error for empty GPG key ID")
	}
}

func TestValidateProfile_SSHEmptyKeyPath(t *testing.T) {
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		SSH:  &SSHConfig{KeyPath: ""},
	}
	err := ValidateProfile(p)
	if err == nil {
		t.Fatal("expected error for empty SSH key path")
	}
}

func TestValidateProfile_InvalidProfileName(t *testing.T) {
	p := &Profile{
		Name: "has spaces!",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
	}
	err := ValidateProfile(p)
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
}

func TestValidateDeep_NoEditorWarning(t *testing.T) {
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
	}
	result := ValidateDeep(p)
	// Should have warnings about missing editor, signing key, etc.
	if len(result.Warnings) == 0 && len(result.Info) == 0 {
		t.Error("expected at least some warnings or info for minimal profile")
	}
}

func TestValidateDeep_SSHKeyPathExists(t *testing.T) {
	tmp := t.TempDir()
	keyPath := tmp + "/test_key"
	os.WriteFile(keyPath, []byte("key"), 0o644) // wrong perms

	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		SSH:  &SSHConfig{KeyPath: keyPath},
	}
	result := ValidateDeep(p)
	// Should have warning about key permissions
	hasPermWarning := false
	for _, w := range result.Warnings {
		if w.Category == "SSH" {
			hasPermWarning = true
		}
	}
	if !hasPermWarning {
		t.Error("expected SSH permission warning")
	}
}

func TestValidateDeep_GPGKeyExpired(t *testing.T) {
	expired := time.Now().Add(-24 * time.Hour)
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		GPG:  &GPGConfig{KeyID: "ABC123", ExpiresAt: &expired},
	}
	result := ValidateDeep(p)
	hasExpiredError := false
	for _, e := range result.Errors {
		if e.Category == "GPG" && len(e.Message) > 0 {
			hasExpiredError = true
		}
	}
	if !hasExpiredError {
		t.Error("expected GPG expiration error for expired key")
	}
}

func TestValidateDeep_GPGKeyExpiresSoon(t *testing.T) {
	expiresSoon := time.Now().Add(7 * 24 * time.Hour) // 7 days from now
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		GPG:  &GPGConfig{KeyID: "ABC123", ExpiresAt: &expiresSoon},
	}
	result := ValidateDeep(p)
	hasSoonWarning := false
	for _, w := range result.Warnings {
		if w.Category == "GPG" && len(w.Message) > 0 {
			hasSoonWarning = true
		}
	}
	if !hasSoonWarning {
		t.Error("expected GPG 'expires soon' warning")
	}
}

func TestValidateDeep_GPGKeyFarFuture(t *testing.T) {
	farFuture := time.Now().Add(365 * 24 * time.Hour) // 1 year from now
	p := &Profile{
		Name: "test",
		Git:  GitConfig{User: GitUser{Name: "A", Email: "a@b.com"}},
		GPG:  &GPGConfig{KeyID: "ABC123", ExpiresAt: &farFuture},
	}
	result := ValidateDeep(p)
	// Should be info, not warning or error
	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors for far-future GPG key: %+v", result.Errors)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings for far-future GPG key: %+v", result.Warnings)
	}
}

func TestIsValidEmail_TooLong(t *testing.T) {
	// RFC 5321 limits total address to 254 octets
	longLocal := strings.Repeat("a", 250)
	longEmail := longLocal + "@b.com"
	if isValidEmail(longEmail) {
		t.Error("expected false for email > 254 chars")
	}
}

func TestValidateCustomKeys_Dangerous(t *testing.T) {
	dangerousKeys := []string{
		"core.sshCommand",
		"core.hooksPath",
		"core.gitProxy",
		"core.askPass",
		"core.pager",
		"credential.helper",
		"diff.external",
		"filter.clean",
		"filter.smudge",
		"remote.origin.proxy",
		"credential.https://github.com.helper",
		"filter.lfs.clean",
		"diff.my-tool.command",
		"merge.custom.driver",
	}

	for _, key := range dangerousKeys {
		p := &Profile{
			Name: "test",
			Git: GitConfig{
				User:   GitUser{Name: "Test", Email: "t@e.com"},
				Custom: map[string]string{key: "malicious-value"},
			},
		}
		if err := ValidateProfile(p); err == nil {
			t.Errorf("expected error for dangerous custom key %q, got nil", key)
		}
	}
}

func TestValidateCustomKeys_Safe(t *testing.T) {
	safeKeys := []string{
		"user.signingkey",
		"commit.gpgsign",
		"pull.rebase",
		"push.default",
		"init.defaultBranch",
		"color.ui",
		"core.editor",
	}

	for _, key := range safeKeys {
		p := &Profile{
			Name: "test",
			Git: GitConfig{
				User:   GitUser{Name: "Test", Email: "t@e.com"},
				Custom: map[string]string{key: "some-value"},
			},
		}
		if err := ValidateProfile(p); err != nil {
			t.Errorf("unexpected error for safe custom key %q: %v", key, err)
		}
	}
}

func TestValidateCustomKeys_NilMap(t *testing.T) {
	p := &Profile{
		Name: "test",
		Git: GitConfig{
			User: GitUser{Name: "Test", Email: "t@e.com"},
		},
	}
	if err := ValidateProfile(p); err != nil {
		t.Errorf("unexpected error for nil custom map: %v", err)
	}
}
