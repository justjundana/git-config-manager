package cli

import (
	"fmt"
	"strings"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_shell "github.com/justjundana/git-config-manager/internal/shell"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
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
			if shellType == _shell.ShellUnknown {
				_ui.Error("Could not detect your shell")
				_ui.Info("Set SHELL environment variable and retry: SHELL=/bin/zsh gcm init")
				return fmt.Errorf("could not detect shell")
			}

			_ui.Header("%s Setting up GCM for %s", _ui.IconRocket, string(shellType))
			_ui.Blank()

			installed, configFile := ctr.ShellManager.IsInstalled(shellType)

			if installed && !force {
				_ui.Success("Shell integration already installed!")
				_ui.Detail("Shell", string(shellType))
				_ui.Detail("Config", configFile)
				_ui.Blank()
				_ui.Print("  To force reinstall: gcm init --force")
			} else {
				// Force reinstall: uninstall first if already present
				if installed && force {
					if _, err := ctr.ShellManager.Uninstall(shellType); err != nil {
						_ui.Warning("Could not uninstall existing hooks: %v", err)
					}
				}

				newConfigFile, err := ctr.ShellManager.Install(shellType)
				if err != nil {
					return err
				}

				if force && installed {
					_ui.Success("Shell integration reinstalled!")
				} else {
					_ui.Success("Shell integration installed!")
				}
				ctr.AuditLogger.Log(_audit.ActionShellInit, "",
					map[string]string{"shell": string(shellType), "config": newConfigFile}, nil)
				_ui.Detail("Shell", string(shellType))
				_ui.Detail("Config", newConfigFile)

				_ui.Blank()
				_ui.Info("Restart your shell or run: source %s", newConfigFile)
			}

			// Register GCM as credential helper for configured providers.
			_ui.Blank()
			if err := RegisterCredentialHelper(); err != nil {
				_ui.Warning("Could not register credential helper: %v", err)
				_ui.Print("  Git will fall back to the system keychain for credentials.")
			} else {
				_ui.Success("Git credential helper registered!")
				_ui.Detail("Scope", strings.Join(credentialHelperServers(), ", "))
			}

			if clearGlobalIdentity {
				if err := ctr.ProfileSwitcher.ClearGlobalIdentity(); err != nil {
					return fmt.Errorf("clear global git identity: %w", err)
				}
				_ui.Blank()
				_ui.Info("Global git identity cleared by explicit request — activate a profile to set your identity:")
				_ui.Print("  gcm setup          (guided wizard)")
				_ui.Print("  gcm use <profile>  (if you already have profiles)")
			} else if ctr.Config.DefaultProfile == "" {
				_ui.Blank()
				_ui.Info("Global git identity was left unchanged. Activate a GCM profile when you want it managed:")
				_ui.Print("  gcm setup")
				_ui.Print("  gcm use <profile>")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall shell integration")
	cmd.Flags().BoolVar(&clearGlobalIdentity, "clear-global-identity", false, "Explicitly clear global git user identity")
	return cmd
}
