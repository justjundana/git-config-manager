# Contributing to GCM

Thank you for your interest in contributing to Git Config Manager!

## Development Setup

1. **Prerequisites**: Go 1.26+, Git, Make
2. **Clone**: `git clone https://github.com/justjundana/git-config-manager.git`
3. **Build**: `make build`
4. **Test**: `make test`

## Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Commit with conventional commits: `feat: add new feature`
7. Push and create a Pull Request

## Code Style

- Follow Go best practices and `gofmt` formatting
- Write godoc comments for all exported functions
- Use table-driven tests
- Target >80% test coverage for new code
- Use structured logging (`logger.Info(msg, logger.F(key, val))`)
- Handle errors with context: `fmt.Errorf("doing thing: %w", err)`

## Project Structure

```
cmd/gcm/        → Entry point
internal/cli/   → Cobra commands
internal/*/     → Domain packages (profile, ssh, gpg, etc.)
pkg/*/          → Shared utilities (logger, ui, version)
```

## Commit Convention

- `feat:` — New feature
- `fix:` — Bug fix
- `docs:` — Documentation
- `test:` — Tests
- `refactor:` — Code refactoring
- `chore:` — Build/CI changes

## Reporting Issues

Include:
1. GCM version (`gcm version`)
2. OS and shell (`gcm doctor`)
3. Steps to reproduce
4. Expected vs actual behavior
