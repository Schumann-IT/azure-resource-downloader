# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. Generate a drift-impact analysis prompt from the latest drift observation

**Goal.** After `resource drift` has found changes, one offline command — `azure-rd docs analyze-drift` —
writes a ready-to-paste prompt that has an LLM analyze the differences between the export baseline and the
observed tenant state and report their **impact**: security posture, compliance, lifecycle, and who is
affected (assignments). The operator reads that report to understand what the drift *means* before deciding
to re-baseline (`resource download`) and let `docs generate-prompt` catch the documentation up.

> **Everything the analysis needs is already on disk — the command never fetches.** `drift/metadata.yaml`
> is self-describing (verdicts, dotted-path deltas, baseline/payload hashes, derived `docPath`, completeness,
> `notComparable`); `drift/<type>/<name>.yaml` holds the current bytes of every added/changed/renamed
> resource; `resources/<baselineKey>.yaml` holds the old bytes; `docs/<key>.md` describes the pre-change
> state; and each type's `resources/<type>/doc-prompt.md` already carries the type's purpose, key settings
> and security relevance. The command is the drift analogue of `docs generate-prompt`: load, preflight,
> splice a template, write one file.
>
> **No per-type drift templates — one generic analysis template, with `doc-prompt.md` as the per-type
> lens.** The comparative procedure (read baseline, read payload, read deltas, judge impact) is
> type-independent; only the judgment lens is type-specific, and that lens already exists per type in
> `doc-prompt.md`, which the template instructs the agent to read for each finding's type (degrading with an
> explicit caveat when the export was run with `--no-prompt`). A dedicated drift template per type would be
> ~60 files of pure maintenance carrying no new information, and folding drift guidance into `doc-prompt.md`
> instead would move every type's `promptSha256` — a full documentation regeneration for a feature that
> never touches the documents. This entry is therefore **not regeneration-gated**: it adds no `.tmpl` edits
> and moves no hash.
>
> **Per-finding severity is safe here**, unlike the parked "per-finding severity in document Security
> sections" idea: a drift report judges a handful of findings once, in one disposable report an operator
> eyeballs — tiny blast radius, no regeneration to fix a bad batch. Same trade-off that allowed the
> tenant-summary findings severity.
>
> **Boundaries.** The analysis agent writes exactly one file — the report at `drift/report.md` — and never
> edits anything else: not `resources/`, not `docs/`, not the rest of `drift/`. Documentation catch-up
> remains the existing loop (re-baseline, then `docs generate-prompt`). The report is advisory prose for
> humans; nothing downstream ever parses it, so it records judgments, not facts, and revising the template
> never invalidates an export.
>
> **Output placement — everything lives and dies with the observation; no drift history.** The prompt lands
> at `drift/analyze.md` and the LLM's report at `drift/report.md`, both at the drift-tree root beside
> `metadata.yaml` (payloads are always ≥ 2 levels deep, so no collision — the same argument that puts
> `generate.md` at the docs root; the report's name is deterministic because only one can exist at a time —
> its `observedAt` and the baseline's `generatedAt` live in its frontmatter, not the filename). The next
> `resource drift` run's existing clear-and-rebuild sweeps prompt and report with the observation they
> belong to — zero new code for that path — and a **re-baselining `resource download` clears the `drift/`
> tree too**: a new baseline supersedes the observation by definition (drift is always relative to the
> baseline), so leaving it on disk would preserve an answer to a question nobody can ask any more.
> Deliberately **no history**: an operator who wants to keep a report archives it before the next drift or
> download run, and the template and README say so explicitly.

**Plan.**

- **Command** `cmd/docs/analyze_drift.go` (package `docs`, exported `NewAnalyzeDriftCommand`, attached in
  `cmd/docs.go`). Flags: the shared auth group plus `--domain` (offline), `--out`, `--prompt` (template
  override) — reusing `resolveExportDir` from the same package. `cmdutil.BindFlags(cmd)` first in `RunE`.
  Exit codes: 0 success (prompt written, or nothing to analyze); 2 cannot-answer, via
  `cmdutil.WithExitCode`. With this command, `--domain` and `--out` are declared by **three** sibling
  subcommands (`generate-prompt`, `generate-index`, this one), so extract a package-local helper in
  `cmd/docs` (all three files share the package) that declares them with a parameterised `--out` usage
  string — the flag-placement rule's "more than one command needs it" threshold is met; promotion to the
  `docs` parent's persistent flags stays off the table while `--out`'s default differs per command.
- **Preflight** (all exit 2 with a remediation message): `drift/metadata.yaml` missing → "run
  `azure-rd resource drift` first"; observation **superseded** (its `baseline.generatedAt` differs from the
  export's current `resources/metadata.yaml` `generatedAt` — a re-download happened after the observation) →
  re-run `resource drift`; tenant cross-check against the resolved domain, as `generate-prompt` does; every
  payload the observation names must exist and match its recorded `payloadSha256` (never trust a file the
  named observation did not produce). Zero findings → report "no drift to analyze", write nothing, exit 0.
- **Re-baseline invalidation.** `resource download` clears the tenant's `drift/` tree whenever it writes
  (or updates) `resources/metadata.yaml` — any metadata write moves `generatedAt`, which is exactly what
  supersedes the observation, partial `--type`-scoped runs included. Never under `--dry-run` (a dry run
  writes nothing, so it deletes nothing); the path is constructed (`<tenantDir>/drift`), never derived from
  input, mirroring `WriteObservation`'s own clear; log the removal when the tree existed. The delete-path
  enumeration in the rules grows from two to three (prune; drift's clear-and-rebuild; download's re-baseline
  clear of `drift/`) — still never touching `resources/` except via `--prune`, and never `docs/`.
- **Engine placement.** The engine needs the `drift.Observation` types and `internal/drift` already imports
  `internal/docs`, so it lives in `internal/drift` (e.g. `analyzeprompt.go` + `analyzeprompt_render.go`),
  not in `internal/docs`. Two exports from `internal/docs` make that possible without duplication:
  the splice helpers (`validateMarkers`, `spliceMarker`), and a small **reference-index facade** over the
  currently-unexported fact-derivation helpers the refmap needs (`referencedGroups`, `buildGroupInfo`,
  `buildFilterInfo`, `templateNames`, `parseAssignments`, `docRel`) — one exported type + builder exposing
  GUID → name/kind/document resolution, not six exported functions. Rebuilding any of them in
  `internal/drift` is off the table: two renderers deriving the same facts independently is exactly the
  divergence the shared indexes exist to prevent. New engine functions comply with `gocognit`'s threshold
  from the start (small render functions, like `generateprompt_render.go`) — the baseline is a debt ledger
  and never grows.
- **Template** `internal/drift/analyze_drift_template.md` (go:embed, `--prompt` override, marker validation
  before any write; the embedded file is the single source of truth, edited directly — same as
  `generate_prompt_template.md`). Marked blocks the tool splices: **observation** (observedAt, baseline generatedAt,
  completeness, unknown types, `removalsSuppressed`, `notComparable` caveats), **worklist** (findings
  grouped by type and verdict, each row naming baseline file, payload file, existing document, display
  names, and the recorded deltas), **refmap** (group/filter/notification-template GUID → name/document,
  resolved from the baseline `resources/metadata.yaml`, so "who is affected" renders from facts). Prose
  around the markers: ground rules (never invent, masked values are not findings, everything is read-only),
  per-finding procedure (what changed — facts from deltas and both files; who is affected — assignments via
  the refmap; impact — security, compliance, lifecycle, user/device; a severity tag per finding), per-verdict
  guidance (added: payload + spec; changed/renamed: deltas + both files; removed: baseline + existing doc,
  which becomes an orphan), an executive summary, and the report destination `drift/report.md` with
  self-describing frontmatter (`observedAt`, baseline `generatedAt`) and an explicit ephemerality note
  (swept by the next drift or download run; archive manually to keep it).
- **`--dry-run`**: the analysis is fully answerable offline, so a dry run performs the whole comparison and
  report, withholds only the write, and warns when an `analyze.md` from an earlier run is still on disk
  (same pattern as `generate-prompt`'s stale-prompt note).
- **Tests**: preflight refusals (no observation, superseded baseline, tenant mismatch, payload hash
  mismatch, broken template markers), zero-findings short-circuit, worklist/refmap rendering, determinism of
  the spliced output over an unchanged observation; download-side clearing (clears `drift/` on a metadata
  write, leaves it alone under `--dry-run` and when no metadata was written, never touches `resources/` or
  `docs/`) — plus a **new** docs-group flag-surface test (none exists today; mirror `cmd/resource_test.go`,
  asserting both directions: every subcommand sees the shared flags, none offers a flag it ignores).
- **Docs and rules**: README — new "Analyzing drift" section and the workflow line (download →
  generate-prompt → … → drift → analyze-drift → re-baseline); output-layout section gains `drift/analyze.md`
  and `drift/report.md` with their ephemerality (swept by the next drift run, and by a re-baselining
  download). `CHANGELOG.md` under `[Unreleased]` in the same edit as the code.
  `.windsurf/rules/01-project.md` (layout: the analyze-prompt engine in `internal/drift/`) and
  `04-security-and-ops.md` (drift-tree lifecycle: `resource drift` owns and rebuilds the tree, a
  re-baselining `resource download` clears it, `docs analyze-drift` writes `analyze.md` and the analysis
  agent writes `report.md` at its root; delete-path enumeration updated to three). `config.example.yaml`
  untouched — the command adds no config option beyond its local flags.

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are removed. Each records why it is parked and what would make it worth doing.

### Idea: routine dependency updates, and consolidating on one Microsoft Graph SDK

Two related pieces of dependency hygiene. First, a routine update pass (`go get -u`, `make deps`, `make check`)
over the direct dependencies. Second, investigate whether the module can carry **one** Microsoft Graph SDK
instead of two. The direction is the opposite of the intuitive one: `msgraph-beta-sdk-go` is imported by 53
handler files and serves the Intune/device-management endpoints that **do not exist on v1.0**, so it can never
be the one removed. The candidate for removal is `msgraph-sdk-go` (v1.0), used by one shared client constructor
and seven handlers (conditional access, groups, organization, authentication methods/strengths, authorization
policy, on-premises synchronization) — the beta endpoint is a superset, so those seven could move. The prize is
real: the two generated SDKs dominate compile time and binary size, and one of them is nearly gone already.
**Not planned — parked deliberately**, for three reasons:

- **A dependency bump is not hash-neutral by construction.** A Graph SDK update can change which properties a
  model carries and therefore the YAML bytes the export writes — moving `sourceSha256` for resources that did
  not change in the tenant, which reads as mass drift and forces documentation regeneration. Until the drift
  command exists there is no cheap way to *prove* a bump content-neutral; after it ships, "update, re-download
  a fixture tenant, drift must report zero findings" becomes the standard verification, and updates get cheap.
- **Moving the seven v1.0 types to beta trades a stability contract for uniformity.** v1.0 responses are
  contractually stable; beta responses may change shape at Microsoft's discretion. Today the most stable,
  most-referenced types (groups, conditional access) deliberately sit on the stable endpoint. Consolidation
  buys shorter builds but makes every type's bytes hostage to beta churn.
- **The switch itself moves hashes once.** Re-fetching those seven types through the beta endpoint will change
  their YAML (beta models carry extra properties), so their `sourceSha256` values move and their documents
  regenerate — a one-time cost that should ride a regeneration scheduled for another reason, not force its own.

**Revisit when** the drift command has shipped (it is the verification tool this work lacks), and either a
security advisory forces an SDK bump anyway or build time becomes a felt cost. If consolidation is picked up:
move the seven handlers one at a time, verify each with a fixture-tenant drift run, expect and batch the
one-time hash movement, and only then drop the v1.0 module. If beta churn is the worry instead, the same
investigation can conclude the opposite consolidation is wiser once Microsoft ports the Intune endpoints to
v1.0 — check that first; it would remove the *beta* SDK and the churn with it.

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

### Idea: resolve Graph object ids to names inside the exported YAML

Add a transformer that resolves Microsoft Graph object ids that appear in a resource — assignment `groupId`s,
filter ids, `notificationTemplateId`s — to their display names at export time, as the `id-resolution`
transformer already does offline for ARM resource ids (which carry their name in the id itself). The YAML
would then read `groupId: 8964516b-… (GBL_D_WIN_...)` instead of a bare GUID. **Not planned — parked
deliberately**, for three reasons:

- **The documentation already resolves them, and does so incrementally.** `docs generate-prompt` builds the
  group, filter and template reference maps from `metadata.yaml`, renders every assignments / "Targeted by" /
  "Used by" block from them, and re-splices exactly those blocks when a referenced object is renamed — without
  touching the resource's own document or its YAML. Resolving in the YAML would duplicate that with a worse
  failure mode.
- **It would put a decision into a fact.** A resource's YAML and its `sourceSha256` are meant to move only when
  the resource itself changes. Embedding another object's *current* name makes every policy's hash move when a
  group is renamed, which forces regenerating every document that assigns it — the exact cascade the marked
  splice blocks exist to avoid.
- **It costs one extra Graph read per referenced id**, on every run, for information the export already holds
  once (in the group's own YAML).

**Revisit only if** a consumer other than the documentation pipeline needs names inside the YAML itself — e.g.
a diff/review workflow on `resources/` that cannot read `metadata.yaml`. If promoted, resolve from the export
(the already-downloaded groups/filters/templates), never from a live lookup, and write the name into a sidecar
`_name` key the way `id-resolution` does — never in place of the id.

### Idea: bootstrap the curated taxonomy from per-document LLM suggestions

Have the doc-generation model suggest, per resource, which programmes it belongs to (as *labels* with a short
rationale, never ids), then harvest those suggestions at index time into `docs/taxonomy-suggestions.yaml` — a
report that diffs the guesses against the curated rules into **coverage gaps** (a resource the model assigns to
an existing programme that no rule matches) and **new-programme candidates** (a label that is not a programme
yet). It would let an operator grow the `taxonomy:` rule set from evidence instead of authoring every regex
cold. **Not planned — parked deliberately**, for two reasons:

- **Its payoff scales with taxonomy size, which is small today.** The report earns its keep only once an
  operator is maintaining a large, drifting rule set across many tenants. Cold-authoring the handful of
  programmes in play now is cheaper than building and reviewing an advisory pipeline.
- **It cannot land cheaply on its own.** The suggestion instruction lives in the per-type templates, so adding
  it moves `promptSha256` for every non-`record` type and forces a full documentation regeneration. It is only
  free if it rides a regeneration already scheduled for another reason — otherwise it forces its own.

**Revisit when** an operator is maintaining programmes at a scale where cold-authoring rules is the bottleneck,
*and* a full documentation regeneration is already scheduled to absorb the template change. If promoted, the
shape is constrained — these are invariants that keep it safe, not open questions:

- **Advisory only, never authoritative.** `facets` stays rules-only and deterministic; suggestions live in a
  separate artifact and the only path from a guess to authoritative data is a human writing a rule. Promotion
  stays manual — auto-writing rules would re-inject non-determinism into the rules source and defeat the point.
- **Labels, not ids.** The operator mints the stable id (`programmeIDPattern`) at promotion time, so the model
  can never spawn `cis`/`cis-l1`/`cis-hardening` as three programmes.
- **Determinism preserved.** Because suggestions are harvested from written frontmatter bytes, both
  `index.yaml` and the suggestions artifact stay byte-identical over an unchanged export; the non-determinism
  is confined to doc-authoring time, exactly as `platformGroup`/`functionGroup` already are.
- **The hint vocabulary must not touch `promptSha256`.** Any hint of the operator's current labels rides the
  non-hashed `docs/generate.md`, never a per-type `doc-prompt.md`, so a cheap offline `taxonomy:` edit never
  couples to an expensive regeneration. Suggesting with no hint (pure bootstrap, no `taxonomy:` yet) must also
  work; label normalisation clusters the free output.
- **`docs/taxonomy-suggestions.yaml`** is the third and last file `azure-rd` writes under `docs/` (with
  `generate.md` and `index.yaml`), at the tree root where no document can be; `--dry-run` writes nothing and
  `--prune` never touches it.
- **`config.example.yaml` stays inert** — the feature needs no new config key (it reuses `taxonomy:` labels as
  an optional hint), so loading the example unmodified still produces byte-identical output including every
  hash in `resources/metadata.yaml`.

### Idea: review the sign-in surface — can a scoped `az login` replace the dedicated-app device-code path?

The tool carries two sign-in paths. The default reuses the `az login` session; `--client-id`/`--tenant-id`
starts a device-code sign-in against a dedicated app registration. The second path exists for exactly one
reason: `az account get-access-token` always mints tokens for the Azure CLI *first-party* app — regardless of
how the user logged in — and that app's Graph token has lacked the delegated scopes most Graph handlers
declare (`DeviceManagementConfiguration.*`, `DeviceManagementApps.*`, `DeviceManagementScripts.*`,
`Policy.Read.All`, …). But `az login` accepts `--scope`: signing in with
`az login --scope https://graph.microsoft.com/.default` (or explicit scopes) asks Entra to add delegated
Graph scopes to that same CLI session — the README's own troubleshooting hint already leans on it for the
"required scopes are missing" failure. If a scoped login reliably lands every declared scope in the session's
Graph token, the entire second path becomes removable: the device-code credential branch, the two flags and
their env/config equivalents, the `PermissionScoped` probe (`RequiresDedicatedApp` /
`DedicatedAppRequirements`), the interactive dedicated-app prompt, and the app-registration setup in
`README.md` — collapsing authentication to one path and one instruction. Even a partial "yes" has value: the
dedicated-app prompt could recommend the exact scoped re-login first and fall back to device code only when
the CLI app genuinely cannot obtain a scope. **Not planned — parked deliberately**, for three reasons:

- **The answer is not in this repository.** Whether a scope lands in the CLI token's `scp` claim depends on
  the tenant's consent policy and on which scopes Microsoft lets its first-party app request — both outside
  the tool's control and changeable by Microsoft without notice. Only a live-tenant experiment settles it:
  perform a scoped `az login`, decode the token's `scp`, verify that azidentity's CLI credential (which
  shells out to `az account get-access-token` per request) actually surfaces the scopes granted at login,
  then run a full download of every dedicated-app-gated type — including `--resolve-secrets`, which needs
  `DeviceManagementConfiguration.ReadWrite.All`.
- **A positive result on one tenant does not generalize.** Consent policies differ per tenant, Microsoft has
  been progressively hardening what the first-party CLI app may do, and some services may gate on the calling
  application rather than the token's scopes alone — only a live call against each gated endpoint settles
  that. The dedicated app registration is the escape hatch the operator controls; deleting it trades
  resilience for a smaller surface.
- **Removal is a breaking change to the auth surface** — the flags, their `AZURE_RD_*` variables and config
  keys — so it should ride a major, not a hygiene pass.

**Revisit when** an operator confirms on a representative tenant that a scoped `az login` yields every
permission the handlers declare, or the next time the authentication surface is reworked anyway. If promoted,
soften before deleting: first teach the dedicated-app prompt to recommend the exact `az login --scope …`
command derived from the selected types' declared permissions, keeping device code as the fallback; only
retire the flags once the CLI path has covered every `PermissionScoped` type against a live tenant, and
record the removal under `Breaking`.

### Idea: clear the `gocognit` baseline

Split up the 26 functions named in the baseline at the end of `.golangci.yml`'s `exclusions.rules` — the ones
that already exceeded the Sonar cognitive-complexity threshold when `gocognit` was switched on, across 20 files —
deleting each entry as its function is fixed, until the block and its explanation can go. The worst are
`GeneratePrompt` (58), `GenerateIndex` (57), `findAndRemoveKeysWithPreserve` (47), `ParseCleaningConfig` (39),
`drift.Compare` (38), `compileTaxonomy` (37) and `PrintSummary` (34); the rest sit between 16 and 30.
**Not planned — parked deliberately**, for three reasons:

- **The baseline already delivers what mattered.** The rule stays enabled at the server's threshold, the gates
  are green, and every function written from now on has to comply — including new functions in the listed files,
  because each entry names one function instead of excluding its file. Nothing is hidden either: Sonar keeps
  reporting all 26, which is deliberately why the baseline is not mirrored in `sonar-project.properties`.
- **A high score here is not a defect, and several of these functions are covered by invariants that a
  refactor must not disturb.** `PrintSummary`, `mergeMetadata`, `pruneCovered` and `Compare` are exactly the
  places where "an incomplete run may not mark anything absent", "covered means the listing succeeded" and the
  prune guards live; `Writer.Write` and `transformResource` sit on the every-request-produces-one-result
  accounting. Splitting them for a metric, without a reason a reader would recognise, risks a real regression in
  return for a number.
- **Go's explicit error handling inflates the metric**, so a mechanical fix — extracting each `if err != nil`
  ladder into a helper — would move the score without making anything clearer, which is the outcome the rule is
  supposed to prevent.

**Revisit when** one of these functions is being changed for another reason (split it then and delete its entry
in the same commit — that is how this shrinks without a campaign), when a function on the list becomes hard to
change safely in practice rather than merely scoring high, or if the baseline ever stops shrinking, which would
mean it has started collecting new debt instead of recording old. If picked up, work one function at a time with
`make test-race` where the function touches the pipeline, keep the tests that pin the invariants above unchanged
(a refactor that needs a test edited is a redesign, not a split), and remember that a stale entry is invisible —
golangci-lint does not report an exclusion that matched nothing, so re-measure by commenting the block out. Each
split is internal and needs no `CHANGELOG.md` entry; deleting the block at the end does.
