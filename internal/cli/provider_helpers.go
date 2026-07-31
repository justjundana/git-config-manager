package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_ui "github.com/justjundana/git-config-manager/pkg/ui"
)

func providerAccountForProfile(p *_profile.Profile, id _provider.ProviderID) _profile.ProviderAccountConfig {
	return _profile.ProviderAccount(p, id)
}

func setProfileProviderAccount(p *_profile.Profile, id _provider.ProviderID, username, authMethod string) {
	_profile.SetProviderAccount(p, id, username, authMethod)
}

type providerTransitionOptions struct {
	AllowPrompt bool
	AutoConfirm bool
}

func applyProfileProviderTransition(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition, username, authMethod string, allowPrompt bool, afterSet func() error) (bool, error) {
	return applyProfileProviderTransitionWithOptions(ctx, profileName, p, def, username, authMethod, providerTransitionOptions{AllowPrompt: allowPrompt}, afterSet)
}

func applyProfileProviderTransitionWithOptions(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition, username, authMethod string, opts providerTransitionOptions, afterSet func() error) (bool, error) {
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
		_ui.Warning("Could not rename SSH key to provider format: %v", err)
	} else if migrated {
		_ui.Detail("SSH Key Renamed", p.SSH.KeyPath)
	}

	return true, nil
}

func confirmProviderTransition(profileName string, oldDefs []_provider.Definition, newDef _provider.Definition) (bool, error) {
	_ui.Warning("Changing provider for profile %q: %s → %s", profileName, providerNames(oldDefs), newDef.DisplayName)
	_ui.Print("  GCM will clean old provider data before this profile uses %s:", newDef.DisplayName)
	_ui.Print("  - stored provider token(s)")
	_ui.Print("  - cached git credentials and credential username")
	_ui.Print("  - uploaded SSH/GPG keys on the old provider when the old token can access them")
	_ui.Print("  - local SSH key filename will be renamed to the new provider format")
	return _ui.AskConfirm("Continue and clean old provider data?", false)
}

func providerDefinitionsToClean(p *_profile.Profile, keep _provider.ProviderID) []_provider.Definition {
	if p == nil || ctr == nil || ctr.ProviderRegistry == nil {
		return nil
	}
	seen := make(map[_provider.ProviderID]bool)
	var defs []_provider.Definition
	add := func(id _provider.ProviderID) {
		if id == "" || id == keep || seen[id] {
			return
		}
		def, ok := ctr.ProviderRegistry.Get(id)
		if !ok || !def.Capabilities.Has(_provider.CapabilityCredentialHelper) {
			return
		}
		seen[id] = true
		defs = append(defs, def)
	}
	for id := range p.Providers {
		add(_provider.ProviderID(id))
	}
	if p.GitHub != nil {
		add(_provider.GitHubID)
	}
	return defs
}

func cleanupProviderData(ctx context.Context, profileName string, p *_profile.Profile, defs []_provider.Definition) {
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
			if sshPubKey != "" && def.Capabilities.Has(_provider.CapabilitySSHKeys) {
				if deleted, delErr := deleteProviderSSHKey(ctx, def, token, sshPubKey); delErr != nil {
					_ui.Warning("Could not delete SSH key from %s: %v", def.DisplayName, delErr)
				} else if deleted {
					_ui.Success("SSH key removed from %s", def.DisplayName)
				}
			}
			if p.GPG != nil && p.GPG.KeyID != "" && def.Capabilities.Has(_provider.CapabilityGPGKeys) {
				if deleted, delErr := deleteProviderGPGKey(ctx, def, token, p.GPG.KeyID); delErr != nil {
					_ui.Warning("Could not delete GPG key from %s: %v", def.DisplayName, delErr)
				} else if deleted {
					_ui.Success("GPG key removed from %s", def.DisplayName)
				}
			}
		}

		removedToken := false
		if delErr := deleteProviderToken(profileName, def, p); delErr == nil {
			removedToken = true
		}
		if def.ID == _provider.GitHubID {
			if delErr := ctr.GitHubClient.DeleteToken(profileName); delErr == nil {
				removedToken = true
			}
		}
		if removedToken {
			_ui.Success("%s token removed", def.DisplayName)
		}

		_ = ctr.GitHubClient.ClearGitCredentials(def.CredentialServer())
		_ = ctr.GitHubClient.SetGitCredentialUsername(def.CredentialServer(), "")
	}
}

func cloneProfileProviderState(p *_profile.Profile) *_profile.Profile {
	if p == nil {
		return nil
	}
	clone := *p
	if p.Providers != nil {
		clone.Providers = make(map[string]_profile.ProviderAccountConfig, len(p.Providers))
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

func restoreProfileProviderState(p *_profile.Profile, snapshot *_profile.Profile) {
	if p == nil || snapshot == nil {
		return
	}
	restored := cloneProfileProviderState(snapshot)
	p.Providers = restored.Providers
	p.GitHub = restored.GitHub
	p.SSH = restored.SSH
	p.GPG = restored.GPG
}

func providerNames(defs []_provider.Definition) string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.DisplayName)
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func profileProviderID(p *_profile.Profile) (_provider.ProviderID, bool) {
	return _profile.ProviderID(p)
}

func profileProviderDefinition(p *_profile.Profile, capability _provider.Capability) (_provider.Definition, bool) {
	id, ok := profileProviderID(p)
	if !ok || ctr == nil || ctr.ProviderRegistry == nil {
		return _provider.Definition{}, false
	}
	def, ok := ctr.ProviderRegistry.Get(id)
	if !ok || !def.Capabilities.Has(capability) {
		return _provider.Definition{}, false
	}
	return def, true
}

func profileUsesProvider(p *_profile.Profile, id _provider.ProviderID) bool {
	return _profile.UsesProvider(p, id)
}

func profileHasMultipleProviders(p *_profile.Profile) bool {
	return _profile.HasMultipleProviders(p)
}

func providerTokenKey(profileName string, def _provider.Definition, account _profile.ProviderAccountConfig) _provider.TokenKey {
	return _provider.TokenKey{
		Profile:  profileName,
		Provider: def.ID,
		Host:     firstProviderHost(def),
		Account:  account.Account,
	}
}

func firstProviderHost(def _provider.Definition) string {
	if len(def.GitHosts) > 0 && def.GitHosts[0] != "" {
		return _provider.NormalizeHost(def.GitHosts[0])
	}
	if def.WebURL != "" {
		return _provider.NormalizeHost(def.WebURL)
	}
	return _provider.NormalizeHost(def.APIURL)
}

func loadProviderToken(profileName string, def _provider.Definition, p *_profile.Profile) (_provider.TokenSet, error) {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.LoadTokenSet(providerTokenKey(profileName, def, account))
}

func saveProviderToken(profileName string, def _provider.Definition, p *_profile.Profile, token _provider.TokenSet) error {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.SaveTokenSet(providerTokenKey(profileName, def, account), token)
}

func deleteProviderToken(profileName string, def _provider.Definition, p *_profile.Profile) error {
	account := providerAccountForProfile(p, def.ID)
	return ctr.TokenStore.DeleteTokenSet(providerTokenKey(profileName, def, account))
}

func providerTokenPresent(profileName string, def _provider.Definition, p *_profile.Profile) bool {
	token, err := loadProviderToken(profileName, def, p)
	return err == nil && token.AccessToken != ""
}

func configureGitCredentialsForProvider(profileName string, p *_profile.Profile, def _provider.Definition, token _provider.TokenSet) {
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

func clearGitCredentialsForOtherProviders(active _provider.Definition) {
	if ctr == nil || ctr.ProviderRegistry == nil {
		return
	}
	for _, def := range ctr.ProviderRegistry.All() {
		if def.ID == active.ID || !def.Capabilities.Has(_provider.CapabilityCredentialHelper) {
			continue
		}
		_ = ctr.GitHubClient.ClearGitCredentials(def.CredentialServer())
	}
}

func clearProfileProviderAccount(p *_profile.Profile, id _provider.ProviderID) {
	_profile.ClearProviderAccount(p, id)
}

func clearAllProfileProviderAccounts(p *_profile.Profile) {
	_profile.ClearProviderAccounts(p)
}

func providerDefinitionsWithCapability(capability _provider.Capability) []_provider.Definition {
	if ctr == nil || ctr.ProviderRegistry == nil {
		return nil
	}
	var defs []_provider.Definition
	for _, def := range ctr.ProviderRegistry.All() {
		if def.Capabilities.Has(capability) {
			defs = append(defs, def)
		}
	}
	return defs
}

func selectProfileProviderWithCapability(profileName string, p *_profile.Profile, requested string, capability _provider.Capability) (_provider.Definition, error) {
	def, ok := profileProviderDefinition(p, capability)
	if !ok {
		return _provider.Definition{}, fmt.Errorf("profile %q has no provider for this operation; set one with: gcm profile edit %s -i", profileName, profileName)
	}
	if requested == "" {
		return def, nil
	}
	requestedID := normalizeProviderSelection(requested)
	if requestedID != def.ID {
		return _provider.Definition{}, fmt.Errorf("profile %q is configured for %s, not %s", profileName, def.DisplayName, requested)
	}
	return def, nil
}

func requireProfileProvider(profileName string, p *_profile.Profile, def _provider.Definition) error {
	if profileHasMultipleProviders(p) {
		return fmt.Errorf("profile %q has multiple providers configured; choose exactly one with: gcm profile edit %s -i", profileName, profileName)
	}
	if !profileUsesProvider(p, def.ID) {
		return fmt.Errorf("profile %q is not configured for %s; run: gcm connect %s --provider %s", profileName, def.DisplayName, profileName, def.ID)
	}
	return nil
}

func normalizeProviderSelection(value string) _provider.ProviderID {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gh", "github":
		return _provider.GitHubID
	case "gl", "gitlab":
		return _provider.GitLabID
	case "bb", "bitbucket":
		return _provider.BitbucketID
	default:
		return _provider.ProviderID(strings.ToLower(strings.TrimSpace(value)))
	}
}

func providerOption(def _provider.Definition) string {
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
	def, ok := profileProviderDefinition(p, _provider.CapabilitySSHKeys)
	if !ok {
		return
	}
	setupUploadKeysForProvider(ctx, profileName, p, def)
}

func setupUploadKeysForProvider(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition) {
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
			_ui.Blank()
		}
		setupGPGKeyUploadForProvider(ctx, profileName, p, def, p.GPG.KeyID)
	}
}

func setupSSHKeyUploadForProvider(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition, publicKey, keyType string) bool {
	if p == nil || !def.UploadKeys || publicKey == "" {
		return false
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return false
	}
	exists, checkErr := providerSSHKeyExists(ctx, def, token, publicKey)
	if checkErr == nil && exists {
		_ui.Blank()
		_ui.Success("SSH key already on %s", def.DisplayName)
		return false
	}
	if checkErr != nil {
		return false
	}

	_ui.Blank()
	upload, askErr := _ui.AskConfirm(fmt.Sprintf("Upload SSH key to %s?", def.DisplayName), true)
	if askErr != nil || !upload {
		return false
	}
	title := providerResourceName(profileName, def, "ssh", keyType)
	if uploadErr := uploadProviderSSHKey(ctx, def, token, title, publicKey); uploadErr != nil {
		if providerSSHKeyAlreadyInUse(uploadErr) {
			printProviderSSHKeyAlreadyInUse(profileName, def)
			return false
		}
		_ui.Warning("Could not upload SSH key to %s: %v", def.DisplayName, uploadErr)
		return false
	}
	_ui.Success("SSH key uploaded to %s", def.DisplayName)
	_ui.Detail("Title", title)
	return true
}

func setupGPGKeyUploadForProvider(ctx context.Context, profileName string, p *_profile.Profile, def _provider.Definition, keyID string) bool {
	if p == nil || !def.UploadKeys || keyID == "" {
		return false
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return false
	}
	exists, checkErr := providerGPGKeyExists(ctx, def, token, keyID)
	if checkErr == nil && exists {
		_ui.Success("GPG key already on %s", def.DisplayName)
		return false
	}
	if checkErr != nil {
		return false
	}

	upload, askErr := _ui.AskConfirm(fmt.Sprintf("Upload GPG key to %s?", def.DisplayName), true)
	if askErr != nil || !upload {
		return false
	}
	pubKey, gpgErr := ctr.GPGManager.GetPublicKey(keyID)
	if gpgErr != nil {
		_ui.Warning("Could not read GPG public key: %v", gpgErr)
		return false
	}
	if uploadErr := uploadProviderGPGKey(ctx, def, token, pubKey); uploadErr != nil {
		_ui.Warning("Could not upload GPG key to %s: %v", def.DisplayName, uploadErr)
		return false
	}
	_ui.Success("GPG key uploaded to %s", def.DisplayName)
	return true
}

func authenticatedProvidersForProfile(profileName string, p *_profile.Profile, capability _provider.Capability) []_provider.Definition {
	def, ok := profileProviderDefinition(p, capability)
	if !ok {
		return nil
	}
	token, err := loadProviderToken(profileName, def, p)
	if err != nil || token.AccessToken == "" {
		return nil
	}
	return []_provider.Definition{def}
}

func providerSSHKeyExists(ctx context.Context, def _provider.Definition, token _provider.TokenSet, publicKey string) (bool, error) {
	return ctr.ProviderClient.SSHKeyExists(ctx, def, token, publicKey)
}

func uploadProviderSSHKey(ctx context.Context, def _provider.Definition, token _provider.TokenSet, title, publicKey string) error {
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

func printProviderSSHKeyAlreadyInUse(profileName string, def _provider.Definition) {
	_ui.Warning("This SSH key is already registered on %s, but not on the authenticated account.", def.DisplayName)
	_ui.Print("  %s only allows one owner for each SSH public key.", def.DisplayName)
	_ui.Print("  Remove the key from the other account/repository, or generate a fresh key for this profile:")
	_ui.Print("    gcm ssh generate %s --overwrite", profileName)
	_ui.Print("    gcm ssh upload %s --provider %s", profileName, def.ID)
}

func deleteProviderSSHKey(ctx context.Context, def _provider.Definition, token _provider.TokenSet, publicKey string) (bool, error) {
	return ctr.ProviderClient.DeleteSSHKey(ctx, def, token, publicKey)
}

func providerGPGKeyExists(ctx context.Context, def _provider.Definition, token _provider.TokenSet, keyID string) (bool, error) {
	return ctr.ProviderClient.GPGKeyExists(ctx, def, token, keyID)
}

func uploadProviderGPGKey(ctx context.Context, def _provider.Definition, token _provider.TokenSet, armoredKey string) error {
	return ctr.ProviderClient.UploadGPGKey(ctx, def, token, armoredKey)
}

func deleteProviderGPGKey(ctx context.Context, def _provider.Definition, token _provider.TokenSet, keyID string) (bool, error) {
	return ctr.ProviderClient.DeleteGPGKey(ctx, def, token, keyID)
}

func providerResourceName(profileName string, def _provider.Definition, parts ...string) string {
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

func sshKeyProfileName(profileName string, p *_profile.Profile) string {
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

func migrateProfileSSHKeyPathToProvider(profileName string, p *_profile.Profile) (bool, error) {
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

	// Carry the generated-keys provenance across to the new path. A stale
	// entry would make the renamed key look like it was never GCM's, and would
	// leave the old path marked as GCM-generated — so anything the user later
	// puts there becomes eligible for deletion by "gcm ssh clean".
	//
	// The files and the profile already agree at this point, so a ledger
	// failure is reported rather than rolled back: "gcm repair --fix" can
	// reconcile it, whereas undoing the rename would be the larger surprise.
	if err := ctr.KeyLedger.RenameSSH(currentPriv, targetPriv); err != nil {
		_ui.Warning("SSH key renamed, but the generated-keys ledger could not be updated: %v", err)
		_ui.Info("  Run %s to reconcile it", _ui.Cyan("gcm repair --fix"))
	}

	return true, nil
}

func providerSSHKeyMigrationTarget(profileName string, p *_profile.Profile) (string, bool) {
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

func providerManualKeyURL(def _provider.Definition, kind string) string {
	webURL := strings.TrimRight(def.WebURL, "/")
	if webURL == "" {
		webURL = strings.TrimRight(def.CredentialServer(), "/")
	}
	switch def.ID {
	case _provider.GitHubID:
		return webURL + "/settings/keys"
	case _provider.GitLabID:
		if kind == "gpg" {
			return webURL + "/-/user_settings/gpg_keys"
		}
		return webURL + "/-/user_settings/ssh_keys"
	default:
		return webURL
	}
}
