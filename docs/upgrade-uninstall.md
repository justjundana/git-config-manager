# Upgrade & Uninstall

How to update GCM to a new version or remove it completely.

---

## Upgrading

### Self-Update

```bash
gcm update              # download and install the latest stable release
gcm update --check      # check for updates without installing
```

This is the simplest method — it downloads the correct binary for your platform, verifies the SHA-256 checksum, and replaces the running binary with safe rollback on failure.

### Install Script

Re-run the install script — it will detect and replace the existing binary:

```bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash
```

### Download Binary Manually

Download the binary for your platform from [GitHub Releases](https://github.com/justjundana/git-config-manager/releases):

```bash
# Example for macOS ARM (Apple Silicon):
curl -fsSL https://github.com/justjundana/git-config-manager/releases/latest/download/gcm-darwin-arm64 -o gcm
chmod +x gcm
sudo mv gcm /usr/local/bin/gcm
```

### From Source

```bash
cd git-config-manager
git pull
make build && make install
```

### Via `go install`

```bash
go install github.com/justjundana/git-config-manager/cmd/gcm@latest
```

### Verify the Upgrade

```bash
gcm version
```

### Compatibility Notes

- GCM reads `config.yaml`, profile files, and templates from `~/.gcm/`. Upgrades never delete or migrate these files.
- New versions may add fields to config or profile YAML. Missing fields use defaults — your existing files continue to work.
- If a breaking change is ever required, the changelog and release notes will include migration instructions.

---

## Uninstalling

Follow these steps to cleanly remove GCM from your system.

### Step 1: Export Your Data (Optional)

Before removing GCM, save your profiles and configuration:

```bash
# Create a final backup
gcm backup create

# Copy the backup somewhere safe
cp ~/.gcm/backups/gcm-backup-*.tar.gz ~/Desktop/
```

### Step 2: Remove Shell Integration

Remove the GCM shell hook from your shell config file.

**Bash** (`~/.bashrc`):
```bash
# Remove the block between these markers:
# >>> GCM shell integration >>>
# ... (auto-generated code)
# <<< GCM shell integration <<<
```

**Zsh** (`~/.zshrc`):
```bash
# Remove the block between these markers:
# >>> GCM shell integration >>>
# ... (auto-generated code)
# <<< GCM shell integration <<<
```

**Fish** (`~/.config/fish/config.fish`):
```bash
# Remove the block between these markers:
# >>> GCM shell integration >>>
# ... (auto-generated code)
# <<< GCM shell integration <<<
```

**PowerShell** (`$PROFILE`):
```powershell
# Remove the block between these markers:
# >>> GCM shell integration >>>
# ... (auto-generated code)
# <<< GCM shell integration <<<
```

### Step 3: Revert Git Configuration

If you want to reset your Git identity to manual management:

```bash
# Review the markers before deleting them — a .gcm-profile is a file you wrote
# to pin a profile to a project, not something GCM created behind your back
find ~/projects -name ".gcm-profile"
find ~/projects -path "*/.git/gcm-session"

# Set your Git identity manually
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
```

**What `scripts/uninstall.sh` does here, and what it will not do.**

It removes `user.name`, `user.email`, `user.signingkey` and `core.sshCommand`
from your global git config **only when the current value matches one of your
GCM profiles** — that is what proves GCM wrote it. A value it cannot attribute
is listed and left in place, along with the command to remove it yourself.
`commit.gpgsign`, `gpg.format`, `gpg.program` and the `tag.*` keys are settings
rather than identity and cannot be attributed at all, so they are always
reported rather than removed.

This runs before `~/.gcm` is deleted, so the profiles are still there to
compare against. If you remove the data directory first, the uninstaller has
nothing to match and will leave your git config alone.

Project markers are listed and confirmed before deletion rather than swept
away.

**Credential helpers.** Registering GCM resets `credential.<host>.helper` so
its helper wins; whatever you had before is parked under
`gcm.saved.<host>.helper` and restored when GCM is unregistered. To check what
is stored:

```bash
git config --global --get-all "gcm.saved.https://github.com.helper"
```

### Step 4: Remove GCM Data Directory

```bash
rm -rf ~/.gcm
```

This removes:
- Configuration (`config.yaml`)
- All profiles (`profiles/`)
- All templates (`templates/`)
- Encrypted tokens (`tokens/`)
- Backups (`backups/`)
- Audit logs (`logs/`)
- Cache (`cache/`)

> **Warning**: This is irreversible. Make sure you've exported anything you need (Step 1).

### Step 5: Remove GitHub Tokens from Keychain

If you used keychain storage for provider tokens:

GCM writes every token under a single service label, **`git-config-manager`**.
The account name identifies the entry:
`<profile>__<provider>__<host>__<account>` — for example
`personal__gitlab__gitlab_com__default`. Tokens from before provider-aware
storage use the bare profile name as the account.

**macOS**:
```bash
# List what GCM stored
security find-generic-password -s "git-config-manager"

# Remove entries one at a time (repeat until none remain)
while security delete-generic-password -s "git-config-manager" 2>/dev/null; do :; done
```

Do **not** delete by `-s github.com`: that service holds credentials written by
your browser, `git-credential-osxkeychain` and other tools, none of which
belong to GCM.

**Linux**:
```bash
secret-tool clear service git-config-manager
```

**Windows**:
```powershell
# Open Credential Manager → Windows Credentials
# Find and remove entries whose target contains "git-config-manager"
cmdkey /list | Select-String "git-config-manager"
```

`scripts/uninstall.sh` performs the macOS cleanup for you, scoped to the
`git-config-manager` service only.

### Step 6: Remove the Binary

**If installed from source** (`make install`):
```bash
rm /usr/local/bin/gcm
```

**If installed via `go install`**:
```bash
rm $(go env GOPATH)/bin/gcm
```

### Step 7: Revoke GitHub OAuth Tokens

Visit https://github.com/settings/applications and revoke the GCM OAuth app authorization.

---

## Verification

After uninstalling, verify GCM is fully removed:

```bash
# Binary removed?
which gcm
# Should return "gcm not found"

# Data directory removed?
ls ~/.gcm
# Should return "No such file or directory"

# Shell hook removed?
grep -r "GCM shell integration" ~/.bashrc ~/.zshrc ~/.config/fish/config.fish 2>/dev/null
# Should return nothing
```

---

## Re-installing After Uninstall

If you change your mind, reinstalling is straightforward:

```bash
# Install
go install github.com/justjundana/git-config-manager/cmd/gcm@latest

# Restore from backup
gcm backup restore ~/Desktop/gcm-backup-YYYY-MM-DD.tar.gz

# Re-setup shell integration
gcm init

# Re-authenticate GitHub
gcm github login work
```

---

## See Also

- [Installation](installation.md) — install methods
- [Quick Start](quick-start.md) — initial setup guide
- [FAQ](faq.md) — common questions
