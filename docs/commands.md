# Commands Reference

Complete reference for every GCM command, subcommand, flag, and alias.

---

## Global Behavior

Every command supports these global flags:

| Flag             | Description                         |
| ---------------- | ----------------------------------- |
| `-h, --help`     | Help for the command                |
| `-v, --verbose`  | Enable verbose (debug) output       |
| `-q, --quiet`    | Suppress non-essential output       |
| `--no-color`     | Disable colored output              |

```bash
gcm --help
gcm profile --help
gcm ssh generate --help
gcm -v use work         # see debug logs during activation
gcm --no-color profile list   # plain text, no ANSI colors
```

GCM silences Cobra's default usage/error output — errors are displayed as clean messages without the full help text.

---

## `gcm setup`

Guided first-time setup wizard. Walks you through the complete GCM configuration.

```bash
gcm setup
gcm quickstart   # alias
```

**Aliases:** `gcm quickstart`

**What it does:**
1. Shell integration (auto-switching, prompt)
2. Creating your first profile (name, email)
3. Provider authentication (choose one provider for the profile)
4. SSH key generation with provider-aware filename
5. GPG signing (optional) and key upload offer when authenticated
6. Activating your profile

Perfect for first-time users. Run this once and you're fully set up.

---

## `gcm status`

Show a quick overview of your GCM setup.

```bash
gcm status
gcm st           # alias
```

**Aliases:** `gcm st`

Shows: active profile, all profiles summary, provider auth status, SSH keys, and any issues that need attention.

Provider token checks are bounded-concurrent when `advanced.parallel_operations` is enabled, so status stays responsive with many profiles.

For credential ownership, helper source, and external-auth inspection, use `gcm auth status` or `gcm auth inspect`.

---

## `gcm repair`

Inspect and optionally repair local provider/profile consistency.

```bash
gcm repair
gcm repair --fix
gcm repair --fix --yes
gcm repair --json
```

**What it checks:**
1. Credential helper registration for configured provider hosts
2. Mixed provider metadata that violates one-profile-one-provider
3. Provider-aware SSH key filename migration
4. Legacy GitHub token entries that can be migrated to provider-aware storage

By default, `repair` only reports. `--fix` applies safe local repairs and asks for confirmation unless `--yes` is supplied. `--json` emits a redacted machine-readable report for automation.

---

## `gcm auth`

Inspect and manage provider authentication ownership. These commands distinguish GCM-managed tokens from external Git credentials such as macOS Keychain, Windows Credential Manager, Git Credential Manager, GitHub CLI, libsecret, and other Git credential helpers.

GCM-owned credentials live in GCM's provider-aware token store. When Git returns a credential through `gcm credential-helper`, it is treated as GCM-owned effective Git auth, not as an external credential. External credentials are detected and explained, but GCM does not adopt or delete them unless you run an explicit `gcm auth adopt` or `gcm auth logout --scope external|all` command.

### `gcm auth status [profile]`

Show source-aware auth status for one profile or all profiles.

```bash
gcm auth status
gcm auth status work --provider github
gcm auth status work --provider gitlab --verbose
gcm auth status --json
```

**Output columns:** Profile, Provider, State, Owner, Source, Username, Findings.

Common states include `authenticated:gcm`, `authenticated:external`, `authenticated:mixed`, `expired`, `revoked`, `conflicted`, `unknown`, and `unauthenticated`. SSH-only profiles (no HTTPS credential) report `unauthenticated` with an `ssh_only` finding.

| Flag             | Short | Description                                      |
| ---------------- | ----- | ------------------------------------------------ |
| `--provider`     |       | Provider to inspect (`github`, `gitlab`)         |
| `--json`         |       | Print machine-readable JSON                      |
| `--verbose`      | `-v` | Show credential helper and finding details       |

### `gcm auth inspect <profile>`

Inspect GCM, external Git, SSH, capabilities, credential helpers, findings, and recommendations for one profile.

```bash
gcm auth inspect work
gcm auth inspect work --provider github
gcm auth inspect work --json
```

`--provider` is required when the profile has no configured provider. Inspection is read-only; it does not store, adopt, or delete credentials.

### `gcm auth adopt <profile>`

Adopt an exportable external Git credential into GCM's provider-aware token store.

```bash
gcm auth adopt work --provider github --dry-run
gcm auth adopt work --provider github --yes
```

The external credential is verified against the selected provider before it is saved. If the profile already has a different GCM-managed token, GCM asks before replacing it unless `--yes` is supplied.

| Flag             | Short | Description                                      |
| ---------------- | ----- | ------------------------------------------------ |
| `--provider`     |       | Provider to adopt; required when profile has no provider |
| `--dry-run`      |       | Show what would be adopted without writing       |
| `--yes`          | `-y` | Confirm adoption without prompting               |

### `gcm auth logout <profile>`

Remove credentials safely by ownership scope.

```bash
gcm auth logout work                         # default: --scope gcm
gcm auth logout work --scope gcm             # remove only GCM-owned token
gcm auth logout work --scope external --dry-run
gcm auth logout work --scope external --yes
gcm auth logout work --scope all --yes
```

Default logout only removes GCM-owned credentials. External credential deletion uses `git credential reject` and requires confirmation unless `--yes` is supplied. Credentials served by `gcm credential-helper` are skipped by `--scope external` because they are already owned by GCM.

| Flag             | Short | Default | Description                                      |
| ---------------- | ----- | ------- | ------------------------------------------------ |
| `--scope`        |       | `gcm`   | Credential scope: `gcm`, `external`, or `all`    |
| `--provider`     |       |         | Provider to log out (`github`, `gitlab`)         |
| `--dry-run`      |       | false   | Show what would be deleted without writing       |
| `--yes`          | `-y` | false   | Confirm external credential deletion             |
| `--json`         |       | false   | Print post-logout status as JSON                 |

### `gcm auth doctor [profile]`

Diagnose auth ownership and helper issues.

```bash
gcm auth doctor
gcm auth doctor work --provider github
gcm auth doctor --json
```

Findings include missing GCM credential helper registration, external credentials that are not GCM-owned, stale/revoked GCM tokens, mixed credentials, account mismatches, and unauthenticated provider profiles. Profiles with no provider configured are local-only and are not counted as auth doctor issues unless you inspect them directly with `gcm auth status`.

### `gcm auth repair [profile]`

Repair safe local auth configuration issues.

```bash
gcm auth repair --dry-run
gcm auth repair --yes
gcm auth repair work --provider gitlab --json
```

Current safe repair action: register GCM as the Git credential helper for configured provider hosts when it is missing. It does not delete or adopt external credentials.

| Flag             | Short | Description                                      |
| ---------------- | ----- | ------------------------------------------------ |
| `--provider`     |       | Provider to repair (`github`, `gitlab`)          |
| `--dry-run`      |       | Show repairs without applying them               |
| `--yes`          | `-y` | Apply repairs without prompting                  |
| `--json`         |       | Print machine-readable JSON                      |

---

## `gcm connect <profile>`

Connect a profile to a Git provider using the provider-neutral PAT workflow.

```bash
gcm connect work --provider github
gcm connect work --provider gitlab
echo "$GITLAB_TOKEN" | gcm connect work --provider gitlab --token-stdin --yes
```

**What it does:**
1. Verifies the token against the selected provider
2. Applies the one-provider-per-profile invariant
3. Cleans old provider data when changing providers
4. Saves the token in provider-aware storage
5. Updates git credentials when the profile is active
6. Offers SSH/GPG upload in interactive mode

| Flag             | Description                                      |
| ---------------- | ------------------------------------------------ |
| `--provider`     | Provider to connect (`github`, `gitlab`)         |
| `--token-stdin`  | Read token from stdin for headless automation    |
| `--yes`, `-y`    | Confirm provider transition cleanup automatically |

---

## `gcm switch-provider <profile> <provider>`

Move a profile to another provider with explicit cleanup semantics.

```bash
gcm switch-provider work gitlab
echo "$GH_TOKEN" | gcm switch-provider work github --token-stdin --yes
```

This command uses the same verification and cleanup path as `gcm connect`, but requires the target provider as an argument to make the intent explicit.

---

## `gcm init`

Set up shell integration and credential helper.

```bash
gcm init
gcm init --force
gcm init --clear-global-identity
```

**What it does:**
1. Detects your shell (`bash`, `zsh`, `fish`, `powershell`)
2. Appends a marked hook block to your shell config file
3. Registers GCM as git's credential helper for configured provider hosts
4. Reports the shell and config file path

**Output:** The config file path. Restart your shell afterward.

| Flag                      | Short | Default | Description                                     |
| ------------------------- | ----- | ------- | ----------------------------------------------- |
| `--force`                 | `-f`  | false   | Reinstall shell integration if already present  |
| `--clear-global-identity` |       | false   | Explicitly unset global git identity values     |

**Notes:**
- Existing global `user.name`, `user.email`, and `user.signingkey` values are left unchanged unless `--clear-global-identity` is passed.
- Without `--force`, an existing shell integration block is left in place.
- If auto-detection fails, it suggests specifying manually.
- The credential helper makes git authentication immune to external credential store changes (VS Code logout, Keychain edits, etc.)

**See also:** [Shell Integration](shell-integration.md)

---

## `gcm profile`

Manage Git profiles. Running `gcm profile` with no subcommand defaults to `gcm profile list`.

**Aliases:** `gcm p`

### `gcm profile create <name>`

Create a new Git profile.

```bash
gcm profile create work -i
gcm profile create work --name "Jane Doe" --email "jane@acme.example"
gcm profile create work --name "Jane" --email "jane@acme.example" --ssh-key ~/.ssh/id_ed25519_work --editor code
```

| Flag               | Short | Default | Description                        |
| ------------------ | ----- | ------- | ---------------------------------- |
| `--interactive`    | `-i`  | false   | Launch the 4-step interactive wizard |
| `--name`           | `-n`  |         | Git `user.name`                    |
| `--email`          | `-e`  |         | Git `user.email`                   |
| `--editor`         |       |         | Git `core.editor`                  |
| `--ssh-key`        |       |         | Path to an existing SSH private key |
| `--from-template`  | `-t`  |         | Apply settings from a template     |

**Interactive wizard steps:**
1. **Basic info** — name, email, editor
2. **SSH key** — generate new key (Ed25519/RSA/ECDSA) or skip
3. **GPG signing** — generate new GPG key or skip
4. **Provider account** — choose one provider for the profile, set its username, or skip

If you change an existing profile from one provider to another, GCM asks for explicit confirmation before cleaning old provider data: stored token, cached git credentials, credential username, uploaded SSH/GPG keys when the old token can still access them, and the local SSH key filename.

**Validation:**
- `--name` and `--email` are required unless `--interactive` is set
- Profile name must start with a letter or digit and may contain only letters, digits, `-`, or `_`
- Profile names that differ only by case are rejected as duplicates for cross-platform consistency

### `gcm profile list`

List all profiles with status, email, provider, signing, and last used.

```bash
gcm profile list
gcm profile ls        # alias
gcm profile           # also runs list
```

**Output columns:** Profile, Status (`active`, `default`, `active default`), Email, Provider, Signing (✓/✗), Last Used.

### `gcm profile show <name>`

Display detailed information about a profile.

```bash
gcm profile show work
```

**Shows:** Git user/email/editor, SSH key path/type/fingerprint, GPG key ID, provider account, metadata (created, usage count).

### `gcm profile edit <name>`

Edit a profile's identity and provider account.

```bash
gcm profile edit work -n "Jane Smith"
gcm profile edit work -e "jane.smith@acme.example"
gcm profile edit work -n "Jane Smith" -e "jane.smith@acme.example"
```

| Flag      | Short | Description         |
| --------- | ----- | ------------------- |
| `--name`  | `-n`  | Update `user.name`  |
| `--email` | `-e`  | Update `user.email` |

Interactive edit can change the profile provider. Provider changes are treated as a destructive transition: GCM confirms first, then removes old provider credentials and remote uploaded keys before saving the new provider account. For a direct provider login/switch workflow, prefer `gcm connect` or `gcm switch-provider`.

### `gcm profile delete <name>`

Delete a profile. Prompts for confirmation unless `--yes` is set.

```bash
gcm profile delete old-work
gcm profile delete old-work -y     # skip confirmation
gcm profile rm old-work            # alias
```

| Flag    | Short | Default | Description          |
| ------- | ----- | ------- | -------------------- |
| `--yes` | `-y`  | false   | Skip confirmation    |

**Aliases:** `rm`

### `gcm profile export <name>`

Export a profile as YAML to stdout.

```bash
gcm profile export work > work-profile.yaml
```

### `gcm profile import <file>`

Import a profile from a YAML file.

```bash
gcm profile import work-profile.yaml
```

### `gcm profile diff <profile1> <profile2>`

Compare two profiles side by side.

```bash
gcm profile diff work personal
```

**Compares:** Name, Email, SSH key path. Only fields that differ are shown.

---

## `gcm use <name>`

Activate a profile.

```bash
gcm use work                 # smart: session (git repo) or local (elsewhere)
gcm use work --global        # set as machine-wide default
gcm use work --local         # pin to current directory
gcm use work --dry-run       # preview changes, apply nothing
```

| Flag        | Short | Default | Description                              |
| ----------- | ----- | ------- | ---------------------------------------- |
| `--global`  | `-g`  | false   | Persist as the default profile           |
| `--local`   | `-l`  | false   | Write `.gcm-profile` in current directory |
| `--dry-run` |       | false   | Show what would change without applying  |

**What it does (non-dry-run):**
1. Loads the profile from `~/.gcm/profiles/<name>.yaml`
2. Writes Git config (`user.name`, `user.email`, `core.editor`, `commit.gpgsign`, `user.signingkey`)
3. Loads the SSH key into the ssh-agent via `ssh-add` (if configured and key file exists)
4. **Pins provider credential usernames** so git only uses credentials belonging to this profile
5. If GCM is the credential helper: git will ask GCM dynamically for credentials (no system keychain involved)
6. If GCM is NOT the credential helper (legacy): clears old credentials via `git credential reject` and stores the new profile's token via `git credential approve`
7. Increments the profile's `usage_count` and `last_used`
8. Logs the activation to the audit log
9. Verifies configured provider token validity (best-effort, warns if expired)

> **Credential Isolation:** After switching, git clone/push/pull will only work with the active profile's configured provider account. Other profiles' credentials cannot bleed through.

**Scopes:**
- **Session** (default, in a git repo) — writes a `.git/gcm-session` marker file for reliable detection, plus local git config
- **Local** (default, outside a git repo) — creates a `.gcm-profile` file in the current directory
- **Global** (`--global`) — saves as `default_profile` in `config.yaml`, persists across shell restarts; also clears any local/session overrides in the current directory

**Smart scope fallback (no explicit flag):**
- Inside a git repo → session scope
- Outside a git repo → local scope (writes `.gcm-profile`)
- This means `gcm use <name>` **always works** regardless of whether you're in a git repository.

---

## `gcm current`

Show the currently active profile.

```bash
gcm current              # full details (user, scope, SSH, GPG)
gcm current --short      # just the profile name (for scripts/prompts)
gcm current --short --hide-default  # nothing if default is active (shell prompts)
```

| Flag             | Default | Description                                              |
| ---------------- | ------- | -------------------------------------------------------- |
| `--short`        | false   | Print only the profile name                              |
| `--hide-default` | false   | Output nothing if the active profile is the default one  |

> **Shell prompt tip:** Use `gcm current --short --hide-default` in your prompt so it only shows a profile indicator when you've explicitly switched away from your default.

**Detection order:**
1. `.git/gcm-session` marker file (written by `gcm use`) → `session`
2. `git config --local user.email` matched against known profiles → `session` (fallback)
3. `.gcm-profile` in the current directory → `local`
4. `default_profile` in `config.yaml` → `global`

> **Why session first?** `gcm use <name>` is an explicit user action and should always take effect immediately. The `.gcm-profile` file is a directory-level pin that serves as a default for that project, but can be overridden by an explicit switch.

---

## `gcm refresh`

Re-evaluate the current directory and activate the appropriate profile.

```bash
gcm refresh              # normal output
gcm refresh --silent     # suppress all output (used by shell hooks)
```

| Flag       | Default | Description              |
| ---------- | ------- | ------------------------ |
| `--silent` | false   | Suppress all output      |

This is the command that shell hooks call on every `cd`.

---

## `gcm ssh`

Manage SSH keys. Running `gcm ssh` with no subcommand defaults to `gcm ssh list`.

### `gcm ssh generate <profile>`

Generate a new SSH key pair or link an existing provider-aware local key to a profile.

```bash
gcm ssh generate work                          # Ed25519, no passphrase
gcm ssh generate work -t rsa -b 4096           # RSA 4096
gcm ssh generate work -t ecdsa                 # ECDSA P-256
gcm ssh generate work -c "work laptop"         # custom comment
gcm ssh generate work -p "my-passphrase"       # with passphrase
gcm ssh generate work --overwrite              # replace existing local key pair
```

| Flag             | Short | Default    | Description                          |
| ---------------- | ----- | ---------- | ------------------------------------ |
| `--type`         | `-t`  | `ed25519`  | Key type: `ed25519`, `rsa`, `ecdsa`  |
| `--bits`         | `-b`  | `4096`     | Key size (RSA only: 2048/3072/4096)  |
| `--comment`      | `-c`  |            | Comment baked into the key           |
| `--passphrase`   | `-p`  |            | Key passphrase (encrypted at rest)   |
| `--overwrite`    |       | `false`    | Replace an existing local key pair at the expected provider-aware path |

**What it does:**
1. Uses the deterministic provider-aware local filename, for example `id_ed25519_work_github`
2. If that exact key pair already exists and the profile has no SSH key configured, links the existing key to the profile instead of failing
3. Generates a new key pair using Go's native `crypto` library when no matching key exists
4. Writes private key with `0600` permissions, public key with `0644`
5. If a passphrase is given, encrypts the private key at rest using OpenSSH native format (bcrypt-KDF + AES-256-CTR)
6. Updates the profile's SSH configuration automatically
7. Prints the public key for easy copying
8. If a token is stored for this profile's provider, offers to upload the key to that provider

GCM does not overwrite an existing local SSH key by default. Use `--overwrite` only when you intentionally want to replace the old local key pair while keeping the same deterministic filename and provider upload title.

### `gcm ssh list`

List SSH keys across all profiles.

```bash
gcm ssh list
gcm ssh ls          # alias
gcm ssh             # also runs list
```

**Output columns:** Key path, Type, Fingerprint, Agent (✓/✗).

### `gcm ssh test <profile>`

Test the SSH connection to a configured provider using the profile's key.

```bash
gcm ssh test work-github --provider github
gcm ssh test work-gitlab --provider gitlab
```

Runs `ssh -T git@<provider-ssh-host>` with the profile's key. If `--provider` is omitted, GCM uses the profile's configured provider. If `--provider` is provided, it must match the profile provider.

### `gcm ssh copy <profile>`

Print the public key to stdout.

```bash
gcm ssh copy work
gcm ssh copy work | pbcopy    # macOS: copy to clipboard
```

### `gcm ssh upload <profile>`

Upload the profile's SSH public key to its configured provider. Checks for duplicates before uploading.

```bash
gcm ssh upload work-github --provider github
gcm ssh upload work-gitlab --provider gitlab
gcm ssh upload work-gitlab --provider gitlab --force    # skip duplicate check
```

| Flag         | Short | Default | Description                    |
| ------------ | ----- | ------- | ------------------------------ |
| `--provider` |       | `""`    | Provider to upload to          |
| `--force`    | `-f`  | `false` | Skip duplicate check           |

**What it does:**
1. Resolves the profile's configured provider
2. Loads the stored provider token for the profile
3. Lists existing SSH keys on that provider and compares by key material
4. If the key already exists, reports "already uploaded" and exits
5. Otherwise, uploads with title `gcm-<profile>-<provider>-ssh-<type>`

If the provider rejects the upload because the key is already registered but it was not listed for the authenticated account, that key belongs to another account or deploy key. Remove it from the other owner, or generate a fresh profile key with `gcm ssh generate <profile> --overwrite` and retry `gcm ssh upload <profile>`.

---

## `gcm gpg`

Manage GPG keys. Running `gcm gpg` with no subcommand defaults to `gcm gpg list`.

### `gcm gpg generate <profile>`

Generate a new GPG key for a profile (uses the profile's name and email).

```bash
gcm gpg generate work
```

The profile must already have `user.name` and `user.email` set.

After generation, the profile is automatically updated with the GPG key ID and `commit.gpgsign = true`.
If a token is stored for this profile's provider, GCM will offer to upload the GPG public key there so commits can show as verified.

### `gcm gpg list`

List GPG keys known to the system.

```bash
gcm gpg list
gcm gpg ls          # alias
gcm gpg             # also runs list
```

**Output columns:** Key ID, Name, Email, Created, Trust.

If GPG is not installed, shows a warning with installation instructions.

### `gcm gpg sign enable <profile>`

Enable commit signing for a profile.

```bash
gcm gpg sign enable work
```

Requires a GPG key to be configured on the profile. Sets `commit.gpgsign = true` and `user.signingkey = <key-id>`.

### `gcm gpg sign disable <profile>`

Disable commit signing for a profile.

```bash
gcm gpg sign disable work
```

### `gcm gpg test <profile>`

Test GPG signing capability.

```bash
gcm gpg test work
```

Performs a test signature using the profile's GPG key ID.

### `gcm gpg upload <profile>`

Upload the profile's GPG public key to its configured provider for commit verification. Checks for duplicates before uploading.

```bash
gcm gpg upload work-github --provider github
gcm gpg upload work-gitlab --provider gitlab
gcm gpg upload work-gitlab --provider gitlab --force    # skip duplicate check
```

| Flag         | Short | Default | Description                    |
| ------------ | ----- | ------- | ------------------------------ |
| `--provider` |       | `""`    | Provider to upload to          |
| `--force`    | `-f`  | `false` | Skip duplicate check           |

**What it does:**
1. Resolves the profile's configured provider
2. Loads the stored provider token for the profile
3. Lists existing GPG keys on that provider and compares by key ID
4. If the key already exists, reports "already uploaded" and exits
5. Otherwise, exports the armored public key and uploads to the provider

---

## `gcm github`

Manage GitHub integration. Supports multiple authentication methods.

**Aliases:** `gcm gh`

### `gcm github login <profile>`

Authenticate with a Personal Access Token (PAT). Useful for CI/CD, headless environments, or fine-grained token control.

```bash
gcm github login work-github                          # interactive (masked input)
echo "$GH_TOKEN" | gcm github login work-github       # piped from env/script
```

**Requirements:** Generate a token at https://github.com/settings/tokens with scopes: `repo`, `admin:public_key`, `admin:gpg_key`.

**What it does:**
1. Reads token from interactive input or stdin pipe
2. Verifies the token against the GitHub API
3. Encrypts and stores the token
4. Updates the profile so GitHub is its single configured provider
5. **If this is the active profile**, git credentials are stored for clone/push/pull

> **Note:** Git credentials are only updated if the profile being logged in is the currently active one. Logging into a non-active profile saves the token but does not affect git operations until you switch to that profile with `gcm use`.

### `gcm github login-oauth <profile>`

Authenticate with GitHub using the OAuth device flow (interactive, browser-based).

```bash
gcm github login-oauth work-github
```

**Flow:**
1. Initiates the OAuth device flow
2. Displays a URL and a user code
3. You open the URL in your browser and enter the code
4. GCM polls until you approve (up to 15 minutes)
5. Token is encrypted and stored in `~/.gcm/tokens/`
6. Profile is updated so GitHub is its single configured provider
7. **If this is the active profile**, git credentials are stored for clone/push/pull

### `gcm github login-gh <profile>`

Import authentication from the GitHub CLI (`gh`). Requires `gh` to be installed and authenticated.

```bash
gcm github login-gh work-github
```

**What it does:**
1. Runs `gh auth token` to retrieve your existing token
2. Verifies the token against the GitHub API
3. Encrypts and stores it in GCM
4. Updates the profile so GitHub is its single configured provider

**Note:** Requires the GitHub CLI (https://cli.github.com) to be installed and logged in (`gh auth login`).

### `gcm github status`

Show source-aware authentication status for GitHub-scoped profiles. This delegates to the same resolver as `gcm auth status` and reports ownership/source information.

```bash
gcm github status
```

**Output columns:** Profile, Provider, State, Owner, Source, Username, Findings.

### `gcm github logout <profile>`

Remove the stored GitHub token for a profile and clear cached HTTPS Git credentials.

```bash
gcm github logout work-github                         # remove token + clear HTTPS credentials
gcm github logout work-github --clear-credentials=false  # only remove GCM token
```

| Flag                  | Short | Default | Description                                                 |
| --------------------- | ----- | ------- | ----------------------------------------------------------- |
| `--clear-credentials` |       | true    | Also clear cached HTTPS credentials and the provider username pin |
| `--force`             | `-f`  | false   | Skip confirmation when logging out a non-active profile     |

**What it does:**
1. If logging out a non-active profile, prompts for confirmation (skip with `--force`)
2. Deletes the stored token from the keychain/encrypted file
3. (If `--clear-credentials` AND the profile is currently active) Clears HTTPS credentials for GitHub from the system credential store and unsets `credential.https://github.com.username`

> **Note:** HTTPS credentials and username pinning are only cleared if the profile being logged out is the currently active one. SSH remotes and profile SSH keys are not affected, so `git pull`/`git push` can still work over SSH after logout.

### `gcm github verify <profile>`

Verify that the stored token is still valid.

```bash
gcm github verify work-github
```

### `gcm github user <profile>`

Show GitHub user information for the authenticated profile.

```bash
gcm github user work-github
```

**Shows:** Login, Name, Email, Company, Location, Public repos, Profile URL.

---

## `gcm gitlab`

Manage GitLab integration. The current GitLab MVP supports Personal Access Token authentication, token verification, credential helper integration, status checks, and SSH/GPG key upload after login.

**Aliases:** `gcm gl`

### `gcm gitlab login <profile>`

Authenticate with a GitLab Personal Access Token (PAT).

```bash
gcm gitlab login work-gitlab
echo "$GITLAB_TOKEN" | gcm gitlab login work-gitlab
```

**Recommended scopes:** `api`, `read_user`, `read_repository`, `write_repository`.

For self-managed GitLab, configure `providers.gitlab.api_url`, `providers.gitlab.web_url`, and `providers.gitlab.git_hosts` before login.

**What it does:**
1. Reads token from interactive input or stdin pipe
2. Verifies the token against the configured GitLab API
3. Stores a provider-aware token under the profile/provider/host key
4. Updates the profile so GitLab is its single configured provider
5. If this is the active profile, updates Git credentials for the configured GitLab host
6. During interactive login, offers to upload SSH/GPG keys when available

### `gcm gitlab status`

Show source-aware authentication status for GitLab-scoped profiles. This delegates to the same resolver as `gcm auth status` and reports ownership/source information.

```bash
gcm gitlab status
```

**Output columns:** Profile, Provider, State, Owner, Source, Username, Findings.

### `gcm gitlab logout <profile>`

Remove the stored GitLab token for a profile and optionally clear active HTTPS Git credentials. SSH remotes and profile SSH keys are not affected.

```bash
gcm gitlab logout work-gitlab
gcm gitlab logout work-gitlab --clear-credentials=false
```

### `gcm gitlab verify <profile>`

Verify that the stored GitLab token is still valid.

```bash
gcm gitlab verify work-gitlab
```

### `gcm gitlab user <profile>`

Show GitLab user information for the authenticated profile.

```bash
gcm gitlab user work-gitlab
```

---

## `gcm template`

Manage configuration templates. Running `gcm template` with no subcommand defaults to `gcm template list`.

**Aliases:** `gcm tpl`

### `gcm template create <name>`

Create a new configuration template. Templates store reusable git settings (editor, pull strategy, signing, aliases).

```bash
gcm template create company-standard -i
gcm template create company-standard --from-profile work
gcm template create minimal --editor "code --wait" --rebase true
gcm template create team --editor vim --gpg-sign true --alias "co=checkout,st=status"
```

| Flag              | Short | Default | Description                              |
| ----------------- | ----- | ------- | ---------------------------------------- |
| `--interactive`   | `-i`  | false   | Interactive creation wizard               |
| `--description`   | `-d`  |         | Template description                     |
| `--from-profile`  |       |         | Extract settings from an existing profile |
| `--editor`        |       |         | Git editor (e.g. `vim`, `code --wait`)   |
| `--rebase`        |       |         | Pull rebase strategy (true/false/merges) |
| `--gpg-sign`      |       |         | Enable commit signing (true/false)       |
| `--alias`         |       |         | Git aliases (format: key=value)          |

### `gcm template list`

List all templates.

```bash
gcm template list
gcm template ls         # alias
gcm template            # also runs list
```

**Output columns:** Template name, Description, Version, Created.

### `gcm template show <name>`

Show template details (prints the full YAML).

```bash
gcm template show company-standard
```

### `gcm template import <file>`

Import a template from a YAML file.

```bash
gcm template import company-standard.yaml
```

### `gcm template export <name>`

Export a template as YAML to stdout.

```bash
gcm template export company-standard > company.yaml
```

### `gcm template apply <template> <profile>`

Apply a template's git settings to an existing profile. Merges settings (editor, pull strategy, commit signing, push settings, aliases) into the target profile. Identity fields (name, email, SSH, GPG keys) are never modified.

```bash
gcm template apply company-standard work
gcm template apply company-standard work --force   # skip confirmation
```

| Flag      | Short | Default | Description         |
| --------- | ----- | ------- | ------------------- |
| `--force` |       | false   | Skip confirmation   |

### `gcm template delete <name>`

Delete a template.

```bash
gcm template delete company-standard
gcm template rm company-standard     # alias
```

---

## `gcm backup`

Backup and restore GCM data.

### `gcm backup create`

Create a timestamped unencrypted `.tar.gz` backup of GCM config, profiles, and templates.

```bash
gcm backup create
```

**Includes:** `config.yaml`, all profiles, all templates. Backup is stored in `~/.gcm/backups/` with `0600` permissions. Provider tokens, audit logs, and SSH private keys are not included.

`backup.encryption: true` and `backup.include_keys: true` fail closed because encrypted/key-inclusive backups are not implemented yet. After creation, configured `backup.retention_days` and `backup.max_backups` are enforced best-effort.

### `gcm backup list`

List all backups.

```bash
gcm backup list
gcm backup ls          # alias
```

**Output columns:** Date, Size, Path.

### `gcm backup restore <file>`

Restore from a backup file. Prompts for confirmation before overwriting.

```bash
gcm backup restore ~/.gcm/backups/gcm-backup-2026-05-18-143000.tar.gz
```

Restoration is staged before live files are replaced and is guarded against path-traversal (zip-slip) attacks.

### `gcm backup prune`

Remove old backups, keeping the most recent ones.

```bash
gcm backup prune              # keep last 5 (default)
gcm backup prune --keep 10    # keep last 10
```

| Flag     | Default | Description                      |
| -------- | ------- | -------------------------------- |
| `--keep` | `5`     | Number of recent backups to keep |

---

## `gcm validate`

Validate profile configurations.

```bash
gcm validate            # validate all profiles
gcm validate work       # validate a specific profile
```

**Checks:**
- YAML schema and required fields
- SSH key paths exist and have correct permissions
- GPG key IDs exist in the keyring
- Email format is valid

**Output:** Per-profile results with ✓ (info), ⚠ (warning), ✗ (error) indicators, plus suggestions for fixing issues.

---

## `gcm doctor`

Run a full system health check.

```bash
gcm doctor
```

**Checks:**
- **System:** OS, architecture, Go version
- **Dependencies:** Git, SSH, GPG (version and availability)
- **Services:** SSH agent status
- **Shell:** Shell integration status
- **Credential Helper:** Whether GCM is registered as git's credential helper for configured provider hosts
- **Configuration:** Config file location, profile count, template count

---

## `gcm clean`

Clean cache and temporary files.

```bash
gcm clean            # remove cache directory
gcm clean --all      # also remove logs directory
```

| Flag    | Default | Description                                |
| ------- | ------- | ------------------------------------------ |
| `--all` | false   | Also clean the logs directory              |

**Note:** This does *not* remove profiles, tokens, or backups. For a full reset, see [Troubleshooting](troubleshooting.md#full-reset).

---

## `gcm version`

Show version information.

```bash
gcm version              # full output
gcm version --short      # just the version number
```

| Flag      | Default | Description               |
| --------- | ------- | ------------------------- |
| `--short` | false   | Short version output only |

**Full output shows:** Version, Commit hash, Build date, Go version, OS/Architecture.

---

## `gcm credential-helper` (Internal)

Git credential helper implementation. This command is hidden from normal help output and is called by git itself when performing authentication.

```bash
# Not called directly — git invokes it automatically when configured:
# credential.https://github.com.helper = !'/path/to/gcm' credential-helper
# credential.https://gitlab.com.helper = !'/path/to/gcm' credential-helper
```

**Subcommands:**
- `get` — Reads the active profile's token from GCM's encrypted store and returns it to git in credential protocol format
- `store` — No-op (GCM manages its own token storage)
- `erase` — No-op (use provider logout commands to remove tokens)

**Registration:**
- Automatically registered by `gcm init` and `gcm setup`
- Verified by `gcm doctor`

**How it works:**
1. Git calls `gcm credential-helper get` when it needs credentials for a configured provider host
2. GCM resolves the provider from the host and determines the active profile via session/local/global scope
3. GCM loads the provider-aware token from its encrypted store
4. GCM returns `protocol`, `host`, `username`, and `password` to git

This makes git authentication independent of the system keychain (macOS Keychain, Windows Credential Manager, etc.), so external credential changes (VS Code logout, browser session clear) cannot break git operations.

---

## Exit Codes

| Code | Meaning                                  |
| ---- | ---------------------------------------- |
| `0`  | Success                                  |
| `1`  | General error (invalid args, not found)  |

---

## Environment Variables

| Variable              | Description                                      |
| --------------------- | ------------------------------------------------ |
| `_GCM_PROMPT`         | Shell variable set by precmd hook with active profile name (used in prompt) |
| `SHELL`               | Used by `gcm init` to detect your shell          |
| `HOME`                | Used to locate `~/.gcm/`                         |

---

## Command Aliases

| Full Command      | Alias              |
| ----------------- | ------------------ |
| `gcm setup`       | `gcm quickstart`   |
| `gcm status`      | `gcm st`           |
| `gcm profile`     | `gcm p`            |
| `gcm github`      | `gcm gh`           |
| `gcm template`    | `gcm tpl`          |
| `gcm profile list`| `gcm profile ls`   |
| `gcm profile delete`| `gcm profile rm` |
| `gcm ssh list`    | `gcm ssh ls`       |
| `gcm gpg list`    | `gcm gpg ls`       |
| `gcm backup list` | `gcm backup ls`    |
| `gcm template list`| `gcm template ls` |
| `gcm template delete`| `gcm template rm`|

---

## Command Cheatsheet

```text
Getting Started
  gcm setup                                    First-time setup wizard
  gcm status                                   Overview of current state
  gcm init                                     Install shell hooks
  gcm doctor                                   System health check
  gcm version [--short]                        Show version info

Profiles
  gcm profile create <name> [-i] [--name] [--email] [--ssh-key] [--editor] [--from-template]
  gcm profile list                             List all profiles
  gcm profile show <name>                      Show profile details
  gcm profile edit <name> [-n] [-e]            Edit name/email
  gcm profile delete <name> [-y]               Delete profile
  gcm profile export <name>                    Export as YAML
  gcm profile import <file>                    Import from YAML
  gcm profile diff <a> <b>                     Compare two profiles

Activation
  gcm use <name> [-g | -l | --dry-run]         Activate profile
  gcm current [--short]                         Show active profile
  gcm refresh [--silent]                        Re-evaluate directory

Auth
  gcm auth status [name] [--provider] [--json]  Source-aware auth status
  gcm auth inspect <name> [--provider]          Inspect auth sources
  gcm auth adopt <name> [--provider] [--dry-run] Adopt external auth
  gcm auth logout <name> [--scope gcm|external|all] Safe logout
  gcm auth doctor [name] [--json]               Diagnose auth ownership
  gcm auth repair [name] [--dry-run] [--yes]    Repair safe auth issues

SSH
  gcm ssh generate <name> [-t] [-b] [-c] [-p] [--overwrite]  Generate or link key
  gcm ssh upload <name> [--provider] [--force]  Upload key to provider
  gcm ssh list                                  List all keys
  gcm ssh test <name> [--provider]              Test provider connection
  gcm ssh copy <name>                           Print public key

GPG
  gcm gpg generate <name>                       Generate GPG key
  gcm gpg upload <name> [--provider] [--force]  Upload key to provider
  gcm gpg list                                  List GPG keys
  gcm gpg sign enable|disable <name>            Toggle signing
  gcm gpg test <name>                           Test signing

GitHub
  gcm github login <name>                       Personal Access Token
  gcm github login-oauth <name>                 OAuth device flow
  gcm github login-gh <name>                    Import from gh CLI
  gcm github status                             Source-aware auth status
  gcm github logout <name>                      Remove token
  gcm github verify <name>                      Verify token
  gcm github user <name>                        Show user info

GitLab
  gcm gitlab login <name>                       Personal Access Token
  gcm gitlab status                             Source-aware auth status
  gcm gitlab logout <name>                      Remove token
  gcm gitlab verify <name>                      Verify token
  gcm gitlab user <name>                        Show user info

Templates
  gcm template create <name> [-i] [--from-profile]  Create template
  gcm template list                             List templates
  gcm template show <name>                      Show details
  gcm template apply <tpl> <profile> [--force]  Apply to profile
  gcm template import <file>                    Import template
  gcm template export <name>                    Export template
  gcm template delete <name>                    Delete template

Backup
  gcm backup create                             Create backup
  gcm backup list                               List backups
  gcm backup restore <file>                     Restore backup
  gcm backup prune [--keep N]                   Remove old backups

Maintenance
  gcm validate [name]                           Validate profiles
  gcm clean [--all]                             Clean cache/logs

Internal (used by git)
  gcm credential-helper get                     Provide credentials to git
  gcm credential-helper store                   No-op (GCM manages own store)
  gcm credential-helper erase                   No-op (GCM manages own store)
```
