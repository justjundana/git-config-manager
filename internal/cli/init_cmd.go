package cli

import (
	"fmt"
	"strings"

	"github.com/justjundana/git-config-manager/internal/audit"
	"github.com/justjundana/git-config-manager/internal/shell"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force bool
	var clearGlobalIdentity bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up shell integration and credential helper",
		Long: `Install shell hooks for auto-switching and prompt integration,
and register GCM as the git credential helper for configured providers.

Use --force to reinstall even if already configured.
Use --clear-global-identity only when you explicitly want GCM to remove
global git user.name, user.email, and user.signingkey.

Examples:
  gcm init                          # Auto-detect and install
  gcm init --force                  # Force reinstall
	gcm init --clear-global-identity  # Explicitly clear global git identity
  SHELL=/bin/zsh gcm init           # Override shell detection`,
		RunE: func(_ *cobra.Command, _ []string) error {
			shellType := ctr.ShellManager.DetectShell()
			if shellType == shell.ShellUnknown {
				ui.Error("Could not detect your shell")
				ui.Info("Set SHELL environment variable and retry: SHELL=/bin/zsh gcm init")
				return fmt.Errorf("could not detect shell")
			}

			ui.Header("%s Setting up GCM for %s", ui.IconRocket, string(shellType))
			ui.Blank()

			installed, configFile := ctr.ShellManager.IsInstalled(shellType)

			if installed && !force {
				ui.Success("Shell integration already installed!")
				ui.Detail("Shell", string(shellType))
				ui.Detail("Config", configFile)
				ui.Blank()
				ui.Print("  To force reinstall: gcm init --force")
			} else {
				// Force reinstall: uninstall first if already present
				if installed && force {
					if _, err := ctr.ShellManager.Uninstall(shellType); err != nil {
						ui.Warning("Could not uninstall existing hooks: %v", err)
					}
				}

				newConfigFile, err := ctr.ShellManager.Install(shellType)
				if err != nil {
					return err
				}

				if force && installed {
					ui.Success("Shell integration reinstalled!")
				} else {
					ui.Success("Shell integration installed!")
				}
				ctr.AuditLogger.Log(audit.ActionShellInit, "",
					map[string]string{"shell": string(shellType), "config": newConfigFile}, nil)
				ui.Detail("Shell", string(shellType))
				ui.Detail("Config", newConfigFile)

				ui.Blank()
				ui.Info("Restart your shell or run: source %s", newConfigFile)
			}

			// Register GCM as credential helper for configured providers.
			ui.Blank()
			if err := RegisterCredentialHelper(); err != nil {
				ui.Warning("Could not register credential helper: %v", err)
				ui.Print("  Git will fall back to the system keychain for credentials.")
			} else {
				ui.Success("Git credential helper registered!")
				ui.Detail("Scope", strings.Join(credentialHelperServers(), ", "))
			}

			if clearGlobalIdentity {
				if err := ctr.ProfileSwitcher.ClearGlobalIdentity(); err != nil {
					return fmt.Errorf("clear global git identity: %w", err)
				}
				ui.Blank()
				ui.Info("Global git identity cleared by explicit request — activate a profile to set your identity:")
				ui.Print("  gcm setup          (guided wizard)")
				ui.Print("  gcm use <profile>  (if you already have profiles)")
			} else if ctr.Config.DefaultProfile == "" {
				ui.Blank()
				ui.Info("Global git identity was left unchanged. Activate a GCM profile when you want it managed:")
				ui.Print("  gcm setup")
				ui.Print("  gcm use <profile>")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall shell integration")
	cmd.Flags().BoolVar(&clearGlobalIdentity, "clear-global-identity", false, "Explicitly clear global git user identity")
	return cmd
}
