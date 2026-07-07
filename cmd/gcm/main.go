// GCM is the GitHub Config Manager CLI entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_cli "github.com/justjundana/git-config-manager/internal/cli"
	_config "github.com/justjundana/git-config-manager/internal/config"
	_container "github.com/justjundana/git-config-manager/internal/container"
	_logger "github.com/justjundana/git-config-manager/pkg/logger"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

// Replaceable for testing.
var (
	configLoad           = _config.Load
	configEnsureDirs     = _config.EnsureDirs
	osExit               = os.Exit
	masterPasswordPrompt = func(msg string) (string, error) {
		return _ui.AskPassword(msg)
	}
)

func main() {
	osExit(run(os.Args[1:]))
}

func run(args []string) int {
	// Load configuration
	cfg, err := configLoad()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Ensure data directories exist
	if err := configEnsureDirs(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		return 1
	}

	// Initialize logger
	log := _logger.New(_logger.LevelInfo, os.Stderr)

	// Handle UI settings
	if !cfg.UI.Color {
		_ui.DisableColor()
	}
	if cfg.UI.Verbose {
		log.SetVerbose(true)
	}
	if cfg.UI.Quiet {
		log.SetQuiet(true)
	}

	// Create dependency container
	ctr := _container.New(cfg, log)

	// Wire the master-password prompt so encrypted token storage can ask
	// the user interactively. Uses the ui.AskPassword helper which
	// suppresses echo when stdin is a terminal.
	ctr.SetMasterPasswordPrompt(masterPasswordPrompt)

	// Wire CLI commands
	_cli.SetContainer(ctr)
	rootCmd := _cli.NewRootCmd()

	// Global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-essential output")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("verbose"); v {
			log.SetVerbose(true)
		}
		if nc, _ := cmd.Flags().GetBool("no-color"); nc {
			_ui.DisableColor()
		}
		if q, _ := cmd.Flags().GetBool("quiet"); q {
			log.SetQuiet(true)
		}
	}

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetArgs(args)

	// Cancel in-flight API calls on SIGINT/SIGTERM so resources are freed
	// promptly on Ctrl-C rather than waiting for the HTTP timeout.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		_ui.Error("%v", err)
		ctr.TokenStore.ZeroPassword()
		return 1
	}

	// Zero sensitive material from memory before process exit.
	ctr.TokenStore.ZeroPassword()
	return 0
}
