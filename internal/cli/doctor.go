package cli

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/justjundana/git-config-manager/internal/profile"
	"github.com/justjundana/git-config-manager/internal/tokenstore"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
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
				ui.Error("profile %q not found", args[0])
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[0])
			}
			validateAndPrint(p)
			return nil
		},
	}
}

func validateAndPrint(p *profile.Profile) {
	result := profile.ValidateDeep(p)

	icon := ui.Green(ui.IconSuccess)
	if !result.IsValid() {
		icon = ui.Red(ui.IconError)
	}

	ui.Print("\n%s Profile: %s", icon, ui.Bold(p.Name))

	for _, issue := range result.Info {
		ui.Print("  %s %s: %s", ui.Green(ui.IconSuccess), issue.Category, issue.Message)
	}
	for _, issue := range result.Warnings {
		ui.Print("  %s %s: %s", ui.Yellow(ui.IconWarning), issue.Category, issue.Message)
		if issue.Suggestion != "" {
			ui.Print("      %s", ui.Dim(issue.Suggestion))
		}
	}
	for _, issue := range result.Errors {
		ui.Print("  %s %s: %s", ui.Red(ui.IconError), issue.Category, issue.Message)
		if issue.Suggestion != "" {
			ui.Print("      %s", ui.Dim(issue.Suggestion))
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
			ui.Header("%s GCM System Health Check", ui.IconDoctor)
			ui.Blank()

			// System info
			ui.SubHeader("System")
			ui.Print("  %s OS: %s/%s", ui.Green(ui.IconSuccess), runtime.GOOS, runtime.GOARCH)
			ui.Print("  %s Go: %s", ui.Green(ui.IconSuccess), runtime.Version())

			// Git
			ui.SubHeader("Dependencies")
			checkCommand("Git", "git", "--version")
			checkCommand("SSH", "ssh", "-V")
			checkCommand("GPG", ctr.Config.Advanced.GPGCommand, "--version")

			// SSH agent
			ui.SubHeader("Services")
			checkSSHAgent()

			// Token storage
			ui.SubHeader("Token Storage")
			if tokenstore.IsKeychainAvailable() {
				ui.Print("  %s OS Keychain: available", ui.Green(ui.IconSuccess))
			} else {
				ui.Print("  %s OS Keychain: unavailable on %s/%s", ui.Yellow(ui.IconWarning), runtime.GOOS, runtime.GOARCH)
				if ctr.Config.Security.EncryptTokens && ctr.Config.Security.MasterPassword {
					ui.Print("    Using encrypted file storage (master password)")
				} else if ctr.Config.Security.AllowPlaintextTokens {
					ui.Print("    Using plaintext file storage")
				} else {
					ui.Print("    No token storage backend configured")
					ui.Print("    Fix: set security.encrypt_tokens + security.master_password in config")
				}
			}

			// Config
			ui.SubHeader("Configuration")
			configPath := "~/.gcm/config.yaml"
			ui.Print("  %s Config: %s", ui.Green(ui.IconSuccess), configPath)

			profiles, err := ctr.ProfileManager.List()
			if err != nil {
				ui.Print("  %s Profiles: error reading", ui.Red(ui.IconError))
			} else {
				ui.Print("  %s Profiles: %d found", ui.Green(ui.IconSuccess), len(profiles))
			}

			currentName, currentScope, _ := ctr.ProfileSwitcher.Current()
			if currentName != "" {
				ui.Print("  %s Active: %s (%s)", ui.Green(ui.IconSuccess), currentName, currentScope.String())
			} else {
				ui.Print("  %s No active profile", ui.Yellow(ui.IconWarning))
			}

			// Shell
			ui.SubHeader("Shell Integration")
			shellType := ctr.ShellManager.DetectShell()
			ui.Print("  %s Detected: %s", ui.Green(ui.IconSuccess), string(shellType))
			if shellType != "unknown" {
				if installed, configFile := ctr.ShellManager.IsInstalled(shellType); installed {
					ui.Print("  %s Hooks installed in %s", ui.Green(ui.IconSuccess), configFile)
					ui.Print("    Auto-switching and prompt integration are active")
				} else {
					ui.Print("  %s Shell hooks not installed", ui.Yellow(ui.IconWarning))
					ui.Print("    Auto-switching is disabled. Fix: run %s", ui.Cyan("gcm init"))
				}
			}

			// Credential Helper
			ui.SubHeader("Credential Helper")
			missingHelpers := missingCredentialHelperServers()
			if len(missingHelpers) == 0 {
				ui.Print("  %s GCM registered as git credential helper for configured provider hosts", ui.Green(ui.IconSuccess))
				ui.Print("    Credentials are served from GCM's encrypted store (immune to external logout)")
			} else {
				ui.Print("  %s GCM is missing credential helper registration for %d provider host(s)", ui.Yellow(ui.IconWarning), len(missingHelpers))
				for _, server := range missingHelpers {
					ui.Print("    %s", server)
				}
				ui.Print("    Git credentials use the system keychain (can be affected by VS Code logout, etc.)")
				ui.Print("    Fix: run %s to register GCM as the credential helper", ui.Cyan("gcm repair --fix"))
			}

			ui.Blank()
			ui.Success("Health check complete")
			if fix {
				ui.Blank()
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
		ui.Print("  %s %s: not installed", ui.Red(ui.IconError), label)
		return
	}
	ver := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	ui.Print("  %s %s: %s", ui.Green(ui.IconSuccess), label, ver)
}

func checkSSHAgent() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		if strings.Contains(output, "Could not open") || strings.Contains(output, "not running") {
			ui.Print("  %s SSH Agent: not running", ui.Red(ui.IconError))
			return
		}
	}

	if strings.Contains(output, "no identities") {
		ui.Print("  %s SSH Agent: running (no keys loaded)", ui.Yellow(ui.IconWarning))
	} else {
		lines := strings.Split(output, "\n")
		ui.Print("  %s SSH Agent: running (%d keys)", ui.Green(ui.IconSuccess), len(lines))
	}
}
