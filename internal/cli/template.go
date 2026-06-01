package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/justjundana/git-config-manager/internal/profile"
	"github.com/justjundana/git-config-manager/internal/template"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func newTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage configuration templates",
		Long: `Manage reusable Git configuration templates.

Templates store git settings (editor, aliases, commit, pull, push) as reusable
presets. They do NOT store identity (name, email, SSH keys, GPG keys).

Use templates to standardize settings across a team or quickly apply your
preferred git config to new profiles.`,
		Aliases: []string{"tpl"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return templateListRun()
		},
	}

	cmd.AddCommand(newTemplateCreateCmd())
	cmd.AddCommand(newTemplateListCmd())
	cmd.AddCommand(newTemplateShowCmd())
	cmd.AddCommand(newTemplateDeleteCmd())
	cmd.AddCommand(newTemplateExportCmd())
	cmd.AddCommand(newTemplateImportCmd())
	cmd.AddCommand(newTemplateApplyCmd())

	return cmd
}

func newTemplateCreateCmd() *cobra.Command {
	var (
		description string
		fromProfile string
		interactive bool
		editor      string
		rebase      string
		gpgSign     string
		aliases     []string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new template",
		Long: `Create a new configuration template.

Templates store reusable git settings (editor, pull strategy, signing, aliases).
You can create one interactively, from flags, or by extracting settings from
an existing profile.

Examples:
  gcm template create company-standard -i
  gcm template create company-standard --from-profile work
  gcm template create minimal --editor "code --wait" --rebase true
  gcm template create team --editor vim --gpg-sign true --alias "co=checkout,st=status"`,
		Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			if fromProfile != "" {
				return templateCreateFromProfile(name, fromProfile, description)
			}

			if interactive {
				return templateCreateInteractive(name)
			}

			// Create from flags
			t := &template.Template{
				Name:        name,
				Description: description,
				Git:         template.GitConfigTemplate{},
			}

			if editor != "" {
				t.Git.Core = map[string]interface{}{"editor": editor}
			}
			if rebase != "" {
				t.Git.Pull = map[string]interface{}{"rebase": rebase}
			}
			if gpgSign != "" {
				sign := gpgSign == "true" || gpgSign == "yes"
				t.Git.Commit = map[string]interface{}{"gpgsign": sign}
			}
			if len(aliases) > 0 {
				t.Git.Aliases = parseAliases(aliases)
			}

			// If nothing specified at all, prompt user
			if editor == "" && rebase == "" && gpgSign == "" && len(aliases) == 0 && description == "" {
				return templateCreateInteractive(name)
			}

			if err := ctr.TemplateManager.Create(t); err != nil {
				return err
			}

			ui.Success("Template %q created", name)
			templatePrintSummary(t)
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "description", "d", "", "Template description")
	cmd.Flags().StringVar(&fromProfile, "from-profile", "", "Extract settings from an existing profile")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive creation wizard")
	cmd.Flags().StringVar(&editor, "editor", "", "Git editor (e.g. 'vim', 'code --wait')")
	cmd.Flags().StringVar(&rebase, "rebase", "", "Pull rebase strategy (true/false/merges)")
	cmd.Flags().StringVar(&gpgSign, "gpg-sign", "", "Enable commit signing (true/false)")
	cmd.Flags().StringSliceVar(&aliases, "alias", nil, "Git aliases (format: key=value)")

	return cmd
}

func templateCreateInteractive(name string) error {
	ui.Header("%s Create Template: %s", ui.IconTemplate, name)
	ui.Blank()

	desc, err := ui.AskString("Description (what is this template for?):", "")
	if err != nil {
		return err
	}

	ui.SubHeader("Git Core Settings")
	editor, err := ui.AskString("Git editor (e.g. vim, code --wait, nano — leave empty to skip):", "")
	if err != nil {
		return err
	}

	ui.SubHeader("Pull Strategy")
	rebase, err := ui.AskSelect("Pull strategy:", []string{
		"rebase (recommended — linear history)",
		"merge (default git behavior)",
		"ff-only (fast-forward only)",
		"skip (don't set)",
	})
	if err != nil {
		return err
	}

	ui.SubHeader("Commit Signing")
	gpgSign, err := ui.AskConfirm("Require GPG-signed commits?", false)
	if err != nil {
		return err
	}

	ui.SubHeader("Push Settings")
	autoSetup, err := ui.AskConfirm("Auto setup remote tracking branch on push?", true)
	if err != nil {
		return err
	}

	ui.SubHeader("Git Aliases (optional)")
	ui.Print("  Enter aliases one per line (format: short=full command)")
	ui.Print("  Leave empty and press Enter to finish.")

	aliasMap := make(map[string]string)
	for {
		alias, askErr := ui.AskString("  Alias (e.g. co=checkout):", "")
		if askErr != nil {
			return askErr
		}
		if alias == "" {
			break
		}
		parts := strings.SplitN(alias, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			ui.Warning("Invalid format. Use: short=command (e.g. co=checkout)")
			continue
		}
		aliasMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	t := &template.Template{
		Name:        name,
		Description: desc,
		Git:         template.GitConfigTemplate{},
	}

	if editor != "" {
		t.Git.Core = map[string]interface{}{"editor": editor}
	}
	if rebase != "skip (don't set)" {
		pullMap := map[string]interface{}{}
		switch {
		case strings.HasPrefix(rebase, "rebase"):
			pullMap["rebase"] = "true"
		case strings.HasPrefix(rebase, "ff-only"):
			pullMap["ff"] = "only"
		}
		if len(pullMap) > 0 {
			t.Git.Pull = pullMap
		}
	}
	if gpgSign {
		t.Git.Commit = map[string]interface{}{"gpgsign": true}
	}
	if autoSetup {
		t.Git.Push = map[string]interface{}{"autosetupremote": true}
	}
	if len(aliasMap) > 0 {
		t.Git.Aliases = aliasMap
	}

	if err := ctr.TemplateManager.Create(t); err != nil {
		return err
	}

	ui.Blank()
	ui.Success("Template %q created!", name)
	templatePrintSummary(t)
	ui.NextSteps([]string{
		fmt.Sprintf("Apply to a profile: gcm template apply %s <profile>", name),
		fmt.Sprintf("Create profile from template: gcm profile create <name> --from-template %s -i", name),
		fmt.Sprintf("Share with team: gcm template export %s > %s.yaml", name, name),
	})
	return nil
}

func templateCreateFromProfile(name, profileName, description string) error {
	p, err := ctr.ProfileManager.Get(profileName)
	if err != nil {
		return err
	}

	t := &template.Template{
		Name:        name,
		Description: description,
		Git:         template.GitConfigTemplate{},
	}

	if t.Description == "" {
		t.Description = fmt.Sprintf("Extracted from profile %q", profileName)
	}

	// Extract core settings
	core := map[string]interface{}{}
	if p.Git.Core.Editor != "" {
		core["editor"] = p.Git.Core.Editor
	}
	if p.Git.Core.AutoCRLF != "" {
		core["autocrlf"] = p.Git.Core.AutoCRLF
	}
	if p.Git.Core.EOL != "" {
		core["eol"] = p.Git.Core.EOL
	}
	if p.Git.Core.FileMode != nil {
		core["filemode"] = *p.Git.Core.FileMode
	}
	if p.Git.Core.IgnoreCase != nil {
		core["ignorecase"] = *p.Git.Core.IgnoreCase
	}
	if len(core) > 0 {
		t.Git.Core = core
	}

	// Extract commit settings
	commit := map[string]interface{}{}
	if p.Git.Commit.GPGSign != nil {
		commit["gpgsign"] = *p.Git.Commit.GPGSign
	}
	if p.Git.Commit.Template != "" {
		commit["template"] = p.Git.Commit.Template
	}
	if p.Git.Commit.Verbose != nil {
		commit["verbose"] = *p.Git.Commit.Verbose
	}
	if len(commit) > 0 {
		t.Git.Commit = commit
	}

	// Extract pull settings
	pull := map[string]interface{}{}
	if p.Git.Pull.Rebase != "" {
		pull["rebase"] = p.Git.Pull.Rebase
	}
	if p.Git.Pull.FF != "" {
		pull["ff"] = p.Git.Pull.FF
	}
	if len(pull) > 0 {
		t.Git.Pull = pull
	}

	// Extract push settings
	push := map[string]interface{}{}
	if p.Git.Push.Default != "" {
		push["default"] = p.Git.Push.Default
	}
	if p.Git.Push.FollowTags != nil {
		push["followtags"] = *p.Git.Push.FollowTags
	}
	if p.Git.Push.AutoSetupRemote != nil {
		push["autosetupremote"] = *p.Git.Push.AutoSetupRemote
	}
	if len(push) > 0 {
		t.Git.Push = push
	}

	// Extract aliases
	if len(p.Git.Aliases) > 0 {
		t.Git.Aliases = p.Git.Aliases
	}

	if err := ctr.TemplateManager.Create(t); err != nil {
		return err
	}

	ui.Success("Template %q created from profile %q", name, profileName)
	templatePrintSummary(t)
	return nil
}

func newTemplateApplyCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "apply <template> <profile>",
		Short: "Apply template settings to a profile",
		Long: `Apply a template's git settings to an existing profile.

This merges the template's settings (editor, pull strategy, commit signing,
push settings, aliases) into the target profile. Identity fields (name, email,
SSH, GPG keys) are never modified.

Examples:
  gcm template apply company-standard work
  gcm template apply company-standard work --force   # skip confirmation`,
		Args: requireArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			templateName := args[0]
			profileName := args[1]

			t, err := ctr.TemplateManager.Get(templateName)
			if err != nil {
				return err
			}

			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return err
			}

			// Show what will change
			ui.Header("Applying template %q to profile %q", templateName, profileName)
			ui.Blank()
			changes := templateShowChanges(t, p)
			if changes == 0 {
				ui.Info("No changes to apply — template has no settings configured")
				return nil
			}

			if !force {
				ui.Blank()
				confirm, askErr := ui.AskConfirm("Apply these changes?", true)
				if askErr != nil {
					return askErr
				}
				if !confirm {
					ui.Info("Cancelled")
					return nil
				}
			}

			// Apply
			applyTemplateToProfile(t, p)

			if err := ctr.ProfileManager.Update(p); err != nil {
				return err
			}

			ui.Blank()
			ui.Success("Template %q applied to profile %q", templateName, profileName)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	return cmd
}

func templateShowChanges(t *template.Template, _ *profile.Profile) int {
	count := 0

	if t.Git.Core != nil {
		for k, v := range t.Git.Core {
			ui.Print("  %s core.%s = %v", ui.Green("+"), k, v)
			count++
		}
	}
	if t.Git.Commit != nil {
		for k, v := range t.Git.Commit {
			ui.Print("  %s commit.%s = %v", ui.Green("+"), k, v)
			count++
		}
	}
	if t.Git.Pull != nil {
		for k, v := range t.Git.Pull {
			ui.Print("  %s pull.%s = %v", ui.Green("+"), k, v)
			count++
		}
	}
	if t.Git.Push != nil {
		for k, v := range t.Git.Push {
			ui.Print("  %s push.%s = %v", ui.Green("+"), k, v)
			count++
		}
	}
	if len(t.Git.Aliases) > 0 {
		for k, v := range t.Git.Aliases {
			ui.Print("  %s alias.%s = %s", ui.Green("+"), k, v)
			count++
		}
	}
	return count
}

func applyTemplateToProfile(t *template.Template, p *profile.Profile) {
	// Apply core settings
	if t.Git.Core != nil {
		if v, ok := t.Git.Core["editor"]; ok {
			p.Git.Core.Editor = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Core["autocrlf"]; ok {
			p.Git.Core.AutoCRLF = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Core["eol"]; ok {
			p.Git.Core.EOL = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Core["filemode"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Core.FileMode = &b
			}
		}
		if v, ok := t.Git.Core["ignorecase"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Core.IgnoreCase = &b
			}
		}
	}

	// Apply commit settings
	if t.Git.Commit != nil {
		if v, ok := t.Git.Commit["gpgsign"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Commit.GPGSign = &b
			}
		}
		if v, ok := t.Git.Commit["template"]; ok {
			p.Git.Commit.Template = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Commit["verbose"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Commit.Verbose = &b
			}
		}
	}

	// Apply pull settings
	if t.Git.Pull != nil {
		if v, ok := t.Git.Pull["rebase"]; ok {
			p.Git.Pull.Rebase = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Pull["ff"]; ok {
			p.Git.Pull.FF = fmt.Sprintf("%v", v)
		}
	}

	// Apply push settings
	if t.Git.Push != nil {
		if v, ok := t.Git.Push["default"]; ok {
			p.Git.Push.Default = fmt.Sprintf("%v", v)
		}
		if v, ok := t.Git.Push["followtags"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Push.FollowTags = &b
			}
		}
		if v, ok := t.Git.Push["autosetupremote"]; ok {
			if b, isBool := v.(bool); isBool {
				p.Git.Push.AutoSetupRemote = &b
			}
		}
	}

	// Apply aliases (merge, don't replace)
	if len(t.Git.Aliases) > 0 {
		if p.Git.Aliases == nil {
			p.Git.Aliases = make(map[string]string)
		}
		for k, v := range t.Git.Aliases {
			p.Git.Aliases[k] = v
		}
	}
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List templates", Aliases: []string{"ls"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return templateListRun()
		},
	}
}

func templateListRun() error {
	templates, err := ctr.TemplateManager.List()
	if err != nil {
		return err
	}

	if len(templates) == 0 {
		ui.Info("No templates found. Create one:")
		ui.Print("  gcm template create <name> -i")
		ui.Print("  gcm template create <name> --from-profile <profile>")
		return nil
	}

	headers := []string{"Template", "Description", "Version", "Created"}
	var rows [][]string
	for _, t := range templates {
		rows = append(rows, []string{
			t.Name, t.Description, t.Metadata.Version,
			t.Metadata.Created.Format("2006-01-02"),
		})
	}

	ui.SimpleTable(headers, rows)
	return nil
}

func newTemplateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <name>", Short: "Show template details", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := ctr.TemplateManager.Get(args[0])
			if err != nil {
				return err
			}

			ui.Header("Template: %s", t.Name)
			if t.Description != "" {
				ui.Detail("Description", t.Description)
			}
			ui.Detail("Version", t.Metadata.Version)
			ui.Detail("Created", t.Metadata.Created.Format("2006-01-02 15:04"))
			if t.Metadata.Author != "" {
				ui.Detail("Author", t.Metadata.Author)
			}

			ui.Blank()
			ui.SubHeader("Git Settings")

			if t.Git.Core != nil {
				for k, v := range t.Git.Core {
					ui.Detail(fmt.Sprintf("core.%s", k), fmt.Sprintf("%v", v))
				}
			}
			if t.Git.Commit != nil {
				for k, v := range t.Git.Commit {
					ui.Detail(fmt.Sprintf("commit.%s", k), fmt.Sprintf("%v", v))
				}
			}
			if t.Git.Pull != nil {
				for k, v := range t.Git.Pull {
					ui.Detail(fmt.Sprintf("pull.%s", k), fmt.Sprintf("%v", v))
				}
			}
			if t.Git.Push != nil {
				for k, v := range t.Git.Push {
					ui.Detail(fmt.Sprintf("push.%s", k), fmt.Sprintf("%v", v))
				}
			}
			if len(t.Git.Aliases) > 0 {
				ui.SubHeader("Aliases")
				for k, v := range t.Git.Aliases {
					ui.Detail(k, v)
				}
			}

			return nil
		},
	}
}

func newTemplateDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use: "delete <name>", Short: "Delete a template", Aliases: []string{"rm"}, Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				confirm, err := ui.AskConfirm(fmt.Sprintf("Delete template %q?", args[0]), false)
				if err != nil || !confirm {
					ui.Info("Cancelled")
					return nil
				}
			}
			if err := ctr.TemplateManager.Delete(args[0]); err != nil {
				return err
			}
			ui.Success("Template %q deleted", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newTemplateExportCmd() *cobra.Command {
	return &cobra.Command{
		Use: "export <name>", Short: "Export template (YAML)", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := ctr.TemplateManager.Export(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, string(data))
			return nil
		},
	}
}

func newTemplateImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import template from file",
		Long: `Import a template from a YAML file.

The file must contain a valid template YAML with at least a 'name' field.

Example file (my-template.yaml):
  name: my-template
  description: "My standard git settings"
  git:
    core:
      editor: "code --wait"
    commit:
      gpgsign: true
    pull:
      rebase: "true"
    aliases:
      co: checkout
      st: status`,
		Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			t, err := ctr.TemplateManager.Import(data)
			if err != nil {
				return err
			}
			ui.Success("Template %q imported", t.Name)
			templatePrintSummary(t)
			return nil
		},
	}
}

func templatePrintSummary(t *template.Template) {
	if t.Description != "" {
		ui.Detail("Description", t.Description)
	}
	settings := 0
	if t.Git.Core != nil {
		settings += len(t.Git.Core)
	}
	if t.Git.Commit != nil {
		settings += len(t.Git.Commit)
	}
	if t.Git.Pull != nil {
		settings += len(t.Git.Pull)
	}
	if t.Git.Push != nil {
		settings += len(t.Git.Push)
	}
	if len(t.Git.Aliases) > 0 {
		settings += len(t.Git.Aliases)
	}
	ui.Detail("Settings", fmt.Sprintf("%d configured", settings))
}

func parseAliases(input []string) map[string]string {
	result := make(map[string]string)
	for _, item := range input {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}
