# Next iterations — deliberately out of scope

Planned work and deliberate scope cuts for the Go CLI. Sections are numbered so the changelog can point at
them (`See NEXT-ITERATIONS.md §1`). `README.md` stays the single source of truth for what the tool *does
today*; this file is only for what was left out on purpose.

> The `§4` reference in the `[RC2]` changelog entry points at the previous edition of this file (retired in
> `b9cc39b`) and is left as-is: released changelog sections are history, and that work — the forward
> re-splice and the `assignmentsSha256` column — has since shipped.

## 1. Section contract for the documentation prompts

**Goal.** Make the generated Markdown structurally predictable enough that the docs browser in `../web` can
style each part of a document, without asking the LLM to hand-write layout.

> **Shipped.** The per-document section contract landed in full — all seven templates declare their H2 list
> closed and verbatim and carry a `<!-- doc-headings: … -->` marker that rides into every type's
> `doc-prompt.md`; the three `&` headings were renamed; the setting `<details>` blocks gained `data-setting` /
> `data-note`; `generate_prompt_template.md` §4 enforces it with a *Heading vocabulary* check that reads each
> family's set from its `doc-prompt.md` and exempts H2s inside `<!-- …:start -->`/`<!-- …:end -->` marker
> pairs; and `docs/summary.md` got the same closed-set declaration for its four H2s. See the `[Unreleased]`
> entry in `CHANGELOG.md`.
>
> **What remains** is confined to the tenant summary — three follow-ups the web side found while reviewing a
> regenerated `summary.md` (§1e-i to §1e-iii), plus an enforcement check for that file. A one-shot
> regeneration of all existing documentation is still required and has not been run.

### 1e-i. Declare `summary.md`'s H3 sub-vocabulary

The **Management summary** section carries `### Findings` and `### Recommendations` sub-headings that §7 of
`internal/docs/generate_prompt_template.md` never declares, and the two reference exports disagree on them.
Give the summary's H3s the same closed-set-and-verbatim treatment the H2s got, so the web side can style and
deep-link them.

### 1e-ii. Give summary findings a severity

Summary findings currently carry no severity, so the landing page cannot rank or filter them. Declare a
small, closed severity vocabulary for the **Findings** list in §7. This is the tenant-summary counterpart to
— and distinct from — the per-document `Security` severity that stays out of scope (below): the summary makes
at most six findings once per tenant, not a subjective call 400+ times across an export.

### 1e-iii. Declare the summary preamble

The content above the first H2 (the posture/intro paragraph) is undeclared, so its shape is not guaranteed.
State what the preamble must contain, the way the per-type templates fix their metadata table and summary
paragraph.

### 1e enforcement

The three declarations above are only worth as much as a check. `summary.md` sits at the `docs/` root, which
the §4 checker skips, and has no `doc-prompt.md` to hold a `doc-headings` marker — so its contract cannot be
validated the way per-type documents are. Add an explicit `summary.md` check (its closed H2 set, the H3
sub-vocabulary from 1e-i, and the preamble from 1e-iii) to §4 or alongside it.

### Deliberately still out

Per-finding severity tags in the per-document `Security` sections (`**[risk]**` / `**[review]**` /
`**[ok]**`) are genuinely model-only information, but they are a subjective call made 400+ times with nothing
able to validate them — distinct from the once-per-tenant summary findings in §1e-ii. Section-level styling
already gets most of the visual benefit; revisit once the vocabulary has proven stable.

### Consumer

The web-side renderer that depends on this is written up in `../web/NEXT-ITERATIONS.md` ("Section-level
styling hooks"). Those changes become *reliable* only once the summary vocabulary above is closed and the
documentation is regenerated.
