# Glossary

Definitions for terms used throughout GCM documentation and source code.

---

## General Terms

| Term | Definition |
| ---- | ---------- |
| **GCM** | Git Config Manager — the CLI tool described in this documentation |
| **Profile** | A named Git identity containing user info, SSH, GPG, and provider account configuration |
| **Identity** | The combination of `user.name`, `user.email`, SSH key, GPG key, and provider account that represents "who you are" to Git |
| **Active profile** | The profile currently applied to your Git configuration in the shell session |
| **Default profile** | The profile stored in `config.yaml` that activates on shell start when no local profile is pinned |

---

## Profile Terms

| Term | Definition |
| ---- | ---------- |
| **Profile name** | A free-form identifier (e.g., `work`, `personal`, `client-acme`) used as the profile's filename and reference in commands |
| **Profile YAML** | The YAML file at `~/.gcm/profiles/<name>.yaml` containing all profile data |
| **Activation scope** | Where a profile is applied: **session** (current shell), **global** (default), or **local** (pinned to directory) |
| **Session scope** | Profile activated for the current shell only; lost on shell restart |
| **Global scope** | Profile saved as `default_profile` in `config.yaml`; persists across shell restarts |
| **Local scope** | Profile pinned to a directory via a `.gcm-profile` file; auto-activated on `cd` |
| **`.gcm-profile`** | A plain-text file containing a profile name, placed in a project directory for auto-switching |
| **Usage count** | The number of times a profile has been activated, tracked in metadata |

---

## SSH Terms

| Term | Definition |
| ---- | ---------- |
| **Ed25519** | A modern elliptic-curve key type; GCM's default. Fast, small keys, high security |
| **RSA** | An older key type still widely supported. GCM supports 2048, 3072, and 4096-bit keys |
| **ECDSA** | Elliptic Curve Digital Signature Algorithm using the P-256 curve |
| **SSH agent** | A background process (`ssh-agent`) that holds decrypted private keys in memory |
| **Fingerprint** | A hash of the public key (e.g., `SHA256:abc123...`) used to identify keys |
| **Passphrase** | An optional password protecting an SSH private key; GCM encrypts it at rest |

---

## GPG Terms

| Term | Definition |
| ---- | ---------- |
| **GPG** | GNU Privacy Guard — a tool for encryption and signing |
| **Commit signing** | Using GPG to cryptographically sign Git commits, proving authorship |
| **Key ID** | The short identifier for a GPG key (e.g., `0xABCD1234`) |
| **Signing key** | The GPG key ID stored in `user.signingkey` Git config |
| **`commit.gpgsign`** | Git config option that auto-signs all commits when `true` |
| **Batch mode** | GPG's non-interactive key generation mode, used by `gcm gpg generate` |

---

## GitHub Terms

| Term | Definition |
| ---- | ---------- |
| **Device flow** | OAuth authorization method where you enter a code in your browser instead of redirecting. Used by `gcm github login-oauth` |
| **User code** | The short code (e.g., `ABCD-1234`) you enter at github.com/login/device |
| **Access token** | The OAuth token received after authorization; stored encrypted |
| **Token store** | GCM's token storage system with three backends: keychain, encrypted file, plain file |
| **Credential helper** | A git-compatible credential helper (`gcm credential-helper`) that serves tokens from GCM's encrypted store, bypassing the system keychain |
| **Auth ownership** | Whether the effective provider credential is owned by GCM, external to GCM, mixed, or unknown |
| **External credential** | A credential returned by Git's helper chain that is owned by another tool such as Keychain, Git Credential Manager, GitHub CLI, libsecret, cache, or store |

---

## Security Terms

| Term | Definition |
| ---- | ---------- |
| **AES-256-GCM** | Authenticated encryption algorithm used for token and passphrase storage |
| **PBKDF2** | Password-Based Key Derivation Function; derives encryption keys from passwords |
| **Salt** | Random bytes mixed with a password before key derivation to prevent rainbow table attacks |
| **Nonce** | A one-time random value used in AES-GCM encryption |
| **Keychain** | The OS credential store (macOS Keychain, Linux secret-service, Windows Credential Manager) |
| **Zip-slip** | A path traversal attack in archive extraction; GCM validates all paths during restore |
| **Audit log** | Append-only JSONL file recording all GCM operations (profile switches, key generation, etc.) |

---

## Configuration Terms

| Term | Definition |
| ---- | ---------- |
| **`config.yaml`** | GCM's global configuration file at `~/.gcm/config.yaml` |
| **`~/.gcm/`** | GCM's data directory containing all profiles, templates, tokens, backups, and logs |
| **Template** | A reusable YAML blueprint for creating profiles with preset configuration |
| **Auto-switch** | The feature that automatically activates profiles when you `cd` into a directory with `.gcm-profile` |
| **Shell hook** | Code injected into your shell config that triggers auto-switching on directory change |
| **Prompt indicator** | The `(profile-name)` shown in your terminal prompt when a profile is active |

---

## Architecture Terms

| Term | Definition |
| ---- | ---------- |
| **Container** | The dependency injection struct that holds all service instances |
| **Domain service** | A package implementing core business logic (e.g., `profile.Manager`, `ssh.Manager`) |
| **Infrastructure service** | A package providing low-level capabilities (e.g., `service/crypto`, `service/file`) |
| **Function-variable hook** | A package-level `var fn = realImplementation` that tests override for mocking |
| **Cobra** | The CLI framework used by GCM (`github.com/spf13/cobra`) |
| **JSONL** | JSON Lines format — one JSON object per line, used for audit logs |

---

## File Permissions

| Permission | Octal | Meaning                              |
| ---------- | ----- | ------------------------------------ |
| `rwx------`| `0700`| Owner can read, write, execute       |
| `rw-------`| `0600`| Owner can read, write                |
| `rw-r--r--`| `0644`| Owner read/write, others read        |
| `rwxr-xr-x`| `0755`| Owner full, others read/execute      |

---

## Common Abbreviations

| Abbreviation | Full Form |
| ------------ | --------- |
| GCM | Git Config Manager |
| CLI | Command-Line Interface |
| SSH | Secure Shell |
| GPG | GNU Privacy Guard |
| DI | Dependency Injection |
| CRUD | Create, Read, Update, Delete |
| YAML | YAML Ain't Markup Language |
| JSONL | JSON Lines |
| AEAD | Authenticated Encryption with Associated Data |
| CSPRNG | Cryptographically Secure Pseudo-Random Number Generator |
| TLS | Transport Layer Security |
| OAuth | Open Authorization |
| API | Application Programming Interface |

---

## See Also

- [Architecture](architecture.md) — design patterns and component diagram
- [Configuration](configuration.md) — config.yaml reference
- [Security Model](security.md) — encryption and permission details
