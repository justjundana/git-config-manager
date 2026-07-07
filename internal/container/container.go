// Package container provides dependency injection for GCM.
package container

import (
	_audit "github.com/justjundana/git-config-manager/internal/audit"
	_backup "github.com/justjundana/git-config-manager/internal/backup"
	_config "github.com/justjundana/git-config-manager/internal/config"
	_github "github.com/justjundana/git-config-manager/internal/github"
	_gitlab "github.com/justjundana/git-config-manager/internal/gitlab"
	_gpg "github.com/justjundana/git-config-manager/internal/gpg"
	_keyledger "github.com/justjundana/git-config-manager/internal/keyledger"
	_profile "github.com/justjundana/git-config-manager/internal/profile"
	_provider "github.com/justjundana/git-config-manager/internal/provider"
	_providerclient "github.com/justjundana/git-config-manager/internal/providerclient"
	_crypto "github.com/justjundana/git-config-manager/internal/service/crypto"
	_file "github.com/justjundana/git-config-manager/internal/service/file"
	_shell "github.com/justjundana/git-config-manager/internal/shell"
	_ssh "github.com/justjundana/git-config-manager/internal/ssh"
	_template "github.com/justjundana/git-config-manager/internal/template"
	_tokenstore "github.com/justjundana/git-config-manager/internal/tokenstore"
	_logger "github.com/justjundana/git-config-manager/pkg/logger"
)

// Container holds all application dependencies.
type Container struct {
	Config           *_config.Config
	Logger           *_logger.Logger
	AuditLogger      *_audit.Logger
	FileService      *_file.Service
	CryptoService    *_crypto.Service
	ProfileManager   *_profile.Manager
	ProfileSwitcher  *_profile.Switcher
	SSHManager       *_ssh.Manager
	GPGManager       *_gpg.Manager
	KeyLedger        *_keyledger.Ledger
	GitHubClient     *_github.Client
	GitLabClient     *_gitlab.Client
	ProviderClient   *_providerclient.Router
	ProviderRegistry *_provider.Registry
	TokenStore       *_tokenstore.TokenStore
	ShellManager     *_shell.Manager
	TemplateManager  *_template.Manager
	BackupManager    *_backup.Manager
}

// SetMasterPasswordPrompt injects the callback used to ask the user for a
// master password when encrypted-file token storage is active. This must be
// called before any Save/Load operation that requires a master password.
func (c *Container) SetMasterPasswordPrompt(fn _tokenstore.PromptFunc) {
	c.TokenStore.SetPromptFunc(fn)
}

// New creates a fully-wired Container from the loaded configuration.
func New(cfg *_config.Config, log *_logger.Logger) *Container {
	fs := _file.NewService()
	crypto := _crypto.NewService()
	auditLog := _audit.NewLogger(cfg)

	tokenStore := _tokenstore.NewTokenStore(cfg, crypto, log, nil)
	registry := _provider.NewRegistry(cfg)
	githubClientCfg := *cfg
	if githubDef, ok := registry.Get(_provider.GitHubID); ok && githubDef.APIURL != "" {
		githubClientCfg.GitHub.APIURL = githubDef.APIURL
		githubClientCfg.GitHub.UploadKeys = githubDef.UploadKeys
		if len(githubDef.Scopes) > 0 {
			githubClientCfg.GitHub.OAuth.Scopes = append([]string(nil), githubDef.Scopes...)
		}
	}
	ghClient := _github.NewClient(&githubClientCfg, log, tokenStore)
	gitlabCfg := cfg.Providers["gitlab"]
	if gitlabCfg.APIURL == "" {
		gitlabCfg = _config.ProviderConfig{
			Type:       "gitlab",
			APIURL:     "https://gitlab.com/api/v4",
			WebURL:     "https://gitlab.com",
			GitHosts:   []string{"gitlab.com"},
			SSHHost:    "gitlab.com",
			UploadKeys: true,
		}
	}
	glClient := _gitlab.NewClient(gitlabCfg, log)
	providerClient := _providerclient.NewRouter(ghClient, glClient)

	pm := _profile.NewManager(cfg, fs, log)
	ps := _profile.NewSwitcher(cfg, pm, log)
	sshMgr := _ssh.NewManager(cfg, log)
	gpgMgr := _gpg.NewManager(cfg, log)
	keyLedger := _keyledger.New()
	shellMgr := _shell.NewManager(log)
	tmplMgr := _template.NewManager(cfg, fs, log)
	bkpMgr := _backup.NewManager(cfg, log)

	return &Container{
		Config:           cfg,
		Logger:           log,
		AuditLogger:      auditLog,
		FileService:      fs,
		CryptoService:    crypto,
		ProfileManager:   pm,
		ProfileSwitcher:  ps,
		SSHManager:       sshMgr,
		GPGManager:       gpgMgr,
		KeyLedger:        keyLedger,
		GitHubClient:     ghClient,
		GitLabClient:     glClient,
		ProviderClient:   providerClient,
		ProviderRegistry: registry,
		TokenStore:       tokenStore,
		ShellManager:     shellMgr,
		TemplateManager:  tmplMgr,
		BackupManager:    bkpMgr,
	}
}
