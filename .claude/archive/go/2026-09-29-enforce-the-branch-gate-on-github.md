---
title: Enforce the branch gate on GitHub
project: go
status: done
started: 2026-09-29
finished: 2026-09-30
branch: feat/github-branch-gate
pr: 31
changelog: Unreleased
---
## Enforce the branch gate on GitHub

*Closed in two steps: the workflow and its documentation on the first close, the runner verification and the
branch-protection setting once pull request #31 had run and the setting was made.*

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

- ✅ Confirm in the runner that the gate needs no script change. The clean-tree preflight must see a fresh
  `go/` (`make ci` writes only the gitignored `azure-rd`, after the preflight). The branch check must read
  the pull request's branch name. The merge-base must resolve through the existing `origin/$RELEASE_BRANCH`
  fallback, and `git describe --match 'go/v*'` must find the tags. Verify this on this branch's own pull
  request, where `branch-ready-go` must report and pass. Only if one of these fails, fix
  `scripts/branch-ready.sh` and cover the fix in `scripts/lib/changelog_test.sh`.
- ✅ Enable branch protection on `main` requiring the `branch-ready-go` and `branch-ready-web` status checks and
  requiring branches to be up to date before merging (the jobs gate the pull request's head, not the merge
  result). This is a repository setting done by hand; leave the bullet unstruck until the user confirms it.
- ✅ Run the `go` job only when the pull request touches something its gate judges. A first job `changes`
  (checkout with full history, `git diff --name-only origin/<base>...HEAD`) exposes the outputs `go` and
  `web`; the `go` job takes `needs: changes` and `if: needs.changes.outputs.go == 'true'`, where `go` is true
  for any change under `go/`, `.claude/archive/go/`, the root `Makefile` (it defines the gate targets) or the
  workflow file itself. Job-level conditions, not a `paths:` filter on the trigger: a skipped job counts as
  passed for a required status check and so keeps reporting. Sources alone would be too narrow — the gate
  also checks the backlog, the changelog, the archive and the commit subjects — so every file under `go/`
  counts. Cover the rule with a comment in the workflow naming both lists.
