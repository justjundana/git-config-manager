# Release Notes

Release history, policies, and upgrade paths for GCM.

---

## Version Format

GCM follows [Semantic Versioning](versioning.md). Version numbers are `MAJOR.MINOR.PATCH`.

---

## Latest Release

### v1.1.0

**Release Date:** July 01, 2026

#### Highlights

- **Self-update command** — `gcm update` checks GitHub Releases for a newer version, downloads the platform binary, verifies SHA-256 checksum, and replaces the running binary with safe backup/rollback; supports `--check` (dry run), `--force` (reinstall), and `--prerelease` flags
- **Key cleanup commands** — `gcm ssh clean` and `gcm gpg clean` remove GCM-generated keys that are no longer referenced by any profile; only keys GCM itself generated (tracked in a `~/.gcm/generated-keys.json` ledger) are eligible, so pre-existing and adopted keys are always left untouched

#### Behavior Changes

- **Installer runs `gcm init` by default** — `install.sh`, `install.ps1`, and `install.bat` now install shell integration (auto-switch on `cd` and the prompt profile indicator) automatically, so the `(profile)` prompt indicator works right after install with no manual step. Pass `--no-init` (`-NoInit` on PowerShell) to skip shell/git config changes. The previous opt-in `--init` flag is kept as a no-op for backward compatibility.

#### Bug Fixes

- **Installer PATH setup** — `install.sh` and `install.ps1` now automatically add the install directory to `PATH` when it isn't already present, fixing `command not found` on fresh installs
- **Test isolation leak** — `TestNonInteractiveCommandRunPaths` no longer overwrites the real repo's `.git/config` with test data when a `.git/gcm-session` marker exists; the test now runs inside an isolated temp git repo

#### New Commands

| Command | Description |
|---------|-------------|
| `gcm update` | Self-update to latest GitHub Release |
| `gcm update --check` | Check for updates without installing |
| `gcm update --force` | Reinstall current version |
| `gcm update --prerelease` | Include pre-release versions |
| `gcm ssh clean` | Remove unused GCM-generated SSH keys |
| `gcm gpg clean` | Remove unused GCM-generated GPG keys |

#### Upgrade

```bash
gcm update
```

Or download from [GitHub Releases](https://github.com/justjundana/git-config-manager/releases/tag/v1.1.0).

---

### v1.0.0

**Release Date:** June 01, 2026

The first public release of GCM.

#### Highlights

- **Complete Git identity management** — switch name, email, editor, SSH key, GPG key, and provider token with one command
- **Git credential isolation** — `gcm use` pins provider-host credential usernames and manages `git credential approve/reject` so credentials never bleed between profiles
- **Smart scope fallback** — `gcm use` works anywhere: session scope in git repos, local scope (`.gcm-profile`) elsewhere
- **Three activation scopes** — session (shell only), global (default, clears local overrides), and local (pinned to directory)
- **SSH key generation** — Ed25519, RSA (2048-4096), ECDSA (P-256) with native Go crypto; auto-upload to the configured provider if authenticated
- **SSH stale-key recovery** — leftover provider-aware local keys are linked back to profiles when `~/.ssh` files remain without GCM config; replacement requires `gcm ssh generate --overwrite`
- **GPG signing** — generate keys, enable/disable per profile; auto-upload to the configured provider if authenticated
- **GitHub OAuth device flow** — secure browser-based authentication with user-friendly error messages
- **Login credential isolation** — logging into a non-active profile stores the token but does not affect git operations until you switch
- **Source-aware auth ownership** — `gcm auth status|inspect|adopt|logout|doctor|repair` distinguishes GCM-owned tokens from external Git credentials and makes adoption/deletion explicit
- **Encrypted token storage** — AES-256-GCM with Argon2id key derivation
- **Built-in credential helper** — bypasses system keychain (osxkeychain/wincred), serves tokens directly from GCM's encrypted store
- **Shell integration** — auto-switch on `cd` for bash, zsh, fish, powershell; `precmd` prompt indicator with `--hide-default` support
- **Templates** — reusable profile blueprints for team standardization
- **Backup & restore** — tar.gz archives with retention-based pruning
- **Audit logging** — JSONL format, daily rotation
- **Diagnostics** — `gcm doctor` checks all dependencies and configuration
- **Cross-platform** — macOS, Linux, Windows (amd64, arm64)

#### Commands

| Command | Description |
|---------|-------------|
| `gcm profile create/list/show/edit/delete` | Full profile CRUD |
| `gcm profile export/import` | Profile sharing |
| `gcm profile diff` | Compare two profiles |
| `gcm validate [profile]` | Deep filesystem validation |
| `gcm use <profile>` | Activate profile with credential isolation |
| `gcm use <profile> --global` | Set default (clears local overrides) |
| `gcm current` | Show active profile |
| `gcm current --short --hide-default` | For shell prompts (silent when default) |
| `gcm ssh generate/list/test/copy/upload` | SSH key management, stale-key recovery, provider upload |
| `gcm gpg generate/list/sign enable/sign disable/test/upload` | GPG signing and provider upload |
| `gcm github login/login-oauth/login-gh` | GitHub auth (credential-isolated) |
| `gcm github status/logout/verify/user` | GitHub source-aware status & management |
| `gcm github logout --clear-credentials` | Remove token + git credentials |
| `gcm auth status/inspect/adopt/logout/doctor/repair` | Source-aware auth ownership workflows |
| `gcm template create/list/show/apply/delete/export/import` | Template management |
| `gcm backup create/list/restore/prune` | Backup management |
| `gcm init` | Install shell integration + credential helper |
| `gcm doctor` | System health check |
| `gcm clean` | Clear cache |
| `gcm version` | Show version info |

#### Requirements

- Go 1.26+ (build from source)
- Git 2.20+
- OpenSSH 7.0+ (for SSH features)
- GPG 2.0+ (optional, for signing)

#### Known Issues

- No package-manager formula/packages yet (Homebrew, apt, etc.)
- `--shell` flag on `gcm init` is not yet implemented (auto-detection only)

---

## Development Versions

Development builds report version as `dev`:

```bash
$ gcm version --short
gcm dev
```

These are built from the `main` branch and may include unreleased features.

---

## Release Process

1. **Feature freeze** — no new features, only bug fixes
2. **Update CHANGELOG.md** — document all changes
3. **Update version** — tag with `vMAJOR.MINOR.PATCH`
4. **Build** — `make release` (cross-compile for all platforms)
5. **Test** — run full test suite on all platforms
6. **Publish** — create GitHub release with binaries and changelog
7. **Announce** — update documentation

---

## Upgrade Path

| From | To | Migration |
|------|-----|-----------|
| dev | v1.0.0 | No migration needed, same format |
| v1.x | v1.y (y > x) | Automatic, backwards compatible |
| v1.x | v2.0 | Follow migration guide in v2.0 release notes |

---

## Security Releases

Security vulnerabilities are treated with high priority:

| Severity | Response Time | Release Type |
|----------|-------------|-------------|
| Critical | 24-48 hours | Patch release |
| High | 1 week | Patch release |
| Medium | Next minor | Minor release |
| Low | Next minor | Minor release |

To report a security issue, see [CONTRIBUTING.md](../CONTRIBUTING.md).

---

## Deprecation Timeline

Features deprecated in one version are removed no earlier than the next major version. See [Versioning](versioning.md#deprecation-process) for the full policy.

---

## Changelog

For a detailed list of all changes, see [CHANGELOG.md](../CHANGELOG.md).

---

## See Also

- [Versioning](versioning.md) — versioning policy and compatibility
- [Upgrade & Uninstall](upgrade-uninstall.md) — upgrade instructions
- [Installation](installation.md) — install methods
