# Git Config Manager - Makefile

BINARY_NAME=gcm
MODULE=git-config-manager
MAIN_PACKAGE=./cmd/gcm
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE?=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GOVERSION?=$(shell go version | awk '{print $$3}')
LDFLAGS=-s -w \
	-X $(MODULE)/pkg/version.Version=$(VERSION) \
	-X $(MODULE)/pkg/version.Commit=$(COMMIT) \
	-X $(MODULE)/pkg/version.Date=$(DATE)
BUILD_FLAGS=-trimpath -ldflags "$(LDFLAGS)"

# All supported platforms (GOOS/GOARCH)
PLATFORMS = \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	linux/arm \
	linux/386 \
	linux/mips64 \
	linux/mips64le \
	linux/ppc64le \
	linux/s390x \
	linux/riscv64 \
	windows/amd64 \
	windows/arm64 \
	windows/386 \
	freebsd/amd64 \
	freebsd/arm64 \
	openbsd/amd64 \
	openbsd/arm64 \
	netbsd/amd64 \
	netbsd/arm64 \
	dragonfly/amd64 \
	solaris/amd64

.PHONY: build build-all build-parallel test lint fmt verify clean install install-system release help

## Build

build: ## Build for current platform
	@echo "Building $(BINARY_NAME) $(VERSION) ($(GOVERSION))..."
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) $(MAIN_PACKAGE)

build-all: ## Cross-compile for all platforms (sequential)
	@echo "Building $(BINARY_NAME) $(VERSION) for $(words $(PLATFORMS)) platforms..."
	@mkdir -p bin
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		output="bin/$(BINARY_NAME)-$${GOOS}-$${GOARCH}"; \
		if [ "$${GOOS}" = "windows" ]; then output="$${output}.exe"; fi; \
		echo "  → $${GOOS}/$${GOARCH}"; \
		CGO_ENABLED=0 GOOS=$${GOOS} GOARCH=$${GOARCH} go build $(BUILD_FLAGS) -o $${output} $(MAIN_PACKAGE) || exit 1; \
	done
	@echo "Done. $(words $(PLATFORMS)) binaries in bin/"

build-parallel: ## Cross-compile for all platforms (parallel)
	@echo "Building $(BINARY_NAME) $(VERSION) for $(words $(PLATFORMS)) platforms (parallel)..."
	@mkdir -p bin
	@failed=0; \
	pids=""; \
	for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		output="bin/$(BINARY_NAME)-$${GOOS}-$${GOARCH}"; \
		if [ "$${GOOS}" = "windows" ]; then output="$${output}.exe"; fi; \
		(CGO_ENABLED=0 GOOS=$${GOOS} GOARCH=$${GOARCH} go build $(BUILD_FLAGS) -o $${output} $(MAIN_PACKAGE) && \
			echo "  ✓ $${GOOS}/$${GOARCH}" || \
			(echo "  ✗ $${GOOS}/$${GOARCH}" && exit 1)) & \
		pids="$${pids} $$!"; \
	done; \
	for pid in $${pids}; do \
		wait $${pid} || failed=$$((failed + 1)); \
	done; \
	if [ $${failed} -gt 0 ]; then \
		echo "FAILED: $${failed} platform(s) failed"; exit 1; \
	fi
	@echo "Done. $(words $(PLATFORMS)) binaries in bin/"

build-checksums: build-all ## Generate SHA256 checksums for all binaries
	@echo "Generating checksums..."
	@cd bin && shasum -a 256 $(BINARY_NAME)-* > checksums.txt
	@echo "Checksums written to bin/checksums.txt"

## Test

test: ## Run tests
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

test-verbose: ## Run tests with verbose output
	go test -race -v -coverprofile=coverage.out ./...

test-coverage: test ## Show coverage report in browser
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: ## Run benchmarks
	go test -bench=. -benchmem ./...

## Lint & Format

lint: ## Run linters
	@echo "Running linters..."
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

fmt: ## Format code
	go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not installed, skipping"; \
	fi

verify: fmt lint test ## Format, lint, and test everything

## Install

GOBIN_DIR ?= $(shell go env GOBIN)
ifeq ($(strip $(GOBIN_DIR)),)
GOBIN_DIR := $(shell go env GOPATH)/bin
endif

install: build ## Install to GOBIN (falls back to $(go env GOPATH)/bin)
	@mkdir -p "$(GOBIN_DIR)"
	@echo "Installing $(BINARY_NAME) -> $(GOBIN_DIR)/$(BINARY_NAME)"
	cp bin/$(BINARY_NAME) "$(GOBIN_DIR)/$(BINARY_NAME)"
	@echo "Done. Make sure $(GOBIN_DIR) is on your PATH."

install-system: build ## Install to /usr/local/bin (may require sudo)
	@echo "Installing $(BINARY_NAME) -> /usr/local/bin/$(BINARY_NAME)"
	install -m 0755 bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

## Release

release: ## Create release with goreleaser
	goreleaser release --clean

release-snapshot: ## Create snapshot release (no publish)
	goreleaser release --snapshot --clean

## Clean

clean: ## Clean build artifacts
	rm -rf bin/ dist/ coverage.out coverage.html

## Development

dev: build ## Build and run
	./bin/$(BINARY_NAME)

run: ## Run without building
	go run $(MAIN_PACKAGE) $(ARGS)

## Dependencies

deps: ## Download dependencies
	go mod download
	go mod tidy

## Info

platforms: ## List all supported build platforms
	@echo "Supported platforms ($(words $(PLATFORMS))):"
	@for platform in $(PLATFORMS); do echo "  $${platform}"; done

version-info: ## Show build version info
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Date:       $(DATE)"
	@echo "Go:         $(GOVERSION)"
	@echo "Module:     $(MODULE)"
	@echo "Platforms:  $(words $(PLATFORMS))"

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
