# Release Notes

Release history, policies, and upgrade paths for GCM.

---

## Version Format

GCM follows [Semantic Versioning](versioning.md). Version numbers are `MAJOR.MINOR.PATCH`.

---

## Latest Release

### v1.1.1

**Release Date:** August 01, 2026

#### Highlights

- **Data-safety release** — several commands could destroy key material, backups or shell configuration that GCM never created, and the profile store itself could be silently relocated into a directory the operating system deletes
- **`GCM_NO_KEYCHAIN=1`** — disables OS keychain access entirely; useful on headless, CI and container hosts. See [Configuration](configuration.md#gcm_no_keychain)
- **Hermetic test sandbox** — every test package redirects `HOME`, the git config scopes, the keychain, the ssh-agent and `GNUPGHOME` into a throwaway directory, so the suite can no longer rewrite a developer's real `~/.gcm`, `~/.gitconfig` or login keychain
- **`shellcheck` in CI** — gates `scripts/*.sh` at warning severity and blocks releases

#### Behavior Changes

- **`gcm status` is read-only again** — it ran the SSH key rename migration for every profile, so viewing the dashboard renamed files and rewrote profile YAML. It now reports the pending rename and points at `gcm repair --fix`
- **SSH key renaming now asks first** — `gcm use`, `gcm connect` and provider removal renamed the profile's key file as a side effect. They show both paths and ask now; with no terminal to answer nothing is renamed, and `gcm repair --fix` still does it directly
- **`gcm use` / `gcm connect` no longer run `git credential reject` against other providers** — git credentials are host-scoped, so the loop only deleted credentials GCM never created
- **Backup archives use forward-slash entry names**, so a backup created on Windows can be restored. Previously it could not be restored anywhere
- **`uninstall.sh` and `uninstall.ps1` read the generated-keys ledger** — neither guesses which SSH/GPG keys are GCM's, and both abort rather than guess when the ledger cannot be parsed. `uninstall.sh` also stops removing every `github.com` credential on the machine
- **Uninstall no longer removes a git identity GCM did not set** — it unset `user.name`, `user.email` and seven other keys unconditionally, destroying identities configured long before GCM existed. Only values matching one of your profiles are removed; the rest are listed and left alone
- **Uninstall asks before deleting `.gcm-profile` markers**, which you wrote, and the installer no longer deletes a file named `gcm` that it cannot identify — "GCM" is also Git Credential Manager
- **A credential helper you already had is restored on uninstall**, parked under `gcm.saved.<host>.helper` while GCM is registered

#### Bug Fixes

- **`gcm ssh clean` / `gcm gpg clean` could delete every generated key** — the "still in use" inventory came from the profile listing, which reports no matches *and no error* when the profiles directory is missing, so an absent store marked every ledger entry orphaned. A profile with a YAML typo caused the same for that profile's keys. Both commands now refuse unless the inventory is verifiably complete
- **`gcm profile delete` deleted keys you created yourself** — it removed the profile's SSH key pair and GPG *secret* key unconditionally, even when the profile merely pointed at a key you already had
- **`gcm backup create` could destroy your backup history** — an unreadable profiles directory was swallowed, an empty archive written, success reported, and retention pruning then removed the older backups that did contain profiles. Age-based pruning also had no floor and could remove every backup
- **`gcm backup restore` restored nothing** when `profiles_dir` was customised: it always wrote under `~/.gcm` regardless of configuration
- **`gcm clean` could delete your home directory** — it ran a recursive delete straight on `cache_dir` from the config file, unvalidated
- **Shell integration removal could truncate your rc file** — a missing end marker meant everything from the start marker to end of file was dropped, taking your own aliases and exports with it
- **`config.yaml` could relocate profiles into the OS temp directory**, where they are purged periodically — profiles vanished with no delete ever issued
- **Atomic writes are atomic on Windows** — the target was unlinked before renaming, leaving a window with no file at all. The rename is attempted first now, retried after clearing the read-only attribute only when that is what blocked it
- **PowerShell profile path** — shell integration always targeted `Documents\PowerShell` (PowerShell 7). Windows PowerShell 5.1 uses `Documents\WindowsPowerShell`, and `Documents` is often redirected into OneDrive
- **`uninstall.ps1` aborted part-way through** — it looped `foreach ($host in ...)`, and `$host` is a PowerShell automatic variable, so assigning to it throws — after the git identity step had already run

#### Known Issues

- **Windows is only partly covered by CI.** It now executes tests rather than only compiling them, which immediately caught four real defects, all fixed here. Seven packages run as a blocking gate; a second, non-blocking step runs the whole suite to surface what remains. Much of the rest asserts POSIX mode bits or uses `chmod` to block access, neither of which means anything on Windows, so failures there are expected until those tests are made portable
- **POSIX directory modes are not enforced on Windows** — `~/.gcm/tokens/`, `backups/` and `logs/` are not access-restricted there. Prefer the Credential Manager backend over file-based token storage. See the [Windows caveat](security.md#platform-caveat-windows)

---

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
