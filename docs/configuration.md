# Configuration Reference

GCM stores all data under `~/.gcm/`. This document covers the directory layout, the global `config.yaml`, and the profile YAML schema.

---

## Directory Layout

```
~/.gcm/
├── config.yaml           # Global GCM settings
├── profiles/             # One YAML file per profile
│   ├── work.yaml
│   └── personal.yaml
├── templates/            # YAML template blueprints
├── tokens/        (0700) # Encrypted provider tokens
├── backups/       (0700) # Timestamped unencrypted .tar.gz snapshots
├── logs/          (0700) # Audit logs (audit-YYYY-MM-DD.jsonl)
└── cache/                # Transient data (safe to delete)
```

Sensitive directories (`tokens/`, `backups/`, `logs/`) are created with `0700` permissions.

---

## Global Configuration (`config.yaml`)

GCM creates `~/.gcm/config.yaml` with sensible defaults on first run. Below is every field with its default value.

```yaml
# The profile to activate when no other is specified.
default_profile: ""

# Directory paths (relative to ~/.gcm/ by default).
profiles_dir:  ~/.gcm/profiles
templates_dir: ~/.gcm/templates
cache_dir:     ~/.gcm/cache
ssh_dir:       ~/.ssh
gpg_home:      ~/.gnupg

# Auto-switching behavior.
auto_switch:
  enabled: true                  # Enable auto-switch on cd
  project_file: ".gcm-profile"  # File name to look for
  detection_strategy: "project_file"  # Implemented profile detection strategy

# Reserved for future URL-pattern-to-profile mapping rules.
# Current shell auto-switching uses auto_switch.project_file.
detection_rules: []
# - pattern: "github.com/acme-corp/*"
#   profile: work
#   priority: 10

# Shell integration settings.
shell:
  integration: true              # Enable shell hooks
  prompt_indicator: true         # Show profile name in prompt
  prompt_format: "(%s)"          # Printf format for the indicator
  completion: true               # Enable tab completion
  auto_detect: true              # Auto-detect shell type

# Legacy GitHub integration block, still supported for backward compatibility.
github:
  api_url: "https://api.github.com"
  upload_keys: true              # Offer to upload SSH keys to GitHub
  oauth:
    client_id: "gcm-oauth-app"
    scopes:
      - repo
      - admin:public_key
      - admin:gpg_key

# Provider integrations. The legacy github block remains supported, but
# new integrations should use this provider map.
providers:
  github:
    type: github
    api_url: "https://api.github.com"
    web_url: "https://github.com"
    git_hosts:
      - github.com
    ssh_host: github.com
    upload_keys: true
    auth:
      default_method: pat
      scopes:
        - repo
        - admin:public_key
        - admin:gpg_key
  gitlab:
    type: gitlab
    api_url: "https://gitlab.com/api/v4"
    web_url: "https://gitlab.com"
    git_hosts:
      - gitlab.com
    ssh_host: gitlab.com
    upload_keys: true
    auth:
      default_method: pat
      scopes:
        - api
        - read_user
        - read_repository
        - write_repository

# Backup settings.
backup:
  auto_backup: false
  interval: "daily"
  retention_days: 30
  max_backups: 10
  include_keys: false            # SSH private key backups are disabled until encrypted backups are implemented
  encryption: false              # Encrypted backup archives are not implemented; true fails closed

# Security settings.
security:
  encrypt_tokens: true           # Encrypt provider tokens at rest
  use_keychain: true             # Use OS keychain when available
  master_password: false         # Require master password
  allow_plaintext_tokens: false  # Explicit unsafe fallback; disabled by default
  audit_log: true                # Enable audit logging

# UI settings.
ui:
  color: true
  emoji: true
  verbose: false
  quiet: false

# Advanced settings.
advanced:
  git_command: "git"
  ssh_command: "ssh"
  gpg_command: "gpg"
  parallel_operations: true
```

---

## Profile Schema

Profiles are stored as YAML in `~/.gcm/profiles/<name>.yaml`. Here is the complete schema with all fields.

```yaml
# Profile identifier (matches the filename without .yaml).
name: work

# Git configuration — applied via `git config` when the profile is activated.
git:
  user:
    name: "Jane Doe"                # user.name
    email: "jane@company.example"   # user.email
    signingkey: "0xABCD1234"        # user.signingkey (optional)

  core:
    editor: "code"                  # core.editor (optional)
    autocrlf: ""                    # core.autocrlf (optional)
    eol: ""                         # core.eol (optional)
    filemode: null                  # core.filemode (optional, bool)
    ignorecase: null                # core.ignorecase (optional, bool)
    precomposeunicode: null         # core.precomposeunicode (optional, bool)

  commit:
    gpgsign: true                   # commit.gpgsign (optional, bool)
    template: ""                    # commit.template (optional, path)
    verbose: null                   # commit.verbose (optional, bool)

  pull:
    rebase: ""                      # pull.rebase (optional)
    ff: ""                          # pull.ff (optional)

  push:
    default: ""                     # push.default (optional)
    followtags: null                # push.followTags (optional, bool)
    autosetupremote: null           # push.autoSetupRemote (optional, bool)

  aliases: {}                       # Git aliases (optional, map)
  custom: {}                        # Custom git config keys (optional, map)

# SSH key configuration (optional section).
ssh:
  key_path: "~/.ssh/id_ed25519_work"   # Path to private key
  key_type: "ed25519"                   # ed25519 | rsa | ecdsa
  comment: "work@laptop"               # Key comment (optional)
  fingerprint: "SHA256:..."            # Key fingerprint (optional, set on generation)
  load_to_agent: true                   # Load key into ssh-agent on activation (optional)

# GPG signing configuration (optional section).
gpg:
  key_id: "0xABCD1234"                 # GPG key ID
  program: ""                           # GPG program override (optional)
  format: ""                            # Signature format (optional)
  expires_at: null                      # Key expiration (optional, timestamp)

# GitHub account configuration (optional section).
github:
  username: "jane-acme"                # GitHub username
  token_path: ""                       # Token file path (optional, managed by GCM)
  upload_keys: true                    # Upload SSH/GPG keys to GitHub (optional)

# Provider account configuration (optional section). Choose exactly one provider
# per profile; create another profile for another provider account.
providers:
  github:
    username: "jane-acme"
    auth_method: "pat"
    upload_keys: true

# Lifecycle metadata (managed by GCM, not typically edited by hand).
metadata:
  created: "2026-01-15T10:30:00Z"
  updated: "2026-05-18T14:00:00Z"
  usage_count: 42
  last_used: "2026-05-18T14:00:00Z"
  version: "1.0.0"
```

### Required Fields

Only these fields are required to create a valid profile:

| Field            | Description              |
| ---------------- | ------------------------ |
| `name`           | Profile identifier       |
| `git.user.name`  | Git user name            |
| `git.user.email` | Git email address        |

All other fields are optional.

### Validation

GCM validates profiles on several levels:

- **Schema** — required fields present, valid types
- **SSH paths** — key file exists, correct permissions (0600)
- **GPG keys** — key ID exists in the GPG keyring
- **Providers** — provider tokens are present and decryptable

Run validation:

```bash
gcm validate           # all profiles
gcm validate work      # one profile
```

---

## Template Schema

Templates use the same structure as profiles but are stored in `~/.gcm/templates/` with additional metadata:

```yaml
name: company-standard
description: "Standard config for ACME Corp engineers"
metadata:
  version: "1.0"
  author: "Platform Team"
  created: "2026-01-01T00:00:00Z"

# Same git/ssh/gpg/github fields as a profile.
git:
  user:
    email: "{{.Email}}"    # placeholder (filled on profile creation)
  core:
    editor: code
  commit:
    gpgsign: true
ssh:
  key_type: ed25519
```

Manage templates:

```bash
gcm template list
gcm template show <name>
gcm template import <file.yaml>
gcm template export <name>
gcm template delete <name>
```

---

## Audit Log

When `security.audit_log` is enabled (default), GCM writes a JSONL audit log to `~/.gcm/logs/audit-YYYY-MM-DD.jsonl`. Each line is a JSON object:

```json
{
  "timestamp": "2026-05-18T14:30:00Z",
  "action": "profile.activate",
  "target": "work",
  "details": {"scope": "session"},
  "error": null
}
```

Actions logged include:
- `profile.create`, `profile.delete`, `profile.activate`
- `ssh.generate`
- `gpg.generate`
- `github.login`, `github.logout`
- `backup.create`, `backup.restore`
- `shell.init`

---

## Environment Variables

| Variable              | Description                                   |
| --------------------- | --------------------------------------------- |
| `_GCM_PROMPT`         | Shell variable set by precmd hook with active profile name (used in prompt) |
| `SHELL`               | Used by `gcm init` to detect your shell       |

---

## See Also

- [Commands Reference](commands.md) — every command and flag
- [Security Model](security.md) — token encryption and permission details
- [Examples](examples.md) — real-world configuration workflows
- [Shell Integration](shell-integration.md) — auto-switch setup
- [Troubleshooting](troubleshooting.md) — common problems
- [Glossary](glossary.md) — term definitions
