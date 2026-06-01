package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/justjundana/git-config-manager/internal/audit"
	"github.com/justjundana/git-config-manager/internal/gpg"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/ssh"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Short:   "Manage Git profiles",
		Long:    "Create, list, show, edit, delete, and export/import Git profiles.",
		Aliases: []string{"p"},
		RunE:    profileListRun,
	}

	cmd.AddCommand(newProfileCreateCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileShowCmd())
	cmd.AddCommand(newProfileEditCmd())
	cmd.AddCommand(newProfileDeleteCmd())
	cmd.AddCommand(newProfileExportCmd())
	cmd.AddCommand(newProfileImportCmd())
	cmd.AddCommand(newProfileDiffCmd())

	return cmd
}

func newProfileCreateCmd() *cobra.Command {
	var (
		name         string
		email        string
		interactive  bool
		editor       string
		sshKey       string
		fromTemplate string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new Git profile",
		Long: `Create a new Git profile with your identity information.

Examples:
  gcm profile create work --interactive
  gcm profile create work --name "John Doe" --email "john@company.com"
  gcm profile create work --from-template company-standard -i`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			if interactive {
				return profileCreateInteractive(profileName, fromTemplate)
			}

			if name == "" || email == "" {
				return fmt.Errorf("--name and --email are required (or use --interactive)")
			}

			p := &profile.Profile{
				Name: profileName,
				Git: profile.GitConfig{
					User: profile.GitUser{Name: name, Email: email},
					Core: profile.GitCore{Editor: editor},
				},
			}

			if sshKey != "" {
				p.SSH = &profile.SSHConfig{KeyPath: sshKey}
			}

			// Apply template settings if specified
			if fromTemplate != "" {
				t, err := ctr.TemplateManager.Get(fromTemplate)
				if err != nil {
					return fmt.Errorf("loading template: %w", err)
				}
				applyTemplateToProfile(t, p)
			}

			if err := ctr.ProfileManager.Create(p); err != nil {
				ctr.AuditLogger.Log(audit.ActionProfileCreate, profileName, nil, err)
				return err
			}
			ctr.AuditLogger.Log(audit.ActionProfileCreate, profileName, nil, nil)

			ui.Success("Profile %q created successfully", profileName)
			ui.Blank()
			ui.Detail("Name", name)
			ui.Detail("Email", email)
			if fromTemplate != "" {
				ui.Detail("Template", fromTemplate)
			}

			// If this is the only profile, activate as global default automatically
			if activated := activateIfOnlyProfile(profileName); activated {
				return nil
			}
			ui.NextSteps([]string{
				fmt.Sprintf("Generate SSH key: gcm ssh generate %s", profileName),
				fmt.Sprintf("Activate profile: gcm use %s", profileName),
			})
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Your full name")
	cmd.Flags().StringVarP(&email, "email", "e", "", "Your email address")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive wizard mode")
	cmd.Flags().StringVar(&editor, "editor", "", "Git editor")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to existing SSH key")
	cmd.Flags().StringVarP(&fromTemplate, "from-template", "t", "", "Apply settings from a template")

	return cmd
}

func profileCreateInteractive(profileName string, fromTemplate string) error {
	ui.Header("%s Create New Profile: %s", ui.IconProfile, profileName)

	// If template specified, show what will be applied
	if fromTemplate != "" {
		t, err := ctr.TemplateManager.Get(fromTemplate)
		if err != nil {
			return fmt.Errorf("loading template: %w", err)
		}
		ui.Print("  Using template: %s", ui.Cyan(fromTemplate))
		if t.Description != "" {
			ui.Print("  %s", ui.Dim(t.Description))
		}
		ui.Blank()
	}

	ui.SubHeader("Step 1/4: Basic Information")
	name, err := ui.AskString("Your full name:", "")
	if err != nil {
		return err
	}
	email, err := ui.AskString("Your email:", "")
	if err != nil {
		return err
	}
	editor, err := ui.AskString("Git editor (leave empty for default):", "")
	if err != nil {
		return err
	}

	ui.SubHeader("Step 2/4: SSH Configuration")
	generateSSH, err := ui.AskConfirm("Generate a new SSH key?", true)
	if err != nil {
		return err
	}

	var sshConfig *profile.SSHConfig
	if generateSSH {
		keyType, askErr := ui.AskSelect("SSH key type:", []string{"ed25519 (recommended)", "rsa (4096 bits)", "ecdsa"})
		if askErr != nil {
			return askErr
		}
		keyTypeClean := "ed25519"
		if keyType == "rsa (4096 bits)" {
			keyTypeClean = "rsa"
		} else if keyType == "ecdsa" {
			keyTypeClean = "ecdsa"
		}

		sp := ui.NewSpinner("Generating SSH key...")
		sp.Start()
		keyInfo, genErr := ctr.SSHManager.Generate(ssh.GenerateOptions{
			Profile: profileName,
			KeyType: keyTypeClean,
		})
		if genErr != nil {
			sp.StopError("Failed to generate SSH key")
			ui.Warning("SSH key generation failed: %v", genErr)
		} else {
			sp.Stop("SSH key generated")
			sshConfig = &profile.SSHConfig{
				KeyPath:     keyInfo.Path,
				KeyType:     profile.KeyType(keyInfo.Type),
				Fingerprint: keyInfo.Fingerprint,
			}
		}
	}

	ui.SubHeader("Step 3/4: GPG Signing")
	enableGPG, err := ui.AskConfirm("Enable commit signing?", true)
	if err != nil {
		return err
	}

	var gpgConfig *profile.GPGConfig
	if enableGPG {
		sp := ui.NewSpinner("Generating GPG key...")
		sp.Start()
		keyInfo, genErr := ctr.GPGManager.Generate(gpg.GenerateOptions{
			Name: name, Email: email,
		})
		if genErr != nil {
			sp.StopError("Failed to generate GPG key")
			ui.Warning("GPG key generation failed: %v", genErr)
		} else {
			sp.Stop("GPG key generated")
			gpgConfig = &profile.GPGConfig{KeyID: keyInfo.KeyID}
		}
	}

	p := &profile.Profile{
		Name: profileName,
		Git: profile.GitConfig{
			User: profile.GitUser{Name: name, Email: email},
			Core: profile.GitCore{Editor: editor},
		},
		SSH: sshConfig, GPG: gpgConfig,
	}

	ui.SubHeader("Step 4/4: Provider Account (Optional)")
	if err := promptProviderAccountUsernames(p); err != nil {
		return err
	}

	if gpgConfig != nil {
		p.Git.Commit.GPGSign = profile.BoolPtr(true)
		p.Git.User.SigningKey = gpgConfig.KeyID
	}

	// Apply template settings if specified
	if fromTemplate != "" {
		t, _ := ctr.TemplateManager.Get(fromTemplate)
		if t != nil {
			applyTemplateToProfile(t, p)
		}
	}

	if err := ctr.ProfileManager.Create(p); err != nil {
		ctr.AuditLogger.Log(audit.ActionProfileCreate, profileName, nil, err)
		return err
	}
	ctr.AuditLogger.Log(audit.ActionProfileCreate, profileName, nil, nil)

	ui.Blank()
	ui.Success("Profile %q created and configured!", profileName)

	// If this is the only profile, activate as global default automatically
	if activated := activateIfOnlyProfile(profileName); activated {
		return nil
	}
	ui.NextSteps([]string{
		fmt.Sprintf("Activate it now: gcm use %s", profileName),
		"Create more profiles: gcm profile create <name> -i",
	})

	return nil
}

// activateIfOnlyProfile activates the profile as global default if it is
// the only profile in the system. Returns true if activation was performed.
func activateIfOnlyProfile(profileName string) bool {
	allProfiles, _ := ctr.ProfileManager.List()
	if len(allProfiles) != 1 {
		return false
	}
	if ctr.Config.DefaultProfile != "" {
		return false
	}
	if err := ctr.ProfileSwitcher.Activate(profileName, profile.ScopeGlobal); err != nil {
		ui.Warning("Could not activate globally: %v", err)
		return false
	}
	ui.Success("Profile %q set as global default (only profile)", profileName)
	return true
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List all profiles", Aliases: []string{"ls"},
		RunE: profileListRun,
	}
}

func profileListRun(_ *cobra.Command, _ []string) error {
	profiles, err := ctr.ProfileManager.List()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		ui.Info("No profiles found. Create one with: gcm profile create <name> -i")
		return nil
	}

	currentName, _, _ := ctr.ProfileSwitcher.Current()

	headers := []string{"Profile", "Status", "Email", "Provider", "Signing", "Last Used"}
	var rows [][]string

	for _, p := range profiles {
		status := ""
		if p.Name == currentName && p.Name == ctr.Config.DefaultProfile {
			status = ui.Green("●") + " " + ui.Cyan("default")
		} else if p.Name == currentName {
			status = ui.Green("●")
		} else if p.Name == ctr.Config.DefaultProfile {
			status = ui.Cyan("default")
		}

		signing := ui.Red("✗")
		if p.Git.Commit.GPGSign != nil && *p.Git.Commit.GPGSign {
			signing = ui.Green("✓")
		}

		lastUsed := ui.Dim("never")
		if p.Metadata.LastUsed != nil {
			lastUsed = formatTimeAgo(*p.Metadata.LastUsed)
		}

		providerName := ui.Dim("—")
		if profileHasMultipleProviders(p) {
			providerName = ui.Yellow("multiple")
		} else if def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper); ok {
			providerName = def.DisplayName
		}

		rows = append(rows, []string{p.Name, status, p.Git.User.Email, providerName, signing, lastUsed})
	}

	ui.SimpleTable(headers, rows)
	ui.Blank()
	ui.Print("%d profiles", len(profiles))
	return nil
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <name>", Short: "Show profile details", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			p, err := ctr.ProfileManager.Get(args[0])
			if err != nil {
				ui.Error("profile %q not found", args[0])
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[0])
			}
			ui.Header("Profile: %s", p.Name)
			ui.SubHeader("Git Configuration")
			ui.Detail("Name", p.Git.User.Name)
			ui.Detail("Email", p.Git.User.Email)
			if p.Git.Core.Editor != "" {
				ui.Detail("Editor", p.Git.Core.Editor)
			}
			if p.SSH != nil {
				ui.SubHeader("SSH")
				ui.Detail("Key", p.SSH.KeyPath)
				ui.Detail("Type", string(p.SSH.KeyType))
				if p.SSH.Fingerprint != "" {
					ui.Detail("Fingerprint", p.SSH.Fingerprint)
				}
			}
			if p.GPG != nil {
				ui.SubHeader("GPG")
				ui.Detail("Key ID", p.GPG.KeyID)
			}
			printProfileProviderAccounts(p)
			ui.SubHeader("Metadata")
			ui.Detail("Created", p.Metadata.Created.Format("2006-01-02 15:04:05"))
			ui.Detail("Usage", fmt.Sprintf("%d", p.Metadata.UsageCount))
			return nil
		},
	}
}

func newProfileEditCmd() *cobra.Command {
	var (
		name        string
		email       string
		editor      string
		signingKey  string
		interactive bool
	)
	cmd := &cobra.Command{
		Use:   "edit <profile>",
		Short: "Edit a profile",
		Long: `Edit a Git profile's configuration.

Without flags, opens an interactive editor showing current values.
With flags, updates only the specified fields.

Examples:
  gcm profile edit work --interactive
  gcm profile edit work --name "New Name" --email "new@email.com"
  gcm profile edit work -e "updated@email.com"`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				ui.Error("profile %q not found", profileName)
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(profileName)
			}

			// Determine if any flag was explicitly set
			flagsSet := cmd.Flags().Changed("name") || cmd.Flags().Changed("email") ||
				cmd.Flags().Changed("editor") || cmd.Flags().Changed("signing-key")

			if !flagsSet || interactive {
				return profileEditInteractive(p)
			}

			// Flag-based update
			changed := false
			if cmd.Flags().Changed("name") {
				p.Git.User.Name = name
				changed = true
			}
			if cmd.Flags().Changed("email") {
				p.Git.User.Email = email
				changed = true
			}
			if cmd.Flags().Changed("editor") {
				p.Git.Core.Editor = editor
				changed = true
			}
			if cmd.Flags().Changed("signing-key") {
				p.Git.User.SigningKey = signingKey
				changed = true
			}

			if !changed {
				ui.Info("No changes specified. Use --interactive for guided editing.")
				return nil
			}

			if err := ctr.ProfileManager.Update(p); err != nil {
				return err
			}
			ctr.AuditLogger.Log(audit.ActionProfileUpdate, profileName, nil, nil)
			ui.Success("Profile %q updated", profileName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "Update name")
	cmd.Flags().StringVarP(&email, "email", "e", "", "Update email")
	cmd.Flags().StringVar(&editor, "editor", "", "Update git editor")
	cmd.Flags().StringVar(&signingKey, "signing-key", "", "Update signing key ID")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive edit mode")
	return cmd
}

func profileEditInteractive(p *profile.Profile) error {
	ui.Header("%s Edit Profile: %s", ui.IconProfile, p.Name)

	ui.SubHeader("Basic Information")
	newName, err := ui.AskString(fmt.Sprintf("Name [%s]:", p.Git.User.Name), p.Git.User.Name)
	if err != nil {
		return err
	}
	newEmail, err := ui.AskString(fmt.Sprintf("Email [%s]:", p.Git.User.Email), p.Git.User.Email)
	if err != nil {
		return err
	}

	editorDefault := p.Git.Core.Editor
	editorPrompt := "Git editor (leave empty for default):"
	if editorDefault != "" {
		editorPrompt = fmt.Sprintf("Git editor [%s]:", editorDefault)
	}
	newEditor, err := ui.AskString(editorPrompt, editorDefault)
	if err != nil {
		return err
	}

	ui.SubHeader("Signing Configuration")
	signingKeyDefault := p.Git.User.SigningKey
	signingPrompt := "Signing key ID (leave empty to skip):"
	if signingKeyDefault != "" {
		signingPrompt = fmt.Sprintf("Signing key ID [%s]:", signingKeyDefault)
	}
	newSigningKey, err := ui.AskString(signingPrompt, signingKeyDefault)
	if err != nil {
		return err
	}

	gpgSignEnabled := p.Git.Commit.GPGSign != nil && *p.Git.Commit.GPGSign
	newGPGSign, err := ui.AskConfirm("Enable commit signing?", gpgSignEnabled)
	if err != nil {
		return err
	}

	ui.SubHeader("Provider Account")
	if err := promptProviderAccountUsernames(p); err != nil {
		return err
	}

	// Apply changes
	p.Git.User.Name = newName
	p.Git.User.Email = newEmail
	p.Git.Core.Editor = newEditor
	p.Git.User.SigningKey = newSigningKey
	p.Git.Commit.GPGSign = profile.BoolPtr(newGPGSign)

	// Update GPG config key ID if signing key changed
	if newSigningKey != "" {
		if p.GPG == nil {
			p.GPG = &profile.GPGConfig{}
		}
		p.GPG.KeyID = newSigningKey
	}

	if err := ctr.ProfileManager.Update(p); err != nil {
		return err
	}
	ctr.AuditLogger.Log(audit.ActionProfileUpdate, p.Name, nil, nil)

	ui.Blank()
	ui.Success("Profile %q updated!", p.Name)
	ui.Detail("Name", newName)
	ui.Detail("Email", newEmail)
	if newEditor != "" {
		ui.Detail("Editor", newEditor)
	}
	if newSigningKey != "" {
		ui.Detail("Signing Key", newSigningKey)
	}
	if newGPGSign {
		ui.Detail("GPG Signing", "enabled")
	}
	printProfileProviderAccountSummary(p)

	return nil
}

func newProfileDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use: "delete <name>", Short: "Delete a profile", Aliases: []string{"rm"}, Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profileName := args[0]

			// Load profile before deletion to get SSH key path
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list", profileName)
			}

			// Check if trying to delete the active profile
			if isActiveProfile(profileName) {
				ui.Warning("Profile %q is currently active.", profileName)
				ui.Print("  Switch to another profile first: gcm use <other-profile>")
				ui.Blank()
				if !yes {
					confirm, err := ui.AskConfirm("Delete the active profile anyway?", false)
					if err != nil || !confirm {
						ui.Info("Cancelled")
						return nil
					}
				}
			} else if !yes {
				confirm, err := ui.AskConfirm(fmt.Sprintf("Delete profile %q?", profileName), false)
				if err != nil || !confirm {
					ui.Info("Cancelled")
					return nil
				}
			}

			// Deactivate git config before deleting (must happen while profile still exists)
			ctr.ProfileSwitcher.Deactivate(profileName)

			if err := ctr.ProfileManager.Delete(profileName, true); err != nil {
				ctr.AuditLogger.Log(audit.ActionProfileDelete, profileName, nil, err)
				return fmt.Errorf("could not delete profile %q\n\n  Make sure the profile exists: gcm profile list", profileName)
			}
			ctr.AuditLogger.Log(audit.ActionProfileDelete, profileName, nil, nil)
			ui.Success("Profile %q deleted", profileName)

			// Read SSH public key before deleting files (needed for GitHub deletion later)
			var sshPubKey string
			if p.SSH != nil && p.SSH.KeyPath != "" {
				sshPubKey, _ = ctr.SSHManager.GetPublicKey(p.SSH.KeyPath)
			}

			// Clean up provider keys before deleting local key material.
			var sshDeletedFromProviders, gpgDeletedFromProviders []string
			ctx := context.Background()
			for _, def := range providerDefinitionsWithCapability(providerpkg.CapabilityCredentialHelper) {
				token, tokenErr := loadProviderToken(profileName, def, p)
				if tokenErr != nil || token.AccessToken == "" {
					continue
				}
				if sshPubKey != "" && def.Capabilities.Has(providerpkg.CapabilitySSHKeys) {
					deleted, delErr := deleteProviderSSHKey(ctx, def, token, sshPubKey)
					if delErr != nil {
						ui.Warning("Could not delete SSH key from %s: %v", def.DisplayName, delErr)
					} else if deleted {
						sshDeletedFromProviders = append(sshDeletedFromProviders, def.DisplayName)
					}
				}

				if p.GPG != nil && p.GPG.KeyID != "" && def.Capabilities.Has(providerpkg.CapabilityGPGKeys) {
					deleted, delErr := deleteProviderGPGKey(ctx, def, token, p.GPG.KeyID)
					if delErr != nil {
						ui.Warning("Could not delete GPG key from %s: %v", def.DisplayName, delErr)
					} else if deleted {
						gpgDeletedFromProviders = append(gpgDeletedFromProviders, def.DisplayName)
					}
				}

				if def.ID == providerpkg.GitHubID {
					removedToken := false
					if delErr := deleteProviderToken(profileName, def, p); delErr == nil {
						removedToken = true
					}
					if delErr := ctr.GitHubClient.DeleteToken(profileName); delErr == nil {
						removedToken = true
					}
					if removedToken {
						ui.Success("%s token removed", def.DisplayName)
					}
				} else if delErr := deleteProviderToken(profileName, def, p); delErr == nil {
					ui.Success("%s token removed", def.DisplayName)
				}
			}

			// Clean up associated SSH key files
			if p.SSH != nil && p.SSH.KeyPath != "" {
				privKey := p.SSH.KeyPath
				pubKey := privKey + ".pub"
				removedAny := false
				if err := os.Remove(privKey); err == nil {
					removedAny = true
				}
				if err := os.Remove(pubKey); err == nil {
					removedAny = true
				}
				if removedAny || len(sshDeletedFromProviders) > 0 {
					if len(sshDeletedFromProviders) > 0 {
						ui.Success("SSH key removed (local + %s)", strings.Join(sshDeletedFromProviders, ", "))
					} else {
						ui.Success("SSH key removed (local)")
					}
					ui.Detail("Key", privKey)
				}
				ctr.SSHManager.RemoveFromAgent(privKey)
			}

			// Clean up associated GPG key
			if p.GPG != nil && p.GPG.KeyID != "" {
				gpgLocalDeleted := false
				if ctr.GPGManager.IsInstalled() {
					if err := ctr.GPGManager.Delete(p.GPG.KeyID); err != nil {
						ui.Warning("Could not delete GPG key from keyring: %v", err)
					} else {
						gpgLocalDeleted = true
					}
				}
				if gpgLocalDeleted || len(gpgDeletedFromProviders) > 0 {
					if len(gpgDeletedFromProviders) > 0 {
						ui.Success("GPG key removed (keyring + %s)", strings.Join(gpgDeletedFromProviders, ", "))
					} else {
						ui.Success("GPG key removed (keyring)")
					}
					ui.Detail("Key ID", p.GPG.KeyID)
				}
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newProfileExportCmd() *cobra.Command {
	return &cobra.Command{
		Use: "export <name>", Short: "Export profile (YAML)", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := ctr.ProfileManager.Export(args[0])
			if err != nil {
				ui.Error("profile %q not found", args[0])
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[0])
			}
			fmt.Fprint(os.Stdout, string(data))
			return nil
		},
	}
}

func newProfileImportCmd() *cobra.Command {
	return &cobra.Command{
		Use: "import <file>", Short: "Import profile from file", Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				ui.Error("could not read file: %s", args[0])
				ui.Blank()
				ui.Print("  Make sure the file path is correct.")
				ui.Print("  To export a profile: gcm profile export <name> > file.yaml")
				return fmt.Errorf("could not read file %q: %w", args[0], err)
			}
			p, err := ctr.ProfileManager.Import(data)
			if err != nil {
				return err
			}
			ui.Success("Profile %q imported", p.Name)
			return nil
		},
	}
}

func newProfileDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use: "diff <profile1> <profile2>", Short: "Compare two profiles", Args: requireArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			p1, err := ctr.ProfileManager.Get(args[0])
			if err != nil {
				ui.Error("profile %q not found", args[0])
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[0])
			}
			p2, err := ctr.ProfileManager.Get(args[1])
			if err != nil {
				ui.Error("profile %q not found", args[1])
				ui.Blank()
				ui.Print("  To see available profiles: gcm profile list")
				return profileNotFoundError(args[1])
			}
			ui.Header("Comparing: %s vs %s", args[0], args[1])
			diffField("Name", p1.Git.User.Name, p2.Git.User.Name, args[0], args[1])
			diffField("Email", p1.Git.User.Email, p2.Git.User.Email, args[0], args[1])
			sshP1, sshP2 := "", ""
			if p1.SSH != nil {
				sshP1 = p1.SSH.KeyPath
			}
			if p2.SSH != nil {
				sshP2 = p2.SSH.KeyPath
			}
			diffField("SSH Key", sshP1, sshP2, args[0], args[1])
			return nil
		},
	}
}

func diffField(label, v1, v2, n1, n2 string) {
	if v1 != v2 {
		ui.Print("\n%s:", label)
		ui.Detail(n1, v1)
		ui.Detail(n2, v2)
	}
}

func promptProviderAccountUsernames(p *profile.Profile) error {
	defs := providerDefinitionsWithCapability(providerpkg.CapabilityCredentialHelper)
	if len(defs) == 0 {
		ui.Info("No providers are configured yet.")
		return nil
	}

	options := []string{"Skip provider account"}
	byOption := make(map[string]providerpkg.Definition, len(defs))
	for _, def := range defs {
		option := providerOption(def)
		options = append(options, option)
		byOption[option] = def
	}

	currentLabel := "none"
	if currentDef, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper); ok {
		currentLabel = currentDef.DisplayName
	}
	choice, err := ui.AskSelect(fmt.Sprintf("Provider for this profile (current: %s):", currentLabel), options)
	if err != nil {
		return err
	}
	if choice == "Skip provider account" {
		oldState := cloneProfileProviderState(p)
		cleanupDefs := providerDefinitionsToClean(oldState, "")
		if len(cleanupDefs) > 0 {
			ui.Warning("Removing provider from profile %q: %s", p.Name, providerNames(cleanupDefs))
			ui.Print("  GCM will remove stored tokens, cached credentials, and uploaded keys when possible.")
			confirm, confirmErr := ui.AskConfirm("Continue and remove provider data?", false)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirm {
				ui.Info("Provider removal cancelled")
				return nil
			}
			cleanupProviderData(context.Background(), p.Name, oldState, cleanupDefs)
		}
		clearAllProfileProviderAccounts(p)
		if migrated, migErr := migrateProfileSSHKeyPathToProvider(p.Name, p); migErr != nil {
			ui.Warning("Could not rename SSH key after provider removal: %v", migErr)
		} else if migrated {
			ui.Detail("SSH Key Renamed", p.SSH.KeyPath)
		}
		return nil
	}

	def := byOption[choice]
	account := providerAccountForProfile(p, def.ID)
	prompt := fmt.Sprintf("%s username (leave empty if unknown):", def.DisplayName)
	if account.Username != "" {
		prompt = fmt.Sprintf("%s username [%s]:", def.DisplayName, account.Username)
	}
	username, err := ui.AskString(prompt, account.Username)
	if err != nil {
		return err
	}
	if ok, err := applyProfileProviderTransition(context.Background(), p.Name, p, def, username, account.AuthMethod, true, nil); err != nil {
		return err
	} else if !ok {
		ui.Info("Provider change cancelled")
	}
	return nil
}

func printProfileProviderAccounts(p *profile.Profile) {
	if profileHasMultipleProviders(p) {
		ui.SubHeader("Provider Account")
		ui.Warning("Profile has multiple provider accounts. Run: gcm profile edit %s -i", p.Name)
		return
	}
	def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper)
	if !ok {
		return
	}
	account := providerAccountForProfile(p, def.ID)
	ui.SubHeader("Provider Account")
	ui.Detail("Provider", def.DisplayName)
	if account.Username != "" {
		ui.Detail("Username", account.Username)
	}
}

func printProfileProviderAccountSummary(p *profile.Profile) {
	if profileHasMultipleProviders(p) {
		ui.Detail("Provider", ui.Yellow("multiple"))
		return
	}
	def, ok := profileProviderDefinition(p, providerpkg.CapabilityCredentialHelper)
	if !ok {
		return
	}
	account := providerAccountForProfile(p, def.ID)
	providerText := def.DisplayName
	if account.Username != "" {
		providerText = fmt.Sprintf("%s (%s)", def.DisplayName, account.Username)
	}
	ui.Detail("Provider", providerText)
}
