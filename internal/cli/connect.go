package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/justjundana/git-config-manager/internal/audit"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/ui"

	"github.com/spf13/cobra"
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
	if !def.Capabilities.Has(providerpkg.CapabilityPATAuth) {
		return fmt.Errorf("%s does not support PAT authentication in GCM yet", def.DisplayName)
	}

	token, stdinMode, err := readConnectToken(profileName, def, opts)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	sp := ui.NewSpinner(fmt.Sprintf("Verifying token with %s...", def.DisplayName))
	sp.Start()
	username, displayName, err := verifyProviderPAT(ctx, def, token)
	if err != nil {
		sp.StopError("Token is not valid")
		ui.Blank()
		ui.Print("The token was rejected by %s at %s.", def.DisplayName, def.APIURL)
		ui.Print("Check token scopes, expiration, revocation, and self-managed provider URL.")
		return err
	}
	sp.Stop("Token verified!")

	tokenSet := providerpkg.TokenSet{AccessToken: token, AuthMethod: providerpkg.AuthMethodPAT, TokenType: "pat"}
	transitionOpts := providerTransitionOptions{AllowPrompt: !stdinMode, AutoConfirm: opts.yes}
	ok, transitionErr := applyProfileProviderTransitionWithOptions(ctx, profileName, p, def, username, providerpkg.AuthMethodPAT, transitionOpts, func() error {
		return saveProviderToken(profileName, def, p, tokenSet)
	})
	if transitionErr != nil {
		ctr.AuditLogger.Log(audit.ActionProviderLogin, profileName,
			map[string]string{"provider": string(def.ID), "method": "pat"}, transitionErr)
		return transitionErr
	}
	if !ok {
		ui.Info("Provider change cancelled")
		return nil
	}

	if err := ctr.ProfileManager.Update(p); err != nil {
		ui.Warning("Token was saved, but profile metadata could not be updated: %v", err)
	}

	ctr.AuditLogger.Log(audit.ActionProviderLogin, profileName,
		map[string]string{"provider": string(def.ID), "user": username, "method": "pat"}, nil)

	ui.Blank()
	if displayName != "" {
		ui.Success("Connected %s to %s as %s (%s)", profileName, def.DisplayName, ui.Bold(username), displayName)
	} else {
		ui.Success("Connected %s to %s as %s", profileName, def.DisplayName, ui.Bold(username))
	}

	if isActiveProfile(profileName) {
		configureGitCredentialsForProvider(profileName, p, def, tokenSet)
		ui.Print("  Git credentials updated for the active profile.")
	}

	if !stdinMode {
		setupUploadKeysForProvider(ctx, profileName, p, def)
	}
	return nil
}

func resolveConnectProvider(profileName string, p *profile.Profile, requested string) (providerpkg.Definition, error) {
	if requested != "" {
		id := normalizeProviderSelection(requested)
		def, ok := ctr.ProviderRegistry.Get(id)
		if !ok {
			return providerpkg.Definition{}, fmt.Errorf("provider %q is not configured", requested)
		}
		return def, nil
	}

	if def, ok := profileProviderDefinition(p, providerpkg.CapabilityPATAuth); ok {
		return def, nil
	}

	defs := providerDefinitionsWithCapability(providerpkg.CapabilityPATAuth)
	if len(defs) == 0 {
		return providerpkg.Definition{}, fmt.Errorf("no provider supports PAT authentication")
	}
	if isStdinPiped() {
		return providerpkg.Definition{}, fmt.Errorf("--provider is required when connecting non-interactively")
	}

	options := make([]string, 0, len(defs))
	byOption := make(map[string]providerpkg.Definition, len(defs))
	for _, def := range defs {
		option := providerOption(def)
		options = append(options, option)
		byOption[option] = def
	}
	selected, err := ui.AskSelect(fmt.Sprintf("Provider for profile %q:", profileName), options)
	if err != nil {
		return providerpkg.Definition{}, err
	}
	return byOption[selected], nil
}

func readConnectToken(profileName string, def providerpkg.Definition, opts connectOptions) (string, bool, error) {
	stdinMode := opts.tokenStdin || isStdinPiped()
	if stdinMode {
		token, err := readStdinToken()
		if err != nil {
			return "", true, fmt.Errorf("could not read token from input\n\n  Example: echo \"$TOKEN\" | gcm connect %s --provider %s --token-stdin", profileName, def.ID)
		}
		return token, true, nil
	}

	ui.Header("%s Connect %s to %s", ui.IconKey, profileName, def.DisplayName)
	ui.Blank()
	ui.Print("Create a Personal Access Token for %s.", def.DisplayName)
	if url := providerPATURL(def); url != "" {
		ui.Print("Token settings: %s", ui.Cyan(url))
	}
	if len(def.Scopes) > 0 {
		ui.Print("Recommended scopes: %s", strings.Join(def.Scopes, ", "))
	}
	ui.Blank()
	token, err := ui.AskPassword("Enter token")
	if err != nil {
		return "", false, fmt.Errorf("could not read token input")
	}
	return token, false, nil
}

func verifyProviderPAT(ctx context.Context, def providerpkg.Definition, token string) (string, string, error) {
	user, err := ctr.ProviderClient.VerifyPAT(ctx, def, token)
	return user.Username, user.Name, err
}

func providerPATURL(def providerpkg.Definition) string {
	webURL := strings.TrimRight(def.WebURL, "/")
	if webURL == "" {
		webURL = strings.TrimRight(def.CredentialServer(), "/")
	}
	switch def.ID {
	case providerpkg.GitHubID:
		return webURL + "/settings/tokens"
	case providerpkg.GitLabID:
		return webURL + "/-/user_settings/personal_access_tokens"
	default:
		return webURL
	}
}
