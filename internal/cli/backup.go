package cli

import (
	"fmt"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup and restore GCM data",
	}

	cmd.AddCommand(newBackupCreateCmd())
	cmd.AddCommand(newBackupListCmd())
	cmd.AddCommand(newBackupRestoreCmd())
	cmd.AddCommand(newBackupPruneCmd())

	return cmd
}

func newBackupCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use: "create", Short: "Create a backup",
		RunE: func(_ *cobra.Command, _ []string) error {
			sp := _ui.NewSpinner("Creating backup...")
			sp.Start()

			info, err := ctr.BackupManager.Create()
			if err != nil {
				sp.StopError("Backup failed")
				ctr.AuditLogger.Log(_audit.ActionBackupCreate, "", nil, err)
				return err
			}
			ctr.AuditLogger.Log(_audit.ActionBackupCreate, "",
				map[string]string{"path": info.Path}, nil)

			sp.Stop("Backup created!")
			_ui.Blank()
			_ui.Detail("Path", info.Path)
			_ui.Detail("Profiles", fmt.Sprintf("%d", info.Profiles))
			_ui.Detail("Templates", fmt.Sprintf("%d", info.Templates))
			_ui.Detail("Size", formatBytes(info.Size))

			return nil
		},
	}
}

func newBackupListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List backups", Aliases: []string{"ls"},
		RunE: func(_ *cobra.Command, _ []string) error {
			backups, err := ctr.BackupManager.List()
			if err != nil {
				return err
			}

			if len(backups) == 0 {
				_ui.Info("No backups found. Create one: gcm backup create")
				return nil
			}

			headers := []string{"Date", "Size", "Path"}
			var rows [][]string
			for _, b := range backups {
				rows = append(rows, []string{
					b.Created.Format("2006-01-02 15:04"),
					formatBytes(b.Size),
					b.Path,
				})
			}

			_ui.SimpleTable(headers, rows)
			return nil
		},
	}
}

func newBackupRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use: "restore <file>", Short: "Restore from backup", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !ctr.FileService.Exists(args[0]) {
				_ui.Error("backup file not found: %s", args[0])
				_ui.Blank()
				_ui.Print("  To see available backups: gcm backup list")
				return fmt.Errorf("backup file not found: %s", args[0])
			}

			confirm, err := _ui.AskConfirm("This will overwrite current data. Continue?", false)
			if err != nil || !confirm {
				_ui.Info("Cancelled")
				return nil
			}

			sp := _ui.NewSpinner("Restoring backup...")
			sp.Start()

			if err := ctr.BackupManager.Restore(args[0]); err != nil {
				sp.StopError("Restore failed")
				ctr.AuditLogger.Log(_audit.ActionBackupRestore, "",
					map[string]string{"path": args[0]}, err)
				return err
			}
			ctr.AuditLogger.Log(_audit.ActionBackupRestore, "",
				map[string]string{"path": args[0]}, nil)

			sp.Stop("Backup restored!")
			return nil
		},
	}
}

func newBackupPruneCmd() *cobra.Command {
	var keep int

	cmd := &cobra.Command{
		Use: "prune", Short: "Remove old backups",
		RunE: func(_ *cobra.Command, _ []string) error {
			removed, err := ctr.BackupManager.Prune(keep)
			if err != nil {
				return err
			}

			if removed == 0 {
				_ui.Info("No backups to remove")
			} else {
				_ui.Success("Removed %d old backups", removed)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&keep, "keep", 5, "Number of recent backups to keep")
	return cmd
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
