---
title: Enforce the branch gate on GitHub
project: go
status: done
started: 2026-09-29
finished: 2026-09-29
branch: feat/github-branch-gate
changelog: Unreleased
---
## Enforce the branch gate on GitHub

*Partially delivered on this branch: the workflow and the documentation shipped; the runner verification and
the branch-protection setting stay in the backlog until the pull request has run and the setting is made.*

**Goal.** Make the branch gate unbypassable: a pull request into `main` cannot be merged until
`make branch-ready-go` has passed on it. Everything local — the start gate, `branch-ready`, the archive
checks — is a report a `git commit` by hand can walk past; branch protection with a required status check is
the only layer that actually gates.

> **Scope.** A GitHub Actions workflow at the repository root running `make branch-ready-go` on every pull
> request into `main` (Go per `go.mod`, golangci-lint v2 pinned at the version used locally, the pull
> request's branch checked out by name with full history so `git rev-parse --abbrev-ref HEAD`,
> `git merge-base` and `git describe` work), and branch protection on `main` requiring it. The workflow has
> no `paths:` filter: a required check that never reports blocks the merge for good, and the gate already
> skips its diff checks when `go/` is unchanged; running both pipelines on every pull request costs a few
> minutes and is accepted. Fork pull requests are out of scope (single-maintainer repository). The web half
> is the mirror entry in `../web/NEXT-ITERATIONS.md`; one workflow file carries both jobs, and this entry
> creates it. A `pre-commit` hook refusing commits on `main` is deliberately not part of this: the release
> flow commits on `main`.
>
> **Not regeneration-gated.**

**Plan.**

- ✅ `.github/workflows/branch-ready.yml` (this entry creates it; the web entry adds its job afterwards):
  `name: branch-ready`, trigger `pull_request` with `branches: [main]` and no `paths:` filter,
  `permissions: contents: read`. It has a job `go` with `name: branch-ready-go` on `ubuntu-latest`. That job
  runs `actions/checkout` with `ref: ${{ github.head_ref }}` and `fetch-depth: 0`, `actions/setup-go` with
  `go-version-file: go/go.mod` and `cache-dependency-path: go/go.sum`, and `golangci/golangci-lint-action`
  with `install-only: true` and `version: v2.11.4` (the version used locally, built with Go 1.26.1; if the
  prebuilt binary is older than `go.mod`'s Go, use `install-mode: goinstall`). Its last step runs
  `make branch-ready-go` from the repository root with `RELEASE_BRANCH: ${{ github.base_ref }}` in the step's
  `env`.
- ✅ `CHANGELOG.md` under `[Unreleased]`; root `README.md` Development workflow step 3 naming the `branch-ready`
  workflow, its `branch-ready-go` / `branch-ready-web` status checks and the branch-protection setting.
- ↪ Confirm in the runner that the gate needs no script change. The clean-tree preflight must see a fresh
  `go/` (`make ci` writes only the gitignored `azure-rd`, after the preflight). The branch check must read
  the pull request's branch name. The merge-base must resolve through the existing `origin/$RELEASE_BRANCH`
  fallback, and `git describe --match 'go/v*'` must find the tags. Verify this on this branch's own pull
  request, where `branch-ready-go` must report and pass. Only if one of these fails, fix
  `scripts/branch-ready.sh` and cover the fix in `scripts/lib/changelog_test.sh`.
  *(still open in the backlog as entry 2)*
- ↪ Enable branch protection on `main` requiring the `branch-ready-go` and `branch-ready-web` status checks and
  requiring branches to be up to date before merging (the jobs gate the pull request's head, not the merge
  result). This is a repository setting done by hand; leave the bullet unstruck until the user confirms it.
  *(still open in the backlog as entry 2)*
