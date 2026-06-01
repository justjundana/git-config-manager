package cli

import (
	"context"
	"fmt"

	"github.com/justjundana/git-config-manager/internal/audit"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
)

func newGitLabCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gitlab",
		Short:   "Manage GitLab integration",
		Aliases: []string{"gl"},
	}

	cmd.AddCommand(newGitLabLoginCmd())
	cmd.AddCommand(newGitLabLogoutCmd())
	cmd.AddCommand(newGitLabVerifyCmd())
	cmd.AddCommand(newGitLabUserCmd())
	cmd.AddCommand(newGitLabStatusCmd())

	return cmd
}

func gitLabProviderDefinition() (providerpkg.Definition, error) {
	def, ok := ctr.ProviderRegistry.Get(providerpkg.GitLabID)
	if !ok {
		return providerpkg.Definition{}, fmt.Errorf("GitLab provider is not configured")
	}
	return def, nil
}

func newGitLabLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <profile>",
		Short: "Authenticate with GitLab using a Personal Access Token (PAT)",
		Long: `Authenticate with GitLab using a Personal Access Token.

For GitLab.com, create a token at https://gitlab.com/-/user_settings/personal_access_tokens.
For self-managed GitLab, configure providers.gitlab.api_url/web_url/git_hosts first.

Recommended scopes for this MVP: api, read_user, read_repository, write_repository.

Examples:
	gcm gitlab login work-gitlab
	echo "$GITLAB_TOKEN" | gcm gitlab login work-gitlab`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]
			profileConfig, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list\n  To create a new profile:   gcm profile create %s -i", profileName, profileName)
			}

			def, err := gitLabProviderDefinition()
			if err != nil {
				return err
			}

			var token string
			stdinPiped := isStdinPiped()
			if stdinPiped {
				token, err = readStdinToken()
				if err != nil {
					return fmt.Errorf("could not read token from input\n\n  Make sure you're piping a valid token:\n  echo \"$GITLAB_TOKEN\" | gcm gitlab login %s", profileName)
				}
			} else {
				ui.Header("%s GitLab Login for Profile: %s", ui.IconKey, profileName)
				ui.Blank()
				ui.Print("Create a GitLab Personal Access Token with scopes: api, read_user, read_repository, write_repository")
				ui.Print("Provider: %s", ui.Cyan(def.WebURL))
				ui.Blank()
				token, err = ui.AskPassword("Enter token")
				if err != nil {
					return fmt.Errorf("could not read token input")
				}
			}

			if token == "" {
				return fmt.Errorf("token cannot be empty")
			}

			tokenSet := providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"}
			ctr.GitLabClient.SetTokenSet(tokenSet)

			sp := ui.NewSpinner("Verifying token with GitLab...")
			sp.Start()
			user, err := ctr.GitLabClient.GetUser(cmd.Context())
			if err != nil {
				sp.StopError("Token is not valid")
				ui.Blank()
				ui.Print("The token was rejected by GitLab at %s.", def.APIURL)
				ui.Print("Common causes: missing scope, expired token, revoked token, or wrong self-managed URL.")
				return fmt.Errorf("GitLab token verification failed")
			}
			sp.Stop("Token verified!")

			ok, transitionErr := applyProfileProviderTransition(cmd.Context(), profileName, profileConfig, def, user.Username, providerpkg.AuthMethodPAT, !stdinPiped, func() error {
				return saveProviderToken(profileName, def, profileConfig, tokenSet)
			})
			if transitionErr != nil {
				ctr.AuditLogger.Log(audit.ActionProviderLogin, profileName,
					map[string]string{"provider": string(providerpkg.GitLabID), "method": "pat"}, transitionErr)
				return transitionErr
			}
			if !ok {
				ui.Info("Provider change cancelled")
				return nil
			}

			if err := ctr.ProfileManager.Update(profileConfig); err != nil {
				ui.Warning("GitLab token was saved, but profile metadata could not be updated: %v", err)
			}

			ctr.AuditLogger.Log(audit.ActionProviderLogin, profileName,
				map[string]string{"provider": string(providerpkg.GitLabID), "user": user.Username, "method": "pat"}, nil)

			ui.Blank()
			if user.Name != "" {
				ui.Success("Logged in to GitLab as %s (%s)", ui.Bold(user.Username), user.Name)
			} else {
				ui.Success("Logged in to GitLab as %s", ui.Bold(user.Username))
			}
			if user.WebURL != "" {
				ui.Detail("GitLab Profile", user.WebURL)
			}

			if isActiveProfile(profileName) {
				configureGitCredentialsForProvider(profileName, profileConfig, def, tokenSet)
				ui.Print("  Git credentials updated — git push/pull will use this GitLab account.")
			} else {
				ui.Blank()
				ui.Print("  Note: This is not the active profile.")
				ui.Print("  Git credentials will be updated when you switch to it:")
				ui.Print("    gcm use %s", profileName)
			}

			activateAsGlobalIfFirst(profileName)
			if !stdinPiped {
				setupUploadKeysForGitLab(cmd.Context(), profileName)
			}
			return nil
		},
	}
}

func newGitLabLogoutCmd() *cobra.Command {
	var clearGitCreds bool
	var forceLogout bool

	cmd := &cobra.Command{
		Use:   "logout <profile>",
		Short: "Remove stored GitLab token for a profile",
		Args:  requireArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profileName := args[0]
			profileConfig, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found", profileName)
			}
			def, err := gitLabProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, profileConfig, def); err != nil {
				return err
			}

			if !isActiveProfile(profileName) && !forceLogout {
				ui.Warning("Profile %q is not the active profile.", profileName)
				confirm, askErr := ui.AskConfirm(fmt.Sprintf("Remove the GitLab token for %q?", profileName), false)
				if askErr != nil || !confirm {
					ui.Info("Cancelled.")
					return nil
				}
			}

			hadStoredToken := providerTokenPresent(profileName, def, profileConfig)
			if err := deleteProviderToken(profileName, def, profileConfig); err != nil {
				ctr.AuditLogger.Log(audit.ActionProviderLogout, profileName,
					map[string]string{"provider": string(providerpkg.GitLabID)}, err)
				return fmt.Errorf("could not remove GitLab token for profile %q", profileName)
			}

			ctr.AuditLogger.Log(audit.ActionProviderLogout, profileName,
				map[string]string{"provider": string(providerpkg.GitLabID)}, nil)
			if hadStoredToken {
				ui.Success("GitLab token removed for profile %q", profileName)
			} else {
				ui.Info("No GitLab token was stored for profile %q.", profileName)
			}

			if clearGitCreds && isActiveProfile(profileName) {
				_ = ctr.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "")
				if err := ctr.GitHubClient.ClearGitCredentials(def.CredentialServer()); err != nil {
					ui.Warning("Git credentials could not be cleared automatically.")
				} else {
					ui.Success("HTTPS Git credentials and username pin cleared for %s.", def.CredentialServer())
					ui.Print("  SSH remotes and profile SSH keys are unchanged, so git may still work over SSH.")
				}
			}

			ui.Blank()
			ui.Print("To re-authenticate later: gcm gitlab login %s", profileName)
			return nil
		},
	}

	cmd.Flags().BoolVar(&clearGitCreds, "clear-credentials", true, "Also clear cached git credentials")
	cmd.Flags().BoolVarP(&forceLogout, "force", "f", false, "Skip confirmation when logging out a non-active profile")
	return cmd
}

func newGitLabVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <profile>",
		Short: "Verify that the stored GitLab token is still valid",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]
			profileConfig, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found", profileName)
			}
			def, err := gitLabProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, profileConfig, def); err != nil {
				return err
			}
			token, err := loadProviderToken(profileName, def, profileConfig)
			if err != nil {
				ui.Print("Profile %q is not authenticated with GitLab yet.", profileName)
				ui.Print("To authenticate: gcm gitlab login %s", profileName)
				return fmt.Errorf("profile %q is not authenticated with GitLab", profileName)
			}
			ctr.GitLabClient.SetTokenSet(token)
			user, err := ctr.GitLabClient.VerifyToken(cmd.Context())
			if err != nil {
				ui.Print("The stored GitLab token for profile %q is no longer valid.", profileName)
				ui.Print("To fix, re-authenticate: gcm gitlab login %s", profileName)
				return fmt.Errorf("GitLab token expired or revoked for profile %q", profileName)
			}
			ui.Success("Authenticated with GitLab as %s", ui.Bold(user.Username))
			return nil
		},
	}
}

func newGitLabUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "user <profile>",
		Short: "Show GitLab user information for a profile",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]
			profileConfig, err := ctr.ProfileManager.Get(profileName)
			if err != nil {
				return fmt.Errorf("profile %q not found", profileName)
			}
			def, err := gitLabProviderDefinition()
			if err != nil {
				return err
			}
			if err := requireProfileProvider(profileName, profileConfig, def); err != nil {
				return err
			}
			token, err := loadProviderToken(profileName, def, profileConfig)
			if err != nil {
				ui.Print("Profile %q is not authenticated with GitLab yet.", profileName)
				ui.Print("To authenticate: gcm gitlab login %s", profileName)
				return fmt.Errorf("profile %q is not authenticated with GitLab", profileName)
			}
			ctr.GitLabClient.SetTokenSet(token)
			user, err := ctr.GitLabClient.GetUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("could not fetch GitLab user info for %q", profileName)
			}
			ui.Header("GitLab User: %s", user.Username)
			ui.Detail("Name", user.Name)
			ui.Detail("Email", user.Email)
			ui.Detail("Public Email", user.PublicEmail)
			ui.Detail("URL", user.WebURL)
			return nil
		},
	}
}

func newGitLabStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show GitLab authentication status for GitLab profiles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			def, err := gitLabProviderDefinition()
			if err != nil {
				return err
			}
			return runProviderSpecificAuthStatus(cmd.Context(), def)
		},
	}
}

func setupUploadKeysForGitLab(ctx context.Context, profileName string) {
	def, err := gitLabProviderDefinition()
	if err != nil {
		return
	}

	profileConfig, err := ctr.ProfileManager.Get(profileName)
	if err != nil || profileConfig == nil {
		return
	}
	setupUploadKeysForProvider(ctx, profileName, profileConfig, def)
}
