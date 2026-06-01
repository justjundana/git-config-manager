// Package container provides dependency injection for GCM.
package container

import (
	"github.com/justjundana/git-config-manager/internal/audit"
	"github.com/justjundana/git-config-manager/internal/backup"
	"github.com/justjundana/git-config-manager/internal/config"
	"github.com/justjundana/git-config-manager/internal/github"
	"github.com/justjundana/git-config-manager/internal/gitlab"
	"github.com/justjundana/git-config-manager/internal/gpg"
	"github.com/justjundana/git-config-manager/internal/profile"
	providerpkg "github.com/justjundana/git-config-manager/internal/provider"
	"github.com/justjundana/git-config-manager/internal/providerclient"
	cryptoSvc "github.com/justjundana/git-config-manager/internal/service/crypto"
	fileSvc "github.com/justjundana/git-config-manager/internal/service/file"
	"github.com/justjundana/git-config-manager/internal/shell"
	"github.com/justjundana/git-config-manager/internal/ssh"
	"github.com/justjundana/git-config-manager/internal/template"
	"github.com/justjundana/git-config-manager/internal/tokenstore"
	"github.com/justjundana/git-config-manager/pkg/logger"
)

// Container holds all application dependencies.
type Container struct {
	Config           *config.Config
	Logger           *logger.Logger
	AuditLogger      *audit.Logger
	FileService      *fileSvc.Service
	CryptoService    *cryptoSvc.Service
	ProfileManager   *profile.Manager
	ProfileSwitcher  *profile.Switcher
	SSHManager       *ssh.Manager
	GPGManager       *gpg.Manager
	GitHubClient     *github.Client
	GitLabClient     *gitlab.Client
	ProviderClient   *providerclient.Router
	ProviderRegistry *providerpkg.Registry
	TokenStore       *tokenstore.TokenStore
	ShellManager     *shell.Manager
	TemplateManager  *template.Manager
	BackupManager    *backup.Manager
}

// SetMasterPasswordPrompt injects the callback used to ask the user for a
// master password when encrypted-file token storage is active. This must be
// called before any Save/Load operation that requires a master password.
func (c *Container) SetMasterPasswordPrompt(fn tokenstore.PromptFunc) {
	c.TokenStore.SetPromptFunc(fn)
}

// New creates a fully-wired Container from the loaded configuration.
func New(cfg *config.Config, log *logger.Logger) *Container {
	fs := fileSvc.NewService()
	crypto := cryptoSvc.NewService()
	auditLog := audit.NewLogger(cfg)

	tokenStore := tokenstore.NewTokenStore(cfg, crypto, log, nil)
	registry := providerpkg.NewRegistry(cfg)
	githubClientCfg := *cfg
	if githubDef, ok := registry.Get(providerpkg.GitHubID); ok && githubDef.APIURL != "" {
		githubClientCfg.GitHub.APIURL = githubDef.APIURL
		githubClientCfg.GitHub.UploadKeys = githubDef.UploadKeys
		if len(githubDef.Scopes) > 0 {
			githubClientCfg.GitHub.OAuth.Scopes = append([]string(nil), githubDef.Scopes...)
		}
	}
	ghClient := github.NewClient(&githubClientCfg, log, tokenStore)
	gitlabCfg := cfg.Providers["gitlab"]
	if gitlabCfg.APIURL == "" {
		gitlabCfg = config.ProviderConfig{
			Type:       "gitlab",
			APIURL:     "https://gitlab.com/api/v4",
			WebURL:     "https://gitlab.com",
			GitHosts:   []string{"gitlab.com"},
			SSHHost:    "gitlab.com",
			UploadKeys: true,
		}
	}
	glClient := gitlab.NewClient(gitlabCfg, log)
	providerClient := providerclient.NewRouter(ghClient, glClient)

	pm := profile.NewManager(cfg, fs, log)
	ps := profile.NewSwitcher(cfg, pm, log)
	sshMgr := ssh.NewManager(cfg, log)
	gpgMgr := gpg.NewManager(cfg, log)
	shellMgr := shell.NewManager(log)
	tmplMgr := template.NewManager(cfg, fs, log)
	bkpMgr := backup.NewManager(cfg, log)

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
