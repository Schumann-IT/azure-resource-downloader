---
title: Count an archived entry as a backlog change in the branch gate
project: go
status: done
started: 2026-09-30
finished: 2026-10-01
branch: fix/branch-gate-archived-entry
changelog: Unreleased
---
## Count an archived entry as a backlog change in the branch gate

**Goal.** `make branch-ready-go` must accept a branch that planned an entry and archived it again: today it
fails with "NEXT-ITERATIONS.md is unchanged on this branch" whenever every entry the branch touched was both
added and archived on it, because the backlog file then ends up identical to `main`. A branch that archived
an entry has visibly delivered its backlog, so that must count.

> **Why.** Check 5 of `scripts/branch-ready.sh` compares `NEXT-ITERATIONS.md` between the merge-base and
> `HEAD` (`git diff --quiet "$base" HEAD -- "$next"`), so only the net difference counts. The rule it guards —
> every branch that changes `go/` delivers, refines or adds an entry — is still met when the entry is added
> and archived on the same branch; the archive file under `.claude/archive/go/` is the evidence. First seen on
> `feat/drift-attribution` on the web side, where every entry was planned and closed on the branch; the go
> gate has the same logic and passed there only because its entry already existed on `main`.
>
> **Contract.** Both branch gates apply the same backlog rule, each to its own project: the check passes
> when `NEXT-ITERATIONS.md` differs between the merge-base and `HEAD`, **or** at least one `.md` file was
> added on the branch under `.claude/archive/go/` (`--diff-filter=A`, pathspec `:(top).claude/archive/go`),
> whatever its `status` (`done` and `dropped` both deliver the backlog; a missing status is already failed
> by the archive check). Archive files of the other project never count. The two ok lines are worded
> identically on both sides: `NEXT-ITERATIONS.md changed on this branch` when the file differs (it wins
> when both hold), else `NEXT-ITERATIONS.md delivered on this branch (<n> archived entry(ies))`. The
> failure message stays as it is. Nothing else in the export tree or the other project is affected.
>
> **Owner.** go owns, at *done*, the gate wording outside `go/`: root `CLAUDE.md` ("the backlog changed on
> the branch"), root `README.md` (the branch-gate paragraph: "`NEXT-ITERATIONS.md` changed on the branch")
> and `.claude/rules/next-iterations.md` ("when the backlog did not change on a branch that changed the
> project"). The web entry of the same name owns nothing outside `web/`. No sequencing: the two gates are
> independent scripts.
>
> **Implementer.** sonnet

**Plan.**

- ✅ New `scripts/lib/branch.sh` (sourced by `scripts/branch-ready.sh` next to `lib/changelog.sh`, read-only
  git only, run with the working directory at `go/`): `archived_files <base>` prints the `.md` files added
  under `:(top).claude/archive/go` between `<base>` and `HEAD` (the query check 7 runs today, moved
  verbatim), and `backlog_state <base>` prints `changed` when `NEXT-ITERATIONS.md` differs from `<base>`,
  else `delivered <n>` when `archived_files` lists `n > 0` files, else nothing.
- ✅ Check 5 in `scripts/branch-ready.sh` uses `backlog_state`: `changed` → ok "NEXT-ITERATIONS.md changed on
  this branch"; `delivered <n>` → ok "NEXT-ITERATIONS.md delivered on this branch (<n> archived
  entry(ies))"; empty → the existing failure message, unchanged. Check 7 takes its file list from
  `archived_files`; its behaviour does not change. Update the check-5 comment to say an archived entry
  counts.
- ✅ New `scripts/lib/branch_test.sh` (same `assert_eq` style as `scripts/lib/changelog_test.sh`): builds
  throw-away repositories under `mktemp -d` (removed by a `trap`, never this checkout) laid out as
  `go/NEXT-ITERATIONS.md`, `go/CHANGELOG.md`, `.claude/archive/go/` and `.claude/archive/web/`, with git run
  as `git -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false` and `init -b main`,
  and `cd`s into `go/` before calling the functions. Cases: a branch that changes only another `go/` file →
  empty; an entry added and then removed from the backlog together with a new `.claude/archive/go/*.md`
  (net-zero backlog diff) → `delivered 1`; the backlog edited → `changed`; edited and archived → `changed`;
  a file added only under `.claude/archive/web/` → empty; `archived_files` lists exactly the added go file.
  Skips with a note and exits 0 when `git` is not available.
- ✅ `make test-scripts` runs both `scripts/lib/changelog_test.sh` and `scripts/lib/branch_test.sh`; its
  Makefile comment replaces "Plain bash over heredoc fixtures; no files, no git" with "the changelog
  readers over heredocs, the branch facts against throw-away git repositories in a temp directory".
- ✅ Documentation at *done*: `CHANGELOG.md` gets a `### Fixed` entry (the gate is an operator-facing script,
  and the gate itself refuses an entry archived as done without `[Unreleased]` growing); `README.md`'s
  `make branch-ready` paragraph, root `README.md`, root `CLAUDE.md`, `.claude/rules/next-iterations.md` and
  the Windsurf twins `.windsurf/rules/06-next-iterations.md` and `.windsurf/rules/02-style-and-quality.md`
  describe the check as "the backlog changed or an entry was archived on the branch".
