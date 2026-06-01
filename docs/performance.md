# Performance

This document describes GCM's performance characteristics and optimization strategies.

---

## Design Philosophy

GCM is a CLI tool that runs **synchronously** and **exits immediately**. There are no daemons, background services, or long-running processes (except the GitHub device flow polling, which has user-visible progress). Performance is designed for instant feedback.

---

## Startup Time

GCM has near-instant startup:

| Stage | Time | Notes |
| ----- | ---- | ----- |
| Binary load | ~5ms | Static Go binary, no runtime to initialize |
| Cobra command parse | ~1ms | No reflection, pre-registered command tree |
| Config load | ~2ms | Single YAML file read + unmarshal |
| **Total cold start** | **~8-10ms** | Negligible for interactive CLI use |

### Why It's Fast

- **Static binary** — no interpreter, no JIT, no dynamic linking
- **Lazy initialization** — services are created in the container but only execute work when called
- **No network on startup** — GCM never phones home, checks for updates, or contacts any server unless you explicitly run a `github` subcommand
- **Minimal file I/O** — only `config.yaml` is read on startup; profiles are loaded on demand

---

## Command Performance

Typical execution times for common operations:

| Command | Time | Bottleneck |
| ------- | ---- | ---------- |
| `gcm profile list` | ~10ms | Directory listing + YAML reads |
| `gcm profile create` | ~5ms | YAML write |
| `gcm use <profile>` | ~15ms | YAML read + git config writes |
| `gcm current` | ~5ms | Config read |
| `gcm ssh generate` (Ed25519) | ~10ms | Key generation + file write |
| `gcm ssh generate` (RSA 4096) | ~50-200ms | RSA key generation (CPU-bound) |
| `gcm gpg generate` | ~1-5s | GPG subprocess (external tool) |
| `gcm github login` | ~2-5s | Token input + API verification |
| `gcm github login-oauth` | 10-60s | Waiting for user to authorize in browser |
| `gcm backup create` | ~20-50ms | tar.gz compression of config files |
| `gcm backup restore` | ~10-30ms | tar.gz extraction |
| `gcm doctor` | ~30ms | Multiple system checks |
| `gcm init` | ~10ms | Shell config file append |

### Key Observations

- **Profile operations** are dominated by filesystem I/O (YAML read/write)
- **SSH Ed25519** key generation is nearly instant (deterministic algorithm)
- **SSH RSA** key generation varies with key size; 4096-bit can take 200ms+
- **GPG operations** depend on the external `gpg` binary and are the slowest
- **GitHub operations** are network-bound (device flow polling every 5 seconds)

---

## Shell Integration Performance

The shell hook runs on every `cd` command. Its impact must be negligible:

```
cd (no .gcm-profile)  → ~1ms  (file existence check only)
cd (with .gcm-profile) → ~15ms (file read + profile activation)
```

### How It Stays Fast

1. **File existence check first** — if `.gcm-profile` doesn't exist, the hook exits immediately
2. **Same-profile short-circuit** — if the profile is already active, no work is done
3. **No subprocess for check** — the shell reads `.gcm-profile` with a built-in (`cat` or shell read)
4. **GCM runs only when switching** — `gcm refresh` is only called when a change is detected

---

## Memory Usage

GCM is lightweight:

| Scenario | RSS |
| -------- | --- |
| Simple command (`gcm current`) | ~8-12 MB |
| Profile activation | ~10-15 MB |
| SSH key generation (Ed25519) | ~10-15 MB |
| SSH key generation (RSA 4096) | ~15-20 MB |
| Backup with many profiles | ~15-25 MB |

These numbers are typical for Go binaries, which include the garbage collector and runtime.

---

## Disk Usage

GCM's disk footprint is minimal:

| Item | Typical Size |
| ---- | ------------ |
| `gcm` binary | ~12-15 MB |
| `config.yaml` | ~500 bytes |
| Profile YAML (each) | ~300-500 bytes |
| Template YAML (each) | ~200-400 bytes |
| Encrypted token (each) | ~200-300 bytes |
| Backup archive | ~2-10 KB |
| Audit log (per day) | ~1-20 KB |

**Total typical footprint**: ~15-20 MB (binary + data for ~5 profiles).

---

## Scaling Characteristics

| Dimension | Behavior | Limit |
| --------- | -------- | ----- |
| Number of profiles | Linear scan on `profile list` | Hundreds are fine |
| Number of templates | Linear scan on `template list` | Hundreds are fine |
| Number of backups | Linear scan on `backup list`, `backup prune` | Hundreds are fine |
| Audit log size | Append-only, one file per day | Rotates daily, no cleanup needed |
| Config file size | Single YAML, loaded once | Always small |

GCM is designed for individual developer use. Even power users rarely have more than 10-20 profiles. The linear scaling of list operations is not a concern.

---

## Optimization Tips

### 1. Use Ed25519 Keys

Ed25519 key generation is ~10-40× faster than RSA 4096:

```bash
# Fast (recommended)
gcm ssh generate work -t ed25519

# Slower
gcm ssh generate work -t rsa -b 4096
```

### 2. Prefer Keychain Over Encrypted File

OS keychain access is typically faster than Argon2id key derivation:

```yaml
# config.yaml
github:
  use_keychain: true   # faster
  encrypt_tokens: false
```

### 3. Keep Audit Logs Manageable

Audit logs rotate daily. If disk space is a concern, periodically clean old logs:

```bash
# Remove logs older than 90 days
find ~/.gcm/logs -name "*.jsonl" -mtime +90 -delete
```

### 4. Prune Backups Regularly

```bash
gcm backup prune --keep 5
```

### 5. Use `--short` for Scripting

Short-form output skips formatting overhead:

```bash
gcm current --short  # just the profile name
```

---

## Benchmarking

You can measure any GCM command with standard tools:

```bash
# macOS/Linux
time gcm profile list
time gcm use work --global

# Detailed profiling (Go)
go test -bench=. -benchmem ./internal/...
```

---

## Comparison with Manual Approach

| Task | Manual | With GCM |
| ---- | ------ | -------- |
| Switch Git identity | ~30s (3+ git config commands) | ~0.5s (`gcm use work`) |
| Generate SSH key | ~10s (ssh-keygen + config edit) | ~2s (`gcm ssh generate`) |
| Set up new machine | ~10-30 min | ~2 min (restore backup + re-auth) |
| Wrong-identity commit | Happens regularly | Never (auto-switch) |

---

## See Also

- [Architecture](architecture.md) — design decisions that affect performance
- [Shell Integration](shell-integration.md) — hook implementation details
- [Configuration](configuration.md) — settings that affect behavior
