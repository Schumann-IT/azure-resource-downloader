---
title: Split CI into pipeline checks and the close report
project: web
status: done
started: 2026-09-30
finished: 2026-09-30
branch: feat/github-branch-gate
pr: 31
changelog: 0.4.0
---
### 2. Split CI into pipeline checks and the close report

**Goal.** Make GitHub Actions the quality authority and keep the local cycle fast. Tests, lint and build run
on every push so a red result arrives while the work is still in hand, and the branch report — strikeouts,
numbering, changelog, archive, commit subjects, version untouched — runs on the pull request in seconds as
the merge gate. Locally, implementation runs build and tests before anything is committed, the review and
fix agents no longer run lint themselves, and closing a branch waits for CI instead of repeating the
pipeline.

> **Why.** `branch-ready` bundles two things with different rhythms. The pipeline belongs on every push;
> the close report is only meaningful at the end and is *expected* to be red while entries are still struck.
> Splitting them keeps every check reporting (skipped jobs count as passed), keeps the merge gate strict, and
> lets the local tooling skip what CI already proves. `npm run branch-ready` with the full pipeline stays for
> offline use. The app does not change; no non-negotiable is touched.

**Plan.**

- ✅ `package.json`: a `branch-ready:report` script running only `scripts/working-tree-clean.js` and
  `scripts/branch-ready.js` (no tests, lint or build). It needs Node but no `npm ci`: the scripts use
  built-ins only.
- ✅ `.github/workflows/branch-ready.yml`: job `ci-web` (`name: ci-web`, on push and pull request, `if` web
  changed): checkout by name with full history, Node from `web/package.json`, `npm ci`, then `npm test`,
  `npm run lint` and `npm run build` in `web/`. Job `branch-ready-web` (pull requests only, `if` web changed):
  checkout and Node only, then `npm --prefix web run branch-ready:report` with
  `RELEASE_BRANCH: ${{ github.base_ref }}`. The `changes` job and the header are the Go entry's.
- ✅ Verify on this branch's pull request: `ci-web` green on push and on the pull request; `branch-ready-web` red
  on the strikeout check only while entries are struck, green after the close.
- ✅ Branch protection on `main` requires `ci-web` and `branch-ready-web` beside the Go checks (by hand). Leave
  unstruck until the user confirms.
- ✅ `CHANGELOG.md` under `[Unreleased]` (amending the entry about the GitHub gate); `README.md` Development
  conventions naming `npm run branch-ready:report` and the two CI jobs.
