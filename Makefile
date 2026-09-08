.PHONY: help release release-status release-ready-go release-ready-web branch-ready branch-ready-go branch-ready-web

# Release entry points for the monorepo. Closing a project's changelog into an
# undated `## [X.Y.Z]` heading (and, for web/, bumping package.json) is done by
# hand and committed first; the per-project targets below only REPORT whether a
# release can be cut, and `make release` stamps the date, commits, tags and
# publishes every project whose newest heading is still undated. The
# repository-wide branch and working-tree checks live in `make release` only.
# See README.md#development-workflow (step 4, Release).
# The `branch-ready*` targets are the ones here that are not part of releasing:
# they report whether a feature or fix branch is ready to be merged.

# Branch releases are cut from; `make release` refuses on any other branch.
RELEASE_BRANCH ?= main
export RELEASE_BRANCH

# Release readiness of go/ (delegates to go/Makefile). Reports only, changes nothing.
release-ready-go:
	@$(MAKE) -C go release-ready

# Release readiness of web/ (delegates to the npm `release-ready` script). Reports only.
release-ready-web:
	@npm --prefix web run release-ready

# Is a feature or fix branch ready to ship? Each project's gate reports only,
# but unlike the release targets above it exits non-zero if any check failed, so
# it can gate a merge, and it refuses to run while its own folder has
# uncommitted changes. Not release targets and not prerequisites of one.
branch-ready-go:
	@$(MAKE) -C go branch-ready

branch-ready-web:
	@npm --prefix web run branch-ready

# Both gates; a branch touching one project only can run just that one.
branch-ready: branch-ready-go branch-ready-web

# Readiness of both projects (prerequisites), then check branch + working tree,
# stamp the date onto every undated version heading, commit, tag, push and
# create the GitHub releases (needs gh).
release: release-ready-go release-ready-web
	@scripts/release.sh publish

# Readiness of both projects, then show which would be published by
# `make release`; changes nothing.
release-status: release-ready-go release-ready-web
	@scripts/release.sh status

help:
	@echo "Azure Resource Downloader - monorepo targets:"
	@echo ""
	@echo "  make branch-ready-go    - Gate: is a go/ branch ready to ship? (clean tree, ci, strikeouts cleared, changelog written)"
	@echo "  make branch-ready-web   - Gate: is a web/ branch ready to ship? (same, plus package.json version untouched)"
	@echo "  make branch-ready       - Both gates"
	@echo ""
	@echo "  make release-ready-go   - Report whether go/ is ready to release (changelog closed, no strikeouts)"
	@echo "  make release-ready-web  - Report whether web/ is ready to release (same, plus package.json version)"
	@echo "  make release-status     - Run readiness, then show which projects have an undated version heading"
	@echo "  make release            - Run readiness, check branch + tree, then stamp date, commit, tag, push, GitHub release (needs gh)"
	@echo ""
	@echo "  RELEASE_BRANCH=<name>   - Branch releases are cut from (default: main)"
	@echo ""
	@echo "Build, test and lint each project from its own folder: 'make -C go help', 'npm --prefix web run'."

.DEFAULT_GOAL := help
