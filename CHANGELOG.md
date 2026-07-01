# Changelog

All notable changes to GCM will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [1.1.0] - 2026-07-01

### Added
- **`gcm ssh clean` / `gcm gpg clean`** — remove GCM-generated SSH/GPG keys that are no longer referenced by any profile; only keys GCM itself generated (tracked in a `~/.gcm/generated-keys.json` ledger) are eligible, so pre-existing and adopted keys are always left untouched; supports `--dry-run` (preview) and `--yes` (skip confirmation)
- **Self-update command** — `gcm update` checks GitHub Releases for a newer version, downloads the platform binary, verifies SHA-256 checksum, and replaces the running binary with safe backup/rollback; supports `--check` (dry run), `--force` (reinstall), and `--prerelease` flags

### Changed
- **Installer runs `gcm init` by default** — `install.sh`, `install.ps1`, and `install.bat` now set up shell integration (auto-switch on `cd` and the prompt profile indicator) automatically after install, so the active-profile prompt works without a manual step; pass `--no-init` (`-NoInit` on PowerShell) to skip any shell/git config changes. A failed `gcm init` is now a warning instead of aborting the install, since the binary itself is already in place. The previous opt-in `--init` flag is kept as a no-op for backward compatibility.

### Fixed
- **Installer PATH setup** — `install.sh` and `install.ps1` now automatically add the install directory to `PATH` when it isn't already present, fixing `command not found` on fresh installs where the binary landed in `~/.local/bin` but `PATH` was only updated when `--add-to-path` was explicitly passed
- **Test isolation leak** — `TestNonInteractiveCommandRunPaths` no longer overwrites the real repo's `.git/config` with test data (`Janet Doe` / `ABC123`) when a `.git/gcm-session` marker exists; the test now runs inside an isolated temp git repo

## [1.0.0] - 2026-05-01

### Added
- **Source-aware auth commands** — `gcm auth status|inspect|adopt|logout|doctor|repair` distinguish GCM-managed tokens from external Git credentials, support JSON reports, adoption previews, safe logout scopes, and helper repair
- **SSH stale-key recovery** — `gcm ssh generate/upload/test/copy` link an existing provider-aware local key back to a profile when `~/.ssh` files remain without GCM config; `gcm ssh generate --overwrite` explicitly replaces the local key pair
- **`gcm ssh upload` / `gcm gpg upload`** — standalone commands to upload SSH/GPG keys to the profile's configured provider with automatic duplicate detection; use `--force` to skip the check
- **Auto-upload duplicate detection** — `gcm ssh generate` and `gcm gpg generate` check if the key already exists on the profile's provider before offering to upload, preventing duplicates
- **Built-in credential helper** — GCM registers itself as git's credential helper for configured provider hosts (`gcm credential-helper`); git push/pull/clone reads tokens directly from GCM's encrypted store, bypassing the system keychain entirely
- **Git credential isolation** — `gcm use` isolates git credentials per profile; credentials are served dynamically from the encrypted store, preventing credential bleed between profiles
- **Credential username pinning** — sets provider-host credential usernames in global git config so git only uses credentials matching the active profile
- **Smart scope fallback** — `gcm use <name>` works anywhere: inside a git repo → session scope, outside → local scope (writes `.gcm-profile`)
- **`--global` clears local overrides** — `gcm use <name> --global` removes any `.gcm-profile` file and session marker in the current directory
- **`--hide-default` flag on `gcm current`** — outputs nothing when the active profile is the default; ideal for shell prompts that only show an indicator when you've explicitly switched
- **`--clear-credentials` flag on `gcm github logout`** — clears git credentials via `git credential reject` (default: true)
- **Login credential isolation** — `gcm github login*` commands only store git credentials if the profile being logged in is currently active; prevents credential bleed from non-active logins
- **Shell prompt integration** — uses `precmd`/`PROMPT_COMMAND` hook with a `$_GCM_PROMPT` variable approach (idempotent, no subshell on every keystroke, hides when default is active)
- **Profile management** — create, list, show, edit, delete, export, import, diff
- **Profile activation** — session, global, and local scopes with dry-run mode
- **Session marker file** (`.git/gcm-session`) for reliable session detection independent of git config
- **Session-aware profile detection** — `gcm current` checks: session marker → local marker → email matching → global default
- **SSH key generation** — Ed25519, RSA, ECDSA with auto-upload to the configured provider
- **SSH key operations** — listing, connection testing, and clipboard copy
- **GPG key generation** — commit signing management with auto-upload to the configured provider
- **GitHub OAuth device flow** authentication (`gcm github login-oauth`) with user-friendly error messages
- **GitHub PAT authentication** (`gcm github login`)
- **GitHub CLI token import** (`gcm github login-gh`)
- **Source-aware GitHub status** — authentication overview (`gcm github status`)
- **Encrypted token storage** — AES-256-GCM with Argon2id key derivation
- **Shell integration** — bash, zsh, fish, and powershell with auto-profile switching on directory change via `.gcm-profile`
- **Shell prompt indicator** for active profile
- **Configuration templates** — create, list, show, delete, import/export, apply
- **Backup and restore** — tar.gz archives with retention-based pruning
- **Profile validation** — basic and deep filesystem checks
- **System health check** (`gcm doctor`)
- **Cache cleaning** utility (`gcm clean`)
- **Audit logging** — JSONL format with daily rotation
- **Responsive table output** — auto-adapts to terminal width: truncate → hide columns → vertical cards
- **User-friendly error messages** — clear `✗ profile "x" not found` with actionable suggestions, usage hints on missing arguments, validation messages for file-not-found cases
- **Safe profile deletion** — deleting the active profile warns and requires extra confirmation
- **Cross-platform support** — macOS, Linux, Windows (amd64, arm64)
- **Comprehensive CLI** with colors, spinners, and interactive prompts
- **GoReleaser configuration** for automated releases
- **Makefile** with build, test, lint, and release targets
- **Unit tests** for core packages (crypto, file service, logger, profile, version)
