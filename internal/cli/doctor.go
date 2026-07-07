package cli

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_tokenstore "github.com/justjundana/git-config-manager/internal/tokenstore"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [profile]",
		Short: "Validate a profile configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				// Validate all profiles
				profiles, err := ctr.ProfileManager.List()
				if err != nil {
					return err
				}
				for _, p := range profiles {
					validateAndPrint(p)
				}
				return nil
			}

			p, err := ctr.ProfileManager.Get(args[0])
			if err != nil {
				_ui.Error("profile %q not found", args[0])
				_ui.Blank()
				_ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[0])
			}
			validateAndPrint(p)
			return nil
		},
	}
}

func validateAndPrint(p *_profile.Profile) {
	result := _profile.ValidateDeep(p)

	icon := _ui.Green(_ui.IconSuccess)
	if !result.IsValid() {
		icon = _ui.Red(_ui.IconError)
	}

	_ui.Print("\n%s Profile: %s", icon, _ui.Bold(p.Name))

	for _, issue := range result.Info {
		_ui.Print("  %s %s: %s", _ui.Green(_ui.IconSuccess), issue.Category, issue.Message)
	}
	for _, issue := range result.Warnings {
		_ui.Print("  %s %s: %s", _ui.Yellow(_ui.IconWarning), issue.Category, issue.Message)
		if issue.Suggestion != "" {
			_ui.Print("      %s", _ui.Dim(issue.Suggestion))
		}
	}
	for _, issue := range result.Errors {
		_ui.Print("  %s %s: %s", _ui.Red(_ui.IconError), issue.Category, issue.Message)
		if issue.Suggestion != "" {
			_ui.Print("      %s", _ui.Dim(issue.Suggestion))
		}
	}
}

func newDoctorCmd() *cobra.Command {
	var fix bool
	var yes bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system health and dependencies",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ui.Header("%s GCM System Health Check", _ui.IconDoctor)
			_ui.Blank()

			// System info
			_ui.SubHeader("System")
			_ui.Print("  %s OS: %s/%s", _ui.Green(_ui.IconSuccess), runtime.GOOS, runtime.GOARCH)
			_ui.Print("  %s Go: %s", _ui.Green(_ui.IconSuccess), runtime.Version())

			// Git
			_ui.SubHeader("Dependencies")
			checkCommand("Git", "git", "--version")
			checkCommand("SSH", "ssh", "-V")
			checkCommand("GPG", ctr.Config.Advanced.GPGCommand, "--version")

			// SSH agent
			_ui.SubHeader("Services")
			checkSSHAgent()

			// Token storage
			_ui.SubHeader("Token Storage")
			if _tokenstore.IsKeychainAvailable() {
				_ui.Print("  %s OS Keychain: available", _ui.Green(_ui.IconSuccess))
			} else {
				_ui.Print("  %s OS Keychain: unavailable on %s/%s", _ui.Yellow(_ui.IconWarning), runtime.GOOS, runtime.GOARCH)
				if ctr.Config.Security.EncryptTokens && ctr.Config.Security.MasterPassword {
					_ui.Print("    Using encrypted file storage (master password)")
				} else if ctr.Config.Security.AllowPlaintextTokens {
					_ui.Print("    Using plaintext file storage")
				} else {
					_ui.Print("    No token storage backend configured")
					_ui.Print("    Fix: set security.encrypt_tokens + security.master_password in config")
				}
			}

			// Config
			_ui.SubHeader("Configuration")
			configPath := "~/.gcm/config.yaml"
			_ui.Print("  %s Config: %s", _ui.Green(_ui.IconSuccess), configPath)

			profiles, err := ctr.ProfileManager.List()
			if err != nil {
				_ui.Print("  %s Profiles: error reading", _ui.Red(_ui.IconError))
			} else {
				_ui.Print("  %s Profiles: %d found", _ui.Green(_ui.IconSuccess), len(profiles))
			}

			currentName, currentScope, _ := ctr.ProfileSwitcher.Current()
			if currentName != "" {
				_ui.Print("  %s Active: %s (%s)", _ui.Green(_ui.IconSuccess), currentName, currentScope.String())
			} else {
				_ui.Print("  %s No active profile", _ui.Yellow(_ui.IconWarning))
			}

			// Shell
			_ui.SubHeader("Shell Integration")
			shellType := ctr.ShellManager.DetectShell()
			_ui.Print("  %s Detected: %s", _ui.Green(_ui.IconSuccess), string(shellType))
			if shellType != "unknown" {
				if installed, configFile := ctr.ShellManager.IsInstalled(shellType); installed {
					_ui.Print("  %s Hooks installed in %s", _ui.Green(_ui.IconSuccess), configFile)
					_ui.Print("    Auto-switching and prompt integration are active")
				} else {
					_ui.Print("  %s Shell hooks not installed", _ui.Yellow(_ui.IconWarning))
					_ui.Print("    Auto-switching is disabled. Fix: run %s", _ui.Cyan("gcm init"))
				}
			}

			// Credential Helper
			_ui.SubHeader("Credential Helper")
			missingHelpers := missingCredentialHelperServers()
			if len(missingHelpers) == 0 {
				_ui.Print("  %s GCM registered as git credential helper for configured provider hosts", _ui.Green(_ui.IconSuccess))
				_ui.Print("    Credentials are served from GCM's encrypted store (immune to external logout)")
			} else {
				_ui.Print("  %s GCM is missing credential helper registration for %d provider host(s)", _ui.Yellow(_ui.IconWarning), len(missingHelpers))
				for _, server := range missingHelpers {
					_ui.Print("    %s", server)
				}
				_ui.Print("    Git credentials use the system keychain (can be affected by VS Code logout, etc.)")
				_ui.Print("    Fix: run %s to register GCM as the credential helper", _ui.Cyan("gcm repair --fix"))
			}

			_ui.Blank()
			_ui.Success("Health check complete")
			if fix {
				_ui.Blank()
				return runRepair(repairOptions{fix: true, yes: yes})
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Run safe repair actions after health checks")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip repair confirmation when used with --fix")
	return cmd
}

func missingCredentialHelperServers() []string {
	servers := credentialHelperServers()
	missing := make([]string, 0, len(servers))
	for _, server := range servers {
		if !IsCredentialHelperConfiguredFor(server) {
			missing = append(missing, server)
		}
	}
	return missing
}

func checkCommand(label, cmd string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fullCmd := exec.CommandContext(ctx, cmd, args...)
	out, err := fullCmd.CombinedOutput()
	if err != nil {
		_ui.Print("  %s %s: not installed", _ui.Red(_ui.IconError), label)
		return
	}
	ver := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	_ui.Print("  %s %s: %s", _ui.Green(_ui.IconSuccess), label, ver)
}

func checkSSHAgent() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		if strings.Contains(output, "Could not open") || strings.Contains(output, "not running") {
			_ui.Print("  %s SSH Agent: not running", _ui.Red(_ui.IconError))
			return
		}
	}

	if strings.Contains(output, "no identities") {
		_ui.Print("  %s SSH Agent: running (no keys loaded)", _ui.Yellow(_ui.IconWarning))
	} else {
		lines := strings.Split(output, "\n")
		_ui.Print("  %s SSH Agent: running (%d keys)", _ui.Green(_ui.IconSuccess), len(lines))
	}
}
