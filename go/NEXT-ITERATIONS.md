# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. Detect drift between the tenant and the export on disk

**Goal.** Answer "has this tenant changed since the last download?" without re-baselining it. A `resource drift`
command fetches the tenant's current state, compares it against the export already on disk, and records what was
added, changed, renamed or removed as a durable per-tenant artifact — so an operator can see drift before
deciding to re-download, CI can gate on it, and the frontend can show which documented resources have moved on
in the tenant.

> **Prerequisite.** This assumes the resource-facing commands live under a `resource` parent (with the download
> command as `resource download`), each subcommand its own package under `cmd/resource/`, and the flags they
> share — authentication, selection, pipeline tuning — declared once on that parent. If that grouping is not in
> place yet, doing it is part of this work: drift is a *sibling* of downloading, not a mode of it — the two do
> the same list, fetch and transform work and differ only in what they do with the resulting bytes — so
> `download drift` would misrepresent it.
>
> **Why not an output-path override or a second copy.** The export on disk *is* the baseline:
> `resources/metadata.yaml` already records `sourceSha256` per resource, plus the `resourceId`,
> `presentInTenant`, `filtered`/`skipped` flags and per-type coverage needed to decide every verdict. Storing a
> second dated copy of a tenant would duplicate a baseline that already exists, and naming an export folder
> anything other than the tenant domain breaks the `metadata.tenant` cross-check and single-export detection
> that `docs generate-prompt` and `docs generate-index` rely on. The live export stays the one export per
> tenant.
>
> **Comparability is a precondition, not a detail.** `run.transformConfigSha256` exists precisely so a mass
> hash movement can be attributed to a config change rather than a mass edit. Comparing across a different
> transformer configuration, a different `resolveSecrets` setting or a different run scope makes every resource
> look drifted, so the command must refuse rather than report noise.
>
> **`--dry-run` governs writes, not fetches.** Unlike a download — which may skip the fetch under `--dry-run`
> because part of its question is answerable offline — nothing about drift can be answered without the tenant's
> current bytes. So a dry run still fetches and still reports in full; it only withholds the artifact, and says
> that an artifact from an earlier run is still on disk and was not refreshed.
>
> **The artifact lives in neither owned tree.** `<tenant>/drift.yaml` sits beside `resources/` and `docs/`
> because a drift record is neither a resource nor a document: it is an observation *about* the export. That
> keeps `resources/` write-exclusive to the download itself and prune territory, and leaves the set of files
> `azure-rd` writes under `docs/` closed. Nothing deletes it — `--prune` remains the only delete path — so a
> re-baseline leaves it behind, to be recognised as stale by the baseline it names rather than removed.
>
> **Scope: inline base64 mode only.** With `base64-decode` in its default `inline` mode the decoded payload
> stays in the resource YAML, so `sourceSha256` covers a resource's full content and no per-artifact fact is
> needed — meaning **no re-download is required to start using the command**. Under `mode: file` with
> `remove-source: true` the content lives only in the sidecar, which no recorded fact covers, so artifact
> content drift is undetectable there and the command must say so instead of under-reporting.
>
> **Not regeneration-gated.** It touches no documentation template and moves no `promptSha256`, so it can ship
> on its own without a documentation regeneration.

**Plan.**

- Add `resource drift` beside `resource download`. It needs no flag registration of its own beyond the shared
  parent-level groups and its own switches, and binds flags per-execution before reading any value, like every
  other command.
- Extract the run preparation the two commands share, currently inline in the download command, into its own
  package: config reading, worker/transformer/filter construction, the offline probe registry and dedicated-app
  prompt, authentication, export-directory and tenant resolution, the real registry and the fetch requests,
  returning a prepared run. Keep `internal/cmdutil` to flag groups and prompts. The extraction earns its keep
  here, with a second consumer: it is what makes the two commands incapable of diverging in auth or selection
  semantics.
- Keep `resource drift` read-only with respect to the export: it writes no resource file, never updates
  `metadata.yaml`, never touches `docs/` and never prunes. Its single output is the drift artifact.
  Re-baselining is a normal `resource download`.
- Resolve the export directory and cross-check it against `metadata.tenant` the way the `docs` subcommands do,
  including the offline `--domain` escape, so drift cannot be reported against the wrong tenant.
- Run a comparability preflight before fetching anything: refuse (distinct "cannot answer" exit code) when
  there is no baseline, or when the baseline's transform-config hash, `resolveSecrets` or run scope differ from
  the current run. Warn once when the effective config is `base64-decode` in `file` mode with `remove-source`,
  stating that artifact content drift is not detected in that configuration.
- Reuse the existing pipeline unchanged (list → fetch → transform) and compare instead of writing, hashing the
  same marshalled bytes the writer hashes so a verdict can never disagree with what a download would record.
  Preserve the accounting invariant: every request still produces exactly one result.
- Decide verdicts from `metadata.yaml` alone — unchanged / changed / added / removed / renamed — matching on
  `resourceId` first so a renamed resource is reported as a rename rather than an add plus a remove, and
  excluding entries that were filtered, permission-skipped or already known absent.
- Gate removals on the same rule as `--prune`: only assert that a resource is gone when the run is `Complete`
  and its type was actually covered. Report types that could not be listed as *unknown*, excluded from the
  totals, and state explicitly when removals were suppressed.
- Persist one artifact per tenant, `<output>/<tenant>/drift.yaml`, overwritten on every run and withheld
  entirely under `--dry-run`. It records facts only — the verdicts and field deltas — never a severity,
  classification or other revisable judgement, so revising how drift is presented never requires re-running a
  check.
- Make the artifact self-describing about what it measured: the baseline it was compared against (the export's
  `generatedAt`, tool version and transform-config hash), the tenant, run completeness, the verdict counts and
  the types whose drift is unknown. A consumer must be able to tell that a re-download has invalidated it
  instead of silently trusting a record of a superseded baseline.
- Key each finding the way `docs generate-index` keys a resource, and carry the derived document path, so a
  drift finding joins to its document and to `index.yaml` with no extra wiring on the consuming side.
- Keep the artifact deterministic apart from the moment of observation: hash the findings alone into a
  `findingsSha256`, so re-running over an unchanged tenant produces identical bytes except the timestamp and
  "the same drift as last time" is a single comparison. This is also what would let an append-only history
  deduplicate, if one is ever added.
- Report the same information on the console — a summary (compared / unchanged / changed / added / removed /
  renamed / unknown / failed) plus a verdict line — with exit codes mirroring the existing commands: success
  whether or not drift was found, an opt-in `--exit-code` for CI gating on drift, "cannot answer" for an
  unanswerable run, and failure only when resources failed to fetch.
- Read the on-disk resource YAML for changed resources only, to record and print dotted-path `old → new` field
  deltas — a detail tier layered on top of the metadata-only verdicts, so the cheap path needs no file reads.
  Truncate long values; keep whole-document output behind `--log-level debug`.
- Close the loop with the documentation pipeline by reporting how many documents the observed drift will make
  stale once re-baselined, pointing at `docs generate-prompt`.
- Deferred, deliberately: an append-only drift history (a timeline of checks rather than the latest one).
  Unbounded growth needs a retention policy, and there is no delete path in the tool besides `--prune`, so it
  waits until a consumer actually needs a timeline rather than "has it drifted since the export?".
- Cover with unit tests over a fixture export: each verdict, rename matching, removal suppression on an
  incomplete run, the comparability refusals, that `--dry-run` writes nothing, and that the artifact is
  byte-identical over an unchanged tenant apart from its timestamp. Cover the extracted run preparation where
  the moved code was covered before.
- Document the command and its artifact in `README.md` (usage, the drift artifact's shape and purpose, and the
  output layout gaining a file at the tenant root), name the new file in the rules' output-layout section, and
  record the capability in `CHANGELOG.md`.

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
