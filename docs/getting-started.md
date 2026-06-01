# Getting Started

This guide walks you through setting up GCM for the first time.

## Prerequisites

- **Git** installed and configured (2.20+ recommended)
- **OpenSSH** available (`ssh`, `ssh-add` on `PATH`)
- (Optional) **GPG** for commit signing

## Step 1: Install GCM

See [Installation](installation.md) for your platform. Verify:

```bash
gcm version
```

## Step 2: Check System Health

```bash
gcm doctor
```

This verifies that Git, SSH, and GPG are properly installed and checks your shell integration status.

## Step 3: Set Up Shell Integration

```bash
gcm init
```

GCM auto-detects your shell and installs hooks for auto-switching and prompt indicators. It also registers GCM's built-in credential helper for configured provider hosts, which serves tokens from GCM's encrypted store — making git auth immune to VS Code logout or external credential changes. **Restart your shell** after running this (or `source ~/.zshrc`, etc.).

See [Shell Integration](shell-integration.md) for details.

## Step 4: Create Your First Profile

### Interactive Mode (Recommended)

```bash
gcm profile create work --interactive
```

The wizard walks you through four steps:

1. **Basic Info** — Name and email for Git commits, default editor
2. **SSH Key** — Optionally generate a new SSH key (Ed25519 by default)
3. **GPG Signing** — Optionally generate a GPG key and enable commit signing
4. **Provider Account** — Link one provider account for this profile

> **Not sure what a prompt means?** See the [Interactive Guide](interactive-guide.md) for a plain-English explanation of every question, term, and option in the wizard.

### Quick Mode

```bash
gcm profile create work \
  --name "John Doe" \
  --email "john@company.com"
```

> **Tip:** Profile names are free-form identifiers you choose. `work` is not special — you could use `acme-corp`, `personal`, `gh-main`, or anything that makes sense. See [usage.md](usage.md#53-profile-naming--what-should-i-call-them) for naming rules.

## Step 5: Activate Your Profile

```bash
# Activate (session scope in git repos, local scope elsewhere)
gcm use work

# Or set as the machine-wide default
gcm use work --global
```

> **Credential isolation:** When you switch profiles, GCM automatically updates git credentials so `git push`/`git clone` authenticate as the selected provider account. No credential bleed between profiles.

## Step 6: Create Additional Profiles

```bash
gcm profile create personal -i
```

## Step 7: Pin Profiles to Projects

```bash
cd ~/projects/work-project
gcm use work --local

cd ~/projects/personal-project
gcm use personal --local
```

This creates a `.gcm-profile` file in each directory. With shell integration active, GCM auto-switches profiles when you `cd` between them.

## Step 8: Verify Setup

```bash
gcm current            # Show active profile with details
gcm current --short    # Just the profile name
gcm profile list       # List all profiles
gcm validate           # Validate all profiles
gcm validate work      # Validate a specific profile
```

## What's Next?

| Task | Command | Guide |
| ---- | ------- | ----- |
| Generate an SSH key | `gcm ssh generate work` | [usage.md — SSH](usage.md#9-ssh-key-management) |
| Connect a provider | `gcm connect work --provider github` | [usage.md — Provider Auth](usage.md#11-provider-authentication) |
| Enable GPG signing | `gcm gpg generate work` | [usage.md — GPG](usage.md#10-gpg-commit-signing) |
| Create team templates | `gcm template import tpl.yaml` | [usage.md — Templates](usage.md#12-templates) |
| Back up your config | `gcm backup create` | [usage.md — Backup](usage.md#13-backup--restore) |
| Full command reference | `gcm --help` | [Commands Reference](commands.md) |

### More Resources

- [Examples & Recipes](examples.md) — real-world workflows for common scenarios
- [Configuration](configuration.md) — customize GCM settings
- [FAQ](faq.md) — answers to common questions
- [Troubleshooting](troubleshooting.md) — solutions to common problems
