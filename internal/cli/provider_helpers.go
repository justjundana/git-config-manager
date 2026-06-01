package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/pkg/ui"
)

func providerAccountForProfile(p *profile.Profile, id providerpkg.ProviderID) profile.ProviderAccountConfig {
	return profile.ProviderAccount(p, id)
}

func setProfileProviderAccount(p *profile.Profile, id providerpkg.ProviderID, username, authMethod string) {
	profile.SetProviderAccount(p, id, username, authMethod)
}

type providerTransitionOptions struct {
	AllowPrompt bool
	AutoConfirm bool
}

func applyProfileProviderTransition(ctx context.Context, profileName string, p *profile.Profile, def providerpkg.Definition, username, authMethod string, allowPrompt bool, afterSet func() error) (bool, error) {
	return applyProfileProviderTransitionWithOptions(ctx, profileName, p, def, username, authMethod, providerTransitionOptions{AllowPrompt: allowPrompt}, afterSet)
}

func applyProfileProviderTransitionWithOptions(ctx context.Context, profileName string, p *profile.Profile, def providerpkg.Definition, username, authMethod string, opts providerTransitionOptions, afterSet func() error) (bool, error) {
	if p == nil {
		return true, nil
	}

	oldState := cloneProfileProviderState(p)
	cleanupDefs := providerDefinitionsToClean(oldState, def.ID)
	if len(cleanupDefs) > 0 {
		if !opts.AllowPrompt && !opts.AutoConfirm {
			return false, fmt.Errorf("profile %q is already configured for %s; change provider interactively first: gcm profile edit %s -i", profileName, providerNames(cleanupDefs), profileName)
		}
		if !opts.AutoConfirm {
			if ok, err := confirmProviderTransition(profileName, cleanupDefs, def); err != nil || !ok {
				return false, err
			}
		}
	}

	setProfileProviderAccount(p, def.ID, username, authMethod)
	if afterSet != nil {
		if err := afterSet(); err != nil {
			restoreProfileProviderState(p, oldState)
			return false, err
		}
	}

	cleanupProviderData(ctx, profileName, oldState, cleanupDefs)
	if migrated, err := migrateProfileSSHKeyPathToProvider(profileName, p); err != nil {
		ui.Warning("Could not rename SSH key to provider format: %v", err)
	} else if migrated {
		ui.Detail("SSH Key Renamed", p.SSH.KeyPath)
	}

	return true, nil
}

func confirmProviderTransition(profileName string, oldDefs []providerpkg.Definition, newDef providerpkg.Definition) (bool, error) {
	ui.Warning("Changing provider for profile %q: %s → %s", profileName, providerNames(oldDefs), newDef.DisplayName)
	ui.Print("  GCM will clean old provider data before this profile uses %s:", newDef.DisplayName)
	ui.Print("  - stored provider token(s)")
	ui.Print("  - cached git credentials and credential username")
	ui.Print("  - uploaded SSH/GPG keys on the old provider when the old token can access them")
	ui.Print("  - local SSH key filename will be renamed to the new provider format")
	return ui.AskConfirm("Continue and clean old provider data?", false)
}

func providerDefinitionsToClean(p *profile.Profile, keep providerpkg.ProviderID) []providerpkg.Definition {
	if p == nil || ctr == nil || ctr.ProviderRegistry == nil {
		return nil
	}
	seen := make(map[providerpkg.ProviderID]bool)
	var defs []providerpkg.Definition
	add := func(id providerpkg.ProviderID) {
		if id == "" || id == keep || seen[id] {
			return
		}
		def, ok := ctr.ProviderRegistry.Get(id)
		if !ok || !def.Capabilities.Has(providerpkg.CapabilityCredentialHelper) {
			return
		}
		seen[id] = true
		defs = append(defs, def)
	}
	for id := range p.Providers {
		add(providerpkg.ProviderID(id))
	}
	if p.GitHub != nil {
		add(providerpkg.GitHubID)
	}
	return defs
}

func cleanupProviderData(ctx context.Context, profileName string, p *profile.Profile, defs []providerpkg.Definition) {
	if p == nil || len(defs) == 0 {
		return
	}

	var sshPubKey string
	if p.SSH != nil && p.SSH.KeyPath != "" {
		sshPubKey, _ = ctr.SSHManager.GetPublicKey(p.SSH.KeyPath)
	}

	for _, def := range defs {
		token, tokenErr := loadProviderToken(profileName, def, p)
		if tokenErr == nil && token.AccessToken != "" {
			if sshPubKey != "" && def.Capabilities.Has(providerpkg.CapabilitySSHKeys) {
				if deleted, delErr := deleteProviderSSHKey(ctx, def, token, sshPubKey); delErr != nil {
					ui.Warning("Could not delete SSH key from %s: %v", def.DisplayName, delErr)
				} else if deleted {
					ui.Success("SSH key removed from %s", def.DisplayName)
				}
			}
			if p.GPG != nil && p.GPG.KeyID != "" && def.Capabilities.Has(providerpkg.CapabilityGPGKeys) {
				if deleted, delErr := deleteProviderGPGKey(ctx, def, token, p.GPG.KeyID); delErr != nil {
					ui.Warning("Could not delete GPG key from %s: %v", def.DisplayName, delErr)
				} else if deleted {
					ui.Success("GPG key removed from %s", def.DisplayName)
				}
			}
		}

		removedToken := false
		if delErr := deleteProviderToken(profileName, def, p); delErr == nil {
			removedToken = true
		}
		if def.ID == providerpkg.GitHubID {
			if delErr := ctr.GitHubClient.DeleteToken(profileName); delErr == nil {
				removedToken = true
			}
		}
		if removedToken {
			ui.Success("%s token removed", def.DisplayName)
		}

		_ = ctr.GitHubClient.ClearGitCredentials(def.CredentialServer())
		_ = ctr.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "")
	}
}

func cloneProfileProviderState(p *profile.Profile) *profile.Profile {
	if p == nil {
		return nil
	}
	clone := *p
	if p.Providers != nil {
		clone.Providers = make(map[string]profile.ProviderAccountConfig, len(p.Providers))
		for id, account := range p.Providers {
			clone.Providers[id] = account
		}
	}
	if p.GitHub != nil {
		githubConfig := *p.GitHub
		clone.GitHub = &githubConfig
	}
	if p.SSH != nil {
		sshConfig := *p.SSH
		clone.SSH = &sshConfig
	}
	if p.GPG != nil {
		gpgConfig := *p.GPG
		clone.GPG = &gpgConfig
	}
	return &clone
}

func restoreProfileProviderState(p *profile.Profile, snapshot *profile.Profile) {
	if p == nil || snapshot == nil {
		return
	}
	restored := cloneProfileProviderState(snapshot)
	p.Providers = restored.Providers
	p.GitHub = restored.GitHub
	p.SSH = restored.SSH
	p.GPG = restored.GPG
}

func providerNames(defs []providerpkg.Definition) string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.DisplayName)
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func profileProviderID(p *profile.Profile) (providerpkg.ProviderID, bool) {
	return profile.ProviderID(p)
}

func profileProviderDefinition(p *profile.Profile, capability providerpkg.Capability) (providerpkg.Definition, bool) {
	id, ok := profileProviderID(p)
	if !ok || ctr == nil || ctr.ProviderRegistry == nil {
		return providerpkg.Definition{}, false
	}
	def, ok := ctr.ProviderRegistry.Get(id)
	if !ok || !def.Capabilities.Has(capability) {
		return providerpkg.Definition{}, false
	}
	return def, true
}

func profileUsesProvider(p *profile.Profile, id providerpkg.ProviderID) bool {
	return profile.UsesProvider(p, id)
}

func profileHasMultipleProviders(p *profile.Profile) bool {
	return profile.HasMultipleProviders(p)
}

func providerTokenKey(profileName string, def providerpkg.Definition, account profile.ProviderAccountConfig) providerpkg.TokenKey {
	return providerpkg.TokenKey{
		Profile:  profileName,
		Provider: def.ID,
		Host:     firstProviderHost(def),
		Account:  account.Account,
	}
}

func firstProviderHost(def providerpkg.Definition) string {
	if len(def.GitHosts) > 0 && def.GitHosts[0] != "" {
		return providerpkg.NormalizeHost(def.GitHosts[0])
	}
	if def.WebURL != "" {
		return providerpkg.NormalizeHost(def.WebURL)
	}
	return providerpkg.NormalizeHost(def.APIURL)
}

func loadProviderToken(profileName string, def providerpkg.Definition, p *profile.Profile) (providerpkg.TokenSet, error) {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.LoadTokenSet(providerTokenKey(profileName, def, account))
}

func saveProviderToken(profileName string, def providerpkg.Definition, p *profile.Profile, token providerpkg.TokenSet) error {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.SaveTokenSet(providerTokenKey(profileName, def, account), token)
}

func deleteProviderToken(profileName string, def providerpkg.Definition, p *profile.Profile) error {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.DeleteTokenSet(providerTokenKey(profileName, def, account))
}

func providerTokenPresent(profileName string, def providerpkg.Definition, p *profile.Profile) bool {
	token, err := loadProviderToken(profileName, def, p)
	return err == nil && token.AccessToken != ""
}

func configureGitCredentialsForProvider(profileName string, p *profile.Profile, def providerpkg.Definition, token providerpkg.TokenSet) {
	server := def.CredentialServer()
	account := providerAccountForProfile(p, def.ID)
	username := def.CredentialUsername(profileName, account.Username, token)
	clearGitCredentialsForOtherProviders(def)

	if IsCredentialHelperConfiguredFor(server) {
		_ = ctr.GitHubClient.SetGitCredentialUsername(server, username)
		return
	}

	_ = ctr.GitHubClient.ClearGitCredentials(server)
	_ = ctr.GitHubClient.StoreGitCredentials(server, username, token.AccessToken)
	_ = ctr.GitHubClient.SetGitCredentialUsername(server, username)
}

func clearGitCredentialsForOtherProviders(active providerpkg.Definition) {
	if ctr == nil || ctr.ProviderRegistry == nil {
		return
	}
	for _, def := range ctr.ProviderRegistry.All() {
		if def.ID == active.ID || !def.Capabilities.Has(providerpkg.CapabilityCredentialHelper) {
			continue
		}
		_ = ctr.GitHubClient.ClearGitCredentials(def.CredentialServer())
	}
}

func clearProfileProviderAccount(p *profile.Profile, id providerpkg.ProviderID) {
	profile.ClearProviderAccount(p, id)
}

func clearAllProfileProviderAccounts(p *profile.Profile) {
	profile.ClearProviderAccounts(p)
}

func providerDefinitionsWithCapability(capability providerpkg.Capability) []providerpkg.Definition {
	if ctr == nil || ctr.ProviderRegistry == nil {
		return nil
	}
	var defs []providerpkg.Definition
	for _, def := range ctr.ProviderRegistry.All() {
		if def.Capabilities.Has(capability) {
			defs = append(defs, def)
		}
	}
	return defs
}

func selectProfileProviderWithCapability(profileName string, p *profile.Profile, requested string, capability providerpkg.Capability) (providerpkg.Definition, error) {
	def, ok := profileProviderDefinition(p, capability)
	if !ok {
		return providerpkg.Definition{}, fmt.Errorf("profile %q has no provider for this operation; set one with: gcm profile edit %s -i", profileName, profileName)
	}
	if requested == "" {
		return def, nil
	}
	requestedID := normalizeProviderSelection(requested)
	if requestedID != def.ID {
		return providerpkg.Definition{}, fmt.Errorf("profile %q is configured for %s, not %s", profileName, def.DisplayName, requested)
	}
	return def, nil
}

func requireProfileProvider(profileName string, p *profile.Profile, def providerpkg.Definition) error {
	if profileHasMultipleProviders(p) {
		return fmt.Errorf("profile %q has multiple providers configured; choose exactly one with: gcm profile edit %s -i", profileName, profileName)
	}
	if !profileUsesProvider(p, def.ID) {
		return fmt.Errorf("profile %q is not configured for %s; run: gcm connect %s --provider %s", profileName, def.DisplayName, profileName, def.ID)
	}
	return nil
}

func normalizeProviderSelection(value string) providerpkg.ProviderID {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gh", "github":
		return providerpkg.GitHubID
	case "gl", "gitlab":
		return providerpkg.GitLabID
	case "bb", "bitbucket":
		return providerpkg.BitbucketID
	default:
		return providerpkg.ProviderID(strings.ToLower(strings.TrimSpace(value)))
	}
}

func providerOption(def providerpkg.Definition) string {
	host := firstProviderHost(def)
	if host == "" {
		return def.DisplayName
	}
	return fmt.Sprintf("%s (%s)", def.DisplayName, host)
}

func setupUploadKeys(ctx context.Context, profileName string) {
	p, err := ctr.ProfileManager.Get(profileName)
	if err != nil || p == nil {
		return
	}
	def, ok := profileProviderDefinition(p, providerpkg.CapabilitySSHKeys)
	if !ok {
		return
	}
	setupUploadKeysForProvider(ctx, profileName, p, def)
}

func setupUploadKeysForProvider(ctx context.Context, profileName string, p *profile.Profile, def providerpkg.Definition) {
	if p == nil || !def.UploadKeys {
		return
	}

	uploaded := false
	if p.SSH != nil && p.SSH.KeyPath != "" {
		pubKey, pubErr := ctr.SSHManager.GetPublicKey(p.SSH.KeyPath)
		if pubErr == nil && pubKey != "" {
			uploaded = setupSSHKeyUploadForProvider(ctx, profileName, p, def, pubKey, string(p.SSH.KeyType))
		}
	}

	if p.GPG != nil && p.GPG.KeyID != "" {
		if !uploaded {
			ui.Blank()
		}
		setupGPGKeyUploadForProvider(ctx, profileName, p, def, p.GPG.KeyID)
	}
}

func setupSSHKeyUploadForProvider(ctx context.Context, profileName string, p *profile.Profile, def providerpkg.Definition, publicKey, keyType string) bool {
	if p == nil || !def.UploadKeys || publicKey == "" {
		return false
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return false
	}
	exists, checkErr := providerSSHKeyExists(ctx, def, token, publicKey)
	if checkErr == nil && exists {
		ui.Blank()
		ui.Success("SSH key already on %s", def.DisplayName)
		return false
	}
	if checkErr != nil {
		return false
	}

	ui.Blank()
	upload, askErr := ui.AskConfirm(fmt.Sprintf("Upload SSH key to %s?", def.DisplayName), true)
	if askErr != nil || !upload {
		return false
	}
	title := providerResourceName(profileName, def, "ssh", keyType)
	if uploadErr := uploadProviderSSHKey(ctx, def, token, title, publicKey); uploadErr != nil {
		if providerSSHKeyAlreadyInUse(uploadErr) {
			printProviderSSHKeyAlreadyInUse(profileName, def)
			return false
		}
		ui.Warning("Could not upload SSH key to %s: %v", def.DisplayName, uploadErr)
		return false
	}
	ui.Success("SSH key uploaded to %s", def.DisplayName)
	ui.Detail("Title", title)
	return true
}

func setupGPGKeyUploadForProvider(ctx context.Context, profileName string, p *profile.Profile, def providerpkg.Definition, keyID string) bool {
	if p == nil || !def.UploadKeys || keyID == "" {
		return false
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return false
	}
	exists, checkErr := providerGPGKeyExists(ctx, def, token, keyID)
	if checkErr == nil && exists {
		ui.Success("GPG key already on %s", def.DisplayName)
		return false
	}
	if checkErr != nil {
		return false
	}

	upload, askErr := ui.AskConfirm(fmt.Sprintf("Upload GPG key to %s?", def.DisplayName), true)
	if askErr != nil || !upload {
		return false
	}
	pubKey, gpgErr := ctr.GPGManager.GetPublicKey(keyID)
	if gpgErr != nil {
		ui.Warning("Could not read GPG public key: %v", gpgErr)
		return false
	}
	if uploadErr := uploadProviderGPGKey(ctx, def, token, pubKey); uploadErr != nil {
		ui.Warning("Could not upload GPG key to %s: %v", def.DisplayName, uploadErr)
		return false
	}
	ui.Success("GPG key uploaded to %s", def.DisplayName)
	return true
}

func authenticatedProvidersForProfile(profileName string, p *profile.Profile, capability providerpkg.Capability) []providerpkg.Definition {
	def, ok := profileProviderDefinition(p, capability)
	if !ok {
		return nil
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return nil
	}
	return []providerpkg.Definition{def}
}

func providerSSHKeyExists(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, publicKey string) (bool, error) {
	return ctr.ProviderClient.SSHKeyExists(ctx, def, token, publicKey)
}

func uploadProviderSSHKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, title, publicKey string) error {
	return ctr.ProviderClient.UploadSSHKey(ctx, def, token, title, publicKey)
}

func providerSSHKeyAlreadyInUse(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "key is already in use") ||
		strings.Contains(message, "already in use") ||
		strings.Contains(message, "has already been taken") ||
		(strings.Contains(message, "fingerprint") && strings.Contains(message, "already"))
}

func printProviderSSHKeyAlreadyInUse(profileName string, def providerpkg.Definition) {
	ui.Warning("This SSH key is already registered on %s, but not on the authenticated account.", def.DisplayName)
	ui.Print("  %s only allows one owner for each SSH public key.", def.DisplayName)
	ui.Print("  Remove the key from the other account/repository, or generate a fresh key for this profile:")
	ui.Print("    gcm ssh generate %s --overwrite", profileName)
	ui.Print("    gcm ssh upload %s --provider %s", profileName, def.ID)
}

func deleteProviderSSHKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, publicKey string) (bool, error) {
	return ctr.ProviderClient.DeleteSSHKey(ctx, def, token, publicKey)
}

func providerGPGKeyExists(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, keyID string) (bool, error) {
	return ctr.ProviderClient.GPGKeyExists(ctx, def, token, keyID)
}

func uploadProviderGPGKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, armoredKey string) error {
	return ctr.ProviderClient.UploadGPGKey(ctx, def, token, armoredKey)
}

func deleteProviderGPGKey(ctx context.Context, def providerpkg.Definition, token providerpkg.TokenSet, keyID string) (bool, error) {
	return ctr.ProviderClient.DeleteGPGKey(ctx, def, token, keyID)
}

func providerResourceName(profileName string, def providerpkg.Definition, parts ...string) string {
	components := []string{"gcm", safeProviderNameComponent(profileName), safeProviderNameComponent(string(def.ID))}
	for _, part := range parts {
		if cleaned := safeProviderNameComponent(part); cleaned != "" {
			components = append(components, cleaned)
		}
	}
	return strings.Join(components, "-")
}

func safeProviderNameComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sshKeyProfileName(profileName string, p *profile.Profile) string {
	id, ok := profileProviderID(p)
	if !ok {
		return profileName
	}
	suffix := safeProviderNameComponent(string(id))
	if suffix == "" {
		return profileName
	}
	return fmt.Sprintf("%s_%s", profileName, suffix)
}

func migrateProfileSSHKeyPathToProvider(profileName string, p *profile.Profile) (bool, error) {
	if p == nil || p.SSH == nil || p.SSH.KeyPath == "" {
		return false, nil
	}

	targetPriv, ok := providerSSHKeyMigrationTarget(profileName, p)
	if !ok {
		return false, nil
	}

	currentPriv := p.SSH.KeyPath
	if _, err := os.Stat(targetPriv); err == nil {
		return false, fmt.Errorf("target SSH key already exists: %s", targetPriv)
	}

	currentPub := currentPriv + ".pub"
	targetPub := targetPriv + ".pub"

	if err := os.Rename(currentPriv, targetPriv); err != nil {
		return false, fmt.Errorf("renaming SSH key: %w", err)
	}

	pubExists := false
	if _, err := os.Stat(currentPub); err == nil {
		pubExists = true
		if err := os.Rename(currentPub, targetPub); err != nil {
			_ = os.Rename(targetPriv, currentPriv)
			return false, fmt.Errorf("renaming SSH public key: %w", err)
		}
	}

	originalPath := p.SSH.KeyPath
	p.SSH.KeyPath = targetPriv
	if err := ctr.ProfileManager.Update(p); err != nil {
		if pubExists {
			_ = os.Rename(targetPub, currentPub)
		}
		_ = os.Rename(targetPriv, currentPriv)
		p.SSH.KeyPath = originalPath
		return false, fmt.Errorf("updating profile after SSH key rename: %w", err)
	}

	return true, nil
}

func providerSSHKeyMigrationTarget(profileName string, p *profile.Profile) (string, bool) {
	if p == nil || p.SSH == nil || p.SSH.KeyPath == "" {
		return "", false
	}

	targetProfileName := sshKeyProfileName(profileName, p)
	keyType := string(p.SSH.KeyType)
	if keyType == "" {
		keyType = inferSSHKeyTypeFromPath(p.SSH.KeyPath)
	}
	if keyType == "" {
		return "", false
	}

	currentPriv := p.SSH.KeyPath
	currentName := filepath.Base(currentPriv)
	legacyName := fmt.Sprintf("id_%s_%s", keyType, profileName)
	legacyProviderPrefix := legacyName + "_"
	targetName := fmt.Sprintf("id_%s_%s", keyType, targetProfileName)
	if currentName == targetName {
		return "", false
	}
	if currentName != legacyName && !strings.HasPrefix(currentName, legacyProviderPrefix) {
		return "", false
	}

	targetPriv := filepath.Join(filepath.Dir(currentPriv), targetName)
	if targetPriv == currentPriv {
		return "", false
	}
	return targetPriv, true
}

func inferSSHKeyTypeFromPath(keyPath string) string {
	base := filepath.Base(strings.TrimSpace(keyPath))
	if !strings.HasPrefix(base, "id_") {
		return ""
	}
	rest := strings.TrimPrefix(base, "id_")
	idx := strings.Index(rest, "_")
	if idx <= 0 {
		return ""
	}
	return rest[:idx]
}

func providerManualKeyURL(def providerpkg.Definition, kind string) string {
	webURL := strings.TrimRight(def.WebURL, "/")
	if webURL == "" {
		webURL = strings.TrimRight(def.CredentialServer(), "/")
	}
	switch def.ID {
	case providerpkg.GitHubID:
		return webURL + "/settings/keys"
	case providerpkg.GitLabID:
		if kind == "gpg" {
			return webURL + "/-/user_settings/gpg_keys"
		}
		return webURL + "/-/user_settings/ssh_keys"
	default:
		return webURL
	}
}
