# Provider Integrations

GCM supports Git hosting providers through a provider-aware architecture. GitHub remains backward compatible, and GitLab is available as the first non-GitHub provider. Each profile is scoped to exactly one provider, so a GitHub identity and a GitLab identity should be modeled as separate profiles. Bitbucket support should be added by implementing the same provider contracts instead of adding provider-specific branching in CLI commands.

---

## Current Providers

| Provider | Status | Auth | Credential Helper | SSH Keys | GPG Keys |
| -------- | ------ | ---- | ----------------- | -------- | -------- |
| GitHub | Stable | PAT, OAuth device flow, GitHub CLI import | Yes | Yes | Yes |
| GitLab | MVP | PAT | Yes | Yes | Yes |
| Bitbucket | Planned | Not implemented | Planned | Planned | Planned |

GitLab MVP targets the production-critical profile workflow first: PAT login, token verification, Git credential resolution, status reporting, and SSH/GPG key upload.

Provider-neutral workflows are available through `gcm connect <profile> --provider <id>` and `gcm switch-provider <profile> <id>`. Source-aware authentication inspection is available through `gcm auth status`, `gcm auth inspect`, `gcm auth adopt`, `gcm auth logout`, `gcm auth doctor`, and `gcm auth repair`. Provider-specific commands such as `gcm github login` and `gcm gitlab login` remain available for users who prefer explicit provider namespaces.

---

## Configuration

Provider definitions live under `providers` in `~/.gcm/config.yaml`.

```yaml
providers:
  github:
    type: github
    api_url: https://api.github.com
    web_url: https://github.com
    git_hosts:
      - github.com
    ssh_host: github.com
    upload_keys: true
    auth:
      default_method: pat
      scopes:
        - repo
        - admin:public_key
        - admin:gpg_key

  gitlab:
    type: gitlab
    api_url: https://gitlab.com/api/v4
    web_url: https://gitlab.com
    git_hosts:
      - gitlab.com
    ssh_host: gitlab.com
    upload_keys: true
    auth:
      default_method: pat
      scopes:
        - api
        - read_user
        - read_repository
        - write_repository
```

For self-managed GitLab, configure all three URL concepts explicitly:

```yaml
providers:
  gitlab:
    type: gitlab
    api_url: https://gitlab.company.example/api/v4
    web_url: https://gitlab.company.example
    git_hosts:
      - gitlab.company.example
    ssh_host: gitlab.company.example
    ssh_port: 22
```

`api_url` is used for REST calls. `web_url` is shown to users and used for web links. `git_hosts` is used by the credential helper to decide whether GCM should answer a Git credential request.

---

## Profile Accounts

Profiles keep provider-specific account metadata under `providers`. A profile must contain at most one provider entry.

Changing the provider for an existing profile is a destructive transition. GCM asks for confirmation, then removes old provider tokens, cached git credentials, credential username settings, and uploaded SSH/GPG keys when the old token can still access them. Local SSH key filenames are migrated to the new provider suffix so local and remote names stay consistent.

```yaml
name: work-github
git:
  user:
    name: Jane Doe
    email: jane@company.example

providers:
  github:
    username: jane-gh
    auth_method: pat
    upload_keys: true
```

Use a separate profile for GitLab:

```yaml
name: work-gitlab
git:
  user:
    name: Jane Doe
    email: jane@company.example

providers:
  gitlab:
    username: jane-gl
    auth_method: pat
    upload_keys: true
```

The legacy `github:` profile block is still supported and is mapped into the GitHub provider path where needed.

---

## Token Storage

Provider-aware token keys include profile, provider, host, and optional account. This prevents provider tokens from colliding across separate provider-scoped profiles and hosts.

The structured token payload can hold PATs today and OAuth refresh-token metadata later:

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "token_type": "bearer",
  "auth_method": "oauth_device",
  "scopes": ["api"],
  "expires_at": "2026-05-24T12:00:00Z"
}
```

Backward compatibility: existing GitHub tokens stored under the old profile-only key still load through the GitHub provider path. `gcm repair --fix` can migrate those legacy entries into provider-aware storage after saving a compatible provider token.

---

## Auth Ownership

GCM separates authentication state from credential ownership:

| Ownership | Meaning |
| --------- | ------- |
| `gcm` | Token is stored in GCM's provider-aware token store and is managed by GCM |
| `external` | Git can authenticate through another helper such as Keychain, Git Credential Manager, GitHub CLI, libsecret, cache, or store |
| `mixed` | GCM and external credentials are both available |
| `unknown` | No verified owner could be determined |

`gcm auth status` and provider-specific status commands report state, owner, source, username, and findings. `gcm auth inspect <profile>` shows the helper chain and source-specific details without adopting or deleting credentials.

Adoption is explicit. `gcm auth adopt <profile> --provider <id>` verifies an exportable external credential against the provider API before saving it into GCM storage. Logout is scoped: `gcm auth logout <profile>` removes only GCM-owned credentials by default; external deletion requires `--scope external` or `--scope all` and confirmation.

---

## Credential Helper Flow

1. Git invokes `gcm credential-helper get` with `protocol`, `host`, and optional `path`.
2. GCM resolves `host` through the provider registry.
3. GCM reads the active profile.
4. GCM verifies that the resolved provider matches the active profile's selected provider.
5. GCM loads the provider-aware token for profile/provider/host.
6. GCM returns provider-specific Git credentials.

Provider username strategy:

| Provider | PAT Username | OAuth Username |
| -------- | ------------ | -------------- |
| GitHub | configured GitHub username, fallback profile name | configured GitHub username |
| GitLab | configured GitLab username, fallback `oauth2` | `oauth2` |
| Bitbucket planned | account/email depending on auth method | `x-token-auth` |

---

## Implementation Notes

Provider-neutral token storage lives in `internal/tokenstore`. GitHub, GitLab, and future providers use the same storage backend and provider-aware token key format.

Provider account invariants live in `internal/profile`, because the profile schema owns the one-provider-per-profile rule. CLI commands should call those profile/provider helpers instead of duplicating provider metadata logic.
