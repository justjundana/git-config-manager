# GCM Complete Usage Guide

> **Git Config Manager (`gcm`)** — a single CLI to juggle multiple Git, SSH, GPG, and GitHub/GitLab identities on one machine. This guide walks you end-to-end: install, first profile, daily flow, every sub-command, troubleshooting, and uninstall.

---

## Table of Contents

- [GCM Complete Usage Guide](#gcm-complete-usage-guide)
  - [Table of Contents](#table-of-contents)
  - [1. What is GCM?](#1-what-is-gcm)
  - [2. Requirements](#2-requirements)
  - [3. Installation](#3-installation)
    - [Option A — build from source](#option-a--build-from-source)
    - [Option B — prebuilt binary](#option-b--prebuilt-binary)
    - [Option C — `go install`](#option-c--go-install)
  - [4. First-Time Setup](#4-first-time-setup)
    - [4.1 Install shell integration](#41-install-shell-integration)
    - [4.2 Verify the environment](#42-verify-the-environment)
  - [5. Creating Your First Profile](#5-creating-your-first-profile)
    - [5.1 Interactive wizard (recommended)](#51-interactive-wizard-recommended)
    - [5.2 Non-interactive (scriptable)](#52-non-interactive-scriptable)
    - [5.3 Profile naming — what should I call them?](#53-profile-naming--what-should-i-call-them)
    - [5.4 Common multi-profile scenarios](#54-common-multi-profile-scenarios)
    - [5.5 Renaming a profile](#55-renaming-a-profile)
  - [6. Activating a Profile](#6-activating-a-profile)
    - [Show the active profile](#show-the-active-profile)
    - [Refresh after external edits](#refresh-after-external-edits)
  - [7. Daily Workflow](#7-daily-workflow)
  - [8. Managing Profiles](#8-managing-profiles)
  - [9. SSH Key Management](#9-ssh-key-management)
  - [10. GPG Commit Signing](#10-gpg-commit-signing)
  - [11. Provider Authentication](#11-provider-authentication)
  - [12. Templates](#12-templates)
  - [13. Backup \& Restore](#13-backup--restore)
  - [14. Diagnostics \& Validation](#14-diagnostics--validation)
  - [15. Configuration Reference](#15-configuration-reference)
  - [16. Security Model](#16-security-model)
  - [17. Built-in Flags](#17-built-in-flags)
  - [18. Command Cheatsheet](#18-command-cheatsheet)
  - [19. Troubleshooting](#19-troubleshooting)
    - [Shell integration isn't loading](#shell-integration-isnt-loading)
    - [Profile doesn't auto-switch on `cd`](#profile-doesnt-auto-switch-on-cd)
    - [`Permissions 0644 for 'id_ed25519_work' are too open`](#permissions-0644-for-id_ed25519_work-are-too-open)
    - [GitHub device flow says `authorization_pending` forever](#github-device-flow-says-authorization_pending-forever)
    - [`gcm use` seems to do nothing](#gcm-use-seems-to-do-nothing)
    - [Git still authenticates as the wrong account](#git-still-authenticates-as-the-wrong-account)
    - [Reset a broken state](#reset-a-broken-state)
  - [20. Uninstall](#20-uninstall)

---

## 1. What is GCM?

GCM is a command-line tool that manages multiple complete Git identities — not just `user.name` and `user.email`, but also SSH keys, GPG signing keys, provider tokens, and editor preferences — and switches between them with one command.

**Key features**

- Named **profiles** (`work`, `personal`, `client-x`, …)
- Per-directory auto-switching via a `.gcm-profile` file
- Native SSH key generation (Ed25519 / RSA / ECDSA) with encrypted-at-rest passphrases
- GPG key generation and per-profile signing toggles
- GitHub/GitLab provider login with AES-256-GCM token storage
- YAML templates for reproducible profile creation
- Timestamped backup / restore with zip-slip protection
- JSONL audit log of every state change
- Shell integration for bash / zsh / fish / PowerShell

---

## 2. Requirements

| Tool    | Required | Notes                                 |
| ------- | -------- | ------------------------------------- |
| Git     | Yes      | 2.20+ recommended                     |
| OpenSSH | Yes      | `ssh`, `ssh-add` on `PATH`            |
| GPG     | Optional | Only needed for commit signing        |
| Go      | Optional | Only needed to build from source      |
| Shell   | Yes      | bash, zsh, fish, or PowerShell        |

Run `gcm doctor` any time to verify the environment.

---

## 3. Installation

### Option A — build from source

```bash
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager
make build            # produces ./bin/gcm
make install          # copies to $(go env GOPATH)/bin/gcm (no sudo)
# or: make install-system   # copies to /usr/local/bin/gcm (needs sudo)
gcm version
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` (typically `~/go/bin`).

### Option B — prebuilt binary

Download the binary and `checksums.txt` for your platform from the same release page, verify the checksum, and move the `gcm` binary onto your `PATH`:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
sudo mv gcm-<os>-<arch> /usr/local/bin/gcm
chmod +x /usr/local/bin/gcm
gcm version
```

### Option C — `go install`

```bash
go install github.com/justjundana/git-config-manager/cmd/gcm@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

---

## 4. First-Time Setup

### 4.1 Install shell integration

```bash
gcm init
```

This detects your shell and appends a marked block to your rc file, and registers GCM's built-in credential helper for configured provider hosts (bypassing the system keychain so git auth is immune to VS Code logout or external credential changes):

```
# >>> GCM shell integration >>>
…
# <<< GCM shell integration <<<
```

If auto-detection picks the wrong shell, set the `SHELL` environment variable before running:

```bash
SHELL=/bin/zsh gcm init
```

Then **restart your shell** (or `source ~/.zshrc`, etc.). After restart you get:

- Auto-switching when you `cd` into a directory with `.gcm-profile`
- An active-profile indicator in your prompt

### 4.2 Verify the environment

```bash
gcm doctor
```

Checks Git, SSH, GPG, data directory permissions, and the shell integration block.

---

## 5. Creating Your First Profile

### 5.1 Interactive wizard (recommended)

```bash
gcm profile create work -i
```

The four-step wizard prompts for:

1. **Basic info** — display name, Git `user.name`, Git `user.email`, default editor
2. **SSH key** — generate a new key (Ed25519 by default) or link an existing one; optional passphrase (stored AES-256-GCM encrypted)
3. **GPG signing** — generate a new key or link an existing key id; toggle auto-signing
4. **GitHub** — username and optional device-flow login

### 5.2 Non-interactive (scriptable)

```bash
gcm profile create work \
  --name  "Jane Doe" \
  --email "jane@company.example" \
  --ssh-key ~/.ssh/id_ed25519_work \
  --editor code
```

Flags:

| Flag              | Description                               |
| ----------------- | ----------------------------------------- |
| `-i, --interactive` | Launch the full wizard                  |
| `--name`          | Git `user.name`                           |
| `--email`         | Git `user.email`                          |
| `--ssh-key`       | Path to an existing private key           |
| `--editor`        | Default Git editor (`vim`, `code`, `nano`…) |

Profiles are stored as YAML in `~/.gcm/profiles/<name>.yaml`.

### 5.3 Profile naming — what should I call them?

The word after `gcm profile create` is just an **identifier you pick**. There is no special `work` profile — you could call it `acme-corp`, `personal`, `client-foo`, `gh-main`, whatever makes sense.

**Rules (enforced by the validator):**

- Must start with a letter or digit
- May contain only letters, digits, `-`, or `_`
- Not empty
- Names that differ only by case (for example `Work` and `work`) are rejected as duplicates for cross-platform consistency

**Recommended:**

- Lowercase letters, digits, `-`, `_`
- Short — the name shows up in your prompt on every `cd`, so `(work)` reads better than `(company-i-joined-in-2026)`
- Pick one casing and stick to it — commands use exact-case lookup even though duplicate creation is case-insensitive
- Avoid spaces — they are invalid; use `-` or `_` instead

All of these are valid:

```bash
gcm profile create work
gcm profile create personal
gcm profile create acme-corp
gcm profile create client_foo
gcm profile create gh-personal
gcm profile create oss-contrib
gcm profile create bootcamp-2026
```

### 5.4 Common multi-profile scenarios

**Day job + personal**

```bash
gcm profile create work     -i   # jane@company.example
gcm profile create personal -i   # jane@gmail.com
```

**Freelancer with multiple clients**

```bash
gcm profile create personal      -i
gcm profile create client-acme   -i
gcm profile create client-globex -i
gcm profile create client-stark  -i
```

**One machine, multiple GitHub orgs**

```bash
gcm profile create gh-personal -i
gcm profile create gh-tokopedia -i
gcm profile create gh-gojek    -i
```

**Employee + open-source + anonymous account**

```bash
gcm profile create dayjob -i
gcm profile create oss    -i
gcm profile create anon   -i
```

In every case, one profile = one full Git identity (name, email, SSH key, GPG key, provider, and provider token). Use separate profiles for GitHub and GitLab accounts. After creating them, pin each repo tree with `gcm use <name> --local` once and they will auto-activate on `cd`.

### 5.5 Renaming a profile

There is no built-in `rename`, but export → edit → import does the job:

```bash
# 1. export the profile's YAML
gcm profile export old-name > /tmp/profile.yaml

# 2. edit the "name:" field inside the file to the new identifier
${EDITOR:-vim} /tmp/profile.yaml

# 3. import under the new name, then delete the old entry
gcm profile import /tmp/profile.yaml
gcm profile delete old-name -y

# 4. if this profile was pinned in any directory, refresh the .gcm-profile
cd ~/code/that-repo
gcm use new-name --local
```

---

## 6. Activating a Profile

```bash
gcm use work              # smart: session (git repo) or local (elsewhere)
gcm use work --global     # persist as the machine-wide default
gcm use work --local      # pin to the current directory (.gcm-profile)
gcm use work --dry-run    # preview changes, apply nothing
```

What `gcm use` does:

- Writes Git config (`user.name`, `user.email`, `core.editor`, `commit.gpgsign`, `user.signingkey`)
- Loads the SSH key into the ssh-agent via `ssh-add` (if configured and key file exists)
- **Pins provider credential usernames** so git only uses this profile's account
- If GCM is the credential helper: git will ask GCM dynamically for credentials
- If GCM is NOT the credential helper (legacy): **clears old credentials** and **stores new credentials** for the profile
- Verifies provider token validity (best-effort, warns if expired)

> **Credential isolation:** After switching, `git push`/`git clone` will authenticate as the active profile's selected provider account only. Other stored credentials cannot bleed through.

> **Smart scope:** Without `--global` or `--local`, GCM uses session scope (`.git/gcm-session`) inside a git repo, or local scope (`.gcm-profile`) outside one. This means `gcm use` **always works** — no "not in a git repository" errors.

### Show the active profile

```bash
gcm current                         # full details
gcm current --short                 # just the name, for scripts or prompts
gcm current --short --hide-default  # nothing if default is active (shell prompts)
```

### Refresh after external edits

```bash
gcm refresh            # re-apply current profile
gcm refresh --silent   # no output, useful in shell hooks
```

---

## 7. Daily Workflow

A realistic setup with two identities:

```bash
# one-time
gcm profile create work     -i
gcm profile create personal -i

# pin each repo tree once
cd ~/code/company
gcm use work --local

cd ~/code/personal
gcm use personal --local
```

From then on, just `cd` between the trees. GCM detects `.gcm-profile` in the current or parent directory, activates the right profile, and updates your prompt. No manual switching.

For a one-off override inside a pinned directory:

```bash
gcm use personal        # session only, doesn't touch .gcm-profile
```

---

## 8. Managing Profiles

```bash
gcm profile list                                  # alias: ls
gcm profile show work                             # detailed view
gcm profile edit work -n "Jane Smith"             # update display name
gcm profile edit work -e "jane@new.example"       # update email
gcm profile edit work -n "Jane" -e "j@x.com"     # update both at once
gcm profile delete work                           # alias: rm; prompts for confirmation
gcm profile delete work -y                        # skip confirmation
gcm profile export work                           # prints the YAML to stdout
gcm profile import ./work.yaml                    # imports from a file
gcm profile diff work personal                    # side-by-side comparison
```

> **Note:** `profile edit` currently supports updating name (`-n`) and email (`-e`) inline. To change other fields (SSH key, GPG, etc.), export the profile, edit the YAML manually, then reimport:
> ```bash
> gcm profile export work > /tmp/work.yaml
> ${EDITOR:-vim} /tmp/work.yaml
> gcm profile delete work -y && gcm profile import /tmp/work.yaml
> ```

Profile names must start with a letter or digit and may contain only letters, digits, `-`, or `_`. Names that differ only by case cannot coexist.

---

## 9. SSH Key Management

```bash
gcm ssh generate work                      # Ed25519, no passphrase
gcm ssh generate work -t rsa -b 4096       # RSA 4096
gcm ssh generate work -t ecdsa             # ECDSA P-256
gcm ssh generate work -c "work@laptop"     # custom comment
gcm ssh generate work -p "correct-horse"   # passphrase (encrypted at rest)
gcm ssh generate work --overwrite          # replace existing local key pair

gcm ssh list             # keys across all profiles
gcm ssh test work        # ssh -T git@github.com using the profile's key
gcm ssh copy work        # copy public key to clipboard
gcm ssh upload work      # upload key to the profile's provider (duplicate-safe)
```

Keys are generated with Go's native crypto (no subprocess, no passphrase leaking into argv). Private keys are written `0600`, public keys `0644`. If a passphrase is provided, the private key is encrypted at rest using OpenSSH native format (bcrypt-KDF + AES-256-CTR); the passphrase itself is not stored anywhere.

If the expected provider-aware key already exists, such as `~/.ssh/id_ed25519_work_github`, and the profile has no SSH key configured, `gcm ssh generate` links that key to the profile instead of failing. `gcm ssh upload`, `gcm ssh test`, and `gcm ssh copy` perform the same recovery before reporting that a profile has no SSH key.

If a provider token is stored for the profile (via `gcm connect <profile> --provider <id>`), GCM will offer to upload the SSH key to that provider automatically after generation.

If upload reports that the SSH key is already registered but not visible on the authenticated account, the key belongs to another account or deploy key. Generate a fresh profile key with `gcm ssh generate <profile> --overwrite`, then rerun `gcm ssh upload <profile>`.

GCM never overwrites an existing local key by default. Use `gcm ssh generate <profile> --overwrite` only when you intentionally want a new key pair at the same deterministic filename and provider upload title.

Flags for `ssh generate`:

| Flag              | Default    | Description                     |
| ----------------- | ---------- | ------------------------------- |
| `-t, --type`      | `ed25519`  | `ed25519` / `rsa` / `ecdsa`     |
| `-b, --bits`      | `4096`     | RSA key size (2048/3072/4096)   |
| `-c, --comment`   | hostname   | Comment baked into the key      |
| `-p, --passphrase`| *(none)*   | Optional passphrase             |
| `--overwrite`     | `false`    | Replace existing local key pair |

---

## 10. GPG Commit Signing

```bash
gcm gpg generate work         # generate a new GPG key for the profile
gcm gpg upload work           # upload the GPG key to the profile's provider
gcm gpg list                  # list keys known to GCM
gcm gpg sign enable work      # turn on auto-signing for this profile
gcm gpg sign disable work     # turn it off
gcm gpg test work             # perform a test signature
```

Inputs passed to `gpg` are validated: control characters and `%` are rejected to prevent injection into the batch-mode parameter file.

If a provider token is stored for the profile, `gcm gpg generate` will offer to upload the GPG public key to that provider automatically. You can also run `gcm gpg upload <profile>` later; it checks for duplicates before uploading unless `--force` is used. Verified badges appear on supported Git hosts.

When signing is enabled, `gcm use <profile>` writes:

```
[commit]
  gpgsign = true
[user]
  signingkey = <key-id>
```

---

## 11. Provider Authentication

For the provider-neutral login path, prefer:

```bash
gcm connect work --provider github
gcm connect work --provider gitlab
```

One profile belongs to one provider. Authentication status is source-aware: GCM distinguishes tokens it owns from external Git credentials returned by helpers such as Keychain, Git Credential Manager, GitHub CLI, or libsecret. Credentials returned by `gcm credential-helper` are treated as GCM-owned effective Git auth, not as external credentials to adopt or delete.

```bash
gcm auth status work --provider github --verbose
gcm auth inspect work --provider github
gcm auth adopt work --provider github --dry-run
gcm auth logout work --scope gcm
gcm auth doctor work
gcm auth repair --dry-run
```

Use `gcm auth adopt` only when you intentionally want to copy an exportable external Git credential into GCM's provider-aware token store. Use `gcm auth logout --scope external` only when you intentionally want GCM to ask Git's credential chain to remove a credential owned by another tool; `gcm-store` credentials are skipped because they already belong to GCM.

GCM supports multiple GitHub authentication methods:

```bash
# Method 1: Personal Access Token (headless, CI/CD, fine-grained control)
gcm github login work
echo "$GH_TOKEN" | gcm github login work   # pipe from env/script

# Method 2: OAuth device flow (interactive, browser-based)
gcm github login-oauth work

# Method 3: Import from GitHub CLI (if you already use 'gh')
gcm github login-gh work

# Manage tokens and auth state
gcm github status            # source-aware auth status for GitHub profiles
gcm auth status work         # GCM/external ownership status
gcm auth inspect work        # detailed source/helper inspection
gcm github verify work       # check if token is still valid
gcm github user work         # print GitHub user info
gcm github logout work       # remove stored token
```

**Device flow:** You'll see a short user code and a URL. Open the URL, paste the code, approve. GCM polls until success (respecting RFC 8628 `slow_down` errors) and stores the token encrypted in `~/.gcm/tokens/`.

**PAT:** Generate a token at https://github.com/settings/tokens with scopes: `repo`, `admin:public_key`, `admin:gpg_key`. GCM verifies the token before saving.

**gh CLI:** Requires `gh` to be installed and authenticated (`gh auth login`). GCM imports the existing token.

---

## 12. Templates

Templates are **reusable YAML files** that store Git configuration presets (editor, aliases, pull/push settings, commit options). They are useful for standardizing settings across a team or quickly recreating configuration patterns.

**What templates store** (see `~/.gcm/templates/<name>.yaml`):

```yaml
name: company-standard
description: "Standard config for ACME Corp developers"
git:
  core:
    editor: "code --wait"
    autocrlf: input
  commit:
    gpgsign: true
  pull:
    rebase: "true"
  push:
    autosetupremote: true
  aliases:
    co: checkout
    br: branch
    st: status
metadata:
  author: "Team Lead"
  version: "1.0"
  created: 2026-01-15T00:00:00Z
  updated: 2026-01-15T00:00:00Z
```

**What templates do NOT store:** user identity (name, email), SSH keys, GPG keys, or provider tokens. Those are per-person and belong in profiles.

**Available commands:**

```bash
gcm template create company-standard -i            # interactive creation wizard
gcm template create company-standard --from-profile work  # extract from profile
gcm template list                          # list all templates (alias: tpl ls)
gcm template show company-standard         # print template contents as YAML
gcm template apply company-standard work   # apply template settings to profile
gcm template export company-standard > t.yaml  # same as show, for saving to file
gcm template import ./company-standard.yaml    # import a YAML file as a template
gcm template delete company-standard       # remove a template (alias: rm)
```

**Typical team workflow:**

```bash
# Team lead: create and share a template file
cat > company-standard.yaml << 'EOF'
name: company-standard
description: "ACME Corp standard settings"
git:
  core:
    editor: "code --wait"
  commit:
    gpgsign: true
  pull:
    rebase: "true"
metadata:
  version: "1.0"
  created: 2026-01-15T00:00:00Z
  updated: 2026-01-15T00:00:00Z
EOF

# Share this file via repo, wiki, or Slack

# Team member: import and review
gcm template import company-standard.yaml
gcm template show company-standard
```

Templates are a **reference** — they are not automatically applied to profiles. Use `gcm template apply <template> <profile>` to merge template settings into a profile.

---

## 13. Backup & Restore

```bash
gcm backup create                  # snapshot of ~/.gcm → timestamped .tar.gz
gcm backup list                    # alias: ls
gcm backup restore <file>          # prompts before overwriting
gcm backup prune                   # keep the last 5 (default)
gcm backup prune --keep 10         # keep the last 10
```

Backups are stored in `~/.gcm/backups/` (permissions forced to `0700`). Restore extraction is guarded against path-traversal ("zip-slip") — any entry pointing outside the target directory is rejected.

Before overwriting, `restore` asks for confirmation unless you've already exported the profiles you care about.

---

## 14. Diagnostics & Validation

```bash
gcm validate               # check every profile
gcm validate work          # check one profile
gcm doctor                 # full environment check
```

`validate` looks at YAML schema, referenced SSH/GPG paths, token presence, and permissions. `doctor` adds Git/SSH/GPG version checks and shell-integration status.

---

## 15. Configuration Reference

**Data directory layout:**

```
~/.gcm/
├── config.yaml           # global settings
├── profiles/             # one YAML file per profile
├── templates/            # YAML template blueprints
├── tokens/        (0700) # encrypted provider tokens
├── backups/       (0700) # timestamped .tar.gz snapshots
├── logs/          (0700) # audit-YYYY-MM-DD.jsonl
└── cache/                # transient
```

**Sample `config.yaml`:**

```yaml
default_profile: work
auto_switch:
  enabled: true
  project_file: ".gcm-profile"
shell:
  integration: true
  prompt_indicator: true
  prompt_format: "(%s)"
github:
  api_url: "https://api.github.com"
backup:
  max_backups: 10
security:
  encrypt_tokens: true
  use_keychain: true
  master_password: false
  allow_plaintext_tokens: false
  audit_log: true
advanced:
  git_command: "git"
  ssh_command: "ssh"
  gpg_command: "gpg"
```

For the complete field reference, profile schema, and template schema, see [configuration.md](configuration.md).

**Sample profile YAML (`~/.gcm/profiles/work.yaml`):**

```yaml
name: work
git:
  user:
    name: Jane Doe
    email: jane@acme.example
    signingkey: 0xDEADBEEFCAFEBABE
  core:
    editor: code
  commit:
    gpgsign: true
ssh:
  key_path: ~/.ssh/id_ed25519_work
  key_type: ed25519
  fingerprint: "SHA256:..."
gpg:
  key_id: 0xDEADBEEFCAFEBABE
github:
  username: jane-acme
metadata:
  created: 2026-01-15T10:30:00Z
  updated: 2026-05-18T14:00:00Z
  usage_count: 42
  version: "1.0.0"
```

See [configuration.md](configuration.md) for a full field reference.

---

## 16. Security Model

- **Filesystem permissions** — `tokens/`, `backups/`, `logs/` are forced to `0700`; private keys `0600`; public keys `0644`.
- **Token encryption** — GitHub OAuth tokens are encrypted with **AES-256-GCM**, key derived via **Argon2id** from the master password (legacy tokens use PBKDF2, transparently migrated on next save).
- **SSH passphrases** — embedded in the private key file using OpenSSH native encryption (bcrypt-KDF + AES-256-CTR); never stored separately or passed on argv.
- **GPG batch input validation** — control characters and `%` are rejected to prevent parameter-file injection.
- **Audit log** — `~/.gcm/logs/audit-YYYY-MM-DD.jsonl`, one JSON object per state change (profile switch, key generation, token write, backup, restore).
- **Backup restore** — zip-slip check rejects any archive entry escaping the target directory.
- **Shell integration** — framed with `# >>> GCM shell integration >>>` / `# <<< GCM shell integration <<<` so uninstall is exact.
- **HTTP client** — 30 s timeout, context-aware OAuth polling with a 15 min overall deadline.

---

## 17. Built-in Flags

Cobra provides the following flag on every command:

| Flag             | Description                         |
| ---------------- | ----------------------------------- |
| `-h, --help`     | Help for the command                |
| `-v, --verbose`  | Enable verbose (debug) output       |
| `-q, --quiet`    | Suppress non-essential output       |
| `--no-color`     | Disable colored output              |

---

## 18. Command Cheatsheet

```text
Setup
  gcm init
  gcm doctor
  gcm version

Profiles
  gcm profile create <name> [-i] [--name] [--email] [--ssh-key] [--editor]
  gcm profile list | show <name> | edit <name> | delete <name> [-y]
  gcm profile export <name> | import <file> | diff <a> <b>

Activation
  gcm use <name> [-g | -l | --dry-run]
  gcm current [--short] [--hide-default]
  gcm refresh [--silent]

Auth
  gcm connect <name> --provider github|gitlab [--token-stdin] [--yes]
  gcm switch-provider <name> github|gitlab [--token-stdin] [--yes]
  gcm auth status [name] [--provider] [--json] [--verbose]
  gcm auth inspect <name> [--provider] [--json]
  gcm auth adopt <name> [--provider] [--dry-run] [--yes]
  gcm auth logout <name> [--provider] [--scope gcm|external|all] [--dry-run] [--yes] [--json]
  gcm auth doctor [name] [--provider] [--json]
  gcm auth repair [name] [--provider] [--dry-run] [--yes] [--json]

SSH
  gcm ssh generate <name> [-t] [-b] [-c] [-p] [--overwrite]
  gcm ssh upload <name> [--provider] [--force]
  gcm ssh list | test <name> | copy <name>

GPG
  gcm gpg generate <name>
  gcm gpg upload <name> [--provider] [--force]
  gcm gpg list
  gcm gpg sign enable|disable <name>
  gcm gpg test <name>

GitHub
  gcm github login <name>           # Personal Access Token
  gcm github login-oauth <name>     # OAuth device flow
  gcm github login-gh <name>        # Import from gh CLI
  gcm github status                 # Source-aware auth status
  gcm github logout <name>
  gcm github verify <name>
  gcm github user <name>

GitLab
  gcm gitlab login <name>           # Personal Access Token
  gcm gitlab status                 # Source-aware auth status
  gcm gitlab logout <name>
  gcm gitlab verify <name>
  gcm gitlab user <name>

Templates
  gcm template create <name> [-i] [--from-profile]
  gcm template list | show <name> | delete <name>
  gcm template apply <tpl> <profile> [--force]
  gcm template export <name> | import <file>

Backup
  gcm backup create | list
  gcm backup restore <file>
  gcm backup prune [--keep N]

Maintenance
  gcm validate [name]
  gcm clean [--all]
  gcm update [--check|--force|--prerelease]
```

---

## 19. Troubleshooting

### Shell integration isn't loading

1. Run `gcm init` to reinstall the block (set `SHELL` env var if auto-detection fails).
2. Restart the shell (don't just `source`).
3. Confirm the block exists:
   ```bash
   grep "GCM shell integration" ~/.zshrc
   ```

### Profile doesn't auto-switch on `cd`

- The directory (or one of its parents) needs a `.gcm-profile` file. Create it with `gcm use <name> --local`.
- Make sure shell integration is active: the prompt indicator (`$_GCM_PROMPT`) should change after `cd`. Run `gcm current` to verify the active profile.

### `Permissions 0644 for 'id_ed25519_work' are too open`

```bash
chmod 600 ~/.ssh/id_ed25519_work
```

GCM writes correct perms on generation — this only happens for keys imported from elsewhere.

### GitHub device flow says `authorization_pending` forever

- Make sure you approved on the exact URL GCM printed.
- There's a 15 min deadline; run `gcm github login <name>` again if you missed it.

### `gcm use` seems to do nothing

Run with `-v` (verbose) to see debug logs:

```bash
gcm -v use work
```

This logs every Git config write, SSH config mutation, and agent call. Also check:

```bash
gcm current               # check what's currently active
gcm validate work         # check for config issues
```

### Git still authenticates as the wrong account

After `gcm use`, git should only use the active profile's credentials. If you're still seeing the wrong account:

1. **Check credential username pinning:**
   ```bash
  # Replace github.com with the profile's provider host when needed.
   git config --global credential.https://github.com.username
   ```
  This should show the active profile's provider username.

2. **Clear stale credentials manually:**
   ```bash
  gcm auth logout <old-profile> --scope external --dry-run
  gcm github logout <old-profile> --clear-credentials
   gcm use <correct-profile>
   ```
  `--clear-credentials` clears both cached HTTPS secrets and the provider username pin for the active profile.

3. **Inspect the source Git sees:**
  ```bash
  gcm auth inspect <correct-profile> --provider github
  ```

4. **Verify which account git sees:**
   ```bash
   ssh -T git@github.com   # SSH check
   # or for HTTPS:
   git credential fill <<< "protocol=https
   host=github.com
   "
   ```

5. **macOS Keychain note:** If you previously saved credentials via Keychain Access, they may override GCM's pinning. Prefer `gcm auth logout <profile> --scope external --dry-run` before deleting entries manually, then re-run `gcm use`.

### Reset a broken state

```bash
gcm clean        # remove cache directory
gcm clean --all  # also remove logs directory
```

If you need to fully reset GCM, back up first and then remove the data directory manually:

```bash
gcm backup create
rm -rf ~/.gcm    # irreversible — make sure you have a backup
```

---

## 20. Uninstall

1. **Remove shell integration** — open your rc file and delete the block between `# >>> GCM shell integration >>>` and `# <<< GCM shell integration <<<`. Restart the shell.
2. **Export anything you want to keep:**
   ```bash
   gcm backup create
   cp ~/.gcm/backups/gcm-*.tar.gz ~/safe-place/
   ```
3. **Remove the data directory:**
   ```bash
   rm -rf ~/.gcm
   ```
4. **Remove the binary:**
   ```bash
   sudo rm /usr/local/bin/gcm
   ```

Done. Your SSH keys in `~/.ssh/` are untouched; only the GCM-managed data directory is removed.

---

*Need more? See [`getting-started.md`](getting-started.md), [`shell-integration.md`](shell-integration.md), [`configuration.md`](configuration.md), and [`troubleshooting.md`](troubleshooting.md) for deeper dives on individual topics.*
