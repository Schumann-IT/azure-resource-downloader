---
trigger: glob
description: How to structure and manage entries in NEXT-ITERATIONS.md, and the workflow from entry to archive
globs: NEXT-ITERATIONS.md
---

# Managing `NEXT-ITERATIONS.md` (both projects)

Each project's `NEXT-ITERATIONS.md` is **the only way work enters the codebase**: outstanding work as numbered
entries, parked ideas, and (in `web/`) standing decisions. `README.md` says what the tool does today;
`CHANGELOG.md` records what shipped and why; the archive under `../.claude/archive/go/` keeps how. It is a
sanctioned Markdown file.

## What a request lets you do

| The user says | You may |
|---|---|
| anything that is **not** one of the phrases below (an idea, "add", "refine", "assess", a follow-up) | edit `NEXT-ITERATIONS.md` only — a new parked idea, a refinement, a follow-up bullet. **No code.** A one-line bug fix is a tiny entry (title, one-line goal, one plan bullet), not an exemption. |
| `promote idea <title>` / `plan idea <title>` / `plan item N` | the promotion flow below (`/promote-idea`). Ends with a committed entry, never with code. |
| `implement item N` | the start gate first — `make start-item N=<n>` in `go/`, `npm run start-item -- <n>` in `web/` — and refuse if it fails; then implement the plan bullet by bullet — code and tests, striking each bullet as it lands; **no `README.md` or `CHANGELOG.md`** yet (`/implement-item`, which runs the agent pipeline below for one side; `implement item N inline` does it in the session). When the plan is delivered, **stop**: the user verifies the work by hand and asks for follow-ups. |
| `implement pair go N web M` (or one side) | `/implement-pair`: one Opus plan reviewer reads both entries, writes the `Contract`, `Owner` and `Implementer` notes into them, refines the plans and **commits the backlog files itself** (the one agent commit); a decision that changes what ships is put to the user as a choice, the answer is recorded in the entry and the review restarts, until a pass raises none — a refused answer fails the run. Then, one per side in parallel: implementers (code, tests, build, struck bullets; the session commits, no push), Opus reviewers (the entry as it stands vs the diff and the contract), and Sonnet QA (fixers, remaining lint at the site, tests, build). The session commits the QA changes once every QA agent succeeded, pushes once, and waits for the `ci-*` jobs. **Agents never push or commit** except the plan reviewer's backlog commits. Ends with a report; `item N is done` stays the user's call. This pipeline exists in Claude Code only; elsewhere implement the two entries one after the other. |
| `item N is done` | close the entry (`/item-done`): **now** write the project's `README.md` and `CHANGELOG.md` from the plan, the diff and the implementation reports, then move the entry — or, for a partially delivered entry, its struck bullets — to `../.claude/archive/go/<finished-date>-<slug>.md`, renumber the rest `1..N`, commit all of it together. |
| `drop item N` | archive it with `status: dropped`, `changelog: none` and a one-line reason (`/item-done`). |
| close the branch / release | `/close-branch`, `/release`. |

The start gate cannot gate its own creation; nothing else is exempt.

## Entry anatomy
Each numbered work entry is a `## N. Title` section (`web/` nests them as `### N.` under `## Features` /
`## Fixes`):
- **Title** — the substantive change in sentence case; common consequences (e.g. "requires regeneration")
  belong in the Goal or Plan, not the title.
- **Goal** (required, exactly one) — a `**Goal.**` paragraph in user/intent terms.
- **Notes** (optional) — one blockquote directly after the Goal, every line starting with `>`; rationale,
  scope, caveats, hash impact, cross-references; labelled notes separated by a bare `>` line.
- **Plan** (required) — a `**Plan.**` bulleted list of concrete, implementable work items (tests and
  documentation included). No outstanding work → the entry is done and gets archived.
- The pipeline maintains three labelled notes in the Notes blockquote of an entry it has reviewed:
  `> **Contract.** …` (what the sibling project relies on, in this side's terms), `> **Owner.** …` (files
  outside `go/` and `web/` this side may touch, and sequencing) and `> **Implementer.** sonnet | opus` (the
  tier for the implementing agent). Decisions the user took during the review land as
  `> **Decision.** <question>: <answer>.`

Ideas never live inside an entry. `web/` also keeps a `## Standing decisions` section for decisions that
constrain ideas without being work items.

## Lifecycle
- **Committed before implemented.** The start gate reads entry N from `HEAD`, not from the working copy.
- **Strike out what ships; do not delete it.** Wrap delivered plan items in `~~…~~`, and the title once the
  whole Plan is delivered. Follow-ups are new, unstruck bullets on the same entry, or a new entry.
- **Documentation is written at done, not while implementing.** Implementation produces code, tests and
  struck bullets; `README.md` and `CHANGELOG.md` are written by `item N is done`, after the user has
  verified the work, from the entry (why), the diff (what) and the implementation reports (operator-visible
  effects) — and committed together with the archive, which is what the branch gate checks. Documentation-
  only bullets stay unstruck until then. Within a branch the README may trail the code; `main` never sees it.
- **Done means archived, not deleted.** `item N is done` moves the entry to
  `../.claude/archive/go/<finished-date>-<slug>.md` with a frontmatter — `title`, `project`, `status:
  done|dropped`, `started`, `finished`, `branch`, `changelog: Unreleased` (stamped to the version by the
  release; `none` for dropped) — followed by the entry verbatim with the `~~` removed, delivered bullets
  prefixed `✅` and follow-ups that stayed behind prefixed `↪` naming the entry they moved to. A partially
  delivered entry keeps its unstruck items here and only its struck ones are archived (the archive file
  accumulates across branches). Archived entries lose their number.
- **The gates check it.** `branch-ready` fails while any strikeout is left, when numbering has a gap, when a
  branch that changed the project neither changed the backlog nor archived an entry, and when an entry
  archived as done did not grow `[Unreleased]`; `release-ready` repeats the strikeout check and lists the archive files it will stamp.
- **Numbering is presentational**; never cite `§N` from other files — describe the work. Stale `§N`
  references in released changelog sections are history.
- **Entries are self-contained**: restate what an entry needs rather than pointing at a sibling.
- Nothing under `.claude/archive/` is loaded automatically. Read an archived entry when the user asks
  (`/archive list|show|search`) or when reworking the area it describes.

## Parked ideas
- A trailing `## Parked ideas` area; each idea is `### Idea: <title>` stating what it is, **why it is parked**
  and the explicit **revisit conditions**.
- **Promotion is a review, not a copy.** In Claude Code, `/promote-idea <title>` locates the block, enters
  plan mode seeded with it, and on approval transcribes the plan file into the entry anatomy — Context →
  Goal and Notes (with the reconciled revisit conditions), work steps and verification → Plan bullets —
  as the next number, deletes the `### Idea` block, and commits `chore(<project>): plan <title>`. Elsewhere:
  draft the entry from the idea, review it with the user, commit. Implementation stays a separate
  `implement item N`.

## Batching regeneration-gated work (Go)
A change is regeneration-gated when it edits `go/internal/models/documentation_prompt.tmpl` or any per-type
`*_prompt.tmpl`, or otherwise moves a type's `promptSha256`. Such work **must be batched**: when planning or
promoting one, survey the other entries and parked ideas for regeneration-gated ones and remind the user so
they share one regeneration. State the coupling in the entry's Notes. Changes riding the non-hashed
`docs/generate.md` or `drift/analyze.md` templates are not gated and carry no reminder.

## On any edit
Reflect any user-visible effect in `CHANGELOG.md`; adding, refining or removing an entry that corresponds to
real work is itself worth recording when it ships.
