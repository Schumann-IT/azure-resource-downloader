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

The seven prompt templates each end with a list of H2 sections the model must produce, in order:

```
internal/models/documentation_prompt.tmpl          References · Lifecycle & operations · Security · Settings
internal/handlers/graph/singleton_prompt.tmpl      References · Lifecycle & operations · Security · Settings
internal/handlers/graph/group_prompt.tmpl          Membership · Usage as assignment target · Security · Properties
internal/handlers/graph/record_prompt.tmpl         References · Lifecycle & operations · Properties
internal/handlers/graph/credential_prompt.tmpl     References · Lifecycle & operations · Expiry & renewal · Security · Properties
internal/handlers/graph/referenced_prompt.tmpl     References · Usage & references · Lifecycle & operations · Security · Definition
internal/handlers/arm/arm_prompt.tmpl              References · Lifecycle & operations · Security · Properties
```

That list is *described* but never *declared binding*, and it has drifted. In the two reference exports:

- **`## Metadata` in 99 documents** — a second, model-invented metadata table, spread across every family
  (conditional access 24, settings-catalog policies 22, groups 20, …), even though the fixed preamble
  already mandates a metadata table above the first H2.
- **`## Assignments` in 4 documents** — duplicating the marked assignments block.

Any drifting heading breaks a frontend that keys off the heading slug, so the contract has to become
explicit before the web side can rely on it.

### 1a. Declare the heading list closed and verbatim

The highest-value change, and the reason for the rest. Append to the section list in **all seven**
templates (diverge one and the vocabulary splits):

```
These H2 headings are a closed set and a machine contract: the documentation browser styles each section
by its heading text. Write them verbatim — exact wording, exact casing, no numbering, no added words — in
the order given, and emit no other H2. Do not introduce `## Metadata`, `## Overview`, `## At a glance`,
`## Assignments` or `## Coverage caveats`: identifying fields belong in the metadata table above,
assignment information belongs in the assignments block above. If a finding fits no section, put it in the
closest one — never in a new one. Use H3/H4 freely *inside* a section to structure it.
```

### 1b. Drop `&` from three headings

`markdown-it-anchor` slugifies `Lifecycle & operations` to `lifecycle-%26-operations` — URL-encoded, awkward
in a CSS selector and in a deep link. Rename in the same pass:

- `Lifecycle & operations` → `Lifecycle and operations` (six templates)
- `Expiry & renewal` → `Expiry and renewal` (`credential_prompt.tmpl`)
- `Usage & references` → `Usage and references` (`referenced_prompt.tmpl`)

### 1c. Attributes on the setting `<details>` blocks

The model already hand-writes these tags for every setting, so attributes cost nothing structurally. Extend
the `Settings:` / `Properties:` / `Definition:` instructions:

```
- Open each block as `<details data-setting="<exact YAML path>">`, e.g.
  `<details data-setting="installExperience.runAsAccount">`. The path is the same string the `<summary>`
  shows — never invent or abbreviate it.
- Add `data-note="security"` when the setting is one you called out in the Security section, or
  `data-note="inert"` when it is present but has no effect because a gating setting is off. Omit the
  attribute otherwise. Use no other value.
```

`data-setting` gives the browser a stable per-setting deep-link target; `data-note` carries the two
judgements it cannot compute. **Deliberately excluded:** a `default` / `non-default` classification. That
needs the service default, which the model would have to recall — exactly the hallucination these templates
otherwise guard against.

### 1d. Enforce it

A contract nothing checks will drift again. `internal/docs/generate_prompt_template.md` §4 already runs
structural checks; add a row (and the matching assertion in its Python checker):

| Check | Expectation |
|---|---|
| Heading vocabulary | Every `##` outside fenced code blocks is in the closed set for that document's template family, spelled exactly, in order, without duplicates. |

Same caveat as the existing *Heading structure* row: `deviceShellScripts` / `deviceManagementScripts`
documents embed shell scripts whose `##` comment lines are not headings.

### What stays out of the prompt

The division of labour matters more than the wording — **never ask the model for something a program can
derive**, because a bad batch can only be fixed by regenerating everything:

| Hook | Produced by | Why not the prompt |
|---|---|---|
| `<section data-section="security">` wrappers | web renderer | Deterministic from the H2 slug; hand-written wrappers across 340+ documents would eventually be unbalanced |
| A class on the metadata table | web renderer | Positional (first table after the H1 + summary) |
| A wrapper around the assignments block | web renderer | The `<!-- assignments:start -->` / `<!-- assignments:end -->` markers already exist and are tool-maintained |
| A document-family marker (`policy`, `group`, `arm`, …) | `docs generate-index` | Derivable from the resource type; the model would only be guessing at its own template |

Per-finding severity tags in `Security` (`**[risk]**` / `**[review]**` / `**[ok]**`) are genuinely
model-only information and tempting, but they are a subjective call made 400+ times with nothing able to
validate them. Section-level styling gets most of the visual benefit — revisit once the vocabulary has
proven stable, not in the same regeneration.

### Cost and blast radius

Editing any `.tmpl` changes that type's `promptSha256`, and `GeneratePrompt` classifies a document stale
when `fm.promptSha256 != type` — so **every document of every affected type must be regenerated** (265 in
`cb-gmbh.com` alone). This is therefore a *one-shot* change: land 1a–1d together, regenerate once. It is
also why per-finding severity is postponed rather than bundled speculatively.

Also touched: `internal/models/documentation_test.go` (asserts `"Lifecycle & operations:\n"`),
`internal/handlers/graph/prompt_templates_test.go` and `internal/handlers/arm/prompt_templates_test.go`
(per-family section lists), plus `CHANGELOG.md` — and the entry must state that a full regeneration is
required.

### 1e. The tenant summary has its own vocabulary

`docs/summary.md` is written from `internal/docs/generate_prompt_template.md`, not from the per-type
templates, and its four headings (`Management summary`, `At a glance`, `Assignment posture`,
`Coverage caveats`) are already consistent across both exports. It is the body of the browser's tenant
landing page, so it deserves the same "closed set, verbatim" declaration — but it is a separate edit that
does **not** change `promptSha256`, so it can ship independently of 1a–1d.

### Consumer

The web-side renderer work that depends on this is written up in `../web/NEXT-ITERATIONS.md`
("Section-level styling hooks"). The renderer changes there are useful on their own, but only become
*reliable* once the vocabulary is closed and regenerated.
