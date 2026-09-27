# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. ~~Generate a drift-impact analysis prompt from the latest drift observation~~

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
> **Boundaries.** ~~The analysis agent writes exactly one file — the report at `drift/report.md` — and never
> edits anything else~~ *(superseded by the follow-up below: the agent writes one drift document per finding
> plus `drift/index.md`)*: not `resources/`, not `docs/`, not the rest of `drift/`. Documentation catch-up
> remains the existing loop (re-baseline, then `docs generate-prompt`). The report is advisory prose for
> humans; nothing downstream ever parses it, so it records judgments, not facts, and revising the template
> never invalidates an export.
>
> **Output placement — everything lives and dies with the observation; no drift history.** The prompt lands
> at `drift/analyze.md` and the LLM's report at ~~`drift/report.md`~~ *(follow-up: per-finding documents +
> `drift/index.md`)*, at the drift-tree root beside `metadata.yaml` (payloads are always ≥ 2 levels deep, so
> no collision — the same argument that puts `generate.md` at the docs root). The next `resource drift` run's
> existing clear-and-rebuild sweeps prompt and report with the observation they belong to — zero new code for
> that path — and a **re-baselining `resource download` clears the `drift/` tree too**: a new baseline
> supersedes the observation by definition (drift is always relative to the baseline), so leaving it on disk
> would preserve an answer to a question nobody can ask any more. Deliberately **no history**: an operator
> who wants to keep a report archives it before the next drift or download run, and the template and README
> say so explicitly.

**Plan.** *(all items below shipped 2026-09-27 and are struck through — kept for the context of the
follow-up; history in `CHANGELOG.md`)*

- ~~**Command** `cmd/docs/analyze_drift.go` (package `docs`, exported `NewAnalyzeDriftCommand`, attached in
  `cmd/docs.go`). Flags: the shared auth group plus `--domain` (offline), `--out`, `--prompt` (template
  override) — reusing `resolveExportDir` from the same package. `cmdutil.BindFlags(cmd)` first in `RunE`.
  Exit codes: 0 success (prompt written, or nothing to analyze); 2 cannot-answer, via
  `cmdutil.WithExitCode`. With this command, `--domain` and `--out` are declared by **three** sibling
  subcommands, so extract a package-local helper in `cmd/docs` that declares them with a parameterised
  `--out` usage string; promotion to the `docs` parent's persistent flags stays off the table while
  `--out`'s default differs per command.~~ *(shipped as `cmd/docs/flags.go` → `addExportFlags`)*
- ~~**Preflight** (all exit 2 with a remediation message): `drift/metadata.yaml` missing → "run
  `azure-rd resource drift` first"; observation **superseded** (its `baseline.generatedAt` differs from the
  export's current `resources/metadata.yaml` `generatedAt`) → re-run `resource drift`; tenant cross-check
  against the resolved domain; every payload the observation names must exist and match its recorded
  `payloadSha256`. Zero findings → report "no drift to analyze", write nothing, exit 0.~~
- ~~**Re-baseline invalidation.** `resource download` clears the tenant's `drift/` tree whenever it writes
  (or updates) `resources/metadata.yaml`, partial `--type`-scoped runs included. Never under `--dry-run`;
  the path is constructed, never derived from input; log the removal when the tree existed. The delete-path
  enumeration in the rules grows from two to three.~~ *(shipped as `drift.ClearTree` +
  `rebaselineClearDrift`)*
- ~~**Engine placement.** In `internal/drift` (`analyzeprompt.go` + `analyzeprompt_render.go`), not
  `internal/docs`. Two exports from `internal/docs`: the splice helpers (`ValidateMarkers`, `SpliceMarker`)
  and a **reference-index facade** over the fact-derivation helpers (one exported type + builder, not six
  exported functions).~~ *(shipped as `docs.ReferenceIndex` in `internal/docs/references.go`)*
- ~~**Template** `internal/drift/analyze_drift_template.md` (go:embed, `--prompt` override, marker
  validation before any write). Marked blocks: **observation**, **worklist**, **refmap**. Prose around the
  markers: ground rules, per-finding procedure, per-verdict guidance, executive summary, report destination
  with self-describing frontmatter and an explicit ephemerality note.~~
- ~~**`--dry-run`**: performs the whole comparison and report, withholds only the write, and warns when an
  `analyze.md` from an earlier run is still on disk.~~
- ~~**Tests**: preflight refusals, zero-findings short-circuit, worklist/refmap rendering, determinism,
  download-side clearing — plus a **new** docs-group flag-surface test mirroring `cmd/resource_test.go`.~~
- ~~**Docs and rules**: README section, output layout, dry-run note; `CHANGELOG.md` under `[Unreleased]`;
  `.windsurf/rules/01-project.md` and `04-security-and-ops.md` updated; `config.example.yaml` untouched.~~

**Follow-up — one drift document per resource, plus `drift/index.md`** *(shipped 2026-09-27, struck through
like the plan above)*. ~~The shipped single `drift/report.md` does not serve the intended frontend: a
side-by-side view needs per-resource artifacts. The YAML side already works — `drift/<key>.yaml` mirrors
`resources/<key>.yaml`, so a baseline-vs-payload diff is implementable from what is on disk today. The
report side must mirror the documentation's one-file-per-resource shape instead of collapsing every finding
into one file:~~

- ~~**Per-finding drift document** `drift/<APIType>/<endpoint>/<name>.md`, beside the payload it judges —
  the same join key as the baseline file (`resources/<key>.yaml`), the payload (`drift/<key>.yaml`) and the
  document (`docs/<key>.md`), differing only in tree root and extension, exactly the derivation rule the
  other trees already follow. Each says **what changed and what the impact is** for that one resource: the
  facts (citing delta paths), who is affected (resolved names), why it matters (security / compliance /
  lifecycle / user-device), severity, and suggested follow-up. The frontend can then render YAML diff and
  drift document side by side per resource. No collision with the payload: same path, different extension.
  For *removed* resources there is no payload, but the drift document still lands at the mirrored path
  (derived from the baseline key), so removals get a document too.~~
- ~~**`drift/index.md`** at the tree root (payloads are ≥ 2 levels deep — the same no-collision argument as
  `metadata.yaml` and `analyze.md`): the drift **summary** as prose — executive summary, findings ordered by
  severity with links to the per-finding documents, and the security / compliance / lifecycle issues as
  prose or tables, whatever fits; plus the not-analyzed caveats (unknown types, unattested entries,
  suppressed removals, missing specs) and the ephemerality note. It replaces `drift/report.md`; nothing
  downstream parses any of it, so the shape inside stays advisory.~~
- ~~**Boundary restated, not relaxed**: the agent writes exactly the per-finding documents named by the
  worklist plus `drift/index.md` — nothing else, still never under `resources/` or `docs/`, still never
  touching `metadata.yaml`, `analyze.md` or the payloads. Ephemerality is unchanged: everything is swept
  with the observation by the next drift run or a re-baselining download.~~
- ~~**Touchpoints**: `analyze_drift_template.md` section 4 rewritten (per-finding destination derived from
  each finding's key, per-file frontmatter carrying `observedAt`/`baselineGeneratedAt`/verdict/severity, the
  index written last from the per-finding results); `drift.ReportFileName` → index name plus a derived
  per-finding report path (exported the way `docPath` is derived, never stored); `AnalyzeResult.ReportPath`
  and the command's closing output updated; README (analyze-drift section, output layout) and
  `04-security-and-ops.md` ("the one file `azure-rd` never produces" becomes the set of agent-written drift
  documents + `drift/index.md`); tests asserting the worklist names each finding's report destination and
  that the template directs the index to the tree root.~~ *(shipped as `drift.ReportPathForKey` +
  `drift.IndexFileName`)*

## 2. Scope the drift analysis like the documentation, and strip template headers from generated prompts

**Goal.** `docs analyze-drift` applies the same documentation scope as `docs generate-prompt`:
`Microsoft.Graph/windowsAutopilotDeviceIdentities` findings are never analyzed, and group findings are
analyzed only when the group is referenced by an assignment. Added and removed resources of the excluded
types still surface in `drift/index.md`'s findings-by-severity table — as tool-fed **inventory rows** with a
fixed `info` severity, no per-finding drift document. Separately, the generated prompts (`docs/generate.md`
and `drift/analyze.md`) no longer carry their templates' explanatory headers.

> **The exclusion mechanism already exists and is engine-side, never template-side.** `inScope` plus the
> `groupsType`/`autopilotIdentitiesType` constants in `internal/docs/generateprompt.go` decide documentation
> scope for both `generate-prompt` and `generate-index`. The drift engine must consume the same decision
> through the exported `docs.ReferenceIndex` facade (a new `InScope(rtype, resourceID)` method backed by the
> same constants and referenced set) — never duplicate the constants in `internal/drift`.
>
> **Scope is a decision, not a fact.** It is applied when the analysis prompt is rendered, never in
> `resource drift` itself: `drift/metadata.yaml` and the payloads keep recording every finding, excluded
> types included, so revising the scope rule never requires re-running drift.
>
> **The referenced-groups set is payload-aware for drift** (user decision): the baseline `referencedGroups`
> set has a blind spot — a drifted policy whose payload newly assigns a group the baseline never referenced.
> Drift unions the baseline set with group IDs harvested from the verified payloads' assignment targets
> (payloads are hash-checked in preflight before rendering, so they are trusted input by then). Verified:
> payloads carry the `assignments` key — they are the same cleaned bytes `AssignmentTargets` facts are
> extracted from in the writer — so the harvest parses `assignments[].target.groupId` from payload YAML.
>
> **`info` is a new severity, reserved for the tool.** The template's severity scale (section 2e) and the
> index frontmatter line (`severities: high n · medium n · low n`) gain `info`; only tool-fed inventory rows
> carry it — the agent never assigns it to an analyzed finding.
>
> **Requiring the `inventory` marker is a behavior change for `--prompt` overrides**: a custom analyze
> template written before this entry fails marker validation (exit 2) until the marker is added. Acceptable
> — the failure is loud and the fix is one marked block — but the `CHANGELOG.md` entry must say so.
>
> **Changed/renamed findings on excluded types are counted, not listed** (user decision): they appear as a
> count under the index's "Not analyzed" caveats, with no table row and no drift document. Only added and
> removed excluded findings become inventory rows. Severity for inventory rows is fixed `info` — the agent
> never analyzed them, so it may not judge them ("never invent").
>
> **An all-excluded observation still writes the prompt** (empty worklist plus inventory), otherwise the
> inventory would be lost: `NothingToAnalyze` keeps meaning zero findings in the observation, not zero
> in-scope findings.
>
> **Header stripping is not regeneration-gated**: neither `generate.md` nor `analyze.md` is hashed. Strip at
> render time — the leading HTML comment block and the literal ` (template)` suffix of the first H1 — via a
> shared helper in `internal/docs`, applied by both engines; a `--prompt` override without a header is a
> no-op.

**Plan.**

- **Facade**: add `InScope(rtype, resourceID string) bool` to `docs.ReferenceIndex`, implemented on the
  existing constants and referenced set; rebase the docs engines' `inScope` calls on the same decision so
  docs and drift can never diverge. Add a constructor variant taking extra referenced group IDs (e.g.
  `NewReferenceIndexWithExtraGroups(m, ids)`) — the plain constructor keeps the docs engines' behaviour —
  and export the assignment-target group-ID extractor (`assignmentGroupIDs` is unexported today) so the
  harvest never re-derives it in `internal/drift`.
- **Payload harvest**: in the drift analysis engine, parse assignment group IDs from the verified payloads
  of added/changed/renamed findings and union them into the referenced set via the facade's extra-IDs
  constructor.
- **Refmap wording**: `renderAnalyzeRefmap` labels groups "referenced by an assignment in the baseline" and
  renders absent ones as dangling — both wrong for a group referenced only by a drifted payload. Relabel the
  list, and render an extra-referenced group with no baseline entry as "new in this observation" (its
  payload, if the group itself drifted, is on disk), never as dangling.
- **Engine scoping**: excluded findings leave the worklist (and get no drift-document destination); their
  added/removed subset feeds a new tool-rendered `inventory` marked block (name, type, verdict, fixed
  `info` severity); their changed/renamed subset becomes counts surfaced in the observation block's caveats.
  `AnalyzeResult` reports both, and the command prints them.
- **Template**: new `inventory` marker (grows `requiredAnalyzeMarkers`); section 5 (index) instructs the
  agent to copy inventory rows into the findings table verbatim — severity `info`, one-liner "inventory
  change — not analyzed", no document link — and the "Not analyzed" part restates the excluded
  changed/renamed count. Section 2e's severity scale and the index frontmatter's severities line gain
  `info` (tool-fed only). Ground rules updated: inventory resources are never analyzed, never get documents.
- **Header stripping**: shared `internal/docs` helper stripping the leading HTML comment and the H1's
  ` (template)` suffix; applied in `GeneratePrompt` and `GenerateAnalyzePrompt` after splicing.
- **Tests**: facade `InScope` parity with the docs engines; payload-union harvest; excluded findings absent
  from the worklist but present as inventory/caveats; all-excluded observation still writes the prompt;
  headers stripped from both generated prompts and `--prompt` override without a header unaffected.
- **Docs**: README (`docs analyze-drift` scope and inventory behaviour; note that generated prompts carry no
  template header), `CHANGELOG.md` under `[Unreleased]` — including the `--prompt` override behavior change
  (new required `inventory` marker); `config.example.yaml` untouched (no new config).

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
