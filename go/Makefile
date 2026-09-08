.PHONY: build install clean test test-race test-coverage run help lint lint-check fmt fmt-check deps check ci loc release-ready branch-ready

# Binary name
BINARY_NAME=azure-rd

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod

# Version stamped into the binary so `--version` and export metadata track the
# build. Releases of this project are tagged `go/vX.Y.Z` in the monorepo, so the
# tag lookup is restricted with --match: an unfiltered `git describe` would pick
# up the sibling `web/` project's newest tag and stamp it into every
# resources/metadata.yaml. The `go/` prefix is then stripped so `--version`
# reports a plain vX.Y.Z. Falls back to the short commit (with a -dirty suffix),
# then "dev". Override with `make build VERSION=v1.2.3`.
GIT_DESCRIBE := $(shell git describe --tags --match 'go/v*' --always --dirty 2>/dev/null)
VERSION ?= $(if $(GIT_DESCRIBE),$(patsubst go/%,%,$(GIT_DESCRIBE)),dev)
LDFLAGS := -X azure-resource-downloader/cmd.version=$(VERSION)

# Build the application
build:
	@echo "🔨 Building $(BINARY_NAME) ($(VERSION))..."
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "✅ Build complete: ./$(BINARY_NAME)"

# Install the application globally
install:
	@echo "📦 Installing $(BINARY_NAME) ($(VERSION))..."
	@$(GOCMD) install -ldflags "$(LDFLAGS)" .
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

clean-output:
	@echo "🧹 Cleaning output..."
	@rm -rf output
	@echo "✅ Clean complete"

# Clean build artifacts
clean-bin:
	@echo "🧹 Cleaning..."
	@rm -f $(BINARY_NAME)
	@echo "✅ Clean complete"

clean: clean-bin clean-output

# Run tests
test:
	@echo "🧪 Running tests..."
	@$(GOTEST) -v ./...

# Run tests with the race detector
test-race:
	@echo "🏁 Running tests with race detector..."
	@$(GOTEST) -race ./...

# Run tests with coverage
test-coverage:
	@echo "🧪 Running tests with coverage..."
	@$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "📥 Downloading dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "✅ Dependencies updated"

# Run the application with example args
run:
	@echo "🚀 Running $(BINARY_NAME)..."
	@$(GOCMD) run . --help

# Format code
fmt:
	@echo "🎨 Formatting code..."
	@$(GOFMT) ./...
	@echo "✅ Code formatted"

# Report unformatted files without rewriting them. `check` and `ci` depend on
# this (and on `lint-check`) rather than on `fmt`/`lint` so they never modify
# the working tree: a report on a commit must describe that commit, not a
# version the pipeline has just rewritten.
fmt-check:
	@echo "🎨 Checking formatting..."
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "❌ Unformatted files — run 'make fmt' and commit the result:" >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi
	@echo "✅ Formatting OK"

# Lint and auto-fix what the linters can fix (requires golangci-lint). Rewrites
# files; remaining findings are reported and fail the target. The config is
# verified first so a typo in .golangci.yml fails loudly instead of silently
# reverting to golangci-lint's defaults — which would lint with a different set
# than the editor does.
lint:
	@echo "🔍 Linting code (with --fix)..."
	@golangci-lint config verify
	@golangci-lint run --fix ./...
	@echo "✅ Linting complete"

# Report lint findings without fixing them. The read-only counterpart of `lint`,
# for the same reason `fmt-check` exists beside `fmt`.
lint-check:
	@echo "🔍 Checking lint..."
	@golangci-lint config verify
	@golangci-lint run ./...
	@echo "✅ Lint OK"

# Count lines of code (requires cloc)
loc:
	@echo "📊 Counting lines of code..."
	@if ! command -v cloc >/dev/null 2>&1; then \
		echo "⚠️  cloc not found. Please install it:"; \
		echo "   macOS:   brew install cloc"; \
		echo "   Linux:   sudo apt-get install cloc  (or yum install cloc)"; \
		exit 1; \
	fi
	@cloc .
	@echo "✅ LOC analysis complete"

# Release readiness: report whether a release can be cut (no struck-out
# NEXT-ITERATIONS entries, CHANGELOG.md closed with an empty [Unreleased] and an
# undated `## [X.Y.Z]` heading). Every check is reported; fails only if all
# fail. Changes nothing and runs no git — branch/working-tree checks, date
# stamping, tagging and publishing happen at the repository root
# (`make release`), which runs this first.
release-ready: ci
	@scripts/release-ready.sh

# Is a feature or fix branch ready to ship? Refuses while go/ has uncommitted
# changes (so the verdict describes the commit that will be merged), runs `ci`,
# then reports: no struck-out NEXT-ITERATIONS entries left, entries numbered
# 1..N, work recorded under [Unreleased]. Edits nothing, but unlike
# `release-ready` it exits non-zero if ANY check failed, so it can gate a merge.
# Not a release target and not a prerequisite of one. The preflight must run
# before `ci`; `ci` itself is read-only (fmt-check/lint-check), so the tree the
# report sees is the tree the preflight approved.
branch-ready:
	@scripts/working-tree-clean.sh
	@$(MAKE) --no-print-directory ci
	@scripts/branch-ready.sh

# Check code quality (fmt-check + lint-check + test). Read-only: nothing here
# rewrites a file, so `check`, `ci` and `release-ready` leave the working tree
# as found.
check: fmt-check lint-check test
	@echo "✅ All checks passed"

# Run all quality checks and build (useful for CI/CD)
ci: check build
	@echo "✅ CI pipeline complete"

# Display help
help:
	@echo "Azure Resource Downloader - Makefile commands:"
	@echo ""
	@echo "Primary targets:"
	@echo "  make build          - Build the binary"
	@echo "  make install        - Install globally to GOPATH/bin"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make test-race      - Run tests with the race detector"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make deps           - Download and tidy dependencies"
	@echo "  make run            - Run the application"
	@echo "  make fmt            - Format code (rewrites files)"
	@echo "  make fmt-check      - Report unformatted files without rewriting them"
	@echo "  make lint           - Verify .golangci.yml, then lint and auto-fix (rewrites files; requires golangci-lint)"
	@echo "  make lint-check     - Verify .golangci.yml, then report lint findings without fixing them"
	@echo "  make loc            - Count lines of code (requires cloc)"
	@echo "  make release-ready  - Report whether a release can be cut (tag/publish from the repo root)"
	@echo "  make branch-ready   - Gate: is this feature/fix branch ready to ship? (clean tree, ci, changelog written, strikeouts cleared)"
	@echo ""
	@echo "Composite targets:"
	@echo "  make check          - Run fmt-check + lint-check + test (modifies nothing)"
	@echo "  make ci             - Run check + build (for CI/CD)"
	@echo ""
	@echo "  make help           - Display this help message"
	@echo ""
	@echo "⚠️  Always use 'make' targets instead of running commands directly!"
	@echo "    ✅ make lint    (not: golangci-lint run --fix)"
	@echo "    ✅ make test    (not: go test)"
	@echo "    ✅ make build   (not: go build)"

# Default target
.DEFAULT_GOAL := ci

