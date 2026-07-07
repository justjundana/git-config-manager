package cli

import (
	"context"
	"fmt"
	"strings"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_gpg "github.com/justjundana/git-config-manager/internal/gpg"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_shell "github.com/justjundana/git-config-manager/internal/shell"
	_ssh "github.com/justjundana/git-config-manager/internal/ssh"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
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
	_ui.Header("%s Welcome to GCM — Let's get you set up!", _ui.IconRocket)
	_ui.Blank()
	_ui.Print("This wizard will guide you through the complete setup.")
	_ui.Print("It takes about 2 minutes. You can skip any step.")
	_ui.Blank()

	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 1: Shell Integration
	// ═══════════════════════════════════════════════
	_ui.Header("Step 1/6: Shell Integration")
	_ui.Print("Shell hooks enable auto-switching when you cd into projects.")
	_ui.Blank()

	shellType := ctr.ShellManager.DetectShell()
	if shellType == _shell.ShellUnknown {
		_ui.Warning("Could not detect your shell")
		_ui.Info("Run %s after setting SHELL environment variable", _ui.Cyan("gcm init"))
	} else if installed, configFile := ctr.ShellManager.IsInstalled(shellType); installed {
		_ui.Success("Shell integration active for %s", string(shellType))
		_ui.Detail("Config", configFile)
	} else {
		_ui.Info("Shell integration not yet installed")
		_ui.Print("  Run %s to enable auto-switching and prompt integration", _ui.Cyan("gcm init"))
	}

	// Register GCM as credential helper (always — this protects against
	// external credential store changes like VS Code logout).
	if !IsCredentialHelperConfigured() {
		if err := RegisterCredentialHelper(); err != nil {
			_ui.Warning("Could not register credential helper: %v", err)
		} else {
			_ui.Success("Git credential helper registered for configured provider hosts")
		}
	}

	_ui.Blank()
	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 2: Create Profile
	// ═══════════════════════════════════════════════
	_ui.Header("Step 2/6: Create Your First Profile")
	_ui.Print("A profile holds your Git identity (name, email, keys).")
	_ui.Print("Most people have 2: %s and %s", _ui.Cyan("work"), _ui.Cyan("personal"))
	_ui.Blank()

	profileName, err := _ui.AskString("Profile name:", "work")
	if err != nil {
		return err
	}

	// Check if it already exists
	existing, _ := ctr.ProfileManager.Get(profileName)
	if existing != nil {
		_ui.Success("Profile %q already exists — using it", profileName)
	} else {
		fullName, nameErr := _ui.AskString("Your full name:", "")
		if nameErr != nil {
			return nameErr
		}
		email, emailErr := _ui.AskString("Your email:", "")
		if emailErr != nil {
			return emailErr
		}

		p := &_profile.Profile{
			Name: profileName,
			Git: _profile.GitConfig{
				User: _profile.GitUser{Name: fullName, Email: email},
			},
		}

		if createErr := ctr.ProfileManager.Create(p); createErr != nil {
			_ui.Error("Could not create profile: %v", createErr)
			return createErr
		}
		ctr.AuditLogger.Log(_audit.ActionProfileCreate, profileName, nil, nil)
		_ui.Success("Profile %q created!", profileName)
	}

	_ui.Blank()
	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 3: Provider Authentication
	// ═══════════════════════════════════════════════
	_ui.Header("Step 3/6: Provider Authentication")
	_ui.Print("Connecting a provider lets GCM manage git credentials automatically.")
	_ui.Blank()

	if err := runSetupProviderAuthentication(ctx, profileName); err != nil {
		return err
	}

	_ui.Blank()
	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 4: SSH Key
	// ═══════════════════════════════════════════════
	_ui.Header("Step 4/6: SSH Key")
	_ui.Print("SSH keys let you push/pull without passwords.")
	_ui.Blank()

	genSSH, err := _ui.AskConfirm("Generate an SSH key for this profile?", true)
	if err != nil {
		return err
	}

	if genSSH {
		p, _ := ctr.ProfileManager.Get(profileName)
		keyProfileName := sshKeyProfileName(profileName, p)
		sshHandled := false
		if keyInfo, adopted, adoptErr := adoptExistingSSHKeyForProfile(profileName, p, []string{"ed25519"}); adoptErr != nil {
			_ui.Warning("%v", adoptErr)
		} else if adopted {
			_ui.Info("Existing SSH key found and linked to profile %q", profileName)
			_ui.Detail("Path", keyInfo.Path)
			_ui.Detail("Fingerprint", keyInfo.Fingerprint)
			_ui.Blank()
			_ui.Print("Public key (add to GitHub/GitLab → user SSH key settings):")
			_ui.Print("  %s", _ui.Dim(keyInfo.PublicKey))
			sshHandled = true
		}

		if !sshHandled {
			sp := _ui.NewSpinner("Generating ed25519 SSH key...")
			sp.Start()
			keyInfo, genErr := ctr.SSHManager.Generate(_ssh.GenerateOptions{
				Profile: keyProfileName,
				KeyType: "ed25519",
			})
			if genErr != nil {
				sp.StopError("SSH key generation failed")
				_ui.Warning("%v", genErr)
			} else {
				sp.Stop("SSH key generated!")
				_ui.Detail("Path", keyInfo.Path)
				_ui.Detail("Fingerprint", keyInfo.Fingerprint)
				ctr.AuditLogger.Log(_audit.ActionSSHGenerate, profileName,
					map[string]string{"type": keyInfo.Type, "path": keyInfo.Path}, nil)

				// Update profile with SSH info
				if p != nil {
					p.SSH = &_profile.SSHConfig{
						KeyPath:     keyInfo.Path,
						KeyType:     _profile.KeyType(keyInfo.Type),
						Fingerprint: keyInfo.Fingerprint,
					}
					_ = ctr.ProfileManager.Update(p)
				}

				_ui.Blank()
				_ui.Print("Public key (add to GitHub/GitLab → user SSH key settings):")
				_ui.Print("  %s", _ui.Dim(keyInfo.PublicKey))
			}
		}
	} else {
		_ui.Info("Skipped — you can run %s later", _ui.Cyan(fmt.Sprintf("gcm ssh generate %s", profileName)))
	}

	_ui.Blank()
	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 5: GPG Signing (optional)
	// ═══════════════════════════════════════════════
	_ui.Header("Step 5/6: Commit Signing (Optional)")
	_ui.Print("GPG signing proves commits came from you (shows 'Verified' badge).")
	_ui.Blank()

	enableGPG, err := _ui.AskConfirm("Enable commit signing?", true)
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

		sp := _ui.NewSpinner("Generating GPG key...")
		sp.Start()
		keyInfo, genErr := ctr.GPGManager.Generate(_gpg.GenerateOptions{
			Name: name, Email: email,
		})
		if genErr != nil {
			sp.StopError("GPG key generation failed")
			_ui.Warning("%v", genErr)
		} else {
			sp.Stop("GPG key generated!")
			_ui.Detail("Key ID", keyInfo.KeyID)

			if p != nil {
				p.GPG = &_profile.GPGConfig{KeyID: keyInfo.KeyID}
				p.Git.Commit.GPGSign = _profile.BoolPtr(true)
				p.Git.User.SigningKey = keyInfo.KeyID
				_ = ctr.ProfileManager.Update(p)
			}
		}
	} else {
		_ui.Info("Skipped — you can enable this later with %s", _ui.Cyan("gcm gpg generate "+profileName))
	}

	// After key generation, offer to upload SSH/GPG keys when provider auth exists.
	setupUploadKeys(ctx, profileName)

	_ui.Blank()
	_ui.Divider()

	// ═══════════════════════════════════════════════
	// Step 6: Activate
	// ═══════════════════════════════════════════════
	_ui.Header("Step 6/6: Activate Profile")
	_ui.Blank()

	// If this is the only profile, activate automatically — no need to ask
	allProfiles, _ := ctr.ProfileManager.List()
	activate := true
	if len(allProfiles) > 1 {
		activate, err = _ui.AskConfirm(fmt.Sprintf("Activate profile %q now?", profileName), true)
		if err != nil {
			return err
		}
	}

	if activate {
		// If not yet set as global default, activate globally first
		if ctr.Config.DefaultProfile == "" {
			if actErr := ctr.ProfileSwitcher.Activate(profileName, _profile.ScopeGlobal); actErr != nil {
				_ui.Warning("Could not activate globally: %v", actErr)
			} else {
				_ui.Success("Profile %q set as global default", profileName)
			}
		}

		// Activate session for shell prompt indicator
		if actErr := ctr.ProfileSwitcher.Activate(profileName, _profile.ScopeSession); actErr != nil {
			// Fallback to local scope
			if actErr2 := ctr.ProfileSwitcher.Activate(profileName, _profile.ScopeLocal); actErr2 != nil {
				_ui.Warning("Could not activate session: %v", actErr2)
			} else {
				_ui.Success("Profile %q activated (local)", profileName)
			}
		} else {
			_ui.Success("Profile %q activated (session)", profileName)
		}
		ctr.AuditLogger.Log(_audit.ActionProfileActivate, profileName,
			map[string]string{"scope": "global"}, nil)
	}

	// ═══════════════════════════════════════════════
	// Done!
	// ═══════════════════════════════════════════════
	_ui.Blank()
	_ui.Divider()
	_ui.Header("%s You're all set!", _ui.IconCheck)
	_ui.Blank()
	_ui.Print("Your GCM setup is complete. Here's what you can do now:")
	_ui.Blank()
	_ui.NextSteps([]string{
		fmt.Sprintf("Check status anytime: %s", _ui.Cyan("gcm status")),
		fmt.Sprintf("Create another profile: %s", _ui.Cyan("gcm profile create <name> -i")),
		fmt.Sprintf("Switch profiles: %s", _ui.Cyan("gcm use <profile>")),
		fmt.Sprintf("View all commands: %s", _ui.Cyan("gcm --help")),
	})
	_ui.Blank()

	return nil
}

func runSetupProviderAuthentication(ctx context.Context, profileName string) error {
	defs := providerDefinitionsWithCapability(_provider.CapabilityPATAuth)
	if len(defs) == 0 {
		_ui.Info("No authentication providers are configured yet.")
		return nil
	}

	authenticate, err := _ui.AskConfirm("Authenticate with a Git provider now?", true)
	if err != nil {
		return err
	}
	if !authenticate {
		_ui.Info("Skipped — you can run gcm connect %s later", profileName)
		return nil
	}

	options := make([]string, 0, len(defs))
	byOption := make(map[string]_provider.Definition, len(defs))
	for _, def := range defs {
		option := providerOption(def)
		options = append(options, option)
		byOption[option] = def
	}
	options = append([]string{"Skip provider authentication"}, options...)
	selected, err := _ui.AskSelect("Provider for this profile:", options)
	if err != nil {
		return err
	}
	if selected == "Skip provider authentication" {
		_ui.Info("Skipped provider authentication")
		return nil
	}

	def := byOption[selected]
	switch def.ID {
	case _provider.GitHubID:
		if err := runSetupGitHubAuthentication(ctx, profileName, def); err != nil {
			return err
		}
	case _provider.GitLabID:
		if err := runSetupGitLabAuthentication(ctx, profileName, def); err != nil {
			return err
		}
	default:
		_ui.Warning("Provider %s is configured but not implemented yet", def.DisplayName)
	}

	return nil
}

func runSetupGitHubAuthentication(ctx context.Context, profileName string, def _provider.Definition) error {
	_ui.SubHeader("GitHub Authentication")
	method, err := _ui.AskSelect("Authentication method:", []string{
		"Personal Access Token (paste a token)",
		"OAuth Device Flow (browser-based)",
	})
	if err != nil {
		return err
	}

	if method == "Personal Access Token (paste a token)" {
		_ui.Blank()
		_ui.Print("Get a token at: %s", _ui.Cyan(providerPATURL(def)))
		if len(def.Scopes) > 0 {
			_ui.Print("Recommended scopes: %s", strings.Join(def.Scopes, ", "))
		}
		_ui.Blank()

		token, tokenErr := _ui.AskPassword("Paste your GitHub token")
		if tokenErr != nil {
			return tokenErr
		}
		if token == "" {
			_ui.Info("Skipped GitHub authentication")
			return nil
		}

		username, _, verifyErr := verifyProviderPAT(ctx, def, token)
		if verifyErr != nil {
			_ui.Error("GitHub token is invalid: %v", verifyErr)
			_ui.Print("  %s You can try again later: %s", _ui.IconArrow, _ui.Cyan(fmt.Sprintf("gcm connect %s --provider github", profileName)))
			return verifyErr
		}
		p, _ := ctr.ProfileManager.Get(profileName)
		if p != nil {
			tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodPAT, TokenType: "pat"}
			ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, username, _provider.AuthMethodPAT, true, func() error {
				return saveProviderToken(profileName, def, p, tokenSet)
			})
			if transitionErr != nil {
				_ui.Error("Could not update provider: %v", transitionErr)
				return transitionErr
			}
			if !ok {
				_ui.Info("Provider change cancelled")
				return nil
			}
			_ = ctr.GitHubClient.SaveToken(profileName, token)
			_ = ctr.ProfileManager.Update(p)
		}
		_ui.Success("Authenticated with GitHub as @%s", username)
		activateAsGlobalIfFirst(profileName)
		return nil
	}

	_ui.Blank()
	dcr, flowErr := ctr.GitHubClient.InitiateDeviceFlow()
	if flowErr != nil {
		_ui.Error("Could not start GitHub device flow: %v", flowErr)
		_ui.Print("  %s Try PAT instead: %s", _ui.IconArrow, _ui.Cyan(fmt.Sprintf("gcm connect %s --provider github", profileName)))
		return flowErr
	}
	_ui.Print("Open this URL in your browser:")
	_ui.Print("  %s", _ui.Cyan(dcr.VerificationURI))
	_ui.Blank()
	_ui.Print("Enter this code: %s", _ui.Bold(dcr.UserCode))
	_ui.Blank()

	sp := _ui.NewSpinner("Waiting for GitHub authorization...")
	sp.Start()
	token, pollErr := ctr.GitHubClient.PollForToken(ctx, dcr.DeviceCode, dcr.Interval)
	if pollErr != nil {
		sp.StopError("GitHub authorization failed")
		_ui.Error("%v", pollErr)
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
		tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodOAuthDevice, TokenType: "bearer"}
		ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, user.Login, _provider.AuthMethodOAuthDevice, true, func() error {
			return saveProviderToken(profileName, def, p, tokenSet)
		})
		if transitionErr != nil {
			_ui.Error("Could not update provider: %v", transitionErr)
			return transitionErr
		}
		if !ok {
			_ui.Info("Provider change cancelled")
			return nil
		}
		_ = ctr.GitHubClient.SaveToken(profileName, token)
		_ = ctr.ProfileManager.Update(p)
	}
	_ui.Success("Authenticated with GitHub as @%s", login)
	activateAsGlobalIfFirst(profileName)
	return nil
}

func runSetupGitLabAuthentication(ctx context.Context, profileName string, def _provider.Definition) error {
	_ui.SubHeader("GitLab Authentication")
	_ui.Print("Get a token at: %s", _ui.Cyan(strings.TrimRight(def.WebURL, "/")+"/-/user_settings/personal_access_tokens"))
	_ui.Print("Recommended scopes: api, read_user, read_repository, write_repository")
	_ui.Blank()

	token, tokenErr := _ui.AskPassword("Paste your GitLab token")
	if tokenErr != nil {
		return tokenErr
	}
	if token == "" {
		_ui.Info("Skipped GitLab authentication")
		return nil
	}

	tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodPAT, TokenType: "pat"}
	username, _, verifyErr := verifyProviderPAT(ctx, def, token)
	if verifyErr != nil {
		_ui.Error("GitLab token is invalid: %v", verifyErr)
		_ui.Print("  %s You can try again later: %s", _ui.IconArrow, _ui.Cyan(fmt.Sprintf("gcm connect %s --provider gitlab", profileName)))
		return verifyErr
	}

	p, _ := ctr.ProfileManager.Get(profileName)
	if p != nil {
		ok, transitionErr := applyProfileProviderTransition(ctx, profileName, p, def, username, _provider.AuthMethodPAT, true, func() error {
			return saveProviderToken(profileName, def, p, tokenSet)
		})
		if transitionErr != nil {
			_ui.Error("Could not update provider: %v", transitionErr)
			return transitionErr
		}
		if !ok {
			_ui.Info("Provider change cancelled")
			return nil
		}
	}
	if p != nil {
		_ = ctr.ProfileManager.Update(p)
	}
	_ui.Success("Authenticated with GitLab as @%s", username)
	activateAsGlobalIfFirst(profileName)
	return nil
}
