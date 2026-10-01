---
title: Accept a dependency-only branch in the branch gate
project: web
status: done
started: 2026-10-01
finished: 2026-10-01
branch: build/dependency-updates
changelog: Unreleased
---
## Accept a dependency-only branch in the branch gate

**Goal.** A Dependabot npm pull request — or a manual dependency bump — that changes only `web/package.json` and
`web/package-lock.json` passes `branch-ready-web` without a backlog entry, while its tests, lint and build still run
in `ci-web`.

> **Why.** The go entry *Routine dependency updates with a byte-neutrality guard, and Dependabot* adds Dependabot
> for npm; the web gate refuses any branch that touches `web/` without a backlog change (check 6, `backlogTouched`
> in `scripts/lib/branch.js`), so its pull requests could never pass the required check.
>
> **Contract.** The same rule the go gate already enforces for `go.mod` / `go.sum` (`dependency_only` in
> `go/scripts/lib/branch.sh`, check 5 of `go/scripts/branch-ready.sh`): a branch is dependency-only when
> `git diff --name-only --relative <merge-base> HEAD -- .`, run with the working directory at `web/`, is
> non-empty and every line is exactly `package.json` or `package-lock.json` (paths relative to `web/`; a nested
> `package.json` does not count). Paths outside `web/` (`.github/…`, `.claude/archive/…`, `go/…`) count neither
> way. The exemption is the **last** branch of the backlog check: backlog changed → its usual ok line; else an
> entry archived → its usual ok line; else dependency-only → the ok line, byte for byte,
> `dependency-only branch: backlog check not required`; else the usual failure. Nothing else changes —
> strikeouts, numbering, `[Unreleased]`, the `package.json` `version` check, not on `main`, Conventional Commits
> and archived-as-done all run as before, so a bump that also edits `version` still fails. Dependabot's npm
> subjects are `build(web): bump …` (prefix in `.github/dependabot.yml`), which the Conventional Commits check
> accepts as is.
>
> **Sequencing.** `.github/dependabot.yml` (npm included) already exists on this branch, shipped with the go
> entry *Routine dependency updates with a byte-neutrality guard, and Dependabot*; npm Dependabot pull requests
> pass `branch-ready-web` once this entry is on `main` (older ones after `@dependabot rebase`).
>
> Not regeneration-gated.
>
> **Owner.** none — every file is under `web/`; `.github/dependabot.yml` belongs to the go entry and is not
> touched here.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `scripts/lib/branch.js` `readBranchFacts`: a new fact `dependencyOnly` (`null` while `base` is `null`, like
  the other diff facts), read through the existing `git()` wrapper as
  `git(['diff', '--name-only', '--relative', base, 'HEAD', '--', '.'], { cwd: root })` — non-empty output whose
  every non-empty line is `package.json` or `package-lock.json`. Update the file's header comment to name it.
- ✅ `scripts/lib/branch.js`: export a pure `backlogCheck(facts)` returning `{ ok: boolean, message: string }` with
  the four outcomes in order — `NEXT-ITERATIONS.md changed on this branch`; `NEXT-ITERATIONS.md delivered on this
  branch (<n> archived entry(ies))`; `dependency-only branch: backlog check not required`; the existing
  unchanged-backlog failure text — so the spec can pin the shared ok line without running the report.
- ✅ `scripts/branch-ready.js` check 6: call `backlogCheck(facts)` and route to `ok()` / `fail()`; extend the
  check's comment (and the header's "touched the backlog at all") with the dependency-only exception. Checks 1–5,
  7 and 8 are not touched.
- ✅ `test/readiness-git.spec.ts` (the existing temp `git init` fixture; new branches off `main`):
  `package.json` + `package-lock.json` only → `dependencyOnly` true, `backlogTouched` false, and `backlogCheck`
  is ok with exactly `dependency-only branch: backlog check not required`; `package-lock.json` alone → true;
  `package.json` plus `src/x.ts` → false and `backlogCheck` fails with the unchanged-backlog text;
  `package.json` plus `NEXT-ITERATIONS.md` → false, `backlogCheck` ok with the changed line;
  `package-lock.json` plus `.github/dependabot.yml` at the repository root → true; only
  `.github/dependabot.yml` → `folderChanged` false and `dependencyOnly` false; the existing `fix/other-file`
  and `fix/go-archive` branches assert `dependencyOnly` false; the no-merge-base case asserts `dependencyOnly`
  null. Pure `backlogCheck` cases for the changed and delivered lines (facts literals, no git).
- ✅ Documentation at *done*: `CHANGELOG.md` *Release workflow*; `README.md` gate-checks list: the dependency-only
  exception.
