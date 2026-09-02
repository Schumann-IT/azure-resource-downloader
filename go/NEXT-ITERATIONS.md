# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. Redact free-text secrets

**Goal.** Bring every tenant's generated documentation back in sync with the prompts after a security fix to
the per-type prompt templates: a full one-shot regeneration across every tenant.

> **Why the templates changed.** Real tenant runs found credential-shaped secrets — a plist
> `REMOTEOFFICEAUTHKEY`, a profile-removal password in a `description` — that the service returned unmasked
> and the per-resource documents then reprinted verbatim, widening exposure from Intune readers to anyone
> with read access to the docs. The fix teaches the prompts to redact such values: a free-text
> credential-redaction rule was added to six per-type templates — `internal/models/documentation_prompt.tmpl`
> (default) and the `singleton`, `group`, `credential`, `referenced` and `arm` overrides — so a
> credential-shaped value in a free-text field (`description`/`notes`) or a decoded payload (plist/XML/base64)
> that the service did not mask is rendered as `«redacted — secret present in source»`, still documented and
> flagged in `Security`, with the literal value left only in the source YAML.
>
> **Why that forces a regeneration.** Editing a `.tmpl` changes that type's assembled `doc-prompt.md`, and
> therefore its `promptSha256`. The incremental engine hashes source + prompt + assignments, so on the next
> `azure-rd docs generate-prompt` run every document of an affected type is flagged stale (prompt-hash
> mismatch) and lands in the work list automatically — no manual list to maintain.
>
> **Scope.** Every type except the `record` family (`windowsAutopilotDeviceIdentities`, `deviceCategories`,
> `ndesConnectors`, `mobileThreatDefenseConnectors`): its template was deliberately left unchanged, so those
> documents are not reissued and keep their current `promptSha256`.
>
> **Not hash-affecting, but shipped in the same run.** The other edits to
> `internal/docs/generate_prompt_template.md` — the §7 `summary.md` contract check, the §8 on-disk run report
> (`docs/report-<date>-<time>.md`), and the three §4 checker fixes (`<details>` counting, retained-doc
> tolerance, write-once mtime snapshot) — live in the *generation* prompt, not in any type's `doc-prompt.md`,
> so they do not change `promptSha256`. They simply take effect on this same regeneration run.

**Plan.**
- Run the documentation regeneration once per tenant (`azure-rd docs generate-prompt`, then the doc pass).
- Confirm completion via the run's own §4/§6/§7 checks and the new on-disk report; until it is done, the
  on-disk documents lag the prompts for every non-`record` type.

## 2. Group documentation by purpose, resolved at index time

**Goal.** Let a reader browse the documentation by what a resource is *for* — its function or the programme it
belongs to — rather than by which Graph endpoint it happened to come from, so a single initiative like CIS L1
hardening reads as one thing instead of being scattered across four unrelated endpoint sections.

> **Why.** `docs/index.yaml` describes every resource only by `type`, so every consumer groups by endpoint:
> the browser's sidebar and the Confluence export both mirror the export layout. In the reference tenant that
> is 263 documents across 28 endpoint sections, with CIS L1 hardening spread over
> `deviceManagementConfigurationPolicies` (25), `deviceShellScripts` (4), `deviceCustomAttributeShellScripts`
> (2) and `deviceManagementScripts` (1) — and nothing in the index saying they are one programme.
>
> **Resolve it in the CLI, never in a consumer.** `web/src/docs/export/confluence.ts` and the browser's
> `buildNavigation()` are two independent readers of the same index; a taxonomy computed in one is silently
> absent from the other. The grouping is therefore computed once, when `docs/index.yaml` is built.
>
> **Two axes, computed three ways.** (a) *Facts already exported* — `odataType`, `platforms`, `technologies`,
> `templateFamily`, `run.scope`, most already on `IndexResource` — are deterministic and need no regeneration
> but are blind to intent (`templateFamily: none` on 55 of 73 settings-catalog policies, and nothing says "this
> is CIS hardening"): good for a platform axis, weak for a function axis. (b) *Model-authored
> `platformGroup`/`functionGroup`* in document frontmatter capture intent but cost a full regeneration and give
> each document exactly one function group, which CIS hardening does not fit — a CIS shell script is both
> *hardening* and *scripts*. (c) *A curated `taxonomy.yaml` resolved at index time* — rules over the facts plus
> name patterns (93 of 225 named resources match `GBL_<kind>_<env>_<scope>_[CIS_]<platform>_<area>[_L1]`, so one
> rule catches all 32 CIS-named resources) — yields a **multi-valued** `groups:` field, needs no regeneration,
> and makes a new programme (`Defender / MDE`, `VPN`, `Update rings`, `Autopilot`, `App delivery`) a config edit
> rather than a 263-document rerun; its cost is a file to maintain and rules that rot silently when naming
> conventions drift.
>
> **The chosen mix.** (b) as the spine — one platform and one function group per document, model-authored,
> because intent is a judgement only the document generation can make — with (c) layered over it as a
> multi-valued *programme* dimension, and (a)'s facts kept as filters.
>
> **Sequencing and hash impact.** (b) emits a `doc-groups` marker into every `doc-prompt.md`, moving each
> affected type's `promptSha256` and forcing a full regeneration. It must therefore ride a regeneration that
> is already happening — but specifically one triggered by a **non-prompt-contract** reason (a bulk
> re-download/refresh), *not* one triggered by another prompt change: a run that also carries, say, a
> redaction-template fix conflates two sources of prompt churn and neither can be verified against a clean
> baseline. That is a narrow window, so (b) stays unscheduled until such a regeneration is on the calendar for
> its own reasons. (c) changes no prompt and needs no regeneration, so it ships independently, now. The harvest
> half of (b) is already wired: `parseFrontmatter` reads `platformGroup`/`functionGroup` and `generateindex.go`
> copies them into `IndexResource` — they are simply empty on all 263 documents today because no prompt asks
> the model to fill them.
>
> **Must emit either way.** An `uncategorised` count surfaced in the §8 run report, so a taxonomy that quietly
> stops matching is visible instead of manifesting as a thinning tree; and stable group identifiers, since
> consumers build navigation and — for Confluence — page labels from them.
>
> **Two further requirements the consumer cannot supply for itself.** The **group vocabulary must travel in
> the index with its display order** — a consumer that only receives per-resource values can sort
> alphabetically or not at all, and a copy of the order kept in the browser is a contract in two
> repositories that will drift on the first vocabulary edit. And **"no group" needs defined semantics**,
> distinguishing a document the model did not classify from a type for which the axis is meaningless (a
> tenant-level singleton has no platform), so the consumer's uncategorised bucket does not merge a real
> taxonomy gap with a category error. Both are cheap at index time and impossible downstream.
>
> **What the programme dimension must additionally carry.** A programme is a *facet*, not a tree level, so
> each membership is a pair — a **stable id** (the value a consumer puts in a URL, which must survive a
> programme rename so a bookmark does not break) and a **display label** — never a bare string doing both
> jobs. And the index must publish **the full set of programmes the taxonomy defines**, in display order,
> including programmes with **zero matches in this tenant**: from per-resource membership alone a consumer
> cannot tell a programme that does not exist from one that simply matched nothing here, and "this programme
> is empty in this tenant" is itself information. This is the programme analogue of the axis vocabulary above.
>
> **Additive and independently releasable.** The browser tolerates schema drift — its `parseTenantIndex()`
> accepts any `version >= 1` and ignores fields it does not know — so these fields can be added and
> `IndexFile.Version` bumped whenever it suits, in either order, without a lockstep browser release.
>
> **Decided shapes (so this is implementable).** The two axis vocabularies are a single global contract, not a
> per-type list: ordered package-level constants in `internal/models` (e.g. `PlatformGroups`,
> `FunctionGroups`), each including a reserved `n/a` value. `taxonomy.yaml`: a top-level `version: 1` and an
> ordered `programmes:` list, each `- id:` (stable, URL-safe, never reused for a different programme), `label:`
> (display) and `match:` (a list of rules OR-ed together; within one rule the closed fact keys `name` — regex
> over displayName — `type`, `odataType`, `platforms` and `scope` are AND-ed). Index header:
> `vocabularies: {platform: […], function: […]}` in display order, plus `programmes: [{id, label, count}]` —
> the full registry, `count` being matches in this tenant with `0` kept. Per resource: `groups: [{id, label}]`,
> empty meaning uncategorised. **Empty-group semantics for the axes:** an explicit `n/a` value means "this axis
> does not apply to this type" (a tenant-level singleton has no platform) and is **not** counted as
> uncategorised; a blank/absent axis field means "not yet classified" and **is** the uncategorised bucket — the
> two are never merged.
>
> **Cross-reference.** The browser side is a planned entry waiting on precisely these fields being non-empty
> — see *Group-driven navigation, with a programme facet over it* in `web/NEXT-ITERATIONS.md`, whose *What
> the CLI must provide before the spine can be built* section is the authority on this list.

**Plan.**

Prerequisite (pure code, no regeneration — unblocks both steps; lands with whichever ships first):
- Define the two vocabularies ONCE as ordered package-level constants in `internal/models` (e.g.
  `PlatformGroups`, `FunctionGroups`), each including the reserved `n/a` value. They are a single global
  contract — not per-type and not a per-template literal like `doc-headings`; the constant is the source of
  truth and every other consumer derives from it. Defining a constant triggers no regeneration, so this is
  NOT gated by the sequencing note — in particular step (c) can consume it and ship without waiting on (b).

Step (b) — model-authored group axes (the marker + frontmatter contract; regeneration-gated, see the
sequencing note above):
- Have `BuildDocumentationPrompt` render those constants into a `<!-- doc-groups: platform=… | function=… -->`
  marker written into every `doc-prompt.md`, so the marker stays self-describing for the checker while the Go
  constant stays canonical (no per-template literal to keep in sync). This threads through the default template
  `internal/models/documentation_prompt.tmpl` and the `singleton`/`group`/`credential`/`referenced`/`arm`
  overrides via the shared render path, not by editing each literal.
- In `internal/docs/generate_prompt_template.md`: §2 require both frontmatter fields; add a NEW frontmatter
  check (adjacent to the existing frontmatter checks, distinct from the §4 heading sweep) that fails a document
  whose `platformGroup`/`functionGroup` is outside the marker's closed set — `n/a` is in the set — never
  inferred, the same discipline `doc-headings` gets.
- Keep the harvest as-is (it already copies the fields into `IndexResource`); add tests for the rendered marker
  and the new frontmatter check.

Step (c) — curated taxonomy (no regeneration; ships independently, now):
- Add `internal/docs/taxonomy.go` (+ `taxonomy_test.go`) parsing `taxonomy.yaml` (shape in the *Decided
  shapes* note) into an ordered programme registry and a rule engine: a programme joins a resource if ANY of
  its rules matches, and within a rule the closed fact keys AND together; a resource matching no programme is
  `uncategorised`. Emit each resource's `groups` in registry (display) order so `index.yaml` stays
  byte-identical across runs — the project's determinism invariant.
- In `internal/docs/generateindex.go`: add `Groups []IndexGroup` (`IndexGroup{ID, Label}`) to `IndexResource`,
  populated at index time; add the header registry `Programmes []IndexProgramme` (`{ID, Label, Count}`, Count =
  matches in this tenant, `0` kept) and the axis `Vocabularies{Platform, Function []string}` (from the shared
  vocabulary constants). Pass any missing raw fact through `internal/docs/metadata.go` if the rules need it.
- Add a `--taxonomy` flag to the `generate-index` command (offline, alongside `--domain`/`--out`) pointing at
  the curated `taxonomy.yaml`; when absent, `groups` and `programmes` are empty (no default taxonomy is
  embedded) and the index falls back to today's per-type grouping.

Shared:
- Report the `uncategorised` counts so a taxonomy that quietly stops matching is visible: the programme
  `uncategorised` count (step (c), computed at index time) in the `generate-index` run report
  (`reportGenerateIndex` in `cmd/docs/generate_index.go`), and the axis `uncategorised` count (blank
  `platformGroup`/`functionGroup`, step (b)) in the doc pass's §8 report. Guarantee group ids are stable
  across runs.
- Document the `index.yaml` grouping contract in `README.md`: the header `vocabularies`/`programmes` fields,
  the per-resource `groups`, and the `n/a`-vs-blank empty-group rule decided above (the axis vocabularies are
  emitted from the shared vocabulary constants, so consumers order navigation from the data, never from a
  drifting copy).
- Bump `IndexFile.Version` when these fields land (the browser accepts any `version >= 1` and ignores unknown
  fields, so the bump is safe and needs no coordinated browser release).
- Add a `CHANGELOG.md` entry under `[Unreleased]` as each step ships, and update `config.example.yaml` only if
  a config option (e.g. the taxonomy path) is added.

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are removed. Each records why it is parked and what would make it worth doing.

### Idea: per-finding severity in document `Security` sections

Tag every individual security callout *inside each resource's document* with `**[risk]**` / `**[review]**` /
`**[ok]**`, so the web side can colour or filter them. **Not planned — parked deliberately**, for three
reasons:

- **It is a subjective, model-only judgement.** Nothing in the export can compute or validate whether a given
  setting is risk / review / ok.
- **It is made 400+ times** (once per callout across every document), so a bad or inconsistent batch is
  likely — and the only fix is regenerating everything.
- **Low marginal benefit.** Section-level styling (the `Security` H2 slug) already gives the frontend most of
  the visual win without the per-item risk.

This is the opposite trade-off from the tenant-summary findings severity, which is decided once per tenant on
at most six findings — tiny blast radius, easy to eyeball — and was therefore done.

**Revisit only if both hold:** (1) the closed-heading contract has proven stable across a real regeneration,
with no drift observed in practice; and (2) the web side needs per-item severity that section-level styling
cannot deliver. If promoted, treat it as its own one-shot: extend the `Security:` instruction across all
seven templates and regenerate every document — and accept that it cannot be automatically validated.
