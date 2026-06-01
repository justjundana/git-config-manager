package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/justjundana/git-config-manager/internal/audit"
	"github.com/justjundana/git-config-manager/internal/gpg"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/shell"
	"github.com/justjundana/git-config-manager/internal/ssh"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Guided first-time setup wizard",
		Long: `Interactive wizard that walks you through the complete GCM setup.

This command will guide you through:
  1. Shell integration (auto-switching, prompt)
  2. Creating your first profile (name, email)
  3. SSH key generation
  4. GPG signing (optional)
	5. Provider authentication
  6. Activating your profile

Perfect for first-time users. Run this once and you're fully set up.`,
		Aliases: []string{"quickstart"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd.Context())
		},
	}
}

func runSetup(ctx context.Context) error {
	ui.Header("%s Welcome to GCM — Let's get you set up!", ui.IconRocket)
	ui.Blank()
	ui.Print("This wizard will guide you through the complete setup.")
	ui.Print("It takes about 2 minutes. You can skip any step.")
	ui.Blank()

	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 1: Shell Integration
	// ═══════════════════════════════════════════════
	ui.Header("Step 1/6: Shell Integration")
	ui.Print("Shell hooks enable auto-switching when you cd into projects.")
	ui.Blank()

	shellType := ctr.ShellManager.DetectShell()
	if shellType == shell.ShellUnknown {
		ui.Warning("Could not detect your shell")
		ui.Info("Run %s after setting SHELL environment variable", ui.Cyan("gcm init"))
	} else if installed, configFile := ctr.ShellManager.IsInstalled(shellType); installed {
		ui.Success("Shell integration active for %s", string(shellType))
		ui.Detail("Config", configFile)
	} else {
		ui.Info("Shell integration not yet installed")
		ui.Print("  Run %s to enable auto-switching and prompt integration", ui.Cyan("gcm init"))
	}

	// Register GCM as credential helper (always — this protects against
	// external credential store changes like VS Code logout).
	if !IsCredentialHelperConfigured() {
		if err := RegisterCredentialHelper(); err != nil {
			ui.Warning("Could not register credential helper: %v", err)
		} else {
			ui.Success("Git credential helper registered for configured provider hosts")
		}
	}

	ui.Blank()
	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 2: Create Profile
	// ═══════════════════════════════════════════════
	ui.Header("Step 2/6: Create Your First Profile")
	ui.Print("A profile holds your Git identity (name, email, keys).")
	ui.Print("Most people have 2: %s and %s", ui.Cyan("work"), ui.Cyan("personal"))
	ui.Blank()

	profileName, err := ui.AskString("Profile name:", "work")
	if err != nil {
		return err
	}

	// Check if it already exists
	existing, _ := ctr.ProfileManager.Get(profileName)
	if existing != nil {
		ui.Success("Profile %q already exists — using it", profileName)
	} else {
		fullName, nameErr := ui.AskString("Your full name:", "")
		if nameErr != nil {
			return nameErr
		}
		email, emailErr := ui.AskString("Your email:", "")
		if emailErr != nil {
			return emailErr
		}

		p := &profile.Profile{
			Name: profileName,
			Git: profile.GitConfig{
				User: profile.GitUser{Name: fullName, Email: email},
			},
		}

		if createErr := ctr.ProfileManager.Create(p); createErr != nil {
			ui.Error("Could not create profile: %v", createErr)
			return createErr
		}
		ctr.AuditLogger.Log(audit.ActionProfileCreate, profileName, nil, nil)
		ui.Success("Profile %q created!", profileName)
	}

	ui.Blank()
	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 3: Provider Authentication
	// ═══════════════════════════════════════════════
	ui.Header("Step 3/6: Provider Authentication")
	ui.Print("Connecting a provider lets GCM manage git credentials automatically.")
	ui.Blank()

	if err := runSetupProviderAuthentication(ctx, profileName); err != nil {
		return err
	}

	ui.Blank()
	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 4: SSH Key
	// ═══════════════════════════════════════════════
	ui.Header("Step 4/6: SSH Key")
	ui.Print("SSH keys let you push/pull without passwords.")
	ui.Blank()

	genSSH, err := ui.AskConfirm("Generate an SSH key for this profile?", true)
	if err != nil {
		return err
	}

	if genSSH {
		p, _ := ctr.ProfileManager.Get(profileName)
		keyProfileName := sshKeyProfileName(profileName, p)
		sshHandled := false
		if keyInfo, adopted, adoptErr := adoptExistingSSHKeyForProfile(profileName, p, []string{"ed25519"}); adoptErr != nil {
			ui.Warning("%v", adoptErr)
		} else if adopted {
			ui.Info("Existing SSH key found and linked to profile %q", profileName)
			ui.Detail("Path", keyInfo.Path)
			ui.Detail("Fingerprint", keyInfo.Fingerprint)
			ui.Blank()
			ui.Print("Public key (add to GitHub/GitLab → user SSH key settings):")
			ui.Print("  %s", ui.Dim(keyInfo.PublicKey))
			sshHandled = true
		}

		if !sshHandled {
			sp := ui.NewSpinner("Generating ed25519 SSH key...")
			sp.Start()
			keyInfo, genErr := ctr.SSHManager.Generate(ssh.GenerateOptions{
				Profile: keyProfileName,
				KeyType: "ed25519",
			})
			if genErr != nil {
				sp.StopError("SSH key generation failed")
				ui.Warning("%v", genErr)
			} else {
				sp.Stop("SSH key generated!")
				ui.Detail("Path", keyInfo.Path)
				ui.Detail("Fingerprint", keyInfo.Fingerprint)
				ctr.AuditLogger.Log(audit.ActionSSHGenerate, profileName,
					map[string]string{"type": keyInfo.Type, "path": keyInfo.Path}, nil)

				// Update profile with SSH info
				if p != nil {
					p.SSH = &profile.SSHConfig{
						KeyPath:     keyInfo.Path,
						KeyType:     profile.KeyType(keyInfo.Type),
						Fingerprint: keyInfo.Fingerprint,
					}
					_ = ctr.ProfileManager.Update(p)
				}

				ui.Blank()
				ui.Print("Public key (add to GitHub/GitLab → user SSH key settings):")
				ui.Print("  %s", ui.Dim(keyInfo.PublicKey))
			}
		}
	} else {
		ui.Info("Skipped — you can run %s later", ui.Cyan(fmt.Sprintf("gcm ssh generate %s", profileName)))
	}

	ui.Blank()
	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 5: GPG Signing (optional)
	// ═══════════════════════════════════════════════
	ui.Header("Step 5/6: Commit Signing (Optional)")
	ui.Print("GPG signing proves commits came from you (shows 'Verified' badge).")
	ui.Blank()

	enableGPG, err := ui.AskConfirm("Enable commit signing?", true)
	if err != nil {
		return err
	}

	if enableGPG {
		p, _ := ctr.ProfileManager.Get(profileName)
		name := profileName
		email := ""
		if p != nil {
			name = p.Git.User.Name
			email = p.Git.User.Email
		}

		sp := ui.NewSpinner("Generating GPG key...")
		sp.Start()
		keyInfo, genErr := ctr.GPGManager.Generate(gpg.GenerateOptions{
			Name: name, Email: email,
		})
		if genErr != nil {
			sp.StopError("GPG key generation failed")
			ui.Warning("%v", genErr)
		} else {
			sp.Stop("GPG key generated!")
			ui.Detail("Key ID", keyInfo.KeyID)

			if p != nil {
				p.GPG = &profile.GPGConfig{KeyID: keyInfo.KeyID}
				p.Git.Commit.GPGSign = profile.BoolPtr(true)
				p.Git.User.SigningKey = keyInfo.KeyID
				_ = ctr.ProfileManager.Update(p)
			}
		}
	} else {
		ui.Info("Skipped — you can enable this later with %s", ui.Cyan("gcm gpg generate "+profileName))
	}

	// After key generation, offer to upload SSH/GPG keys when provider auth exists.
	setupUploadKeys(ctx, profileName)

	ui.Blank()
	ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 6: Activate
	// ═══════════════════════════════════════════════
	ui.Header("Step 6/6: Activate Profile")
	ui.Blank()

	// If this is the only profile, activate automatically — no need to ask
	allProfiles, _ := ctr.ProfileManager.List()
	activate := true
	if len(allProfiles) > 1 {
		activate, err = ui.AskConfirm(fmt.Sprintf("Activate profile %q now?", profileName), true)
		if err != nil {
			return err
		}
	}

	if activate {
		// If not yet set as global default, activate globally first
		if ctr.Config.DefaultProfile == "" {
			if actErr := ctr.ProfileSwitcher.Activate(profileName, profile.ScopeGlobal); actErr != nil {
				ui.Warning("Could not activate globally: %v", actErr)
			} else {
				ui.Success("Profile %q set as global default", profileName)
			}
		}

		// Activate session for shell prompt indicator
		if actErr := ctr.ProfileSwitcher.Activate(profileName, profile.ScopeSession); actErr != nil {
			// Fallback to local scope
			if actErr2 := ctr.ProfileSwitcher.Activate(profileName, profile.ScopeLocal); actErr2 != nil {
				ui.Warning("Could not activate session: %v", actErr2)
			} else {
				ui.Success("Profile %q activated (local)", profileName)
			}
		} else {
			ui.Success("Profile %q activated (session)", profileName)
		}
		ctr.AuditLogger.Log(audit.ActionProfileActivate, profileName,
			map[string]string{"scope": "global"}, nil)
	}

	// ═══════════════════════════════════════════════
	// Done!
	// ═══════════════════════════════════════════════
	ui.Blank()
	ui.Divider()
	ui.Header("%s You're all set!", ui.IconCheck)
	ui.Blank()
	ui.Print("Your GCM setup is complete. Here's what you can do now:")
	ui.Blank()
	ui.NextSteps([]string{
		fmt.Sprintf("Check status anytime: %s", ui.Cyan("gcm status")),
		fmt.Sprintf("Create another profile: %s", ui.Cyan("gcm profile create <name> -i")),
		fmt.Sprintf("Switch profiles: %s", ui.Cyan("gcm use <profile>")),
		fmt.Sprintf("View all commands: %s", ui.Cyan("gcm --help")),
	})
	ui.Blank()

	return nil
}

func runSetupProviderAuthentication(ctx context.Context, profileName string) error {
	defs := providerDefinitionsWithCapability(providerpkg.CapabilityPATAuth)
	if len(defs) == 0 {
		ui.Info("No authentication providers are configured yet.")
		return nil
	}

	authenticate, err := ui.AskConfirm("Authenticate with a Git provider now?", true)
	if err != nil {
		return err
	}
	if !authenticate {
		ui.Info("Skipped — you can run gcm connect %s later", profileName)
		return nil
	}

	options := make([]string, 0, len(defs))
	byOption := make(map[string]providerpkg.Definition, len(defs))
	for _, def := range defs {
		option := providerOption(def)
		options = append(options, option)
		byOption[option] = def
	}
	options = append([]string{"Skip provider authentication"}, options...)
	selected, err := ui.AskSelect("Provider for this profile:", options)
	if err != nil {
		return err
	}
	if selected == "Skip provider authentication" {
		ui.Info("Skipped provider authentication")
		return nil
	}

	def := byOption[selected]
	switch def.ID {
	case providerpkg.GitHubID:
		if err := runSetupGitHubAuthentication(ctx, profileName, def); err != nil {
			return err
		}
	case providerpkg.GitLabID:
		if err := runSetupGitLabAuthentication(ctx, profileName, def); err != nil {
			return err
		}
	default:
		ui.Warning("Provider %s is configured but not implemented yet", def.DisplayName)
	}

	return nil
}

func runSetupGitHubAuthentication(ctx context.Context, profileName string, def providerpkg.Definition) error {
	ui.SubHeader("GitHub Authentication")
	method, err := ui.AskSelect("Authentication method:", []string{
		"Personal Access Token (paste a token)",
		"OAuth Device Flow (browser-based)",
	})
	if err != nil {
		return err
	}

	if method == "Personal Access Token (paste a token)" {
		ui.Blank()
		ui.Print("Get a token at: %s", ui.Cyan(providerPATURL(def)))
		if len(def.Scopes) > 0 {
			ui.Print("Recommended scopes: %s", strings.Join(def.Scopes, ", "))
		}
		ui.Blank()

		token, tokenErr := ui.AskPassword("Paste your GitHub token")
		if tokenErr != nil {
			return tokenErr
		}
		if token == "" {
			ui.Info("Skipped GitHub authentication")
			return nil
		}

		username, _, verifyErr := verifyProviderPAT(ctx, def, token)
		if verifyErr != nil {
			ui.Error("GitHub token is invalid: %v", verifyErr)
			ui.Print("  %s You can try again later: %s", ui.IconArrow, ui.Cyan(fmt.Sprintf("gcm connect %s --provider github", profileName)))
			return verifyErr
		}
		p, _ := ctr.ProfileManager.Get(profileName)
		if p != nil {
			tokenSet := providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"}
			ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, username, providerpkg.AuthMethodPAT, true, func() error {
				return saveProviderToken(profileName, def, p, tokenSet)
			})
			if transitionErr != nil {
				ui.Error("Could not update provider: %v", transitionErr)
				return transitionErr
			}
			if !ok {
				ui.Info("Provider change cancelled")
				return nil
			}
			_ = ctr.GitHubClient.SaveToken(profileName, token)
			_ = ctr.ProfileManager.Update(p)
		}
		ui.Success("Authenticated with GitHub as @%s", username)
		activateAsGlobalIfFirst(profileName)
		return nil
	}

	ui.Blank()
	dcr, flowErr := ctr.GitHubClient.InitiateDeviceFlow()
	if flowErr != nil {
		ui.Error("Could not start GitHub device flow: %v", flowErr)
		ui.Print("  %s Try PAT instead: %s", ui.IconArrow, ui.Cyan(fmt.Sprintf("gcm connect %s --provider github", profileName)))
		return flowErr
	}
	ui.Print("Open this URL in your browser:")
	ui.Print("  %s", ui.Cyan(dcr.VerificationURI))
	ui.Blank()
	ui.Print("Enter this code: %s", ui.Bold(dcr.UserCode))
	ui.Blank()

	sp := ui.NewSpinner("Waiting for GitHub authorization...")
	sp.Start()
	token, pollErr := ctr.GitHubClient.PollForToken(ctx, dcr.DeviceCode, dcr.Interval)
	if pollErr != nil {
		sp.StopError("GitHub authorization failed")
		ui.Error("%v", pollErr)
		return pollErr
	}
	sp.Stop("GitHub authorized!")

	ctr.GitHubClient.SetToken(token)
	user, _ := ctr.GitHubClient.VerifyToken(ctx)
	login := profileName
	if user != nil {
		login = user.Login
	}
	if p, _ := ctr.ProfileManager.Get(profileName); p != nil && user != nil {
		tokenSet := providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodOAuthDevice, TokenType: "bearer"}
		ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, user.Login, providerpkg.AuthMethodOAuthDevice, true, func() error {
			return saveProviderToken(profileName, def, p, tokenSet)
		})
		if transitionErr != nil {
			ui.Error("Could not update provider: %v", transitionErr)
			return transitionErr
		}
		if !ok {
			ui.Info("Provider change cancelled")
			return nil
		}
		_ = ctr.GitHubClient.SaveToken(profileName, token)
		_ = ctr.ProfileManager.Update(p)
	}
	ui.Success("Authenticated with GitHub as @%s", login)
	activateAsGlobalIfFirst(profileName)
	return nil
}

func runSetupGitLabAuthentication(ctx context.Context, profileName string, def providerpkg.Definition) error {
	ui.SubHeader("GitLab Authentication")
	ui.Print("Get a token at: %s", ui.Cyan(strings.TrimRight(def.WebURL, "/")+"/-/user_settings/personal_access_tokens"))
	ui.Print("Recommended scopes: api, read_user, read_repository, write_repository")
	ui.Blank()

	token, tokenErr := ui.AskPassword("Paste your GitLab token")
	if tokenErr != nil {
		return tokenErr
	}
	if token == "" {
		ui.Info("Skipped GitLab authentication")
		return nil
	}

	tokenSet := providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"}
	username, _, verifyErr := verifyProviderPAT(ctx, def, token)
	if verifyErr != nil {
		ui.Error("GitLab token is invalid: %v", verifyErr)
		ui.Print("  %s You can try again later: %s", ui.IconArrow, ui.Cyan(fmt.Sprintf("gcm connect %s --provider gitlab", profileName)))
		return verifyErr
	}

	p, _ := ctr.ProfileManager.Get(profileName)
	if p != nil {
		ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, username, providerpkg.AuthMethodPAT, true, func() error {
			return saveProviderToken(profileName, def, p, tokenSet)
		})
		if transitionErr != nil {
			ui.Error("Could not update provider: %v", transitionErr)
			return transitionErr
		}
		if !ok {
			ui.Info("Provider change cancelled")
			return nil
		}
	}
	if p != nil {
		_ = ctr.ProfileManager.Update(p)
	}
	ui.Success("Authenticated with GitLab as @%s", username)
	activateAsGlobalIfFirst(profileName)
	return nil
}
