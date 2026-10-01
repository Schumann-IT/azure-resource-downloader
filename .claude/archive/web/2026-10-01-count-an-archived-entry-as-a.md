---
title: Count an archived entry as a backlog change in the branch gate
project: web
status: done
started: 2026-09-30
finished: 2026-10-01
branch: fix/branch-gate-archived-entry
changelog: Unreleased
---
## Count an archived entry as a backlog change in the branch gate

**Goal.** `npm run branch-ready` (and `make branch-ready-web`) must accept a branch that planned an entry and
archived it again: today it fails with "NEXT-ITERATIONS.md is unchanged on this branch" whenever every entry
the branch touched was both added and archived on it, because the backlog file then ends up identical to
`main`. A branch that archived an entry has visibly delivered its backlog, so that must count.

> **Why.** `scripts/lib/branch.js` sets `backlogChanged` from `git diff --quiet <base> HEAD --
> NEXT-ITERATIONS.md`, so only the net difference counts. The rule it guards — every branch that changes
> `web/` delivers, refines or adds an entry — is still met when the entry is added and archived on the same
> branch; `facts.archived`, computed a few lines further down from the files added under
> `.claude/archive/web/`, is the evidence. First seen on `feat/drift-attribution`, where the drift
> attribution entry and the page-width fix were both planned and closed on the branch.
>
> **Contract.** Both branch gates apply the same backlog rule, each to its own project: the check passes
> when `NEXT-ITERATIONS.md` differs between the merge-base and `HEAD`, **or** at least one `.md` file was
> added on the branch under `.claude/archive/web/` (`--diff-filter=A`, pathspec `:(top).claude/archive/web`
> — exactly what `facts.archived` already holds), whatever its `status` (`done` and `dropped` both deliver
> the backlog; a missing status is already failed by check 8). Archive files of the other project never
> count. The two ok lines are worded identically on both sides: `NEXT-ITERATIONS.md changed on this
> branch` when the file differs (it wins when both hold), else `NEXT-ITERATIONS.md delivered on this branch
> (<n> archived entry(ies))`. The failure message stays as it is. Nothing in the export tree or the other
> project is affected.
>
> **Owner.** none — the root `CLAUDE.md`, root `README.md` and `.claude/rules/next-iterations.md` wording is
> updated at *done* by the go entry of the same name. No sequencing: the two gates are independent scripts.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `scripts/lib/branch.js`: `readBranchFacts` gains `backlogTouched` (initialised `null` like
  `backlogChanged`; when a merge-base exists, set after `facts.archived` is computed to
  `facts.backlogChanged || facts.archived.length > 0`). `backlogChanged` keeps its meaning. Update the
  module comment.
- ✅ `scripts/branch-ready.js` check 6: `backlogChanged` → ok "NEXT-ITERATIONS.md changed on this branch";
  else `backlogTouched` → ok "NEXT-ITERATIONS.md delivered on this branch (<n> archived entry(ies))" with
  `n = facts.archived.length`; else the existing failure message, unchanged. Update the check-6 comment to
  say an archived entry counts.
- ✅ `test/readiness-git.spec.ts` (temp `git init` repositories, never this checkout): the existing
  archive case also asserts `backlogTouched === true`; new cases on fresh branches of the fixture repository —
  a branch that changes only another `web/` file → `backlogChanged` and `backlogTouched` both `false`; an
  entry added and then removed from the backlog together with a new `.claude/archive/web/*.md` (net-zero
  backlog diff) → `backlogChanged === false`, one `archived` file, `backlogTouched === true`; a file added
  only under `.claude/archive/go/` → `backlogTouched === false`. Without a merge-base `backlogTouched` stays
  `null` (extend the existing no-merge-base case).
- ✅ Documentation at *done*: `CHANGELOG.md` gets a `### Fixed` entry (the gate is an operator-facing script,
  and the gate itself refuses an entry archived as done without `[Unreleased]` growing); `README.md`'s
  `npm run branch-ready` paragraph and the Windsurf twins `.windsurf/rules/06-next-iterations.md` and
  `.windsurf/rules/02-style-and-quality.md` describe the check as "the backlog changed or an entry was
  archived on the branch".
