---
title: Enforce the branch gate on GitHub
project: web
status: done
started: 2026-09-29
finished: 2026-09-30
branch: feat/github-branch-gate
pr: 31
changelog: Unreleased
---
### 1. Enforce the branch gate on GitHub

*Closed in two steps: the workflow job and its documentation on the first close, the runner verification and
the branch-protection setting once pull request #31 had run and the setting was made.*

**Goal.** Make the branch gate unbypassable: a pull request into `main` cannot be merged until
`make branch-ready-web` has passed on it. Everything local — the start gate, `branch-ready`, the archive
checks — is a report a `git commit` by hand can walk past; branch protection with a required status check is
the only layer that actually gates.

> **Scope.** The web job of a root `.github/workflows/branch-ready.yml` (the Go half is the mirror entry in
> `../go/NEXT-ITERATIONS.md`; one workflow file carries both jobs, and the Go entry creates it), and branch
> protection on `main` requiring it. The workflow runs on every pull request into `main` without a `paths:`
> filter — a required check that never reports blocks the merge for good, and the gate already skips its
> diff checks when `web/` is unchanged. Fork pull requests are out of scope (single-maintainer repository).
> The app does not change; no non-negotiable is touched.

**Plan.**

- ✅ Confirm in the runner that the gate needs no script change. `npm ci` must write only the gitignored
  `node_modules/`. `public/app.css` is gitignored and built after the clean-tree preflight.
  `test/readiness-git.spec.ts` sets its own git identity and `-b main`, so it runs on a bare runner. The
  branch check must read the pull request's branch name, and the merge-base must resolve through the
  `origin/$RELEASE_BRANCH` fallback in `scripts/lib/git.js`. Verify this on this branch's own pull request,
  where `branch-ready-web` must report and pass. Only if one of these fails, fix `scripts/lib/git.js` or
  `scripts/lib/branch.js` and cover the fix in `test/readiness-git.spec.ts`.
- ✅ Branch protection on `main` is one repository setting shared with the Go entry, done by hand: require
  `branch-ready-web` beside `branch-ready-go`. Leave it unstruck until the user confirms.
- ✅ Run the `web` job only when the pull request touches something its gate judges: the `web` job takes
  `needs: changes` (the `changes` job the Go entry adds) and `if: needs.changes.outputs.web == 'true'`, where
  `web` is true for any change under `web/`, `.claude/archive/web/`, the root `Makefile` or the workflow file
  itself. Job-level conditions, not a `paths:` filter on the trigger: a skipped job counts as passed for a
  required status check and so keeps reporting. Every file under `web/` counts, because the gate also checks
  the backlog, the changelog, the archive and the commit subjects, not only sources, templates and the lock
  file.
