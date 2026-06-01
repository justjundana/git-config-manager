# Upgrade & Uninstall

How to update GCM to a new version or remove it completely.

---

## Upgrading

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
# Remove any .gcm-profile files from your projects
find ~/projects -name ".gcm-profile" -delete

# Remove session markers from git repositories
find ~/projects -path "*/.git/gcm-session" -delete

# Set your Git identity manually
git config --global user.name "Your Name"
git config --global user.email "you@example.com"
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

If you used keychain storage for GitHub tokens:

**macOS**:
```bash
# Open Keychain Access app and search for "gcm"
# Or use security CLI:
security delete-generic-password -s "gcm-github-token-work" 2>/dev/null
security delete-generic-password -s "gcm-github-token-personal" 2>/dev/null
```

**Linux**:
```bash
# Use secret-tool to remove stored tokens
secret-tool clear service gcm-github-token-work
secret-tool clear service gcm-github-token-personal
```

**Windows**:
```powershell
# Open Credential Manager → Windows Credentials
# Find and remove entries starting with "gcm-github-token-"
```

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
