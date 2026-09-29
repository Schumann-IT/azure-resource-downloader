---
paths:
  - "go/NEXT-ITERATIONS.md"
  - "web/NEXT-ITERATIONS.md"
---

# Managing `NEXT-ITERATIONS.md` (both projects)

Each project's `NEXT-ITERATIONS.md` tracks **outstanding work and parked ideas**, plus the entries that
shipped on the current branch and are struck through until the branch is closed. `README.md` says what the
tool does today; `CHANGELOG.md` records what shipped. It is a sanctioned Markdown file.

## Entry anatomy
Each numbered work entry is a `## N. Title` section (`web/` nests them under `## Features` / `## Fixes`):
- **Title** — the substantive change in sentence case; common consequences (e.g. "requires regeneration")
  belong in the Goal or Plan, not the title.
- **Goal** (required, exactly one) — a `**Goal.**` paragraph in user/intent terms.
- **Notes** (optional) — one blockquote directly after the Goal, every line starting with `>`; rationale,
  scope, caveats, hash impact, cross-references; labelled notes separated by a bare `>` line.
- **Plan** (required) — a `**Plan.**` bulleted list of concrete work items. No outstanding work → the entry
  does not belong here.

Ideas never live inside an entry. `web/` also keeps a `## Standing decisions` section for decisions that
constrain ideas without being work items.

## Lifecycle
- **Strike out what ships; do not delete it.** Wrap delivered plan items in `~~…~~`, and the title once the
  whole Plan is delivered. Writing the `CHANGELOG.md` entry is part of the same edit.
- **Deleting is part of closing the branch, not of implementing.** Only then are struck entries removed and
  the rest renumbered `1..N` — the `/close-branch` skill. `branch-ready` fails while any strikeout is left or
  numbering has a gap; `release-ready` repeats the strikeout check as a backstop.
- **A partially delivered entry keeps its unstruck items** and survives with them.
- **Numbering is presentational**; never cite `§N` from other files — describe the work. Stale `§N`
  references in released changelog sections are history.
- **Entries are self-contained**: restate what an entry needs rather than pointing at a sibling.

## Parked ideas
- A trailing `## Parked ideas` area; each idea is `### Idea: <title>` stating what it is, **why it is parked**
  and the explicit **revisit conditions**.
- **Promotion** moves an idea into a new numbered entry and refines it (Goal, Notes, Plan) against what is
  true now, deleting the `### Idea` block. A promotion is a review, not a copy.

## Batching regeneration-gated work (Go)
A change is regeneration-gated when it edits `go/internal/models/documentation_prompt.tmpl` or any per-type
`*_prompt.tmpl`, or otherwise moves a type's `promptSha256`. Such work **must be batched**: when planning or
promoting one, survey the other entries and parked ideas for regeneration-gated ones and remind the user so
they share one regeneration. State the coupling in the entry's Notes. Changes riding the non-hashed
`docs/generate.md` or `drift/analyze.md` templates are not gated and carry no reminder.

## On any edit
Reflect any user-visible effect in `CHANGELOG.md`; adding, refining or removing an entry that corresponds to
real work is itself worth recording when it ships.
