# Requirements

System requirements and compatibility information for GCM.

---

## Overview

GCM is a static Go binary with minimal dependencies. Most features work out of the box. Optional features (GPG signing, provider integrations, OS keychain) require additional software.

---

## System Requirements

### Operating Systems

| OS | Architecture | Status |
|----|-------------|--------|
| macOS 12+ (Monterey) | amd64, arm64 (Apple Silicon) | ✅ Fully supported |
| Ubuntu 20.04+ / Debian 11+ | amd64, arm64 | ✅ Fully supported |
| Fedora 36+ / RHEL 9+ | amd64, arm64 | ✅ Fully supported |
| Arch Linux | amd64 | ✅ Fully supported |
| Windows 10/11 | amd64 | ✅ Fully supported |
| WSL 1/2 | amd64, arm64 | ✅ Supported (see notes) |
| FreeBSD | amd64 | ⚠️ Untested |

### Disk Space

| Component | Size |
|-----------|------|
| `gcm` binary | ~12-15 MB |
| `~/.gcm/` data directory | ~1-10 KB per profile |
| Backups | ~2-10 KB per archive |
| Total typical footprint | ~15-20 MB |

### Memory

GCM uses 8-25 MB RSS depending on the operation. See [Performance](performance.md) for details.

---

## Required Software

### Git (Required)

| | |
|-|-|
| **Minimum** | Git 2.20+ |
| **Recommended** | Git 2.40+ |
| **Used for** | All profile operations (`user.name`, `user.email`, `core.editor`, signing) |

```bash
# Verify
git --version
```

### SSH Client (Required for SSH features)

| | |
|-|-|
| **Component** | OpenSSH (`ssh`, `ssh-add`, `ssh-agent`) |
| **Minimum** | OpenSSH 7.0+ |
| **Used for** | `gcm ssh generate`, `gcm ssh list`, `gcm ssh test`, `gcm ssh copy` |

```bash
# Verify
ssh -V
```

**Platform notes:**
- **macOS**: Included by default
- **Linux**: Install `openssh-client` (`apt install openssh-client` / `dnf install openssh-clients`)
- **Windows**: Included in Windows 10 1809+, or install via `winget install Microsoft.OpenSSH.Client`

---

## Optional Software

### GPG (Optional — for commit signing)

| | |
|-|-|
| **Component** | GnuPG (`gpg`) |
| **Minimum** | GPG 2.0+ |
| **Recommended** | GPG 2.2+ |
| **Used for** | `gcm gpg generate`, `gcm gpg sign enable/disable` |

```bash
# Verify
gpg --version
```

**Platform notes:**
- **macOS**: `brew install gnupg`
- **Linux**: Usually pre-installed. If not: `apt install gnupg` / `dnf install gnupg2`
- **Windows**: Install [Gpg4win](https://gpg4win.org)

### OS Keychain (Optional — for token storage)

GCM uses the OS keychain as the preferred provider token storage backend.

| Platform | Keychain | Package |
|----------|----------|---------|
| macOS | Keychain Services | Built-in |
| Linux | Secret Service (D-Bus) | `gnome-keyring` or `kwallet` |
| Windows | Windows Credential Manager | Built-in |

```bash
# Linux: verify secret-service is available
dbus-send --session --dest=org.freedesktop.secrets \
  /org/freedesktop/secrets org.freedesktop.DBus.Peer.Ping 2>/dev/null && echo "OK"
```

If keychain is unavailable, GCM falls back only to AES-256-GCM encrypted file storage configured with a master password. Plain-text token files require explicit `security.allow_plaintext_tokens: true`. See [Security Model](security.md#token-storage-backends).

### Go Toolchain (Optional — for building from source)

| | |
|-|-|
| **Minimum** | Go 1.26.0 |
| **Used for** | Building GCM from source, running tests |

```bash
# Verify
go version
```

Not needed if using pre-built binaries.

---

## Shell Compatibility

| Shell | Auto-Switch | Prompt | Config File | Min Version |
|-------|------------|--------|-------------|-------------|
| Bash | ✅ | ✅ | `~/.bashrc` | 4.0+ |
| Zsh | ✅ | ✅ | `~/.zshrc` | 5.0+ |
| Fish | ✅ | ✅ | `~/.config/fish/config.fish` | 3.0+ |
| PowerShell | ✅ | ✅ | `$PROFILE` | 5.1+ / 7.0+ (Core) |
| sh / dash | ❌ | ❌ | — | Not supported |
| cmd.exe | ❌ | ❌ | — | Not supported |

```bash
# Check your shell
echo $SHELL
$SHELL --version
```

---

## Network Requirements

GCM is **offline by default**. Network access is only needed for:

| Feature | Endpoint | Port |
|---------|----------|------|
| `gcm connect --provider github` | `api.github.com` or configured GitHub API | 443 (HTTPS) |
| `gcm connect --provider gitlab` | `gitlab.com` or configured GitLab API | 443 (HTTPS) |
| `gcm github verify/user` | `api.github.com` or configured GitHub API | 443 (HTTPS) |
| `gcm gitlab verify/user` | `gitlab.com` or configured GitLab API | 443 (HTTPS) |
| `gcm ssh test` | Target SSH host | 22 (SSH) |

### Proxy Support

GCM uses Go's standard `net/http` client, which respects:

```bash
export HTTPS_PROXY=http://proxy.example.com:8080
export HTTP_PROXY=http://proxy.example.com:8080
export NO_PROXY=localhost,127.0.0.1
```

### Firewall Rules

If your network restricts outbound connections, allow:

```
github.com:443          # OAuth device flow
api.github.com:443      # GitHub API
```

---

## Permissions

### No Root Required

GCM never requires `root` or `sudo`. Everything operates in user space:

| Location | Permission | Purpose |
|----------|-----------|---------|
| `~/.gcm/` | `0755` | Data directory |
| `~/.gcm/tokens/` | `0700` | Encrypted tokens |
| `~/.gcm/logs/` | `0700` | Audit logs |
| `~/.gcm/backups/` | `0700` | Backup archives |
| `~/.ssh/id_*` (private) | `0600` | SSH private keys |
| `~/.ssh/id_*.pub` | `0644` | SSH public keys |

### PowerShell Execution Policy

On Windows, you may need:

```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

---

## Platform-Specific Notes

### macOS (Apple Silicon / M1+)

GCM runs natively on ARM64. No Rosetta needed.

```bash
# Verify native binary
file $(which gcm)
# Expected: Mach-O 64-bit executable arm64
```

### Windows Subsystem for Linux (WSL)

GCM works in WSL 1 and WSL 2. Notes:

- Install GCM **inside** WSL (not the Windows host)
- The Linux keychain (`gnome-keyring`) may not be available — configure master-password encrypted file storage for secure fallback
- Shell integration works normally inside WSL terminals
- SSH keys generated in WSL are separate from Windows host keys

### Docker / Containers

GCM can run in containers for CI/CD. Mount `~/.gcm/` as a volume:

```dockerfile
COPY --from=builder /usr/local/bin/gcm /usr/local/bin/gcm
RUN gcm version
```

Note: OS keychain is unavailable in containers. Use master-password encrypted file storage, or explicitly opt into `security.allow_plaintext_tokens: true` only when the container environment accepts that risk.

---

## Verification

Run these commands to verify your system meets all requirements:

```bash
# Required
git --version           # Git 2.20+
ssh -V                  # OpenSSH 7.0+
gcm version             # GCM installed

# Optional
gpg --version           # GPG 2.0+ (for signing)
go version              # Go 1.26+ (for development)

# All-in-one health check
gcm doctor
```

`gcm doctor` checks all dependencies and reports status:

```
✓ Git: found (v2.44.0)
✓ SSH: found (OpenSSH_9.7)
✓ GPG: found (gpg 2.4.5)
✓ Shell integration: installed (zsh)
✓ Config: valid
```

---

## Known Limitations

| Limitation | Workaround |
|-----------|------------|
| No package-manager formula/packages yet | Use the release installer, download a prebuilt binary, build from source, or `go install` |
| Windows cmd.exe not supported | Use PowerShell or WSL |
| Headless Linux may lack keychain | Encrypted file storage works without keychain |
| GPG agent can conflict | Restart `gpg-agent` or kill existing sessions |

---

## See Also

- [Installation](installation.md) — install GCM
- [Quick Start](quick-start.md) — 5-minute setup
- [Troubleshooting](troubleshooting.md) — solving common problems
- [Shell Integration](shell-integration.md) — auto-switch setup
