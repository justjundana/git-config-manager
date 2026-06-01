# Data Flow & Diagrams

This document traces how key GCM operations work end-to-end, with visual diagrams.

---

## System Overview

```mermaid
graph TB
    User[User] --> CLI[CLI Layer]
    CLI --> Container[Container DI]
    
    Container --> ProfileMgr[Profile Manager]
    Container --> Switcher[Profile Switcher]
    Container --> SSHMgr[SSH Manager]
    Container --> GPGMgr[GPG Manager]
    Container --> GHClient[GitHub Client]
    Container --> ShellMgr[Shell Manager]
    Container --> BackupMgr[Backup Manager]
    Container --> AuditLog[Audit Logger]
    
    ProfileMgr --> FileService[File Service]
    GHClient --> TokenStore[Token Store]
    TokenStore --> CryptoSvc[Crypto Service]
    TokenStore --> Keychain[OS Keychain]
    
    AuditLog --> LogFiles[JSONL Files]
    ProfileMgr --> ProfileYAML[Profile YAML]
    BackupMgr --> TarGz[.tar.gz Archives]
    ShellMgr --> ShellRC[Shell Config Files]
    
    style User fill:#e1f5ff
    style Container fill:#fff3cd
    style CryptoSvc fill:#f8d7da
    style FileService fill:#d4edda
```

---

## Profile Creation Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant ProfileMgr as Profile Manager
    participant Validator
    participant FileService as File Service
    participant AuditLog as Audit Logger

    User->>CLI: gcm profile create work -i
    CLI->>CLI: Interactive wizard (4 steps)
    CLI->>CLI: Build Profile struct
    CLI->>ProfileMgr: Create(profile)
    ProfileMgr->>Validator: ValidateProfile(profile)
    Validator-->>ProfileMgr: OK
    ProfileMgr->>ProfileMgr: Set metadata (created, version)
    ProfileMgr->>FileService: Write(~/.gcm/profiles/work.yaml)
    FileService-->>ProfileMgr: OK
    ProfileMgr-->>CLI: OK
    CLI->>AuditLog: Log(profile.create, "work")
    CLI-->>User: ✓ Profile "work" created
```

---

## Profile Activation Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Switcher
    participant ProfileMgr as Profile Manager
    participant Git
    participant Agent as SSH Agent
    participant Config
    participant AuditLog as Audit Logger

    User->>CLI: gcm use work --global
    CLI->>Switcher: Activate("work", ScopeGlobal)
    Switcher->>ProfileMgr: Get("work")
    ProfileMgr-->>Switcher: Profile data
    
    Switcher->>Git: git config user.name "Jane Doe"
    Switcher->>Git: git config user.email "jane@acme.example"
    Switcher->>Git: git config core.editor "code"
    Switcher->>Git: git config commit.gpgsign "true"
    
    Switcher->>Agent: ssh-add ~/.ssh/id_ed25519_work
    
    Switcher->>Config: Set default_profile = "work"
    Config->>Config: Save config.yaml
    
    Switcher->>ProfileMgr: IncrementUsage("work")
    Switcher-->>CLI: OK
    CLI->>AuditLog: Log(profile.activate, "work", scope=global)
    CLI-->>User: ✓ Profile "work" activated (global)
```

---

## Auto-Switch Flow (on `cd`)

```mermaid
flowchart TD
    Start[User: cd ~/projects/work] --> Hook{Shell Hook Triggered}
    Hook --> CheckFile{.gcm-profile exists?}
    CheckFile -->|No| End[No Action]
    CheckFile -->|Yes| ReadFile[Read profile name from file]
    ReadFile --> Refresh[gcm refresh --silent]
    Refresh --> GetCurrent[Get current profile]
    GetCurrent --> Same{Same profile?}
    Same -->|Yes| End
    Same -->|No| Activate[Activate new profile]
    Activate --> UpdateGit[Update Git config]
    UpdateGit --> UpdateSSH[Load SSH key into agent]
    UpdateSSH --> UpdatePrompt[Update prompt indicator]
    UpdatePrompt --> End
```

---

## SSH Key Generation Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant SSHMgr as SSH Manager
    participant Crypto as Go crypto
    participant FS as File System
    participant ProfileMgr as Profile Manager
    participant GitHub as GitHub API
    participant AuditLog as Audit Logger

    User->>CLI: gcm ssh generate work -t ed25519
    CLI->>SSHMgr: Generate(options)
    SSHMgr->>Crypto: Generate Ed25519 key pair
    Crypto-->>SSHMgr: Private key + Public key
    
    SSHMgr->>FS: Write private key (0600)
    SSHMgr->>FS: Write public key (0644)
    
    alt Passphrase provided
        SSHMgr->>Crypto: Encrypt passphrase (AES-256-GCM)
        SSHMgr->>FS: Store encrypted passphrase
    end
    
    SSHMgr->>SSHMgr: Compute fingerprint
    SSHMgr-->>CLI: KeyInfo (path, type, fingerprint, pubkey)
    
    CLI->>ProfileMgr: Update profile SSH config

    alt Provider token exists for profile
        CLI-->>User: Upload SSH key to provider automatically? [Y/n]
        User-->>CLI: Yes
        CLI->>Provider: Upload public key
        Provider-->>CLI: Created
        CLI-->>User: ✓ SSH key uploaded to provider!
    end

    CLI->>AuditLog: Log(ssh.generate, "work")
    CLI-->>User: ✓ SSH key generated
    CLI-->>User: Public key: ssh-ed25519 AAAA...
```

---

## GitHub Device Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant GHClient as GitHub Client
    participant GitHub as GitHub API
    participant TokenStore as Token Store
    participant Crypto as Crypto Service
    participant AuditLog as Audit Logger

    User->>CLI: gcm github login-oauth work
    CLI->>GHClient: InitiateDeviceFlow()
    GHClient->>GitHub: POST /login/device/code
    GitHub-->>GHClient: DeviceCode + UserCode + URL
    
    CLI-->>User: Open https://github.com/login/device
    CLI-->>User: Enter code: ABCD-1234
    
    loop Poll until authorized (up to 15 min)
        CLI->>GHClient: PollForToken(deviceCode, interval)
        GHClient->>GitHub: POST /login/oauth/access_token
        GitHub-->>GHClient: authorization_pending / token
    end
    
    GHClient-->>CLI: Access token
    CLI->>TokenStore: SaveToken("work", token)
    
    alt Keychain enabled
        TokenStore->>TokenStore: Store in OS keychain
    else Encryption enabled
        TokenStore->>Crypto: Encrypt(token)
        Crypto-->>TokenStore: Encrypted bytes
        TokenStore->>TokenStore: Write to ~/.gcm/tokens/work
    end
    
    CLI->>GHClient: GetUser()
    GHClient->>GitHub: GET /user (with token)
    GitHub-->>GHClient: User info
    
    CLI->>AuditLog: Log(github.login, "work")
    CLI-->>User: ✓ Logged in as jane-acme
```

---

## Backup & Restore Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant BackupMgr as Backup Manager
    participant FS as File System
    participant AuditLog as Audit Logger

    User->>CLI: gcm backup create
    CLI->>BackupMgr: Create()
    BackupMgr->>FS: Create ~/.gcm/backups/ (0700)
    BackupMgr->>BackupMgr: Open tar.gz writer
    BackupMgr->>FS: Read config.yaml → add to archive
    BackupMgr->>FS: Read profiles/*.yaml → add to archive
    BackupMgr->>FS: Read templates/*.yaml → add to archive
    BackupMgr->>BackupMgr: Close archive
    BackupMgr-->>CLI: BackupInfo (path, size, counts)
    CLI->>AuditLog: Log(backup.create)
    CLI-->>User: ✓ Backup created (path, size)
```

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant BackupMgr as Backup Manager
    participant FS as File System

    User->>CLI: gcm backup restore <file>
    CLI->>User: Confirm overwrite?
    User-->>CLI: Yes
    CLI->>BackupMgr: Restore(file)
    BackupMgr->>FS: Open tar.gz reader
    
    loop Each archive entry
        BackupMgr->>BackupMgr: Validate path (zip-slip check)
        alt Path escapes target
            BackupMgr-->>CLI: Error: path traversal detected
        else Path is safe
            BackupMgr->>FS: Extract to ~/.gcm/
        end
    end
    
    BackupMgr-->>CLI: OK
    CLI-->>User: ✓ Backup restored
```

---

## Credential Helper Flow

```mermaid
sequenceDiagram
    participant Git
    participant CredHelper as gcm credential-helper
    participant ProfileMgr as Profile Manager
    participant TokenStore as Token Store
    participant Crypto as Crypto Service

    Git->>CredHelper: get (protocol=https, host=github.com)
    CredHelper->>ProfileMgr: GetActiveProfile()
    ProfileMgr-->>CredHelper: "work"
    CredHelper->>TokenStore: GetToken("work")
    TokenStore->>Crypto: Decrypt(~/.gcm/tokens/work)
    Crypto-->>TokenStore: Plaintext token
    TokenStore-->>CredHelper: Token
    CredHelper-->>Git: username=work-user\npassword=<token>
```

Git calls `gcm credential-helper get` whenever it needs HTTPS credentials for a configured provider host. GCM resolves the active profile, decrypts the provider-aware token from its own store, and returns it directly — bypassing the system keychain entirely.

---

## Shell Integration Install Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant ShellMgr as Shell Manager
    participant FS as File System
    participant AuditLog as Audit Logger

    User->>CLI: gcm init
    CLI->>ShellMgr: DetectShell()
    ShellMgr->>ShellMgr: Check $SHELL env var
    ShellMgr-->>CLI: "zsh"
    
    CLI->>ShellMgr: Install("zsh")
    ShellMgr->>ShellMgr: Get config file (~/.zshrc)
    ShellMgr->>FS: Read ~/.zshrc
    ShellMgr->>ShellMgr: Check for existing markers
    
    alt Already installed
        ShellMgr-->>CLI: Error: already installed
    else Not installed
        ShellMgr->>ShellMgr: Generate hook code
        ShellMgr->>FS: Append to ~/.zshrc
        ShellMgr-->>CLI: Config file path
    end
    
    CLI->>AuditLog: Log(shell.init, shell=zsh)
    CLI-->>User: ✓ Shell integration installed
    CLI-->>User: Restart your shell
```

---

## Configuration Loading Flow

```mermaid
sequenceDiagram
    participant Main as main.go
    participant Config as config.Load()
    participant FS as File System

    Main->>Config: Load()
    Config->>Config: DefaultConfig()
    Config->>FS: Check ~/.gcm/config.yaml exists
    
    alt Config exists
        Config->>FS: Read YAML
        FS-->>Config: Raw bytes
        Config->>Config: Unmarshal YAML into Config struct
        Config->>Config: Merge with defaults (fill missing fields)
    else No config
        Config->>Config: Use DefaultConfig() as-is
    end
    
    Config->>Config: EnsureDirs() - create directories
    Config->>FS: MkdirAll profiles/, templates/, cache/
    Config->>FS: MkdirAll tokens/ (0700), logs/ (0700), backups/ (0700)
    Config-->>Main: *Config
```

---

## Token Storage Decision Tree

```mermaid
flowchart TD
    Start[SaveToken] --> CheckKeychain{use_keychain?}
    CheckKeychain -->|Yes| TryKeychain[Store in OS keychain]
    TryKeychain --> KeychainOK{Success?}
    KeychainOK -->|Yes| Done[Done]
    KeychainOK -->|No| FallThrough[Secure fallback]
    
    CheckKeychain -->|No| FallThrough
    FallThrough --> CheckEncrypt{encrypt_tokens + master_password?}
    CheckEncrypt -->|Yes| PromptPW[Prompt for master password]
    PromptPW --> DeriveKey[Argon2id derive key]
    DeriveKey --> Encrypt[AES-256-GCM encrypt]
    Encrypt --> WriteEnc[Write v2 header+salt+ciphertext to file 0600]
    WriteEnc --> Done
    
    CheckEncrypt -->|No| CheckPlain{allow_plaintext_tokens?}
    CheckPlain -->|Yes| WritePlain[Write plain text to file 0600]
    CheckPlain -->|No| FailClosed[Fail closed]
    WritePlain --> Done
```

---

## Component Dependencies

```mermaid
graph TD
    CLI[internal/cli] --> Container[internal/container]
    Container --> Config[internal/config]
    Container --> Profile[internal/profile]
    Container --> SSH[internal/ssh]
    Container --> GPG[internal/gpg]
    Container --> GitHub[internal/github]
    Container --> Shell[internal/shell]
    Container --> Template[internal/template]
    Container --> Backup[internal/backup]
    Container --> Audit[internal/audit]
    Container --> CryptoSvc[internal/service/crypto]
    Container --> FileSvc[internal/service/file]
    Container --> Logger[pkg/logger]
    
    Profile --> FileSvc
    Profile --> Config
    GitHub --> CryptoSvc
    GitHub --> Keyring[go-keyring]
    Template --> FileSvc
    
    CLI --> UI[pkg/ui]
    CLI --> Version[pkg/version]
    
    style CLI fill:#e1f5ff
    style Container fill:#fff3cd
    style Config fill:#d4edda
    style CryptoSvc fill:#f8d7da
    style FileSvc fill:#d4edda
```

---

## State Machine: Profile Activation

```mermaid
stateDiagram-v2
    [*] --> NoProfile: Fresh install
    NoProfile --> Session: gcm use X
    NoProfile --> Global: gcm use X --global
    NoProfile --> Local: gcm use X --local
    
    Session --> Session: gcm use Y
    Session --> Global: gcm use Y --global
    Session --> Local: gcm use Y --local
    Session --> NoProfile: Shell restart
    
    Global --> Session: gcm use Y (session only)
    Global --> Global: gcm use Y --global
    Global --> Local: gcm use Y --local
    Global --> Global: Shell restart (persists)
    
    Local --> Session: gcm use Y (session only)
    Local --> Global: gcm use Y --global
    Local --> Local: cd to dir with different .gcm-profile
```

---

## File System Layout

```mermaid
graph TB
    Home[~/.gcm/] --> ConfigYAML[config.yaml]
    Home --> Profiles[profiles/]
    Home --> Templates[templates/]
    Home --> Tokens[tokens/ 0700]
    Home --> Backups[backups/ 0700]
    Home --> Logs[logs/ 0700]
    Home --> Cache[cache/]
    
    Profiles --> Work[work.yaml]
    Profiles --> Personal[personal.yaml]
    
    Templates --> CompanyTpl[company-standard.yaml]
    
    Tokens --> WorkToken[work encrypted]
    Tokens --> PersonalToken[personal encrypted]
    
    Backups --> Backup1[gcm-backup-2026-05-18.tar.gz]
    
    Logs --> AuditLog[2026-05-18.jsonl]
    
    style Home fill:#e1f5ff
    style Tokens fill:#f8d7da
    style Backups fill:#f8d7da
    style Logs fill:#f8d7da
```

---

## See Also

- [Architecture Overview](architecture.md) — design patterns and principles
- [Project Structure](project-structure.md) — file-by-file map
- [Security Model](security.md) — encryption and permission details
