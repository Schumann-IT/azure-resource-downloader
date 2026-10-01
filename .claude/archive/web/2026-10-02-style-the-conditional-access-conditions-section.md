---
title: Style the Conditional Access `Conditions` section and close two section-style gaps
project: web
status: done
started: 2026-10-01
finished: 2026-10-02
branch: feat/ca-template-and-conditions
pr: 53
changelog: Unreleased
---
## Style the Conditional Access `Conditions` section and close two section-style gaps

*Kind:* feat

**Goal.** When the regenerated documentation arrives, a Conditional Access policy's new `Conditions` section is
styled like every other contract section instead of rendering as plain unstyled prose, and the two known gaps in
the section styling are closed: `membership` is not treated as a `<details>` container, and nested `<details>` in a
`definition` section get the same depth rail as settings and properties.

> **Why.** The go entry *Template content fixes and a Conditional Access template* gives Conditional Access policies
> their own template with a new H2, `Conditions`. The browser styles a section only when its slug is in
> `SECTION_VOCABULARY` (`src/docs/section-hooks.ts`); an unknown H2 renders unstyled. The review of the templates
> against 411 generated documents also found that `src/styles.css` gives `membership` the dense `<details>`
> treatment although the group template's Membership section is prose, and that the nested-`<details>` depth rail
> covers `settings` and `properties` but not `definition`.
>
> **Contract.** The CLI declares each document type's H2 set in its `doc-prompt.md`; the browser keys styling
> on the slug of the heading text. After the go regeneration, a Conditional Access policy document
> (`docs/Microsoft.Graph/conditionalAccessPolicies/*.md`) carries exactly the H2s `References`, `Conditions`,
> `Lifecycle and operations`, `Security`, `Settings`, in that order, and no `<!-- assignments:… -->` markers.
> `Conditions` slugs to `conditions`; it holds targeting tables (`Condition | Include | Exclude`) and prose, never
> `<details data-setting>` blocks, so it gets a section identity (role colour and icon) but not the dense
> settings mode. Every other document type keeps its heading set; `roleScopeTags` and `reusablePolicySettings`
> documents switch to the existing referenced set (`References | Usage and references | Lifecycle and
> operations | Security | Definition`), already styled. Marker names, `data-setting` and `data-note` values are
> unchanged.
>
> **Owner.** none — every file is under `web/`. No sequencing constraint: the change is harmless before the
> regeneration (the slug simply never occurs in today's documents) and ships in one pair with the go entry.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `src/docs/section-hooks.ts`: add `conditions` to `SECTION_VOCABULARY` and a line to the template list in its comment (`conditional-access   references, conditions, lifecycle-and-operations, security, settings`); note that `roleScopeTags` / `reusablePolicySettings` now use the referenced set.
- ✅ `src/styles.css`: `[data-section="conditions"]` joins the *Relations* role (`--section-color: var(--sec-relation)` — targeting is who and what the policy points at), so the dark variant comes from the existing `--sec-relation` lift with no extra rule; add a `--section-icon` for it in the existing Lucide data-URI format (e.g. `funnel`); `conditions` gets no dense rule. Drop `membership` from the three dense rules (the `font-size`, `details` margin and `summary` padding selectors under `.doc-section[data-section=…]`) while keeping its colour and icon; add `.doc-section[data-section="definition"] details details` to the nested-`<details>` depth rail.
- ✅ Tests: `test/section-hooks.spec.ts` — a `## Conditions` heading gets `data-section="conditions"` and the `doc-section-heading` class, and `wrapSections` wraps its run in a `doc-section` with that slug; `test/styles-build.spec.ts` — the built CSS contains the `[data-section="conditions"]` identity rule and the `definition` depth-rail selector, and contains neither a `.doc-section[data-section="membership"]` dense rule nor a `.doc-section[data-section="conditions"]` dense rule.
- ✅ Documentation at *done*: `README.md` section-styling paragraph (the heading sets per template); `CHANGELOG.md`
  `### Changed` under the views area.
