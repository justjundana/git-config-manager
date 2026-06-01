# Git Config Manager (GCM)

<p align="center">
  <strong>Manage your Git identities with ease</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#profile-naming">Profile Naming</a> •
  <a href="#commands">Commands</a> •
  <a href="#documentation">Documentation</a>
</p>

---

**GCM** is a CLI tool for managing multiple provider-scoped Git identities, SSH keys, GPG keys, and GitHub/GitLab accounts. Switch between work, personal, and client profiles instantly.

## Features

- 🎯 **Profile Management** — Create, edit, and switch between Git identities
- 🔑 **SSH Key Management** — Generate and manage SSH keys per profile (auto-upload to the profile provider)
- 🔐 **GPG Signing** — Generate GPG keys and enable commit signing (auto-upload to the profile provider)
- 🐙 **GitHub Integration** — OAuth login, token verification, SSH/GPG key upload via API
- 🦊 **GitLab Integration** — PAT login, token verification, SSH/GPG key upload via API
- 🛡️ **Credential Isolation** — Git credentials are pinned per profile; no bleed between accounts. Built-in credential helper is immune to external logout (VS Code, browser, etc.)
- 🔎 **Auth Ownership Inspection** — Distinguish GCM-owned tokens from external Git credentials, adopt intentionally, and log out safely
- 🐚 **Shell Integration** — Auto-switch profiles on `cd` (bash, zsh, fish, powershell)
- 📋 **Templates** — Share configuration standards across teams
- 💾 **Backup & Restore** — Protect your configuration data
- 🏥 **Diagnostics** — Health checks and profile validation
- 🔒 **Security** — AES-256-GCM encrypted token storage, audit logging
- 🌍 **Cross-Platform** — macOS, Linux, Windows

## Demo & Preview

<details>
<summary><strong>Individual Feature Demos</strong> (click to expand)</summary>

| Feature | Demo |
|---------|------|
| Version & Help | ![Version](docs/demos/01-version-overview.gif) |
| Create Profiles | ![Create](docs/demos/02-create-git-profiles.gif) |
| List & Inspect | ![List](docs/demos/03-list-inspect-profiles.gif) |
| Switch Profiles | ![Switch](docs/demos/04-switch-between-profiles.gif) |
| SSH Keys | ![SSH](docs/demos/05-ssh-key-management.gif) |
| GPG Signing | ![GPG](docs/demos/06-gpg-commit-signing.gif) |
| Provider Commands | ![Provider](docs/demos/07-provider-integrations.gif) |
| Auth Ownership | ![Auth Ownership](docs/demos/08-auth-ownership-sources.gif) |
| Validate | ![Validate](docs/demos/09-validate-profiles.gif) |
| Doctor | ![Doctor](docs/demos/10-system-health-check.gif) |
| Status | ![Status](docs/demos/11-status-dashboard.gif) |
| Templates | ![Templates](docs/demos/12-configuration-templates.gif) |
| Backup & Restore | ![Backup](docs/demos/13-backup-restore.gif) |
| Shell Integration | ![Shell](docs/demos/14-shell-integration.gif) |
| Export & Diff | ![Export](docs/demos/15-profile-export-diff.gif) |

</details>

## Installation

### One-Line Installation
```bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash
```

The installer downloads the release checksum manifest (`checksums.txt`), verifies the binary checksum, and installs only the `gcm` binary by default. It does not edit your shell files or run `gcm init` unless you explicitly opt in:

```bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash -s -- --add-to-path
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash -s -- --add-to-path --init
```

### From Source
```bash
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager
make build          # produces ./bin/gcm
make install        # installs to $(go env GOPATH)/bin/gcm (no sudo needed)
# or: make install-system   # installs to /usr/local/bin/gcm (needs sudo)
```

## Quick Start

```bash
# Option A: Guided wizard (recommended for first-time users)
gcm setup

# Option B: Step by step
gcm profile create work --interactive   # 1. Create a profile
gcm use work                            # 2. Activate it
gcm init                                # 3. Optional shell integration (auto-switch on cd)
gcm doctor                              # 4. Verify everything works
```

## Profile Naming

`work` is not a reserved word — the name after `gcm profile create` is a free-form identifier you pick yourself. Valid names start with a letter or digit, and may contain letters, digits, `-`, or `_`. Names that differ only by case (e.g., `Work` and `work`) are treated as duplicates for cross-platform consistency.

Pick names that match your real identities:

```bash
# day job + personal
gcm profile create work     -i
gcm profile create personal -i

# freelancer with several clients
gcm profile create personal      -i
gcm profile create client-acme   -i
gcm profile create client-globex -i

# multiple GitHub orgs on one machine
gcm profile create gh-personal  -i
gcm profile create gh-tokopedia -i
gcm profile create gh-gojek     -i
```

Every command that takes a profile (`gcm use`, `gcm ssh generate`, `gcm connect`, …) takes whatever identifier you chose. See [`docs/usage.md`](docs/usage.md#53-profile-naming--what-should-i-call-them) for the full naming rules, rename recipe, and scenario examples.

## Commands

### Profile Management
```bash
gcm profile create <name> -i    # Interactive wizard
gcm profile list                # List all profiles
gcm profile show <name>         # Show profile details
gcm profile edit <name>         # Edit a profile
gcm profile delete <name>       # Delete a profile
gcm profile export <name>       # Export to YAML
gcm profile import <file>       # Import from YAML
gcm profile diff <a> <b>        # Compare two profiles
```

### Profile Activation
```bash
gcm use <name>                  # Smart: session (git repo) or local (elsewhere)
gcm use <name> --global         # Set as default (clears local overrides)
gcm use <name> --local          # Pin to current project
gcm use <name> --dry-run        # Preview changes
gcm current                     # Show active profile
gcm current --short --hide-default  # For shell prompts
gcm refresh                     # Re-evaluate current directory
```

> Switching profiles automatically isolates git credentials — `git push`/`clone` will only authenticate as the active profile's selected provider account.

### Provider Connection
```bash
gcm connect <profile> --provider github       # Provider-neutral PAT login
gcm connect <profile> --provider gitlab       # Connect to GitLab
gcm switch-provider <profile> gitlab          # Move a profile to GitLab with cleanup
echo "$TOKEN" | gcm connect work --provider gitlab --token-stdin --yes
```

One profile belongs to one provider. Provider changes clean old provider tokens, cached credentials, credential usernames, and uploaded keys when the old token can still access them.

### Authentication Ownership
```bash
gcm auth status [profile] --provider github --verbose  # Source-aware auth status
gcm auth inspect <profile> --provider gitlab            # Inspect GCM/external sources
gcm auth adopt <profile> --provider github --dry-run    # Preview adopting external auth
gcm auth logout <profile> --scope gcm                   # Remove GCM-owned token only
gcm auth logout <profile> --scope external --dry-run    # Preview external credential removal
gcm auth doctor [profile]                               # Diagnose auth ownership issues
gcm auth repair --dry-run                               # Preview safe auth repairs
```

GCM treats its encrypted token store as the source of truth for GCM-owned auth. External credentials from Keychain, Git Credential Manager, GitHub CLI, libsecret, or other helpers are detected and explained, but are only adopted or removed when you explicitly ask.

### SSH Keys
```bash
gcm ssh generate <profile>      # Generate SSH key (ed25519)
gcm ssh generate <profile> --overwrite  # Replace existing local key pair explicitly
gcm ssh upload <profile>        # Upload SSH key to the profile provider (dedup-safe)
gcm ssh list                    # List all SSH keys
gcm ssh test <profile>          # Test provider SSH connection
gcm ssh copy <profile>          # Show public key
```

If a deterministic provider-aware key file already exists from an old manual cleanup, for example `id_ed25519_personal_github`, `gcm ssh generate`, `gcm ssh upload`, `gcm ssh test`, and `gcm ssh copy` can link that existing key back to the profile instead of leaving the profile half-configured.

### GPG Signing
```bash
gcm gpg generate <profile>      # Generate GPG key
gcm gpg upload <profile>        # Upload GPG key to the profile provider (dedup-safe)
gcm gpg list                    # List GPG keys
gcm gpg sign enable <profile>   # Enable commit signing
gcm gpg sign disable <profile>  # Disable signing
gcm gpg test <profile>          # Test signing capability
```

### GitHub
```bash
gcm github login <profile>          # Login with Personal Access Token (PAT)
gcm github login-oauth <profile>    # OAuth device flow login (browser)
gcm github login-gh <profile>       # Import token from GitHub CLI (gh)
gcm github status                   # Source-aware auth status for GitHub profiles
gcm github logout <profile>         # Remove stored GitHub token
gcm github verify <profile>         # Verify authentication
gcm github user <profile>           # Show user info
```

### GitLab
```bash
gcm gitlab login <profile>      # Login with Personal Access Token (PAT)
gcm gitlab status               # Source-aware auth status for GitLab profiles
gcm gitlab logout <profile>     # Remove stored GitLab token
gcm gitlab verify <profile>     # Verify authentication
gcm gitlab user <profile>       # Show user info
```

### Diagnostics & Repair
```bash
gcm status                      # Dashboard with bounded provider auth checks
gcm doctor                      # System health check
gcm repair                      # Inspect local provider/profile consistency
gcm repair --fix                # Apply safe local repairs
gcm repair --json               # Machine-readable report
gcm auth doctor                 # Diagnose auth ownership/source issues
gcm auth repair --dry-run       # Preview safe auth helper repairs
```

### Shell Integration
```bash
gcm init                        # Install shell hooks
```

### Templates
```bash
gcm template create <name> -i   # Interactive template creation
gcm template list               # List templates
gcm template show <name>        # Show template details
gcm template apply <tpl> <prof> # Apply template to a profile
gcm template import <file>      # Import from YAML
gcm template export <name>      # Export to YAML
gcm template delete <name>      # Delete template
```

### Backup
```bash
gcm backup create               # Create backup
gcm backup list                 # List backups
gcm backup restore <file>       # Restore backup
gcm backup prune --keep 5       # Keep latest N backups
```

### Diagnostics
```bash
gcm validate [profile]          # Validate profile(s)
gcm doctor                      # System health check
gcm clean                       # Clean cache
gcm version                     # Show version info
```

## Shell Auto-Switch

After running `gcm init`, GCM will automatically switch profiles when you `cd` into a directory containing a `.gcm-profile` file:

```bash
# Pin a profile to a project
cd ~/projects/work-project
gcm use work --local

# Now every time you cd into this directory, GCM switches to "work"
```

## Security

- **SSH keys** are never stored by GCM — only file paths are managed
- **GPG keys** use the system keyring — only key IDs are stored
- **Provider tokens** are encrypted at rest using AES-256-GCM
- **Credential helper** — GCM serves credentials directly to git from its own encrypted store, immune to external credential changes (VS Code logout, Keychain edits, etc.)
- **Auth ownership** — `gcm auth` classifies GCM-owned vs external credentials before adoption or deletion
- **Audit logging** tracks all configuration changes
- File permissions are validated (600 for keys)

## Configuration

GCM stores its configuration in `~/.gcm/`:

```
~/.gcm/
├── config.yaml       # Global settings
├── profiles/         # Profile YAML files
├── templates/        # Configuration templates
├── tokens/           # Encrypted provider tokens
├── backups/          # Backup archives
├── logs/             # Audit logs
└── cache/            # Temporary cache
```

## Documentation

Full documentation is at [docs/index.md](docs/index.md).

### User Guide

- [Quick Start](docs/quick-start.md) — 5-minute setup
- [Installation](docs/installation.md) — all platforms
- [Requirements](docs/requirements.md) — system requirements, shell compatibility
- [Getting Started](docs/getting-started.md) — first-time walkthrough
- [Commands Reference](docs/commands.md) — every command, flag, and exit code
- [Interactive Guide](docs/interactive-guide.md) — every prompt and term explained
- [Configuration Reference](docs/configuration.md) — config.yaml, profile schema, templates
- [Shell Integration](docs/shell-integration.md) — auto-switch on `cd`, prompt indicators
- [Examples & Recipes](docs/examples.md) — real-world workflows
- [Migration Guide](docs/migration-guide.md) — migrate from manual config or other tools
- [Developer Onboarding](docs/developer-onboarding.md) — team adoption, CI/CD, checklists
- [FAQ](docs/faq.md) — frequently asked questions
- [Troubleshooting](docs/troubleshooting.md) — problem → solution
- [Upgrade & Uninstall](docs/upgrade-uninstall.md) — update or remove GCM
- [Complete Usage Guide](docs/usage.md) — end-to-end walkthrough

### Developer Guide

- [Architecture](docs/architecture.md) — design, patterns, component diagram
- [Project Structure](docs/project-structure.md) — file-by-file codebase map
- [Data Flow & Diagrams](docs/data-flow.md) — operation traces with mermaid diagrams
- [Security Model](docs/security.md) — threat model, encryption, permissions
- [Dependencies](docs/dependencies.md) — modules, rationale, dependency graph
- [Performance](docs/performance.md) — benchmarks and optimization
- [Versioning](docs/versioning.md) — SemVer policy, compatibility guarantees
- [Release Notes](docs/release-notes.md) — release history and upgrade paths
- [Glossary](docs/glossary.md) — term definitions
- [Contributing](docs/contributing.md) — development setup, coding standards, PR process

## Development

```bash
# Build
make build

# Test
make test

# Lint
make lint

# Cross-compile
make build-all

# Release
make release
```

## Requirements

- **Go** 1.26+ (for building from source)
- **Git** 2.20+
- **SSH client** (for SSH key management)
- **GPG** 2.0+ (optional, for commit signing)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.
