# Security Model

This document describes GCM's security architecture, threat model, encryption details, and permission policies.

---

## Security Principles

1. **Encrypt secrets at rest** — provider tokens are never stored as plain text by default
2. **Minimal permissions** — Sensitive files get `0600`, sensitive directories `0700`
3. **No secrets in argv** — Passphrases are never passed as command-line arguments to subprocesses
4. **Validate all input** — Profile names, GPG batch input, and archive paths are validated
5. **Audit everything** — Every state change is logged to an append-only JSONL file
6. **Defense in depth** — Multiple layers (encryption + permissions + keychain + audit)

---

## Threat Model

| Threat                                  | Mitigation                                                |
| --------------------------------------- | --------------------------------------------------------- |
| Token theft from disk                   | AES-256-GCM encryption or OS keychain                     |
| SSH private key exposure                | Written with `0600` permissions                           |
| SSH passphrase in process listing       | Never passed via argv; encrypted at rest with AES-256-GCM |
| GPG batch-mode injection                | Control characters and `%` rejected in input              |
| Path traversal on backup restore        | Zip-slip check rejects entries escaping target directory   |
| Profile name path traversal             | `/`, `\`, `..`, and control characters rejected           |
| Audit log date parameter injection      | `ReadLog()` validates date against `/`, `\`, `.`          |
| Unbounded API response consumption      | GitHub API responses capped at 5 MiB (`maxResponseSize`)  |
| HTTP timeout / hanging connections      | 60-second HTTP client timeout                             |
| Token file path traversal               | `sanitizeTokenPath()` rejects `..` and absolute paths     |
| Unauthorized config modification        | Sensitive dirs (`tokens/`, `logs/`, `backups/`) are `0700`|

---

## Token Storage

Provider tokens are stored using one of three backends, selected based on `security` configuration:

### 1. OS Keychain (default)

When `security.use_keychain: true`, tokens are stored in the platform's credential manager:

| Platform | Backend                              |
| -------- | ------------------------------------ |
| macOS    | Keychain (via Security framework)    |
| Linux    | secret-service / KWallet (via D-Bus) |
| Windows  | Credential Manager (via wincred)     |

**Library:** `github.com/zalando/go-keyring`

### 2. Encrypted File

When `security.encrypt_tokens: true` and `security.master_password: true`:

- Derives an AES-256 key from the master password using **Argon2id** (time=3, memory=64 MiB, threads=4)
- Generates a random 16-byte salt per token
- Encrypts the token with **AES-256-GCM** (authenticated encryption)
- On-disk format (v2): `[0x02 | 2-byte salt-length | salt | ciphertext]` in `~/.gcm/tokens/<profile>__<provider>__<host>__<account>.token` with `0600` permissions
- Legacy tokens (v1, PBKDF2-derived) are transparently decrypted on read but re-encrypted with Argon2id on next save
- Master password is prompted once per session and cached in memory

If keychain storage fails and encrypted file storage is configured with `security.master_password: true`, GCM falls back to encrypted file storage. If neither secure backend is available, token save/load fails closed.

### 3. Plain-Text File (explicit opt-in only)

When `security.allow_plaintext_tokens: true`:

- Token written to `~/.gcm/tokens/<profile>__<provider>__<host>__<account>.token` with `0600` permissions
- Relies solely on filesystem ACLs for protection
- Disabled by default; use only in constrained environments where no keychain or master-password encrypted storage is available

---

## Encryption Details

### Algorithm: AES-256-GCM

- **Key size:** 256 bits (32 bytes)
- **Nonce size:** 12 bytes (standard GCM)
- **Authentication:** Built-in GCM tag (no separate HMAC needed)
- **Library:** Go standard library `crypto/aes` + `crypto/cipher`

### Key Derivation: Argon2id (primary)

- **Time cost:** 3 iterations
- **Memory cost:** 64 MiB
- **Parallelism:** 4 threads
- **Salt:** 16 bytes, randomly generated per encryption
- **Output:** 32-byte AES key
- **Library:** `golang.org/x/crypto/argon2`

Argon2id is memory-hard and resistant to GPU/ASIC attacks (OWASP recommended).

### Key Derivation: PBKDF2 (legacy, read-only)

Existing tokens encrypted before the Argon2id migration use PBKDF2:

- **Iterations:** 100,000
- **Hash:** SHA-256
- **Salt:** 16 bytes
- **Output:** 32-byte AES key
- **Library:** `golang.org/x/crypto/pbkdf2`

Legacy tokens are transparently decrypted on read. New saves always use Argon2id.

### Random Number Generation

- All randomness comes from `crypto/rand.Reader` (CSPRNG)
- Test hook: `var randReader io.Reader = rand.Reader` allows failure injection in tests

---

## SSH Key Security

### Key Generation

- Keys are generated using Go's native `crypto` library:
  - Ed25519: `crypto/ed25519`
  - RSA: `crypto/rsa`
  - ECDSA: `crypto/ecdsa` with P-256
- No subprocess calls — no risk of passphrase leaking into argv

### File Permissions

| File            | Permissions | Description      |
| --------------- | ----------- | ---------------- |
| Private key     | `0600`      | Owner read/write |
| Public key      | `0644`      | World readable   |
| `~/.ssh/`       | `0700`      | Owner only       |

### Passphrase Storage

When a passphrase is provided during key generation:
1. The private key is encrypted at rest using OpenSSH native format (bcrypt-KDF + AES-256-CTR) via `ssh.MarshalPrivateKeyWithPassphrase`
2. The passphrase itself is **not** stored anywhere — neither in the profile config nor on disk
3. The passphrase is only held in memory during the key generation call
4. Passphrase never appears in process listings or command-line arguments (generation is done natively, not via ssh-keygen subprocess)

---

## Git Credential Isolation

When switching profiles with `gcm use`, GCM ensures git operations authenticate as the correct account:

### How It Works

1. **Reject old credentials** — calls `git credential reject` for the previous profile's provider credentials, clearing them from the OS credential store
2. **Approve new credentials** — calls `git credential approve` with the new profile's username and token, pre-seeding the credential store
3. **Pin username** — sets the provider-host `credential.*.username` value in global git config to the active profile's username

### Why Pinning Matters

Git's credential helper may have multiple stored credentials for the same provider host. Without pinning, git picks the first match — often the wrong profile. The `credential.*.username` configuration tells git to **only accept credentials matching that username**, preventing bleed.

### Login Isolation

When running `gcm github login*` commands:
- Credentials are only stored in git if the profile being logged in is the **currently active** one
- Logging into a non-active profile saves the encrypted token but does not touch git's credential store
- Credentials become active when you `gcm use` that profile

### Logout Behavior

`gcm github logout` (with `--clear-credentials`, default true):
- Only clears HTTPS Git credentials and the provider username pin if the logged-out profile is the currently active one (prevents breaking another profile's auth)
- Calls `git credential reject` to remove the profile's HTTPS credentials from the OS credential store
- Unsets `credential.https://<provider-host>.username` so future HTTPS prompts ask for a username instead of reusing the old profile account
- On macOS this removes the entry from Keychain Access
- On Windows this removes it from Credential Manager
- On Linux this removes it from secret-service/KWallet
- SSH remotes and profile SSH keys are not affected, so Git may still work over SSH after logout

`gcm auth logout` is ownership-aware:
- Default `--scope gcm` removes the GCM-managed provider token and, for the active profile, also clears the credential username pin and rejects any cached credential from Git's helper chain
- `--scope external` asks Git's credential chain to reject an external credential and requires confirmation unless `--yes` is supplied
- `--scope all` removes both GCM-owned and external credentials for the selected provider/profile
- `--dry-run` reports what would be removed without deleting anything

---

## Credential Helper Isolation

GCM includes a built-in git credential helper that bypasses the system keychain entirely, making git authentication immune to external credential changes (e.g., VS Code logout clearing macOS Keychain entries).

### How It Works

1. `gcm init` (or `gcm setup`) registers GCM as the credential helper for configured provider hosts:
   ```
   [credential "https://github.com"]
     helper = !'/path/to/gcm' credential-helper
   [credential "https://gitlab.com"]
     helper = !'/path/to/gcm' credential-helper
   ```
2. When git needs credentials, it calls `gcm credential-helper get`
3. GCM resolves the active profile, reads the provider-aware token from its own encrypted store, and returns it to git
4. `gcm credential-helper store` and `gcm credential-helper erase` handle the full credential helper protocol

### Why This Matters

| Scenario | Without GCM credential helper | With GCM credential helper |
| -------- | ----------------------------- | -------------------------- |
| VS Code logout | macOS Keychain entry deleted → `git push` fails | Token served from GCM's store → unaffected |
| External credential manager change | Git picks up stale/wrong credentials | GCM always serves the active profile's token |
| Multiple provider accounts | System keychain may return wrong token | GCM returns the token for the active profile |

### Encryption

Provider-aware token files in `~/.gcm/tokens/` are encrypted with **AES-256-GCM** (Argon2id key derivation). The credential helper decrypts in-memory and never writes plaintext tokens to disk or passes them via command-line arguments.

### Diagnostics

`gcm doctor` checks that the credential helper is properly registered. If it's missing, run `gcm init` to re-register it.

`gcm auth status` and `gcm auth inspect` add source-aware diagnostics. Credentials returned by `gcm credential-helper` are classified as GCM-owned because Git is reading from GCM's provider-aware token store, not from an external credential manager.

- `authenticated:gcm` — GCM owns and can verify the provider token
- `authenticated:external` — Git can authenticate through a credential GCM does not own
- `authenticated:mixed` — GCM and external credentials are both present
- `unauthenticated` — no HTTPS credential is available (SSH key may still exist; shown as `ssh_only` finding)
- `conflicted` — credentials resolve to different accounts or do not match profile metadata
- `revoked` / `expired` — GCM-managed token is present but no longer usable

`gcm auth adopt` verifies an exportable external credential before saving it into GCM's token store. `gcm auth logout --scope external` skips `gcm-store` credentials and only asks Git to reject credentials owned by another helper. `gcm auth repair` can safely re-register the credential helper; it does not adopt or delete external credentials.

---

## GPG Security

### Batch Input Validation

GPG key generation uses `gpg --batch --gen-key` with a parameter file. To prevent injection:

- **Control characters** (bytes 0x00–0x1F) are rejected in name and email
- **`%` character** is rejected (GPG uses `%` as an escape prefix in batch mode)
- Validation runs before the parameter file is written

### Key Storage

GCM only stores the **GPG key ID** in the profile YAML. The actual key material lives in the system's GPG keyring (`~/.gnupg/`), managed by GPG itself.

---

## File Permission Policy

### Directory Permissions

| Directory           | Mode   | Rationale                              |
| ------------------- | ------ | -------------------------------------- |
| `~/.gcm/`           | `0755` | Standard user directory                |
| `~/.gcm/tokens/`    | `0700` | Contains encrypted tokens              |
| `~/.gcm/backups/`   | `0700` | Contains configuration snapshots       |
| `~/.gcm/logs/`      | `0700` | Contains audit logs                    |
| `~/.gcm/profiles/`  | `0755` | Profile YAML (no secrets)              |
| `~/.gcm/templates/` | `0755` | Template YAML (no secrets)             |

### File Permissions

| File                   | Mode   | Rationale                           |
| ---------------------- | ------ | ----------------------------------- |
| `config.yaml`          | `0644` | User settings (no secrets)          |
| `profiles/*.yaml`      | `0644` | Profile data (no secrets)           |
| `tokens/*`             | `0600` | Encrypted tokens                    |
| `logs/*.jsonl`         | `0600` | Audit log entries                   |
| `backups/*.tar.gz`     | `0600` | Unencrypted configuration snapshots |
| SSH private keys       | `0600` | Standard SSH requirement            |
| SSH public keys        | `0644` | Standard SSH requirement            |

---

## Backup Security

### Path Traversal Protection (Zip-Slip)

During `gcm backup restore`, every tar entry's path is validated:

1. The entry name is cleaned and joined with the target directory
2. The result is checked to ensure it starts with the target directory prefix
3. Any entry that would escape the target directory is rejected

The archive is fully extracted into a staging directory before live files are replaced, so extraction failures do not partially overwrite existing profiles, templates, or config.

### Backup Contents

Backups include:
- `config.yaml`
- All profile YAML files
- All template YAML files

Backups **do not** include:
- SSH private keys (`backup.include_keys: true` fails closed until encrypted backup support exists)
- Provider tokens
- Audit logs

Backup archives are unencrypted `.tar.gz` files. `backup.encryption: true` fails closed rather than producing a misleading unencrypted archive.

---

## Audit Logging

### Log Format

Append-only JSONL files at `~/.gcm/logs/YYYY-MM-DD.jsonl`:

```json
{"timestamp":"2026-05-18T14:30:00Z","action":"profile.activate","profile":"work","details":{"scope":"session"},"success":true}
```

### Logged Actions

| Action               | Trigger                          |
| -------------------- | -------------------------------- |
| `profile.create`     | `gcm profile create`            |
| `profile.update`     | `gcm profile edit`              |
| `profile.delete`     | `gcm profile delete`            |
| `profile.activate`   | `gcm use`                       |
| `ssh.generate`       | `gcm ssh generate`              |
| `gpg.generate`       | `gcm gpg generate`              |
| `github.login`       | `gcm github login`              |
| `github.logout`      | `gcm github logout`             |
| `backup.create`      | `gcm backup create`             |
| `backup.restore`     | `gcm backup restore`            |
| `template.create`    | `gcm template import`           |
| `shell.init`         | `gcm init`                      |

### Security Properties

- **Append-only** — logs are opened with `O_APPEND | O_WRONLY`
- **Mutex-protected** — concurrent writes are serialized
- **Date-validated** — `ReadLog()` rejects dates with path separators

---

## Network Security

### GitHub API

- Provider API requests use configured HTTPS API URLs
- HTTP client has a 60-second timeout
- OAuth device flow uses the standard GitHub device authorization endpoint
- Response bodies are capped at 5 MiB to prevent memory exhaustion
- OAuth polling respects RFC 8628 `slow_down` responses (increases interval)

### No Phone Home

GCM does not:
- Send telemetry or analytics
- Check for updates automatically
- Contact any server other than configured provider API URLs

---

## Privacy

- All data stays on your machine in `~/.gcm/`
- No data is sent to third parties
- Provider tokens are only sent to their configured provider API when you explicitly authenticate, verify, upload keys, or run online status checks
- Audit logs are local-only and never transmitted

---

## Reporting Security Issues

If you discover a security vulnerability, please **do not** open a public issue. Instead, email the maintainer or use GitHub's private vulnerability reporting feature.

See [CONTRIBUTING.md](../CONTRIBUTING.md) for contact information.

---

## See Also

- [Configuration](configuration.md) — `security` section in config.yaml
- [Architecture](architecture.md) — design patterns and component diagram
- [Troubleshooting](troubleshooting.md) — permission and token issues
