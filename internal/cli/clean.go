package cli

import (
	"os"
	"path/filepath"

	_config "github.com/justjundana/git-config-manager/internal/config"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean cache and temporary files",
		RunE: func(_ *cobra.Command, _ []string) error {
			cacheDir := ctr.Config.CacheDir

			if all {
				// Also clean logs and old tokens
				logsDir := filepath.Join(_config.GCMDir(), "logs")
				if err := os.RemoveAll(logsDir); err != nil {
					_ui.Warning("Failed to clean logs: %v", err)
				} else {
					_ui.Success("Cleaned logs directory")
				}
			}

			if err := os.RemoveAll(cacheDir); err != nil {
				return err
			}
			if err := os.MkdirAll(cacheDir, 0755); err != nil {
				return err
			}

			_ui.Success("Cache cleaned")
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Also clean logs and temporary data")
	return cmd
}
