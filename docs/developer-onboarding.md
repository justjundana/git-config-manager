# Developer Onboarding

A guide for teams adopting GCM. Covers role-based setup, CI/CD integration, onboarding checklists, and team workflows.

---

## For Team Leads

### 1. Initial Setup

Before onboarding your team:

```bash
# Install GCM
make build && make install

# Create the team's standard template as a YAML file
cat > company-standard.yaml << 'EOF'
name: company-standard
description: "Standard Git config for our team"
git:
  core:
    editor: "code --wait"
  commit:
    gpgsign: true
  pull:
    rebase: "true"
  aliases:
    co: checkout
    br: branch
    st: status
metadata:
  version: "1.0"
  created: 2026-01-01T00:00:00Z
  updated: 2026-01-01T00:00:00Z
EOF

# Import it into GCM
gcm template import company-standard.yaml

# Verify
gcm template show company-standard
```

### 2. Create Project Configuration

Pin a profile name to each project repository:

```bash
cd ~/projects/company-app
echo "work" > .gcm-profile
```

**Should you commit `.gcm-profile`?**

| Scenario | Recommendation |
|----------|---------------|
| Solo project | ✅ Commit it |
| Small team, same profile names | ✅ Commit it |
| Large team, varied names | ❌ Add to `.gitignore` |
| Open source | ❌ Add to `.gitignore` |

If profile names vary across the team, add `.gcm-profile` to `.gitignore` and document the expected setup in your project's `CONTRIBUTING.md`.

### 3. Document Team Standards

Add a setup section to your project's `README.md` or `CONTRIBUTING.md`:

```markdown
## Git Identity Setup

This project uses [GCM](https://github.com/justjundana/git-config-manager)
for Git identity management.

### First-Time Setup

1. Install GCM: `go install github.com/justjundana/git-config-manager/cmd/gcm@latest`
2. Import the team template: `gcm template import company-standard.yaml`
3. Create your profile: `gcm profile create work -i`
4. Set up shell integration: `gcm init && exec $SHELL`
5. Pin this project: `echo "work" > .gcm-profile`
6. Connect the provider: `gcm connect work --provider github`
```

### 4. Set Up CI/CD

Example GitHub Actions workflow using GCM:

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Install GCM
        run: go install github.com/justjundana/git-config-manager/cmd/gcm@latest

      - name: Configure Git Identity
        run: |
          gcm profile create ci \
            --name "CI Bot" \
            --email "ci@example.com"
          gcm use ci --global

      - name: Build
        run: make build

      - name: Test
        run: make test
```

---

## For Team Members

### Onboarding Checklist

Follow these steps when joining a project that uses GCM:

- [ ] **Install GCM**
  ```bash
  go install github.com/justjundana/git-config-manager/cmd/gcm@latest
  ```

- [ ] **Verify installation**
  ```bash
  gcm version
  gcm doctor
  ```

- [ ] **Import team template** (if provided)
  ```bash
  gcm template import company-standard.yaml
  ```

- [ ] **Create your profile**
  ```bash
  gcm profile create work -i
  ```
  The interactive wizard prompts for name, email, SSH key, GPG signing, and one provider account. Review the team template (`gcm template show company-standard`) and match those settings.

- [ ] **Generate SSH key**
  ```bash
  gcm ssh generate work
  # Copy public key to GitHub: gcm ssh copy work
  ```

- [ ] **Connect provider authentication**
  ```bash
  gcm connect work --provider github
  ```

- [ ] **Set up shell integration**
  ```bash
  gcm init
  exec $SHELL
  ```

- [ ] **Pin your projects**
  ```bash
  cd ~/projects/company-app
  echo "work" > .gcm-profile
  ```

- [ ] **Verify everything works**
  ```bash
  gcm current           # should show "work"
  git config user.email # should show your work email
  gcm ssh test work     # should succeed
  ```

### Daily Workflow

Once set up, GCM is invisible in your daily workflow:

```bash
# Auto-switch happens on cd
cd ~/projects/work-app    # → work profile
cd ~/projects/personal    # → personal profile

# Check what's active
gcm current --short

# Manual switch if needed
gcm use work
```

---

## Common Team Scenarios

### New Team Member Joins

1. Team lead shares the template file or URL
2. New member follows the onboarding checklist above
3. Team lead verifies: `git log --format='%ae' -1` shows correct email

### Team Member Changes Email

```bash
gcm profile edit work -e "new-email@company.example"
gcm use work --global
# Verify
git config user.email
```

### Rotating SSH Keys

```bash
# Generate new key (auto-uploads to the connected provider if authenticated)
gcm ssh generate work -t ed25519

# If not logged in, upload manually:
gcm ssh copy work
# Paste in GitHub → Settings → SSH Keys

# Old key can be removed from GitHub
```

### Switching Between Projects

```bash
# If each project has .gcm-profile
cd ~/projects/client-a    # auto-switches to client-a profile
cd ~/projects/client-b    # auto-switches to client-b profile

# Or manually
gcm use client-a
```

---

## Best Practices

### 1. One Profile Per Identity

Don't try to share profiles between different Git identities. Create separate profiles:

```bash
gcm profile create work-main      # Main job
gcm profile create work-client     # Client project
gcm profile create personal        # Open source
```

### 2. Use Templates for Consistency

Templates store team standards (editor, signing, aliases) as a shareable YAML file:

```bash
# Team lead creates a template file and imports it
cat > acme-corp.yaml << 'EOF'
name: acme-corp
description: "ACME Corp standard Git settings"
git:
  core:
    editor: "code --wait"
  commit:
    gpgsign: true
  pull:
    rebase: "true"
metadata:
  version: "1.0"
  created: 2026-01-01T00:00:00Z
  updated: 2026-01-01T00:00:00Z
EOF
gcm template import acme-corp.yaml

# Team members: import and review, then create profile matching those settings
gcm template import acme-corp.yaml
gcm template show acme-corp
gcm profile create work -i    # set editor, signing, etc. to match the template
```

### 3. Pin Every Project

Add `.gcm-profile` to every project directory to prevent wrong-identity commits:

```bash
find ~/projects -maxdepth 1 -type d | while read dir; do
  echo "Which profile for $(basename $dir)?"
done
```

### 4. Back Up Regularly

```bash
# Add to crontab or run weekly
gcm backup create
gcm backup prune --keep 5
```

### 5. Run Doctor After Setup Changes

```bash
gcm doctor
```

---

## Team Communication Templates

### Template: Introducing GCM to Your Team

```
Subject: Git Identity Management with GCM

Team,

We're adopting GCM (Git Config Manager) for Git identity management.
This ensures everyone commits with the correct email and SSH key.

Setup takes ~5 minutes:
1. Install: go install github.com/justjundana/git-config-manager/cmd/gcm@latest
2. Create profile: gcm profile create work --interactive
3. Shell integration: gcm init && exec $SHELL
4. Pin projects: echo "work" > .gcm-profile

Full guide: [link to your CONTRIBUTING.md]

Benefits:
- No more "wrong email" commits
- Auto-switch on cd between projects
- Consistent SSH and GPG key management

Questions? Ask in #dev-tools.
```

### Template: Profile Update Required

```
Subject: GCM Profile Update Needed

Team,

Please update your GCM work profile:
- New email domain: @newdomain.example
- GPG signing now required

Steps:
1. gcm profile edit work (update email)
2. gcm gpg generate work (generate signing key)
3. gcm use work --global (re-activate)
4. Verify: git config user.email && git config user.signingkey

Deadline: [date]
```

---

## Metrics and Success Criteria

Track adoption with these metrics:

| Metric | How to Check | Target |
|--------|-------------|--------|
| All commits use correct email | `git log --format='%ae'` | 100% |
| Team members have profiles | Ask each member: `gcm profile list` | 100% |
| Shell integration active | `gcm doctor` on each machine | 100% |
| Backups exist | `gcm backup list` | ≥1 per member |
| SSH keys uploaded to GitHub | `gcm github verify work` | 100% |

---

## Troubleshooting

### "My teammate committed with the wrong email"

```bash
# Check their profile
gcm current
git config user.email

# Fix: ensure .gcm-profile exists in the repo
echo "work" > .gcm-profile

# Fix: rewrite the bad commit (only if not pushed)
git commit --amend --author="Name <correct@email.com>"
```

### "New member can't authenticate with GitHub"

```bash
# Re-run the device flow
gcm github login work

# Verify
gcm github verify work
gcm github user work
```

### "Profile auto-switch not working"

```bash
# Check shell integration
gcm doctor

# Re-install if needed
gcm init
exec $SHELL

# Verify .gcm-profile exists
cat .gcm-profile
```

---

## See Also

- [Quick Start](quick-start.md) — individual setup guide
- [Examples](examples.md) — workflow recipes
- [Configuration](configuration.md) — template and profile reference
- [Shell Integration](shell-integration.md) — auto-switch setup
- [FAQ](faq.md) — common questions
