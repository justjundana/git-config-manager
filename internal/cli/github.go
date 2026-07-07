package cli

import (
	"fmt"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

// gitServer returns the git host URL for credential operations.
func gitServer() string {
	if ctr != nil && ctr.ProviderRegistry != nil {
		if def, ok := ctr.ProviderRegistry.Get(_provider.GitHubID); ok {
			return def.CredentialServer()
		}
	}
	server := ctr.Config.GitHub.APIURL
	if server == "" || server == "https://api.github.com" {
		return "https://github.com"
	}
	return server
}

func githubProviderDefinition() (_provider.Definition, error) {
	def, ok := ctr.ProviderRegistry.Get(_provider.GitHubID)
	if !ok {
		return _provider.Definition{}, fmt.Errorf("GitHub provider is not configured")
	}
	return def, nil
}

// isActiveProfile returns true if the given profile name is the currently active one.
func isActiveProfile(name string) bool {
	current, _, err := ctr.ProfileSwitcher.Current()
	return err == nil && current == name
}

func newGitHubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "github",
		Short:   "Manage GitHub integration",
		Aliases: []string{"gh"},
	}

	cmd.AddCommand(newGitHubLoginCmd())
	cmd.AddCommand(newGitHubLoginOAuthCmd())
	cmd.AddCommand(newGitHubLoginGHCmd())
	cmd.AddCommand(newGitHubLogoutCmd())
	cmd.AddCommand(newGitHubVerifyCmd())
	cmd.AddCommand(newGitHubUserCmd())
	cmd.AddCommand(newGitHubStatusCmd())

	return cmd
}

func newGitHubLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <profile>",
		Short: "Authenticate with a Personal Access Token (PAT)",
		Long: `Authenticate with GitHub using a Personal Access Token.

This is the primary login method. It works in all environments including
CI/CD, headless servers, and interactive terminals.

How to get a token:
  1. Go to https://github.com/settings/tokens
  2. Click "Generate new token (classic)"
  3. Select scopes: repo, admin:public_key, admin:gpg_key, and any others you need
  4. Copy the token and paste it below

Examples:
	gcm github login work-github                         (interactive, will prompt)
	echo "$GH_TOKEN" | gcm github login work-github      (piped from environment)`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			// Verify profile exists first
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list\n  To create a new profile:   gcm profile create %s -i", profileName, profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}

			var token string

			// Check if --stdin flag or pipe input
			stdinPiped := isStdinPiped()
			if stdinPiped {
				token, err = readStdinToken()
				if err != nil {
					return fmt.Errorf("could not read token from input\n\n  Make sure you're piping a valid token:\n  echo \"$GH_TOKEN\" | gcm github login %s", profileName)
				}
			} else {
				_ui.Header("%s GitHub Login for Profile: %s", _ui.IconKey, profileName)
				_ui.Blank()
				_ui.Print("You need a Personal Access Token (PAT) from GitHub.")
				_ui.Blank()
				_ui.Print("To create one:")
				_ui.Print("  1. Go to %s", _ui.Cyan("https://github.com/settings/tokens"))
				_ui.Print("  2. Click 'Generate new token (classic)'")
				_ui.Print("  3. Select scopes: repo, admin:public_key, admin:gpg_key")
				_ui.Print("  4. Copy and paste the token below")
				_ui.Blank()
				token, err = _ui.AskPassword("Enter token")
				if err != nil {
					return fmt.Errorf("could not read token input")
				}
			}

			if token == "" {
				return fmt.Errorf("token cannot be empty\n\n  Please provide a valid Personal Access Token.\n  Generate one at: https://github.com/settings/tokens")
			}

			// Verify the token works
			sp := _ui.NewSpinner("Verifying token with GitHub...")
			sp.Start()

			ctr.GitHubClient.SetToken(token)
			user, err := ctr.GitHubClient.GetUser(cmd.Context())
			if err != nil {
				sp.StopError("Token is not valid")
				_ui.Blank()
				_ui.Print("The token you provided was rejected by GitHub.")
				_ui.Print("Common causes:")
				_ui.Print("  • Token was copied incorrectly (missing characters)")
				_ui.Print("  • Token has been revoked or expired")
				_ui.Print("  • Token does not have the required scopes")
				_ui.Blank()
				_ui.Print("Generate a new token at: https://github.com/settings/tokens")
				_ui.Print("Required scopes: repo, admin:public_key, admin:gpg_key")
				return fmt.Errorf("token verification failed")
			}
			sp.Stop("Token verified!")

			tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodPAT, TokenType: "pat"}
			ok, transitionErr := applyProfileProviderTransition(cmd.Context(), profileName, p, def, user.Login, _provider.AuthMethodPAT, !stdinPiped, func() error {
				return saveProviderToken(profileName, def, p, tokenSet)
			})
			if transitionErr != nil {
				ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName, nil, transitionErr)
				return transitionErr
			}
			if !ok {
				_ui.Info("Provider change cancelled")
				return nil
			}
			_ = ctr.GitHubClient.SaveToken(profileName, token)

			ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName,
				map[string]string{"user": user.Login, "method": "pat"}, nil)
			_ui.Blank()
			if user.Name != "" {
				_ui.Success("Logged in as %s (%s)", _ui.Bold(user.Login), user.Name)
			} else {
				_ui.Success("Logged in as %s", _ui.Bold(user.Login))
			}

			// Only update git credentials if this is the active profile
			if isActiveProfile(profileName) {
				configureGitCredentialsForProvider(profileName, p, def, tokenSet)
				_ui.Print("  Git credentials updated — git push/pull will use this account.")
			} else {
				_ui.Blank()
				_ui.Print("  Note: This is not the active profile.")
				_ui.Print("  Git credentials will be updated when you switch to it:")
				_ui.Print("    gcm use %s", profileName)
			}

			_ = ctr.ProfileManager.Update(p)
			// Auto-activate globally if this is the first authenticated profile
			activateAsGlobalIfFirst(profileName)

			if !stdinPiped && p != nil {
				if def, ok := ctr.ProviderRegistry.Get(_provider.GitHubID); ok {
					setupUploadKeysForProvider(cmd.Context(), profileName, p, def)
				}
			}

			return nil
		},
	}
}

func newGitHubLogoutCmd() *cobra.Command {
	var clearGitCreds bool
	var forceLogout bool

	cmd := &cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove stored GitHub token for a profile",
		Long: `Remove the stored GitHub token for a profile.

This deletes the encrypted token from GCM's storage. By default, it also
clears the cached git credentials from your system (macOS Keychain, Windows
Credential Manager, or Linux secret-service). HTTPS Git operations may prompt
again; SSH remotes and profile SSH keys are not affected.

Use --clear-credentials=false if you only want to remove the token from GCM
without affecting git operations.

Examples:
	gcm github logout work-github
	gcm github logout work-github --clear-credentials=false`,
		Args: requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profileName := args[0]
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found", profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, p, def); err != nil {
				return err
			}

			// Guard: require confirmation when logging out a non-active profile
			if !isActiveProfile(profileName) && !forceLogout {
				_ui.Warning("Profile %q is not the active profile.", profileName)
				_ui.Blank()
				confirm, err := _ui.AskConfirm(fmt.Sprintf("Are you sure you want to remove the token for %q?", profileName), false)
				if err != nil || !confirm {
					_ui.Info("Cancelled.")
					return nil
				}
			}

			hadStoredToken := providerTokenPresent(profileName, def, p)
			providerDeleteErr := deleteProviderToken(profileName, def, p)
			legacyDeleteErr := ctr.GitHubClient.DeleteToken(profileName)
			if hadStoredToken && providerDeleteErr != nil && legacyDeleteErr != nil {
				ctr.AuditLogger.Log(_audit.ActionGitHubLogout, profileName, nil, providerDeleteErr)
				return fmt.Errorf("could not remove token for profile %q\n\n  The token file may not exist or cannot be accessed.\n  Check with: gcm github status", profileName)
			}

			ctr.AuditLogger.Log(_audit.ActionGitHubLogout, profileName, nil, nil)
			if hadStoredToken {
				_ui.Success("GitHub token removed for profile %q", profileName)
			} else {
				_ui.Info("No GitHub token was stored for profile %q.", profileName)
			}

			if clearGitCreds && isActiveProfile(profileName) {
				// Only clear git credentials if this is the currently active profile.
				// Clearing credentials for a non-active profile would break the active one.
				_ = ctr.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "")
				if err := ctr.GitHubClient.ClearGitCredentials(gitServer()); err != nil {
					_ui.Warning("Git credentials could not be cleared automatically.")
					_ui.Print("  You may need to clear them manually from your system's credential store.")
				} else {
					_ui.Success("HTTPS Git credentials and username pin cleared for %s.", def.CredentialServer())
					_ui.Print("  SSH remotes and profile SSH keys are unchanged, so git may still work over SSH.")
				}
			} else if clearGitCreds && !isActiveProfile(profileName) {
				_ui.Print("  Note: Git credentials were not cleared because %q is not the active profile.", profileName)
				_ui.Print("  The active profile's credentials remain intact.")
			}

			_ui.Blank()
			_ui.Print("To re-authenticate later: gcm github login %s", profileName)

			return nil
		},
	}

	cmd.Flags().BoolVar(&clearGitCreds, "clear-credentials", true,
		"Also clear cached git credentials from system credential store")
	cmd.Flags().BoolVarP(&forceLogout, "force", "f", false,
		"Skip confirmation when logging out a non-active profile")

	return cmd
}

func newGitHubVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <profile>",
		Short: "Verify that the stored GitHub token is still valid",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				_ui.Error("profile %q not found", profileName)
				_ui.Blank()
				_ui.Print("  To see available profiles: gcm profile list")
				_ui.Print("  To create a new profile:   gcm profile create %s -i", profileName)
				return profileNotFoundError(profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, p, def); err != nil {
				return err
			}

			token, err := loadProviderToken(profileName, def, p)
			if err != nil {
				_ui.Blank()
				_ui.Print("Profile %q is not authenticated with GitHub yet.", profileName)
				_ui.Blank()
				_ui.Print("To authenticate, use one of these commands:")
				_ui.Print("  gcm github login %s         (Personal Access Token, recommended)", profileName)
				_ui.Print("  gcm github login-oauth %s   (browser-based OAuth)", profileName)
				_ui.Print("  gcm github login-gh %s      (import from GitHub CLI)", profileName)
				return fmt.Errorf("profile %q is not authenticated", profileName)
			}
			ctr.GitHubClient.SetToken(token.AccessToken)
			user, err := ctr.GitHubClient.VerifyToken(cmd.Context())
			if err != nil {
				_ui.Blank()
				_ui.Print("The stored token for profile %q is no longer valid.", profileName)
				_ui.Print("This usually means the token was revoked or has expired.")
				_ui.Blank()
				_ui.Print("To fix, re-authenticate:")
				_ui.Print("  gcm github login %s", profileName)
				return fmt.Errorf("token expired or revoked for profile %q", profileName)
			}
			_ui.Success("Authenticated as %s", _ui.Bold(user.Login))
			return nil
		},
	}
}

func newGitHubUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "user <profile>",
		Short: "Show GitHub user information for a profile",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				_ui.Error("profile %q not found", profileName)
				_ui.Blank()
				_ui.Print("  To see available profiles: gcm profile list")
				_ui.Print("  To create a new profile:   gcm profile create %s -i", profileName)
				return profileNotFoundError(profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, p, def); err != nil {
				return err
			}

			token, err := loadProviderToken(profileName, def, p)
			if err != nil {
				_ui.Blank()
				_ui.Print("Profile %q is not authenticated with GitHub yet.", profileName)
				_ui.Blank()
				_ui.Print("To authenticate: gcm github login %s", profileName)
				return fmt.Errorf("profile %q is not authenticated", profileName)
			}
			ctr.GitHubClient.SetToken(token.AccessToken)
			user, err := ctr.GitHubClient.GetUser(cmd.Context())
			if err != nil {
				_ui.Blank()
				_ui.Print("Could not fetch your GitHub profile. The token may have expired.")
				_ui.Print("To re-authenticate: gcm github login %s", profileName)
				return fmt.Errorf("could not fetch GitHub user info for %q", profileName)
			}
			_ui.Header("GitHub User: %s", user.Login)
			_ui.Detail("Name", user.Name)
			_ui.Detail("Email", user.Email)
			_ui.Detail("Company", user.Company)
			_ui.Detail("Location", user.Location)
			_ui.Detail("Repos", fmt.Sprintf("%d", user.PublicRepos))
			_ui.Detail("URL", user.HTMLURL)
			return nil
		},
	}
}

func newGitHubLoginOAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login-oauth <profile>",
		Short: "Authenticate with GitHub using OAuth device flow (browser-based)",
		Long: `Authenticate with GitHub using the OAuth device flow.

This opens a browser where you authorize GCM to access your GitHub account.
After authorization, the token is encrypted and stored securely.

Requirements:
  • A valid OAuth App client_id must be configured in ~/.gcm/config.yaml
  • Internet connection to reach github.com

Examples:
	gcm github login-oauth work-github
  gcm github login-oauth personal`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			// Verify profile exists first
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list\n  To create a new profile:   gcm profile create %s -i", profileName, profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}

			_ui.Header("%s GitHub OAuth Login for Profile: %s", _ui.IconGlobe, profileName)
			_ui.Blank()

			sp := _ui.NewSpinner("Connecting to GitHub...")
			sp.Start()

			dcr, err := ctr.GitHubClient.InitiateDeviceFlow()
			if err != nil {
				sp.StopError("Could not connect to GitHub")
				_ui.Blank()
				_ui.Print("This usually means:")
				_ui.Print("  1. The OAuth App client_id in ~/.gcm/config.yaml is not valid")
				_ui.Print("  2. You don't have internet access to github.com")
				_ui.Blank()
				_ui.Print("To fix: update 'github.oauth.client_id' in ~/.gcm/config.yaml")
				_ui.Print("        with a valid GitHub OAuth App client ID.")
				_ui.Blank()
				_ui.Print("Alternative login methods (no OAuth App needed):")
				_ui.Print("  gcm github login %s      (use a Personal Access Token)", profileName)
				_ui.Print("  gcm github login-gh %s   (import from GitHub CLI)", profileName)
				return fmt.Errorf("could not start GitHub OAuth login")
			}

			sp.Stop("Connected!")
			_ui.Blank()
			_ui.Print("Step 1: Open this URL in your browser:")
			_ui.Print("        %s", _ui.Cyan(dcr.VerificationURI))
			_ui.Blank()
			_ui.Print("Step 2: Enter this code when prompted:")
			_ui.Print("        %s", _ui.Bold(dcr.UserCode))
			_ui.Blank()

			sp2 := _ui.NewSpinner("Waiting for you to authorize in the browser (up to 15 minutes)...")
			sp2.Start()

			token, err := ctr.GitHubClient.PollForToken(cmd.Context(), dcr.DeviceCode, dcr.Interval)
			if err != nil {
				sp2.StopError("Authorization was not completed")
				_ui.Blank()
				_ui.Print("Possible reasons:")
				_ui.Print("  • You didn't approve the request in the browser")
				_ui.Print("  • The code expired (15 minute time limit)")
				_ui.Print("  • You denied the request")
				_ui.Blank()
				_ui.Print("To try again: gcm github login-oauth %s", profileName)
				return fmt.Errorf("authorization not completed")
			}

			sp2.Stop("Authorization successful!")

			ctr.GitHubClient.SetToken(token)
			user, err := ctr.GitHubClient.GetUser(cmd.Context())
			if err != nil {
				ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName, nil, nil)
				_ui.Blank()
				_ui.Warning("Could not verify your GitHub username, so the token was not saved.")
				_ui.Print("  This is usually temporary. Verify later with: gcm github verify %s", profileName)
				return nil
			}

			tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodOAuthDevice, TokenType: "bearer"}
			ok, transitionErr := applyProfileProviderTransition(cmd.Context(), profileName, p, def, user.Login, _provider.AuthMethodOAuthDevice, true, func() error {
				return saveProviderToken(profileName, def, p, tokenSet)
			})
			if transitionErr != nil {
				ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName, nil, transitionErr)
				return transitionErr
			}
			if !ok {
				_ui.Info("Provider change cancelled")
				return nil
			}
			_ = ctr.GitHubClient.SaveToken(profileName, token)

			ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName,
				map[string]string{"user": user.Login, "method": "oauth"}, nil)
			_ui.Blank()
			if user.Name != "" {
				_ui.Success("Logged in as %s (%s)", _ui.Bold(user.Login), user.Name)
			} else {
				_ui.Success("Logged in as %s", _ui.Bold(user.Login))
			}
			_ui.Detail("GitHub Profile", user.HTMLURL)

			// Only update git credentials if this is the active profile
			if isActiveProfile(profileName) {
				configureGitCredentialsForProvider(profileName, p, def, tokenSet)
				_ui.Print("  Git credentials updated — git push/pull will use this account.")
			} else {
				_ui.Blank()
				_ui.Print("  Note: This is not the active profile.")
				_ui.Print("  Git credentials will be updated when you switch to it:")
				_ui.Print("    gcm use %s", profileName)
			}

			_ = ctr.ProfileManager.Update(p)

			// Auto-activate globally if this is the first authenticated profile
			activateAsGlobalIfFirst(profileName)

			setupUploadKeysForProvider(cmd.Context(), profileName, p, def)

			return nil
		},
	}
}

func newGitHubLoginGHCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login-gh <profile>",
		Short: "Import authentication from GitHub CLI (gh)",
		Long: `Import your existing GitHub CLI authentication into GCM.

This reads the token from 'gh auth token' and stores it in GCM.
You must have the GitHub CLI installed and already logged in.

If you don't have the GitHub CLI:
  • Install it from https://cli.github.com
  • Then run: gh auth login

Examples:
	gcm github login-gh work-github`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			// Verify profile exists first
			p, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list\n  To create a new profile:   gcm profile create %s -i", profileName, profileName)
			}
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}

			sp := _ui.NewSpinner("Reading token from GitHub CLI...")
			sp.Start()

			token, err := ctr.GitHubClient.ImportFromGHCLI()
			if err != nil {
				sp.StopError("Could not get token from GitHub CLI")
				_ui.Blank()
				_ui.Print("GCM tried to run 'gh auth token' but it failed.")
				_ui.Blank()
				_ui.Print("Possible causes:")
				_ui.Print("  • GitHub CLI (gh) is not installed")
				_ui.Print("  • GitHub CLI is not logged in yet")
				_ui.Print("  • gh is not in your PATH")
				_ui.Blank()
				_ui.Print("To fix:")
				_ui.Print("  1. Install GitHub CLI: https://cli.github.com")
				_ui.Print("  2. Login: gh auth login")
				_ui.Print("  3. Then retry: gcm github login-gh %s", profileName)
				_ui.Blank()
				_ui.Print("Alternative login methods:")
				_ui.Print("  gcm github login %s         (use a Personal Access Token)", profileName)
				_ui.Print("  gcm github login-oauth %s   (browser-based OAuth)", profileName)
				return fmt.Errorf("could not import from GitHub CLI")
			}
			sp.Stop("Token retrieved from GitHub CLI")

			// Verify
			sp2 := _ui.NewSpinner("Verifying token with GitHub...")
			sp2.Start()

			ctr.GitHubClient.SetToken(token)
			user, err := ctr.GitHubClient.GetUser(cmd.Context())
			if err != nil {
				sp2.StopError("Token from GitHub CLI is not valid")
				_ui.Blank()
				_ui.Print("The token from 'gh auth token' was rejected by GitHub.")
				_ui.Print("Your GitHub CLI session may have expired.")
				_ui.Blank()
				_ui.Print("To fix:")
				_ui.Print("  1. Re-login in GitHub CLI: gh auth login")
				_ui.Print("  2. Then retry: gcm github login-gh %s", profileName)
				return fmt.Errorf("token from GitHub CLI is not valid")
			}
			sp2.Stop("Token verified!")

			tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodLegacy, TokenType: "pat"}
			ok, transitionErr := applyProfileProviderTransition(cmd.Context(), profileName, p, def, user.Login, _provider.AuthMethodLegacy, true, func() error {
				return saveProviderToken(profileName, def, p, tokenSet)
			})
			if transitionErr != nil {
				ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName, nil, transitionErr)
				return transitionErr
			}
			if !ok {
				_ui.Info("Provider change cancelled")
				return nil
			}
			_ = ctr.GitHubClient.SaveToken(profileName, token)

			ctr.AuditLogger.Log(_audit.ActionGitHubLogin, profileName,
				map[string]string{"user": user.Login, "method": "gh-cli"}, nil)
			_ui.Blank()
			if user.Name != "" {
				_ui.Success("Logged in as %s (%s) via GitHub CLI", _ui.Bold(user.Login), user.Name)
			} else {
				_ui.Success("Logged in as %s via GitHub CLI", _ui.Bold(user.Login))
			}

			// Only update git credentials if this is the active profile
			if isActiveProfile(profileName) {
				configureGitCredentialsForProvider(profileName, p, def, tokenSet)
				_ui.Print("  Git credentials updated — git push/pull will use this account.")
			} else {
				_ui.Blank()
				_ui.Print("  Note: This is not the active profile.")
				_ui.Print("  Git credentials will be updated when you switch to it:")
				_ui.Print("    gcm use %s", profileName)
			}

			_ = ctr.ProfileManager.Update(p)

			// Auto-activate globally if this is the first authenticated profile
			activateAsGlobalIfFirst(profileName)

			if p != nil {
				if def, ok := ctr.ProviderRegistry.Get(_provider.GitHubID); ok {
					setupUploadKeysForProvider(cmd.Context(), profileName, p, def)
				}
			}

			return nil
		},
	}
}

func newGitHubStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "Show authentication status for GitHub profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			def, err := githubProviderDefinition()
			if err != nil {
				return err
			}
			return runProviderSpecificAuthStatus(cmd.Context(), def)
		},
	}
}
