# Changelog

All notable changes to GCM will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [1.1.1] - 2026-08-01

### Security
- **OS keychain kill switch** — set `GCM_NO_KEYCHAIN=1` to make every keychain read, write and delete fail closed instead of reaching the platform credential store. Useful on headless or containerised machines, and enforced inside the keyring accessors themselves so no call site can bypass it
- **Uninstall no longer clears unrelated credentials** — `uninstall.sh` looped `security delete-internet-password -s github.com` until it stopped succeeding, removing *every* `github.com` credential on the machine (browser logins, `git-credential-osxkeychain` entries, other tools' tokens). It now targets only the `git-config-manager` service that GCM actually writes to, with a bounded loop
- **Uninstall no longer guesses which keys are GCM's** — SSH keys were derived from profile names (`~/.ssh/id_{ed25519,rsa,ecdsa}_<profile>`) and GPG key IDs scraped from profile YAML, so keys you created yourself were deleted and GPG *secret* keys merely attached to a profile were destroyed irreversibly. Both now read `~/.gcm/generated-keys.json`, and a ledger that cannot be parsed aborts the removal rather than falling back to guesswork
- **Uninstall no longer removes a git identity GCM did not set** — `remove_git_identity` unset `user.name`, `user.email`, `user.signingkey`, `commit.gpgsign`, `gpg.format`, `gpg.program`, `core.sshCommand` and both `tag.*` keys from global git config — and four of them from whichever repository you happened to be standing in — without checking who set them. An identity configured years before GCM was installed was destroyed by uninstalling it. GCM writes the global identity from the active profile, so only values that match one of your profiles are removed; everything else is listed and left in place
- **Uninstall no longer deletes project markers without asking** — every `.gcm-profile` and `gcm-session` file found under nine home directories was deleted with no listing and no confirmation. A `.gcm-profile` is written by *you* to pin a profile to a project. It now shows what it found and asks first
- **The installer no longer deletes a binary it cannot identify** — a file named `gcm` that did not answer `gcm version` was removed as "corrupted". "GCM" is also the common abbreviation for Git Credential Manager, so that could delete an unrelated program. Nothing is deleted now: the install copies over its own target anyway
- **`gcm profile delete` respects the generated-keys ledger** — it called `os.Remove` on the profile's SSH key pair and `gpg --delete-secret-keys` on its GPG key unconditionally. A profile can point at a key you already had, so deleting the profile destroyed your own private key. Only key material GCM generated is removed; a ledger read error keeps the key

### Added
- **Pre-existing credential helpers are restored on uninstall** — registering GCM resets `credential.<host>.helper` so its helper wins, which discarded whatever you already had with no way back. The previous non-GCM helpers are now parked under `gcm.saved.<host>.helper` and put back by `gcm init`'s uninstall path. Re-running `gcm init` cannot overwrite that record
- **`GCM_NO_KEYCHAIN` environment variable** — see above; documented in [Configuration](docs/configuration.md#environment-variables)
- **Hermetic test sandbox** (`pkg/testutil`) — `Isolate(t)` and `RunIsolated(m)` redirect `HOME`, all three git config scopes, the OS keychain, the ssh-agent and `GNUPGHOME` into a throwaway directory. Every test package now installs it from `TestMain`, so tests cannot rewrite the developer's real `~/.gcm`, `~/.gitconfig` or login keychain
- **`shellcheck` CI job** — gates `scripts/*.sh` at warning severity and blocks the release job; all 27 pre-existing warnings (SC2155/SC2206/SC2183/SC2034) are fixed. CI also parses `uninstall.ps1` alongside `install.ps1`
- **Wider Windows CI** — `internal/shell` joins the blocking Windows test set, and a second non-blocking step runs the full suite to surface what still fails there

### Changed
- **`gcm status` is read-only again** — it ran the provider SSH key rename migration for every profile, so viewing the dashboard renamed key files on disk and rewrote profile YAML. It now reports the pending rename and points at `gcm repair --fix`, which already performs it
- **SSH key renaming now asks first** — `gcm use`, `gcm connect` and provider removal renamed the profile's key file to the provider-aware layout as a side effect, without saying so. They now show the current and target paths and ask; declining leaves the key alone. With no terminal to answer, nothing is renamed and the suggestion points at `gcm repair --fix`, which still renames directly because there it is the requested action
- **`gcm use` / `gcm connect` no longer reject other providers' credentials** — switching profiles ran `git credential reject` against every other configured provider host. Git credentials are scoped per host, so a GitLab entry never affects a GitHub operation; the loop only deleted credentials GCM never created. It now skips hosts where GCM is not the registered credential helper
- **Backup archives use forward-slash entry names** — entries were built with `filepath.Join`, producing `profiles\name.yaml` on Windows, which the extractor rejects as path traversal. Backups created on Windows could therefore never be restored, on any platform
- **`uninstall.sh` backs up shell rc files** before rewriting them, and reports the backup path

### Fixed
- **`gcm ssh clean` / `gcm gpg clean` could delete every generated key** — the "which keys are still in use" inventory came from `Manager.List`, which is built on `filepath.Glob` and reports no matches *and no error* when the profiles directory is missing. An absent store therefore marked every ledger entry orphaned. Both commands now refuse unless the inventory is verifiably complete
- **…and could delete the keys of a profile with a YAML typo** — profiles that fail to parse are skipped by `List`, so their keys looked referenced by nobody. Cleaning now refuses and names the unreadable files
- **`gcm backup create` could destroy the backup history** — an unreadable profiles directory was swallowed, an empty archive written, success reported, and retention pruning then removed the older backups that *did* contain profiles. It now refuses; an existing but empty directory still succeeds
- **`PruneOlderThan` could delete every backup** — a retention cutoff newer than all backups removed all of them. The newest is now always kept, matching the `keep >= 1` floor `Prune` already enforced
- **`gcm backup restore` restored nothing when `profiles_dir` was customised** — `create` reads `profiles_dir`/`templates_dir` but stores fixed `profiles/` and `templates/` prefixes, while `restore` rebuilt every path under `~/.gcm`. Restore now maps the prefixes back onto the configured directories
- **`gcm clean` could delete your home directory** — it ran `os.RemoveAll` directly on `cache_dir` from the config file, with no validation. Empty, relative, filesystem-root, home-directory and any ancestor of home or `~/.gcm` are now rejected
- **Shell integration removal could truncate your rc file** — the GCM block was stripped by dropping lines until the end marker, so a missing end marker deleted everything from the start marker to end of file, taking your own aliases and exports with it. It now refuses and leaves the file untouched. The rewrite is also atomic and preserves the file's existing permissions instead of forcing `0644`
- **`default_profile` could name a deleted profile** — `gcm profile delete` cleared it in memory only, so `config.yaml` kept pointing at a profile that no longer existed
- **The generated-keys ledger now follows SSH key renames** — after GCM renamed a key the ledger still pointed at the old path, so the key lost its provenance and the old path stayed marked as GCM's, making anything later placed there eligible for deletion
- **`config.Save` refuses temp-directory data directories** — `validateConfigPaths` claimed to block test data from reaching the production config but only ever checked `git_command`, ignoring `profiles_dir`, `templates_dir` and `cache_dir`. A test's config could relocate profiles into a directory the OS purges, so they vanished with no delete ever issued
- **Atomic writes are atomic on Windows** — `WriteAtomic` unlinked the target before renaming, which opened a window where an interruption left no file at all. The rename is now attempted first, and only if it fails on an existing regular file is the read-only attribute cleared and the rename retried. That middle ground matters: `os.Rename` uses `MoveFileEx` with `MOVEFILE_REPLACE_EXISTING`, which does replace an existing target — but still fails with "Access is denied" when that target carries `FILE_ATTRIBUTE_READONLY`. Every other write keeps the atomic path instead of being degraded for that one case
- **Tests resolved the wrong home directory on Windows** — they set it with `t.Setenv("HOME", ...)`, but `os.UserHomeDir` reads `%USERPROFILE%` there, so the call did nothing and every test fell back to the shared sandbox home. State from one test leaked into the next: two `IsInstalled` tests read a `.zshrc` an earlier test had installed hooks into, and a third looked for a `.bashrc` written somewhere else. `testutil.SetHome` sets both variables, and all 193 call sites use it. The failures were invisible on macOS and Linux and only surfaced once the suite ran on Windows CI
- **PowerShell profile path** — shell integration always targeted `Documents\PowerShell` (PowerShell 7). Windows PowerShell 5.1, the edition that ships with Windows, uses `Documents\WindowsPowerShell`, and `Documents` is often redirected into OneDrive. GCM now prefers whichever profile directory exists
- **`uninstall.ps1` aborted part-way through** — it looped `foreach ($host in ...)`, and `$host` is a PowerShell automatic variable holding the host UI object, so assigning to it throws. The failure landed after the git identity step had already run. The loop variable is now `$providerHost`
- **`uninstall.ps1` guessed which keys were GCM's** — it derived SSH paths from profile names and scraped GPG key IDs from profile YAML, exactly as `uninstall.sh` used to. It now reads `generated-keys.json` and aborts rather than guessing when the ledger cannot be parsed
- **`uninstall.sh` could truncate your rc file** — `sed '/# >>> GCM/,/# <<< GCM/d'` uses a range whose end address, when it never matches, extends to end of file. Both markers are now required

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
