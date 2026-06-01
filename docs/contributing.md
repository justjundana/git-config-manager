# Contributing

Thank you for your interest in contributing to GCM! This guide covers everything from reporting bugs to submitting pull requests.

---

## Code of Conduct

Be respectful, constructive, and inclusive. We follow the [Contributor Covenant](https://www.contributor-covenant.org/).

---

## How to Contribute

### Reporting Bugs

1. Check [existing issues](https://github.com/justjundana/git-config-manager/issues) first
2. Open a new issue with:
   - Output of `gcm version` and `gcm doctor`
   - Steps to reproduce
   - Expected vs. actual behavior
   - Your OS and shell

### Requesting Features

Open an issue with the `enhancement` label. Describe:
- The problem you're solving
- Your proposed solution
- Any alternatives you considered

### Improving Documentation

Documentation lives in `docs/`. See [Project Structure](project-structure.md) for the layout. All docs are Markdown — fix typos, add examples, or improve explanations by submitting a PR.

---

## Development Setup

### Prerequisites

| Tool            | Version  | Notes                        |
| --------------- | -------- | ---------------------------- |
| Go              | 1.26+    | Required                     |
| Git             | 2.20+    | Required                     |
| Make            |          | For build targets            |
| golangci-lint   | latest   | Optional, for linting        |

### Clone and Build

```bash
git clone https://github.com/justjundana/git-config-manager.git
cd git-config-manager
make build        # produces ./bin/gcm
```

### Run Tests

```bash
make test                     # all tests with race detector + coverage
make test-verbose             # verbose output
go test ./internal/profile/   # single package
go test -run TestCreate ./internal/profile/   # single test
```

### Lint

```bash
make lint          # go vet + golangci-lint (if installed)
make fmt           # gofmt + goimports
```

### Install Locally

```bash
make install           # to $(go env GOPATH)/bin/gcm
make install-system    # to /usr/local/bin/gcm (needs sudo)
```

---

## Making Changes

### Workflow

1. Fork the repo and create a feature branch:
   ```bash
   git checkout -b feature/my-change
   ```
2. Make your changes
3. Write or update tests
4. Run the full test suite:
   ```bash
   make test
   ```
5. Ensure linting passes:
   ```bash
   make lint
   ```
6. Commit with a clear message
7. Push and open a pull request

### Coding Standards

- **Go style** — Follow standard Go conventions (`gofmt`, `goimports`)
- **Error handling** — Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- **Naming** — Use Go conventions: `NewManager`, `ProfileError`, `AskConfirm`
- **Comments** — Package-level comments on every package; exported functions documented
- **No globals** — Use dependency injection via the `Container`
- **Test hooks** — For OS/terminal/network dependencies, use function variables (see [Architecture](architecture.md#function-variable-hooks-for-testability))

### Commit Messages

Use conventional commit style:

```
feat: add profile rename command
fix: handle empty SSH key path in validation
docs: add CI/CD examples
test: improve crypto service coverage
refactor: simplify profile switcher logic
```

### Testing Guidelines

- **90%+ coverage** is enforced for all packages
- Use **table-driven tests** with `t.Run()`
- Mock OS dependencies with **function variable hooks** (not interfaces)
- Use `t.TempDir()` for filesystem tests
- Use `t.Setenv()` for environment variable tests
- Use `httptest.Server` for HTTP tests
- No external test frameworks — pure `testing` package

### Adding a New Command

1. Create the command function in the appropriate `internal/cli/*.go` file
2. Register it in `root.go` or the parent command
3. Add the domain logic in the appropriate `internal/*/` package
4. Wire any new services in `internal/container/container.go`
5. Write tests for both the domain logic and CLI behavior
6. Document the command in `docs/commands.md`
7. Add examples to `docs/examples.md`

### Adding a New Package

1. Create the package under `internal/` (private) or `pkg/` (public)
2. Add the `// Package xyz ...` doc comment
3. Create `*_test.go` alongside the code
4. Wire into the container if needed
5. Document in `docs/project-structure.md`

---

## Pull Request Process

1. **Title** — Clear, concise description of the change
2. **Description** — What changed and why; link to the issue if applicable
3. **Tests** — All tests pass, new code has tests
4. **Lint** — `make lint` passes cleanly
5. **Docs** — Updated if the change affects user-facing behavior
6. **Single concern** — One PR per feature/fix

### Review

- PRs require at least one approval
- Address review comments with additional commits (don't force-push during review)
- Squash on merge

---

## Makefile Targets

```bash
make build              # Build for current platform
make build-all          # Cross-compile for all platforms
make test               # Run tests with race detector + coverage
make test-verbose       # Verbose test output
make test-coverage      # Generate HTML coverage report
make bench              # Run benchmarks
make lint               # Run go vet + golangci-lint
make fmt                # Format code
make install            # Install to GOPATH/bin
make install-system     # Install to /usr/local/bin
make release            # Create release with goreleaser
make release-snapshot   # Snapshot release (no publish)
make clean              # Remove build artifacts
make help               # Show all targets
```

---

## Project Layout

See [Project Structure](project-structure.md) for a complete map. Key directories:

```
cmd/gcm/          Entry point
internal/cli/     Cobra commands (thin layer)
internal/*/       Domain packages (logic lives here)
internal/service/ Infrastructure (crypto, file I/O)
pkg/              Shared utilities (ui, logger, version)
docs/             Documentation
```

---

## Release Process

1. Update `CHANGELOG.md`
2. Tag the release:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```
3. Build release binaries:
   ```bash
   make release
   # or: make build-all
   ```
4. Version info is embedded via ldflags automatically

---

## Getting Help

- **Issues:** https://github.com/justjundana/git-config-manager/issues
- **Discussions:** Open a discussion on the repo
- **Code:** See [Architecture](architecture.md) and [Project Structure](project-structure.md)
- **Docs:** See [index.md](index.md) for the full documentation map

---

## License

GCM is licensed under the [MIT License](../LICENSE). By contributing, you agree that your contributions will be licensed under the same license.
