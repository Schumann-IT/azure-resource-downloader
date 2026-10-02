---
title: Development workflow protocol: start gate, branch gate, plan archive
project: web
status: done
started: 2026-09-29
finished: 2026-09-29
branch: chore/claude-setup
pr: 30
changelog: 0.4.0
---
### Development workflow protocol: start gate, branch gate, plan archive

**Goal.** Make this backlog the only way work enters the codebase, and make the tooling say so. Every change
starts as a committed entry here, is implemented from it on a branch after a start gate has confirmed the entry
is in `HEAD`, is struck out and recorded in `CHANGELOG.md` as it lands, and when done is **archived** — moved
with its full plan into `../.claude/archive/web/`, so the *how* survives for later review where the changelog
keeps only the what and why. The branch gate then checks all of it: not on `main`, the backlog touched, every
entry archived as done recorded under `[Unreleased]`.

> **Decisions.** Archived entries live forever, one file per entry at
> `../.claude/archive/web/<finished-date>-<slug>.md`, with frontmatter `title`, `project`, `status: done|dropped`,
> `started`, `finished`, `branch` and `changelog: Unreleased` until the release stamps the version (`none` for a
> dropped entry). Nothing under that directory is loaded automatically; it is read on request. The changelog
> entry is still written at strikeout time — "item N is done" only archives and asks for follow-ups. An
> abandoned entry is archived as dropped with a one-line reason. `todo.md` at the repository root is removed and
> its findings folded into the two backlogs. GitHub branch protection with a CI status check is a later entry.
>
> **Why moving instead of an in-file archive.** Because "done" moves the entry out of this file, the backlog never
> holds strikeouts at branch close, so the existing no-strikeouts, contiguous-numbering and version-untouched
> checks stay exactly as they are; only new checks are added. The same checks, with the same wording, exist in
> `../go/` (Node here, bash there), read-only, and skip with a note outside a git clone.
>
> **Bootstrap exception.** The start gate cannot gate its own creation; this entry is implemented without it.
>
> **Non-negotiables untouched.** Tooling under `scripts/`, specs and documentation only; the app itself does not
> change.

**Plan.**

- ✅ `scripts/lib/changelog.js`: string-based readers (`parseChangelog`, `entrySection`, `entryTitle`,
  `entryTitleStruck`, `entryGoal`, `entryPlan`, `planOpenItems`, `parseFrontmatter`, `struckLines`,
  `entryNumbers`); the path-based readers become wrappers over them.
- ✅ `scripts/lib/git.js`: the one place this project's tooling shells out (`git`, `isRepo`, `currentBranch`,
  `mergeBase`, `dirtyFiles`, `showFile`) carrying the single `sonarjs/no-os-command-from-path` directive;
  `working-tree-clean.js` becomes a thin CLI over it and its `eslint-suppressions.json` entry is pruned.
- ✅ `scripts/start-item.js` behind `npm run start-item -- <n>`: the same checks and wording as the Go gate (usage
  exit 2; not on `RELEASE_BRANCH`; `web/` clean; entry `N` in **`HEAD`'s** `NEXT-ITERATIONS.md` with a
  `**Plan.**` block and an unstruck bullet; prints title, Goal and Plan); exports
  `startItem({ root, n, releaseBranch, out, err })` for the spec.
- ✅ `scripts/lib/branch.js` (`readBranchFacts`) and the extended `branch-ready.js`: the same checks and wording as
  the Go gate — not on `RELEASE_BRANCH`, `web/` changed since the merge-base, `NEXT-ITERATIONS.md` changed,
  archive files added under `../.claude/archive/web/` carry `status: done|dropped` and a `done` one means
  `[Unreleased]` grew. Reword check 1's remedy from "delete" to "archive".
- ✅ `release-ready.js`: archive files still saying `changelog: Unreleased` listed in the awaiting-release path
  (report only); the root `scripts/release.sh` stamps them.
- ✅ Specs: `test/readiness-readers.spec.ts` for the string readers; new `test/readiness-git.spec.ts` with a
  `git init` temp repository (skipped without git) for `startItem` and `readBranchFacts`.
- ✅ `package.json` script; `README.md` (conventions, scripts, layout); `CHANGELOG.md` under `[Unreleased]`,
  stating in bold that a branch changing `web/` without touching its backlog now fails `npm run branch-ready`;
  `.windsurf/rules/02`, `06` and their `.claude` twins.
