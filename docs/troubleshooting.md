# Troubleshooting

## Diagnostics

Always start with the built-in health check:

```bash
gcm doctor       # check Git, SSH, GPG, config, and shell integration
gcm validate     # validate all profiles
gcm validate work  # validate a specific profile
```

`doctor` checks system dependencies and environment. `validate` inspects profile YAML structure, referenced SSH/GPG paths, and permissions.

---

## Profile Issues

### "profile not found"

The profile doesn't exist or is misspelled:

```bash
gcm profile list                    # see available profiles
gcm profile create <name> -i       # create it
```

Profile commands use exact-case lookup, but GCM rejects new profiles that differ only by case from an existing one. Run `gcm profile list` and use the casing shown there.

### "cannot delete active profile"

Switch to a different profile before deleting:

```bash
gcm use <other-profile>
gcm profile delete <profile>
```

### Profile not activating

Use `gcm use` and check the output:

```bash
gcm use work
gcm current       # confirm it activated
```

If `gcm current` shows no active profile, the profile may have validation errors:

```bash
gcm validate work
```

### Profile YAML corrupted

Restore from backup:

```bash
gcm backup list
gcm backup restore <backup-file>
```

Or manually inspect and edit the profile:

```bash
cat ~/.gcm/profiles/work.yaml
# fix the YAML manually, or delete and recreate
gcm profile delete work -y
gcm profile create work -i
```

---

## SSH Issues

### SSH key file exists but profile has no SSH key configured

This can happen after manually deleting GCM profiles/config while leaving `~/.ssh/id_<type>_<profile>_<provider>` files behind.

```bash
gcm ssh generate personal
```

If the leftover key matches GCM's deterministic provider-aware filename, GCM links it back to the profile automatically. The same recovery runs before `gcm ssh upload`, `gcm ssh test`, and `gcm ssh copy` report a missing SSH key.

To intentionally replace the old local key pair, use:

```bash
gcm ssh generate personal --overwrite
```

Overwrite keeps the same local filename and provider upload title, but the key material changes. If the old public key was already uploaded to GitHub/GitLab, upload the new key afterward and remove the old remote key if the provider rejects duplicate titles.

### Permission errors: "Permissions are too open"

SSH requires strict file permissions:

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_ed25519_*        # private keys
chmod 644 ~/.ssh/id_ed25519_*.pub    # public keys
```

GCM sets correct permissions when generating keys. This only happens with keys imported from elsewhere.

### SSH agent not running

Start the agent:

```bash
eval "$(ssh-agent -s)"
```

On macOS, the agent usually runs automatically. On Linux, add the above to your shell config if needed.

### SSH test fails

```bash
gcm ssh test work
```

If this fails:
1. Verify the key exists: `ls -la ~/.ssh/id_ed25519_work`
2. Verify the public key is uploaded to GitHub: Settings → SSH and GPG keys
3. Test manually: `ssh -T -i ~/.ssh/id_ed25519_work git@github.com`

### Key not added to agent after `gcm use`

`gcm use` attempts to load the SSH key into the agent. If it fails silently, try manually:

```bash
ssh-add ~/.ssh/id_ed25519_work
```

If the key has a passphrase, you'll be prompted for it.

---

## GPG Issues

### GPG not installed

Install GPG for your platform:

| Platform       | Command                       |
| -------------- | ----------------------------- |
| macOS          | `brew install gnupg`          |
| Ubuntu/Debian  | `sudo apt install gnupg`      |
| Fedora         | `sudo dnf install gnupg2`     |
| Arch           | `sudo pacman -S gnupg`        |
| Windows        | Download from gnupg.org       |

### GPG signing test fails

```bash
gcm gpg test work
```

If this fails:
1. Check the key exists: `gpg --list-secret-keys`
2. Verify the key ID in the profile matches: `gcm profile show work`
3. Test manually: `echo "test" | gpg --clearsign`

### "No GPG key configured"

Generate one first:

```bash
gcm gpg generate work
```

This uses the name and email from the profile, so make sure those are set.

---

## GitHub Issues

### "Could not connect to GitHub" during login

This means the OAuth device flow could not start. Common causes:

1. **Invalid client_id** — The most common cause. Check `~/.gcm/config.yaml`:
   ```yaml
   github:
     oauth:
       client_id: "YOUR_REAL_CLIENT_ID"  # Must be a registered GitHub OAuth App
   ```
   Register an OAuth App at: https://github.com/settings/developers

2. **No internet** — Verify you can reach `github.com`

3. **Use an alternative login method** (no OAuth App needed):
   ```bash
   gcm github login work        # Personal Access Token
   gcm github login-gh work      # Import from GitHub CLI
   ```

### OAuth device flow timeout

The device flow has a **15-minute deadline**. If you missed it:

```bash
gcm github login-oauth work    # start a fresh flow
```

Make sure you:
1. Open the **exact URL** GCM printed
2. Enter the **exact user code** shown
3. Approve the authorization in your browser

### "Profile is not authenticated"

This means you haven't logged in yet for this profile:

```bash
gcm auth status work --provider github  # confirm GCM vs external auth state
gcm github login work              # Personal Access Token
gcm github login-oauth work        # OAuth device flow (browser-based)
gcm github login-gh work           # import from GitHub CLI
```

### SSH key upload says the key is already in use

Providers allow one owner for each SSH public key. If `gcm ssh upload <profile>` says the key is already registered but it was not found on the authenticated account, the same public key is attached to another provider account or as a deploy key.

Remove the key from the other owner, or replace the local profile key and upload the new one:

```bash
gcm ssh generate <profile> --overwrite
gcm ssh upload <profile>
```

### Token expired or revoked

If `gcm github verify` says the token is invalid:

```bash
gcm github logout work    # remove the old token
gcm github login work     # re-authenticate
```

### Git credentials bleed between profiles

GCM automatically isolates git credentials per profile when you `gcm use`. If you still see the wrong account:

1. **Re-activate the profile** to force credential refresh:
   ```bash
   gcm use <correct-profile>
   ```

2. **Check credential pinning** is set:
   ```bash
   # Replace github.com with the profile's provider host when needed.
   git config --global credential.https://github.com.username
   # Should show the active profile's provider username
   ```

3. **Clear stale credentials** from the OS credential store:
   ```bash
   gcm auth logout <wrong-profile> --scope external --dry-run
   gcm github logout <wrong-profile> --clear-credentials
   ```
   This clears HTTPS credentials and provider username pinning only. SSH remotes and profile SSH keys are unchanged.

4. **Inspect credential ownership and source:**
   ```bash
   gcm auth inspect <correct-profile> --provider github
   ```

5. **macOS Keychain interference:** Open Keychain Access → search "github.com" → delete old entries that weren't managed by GCM. Prefer the `gcm auth logout --scope external --dry-run` preview first.

6. **Verify which account git sees:**
   ```bash
   # For HTTPS:
   echo "protocol=https\nhost=github.com\n" | git credential fill
   # For SSH:
   ssh -T git@github.com
   ```

### Login updated wrong profile's credentials

GCM only stores git credentials during login if the logged-in profile is the currently **active** one. If you login a non-active profile, only the encrypted token is saved — git credentials remain unchanged until you `gcm use` that profile.

### Git push fails after VS Code logout

If `git push` suddenly fails with authentication errors after logging out of GitHub in VS Code:

**Root cause:** VS Code clears the macOS Keychain (or Windows Credential Manager) entry for `github.com` on logout. If git is configured to use `osxkeychain` or `wincred`, the token is gone.

**Solution:** Register GCM's built-in credential helper, which serves tokens from its own encrypted store instead of the system keychain:

```bash
gcm init
```

This configures git to use `gcm credential-helper` for configured provider hosts, making git auth immune to external credential changes. Verify with:

```bash
gcm doctor
gcm auth inspect work --provider github
```

If `git push` works but GCM says the profile is not authenticated, Git may be using an external credential that GCM does not own. Use `gcm auth inspect <profile> --provider github` to see the source. If the source is `gcm-store`, Git is being served by GCM's own credential helper for the active profile; use `gcm use <profile>` or login the target profile instead of adopting it as external auth. If the external credential is exportable and matches the intended account, adopt it explicitly:

```bash
gcm auth adopt work --provider github --dry-run
gcm auth adopt work --provider github --yes
```

If you do not want GCM to own that credential, leave it external; status will report true non-GCM credentials as `authenticated:external` instead of pretending they are GCM-managed.

---

## Shell Integration Issues

### Auto-switch not working

See [Shell Integration — Troubleshooting](shell-integration.md#troubleshooting) for detailed steps.

Quick checklist:
1. **Integration installed?** `grep "GCM shell integration" ~/.zshrc`
2. **Shell restarted?** `exec $SHELL`
3. **`.gcm-profile` exists?** `cat .gcm-profile`
4. **Profile exists?** `gcm profile list`

### `gcm init` says "already installed"

Remove the existing block first, then reinstall:

1. Open your shell config (`~/.zshrc`, `~/.bashrc`, etc.)
2. Delete everything between `# >>> GCM shell integration >>>` and `# <<< GCM shell integration <<<`
3. Run `gcm init` again

---

## General Issues

### Clearing cache

```bash
gcm clean          # remove the cache directory
gcm clean --all    # also remove the logs directory
```

### Full reset

If GCM state is severely broken, back up and start fresh:

```bash
# 1. Save a backup first
gcm backup create
cp ~/.gcm/backups/gcm-*.tar.gz ~/safe-place/

# 2. Remove the data directory
rm -rf ~/.gcm

# 3. Re-initialize
gcm doctor
gcm init
gcm profile create work -i
```

> **Warning:** `rm -rf ~/.gcm` deletes all profiles, tokens, templates, backups, and logs. Make sure you've saved anything important.

### Data directory location

GCM stores everything under `~/.gcm/`. See [configuration.md](configuration.md) for the full directory layout.

---

## Getting Help

```bash
# Built-in help
gcm --help
gcm <command> --help
gcm <command> <subcommand> --help

# Example
gcm profile --help
gcm ssh generate --help
```

### Report a Bug

If you've found a bug, please open an issue at:

https://github.com/justjundana/git-config-manager/issues

Include:
- Output of `gcm version`
- Output of `gcm doctor`
- Steps to reproduce
- Expected vs. actual behavior

---

## See Also

- [FAQ](faq.md) — common questions and answers
- [Commands Reference](commands.md) — complete CLI reference
- [Configuration](configuration.md) — config file reference
- [Shell Integration](shell-integration.md) — auto-switch setup
- [Examples](examples.md) — real-world workflows
