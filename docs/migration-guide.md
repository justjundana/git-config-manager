# Migration Guide

How to migrate to GCM from manual Git identity management or other tools.

---

## From Manual Git Config

If you're currently managing Git identities with `git config` commands, switching to GCM is straightforward.

### Step 1: Record Your Current Setup

```bash
# Global identity
git config --global user.name
git config --global user.email
git config --global user.signingkey
git config --global core.editor

# Check SSH keys
ls -la ~/.ssh/id_*

# Check per-repo overrides
cd ~/projects/work-repo
git config user.name
git config user.email
```

### Step 2: Create Profiles

Create a GCM profile for each identity you use:

```bash
# Main identity
gcm profile create personal \
  --name "$(git config --global user.name)" \
  --email "$(git config --global user.email)"

# Work identity (if you have per-repo overrides)
gcm profile create work \
  --name "Jane Doe" \
  --email "jane@company.example" \
  --editor "code --wait"
```

### Step 3: Link Existing SSH Keys

If you already have SSH keys, reference them when creating a profile:

```bash
gcm profile create personal --ssh-key ~/.ssh/id_ed25519
gcm profile create work --ssh-key ~/.ssh/id_ed25519_work
```

Or if profiles already exist, export → edit → reimport:

```bash
gcm profile export work > /tmp/work.yaml
# Edit /tmp/work.yaml: set ssh.key_path to ~/.ssh/id_ed25519_work
gcm profile delete work -y && gcm profile import /tmp/work.yaml
```

### Step 4: Set Up Auto-Switching

```bash
# Install shell integration
gcm init
exec $SHELL

# Pin your projects
echo "work" > ~/projects/work-repo/.gcm-profile
echo "personal" > ~/projects/oss-repo/.gcm-profile
```

### Step 5: Verify

```bash
cd ~/projects/work-repo
gcm current --short     # → work
git config user.email   # → jane@company.example

cd ~/projects/oss-repo
gcm current --short     # → personal
git config user.email   # → jane@personal.example
```

### What You Can Remove

After verifying, you can clean up manual config:

```bash
# Remove per-repo identity overrides (GCM manages these now)
cd ~/projects/work-repo
git config --unset user.name
git config --unset user.email

# Keep your global config as a fallback
# GCM's "default_profile" replaces it functionally
```

---

## From Shell Functions / Aliases

Many developers use shell functions like:

```bash
# Old approach
git-work() {
  git config --global user.name "Jane Doe"
  git config --global user.email "jane@company.example"
  ssh-add ~/.ssh/id_ed25519_work
}

git-personal() {
  git config --global user.name "janedoe"
  git config --global user.email "jane@personal.example"
  ssh-add ~/.ssh/id_ed25519_personal
}
```

### Migration Steps

1. **Create equivalent profiles:**

```bash
gcm profile create work \
  --name "Jane Doe" \
  --email "jane@company.example" \
  --ssh-key ~/.ssh/id_ed25519_work

gcm profile create personal \
  --name "janedoe" \
  --email "jane@personal.example" \
  --ssh-key ~/.ssh/id_ed25519_personal
```

2. **Replace shell functions with GCM commands:**

```bash
# Old: git-work
# New: gcm use work

# Old: git-personal
# New: gcm use personal
```

3. **Remove old functions from `~/.bashrc` / `~/.zshrc`**

4. **Set up auto-switching** (eliminates manual switching entirely):

```bash
gcm init
echo "work" > ~/projects/work/.gcm-profile
echo "personal" > ~/projects/oss/.gcm-profile
```

---

## From Git Conditional Includes

If you're using Git's `includeIf` for directory-based identity:

```ini
# ~/.gitconfig (old approach)
[user]
    name = Jane Doe
    email = jane@personal.example

[includeIf "gitdir:~/work/"]
    path = ~/.gitconfig-work
```

```ini
# ~/.gitconfig-work
[user]
    name = Jane Doe
    email = jane@company.example
```

### Migration Steps

1. **Create profiles matching each identity:**

```bash
gcm profile create personal \
  --name "Jane Doe" \
  --email "jane@personal.example"

gcm profile create work \
  --name "Jane Doe" \
  --email "jane@company.example"
```

2. **Pin directories:**

```bash
# Replace includeIf with .gcm-profile files
find ~/work -maxdepth 1 -type d -exec sh -c 'echo "work" > "$1/.gcm-profile"' _ {} \;
```

3. **Set up shell integration:**

```bash
gcm init && exec $SHELL
```

4. **Remove old includeIf blocks** from `~/.gitconfig` once verified.

### Why GCM Over `includeIf`?

| Feature | `includeIf` | GCM |
|---------|------------|-----|
| Git name/email | ✅ | ✅ |
| SSH key switching | ❌ | ✅ |
| GPG key switching | ❌ | ✅ |
| GitHub token switching | ❌ | ✅ |
| Editor per identity | ❌ | ✅ |
| Visual confirmation | ❌ | ✅ (prompt indicator) |
| Backup/restore | ❌ | ✅ |
| Template sharing | ❌ | ✅ |
| Audit logging | ❌ | ✅ |

---

## From Dotfile Managers (chezmoi, stow, dotbot)

Dotfile managers handle ALL dotfiles. GCM focuses specifically on Git identity. They can coexist.

### Using GCM Alongside chezmoi

```bash
# chezmoi manages: ~/.bashrc, ~/.vimrc, ~/.config/*, etc.
# GCM manages: Git identity, SSH keys, GPG signing, GitHub auth

# Add GCM's config to chezmoi
chezmoi add ~/.gcm/config.yaml
chezmoi add ~/.gcm/profiles/
chezmoi add ~/.gcm/templates/

# Don't add tokens (they're encrypted/keychained)
echo ".gcm/tokens/" >> ~/.chezmoiignore
```

### Using GCM Alongside GNU Stow

```bash
# Stow manages dotfiles via symlinks
# GCM manages Git identity separately

# GCM's ~/.gcm/ is independent of stow
# No conflict — different files
```

### When to Use GCM vs. Dotfile Managers

| Need | Tool |
|------|------|
| Sync all dotfiles across machines | chezmoi / stow |
| Switch Git identity per project | **GCM** |
| Manage SSH keys per identity | **GCM** |
| GPG signing per identity | **GCM** |
| GitHub OAuth per identity | **GCM** |
| Auto-switch on `cd` | **GCM** |

---

## General Migration Checklist

Use this checklist regardless of your starting point:

- [ ] List all Git identities you use (name, email, SSH key, GPG key)
- [ ] Install GCM (`go install` or build from source)
- [ ] Run `gcm doctor` to verify prerequisites
- [ ] Create a profile for each identity
- [ ] Link existing SSH keys to profiles (or generate new ones)
- [ ] Set up GPG signing if needed
- [ ] Authenticate GitHub for each profile
- [ ] Install shell integration (`gcm init`)
- [ ] Pin each project directory (`.gcm-profile`)
- [ ] Verify auto-switching works (`cd` between projects)
- [ ] Back up your new setup (`gcm backup create`)
- [ ] Remove old manual configuration (shell functions, includeIf, etc.)

---

## Post-Migration Verification

```bash
# Check all profiles
gcm profile list

# Verify each profile's identity
for profile in $(gcm profile list --short 2>/dev/null); do
  echo "--- $profile ---"
  gcm profile show "$profile"
done

# Test auto-switching
cd ~/projects/work-app && gcm current --short
cd ~/projects/personal-app && gcm current --short

# Run diagnostics
gcm doctor
```

---

## Rollback

If you need to go back to your old setup:

1. **Remove shell integration:**
   Edit `~/.zshrc` (or equivalent) and remove the `>>> GCM shell integration >>>` block.

2. **Restore old Git config:**
   ```bash
   git config --global user.name "Your Name"
   git config --global user.email "you@example.com"
   ```

3. **Remove GCM data (optional):**
   ```bash
   rm -rf ~/.gcm
   ```

Your SSH keys, GPG keys, and Git history are **never modified** by GCM. Only Git config and shell hooks are changed.

---

## See Also

- [Installation](installation.md) — install methods
- [Quick Start](quick-start.md) — 5-minute setup
- [Configuration](configuration.md) — config file reference
- [FAQ](faq.md) — common questions
- [Upgrade & Uninstall](upgrade-uninstall.md) — removing GCM
