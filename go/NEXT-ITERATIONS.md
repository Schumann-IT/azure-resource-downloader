# Next iterations — deliberately out of scope

Planned work, deliberate scope cuts, and parked ideas for the Go CLI. Sections are numbered so the changelog
can point at them (`See NEXT-ITERATIONS.md §1`). `README.md` stays the single source of truth for what the
tool *does today*; this file is only for what was left out on purpose.

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
> **The tenant summary is done too.** Its four H2s, the `### Findings` / `### Recommendations` H3
> sub-vocabulary, the severity-sorted findings table with its closed `Severity` column, and the fixed
> `# Tenant summary` preamble are all declared (§1e-i to §1e-iii) **and** validated by an explicit
> `docs/summary.md` check in section 7 of `generate_prompt_template.md` (§1e enforcement) — it runs after the
> file is written, since the section-4 sweep runs earlier and skips the `docs/` root.
>
> **What remains:** a one-shot regeneration of all existing documentation, which has not been run. The parked
> idea below is deliberately not scheduled.

### Idea (parked): per-finding severity in document `Security` sections

Tag every individual security callout *inside each resource's document* with `**[risk]**` / `**[review]**` /
`**[ok]**`, so the web side can colour or filter them. **Not planned — parked deliberately**, for three
reasons:

- **It is a subjective, model-only judgement.** Nothing in the export can compute or validate whether a given
  setting is risk / review / ok.
- **It is made 400+ times** (once per callout across every document), so a bad or inconsistent batch is
  likely — and the only fix is regenerating everything.
- **Low marginal benefit.** Section-level styling (the `Security` H2 slug) already gives the frontend most of
  the visual win without the per-item risk.

This is the opposite trade-off from the summary findings severity (§1e-ii), which is decided once per tenant
on at most six findings — tiny blast radius, easy to eyeball — and was therefore done.

**Revisit only if both hold:** (1) the closed-heading contract has proven stable across a real regeneration,
with no drift observed in practice; and (2) the web side needs per-item severity that section-level styling
cannot deliver. If promoted, treat it as its own one-shot: extend the `Security:` instruction across all
seven templates and regenerate every document — and accept that it cannot be automatically validated.

### Consumer

The web-side renderer that depends on this is written up in `../web/NEXT-ITERATIONS.md` ("Section-level
styling hooks"). Those changes become *reliable* only once the summary vocabulary above is closed and the
documentation is regenerated.
