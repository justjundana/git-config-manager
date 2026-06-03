# Installation

## Requirements

| Tool    | Required | Notes                                 |
| ------- | -------- | ------------------------------------- |
| **Go**  | Yes      | 1.26+ (only for building from source) |
| **Git** | Yes      | 2.20+ recommended                     |
| OpenSSH | Yes      | `ssh`, `ssh-add` on `PATH`            |
| GPG     | Optional | Only needed for commit signing        |

## From Source (recommended)

```bash
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager
make build          # produces ./bin/gcm
make install        # installs to $(go env GOPATH)/bin/gcm (no sudo needed)
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` (typically `~/go/bin`).

To install system-wide instead:

```bash
make install-system   # installs to /usr/local/bin/gcm (needs sudo)
```

## Via `go install`

```bash
go install github.com/justjundana/git-config-manager/cmd/gcm@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Prebuilt Binaries

Release binaries are published by GoReleaser for each supported platform. The installer downloads `checksums.txt`, verifies the binary with SHA-256, and does not modify your shell environment unless you pass an opt-in flag:

```bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash -s -- --add-to-path
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash -s -- --add-to-path --init
```

On Windows, use PowerShell:

```powershell
iwr https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1
.\install.ps1 -AddToPath
```

To install manually, download both the binary and `checksums.txt` from the same GitHub release, verify the checksum, and move the binary onto your `PATH`:

```bash
shasum -a 256 -c checksums.txt --ignore-missing
sudo mv gcm-<os>-<arch> /usr/local/bin/gcm
chmod +x /usr/local/bin/gcm
```

## Cross-Compilation

Build for all supported platforms at once:

```bash
make build-all
```

This produces binaries in `./bin/` for:
- `darwin/amd64`, `darwin/arm64` (macOS)
- `linux/amd64`, `linux/arm64`, `linux/arm`, `linux/386`, `linux/mips64`, `linux/mips64le`, `linux/ppc64le`, `linux/s390x`, `linux/riscv64`
- `windows/amd64`, `windows/arm64`, `windows/386`
- `freebsd/amd64`, `freebsd/arm64`, `openbsd/amd64`, `openbsd/arm64`, `netbsd/amd64`, `netbsd/arm64`, `dragonfly/amd64`, `solaris/amd64`

## Verify Installation

```bash
gcm version     # show version, commit, build date
gcm doctor      # check Git, SSH, GPG, and shell integration
```

## Keeping Up to Date

Update GCM with a single command:

```bash
gcm update              # download and install the latest release
gcm update --check      # check for updates without installing
```

Or re-run the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/justjundana/git-config-manager/main/scripts/install.sh | bash
```

See [Upgrade & Uninstall](upgrade-uninstall.md) for all upgrade methods.

## Next Steps

After installing, continue with:

1. [Quick Start](quick-start.md) — up and running in 5 minutes
2. [Getting Started](getting-started.md) — create your first profile
3. [Shell Integration](shell-integration.md) — set up auto-switching on `cd`
4. [Commands Reference](commands.md) — explore every command and flag
