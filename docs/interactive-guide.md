# Interactive Guide

A complete explanation of every prompt, term, and option you'll encounter while using GCM interactively. If you're unsure what GCM is asking — this page has the answer.

---

## Profile Creation Wizard

When you run `gcm profile create <name> --interactive` (or `-i`), GCM walks you through a 4-step wizard. Here's what each step means and what you should enter.

---

### Step 1/4: Basic Information

```
Step 1/4: Basic Information
───────────────────────────
? Your full name: 
? Your email: 
? Git editor (leave empty for default): 
```

#### "Your full name"

**What it is:** The name that appears on your Git commits (the `Author:` line).

**What to enter:** Your real name as you want it to appear in commit history. This is public on GitHub/GitLab.

- Work profile: Your full name as registered with your company (`"Jane Doe"`, `"Muhammad Jundana"`)
- Personal profile: Whatever you want your open-source commits to show (`"janedoe"`, `"Jane D."`)

**Example:** `Muhammad Jundana`, `jundana-sirclo`, `Jane Doe`

> This sets `git config user.name` for the profile.

---

#### "Your email"

**What it is:** The email associated with your Git commits. GitHub uses this to link commits to your account.

**What to enter:** 
- **Work:** Your company email (`muhammad.jundana@sirclo.com`)
- **Personal:** Your personal email or GitHub noreply email (`janedoe@gmail.com` or `123456+janedoe@users.noreply.github.com`)

**Why it matters:** If the email doesn't match your GitHub account, your commits will appear as "unverified" and won't show your avatar.

> This sets `git config user.email` for the profile.

**Tip:** Find your GitHub noreply email at https://github.com/settings/emails → "Keep my email addresses private" shows your noreply address.

---

#### "Git editor (leave empty for default)"

**What it is:** The text editor that opens when Git needs you to type something (commit messages, interactive rebase, merge conflicts).

**What to enter:**
| Editor | Value |
|--------|-------|
| VS Code | `code --wait` |
| VS Code Insiders | `code-insiders --wait` |
| Vim | `vim` |
| Neovim | `nvim` |
| Nano | `nano` |
| Sublime Text | `subl -n -w` |
| Leave empty | Uses system default (usually `vim`) |

**The `--wait` flag:** Tells Git to wait until you close the editor tab before continuing. Without it, Git thinks you're done immediately.

> This sets `git config core.editor` for the profile.

---

### Step 2/4: SSH Configuration

```
Step 2/4: SSH Configuration
───────────────────────────
? Generate a new SSH key? [Y/n]: 
? SSH key type:
  ✓ ed25519 (recommended)
    rsa (4096 bits)
    ecdsa
```

#### "Generate a new SSH key?"

**What is SSH?** SSH (Secure Shell) is how your computer securely talks to GitHub/GitLab without using passwords. Instead of typing a password every time you `git push`, your computer uses a private key file to prove your identity.

**What is an SSH key?** It's a pair of files:
- **Private key** (`~/.ssh/id_ed25519_work`) — Your secret. Never share this.
- **Public key** (`~/.ssh/id_ed25519_work.pub`) — Safe to share. You upload this to GitHub.

**When to say Yes:**
- You don't have an SSH key for this identity yet
- You want a separate key per Git identity (recommended)
- This is a new machine

**When to say No:**
- You already have an SSH key you want to reuse
- Your company provides SSH keys

---

#### "SSH key type" — ed25519 vs RSA vs ECDSA

| Type | What It Is | Why Choose It |
|------|-----------|---------------|
| **ed25519** (recommended) | Modern elliptic curve algorithm | Fastest, smallest key, highest security. Works everywhere since 2014. **Pick this unless told otherwise.** |
| **rsa (4096 bits)** | Older algorithm, still universal | Only choose if your server requires RSA (rare legacy systems) |
| **ecdsa** | Another elliptic curve variant | Rarely needed. Only if your organization mandates it |

**TL;DR:** Pick **ed25519**. It's what GitHub, GitLab, and every modern server recommends.

**After generation, you'll see:**
```
✓ SSH key generated
  Path: ~/.ssh/id_ed25519_work
  Fingerprint: SHA256:abc123...
```

- **Path:** Where the key file was saved
- **Fingerprint:** A unique ID for your key (used to verify it on GitHub's SSH key settings page)

---

### Step 3/4: GPG Signing

```
Step 3/4: GPG Signing
─────────────────────
? Enable commit signing? [y/N]: 
```

#### "Enable commit signing?"

**What is commit signing?** A way to cryptographically prove that YOU made a commit — not someone who just typed your name and email. Signed commits show a green "Verified" badge on GitHub.

**What is GPG?** GNU Privacy Guard — a tool for encryption and digital signatures. GCM uses it to sign your Git commits.

**When to say Yes:**
- Your company requires verified commits
- You contribute to security-sensitive open source projects
- You want a verified-commit badge on a supported Git host

**When to say No (default):**
- You're just getting started and don't need verification yet
- Your company doesn't require it
- You don't have GPG installed (`gpg --version` to check)

**If you say Yes:** GCM generates a GPG key using your profile's name and email, then configures Git to auto-sign every commit. You'll need GPG installed (`brew install gnupg` on macOS).

> This sets `git config commit.gpgsign = true` and `git config user.signingkey = <key-id>`.

---

### Step 4/4: Provider Account (Optional)

```
Step 4/4: Provider Account (Optional)
───────────────────────────
? Provider for this profile (current: none):
```

#### "Provider account"

**What it is:** The Git hosting provider account attached to this profile. A profile is scoped to exactly one provider.

**What to enter:**
- Choose GitHub, GitLab, or another configured provider
- Enter the username for that provider account
- Skip if you want to connect the provider later

**This does NOT log you in.** To authenticate with a provider API (upload SSH keys, etc.), run `gcm connect <profile> --provider <id>` or a provider-specific login command.

---

## GitHub Login (Device Flow)

When you run `gcm github login-oauth <profile>`:

```
🌐 GitHub Login for Profile: work

Please open the following URL in your browser:
  https://github.com/login/device

Enter code: ABCD-1234

⠋ Waiting for authorization...
```

#### "Device flow" — What's happening?

This is GitHub's secure way to log in from a terminal:

1. **GCM asks GitHub** for a one-time code
2. **You open the URL** in any browser (phone works too)
3. **You type the code** on GitHub's website
4. **You click "Authorize"** in the browser
5. **GCM receives a token** automatically (no password ever enters your terminal)

#### Why not just paste a password?

The device flow is more secure:
- Your password never touches the terminal (can't be leaked in shell history)
- No client secret needed
- Works even over SSH (no browser redirect)
- You can revoke access anytime from GitHub Settings

#### What is an "access token"?

A long random string that proves you're authorized. Like a password, but:
- Scoped (can only do specific things)
- Revocable (you can delete it without changing your password)
- Stored encrypted by GCM (AES-256-GCM or OS keychain)

#### After success:

```
✓ Authorization successful!
  Logged in as justjundana (Muhammad Jundana)
```

---

## SSH Commands

### `gcm ssh generate <profile>`

```
⠋ Generating SSH key...
✓ SSH key generated
  Path: /Users/you/.ssh/id_ed25519_work
  Type: ed25519
  Fingerprint: SHA256:xyzabc123...
  Public key: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA...
```

| Term | Meaning |
|------|---------|
| **Path** | File location of your private key. The `.pub` version is next to it |
| **Type** | Algorithm used (ed25519, rsa, ecdsa) |
| **Fingerprint** | Unique hash of the key. Used to identify which key is which |
| **Public key** | The text you paste into GitHub → Settings → SSH Keys |

### `gcm ssh test <profile>`

```
⠋ Testing SSH connection...
✓ SSH connection successful!
```

Tests if GitHub/GitLab accepts your SSH key. If it fails, you probably need to:
1. Upload the public key to GitHub — this is offered automatically during `gcm ssh generate` if you're logged in. Otherwise: Settings → SSH and GPG keys → New SSH key
2. Or add the key to your SSH agent: the key isn't loaded in memory

### `gcm ssh list`

```
Profile: work
  Path: ~/.ssh/id_ed25519_work
  Type: ed25519
  Fingerprint: SHA256:abc...
  Agent: ✓
```

| Term | Meaning |
|------|---------|
| **Agent: ✓** | The key is loaded in your SSH agent (ready to use) |
| **Agent: ✗** | The key is NOT loaded. Run `ssh-add <path>` to load it |

**What is the SSH agent?** A background program that holds your decrypted keys in memory so you don't have to type your passphrase every time you `git push`.

---

## GPG Commands

### `gcm gpg generate <profile>`

```
⠋ Generating GPG key...
✓ GPG key generated
  Key ID: 0xABCD1234
```

| Term | Meaning |
|------|---------|
| **Key ID** | A short hexadecimal identifier for your GPG key (e.g., `0xABCD1234`). Git uses this to know which key to sign with |

### `gcm gpg sign enable/disable <profile>`

```
✓ Commit signing enabled for 'work'
```

After enabling:
- Every `git commit` is automatically signed
- Commits can show a verified badge on supported Git hosts once the GPG key is uploaded
- GCM auto-uploads the key during `gcm gpg generate` if a provider token is stored for the profile
- Manual alternative: open your provider's GPG key settings and paste output of `gpg --armor --export <KEY_ID>`

---

## Profile Activation (Scopes)

When you run `gcm use <profile>`:

```
✓ Profile 'work' activated (global)
```

The three **scopes**:

| Scope | Command | What It Means | When It Resets |
|-------|---------|---------------|----------------|
| **Session** | `gcm use work` | Only this terminal window | When you close the terminal |
| **Global** | `gcm use work --global` | Default for all new terminals | Never (until you change it) |
| **Local** | `gcm use work --local` | Auto-activates when you `cd` into this directory | Never (stored in `.gcm-profile` file) |

### "Dry-run mode"

```
🔍 Dry-run mode: No changes will be made
```

When you add `--dry-run`, GCM shows what WOULD happen without actually doing anything. Useful for checking before you commit to a change.

---

## System Health Check

When you run `gcm doctor`:

```
🏥 GCM System Health Check

System
  OS: darwin/arm64
  Go: go1.26.0

Dependencies
  Git: ✓ (v2.44.0)
  SSH: ✓ (OpenSSH_9.7)
  GPG: ✓ (gpg 2.4.5)
  SSH Agent: ✓

Configuration
  Config: ✓
  Profiles: 3

Shell
  Detected: zsh
  Integration: ✓
```

| Term | Meaning |
|------|---------|
| **OS: darwin/arm64** | Operating system (`darwin` = macOS) and CPU type (`arm64` = Apple Silicon) |
| **Go: go1.26.0** | Go language version used to build GCM |
| **Git: ✓** | Git is installed and working |
| **SSH: ✓** | OpenSSH client is installed |
| **GPG: ✓** | GnuPG is installed (or ✗ if not — that's OK if you don't use signing) |
| **SSH Agent: ✓** | SSH agent is running (can hold your keys in memory) |
| **Config: ✓** | `~/.gcm/config.yaml` exists and is valid |
| **Profiles: 3** | Number of profile files found |
| **Detected: zsh** | Your current shell |
| **Integration: ✓** | Shell hooks are installed (auto-switching works) |

---

## Backup & Restore

### `gcm backup create`

```
⠋ Creating backup...
✓ Backup created
  Path: ~/.gcm/backups/gcm-backup-2026-05-18.tar.gz
  Profiles: 3
  Templates: 1
  Size: 4.2 KB
```

| Term | Meaning |
|------|---------|
| **tar.gz** | A compressed archive format (like a .zip file) |
| **Profiles: 3** | Number of profile YAML files included |
| **Templates: 1** | Number of template YAML files included |

### `gcm backup restore <file>`

```
? This will overwrite current data. Continue? [y/N]: 
```

**"Overwrite current data"** means: your current `config.yaml`, profiles, and templates will be replaced with what's in the backup. Make sure you want this.

### `gcm backup prune --keep <n>`

```
✓ Removed 5 old backups (kept 3)
```

**Prune** = delete old backups, keeping only the N most recent. Saves disk space.

---

## Shell Integration

### `gcm init`

```
🚀 Setting up GCM for zsh

✓ Shell integration installed!
  Config file: ~/.zshrc
  Restart your shell or run: source ~/.zshrc
```

| Term | Meaning |
|------|---------|
| **Shell integration** | Code added to your shell config that enables auto-switching |
| **Config file: ~/.zshrc** | The file GCM appended code to |
| **source ~/.zshrc** | Reloads your shell config without closing the terminal |

**What "shell integration" actually does:**
1. Adds a hook that runs every time you `cd` to a new directory
2. The hook checks if a `.gcm-profile` file exists
3. If it does, GCM automatically switches to that profile
4. Also adds a prompt indicator showing your active profile name
5. Registers GCM's built-in credential helper for configured provider hosts (serves tokens from GCM's encrypted store, immune to VS Code logout)

---

## Template Operations

### `gcm template create`

```
Template creation wizard coming soon. For now, create templates by importing YAML files:
  gcm template import template.yaml
```

**What is a template?** A reusable blueprint for creating profiles. If everyone on your team should have the same editor settings and GPG signing enabled, create a template and have everyone create profiles from it.

---

## Common Error Messages Explained

| Error | What It Means | How to Fix |
|-------|--------------|------------|
| `profile 'work' has no SSH key configured` | No SSH key is linked to this profile | `gcm ssh generate work` |
| `profile 'work' has no GPG key` | No GPG key was generated for this profile | `gcm gpg generate work` |
| `not authenticated` | No provider token saved for this profile | `gcm connect work --provider <github|gitlab>` |
| `token invalid. Re-authenticate` | Your saved provider token expired or was revoked | `gcm connect work --provider <github|gitlab>` |
| `GPG is not installed on your system` | The `gpg` command isn't available | Install GPG: `brew install gnupg` (macOS) or `apt install gnupg` (Linux) |
| `Could not detect your shell` | GCM couldn't determine your shell from `$SHELL` | Set `SHELL` env var correctly: `SHELL=/bin/zsh gcm init` |

---

## Terminology Quick Reference

| Term | Plain English |
|------|--------------|
| **Profile** | A saved Git identity (name + email + SSH key + settings) |
| **SSH key** | A file that proves your identity to GitHub (instead of a password) |
| **GPG key** | A key that digitally signs your commits (proves you made them) |
| **Commit signing** | Adding a cryptographic signature to each commit → "Verified" badge |
| **SSH agent** | A background service that remembers your SSH keys so you don't retype passphrases |
| **Device flow** | A login method where you enter a code in your browser instead of typing a password in terminal |
| **Access token** | A revocable credential that lets GCM talk to GitHub's API on your behalf |
| **Fingerprint** | A short hash that uniquely identifies a key (like a key's "ID card") |
| **ed25519** | A modern, fast, secure key type. The default and recommended choice |
| **RSA** | An older key type. Still works but larger and slower than ed25519 |
| **Scope (session/global/local)** | Where and how long a profile stays active |
| **Shell integration** | Auto-switching profiles when you `cd` between directories |
| **`.gcm-profile`** | A tiny file in a project folder that tells GCM which profile to use there |
| **Prune** | Delete old items (backups) keeping only the most recent N |
| **Dry-run** | Show what would happen without actually doing it |
| **tar.gz** | A compressed archive format (like .zip) |
| **config.yaml** | GCM's main settings file at `~/.gcm/config.yaml` |

---

## See Also

- [Quick Start](quick-start.md) — 5-minute setup
- [Getting Started](getting-started.md) — step-by-step guide
- [Commands Reference](commands.md) — full CLI reference
- [FAQ](faq.md) — common questions
- [Glossary](glossary.md) — complete term definitions
