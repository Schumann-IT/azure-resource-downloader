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
>
> **Also riding this regeneration: the model-authored group axes.** A `<!-- doc-groups: platform=… |
> function=… -->` marker is now appended to every non-`record` type's `doc-prompt.md`, so it changes the same
> `promptSha256` set this regeneration already covers (record types opt out via `OmitGroupAxes` and keep
> theirs, exactly matching the redaction scope above). As each document is reissued it must additionally
> carry a `platformGroup` and a `functionGroup` in frontmatter, each a value from that marker's closed set
> (`n/a` where the axis does not apply); the §4 check enforces this and the §8 report surfaces the axis
> `uncategorised` count. So this one regeneration brings the docs back in sync with the redaction fix *and*
> populates the grouping axes `docs generate-index` already emits into `index.yaml`.

**Plan.**
- Run the documentation regeneration once per tenant (`azure-rd docs generate-prompt`, then the doc pass).
- Confirm completion via the run's own §4/§6/§7 checks and the new on-disk report; until it is done, the
  on-disk documents lag the prompts for every non-`record` type.
- Confirm each reissued document carries `platformGroup`/`functionGroup` and that the §8 report's axis
  `uncategorised` count is only the `record` family (the types that legitimately have no `doc-groups` marker).

## 2. Bootstrap the curated taxonomy from per-document LLM suggestions

**Goal.** Lower the cost of authoring and maintaining the curated `taxonomy:` without surrendering its
determinism. The model has already read each resource and written its document, so it is well placed to
suggest which programmes a resource belongs to; turning those suggestions into reviewed promotion candidates
lets an operator grow the rule set from evidence instead of authoring every regex cold.

> **Advisory only, never authoritative.** `facets` stay rules-only and deterministic; the model's
> output lives in a separate artifact and the *only* path from a suggestion to authoritative data is a human
> writing a rule. There is therefore no runtime precedence between the model and the config — they are in
> different lanes, and a guess can never be mistaken for, or override, a stated decision.
>
> **Labels, not ids.** The model emits programme *labels* (with a short rationale), never url-safe ids. The
> operator mints the stable id (`programmeIDPattern`) at promotion time, so the stable-id-in-a-URL and
> cross-tenant-consistency contracts stay entirely on the curated side and the model can never spawn
> `cis`/`cis-l1`/`cis-hardening` as three programmes.
>
> **The report is a diff against the rules.** `docs generate-index` (offline, deterministic — it already reads
> frontmatter) aggregates the suggestions tenant-wide and emits two signals: **coverage gaps** (resources the
> model assigns to a programme that already exists but no rule matches) and **new-programme candidates** (a
> label that is not a programme yet). It writes `docs/taxonomy-suggestions.yaml`, a sibling of `index.yaml` at
> the docs tree root — the third and last file `azure-rd` writes under `docs/`, alongside `generate.md` and
> `index.yaml`, all at the root where no document can be. `--prune` must never touch it.
>
> **Determinism preserved.** Because the suggestion is harvested from written frontmatter bytes, both
> `index.yaml` and the suggestions artifact stay byte-identical over an unchanged export; the non-determinism
> is confined to doc-authoring time, exactly as `platformGroup`/`functionGroup` already are.
>
> **The hint vocabulary must not touch `promptSha256`.** If the model is hinted with the operator's current
> programme labels (so it reuses `defender` rather than inventing a synonym), that hint must ride the
> non-hashed generation prompt (`docs/generate.md`), never the per-type `doc-prompt.md`. Baking the config's
> vocabulary into a hashed prompt would couple a cheap, offline `taxonomy:` edit to an expensive documentation
> regeneration — the precise coupling this whole design exists to avoid. Suggesting with no hint at all (pure
> bootstrap, no `taxonomy:` yet) must also work; the report's label normalisation clusters the free output.
>
> **Sequencing.** Adding the suggestion instruction to the per-type templates moves `promptSha256` for every
> affected type, forcing a documentation regeneration. Any prompt-template change already reissues every
> non-`record` document, so whenever a full regeneration is scheduled, land this template change **before it
> runs** and it rides along at zero marginal cost — otherwise it forces its own full regeneration later.
> Schedule it with the next regeneration rather than on its own.
>
> **Promotion stays manual.** The tool reports; the operator writes rules. Auto-writing rules from model
> output would re-inject non-determinism into the rules source and defeat the purpose.
>
> **`config.example.yaml` stays inert.** The feature needs no new config key (it reuses `taxonomy:` labels as
> an optional hint), so loading the example unmodified still produces byte-identical output including every
> hash in `resources/metadata.yaml`.

**Plan.**

- **Add a taxonomy-suggestion instruction** to the non-`record` per-type templates (the default plus the
  `singleton`/`group`/`credential`/`referenced`/`arm` overrides — confirm the set against the templates at
  implementation time), producing a frontmatter `suggestedGroups: [{label, why}]` (labels only, no ids). Gate
  the work to ride the next documentation regeneration (see Sequencing).
- **Harvest and diff in `GenerateIndex`**: read `suggestedGroups` from each document's frontmatter, aggregate
  per normalised label, and emit `docs/taxonomy-suggestions.yaml` splitting each into coverage-gap (matches an
  existing programme by normalised label but no curated rule covers the resource) vs new-programme-candidate.
  Deterministic output; `--dry-run` writes nothing; `--prune` never touches it.
- **Keep `facets` rules-only** — suggestions never merge into the authoritative facets map or the facets
  header.
- **Keep any hint vocabulary out of `doc-prompt.md`** — pass it through the non-hashed `generate.md` so
  `taxonomy:` edits stay decoupled from `promptSha256`.
- **Tests**: frontmatter harvest, gap-vs-candidate classification, byte-identical re-run over an unchanged
  export, the no-suggestions case, and the no-`taxonomy:` bootstrap case (free labels, still clustered).
- **Document it**: the new artifact and the review→promote workflow in the taxonomy section of `README.md`,
  and an entry under `## [Unreleased]` in `CHANGELOG.md`.

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
