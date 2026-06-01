// Package cli implements all Cobra CLI commands for GCM.
package cli

import (
	"github.com/justjundana/git-config-manager/internal/container"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

// ctr is set by SetContainer before commands execute.
var ctr *container.Container

// SetContainer injects the dependency container into CLI commands.
func SetContainer(c *container.Container) {
	ctr = c
}

// NewRootCmd creates the root gcm command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gcm",
		Short: "Git Config Manager",
		Long: "Git Config Manager (GCM) - Manage your Git identities with ease.\n\n" +
			"GCM helps you manage multiple provider-scoped Git identities, SSH keys,\n" +
			"GPG keys, and provider accounts from a single, intuitive CLI tool.\n\n" +
			ui.Bold("Getting Started") + "\n" +
			"  gcm setup                     Guided first-time setup (start here!)\n" +
			"  gcm status                    See your current state at a glance\n" +
			"  gcm repair                    Inspect profile/provider consistency\n\n" +
			ui.Bold("Daily Use") + "\n" +
			"  gcm use <profile>             Switch to a profile\n" +
			"  gcm current                   Show which profile is active\n" +
			"  gcm profile list              See all profiles\n" +
			"  gcm connect <profile>         Connect a profile to GitHub/GitLab\n\n" +
			ui.Bold("Management") + "\n" +
			"  gcm profile create <name> -i  Create a new profile interactively\n" +
			"  gcm ssh generate <profile>    Generate SSH key for a profile\n" +
			"  gcm switch-provider <p> <id>  Move a profile to another provider\n\n" +
			"Run \"gcm <command> --help\" for details on any command.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// ─── Getting Started ───
	rootCmd.AddCommand(newSetupCmd())
	rootCmd.AddCommand(newStatusCmd())

	// ─── Daily Use ───
	rootCmd.AddCommand(newUseCmd())
	rootCmd.AddCommand(newCurrentCmd())
	rootCmd.AddCommand(newRefreshCmd())

	// ─── Profile Management ───
	rootCmd.AddCommand(newProfileCmd())

	// ─── Key Management ───
	rootCmd.AddCommand(newSSHCmd())
	rootCmd.AddCommand(newGPGCmd())

	// ─── Providers ───
	rootCmd.AddCommand(newConnectCmd())
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newSwitchProviderCmd())
	rootCmd.AddCommand(newGitHubCmd())
	rootCmd.AddCommand(newGitLabCmd())

	// ─── Shell ───
	rootCmd.AddCommand(newInitCmd())

	// ─── Templates ───
	rootCmd.AddCommand(newTemplateCmd())

	// ─── Backup ───
	rootCmd.AddCommand(newBackupCmd())

	// ─── Diagnostics ───
	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newRepairCmd())

	// ─── Utilities ───
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newCleanCmd())

	// ─── Internal (used by git) ───
	rootCmd.AddCommand(newCredentialHelperCmd())

	return rootCmd
}
