// Package gpg provides GPG key management for GCM.
package gpg

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// defaultCommandTimeout bounds GPG invocations (listing, signing, exporting).
// Key generation uses a longer timeout since it may block on entropy.
const (
	defaultCommandTimeout = 30 * time.Second
	generateKeyTimeout    = 5 * time.Minute
)

// Manager handles GPG key operations.
type Manager struct {
	cfg *config.Config
	log *logger.Logger
}

// NewManager creates a new GPG manager.
func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{cfg: cfg, log: log}
}

// resolveGPGCommand finds the GPG binary. Checks the configured command first,
// then tries common alternatives (gpg2, gpg, gpg.exe) only if the configured
// command is a default value. Returns empty string if GPG is not available.
func (m *Manager) resolveGPGCommand() string {
	cmd := m.cfg.Advanced.GPGCommand
	if cmd != "" {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
		// If user explicitly set a non-default command and it's not found, don't
		// fall back — they want that specific binary.
		if cmd != "gpg" && cmd != "gpg2" {
			return ""
		}
	}
	// Try common alternatives
	for _, candidate := range []string{"gpg2", "gpg", "gpg.exe"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// GenerateOptions holds parameters for GPG key generation.
type GenerateOptions struct {
	Name       string
	Email      string
	Comment    string
	KeyType    string // RSA, DSA, default RSA
	KeyLength  int    // default 4096
	Expiration string // 0, 1y, 2y, etc.
}

// KeyInfo holds information about a GPG key.
type KeyInfo struct {
	KeyID       string
	Fingerprint string
	Name        string
	Email       string
	Created     time.Time
	Expires     *time.Time
	Trust       string
}

// Generate creates a new GPG key pair.
//
// User-supplied fields (Name, Email, Comment, Expiration) are validated so
// they cannot inject additional GPG batch directives via embedded newlines or
// control characters. See https://www.gnupg.org/documentation/manuals/gnupg/Unattended-GPG-key-generation.html
func (m *Manager) Generate(opts GenerateOptions) (*KeyInfo, error) {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return nil, fmt.Errorf("GPG is not installed\n\n  Install GPG:\n    macOS:   brew install gnupg\n    Ubuntu:  sudo apt install gnupg\n    Windows: https://www.gnupg.org/download/")
	}

	if opts.KeyType == "" {
		opts.KeyType = "RSA"
	}
	if opts.KeyLength == 0 {
		opts.KeyLength = 4096
	}
	if opts.Expiration == "" {
		opts.Expiration = "2y"
	}

	if err := validateGenerateOptions(&opts); err != nil {
		return nil, err
	}

	// Build batch input. All interpolated values have been validated above to
	// reject newlines, carriage returns, and percent-escaped batch markers.
	batchInput := fmt.Sprintf(`%%no-protection
Key-Type: %s
Key-Length: %d
Subkey-Type: %s
Subkey-Length: %d
Name-Real: %s
Name-Email: %s
Expire-Date: %s
%%commit
`, opts.KeyType, opts.KeyLength, opts.KeyType, opts.KeyLength,
		opts.Name, opts.Email, opts.Expiration)

	if opts.Comment != "" {
		batchInput = strings.Replace(batchInput, "Name-Email:",
			fmt.Sprintf("Name-Comment: %s\nName-Email:", opts.Comment), 1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), generateKeyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpgCmd, "--batch", "--gen-key")
	cmd.Stdin = strings.NewReader(batchInput)

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("GPG key generation failed: %s: %w",
			strings.TrimSpace(string(out)), err)
	}

	// Find the key we just generated
	keys, err := m.listKeys(opts.Email)
	if err != nil {
		return nil, fmt.Errorf("finding generated key: %w", err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("key was generated but could not be found")
	}

	key := keys[len(keys)-1] // Latest key
	m.log.Debug("GPG key generated",
		logger.F("keyID", key.KeyID),
		logger.F("email", opts.Email))

	return &key, nil
}

// validateKeyTypeRegex restricts GPG key types to a safe algorithm name.
var (
	validKeyTypeRegex    = regexp.MustCompile(`^[A-Za-z0-9]{2,10}$`)
	validExpirationRegex = regexp.MustCompile(`^(0|\d{1,4}[dwmy]|\d{4}-\d{2}-\d{2})$`)
	// Names, emails, and comments must not contain control characters or
	// characters that could terminate a batch directive.
	unsafeBatchChars = regexp.MustCompile(`[\x00-\x1f\x7f%]`)
)

// validateGenerateOptions rejects inputs that could inject GPG batch commands.
func validateGenerateOptions(opts *GenerateOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("name is required")
	}
	if opts.Email == "" {
		return fmt.Errorf("email is required")
	}

	if !validKeyTypeRegex.MatchString(opts.KeyType) {
		return fmt.Errorf("invalid key type %q (expected RSA, DSA, ECDSA, EDDSA)", opts.KeyType)
	}
	if opts.KeyLength < 1024 || opts.KeyLength > 16384 {
		return fmt.Errorf("invalid key length %d (expected 1024-16384)", opts.KeyLength)
	}
	if !validExpirationRegex.MatchString(opts.Expiration) {
		return fmt.Errorf("invalid expiration %q (expected 0, 1y, 2y, 90d, or YYYY-MM-DD)", opts.Expiration)
	}

	for field, val := range map[string]string{
		"name":    opts.Name,
		"email":   opts.Email,
		"comment": opts.Comment,
	} {
		if unsafeBatchChars.MatchString(val) {
			return fmt.Errorf("%s contains disallowed characters (control chars or %%)", field)
		}
	}
	if !strings.Contains(opts.Email, "@") {
		return fmt.Errorf("email %q does not contain @", opts.Email)
	}

	return nil
}

// List returns all GPG keys.
func (m *Manager) List() ([]KeyInfo, error) {
	return m.listKeys("")
}

// GetKey returns a specific GPG key by ID.
func (m *Manager) GetKey(keyID string) (*KeyInfo, error) {
	keys, err := m.listKeys("")
	if err != nil {
		return nil, err
	}

	for _, k := range keys {
		if k.KeyID == keyID || strings.HasSuffix(k.Fingerprint, keyID) {
			return &k, nil
		}
	}

	return nil, fmt.Errorf("GPG key %s not found", keyID)
}

// GetPublicKey exports the public key in ASCII armor format.
func (m *Manager) GetPublicKey(keyID string) (string, error) {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return "", fmt.Errorf("GPG is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpgCmd, "--armor", "--export", keyID)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exporting GPG key: %w", err)
	}
	return string(out), nil
}

// TestSigning tests GPG signing capability.
func (m *Manager) TestSigning(keyID string) error {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return fmt.Errorf("GPG is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpgCmd, "--local-user", keyID, "--sign", "--armor")
	cmd.Stdin = strings.NewReader("test signing")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("GPG signing test failed: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Delete removes a GPG key (secret + public) from the local keyring.
func (m *Manager) Delete(keyID string) error {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return fmt.Errorf("GPG is not installed")
	}

	// Get the full fingerprint (GPG 2.1+ requires it for deletion)
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, gpgCmd, "--with-colons", "--fingerprint", keyID).Output()
	if err != nil {
		return fmt.Errorf("GPG key %s not found in keyring", keyID)
	}

	var fingerprint string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "fpr:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 10 && parts[9] != "" {
				fingerprint = parts[9]
				break
			}
		}
	}
	if fingerprint == "" {
		return fmt.Errorf("could not find fingerprint for GPG key %s", keyID)
	}

	// Delete secret key first
	ctx2, cancel2 := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel2()
	cmd := exec.CommandContext(ctx2, gpgCmd, "--batch", "--yes", "--delete-secret-keys", fingerprint)
	if out, err := cmd.CombinedOutput(); err != nil {
		m.log.Debug("delete secret key failed", logger.F("output", string(out)))
		// Not fatal — key might not have a secret component
	}

	// Delete public key
	ctx3, cancel3 := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel3()
	cmd = exec.CommandContext(ctx3, gpgCmd, "--batch", "--yes", "--delete-keys", fingerprint)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete GPG key %s: %s: %w", keyID, strings.TrimSpace(string(out)), err)
	}

	return nil
}

// IsInstalled checks if GPG is available on the system.
func (m *Manager) IsInstalled() bool {
	return m.resolveGPGCommand() != ""
}

// GetVersion returns the GPG version string.
func (m *Manager) GetVersion() (string, error) {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return "", fmt.Errorf("GPG is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpgCmd, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	return strings.TrimSpace(lines[0]), nil
}

func (m *Manager) listKeys(filterEmail string) ([]KeyInfo, error) {
	gpgCmd := m.resolveGPGCommand()
	if gpgCmd == "" {
		return nil, fmt.Errorf("GPG is not installed")
	}

	args := []string{"--list-keys", "--with-colons", "--fixed-list-mode"}
	if filterEmail != "" {
		args = append(args, filterEmail)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpgCmd, args...)
	out, err := cmd.Output()
	if err != nil {
		// No keys found is not an error
		if strings.Contains(string(out), "not found") || strings.Contains(err.Error(), "exit status 2") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing GPG keys: %w", err)
	}

	return parseGPGOutput(string(out)), nil
}

// parseGPGTimestamp parses a GPG colon-format date field which may be either
// a Unix timestamp (modern GnuPG) or an ISO-style "yyyymmddThhmmss" / "yyyymmdd"
// date string (older GnuPG).
func parseGPGTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	// Older GPG: ISO-style compact date-time or date-only.
	if len(s) == len("20060102T150405") && strings.Contains(s, "T") {
		if t, err := time.Parse("20060102T150405", s); err == nil {
			return t, true
		}
	}
	if len(s) == len("20060102") {
		if t, err := time.Parse("20060102", s); err == nil {
			return t, true
		}
	}

	// Modern GPG: seconds since epoch.
	// Use a minimum length guard so short numeric junk doesn't parse as a valid timestamp.
	if len(s) >= 9 {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil && ts > 0 {
			return time.Unix(ts, 0).UTC(), true
		}
	}
	return time.Time{}, false
}

func parseGPGOutput(output string) []KeyInfo {
	var keys []KeyInfo
	var current *KeyInfo

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 10 {
			continue
		}

		switch fields[0] {
		case "pub":
			key := KeyInfo{
				KeyID: fields[4],
				Trust: fields[1],
			}
			if t, ok := parseGPGTimestamp(fields[5]); ok {
				key.Created = t
			}
			if t, ok := parseGPGTimestamp(fields[6]); ok {
				key.Expires = &t
			}
			keys = append(keys, key)
			current = &keys[len(keys)-1]

		case "fpr":
			if current != nil {
				current.Fingerprint = fields[9]
			}

		case "uid":
			if current != nil {
				uid := fields[9]
				// Parse "Name (comment) <email>"
				if idx := strings.Index(uid, " <"); idx >= 0 {
					current.Name = uid[:idx]
					current.Email = strings.Trim(uid[idx+2:], ">")
				} else {
					current.Name = uid
				}
			}
		}
	}

	return keys
}
