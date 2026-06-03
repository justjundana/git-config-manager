# Project Structure

A complete map of every directory and file in the GCM codebase, with purpose and responsibilities.

---

## Top-Level Layout

```
git-config-manager/
├── cmd/gcm/main.go           # Entry point
├── internal/                  # Private application code
│   ├── audit/                 # Audit logging
│   ├── auth/                  # Source-aware auth ownership resolver
│   ├── backup/                # Backup & restore
│   ├── cli/                   # Cobra CLI commands
│   ├── config/                # Configuration management
│   ├── container/             # Dependency injection
│   ├── github/                # GitHub API client
│   ├── gitlab/                # GitLab API client
│   ├── gpg/                   # GPG key management
│   ├── profile/               # Profile CRUD + switching
│   ├── provider/              # Provider definitions, capabilities, registry
│   ├── providerclient/        # Provider-neutral API operation router
│   ├── service/
│   │   ├── crypto/            # AES-256-GCM encryption
│   │   └── file/              # File operations
│   ├── shell/                 # Shell integration
│   ├── ssh/                   # SSH key management
│   ├── tokenstore/            # Provider-aware token persistence
│   └── template/              # Configuration templates
├── pkg/                       # Public shared libraries
│   ├── logger/                # Structured logging
│   ├── ui/                    # Terminal UI (prompts, tables)
│   └── version/               # Build version info
├── docs/                      # Documentation
├── go.mod                     # Go module definition
├── go.sum                     # Dependency checksums
├── Makefile                   # Build, test, lint targets
├── README.md                  # Project README
├── CONTRIBUTING.md            # Contribution guidelines
├── CHANGELOG.md               # Release history
└── LICENSE                    # MIT license
```

---

## `cmd/gcm/`

| File      | Purpose                                                       |
| --------- | ------------------------------------------------------------- |
| `main.go` | Entry point. Loads config → creates logger → builds container → runs CLI |

The entire `main()` is ~30 lines. All logic lives in `internal/`.

---

## `internal/cli/`

The Cobra command layer. Each file owns one top-level command group.

| File             | Commands                                              |
| ---------------- | ----------------------------------------------------- |
| `root.go`        | Root `gcm` command; registers all subcommands         |
| `profile.go`     | `profile create\|list\|show\|edit\|delete\|export\|import\|diff` |
| `use.go`         | `use`, `current`, `refresh`                           |
| `ssh.go`         | `ssh generate\|list\|test\|copy\|upload`              |
| `gpg.go`         | `gpg generate\|list\|sign enable\|sign disable\|test\|upload` |
| `auth.go`        | `auth status\|inspect\|adopt\|logout\|doctor\|repair` |
| `github.go`      | `github login\|login-oauth\|login-gh\|status\|logout\|verify\|user` |
| `gitlab.go`      | `gitlab login\|status\|logout\|verify\|user`          |
| `template.go`    | `template create\|list\|show\|delete\|export\|import\|apply` |
| `backup.go`      | `backup create\|list\|restore\|prune`                 |
| `doctor.go`      | `validate`, `doctor`                                  |
| `credential_helper.go` | `credential-helper get\|store\|erase` (hidden, called by git) |
| `init_cmd.go`    | `init`                                                |
| `clean.go`       | `clean`                                               |
| `update.go`      | `update`                                              |
| `version_cmd.go` | `version`                                             |
| `helpers.go`     | `formatTimeAgo()`, `formatBytes()`                    |
| `cli_test.go`    | Tests for CLI commands                                |

**Pattern:** Each command file defines `newXxxCmd() *cobra.Command` which is called by `root.go`.

---

## `internal/auth/`

Source-aware authentication state and credential ownership resolution.

| File                  | Purpose                                                   |
| --------------------- | --------------------------------------------------------- |
| `types.go`            | Auth states, ownership/source enums, JSON report structs  |
| `resolver.go`         | Aggregates GCM token, external Git credential, SSH, capabilities, findings |
| `git_credentials.go`  | Bounded Git credential helper inspection and rejection    |
| `*_test.go`           | 100% statement coverage for auth resolver/inspector paths |

This package is provider-neutral and depends on provider definitions, provider-aware token storage, and providerclient verification rather than GitHub/GitLab-specific CLI branches.

---

## `internal/config/`

Configuration loading, saving, and default values.

| File                  | Purpose                                              |
| --------------------- | ---------------------------------------------------- |
| `types.go`            | All config struct types (`Config`, `AutoSwitchConfig`, etc.), `GCMDir()`, `DefaultConfig()` |
| `loader.go`           | `Load()`, `Save()`, `EnsureDirs()`, YAML read/write |
| `loader_test.go`      | Tests for loader                                     |
| `loader_save_test.go` | Tests for save functionality                         |

**Key types:**
- `Config` — root config with all subsections
- `AutoSwitchConfig`, `ShellConfig`, `GitHubAppConfig`, `BackupConfig`, `SecurityConfig`, `UIConfig`, `AdvancedConfig`

**Test hooks:** `userHomeDirFn`, `exitFn` — allow testing `os.UserHomeDir` and `os.Exit` paths.

---

## `internal/container/`

Dependency injection wiring.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `container.go`    | `Container` struct + `New()` constructor              |
| `container_test.go` | Tests for container creation                        |

`New()` creates all services and wires their dependencies. The `Container` is passed to the CLI layer.

---

## `internal/profile/`

Profile domain logic — the core of GCM.

| File              | Purpose                                                |
| ----------------- | ------------------------------------------------------ |
| `types.go`        | `Profile`, `GitConfig`, `SSHConfig`, `GPGConfig`, `GitHubConfig`, `Metadata`, `ActivationScope` |
| `manager.go`      | `Manager` — CRUD: `Create`, `Get`, `Update`, `Delete`, `List`, `Export`, `Import`, `Exists`, `IncrementUsage` |
| `switcher.go`     | `Switcher` — `Activate` (session/global/local), `Current`, `Refresh` |
| `validator.go`    | `ValidateProfile`, `ValidateDeep`, `ValidationResult` |
| `errors.go`       | `ProfileError` type with codes and factory functions   |
| `types_test.go`   | Tests for types                                        |
| `manager_test.go` | Tests for manager                                      |
| `switcher_test.go`| Tests for switcher                                     |
| `validator_test.go`| Tests for validator                                   |

**Key types:**
- `Profile` — complete Git identity (Git, SSH, GPG, GitHub, Metadata)
- `ActivationScope` — `ScopeSession`, `ScopeGlobal`, `ScopeLocal`
- `ProfileError` — structured error with code, message, suggestion

---

## `internal/ssh/`

SSH key generation and management.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `manager.go`      | `Manager` — `Generate`, `List`, `TestConnection`, `GetPublicKey`, `GetFingerprint`, `AddToAgent` |
| `manager_test.go` | Tests (including fake SSH scripts for subprocess mocking) |

**Supported key types:** Ed25519, RSA (2048/3072/4096), ECDSA (P-256).

Keys are generated using Go's native `crypto` library — no subprocess calls for generation.

---

## `internal/gpg/`

GPG key management.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `manager.go`      | `Manager` — `Generate`, `List`, `TestSigning`, `IsInstalled` |
| `manager_test.go` | Tests                                                 |

GPG operations shell out to `gpg` via `exec.Command`. Input is validated to prevent injection into the batch-mode parameter file.

---

## `internal/github/`

GitHub API integration and token storage.

| File                 | Purpose                                            |
| -------------------- | -------------------------------------------------- |
| `client.go`          | `Client` — `InitiateDeviceFlow`, `PollForToken`, `GetUser`, `VerifyToken`, `UploadSSHKey` |
| `token_store.go`     | `TokenStore` — `SaveToken`, `LoadToken`, `DeleteToken` with three backends (keychain, encrypted file, plain file) |
| `client_test.go`     | Tests with `httptest.Server`                       |
| `token_store_test.go`| Tests with mock keyring                            |

**Token storage backends (selected at runtime):**
1. OS Keychain (macOS Keychain, Linux secret-service, Windows Credential Manager)
2. Encrypted file (AES-256-GCM + Argon2id; legacy PBKDF2 tokens read transparently)
3. Plain-text file (`0600`, explicit opt-in only)

**Test hooks:** `keyringSet`, `keyringGet`, `keyringDelete` — in-memory keyring for tests.

---

## `internal/shell/`

Shell integration (hooks, prompt, detection).

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `manager.go`      | `Manager` — `DetectShell`, `Install`, `Uninstall`, `GenerateInitScript`, `GenerateCompletionScript`, hook generation per shell |
| `manager_test.go` | Tests                                                 |

**Supported shells:** Bash, Zsh, Fish, PowerShell.

Hook code is generated as string literals — no template files.

---

## `internal/template/`

Configuration template management.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `manager.go`      | `Manager` — `Create`, `List`, `Delete`, `Export`, `Import` |
| `manager_test.go` | Tests                                                 |

Templates are YAML files in `~/.gcm/templates/` with metadata (author, version, created).

---

## `internal/backup/`

Backup and restore operations.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `manager.go`      | `Manager` — `Create`, `List`, `Restore`, `Prune`      |
| `manager_test.go` | Tests                                                 |

Backups are unencrypted `.tar.gz` archives containing config, profile YAML, and template YAML. Restore stages extraction first and is guarded against path-traversal (zip-slip). Encrypted/key-inclusive backup settings fail closed until those archive formats are implemented.

---

## `internal/audit/`

Audit logging.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `logger.go`       | `Logger` — `Log()` writes JSONL entries; `ReadLog()` reads by date |
| `logger_test.go`  | Tests                                                 |

Each entry: timestamp, action, profile, details, success/error. File: `~/.gcm/logs/YYYY-MM-DD.jsonl`.

---

## `internal/service/crypto/`

Cryptographic primitives.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `service.go`      | `Service` — `Encrypt`, `Decrypt`, `GenerateKey`, `GenerateSalt` |
| `service_test.go` | Tests (including `randReader` hook for failure injection) |

Uses AES-256-GCM for authenticated encryption, Argon2id (time=3, memory=64 MiB, threads=4) for key derivation, and PBKDF2 (100,000 iterations, SHA-256) for legacy backward-compatible decryption.

---

## `internal/service/file/`

File system operations.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `service.go`      | `Service` — `Read`, `Write`, `WriteAtomic`, `Exists`, `EnsureDir`, `EnsurePermissions`, `ExpandPath`, `CopyFile` |
| `service_test.go` | Tests                                                 |

`WriteAtomic` uses temp-file + rename for crash-safe writes. `ExpandPath` resolves `~` to the home directory.

---

## `pkg/ui/`

Terminal UI components (shared, importable by external packages).

| File                 | Purpose                                           |
| -------------------- | ------------------------------------------------- |
| `ui.go`              | Colors, icons, printing helpers (`Success`, `Error`, `Warning`, `Header`, `Detail`, `NextSteps`) |
| `prompt.go`          | `AskString`, `AskConfirm`, `AskPassword`, `AskSelect`, `AskMultiSelect` |
| `interactive.go`     | `interactiveSelect` — raw terminal arrow-key UI   |
| `spinner.go`         | `Spinner` — animated progress indicator           |
| `table.go`           | `SimpleTable`, `PrintTable` with ANSI-aware width |
| `*_test.go`          | Tests for all of the above                        |

**Test hooks:** `isTerminalFn`, `readPasswordFn`, `interactiveSelectFn`, `makeRawFn`, `restoreFn`, `stdinReader`.

---

## `pkg/logger/`

Structured logging.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `logger.go`       | `Logger` — `Info`, `Warn`, `Error`, `Debug` with fields |
| `logger_test.go`  | Tests                                                 |

Fields use `logger.F("key", "value")` syntax.

---

## `pkg/version/`

Build version information.

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `version.go`      | `Get()` returns `Info{Version, Commit, Date, OS, Arch}` |
| `version_test.go` | Tests                                                 |

Variables are populated at build time via `-ldflags`:
```
-X github.com/justjundana/git-config-manager/pkg/version.Version=$(VERSION)
-X github.com/justjundana/git-config-manager/pkg/version.Commit=$(COMMIT)
-X github.com/justjundana/git-config-manager/pkg/version.Date=$(DATE)
```

---

## Build Files

| File              | Purpose                                               |
| ----------------- | ----------------------------------------------------- |
| `Makefile`        | `build`, `build-all`, `test`, `lint`, `fmt`, `install`, `install-system`, `release` |
| `go.mod`          | Module path: `github.com/justjundana/git-config-manager`, Go 1.26+ |
| `go.sum`          | Dependency checksums                                  |

---

## Code Organization Principles

1. **`internal/` vs `pkg/`** — `internal/` is private to this module; `pkg/` could be imported by external code
2. **One domain per package** — `internal/ssh/` only does SSH, `internal/gpg/` only does GPG
3. **No import cycles** — Dependency flows top-down; `pkg/` has no `internal/` imports
4. **Tests alongside code** — `*_test.go` files live next to the code they test
5. **No generated code** — All CLI commands are hand-written Cobra definitions

---

## See Also

- [Architecture Overview](architecture.md) — design patterns and component diagram
- [Data Flow & Diagrams](data-flow.md) — operation traces
- [Dependencies](dependencies.md) — why each module is used
