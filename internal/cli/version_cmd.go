package cli

import (
	"github.com/justjundana/git-config-manager/pkg/ui"
	"github.com/justjundana/git-config-manager/pkg/version"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show GCM version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			info := version.Get()

			if short {
				ui.Print(info.Short())
				return nil
			}

			ui.Header("Git Config Manager (GCM)")
			ui.Blank()
			ui.Detail("Version", info.Version)
			ui.Detail("Commit", info.Commit)
			ui.Detail("Built", info.Date)
			ui.Detail("OS/Arch", info.OS+"/"+info.Arch)

			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "Short version output")
	return cmd
}
