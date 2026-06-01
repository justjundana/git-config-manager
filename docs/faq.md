# FAQ

Frequently asked questions about GCM.

---

## General

### What is GCM?

GCM (Git Config Manager) is a CLI tool that manages multiple complete Git identities — `user.name`, `user.email`, SSH keys, GPG signing keys, provider tokens, and editor preferences — and switches between them with one command.

### Why do I need GCM?

If you use multiple Git identities (work, personal, client projects, open source), you've probably committed with the wrong email or pushed with the wrong SSH key. GCM eliminates that by auto-switching your entire identity when you `cd` into a project.

### Is GCM free and open source?

Yes. GCM is licensed under the [MIT License](../LICENSE).

### What platforms does GCM support?

- **macOS** (amd64, arm64)
- **Linux** (amd64, arm64, arm)
- **Windows** (amd64)

### Does GCM support multiple Git hosts (GitHub, GitLab, Bitbucket)?

Yes. GCM has provider-aware profile, credential, SSH, and GPG flows for GitHub and GitLab. Each profile is scoped to exactly one provider, so keep separate profiles for separate provider accounts. Bitbucket is planned but not implemented yet.

---

## Installation & Setup

### How do I install GCM?

Build from source (Go 1.26+ required):

```bash
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager
make build && make install
```

Or via `go install`:

```bash
go install github.com/justjundana/git-config-manager/cmd/gcm@latest
```

See [Installation](installation.md) for details.

### Is there a Homebrew formula?

Not yet — it's planned. For now, build from source.

### Where does GCM store its data?

Everything is in `~/.gcm/`:

```
~/.gcm/
├── config.yaml       # Global settings
├── profiles/         # Profile YAML files
├── templates/        # Template YAML files
├── tokens/           # Encrypted provider tokens
├── backups/          # Backup archives
├── logs/             # Audit logs
└── cache/            # Temporary data
```

See [Configuration](configuration.md) for the full layout.

### How do I set up shell integration?

```bash
gcm init
exec $SHELL   # restart your shell
```

See [Shell Integration](shell-integration.md) for details.

---

## Profiles

### What is a profile?

A profile is a named Git identity. It contains:
- Git user info (name, email, editor)
- SSH key reference
- GPG signing configuration
- One provider account reference (GitHub, GitLab, etc.)
- Metadata (created, last used, usage count)

Profiles are stored as YAML files in `~/.gcm/profiles/`.

### Can I name my profile anything?

Almost. Profile names must:
- Start with a letter or digit
- Contain only letters, digits, `-`, or `_`
- Not differ only by case from an existing profile

Recommended: lowercase letters, digits, `-`, `_`. Examples: `work`, `personal`, `client-acme`, `gh-oss`.

### How do I switch profiles?

```bash
gcm use work          # session only
gcm use work --global # default
gcm use work --local  # pin to directory
```

With shell integration, `cd` into a pinned directory auto-switches.

### Can I have multiple profiles active at once?

No. Only one profile is active at a time per shell session. But different terminal windows can have different active profiles.

### How do I see which profile is active?

```bash
gcm current          # full details
gcm current --short  # just the name
```

### How do I delete a profile?

```bash
gcm profile delete work      # prompts for confirmation
gcm profile delete work -y   # skip confirmation
```

This deletes the YAML file. SSH keys in `~/.ssh/` are NOT deleted.

### Does deleting a profile delete my SSH keys?

No. GCM only deletes the profile YAML file. Your SSH keys in `~/.ssh/` are untouched.

---

## SSH Keys

### How are SSH keys generated?

GCM uses Go's native `crypto` library — no subprocess calls. Supported types:
- **Ed25519** (recommended, default)
- **RSA** (2048, 3072, or 4096 bits)
- **ECDSA** (P-256)

### Where are SSH keys stored?

By default in `~/.ssh/`. The key path is stored in the profile YAML.

### Are SSH passphrases stored securely?

Yes. If you provide a passphrase during `gcm ssh generate`, the private key is encrypted at rest using OpenSSH native format (bcrypt-KDF + AES-256-CTR). The passphrase itself is not stored anywhere — only the encrypted key file remains on disk. It never appears in command-line arguments.

### Can I use an existing SSH key?

Yes:

```bash
gcm profile create work --ssh-key ~/.ssh/id_ed25519_existing
```

Or add it to an existing profile by exporting, editing, and reimporting:

```bash
gcm profile export work > /tmp/work.yaml
# Edit the ssh.key_path field in /tmp/work.yaml
gcm profile delete work -y && gcm profile import /tmp/work.yaml
```

---

## GPG Signing

### Do I need GPG installed?

Only if you want commit signing. GPG is optional — GCM works fine without it.

### How does GPG signing work?

1. `gcm gpg generate work` creates a GPG key using the profile's name and email
2. The GPG key ID is stored in the profile
3. When the profile is activated, `commit.gpgsign = true` and `user.signingkey = <key-id>` are set in Git config
4. Git uses GPG to sign commits automatically

### Can I disable signing for specific profiles?

```bash
gcm gpg sign disable personal
```

---

## Provider Authentication

### How does GitHub login work?

GCM uses the OAuth [device flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow):

1. GCM generates a short code
2. You open https://github.com/login/device in your browser
3. You enter the code and approve
4. GCM receives and stores the access token

No client secret needed. Works over SSH.

### How are provider tokens stored?

By default, in your OS keychain (macOS Keychain, Linux secret-service, Windows Credential Manager). If keychain isn't available, tokens are encrypted with AES-256-GCM and stored in `~/.gcm/tokens/`.

### Why does Git work while GCM says the profile is not authenticated?

Git may be using an external credential from Keychain, Git Credential Manager, GitHub CLI, libsecret, or another helper. GCM reports that as external instead of pretending it owns the token:

```bash
gcm auth status work --provider github --verbose
gcm auth inspect work --provider github
```

If the credential is exportable and belongs to the intended account, adopt it explicitly with `gcm auth adopt work --provider github`. To remove an external credential, preview first with `gcm auth logout work --provider github --scope external --dry-run`.

### Can I use GCM without GitHub?

Absolutely. GitHub features are optional. You can use GCM with GitLab or purely for Git identity management, SSH keys, and GPG signing.

### The device flow timed out. What do I do?

Just try again:

```bash
gcm github login work
```

The flow has a 15-minute timeout. Make sure you open the exact URL and enter the exact code.

---

## Shell Integration

### Which shells are supported?

| Shell      | Auto-Switch | Prompt |
| ---------- | ----------- | ------ |
| Bash       | ✓           | ✓      |
| Zsh        | ✓           | ✓      |
| Fish       | ✓           | ✓      |
| PowerShell | ✓           | ✓      |

### What does `gcm init` actually do?

It appends a marked block of shell code to your config file (e.g., `~/.zshrc`). The block adds:
1. An auto-switch hook that runs on `cd`
2. A prompt indicator showing the active profile name
3. Registers GCM's built-in credential helper for configured provider hosts

### Why does git push fail after I logout from VS Code?

VS Code clears the macOS Keychain (or Windows Credential Manager) entry for `github.com` when you sign out. If git uses `osxkeychain`/`wincred` as its credential helper, the token is gone and `git push` fails.

**Fix:** Run `gcm init` to register GCM's built-in credential helper. This bypasses the system keychain entirely — tokens are served directly from GCM's provider-aware encrypted store, so external credential changes can't break your git auth.

### Can I have auto-switching without the prompt indicator?

Currently they're bundled together. You can manually edit the shell hook block to remove the prompt part.

### How does auto-switching work?

When you `cd` into a directory, the shell hook checks for a `.gcm-profile` file. If it finds one, it runs `gcm refresh --silent` to activate that profile.

### Should I commit `.gcm-profile` to Git?

Depends:
- **Solo project:** Yes, for consistency across machines
- **Team project:** No, add to `.gitignore` — profile names are personal
- **Monorepo:** Each developer manages their own `.gcm-profile` locally

---

## Backup & Restore

### What's included in a backup?

- `config.yaml`
- All profile YAML files (`~/.gcm/profiles/*.yaml`)
- All template YAML files (`~/.gcm/templates/*.yaml`)

Not included: SSH keys, provider tokens, audit logs.

### Can I restore on a different machine?

Yes. Copy the `.tar.gz` to the new machine and run:

```bash
gcm backup restore <file>
```

You'll need to re-authenticate GitHub (`gcm github login`) since tokens aren't backed up.

### How do I automate backups?

```bash
# Add to crontab
0 0 * * * /path/to/gcm backup create && /path/to/gcm backup prune --keep 7
```

---

## Security

### Is GCM safe to use?

Yes. GCM follows security best practices:
- Tokens encrypted at rest (AES-256-GCM or OS keychain)
- SSH private keys written with `0600` permissions
- Passphrases never in argv
- Audit logging of all operations
- Path traversal protection on imports/restores

See [Security Model](security.md) for the full threat model.

### Does GCM phone home?

No. GCM never sends telemetry, checks for updates automatically, or contacts any server other than the configured GitHub API (and only when you explicitly run `gcm github` commands).

### Can GCM access my private repos?

Only if you authorize it via `gcm github login-oauth`. The OAuth scopes requested are `repo`, `admin:public_key`, and `admin:gpg_key`. For PAT-based login (`gcm github login`), you control the scopes manually when generating the token.

---

## Troubleshooting

### `gcm: command not found`

Make sure `$(go env GOPATH)/bin` or `/usr/local/bin` is in your `PATH`. See [Installation](installation.md).

### Profile not auto-switching

1. Shell integration installed? `grep "GCM shell integration" ~/.zshrc`
2. Shell restarted? `exec $SHELL`
3. `.gcm-profile` exists? `cat .gcm-profile`
4. Profile exists? `gcm profile list`

See [Troubleshooting](troubleshooting.md) for more.

### "profile not found"

Check available profiles with `gcm profile list`. Names are case-sensitive.

### Where can I get help?

```bash
gcm --help
gcm <command> --help
```

Or open an issue: https://github.com/justjundana/git-config-manager/issues

---

## See Also

- [Troubleshooting](troubleshooting.md) — detailed problem/solution guide
- [Commands Reference](commands.md) — every command and flag
- [Examples](examples.md) — real-world workflows
