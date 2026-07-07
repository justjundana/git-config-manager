package cli

import (
	_ui "github.com/justjundana/git-config-manager/pkg/ui"
	_version "github.com/justjundana/git-config-manager/pkg/version"

	cobra "github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show GCM version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			info := _version.Get()

			if short {
				_ui.Print(info.Short())
				return nil
			}

			_ui.Header("Git Config Manager (GCM)")
			_ui.Blank()
			_ui.Detail("Version", info.Version)
			_ui.Detail("Commit", info.Commit)
			_ui.Detail("Built", info.Date)
			_ui.Detail("OS/Arch", info.OS+"/"+info.Arch)

			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "Short version output")
	return cmd
}
