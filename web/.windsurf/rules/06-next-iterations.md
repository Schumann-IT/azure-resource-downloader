---
trigger: glob
description: How to structure and manage entries in NEXT-ITERATIONS.md
globs: NEXT-ITERATIONS.md
---

# Managing NEXT-ITERATIONS.md

`NEXT-ITERATIONS.md` (repo root) tracks **only outstanding work and parked ideas**. `README.md` is the single
source of truth for what the tool does today; `CHANGELOG.md` is the historical record of what shipped. Nothing
that has already shipped belongs in this file.

This is a sanctioned Markdown file (alongside `README.md` and `CHANGELOG.md`) and is exempt from the "no
separate Markdown files" documentation rule.

## Entry anatomy

Each numbered entry is a `## N. Title` section with these parts, in this order:

- **Title** — name the substantive change or objective in sentence case. Do **not** put a common
  consequence in the title (e.g. that the change requires a documentation regeneration) — that belongs in
  the Goal or Plan.
- **Goal** (required, exactly one) — a `**Goal.**` bolded lead paragraph stating the objective in
  user/intent terms, not implementation.
- **Notes** (optional) — a single blockquote placed directly after the Goal; **every line starts with `>`**.
  Holds rationale, scope, caveats, hash impact, and cross-references. Separate labelled notes with a bare `>`
  line between them.
- **Plan** (required) — a `**Plan.**` block: a bulleted list of concrete, implementable work items. If there
  is no outstanding work, the entry does not belong here (see Lifecycle).

Ideas do **not** live inside an entry — see Parked ideas.

## Lifecycle

- **Remove once shipped.** When an entry's Plan is delivered in full, delete the entry. Its history is the
  `CHANGELOG.md` entry that recorded the work; do not leave "done" entries behind.
- **Numbering is presentational.** Renumber the remaining entries to stay contiguous (`1..N`) after a
  removal. Because numbers shift, do **not** rely on `See NEXT-ITERATIONS.md §N` as a stable anchor from
  other files — describe the work instead. Stale `§N` references already in released `CHANGELOG.md` sections
  are history: leave them as-is.
- **Keep entries self-contained.** An entry must not depend on another entry it might outlive. Restate what
  it needs rather than pointing at a sibling `§N`.

## Parked ideas

- A trailing `## Parked ideas` area collects ideas that are deliberately not scheduled, kept here rather than
  in a work entry so they survive as the entries around them ship and are removed.
- Each idea is a `### Idea: <title>` subsection stating what it is, **why it is parked**, and the explicit
  **revisit conditions** that would make it worth doing.
- **Promotion.** When a parked idea is picked up, **move it into a new numbered work entry and refine it**:
  rewrite it as a proper entry (`**Goal.**`, optional Notes, `**Plan.**` with concrete work items),
  reconciling its rationale and revisit conditions against what is true now, and delete the `### Idea` block.
  A promotion is a review, not a mechanical copy.

## On any edit

- Reflect any user-visible effect in `CHANGELOG.md` per the Changelog Policy — adding, refining, or removing
  an entry that corresponds to real work is itself a change worth recording when it ships.
