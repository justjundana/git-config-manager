# Examples

Real-world workflows, recipes, and patterns for using GCM effectively.

---

## Basic Workflows

### First-Time Setup

```bash
# Install
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager && make build && make install

# Set up shell integration
gcm init
exec $SHELL

# Health check
gcm doctor

# Create your first profile
gcm profile create work --interactive
gcm use work --global
```

### Day Job + Personal

```bash
# Create both profiles
gcm profile create work -i
gcm profile create personal -i

# Pin to project directories
cd ~/projects/company && gcm use work --local
cd ~/projects/personal && gcm use personal --local

# Now auto-switching works on cd
cd ~/projects/company     # → (work) prompt
cd ~/projects/personal    # → (personal) prompt
```

### Freelancer with Multiple Clients

```bash
gcm profile create personal -i
gcm profile create client-acme -i
gcm profile create client-globex -i
gcm profile create client-stark -i

# Pin each client's repos
cd ~/clients/acme && gcm use client-acme --local
cd ~/clients/globex && gcm use client-globex --local
cd ~/clients/stark && gcm use client-stark --local
```

### Multiple GitHub Organizations

```bash
gcm profile create gh-personal -i
gcm profile create gh-company -i

# Each profile has its own SSH key, so GitHub sees them as separate identities
gcm ssh generate gh-personal
gcm ssh generate gh-company

# Keys are auto-uploaded to the profile provider if you're logged in (gcm connect <profile> --provider github)
# Otherwise, upload manually:
gcm ssh copy gh-personal | pbcopy  # paste into GitHub settings
gcm ssh copy gh-company | pbcopy
```

---

## SSH Key Workflows

### Generate Keys for All Profiles

```bash
# Ed25519 (recommended, default)
gcm ssh generate work

# RSA 4096 (for legacy systems)
gcm ssh generate legacy-server -t rsa -b 4096

# ECDSA
gcm ssh generate ecdsa-profile -t ecdsa

# With passphrase (encrypted at rest)
gcm ssh generate secure-work -p "my-strong-passphrase"

# With custom comment
gcm ssh generate work -c "jane@work-laptop-2026"
```

### Test SSH Connection

```bash
gcm ssh test work
# ✓ SSH connection successful!
# Hi jane-acme! You've successfully authenticated...
```

### Copy Public Key for GitHub Upload

```bash
# Print to terminal
gcm ssh copy work

# macOS: copy to clipboard
gcm ssh copy work | pbcopy

# Linux: copy to clipboard (xclip)
gcm ssh copy work | xclip -selection clipboard

# Save to file
gcm ssh copy work > /tmp/work-key.pub
```

---

## GPG Signing Workflows

### Enable GPG Signing

```bash
# 1. Generate GPG key (uses profile's name and email)
gcm gpg generate work

# 2. Verify it works
gcm gpg test work

# 3. Enable auto-signing (done automatically by generate, but can toggle)
gcm gpg sign enable work

# 4. Upload to GitHub for verified commits (auto-prompted during generate if logged in)
#    If not logged in yet:
gcm github login work
```

### Disable Signing for a Profile

```bash
gcm gpg sign disable personal
```

### List All GPG Keys

```bash
gcm gpg list
```

---

## GitHub Authentication Workflows

### First-Time Login (OAuth Device Flow)

```bash
gcm github login-oauth work
# 🌐 GitHub Login for Profile: work
#
# Please open the following URL in your browser:
#   https://github.com/login/device
#
# Enter code: ABCD-1234
#
# ⏳ Waiting for authorization...
# ✓ Authorization successful!
# ✓ Logged in as jane-acme (Jane Doe)
```

### First-Time Login (Personal Access Token)

```bash
gcm github login work
# 🔑 GitHub Login (PAT) for Profile: work
#
# Enter your Personal Access Token:
# ✓ Token verified
# ✓ Logged in as jane-acme (Jane Doe)
```

### Verify Authentication

```bash
gcm github verify work
# ✓ Authenticated as jane-acme
```

### View GitHub Profile

```bash
gcm github user work
# GitHub User: jane-acme
#   Name:     Jane Doe
#   Email:    jane@acme.example
#   Company:  Acme Corp
#   Repos:    42
#   URL:      https://github.com/jane-acme
```

### Re-authenticate (Token Expired)

```bash
gcm github logout work
gcm github login work       # or: gcm github login-oauth work
```

### Credential Isolation Between Profiles

```bash
# Login both profiles
gcm github login work       # stores work token
gcm github login personal   # stores personal token

# Switch to work — git now authenticates as work account only
gcm use work
git clone https://github.com/acme-corp/private-repo.git   # ✓ works

# Switch to personal — git now authenticates as personal account only
gcm use personal
git clone https://github.com/jane/my-project.git          # ✓ works
git clone https://github.com/acme-corp/private-repo.git   # ✗ access denied (correct!)
```

> **How it works:** When GCM's credential helper is registered, git asks `gcm credential-helper` for the active profile's provider token. In legacy system-helper mode, `gcm use` calls `git credential reject`/`approve` and pins the provider-host `credential.*.username` value to prevent fallback to other stored credentials. Use `gcm auth inspect <profile>` to see whether the effective credential is GCM-owned or external.

---

## Template Workflows

### Create a Template

```bash
# Interactive wizard
gcm template create company-standard -i

# From an existing profile's settings
gcm template create company-standard --from-profile work

# From flags
gcm template create company-standard --editor "code --wait" --rebase true --gpg-sign true
```

### Share Team Configuration

```bash
# Create a template file
cat > company-standard.yaml << 'EOF'
name: company-standard
description: "Standard config for Acme Corp engineers"
git:
  core:
    editor: code
  commit:
    gpgsign: true
  pull:
    rebase: "true"
  push:
    autosetupremote: true
metadata:
  author: "Platform Team"
  version: "1.0"
EOF

# Import the template
gcm template import company-standard.yaml

# Share the file with your team (via Git, Slack, etc.)
# Team members import it on their machines
```

### Use a Template

```bash
# List available templates
gcm template list

# View a template
gcm template show company-standard

# Apply template settings to an existing profile
gcm template apply company-standard work

# Or create a new profile with template settings pre-applied
gcm profile create work --from-template company-standard -i
```

### Export and Share

```bash
gcm template export company-standard > company-standard.yaml
# Send to teammates
```

---

## Backup & Restore Workflows

### Regular Backups

```bash
# Create a backup
gcm backup create
# ✓ Backup created!
#   Path:      ~/.gcm/backups/gcm-backup-2026-05-18-143000.tar.gz
#   Profiles:  3
#   Templates: 1
#   Size:      2.1 KB

# List all backups
gcm backup list
```

### Restore After Disaster

```bash
# List available backups
gcm backup list

# Restore (prompts for confirmation)
gcm backup restore ~/.gcm/backups/gcm-backup-2026-05-18-143000.tar.gz
```

### Prune Old Backups

```bash
# Keep only the last 5 (default)
gcm backup prune

# Keep only the last 3
gcm backup prune --keep 3
```

### Migrate to a New Machine

```bash
# On the old machine
gcm backup create
scp ~/.gcm/backups/gcm-backup-*.tar.gz user@new-machine:~/

# On the new machine
make install   # install GCM
gcm backup restore ~/gcm-backup-*.tar.gz
gcm init       # set up shell integration

# Re-authenticate GitHub (tokens are not backed up)
gcm github login work
gcm github login personal
```

---

## Profile Management Workflows

### Compare Profiles

```bash
gcm profile diff work personal
# Comparing: work vs personal
#
# Name:
#   work:     Jane Doe
#   personal: jane
#
# Email:
#   work:     jane@acme.example
#   personal: jane@gmail.com
```

### Export and Import Profiles

```bash
# Export
gcm profile export work > work-backup.yaml

# Edit externally
vim work-backup.yaml

# Import (creates a new profile or overwrites)
gcm profile import work-backup.yaml
```

### Rename a Profile

```bash
# GCM has no built-in rename, but export → edit → import works:
gcm profile export old-name > /tmp/profile.yaml
# Edit the "name:" field in the file
vim /tmp/profile.yaml
gcm profile import /tmp/profile.yaml
gcm profile delete old-name -y

# Update any local pins
cd ~/projects/that-repo
gcm use new-name --local
```

### Batch Profile Inspection

```bash
# Validate all profiles at once
gcm validate

# List with status
gcm profile list

# Show details for each
for p in work personal client-acme; do
  echo "=== $p ==="
  gcm profile show $p
  echo
done
```

---

## Shell Integration Workflows

### Pin Profile to a Monorepo Subdirectory

```bash
cd ~/monorepo/services/auth
gcm use work --local

cd ~/monorepo/services/oss-lib
gcm use personal --local
```

### Temporary Override

```bash
# You're in a "work" directory but need to make a personal commit
gcm use personal       # session only, doesn't touch .gcm-profile
git commit -m "fix"
gcm refresh            # re-activate the local profile
```

### Check Active Profile in Scripts

```bash
#!/bin/bash
profile=$(gcm current --short)
if [ "$profile" != "work" ]; then
  echo "Warning: not using work profile!"
  exit 1
fi
git push origin main
```

---

## CI/CD Integration

### GitHub Actions

```yaml
# .github/workflows/verify-identity.yml
name: Verify Git Identity
on: push

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Check commit author
        run: |
          AUTHOR_EMAIL=$(git log -1 --format='%ae')
          if [[ "$AUTHOR_EMAIL" != *"@acme.example" ]]; then
            echo "::error::Commit must use @acme.example email"
            exit 1
          fi
```

### Pre-Commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit
# Ensure the correct profile is active before committing

EXPECTED_EMAIL="jane@acme.example"
ACTUAL_EMAIL=$(git config user.email)

if [ "$ACTUAL_EMAIL" != "$EXPECTED_EMAIL" ]; then
  echo "ERROR: Wrong Git identity!"
  echo "Expected: $EXPECTED_EMAIL"
  echo "Actual:   $ACTUAL_EMAIL"
  echo ""
  echo "Run: gcm use work"
  exit 1
fi
```

### Makefile Integration

```makefile
# Makefile
.PHONY: check-identity
check-identity:
	@profile=$$(gcm current --short 2>/dev/null); \
	if [ "$$profile" != "work" ]; then \
		echo "Switch to work profile: gcm use work"; \
		exit 1; \
	fi

commit: check-identity
	git add -A && git commit
```

---

## Diagnostic Workflows

### Full System Check

```bash
gcm doctor
# 🏥 GCM System Health Check
#
# System
#   ✓ OS: darwin/arm64
#   ✓ Go: go1.26.0
#
# Dependencies
#   ✓ Git: git version 2.44.0
#   ✓ SSH: OpenSSH_9.6p1
#   ✓ GPG: gpg (GnuPG) 2.4.5
#
# Services
#   ✓ SSH Agent: running
#
# Configuration
#   ✓ Config: ~/.gcm/config.yaml
#   ✓ Profiles: 3
#   ✓ Templates: 1
```

### Validate All Profiles

```bash
gcm validate
# ✓ Profile: work
#   ✓ Git: Name and email configured
#   ✓ SSH: Key exists with correct permissions
#   ✓ GPG: Signing enabled
#
# ⚠ Profile: personal
#   ✓ Git: Name and email configured
#   ⚠ SSH: Key not found at ~/.ssh/id_ed25519_personal
```

### Clean Up

```bash
gcm clean            # remove cache
gcm clean --all      # remove cache + logs
gcm backup prune     # remove old backups
```

---

## See Also

- [Commands Reference](commands.md) — every flag and option
- [Shell Integration](shell-integration.md) — auto-switching details
- [Configuration](configuration.md) — config.yaml and profile schema
- [Troubleshooting](troubleshooting.md) — when things go wrong
