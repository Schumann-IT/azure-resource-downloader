---
title: Development workflow protocol: start gate, branch gate, plan archive
project: go
status: done
started: 2026-09-29
finished: 2026-09-29
branch: chore/claude-setup
changelog: Unreleased
---
## Development workflow protocol: start gate, branch gate, plan archive

**Goal.** Make this backlog the only way work enters the codebase, and make the tooling say so. Every change
starts as a committed entry here, is implemented from it on a branch after a start gate has confirmed the entry
is in `HEAD`, is struck out and recorded in `CHANGELOG.md` as it lands, and when done is **archived** — moved
with its full plan into `../.claude/archive/go/`, so the *how* survives for later review where the changelog
keeps only the what and why. The branch gate then checks all of it: not on `main`, the backlog touched, every
entry archived as done recorded under `[Unreleased]`.

> **Decisions.** Archived entries live forever, one file per entry at
> `../.claude/archive/go/<finished-date>-<slug>.md`, with frontmatter `title`, `project`, `status: done|dropped`,
> `started`, `finished`, `branch` and `changelog: Unreleased` until the release stamps the version (`none` for a
> dropped entry). Nothing under that directory is loaded automatically; it is read on request. The changelog
> entry is still written at strikeout time — "item N is done" only archives and asks for follow-ups. An
> abandoned entry is archived as dropped with a one-line reason. `todo.md` at the repository root is removed and
> its findings folded into the two backlogs. GitHub branch protection with a CI status check is a later entry.
>
> **Why moving instead of an in-file archive.** Because "done" moves the entry out of this file, the backlog never
> holds strikeouts at branch close, so the existing no-strikeouts and contiguous-numbering checks stay exactly as
> they are; only new checks are added. Archived entries lose their number — numbering is presentational. The same
> checks, with the same wording, exist in `../web/` (bash here, Node there), read-only, and skip with a note
> outside a git clone.
>
> **Bootstrap exception.** The start gate cannot gate its own creation; this entry is implemented without it.
>
> **Not regeneration-gated.** Tooling, rules and documentation only; no template changes, no hash moves.

**Plan.**

- ✅ Readers in `scripts/lib/changelog.sh` take their content from stdin (`*_in` variants; the existing names
  become wrappers) so `git show HEAD:…` and `git show <base>:…` can be read without temp files; add
  `entry_section_in N` (fence-aware, `## N.`/`### N.`, stops at the next heading of the same or higher level),
  `entry_title_in`, `entry_title_struck_in`, `entry_goal_in`, `entry_plan_in`, `plan_open_items_in`,
  `unreleased_items_in` and `frontmatter_value_in KEY`; BSD awk and bash 3.2 safe.
- ✅ `scripts/start-item.sh` behind `make start-item N=<n>`: usage (exit 2) unless `N` is a positive integer; not
  on `RELEASE_BRANCH` and not detached; `go/` clean (reuses `working-tree-clean.sh`); entry `N` present in
  **`HEAD`'s** `NEXT-ITERATIONS.md` with a `**Plan.**` block and at least one unstruck bullet; prints title, Goal
  and Plan; exit 1 on any refusal; outside a clone it reads the working copy and says so.
- ✅ Extend `scripts/branch-ready.sh` with read-only git checks, skipped with a note outside a clone: not on
  `RELEASE_BRANCH`; base = merge-base with it (or `origin/…`); when `go/` is unchanged since the base, skip the
  rest and say so; `NEXT-ITERATIONS.md` must have changed since the base; every archive file added under
  `../.claude/archive/go/` carries `status: done|dropped`, and if any is `done` the `[Unreleased]` line count
  must have grown since the base. Reword check 1's remedy from "delete" to "archive".
- ✅ `scripts/release-ready.sh`: in the awaiting-release path, list archive files still saying
  `changelog: Unreleased` (report only); the root `scripts/release.sh` stamps them to the version in the
  release commit.
- ✅ `scripts/lib/changelog_test.sh` over heredoc fixtures, behind `make test-scripts`, part of `make check`.
- ✅ `Makefile`: `start-item`, `test-scripts`, `check`, `help`, and the comments claiming the gate runs a single
  git command.
- ✅ Documentation: `README.md` Development section (the protocol, `start-item`, the extended gate, the archive,
  the git the gate now runs); `CHANGELOG.md` under `[Unreleased]`, stating in bold that a branch changing `go/`
  without touching its backlog now fails `make branch-ready`; `.windsurf/rules/02`, `03`, `06` and their
  `.claude` twins.
