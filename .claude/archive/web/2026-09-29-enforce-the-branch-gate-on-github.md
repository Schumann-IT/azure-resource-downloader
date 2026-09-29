---
title: Enforce the branch gate on GitHub
project: web
status: done
started: 2026-09-29
finished: 2026-09-29
branch: feat/github-branch-gate
pr: 31
changelog: Unreleased
---
## Enforce the branch gate on GitHub

*Partially delivered on this branch: the workflow job and the documentation shipped; the runner verification
and the branch-protection setting stay in the backlog until the pull request has run and the setting is made.*

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

- ✅ Add a job `web` with `name: branch-ready-web` to `.github/workflows/branch-ready.yml` once the Go entry has
  created it. It uses the same workflow and the same `pull_request` trigger on `main`, with no `paths:`
  filter. The job runs `actions/checkout` with `ref: ${{ github.head_ref }}` and `fetch-depth: 0`,
  `actions/setup-node` with `node-version-file: web/package.json`, `cache: npm` and
  `cache-dependency-path: web/package-lock.json`, and `npm ci` with `working-directory: web`. Its last step
  runs `make branch-ready-web` from the repository root with `RELEASE_BRANCH: ${{ github.base_ref }}` in the
  step's `env`.
- ✅ `CHANGELOG.md` under `[Unreleased]`; `README.md` Development conventions naming the `branch-ready-web` status
  check.
- ↪ Confirm in the runner that the gate needs no script change. `npm ci` must write only the gitignored
  `node_modules/`. `public/app.css` is gitignored and built after the clean-tree preflight.
  `test/readiness-git.spec.ts` sets its own git identity and `-b main`, so it runs on a bare runner. The
  branch check must read the pull request's branch name, and the merge-base must resolve through the
  `origin/$RELEASE_BRANCH` fallback in `scripts/lib/git.js`. Verify this on this branch's own pull request,
  where `branch-ready-web` must report and pass. Only if one of these fails, fix `scripts/lib/git.js` or
  `scripts/lib/branch.js` and cover the fix in `test/readiness-git.spec.ts`.
  *(still open in the backlog as entry 1)*
- ↪ Branch protection on `main` is one repository setting shared with the Go entry, done by hand: require
  `branch-ready-web` beside `branch-ready-go`. Leave it unstruck until the user confirms.
  *(still open in the backlog as entry 1)*
