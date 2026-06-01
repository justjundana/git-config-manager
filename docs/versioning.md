# Versioning

GCM's versioning policy, compatibility guarantees, and support lifecycle.

---

## Semantic Versioning

GCM follows [Semantic Versioning 2.0.0](https://semver.org/) (SemVer):

```
MAJOR.MINOR.PATCH
```

| Component | When Incremented | Example |
|-----------|-----------------|---------|
| **MAJOR** | Breaking changes to CLI flags, config format, or behavior | 1.0.0 → 2.0.0 |
| **MINOR** | New features, new commands, new config fields | 1.0.0 → 1.1.0 |
| **PATCH** | Bug fixes, security patches, documentation fixes | 1.0.0 → 1.0.1 |

Pre-release versions use a suffix: `1.0.0-beta.1`, `1.0.0-rc.1`.

---

## Version Components

```bash
$ gcm version
gcm 1.2.3 (darwin/arm64) built 2026-05-18 commit abc1234 go 1.26.0
```

| Field | Meaning |
|-------|--------|
| `1.2.3` | SemVer version |
| `darwin/arm64` | OS and CPU architecture |
| `built 2026-05-18` | Build date |
| `commit abc1234` | Git commit hash |
| `go 1.26.0` | Go toolchain version used to build |

---

## Compatibility Guarantees

### Within a MAJOR version (e.g., 1.x.x)

| Component | Guarantee |
|-----------|-----------|
| CLI commands and flags | ✅ No removal, no breaking behavior changes |
| `config.yaml` format | ✅ Backwards compatible (new fields have defaults) |
| Profile YAML format | ✅ Backwards compatible (new fields have defaults) |
| Template YAML format | ✅ Backwards compatible |
| Backup archive format | ✅ Older backups restore on newer versions |
| Shell integration hooks | ✅ `gcm init` upgrades hooks safely (idempotent) |
| Exit codes | ✅ Existing codes preserved, new codes may be added |
| Audit log format | ✅ New fields may be added, existing fields stable |

### What May Change in MINOR versions

- New commands and subcommands (additive)
- New flags on existing commands (with defaults preserving old behavior)
- New config fields (with defaults)
- New profile fields (optional, old profiles work unchanged)
- Performance improvements
- Better error messages

### What May Change in MAJOR versions

- Removed commands or flags (deprecated first)
- Changed default behavior
- Config format changes requiring migration
- Profile format changes requiring migration
- Changed exit codes
- Removed features

---

## Deprecation Process

When a feature is scheduled for removal:

1. **Announce** — Document in release notes and CHANGELOG
2. **Warn** — CLI prints a deprecation warning when the feature is used
3. **Minimum grace period** — Feature remains for at least one minor release cycle
4. **Remove** — Feature removed in the next major version

Example:

```
v1.5.0: "gcm foo" deprecated, prints warning: "Use 'gcm bar' instead"
v1.6.0: "gcm foo" still works, still prints warning
v2.0.0: "gcm foo" removed
```

---

## Release Cycle

| Channel | Cadence | Content |
|---------|---------|---------|
| **Patch** | As needed | Bug fixes, security patches |
| **Minor** | Monthly (approximate) | New features |
| **Major** | When necessary | Breaking changes (rare) |

### Support Matrix

| Version | Status | Support |
|---------|--------|---------|
| Current minor (e.g., 1.3.x) | ✅ Active | Bug fixes + security patches |
| Previous minor (e.g., 1.2.x) | ⚠️ Maintenance | Security patches only |
| Older minors | ❌ EOL | No support, upgrade recommended |

---

## Configuration Versioning

### config.yaml

New fields are added with sensible defaults. Your existing `config.yaml` always works:

```yaml
# v1.0 config.yaml — works with v1.5 GCM
default_profile: work
auto_switch: true
```

```yaml
# v1.5 config.yaml — new optional fields
default_profile: work
auto_switch: true
github:                    # new in v1.3
  use_keychain: true
audit:                     # new in v1.4
  enabled: true
```

GCM never removes fields. Unknown fields are ignored.

### Profile YAML

Same approach — new fields are optional with defaults:

```yaml
# v1.0 profile — works forever
name: "Jane Doe"
email: "jane@example.com"
```

### Backup Archives

Newer GCM can restore older backups. Older GCM may not fully understand newer backup contents (new files are ignored during extraction).

---

## Build Version Information

Version is injected at build time via Go `ldflags`:

```makefile
VERSION := $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

go build -ldflags "-X pkg/version.Version=$(VERSION) \
                    -X pkg/version.Commit=$(COMMIT) \
                    -X pkg/version.Date=$(DATE)"
```

Development builds show `dev` as the version:

```bash
$ gcm version
gcm dev (darwin/arm64) built unknown commit unknown go 1.26.0
```

---

## Version Pinning for CI/CD

In CI pipelines, pin to a specific version:

```bash
# Pin to exact version
go install github.com/justjundana/git-config-manager/cmd/gcm@v1.2.3

# Pin to latest patch of a minor version
go install github.com/justjundana/git-config-manager/cmd/gcm@v1.2
```

In Dockerfiles:

```dockerfile
ARG GCM_VERSION=v1.2.3
RUN go install github.com/justjundana/git-config-manager/cmd/gcm@${GCM_VERSION}
```

---

## Checking Your Version

```bash
# Short
gcm version

# Check if update is available (compare with latest release)
gcm version
# Then check: https://github.com/justjundana/git-config-manager/releases
```

---

## Breaking Changes Policy

Breaking changes are categorized as:

| Category | Examples | Required in |
|----------|---------|-------------|
| CLI breaking | Removed command, changed flag meaning | MAJOR only |
| Config breaking | Removed field, changed field type | MAJOR only |
| Behavior breaking | Changed default, different output format | MAJOR only |
| Security breaking | Forced encryption upgrade | MINOR (with migration path) |

Security-related breaking changes (e.g., upgrading encryption) may happen in a minor version if they include an automatic migration path and clear documentation.

---

## Backwards Compatibility

### Guaranteed

- Existing CLI commands and flags work as documented
- Config files from older versions load without errors
- Profiles from older versions activate correctly
- Backups from older versions restore successfully
- Shell hooks installed by older versions work with newer GCM
- Audit logs from older versions are valid

### Not Guaranteed

- Internal API (Go package interfaces) — may change between minor versions
- Debug output format
- Exact error message wording
- Test helper utilities
- Undocumented behavior

---

## See Also

- [Release Notes](release-notes.md) — changelog and release history
- [Upgrade & Uninstall](upgrade-uninstall.md) — how to upgrade GCM
- [Configuration](configuration.md) — config file format
- [Contributing](contributing.md) — development and release process
