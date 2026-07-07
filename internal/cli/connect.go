package cli

import (
	"context"
	"fmt"
	"strings"

	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"

	cobra "github.com/spf13/cobra"
)

type connectOptions struct {
	provider   string
	tokenStdin bool
	yes        bool
}

func newConnectCmd() *cobra.Command {
	opts := connectOptions{}
	cmd := &cobra.Command{
		Use:   "connect <profile>",
		Short: "Connect a profile to its Git provider",
		Long: `Connect a profile to a Git provider with one provider-scoped workflow.

This is the provider-neutral login path. It verifies the token, applies the
one-provider-per-profile invariant, cleans old provider data when needed, and
updates credentials for the active profile.`,
		Example: `  gcm connect work --provider github
  echo "$GITLAB_TOKEN" | gcm connect work --provider gitlab --token-stdin --yes`,
		Args: requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnect(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider to connect (github, gitlab)")
	cmd.Flags().BoolVar(&opts.tokenStdin, "token-stdin", false, "Read the provider token from stdin")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Confirm provider transition cleanup without prompting")
	return cmd
}

func newSwitchProviderCmd() *cobra.Command {
	opts := connectOptions{}
	cmd := &cobra.Command{
		Use:   "switch-provider <profile> <provider>",
		Short: "Move a profile to another provider",
		Long: `Move a profile to another provider using the same cleanup semantics as login.

The command verifies the new provider token before changing the profile. When
the profile already belongs to another provider, GCM cleans old provider token,
cached git credentials, credential username, and uploaded keys when possible.`,
		Example: `  gcm switch-provider work gitlab
  echo "$GH_TOKEN" | gcm switch-provider work github --token-stdin --yes`,
		Args: requireArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.provider = args[1]
			return runConnect(cmd.Context(), args[0], opts)
		},
	}
	cmd.Flags().BoolVar(&opts.tokenStdin, "token-stdin", false, "Read the provider token from stdin")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "Confirm provider transition cleanup without prompting")
	return cmd
}

func runConnect(ctx context.Context, profileName string, opts connectOptions) error {
	p, err := ctr.ProfileManager.Get(profileName)
	if err != nil {
		return fmt.Errorf("profile %q not found\n\n  To see available profiles: gcm profile list\n  To create a new profile:   gcm profile create %s -i", profileName, profileName)
	}

	def, err := resolveConnectProvider(profileName, p, opts.provider)
	if err != nil {
		return err
	}
	if !def.Capabilities.Has(_provider.CapabilityPATAuth) {
		return fmt.Errorf("%s does not support PAT authentication in GCM yet", def.DisplayName)
	}

	token, stdinMode, err := readConnectToken(profileName, def, opts)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	sp := _ui.NewSpinner(fmt.Sprintf("Verifying token with %s...", def.DisplayName))
	sp.Start()
	username, displayName, err := verifyProviderPAT(ctx, def, token)
	if err != nil {
		sp.StopError("Token is not valid")
		_ui.Blank()
		_ui.Print("The token was rejected by %s at %s.", def.DisplayName, def.APIURL)
		_ui.Print("Check token scopes, expiration, revocation, and self-managed provider URL.")
		return err
	}
	sp.Stop("Token verified!")

	tokenSet := _provider.TokenSet{AccessToken: token, AuthMethod: _provider.AuthMethodPAT, TokenType: "pat"}
	transitionOpts := providerTransitionOptions{AllowPrompt: !stdinMode, AutoConfirm: opts.yes}
	ok, transitionErr := applyProfileProviderTransitionWithOptions(ctx, profileName, p, def, username, _provider.AuthMethodPAT, transitionOpts, func() error {
		return saveProviderToken(profileName, def, p, tokenSet)
	})
	if transitionErr != nil {
		ctr.AuditLogger.Log(_audit.ActionProviderLogin, profileName,
			map[string]string{"provider": string(def.ID), "method": "pat"}, transitionErr)
		return transitionErr
	}
	if !ok {
		_ui.Info("Provider change cancelled")
		return nil
	}

	if err := ctr.ProfileManager.Update(p); err != nil {
		_ui.Warning("Token was saved, but profile metadata could not be updated: %v", err)
	}

	ctr.AuditLogger.Log(_audit.ActionProviderLogin, profileName,
		map[string]string{"provider": string(def.ID), "user": username, "method": "pat"}, nil)

	_ui.Blank()
	if displayName != "" {
		_ui.Success("Connected %s to %s as %s (%s)", profileName, def.DisplayName, _ui.Bold(username), displayName)
	} else {
		_ui.Success("Connected %s to %s as %s", profileName, def.DisplayName, _ui.Bold(username))
	}

	if isActiveProfile(profileName) {
		configureGitCredentialsForProvider(profileName, p, def, tokenSet)
		_ui.Print("  Git credentials updated for the active profile.")
	}

	if !stdinMode {
		setupUploadKeysForProvider(ctx, profileName, p, def)
	}
	return nil
}

func resolveConnectProvider(profileName string, p *_profile.Profile, requested string) (_provider.Definition, error) {
	if requested != "" {
		id := normalizeProviderSelection(requested)
		def, ok := ctr.ProviderRegistry.Get(id)
		if !ok {
			return _provider.Definition{}, fmt.Errorf("provider %q is not configured", requested)
		}
		return def, nil
	}

	if def, ok := profileProviderDefinition(p, _provider.CapabilityPATAuth); ok {
		return def, nil
	}

	defs := providerDefinitionsWithCapability(_provider.CapabilityPATAuth)
	if len(defs) == 0 {
		return _provider.Definition{}, fmt.Errorf("no provider supports PAT authentication")
	}
	if isStdinPiped() {
		return _provider.Definition{}, fmt.Errorf("--provider is required when connecting non-interactively")
	}

	options := make([]string, 0, len(defs))
	byOption := make(map[string]_provider.Definition, len(defs))
	for _, def := range defs {
		option := providerOption(def)
		options = append(options, option)
		byOption[option] = def
	}
	selected, err := _ui.AskSelect(fmt.Sprintf("Provider for profile %q:", profileName), options)
	if err != nil {
		return _provider.Definition{}, err
	}
	return byOption[selected], nil
}

func readConnectToken(profileName string, def _provider.Definition, opts connectOptions) (string, bool, error) {
	stdinMode := opts.tokenStdin || isStdinPiped()
	if stdinMode {
		token, err := readStdinToken()
		if err != nil {
			return "", true, fmt.Errorf("could not read token from input\n\n  Example: echo \"$TOKEN\" | gcm connect %s --provider %s --token-stdin", profileName, def.ID)
		}
		return token, true, nil
	}

	_ui.Header("%s Connect %s to %s", _ui.IconKey, profileName, def.DisplayName)
	_ui.Blank()
	_ui.Print("Create a Personal Access Token for %s.", def.DisplayName)
	if url := providerPATURL(def); url != "" {
		_ui.Print("Token settings: %s", _ui.Cyan(url))
	}
	if len(def.Scopes) > 0 {
		_ui.Print("Recommended scopes: %s", strings.Join(def.Scopes, ", "))
	}
	_ui.Blank()
	token, err := _ui.AskPassword("Enter token")
	if err != nil {
		return "", false, fmt.Errorf("could not read token input")
	}
	return token, false, nil
}

func verifyProviderPAT(ctx context.Context, def _provider.Definition, token string) (string, string, error) {
	user, err := ctr.ProviderClient.VerifyPAT(ctx, def, token)
	return user.Username, user.Name, err
}

func providerPATURL(def _provider.Definition) string {
	webURL := strings.TrimRight(def.WebURL, "/")
	if webURL == "" {
		webURL = strings.TrimRight(def.CredentialServer(), "/")
	}
	switch def.ID {
	case _provider.GitHubID:
		return webURL + "/settings/tokens"
	case _provider.GitLabID:
		return webURL + "/-/user_settings/personal_access_tokens"
	default:
		return webURL
	}
}
