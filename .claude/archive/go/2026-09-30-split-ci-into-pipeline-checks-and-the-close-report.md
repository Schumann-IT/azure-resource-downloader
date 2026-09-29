---
title: Split CI into pipeline checks and the close report
project: go
status: done
started: 2026-09-30
finished: 2026-09-30
branch: feat/github-branch-gate
pr: 31
changelog: Unreleased
---
## Split CI into pipeline checks and the close report

**Goal.** Make GitHub Actions the quality authority and keep the local cycle fast. The pipeline — format,
lint, tests, build — runs on every push so a red result arrives while the work is still in hand, and the
branch report — strikeouts, numbering, changelog, archive, commit subjects — runs on the pull request in
seconds as the merge gate. Locally, implementation runs build and tests before anything is committed, the
review and fix agents no longer run lint themselves, and closing a branch waits for CI instead of repeating
the pipeline.

> **Why.** `branch-ready` bundles two things with different rhythms. The pipeline belongs on every push;
> the close report is only meaningful at the end and is *expected* to be red while entries are still struck.
> Running both as one required check makes the pull request red for most of its life and makes the local
> gate slow. Splitting them keeps every check reporting (skipped jobs count as passed), keeps the merge gate
> strict, and lets the local tooling skip what CI already proves.
>
> **Scope.** The workflow and the Makefile targets. `make branch-ready` with the full pipeline stays for
> offline use. The pipeline agents and the skills change alongside (Claude setup, no entry).
>
> **Not regeneration-gated.**

**Plan.**

- ✅ `Makefile`: a `branch-ready-report` target running only the clean-tree preflight and `scripts/branch-ready.sh`
  (no `ci`), with the `help` line; root `Makefile` gains `branch-ready-report-go`, `-web` and
  `branch-ready-report` beside the full targets, and the comments say which is the local full gate and which
  the CI merge gate.
- ✅ `.github/workflows/branch-ready.yml`: trigger on `push` (every branch) and `pull_request` into `main`. The
  `changes` job diffs against `origin/<base>` where `<base>` is the pull request's base branch or `main` on a
  push. Job `ci-go` (`name: ci-go`, on both events, `if` go changed): checkout by name with full history,
  Go from `go.mod`, golangci-lint v2.11.4 via the action pointed at `go/`, then `make -C go ci` with
  `RELEASE_BRANCH` set. Job `branch-ready-go` (pull requests only, `if` go changed): checkout only, then
  `make branch-ready-report-go` with `RELEASE_BRANCH: ${{ github.base_ref }}`. The header comment states the
  two rhythms and that branch protection requires `ci-go`, `ci-web`, `branch-ready-go`, `branch-ready-web`.
- ✅ Verify on this branch's pull request: `ci-go` green on push and on the pull request; `branch-ready-go` red on
  the strikeout check only while entries are struck, green after the close.
- ✅ Branch protection on `main` requires all four checks (by hand; replaces the two-check setting of the
  previous entry). Leave unstruck until the user confirms.
- ✅ `CHANGELOG.md` under `[Unreleased]` (amending the entry about the GitHub gate); `README.md` Development
  section naming `make branch-ready-report` and the two CI jobs; root `README.md` step 3.
