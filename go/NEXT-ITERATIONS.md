# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## ~~1. Regroup the resource-facing commands under one `resource` noun~~

**Goal.** Give the commands that act on a tenant's resources a single home: `azure-rd download` becomes
`azure-rd resource download` and `azure-rd list` becomes `azure-rd resource list`. Both move unchanged in
behaviour; what changes is where their shared flags live — declared once on the `resource` parent, so every
resource verb, present and future, inherits one definition of `--subscription`, `--type`, `--workers` and the
rest instead of opting into them command by command.

> **Why a noun group.** `docs generate-prompt` / `docs generate-index` already establishes noun-plus-verb, so a
> flat `download` and `list` beside it are the odd ones out. Grouping also puts the resource commands in one
> package, where anything they share is a direct call rather than a contortion across parent and child, and
> gives the shared flags somewhere to live.
>
> **A flag goes on the parent only once every subcommand under it honours the flag.** Persistent flags were
> deliberately narrowed before, because `list` advertised `--type` and `--resource-group` and silently ignored
> them; hoisting flags to a group parent is the same mechanism and can reintroduce the same defect. So the
> parent starts with what all of its children genuinely honour — the authentication flags — and each later verb
> promotes the groups it makes universal as it lands. The end state, once the group holds listing, downloading
> and drift detection, is that authentication, selection and pipeline tuning are all parent-level; getting there
> by promotion rather than up front is what keeps the promise that a flag a command offers is a flag it obeys.
> Root's four globals (`--config`, `--output`, `--dry-run`, `--log-level`) are unaffected.
>
> **The binding helper has to follow the flags.** Binding visits the command's *local* flags, which excludes
> anything inherited from a parent — so a flag moved onto `resource` would silently stop being bound to Viper
> and lose its env-var and config-file precedence, with only the flag itself still working. Moving flags up and
> leaving the helper as it is would break configuration quietly, in the one direction nobody tests.
>
> **It is a breaking change, deliberately unmitigated.** The old spellings are not kept as hidden aliases:
> carrying two names for each command, and the code that maps them, costs more than the one-time break. Record
> the old-to-new mapping under `Breaking` instead.

**Plan.**

- ~~Add a `resource` parent command in package `cmd`, mirroring `docs`, and move `download` and `list` under it as
  `resource download` and `resource list`, behaviour unchanged. Each subcommand lives in `cmd/resource/` as its
  own package exposing a constructor the parent attaches, so it never imports package `cmd`.~~
- ~~Declare the shared flags once, as persistent flags on the `resource` parent, starting with the authentication
  group that both subcommands already honour. Selection and pipeline tuning stay on `resource download` until a
  sibling genuinely honours them, at which point that work promotes them. Command-specific switches
  (`--resolve-secrets`, `--no-prompt`, `--prune`) stay on the subcommand that implements them, always.~~
- ~~Prove the `client-id`/`tenant-id` required-together pairing still fails on the subcommands once the flags are
  inherited — Cobra's flag-group validation has historically been unreliable across the parent/child boundary.
  Add a regression test (`resource download --client-id` without `--tenant-id` must error), and if Cobra does
  not enforce the pairing on children, enforce it in the parent's persistent pre-run instead of dropping it.~~
  Cobra does not enforce it on children, so `cmdutil.RequireAuthFlagPair` enforces it in the group's persistent
  pre-run.
- ~~Keep `internal/cmdutil` the single place the flag groups are defined, and teach its helpers to register on a
  persistent flag set as well as a local one — one definition of names, defaults and usage strings, with the
  caller choosing the target — so parent-level and command-level registration cannot drift into two spellings of
  the same flag.~~
- ~~Leave root's own local copy of the auth flags in place: `azure-rd --debug` needs them, and they are
  deliberately local so other top-level commands do not inherit them. After this change the auth flags are
  registered twice — locally on root, persistently on `resource` — which is intentional and must be commented as
  such at both sites, because it reads like an oversight.~~
- ~~Make the Viper binding helper bind inherited flags as well as local ones, keeping it per-execution so a
  sibling command's identically named flag still cannot be picked up. Add a test that a parent-declared flag
  resolves in the documented order — flag, then `AZURE_RD_*` env var, then config file, then default — because
  this is exactly the breakage that would otherwise pass unnoticed.~~
- ~~Keep `resource list` offline and subscription-free exactly as it is today, including the lazy credential that
  lets it work without a signed-in session.~~
- ~~Break the one hidden dependency on package `cmd` that the move exposes: the tool-version string recorded in
  export metadata is derived from the root command's `Version`, which a subcommand package cannot reach. Move
  the version into a shared package that both the root command and the moved code read, so it stays a single
  source of truth.~~ Now `internal/version`; the Makefile's `-ldflags -X` target moved with it.
- ~~Carry the existing download and list tests over to the new packages unchanged in intent.~~
- ~~Update `README.md` (every command invocation, including the examples), the rules' command-recipe section, and
  any config documentation that names a command. Record the rename in `CHANGELOG.md` under `Breaking` with the
  old-to-new mapping spelled out.~~

## 2. Separate enumerating what the tool handles from what the tenant contains

**Goal.** Split the overloaded listing command in two. `resource types` answers "what can this binary handle?" —
the handler behind each Azure type and the API it speaks — and, whenever a usable session happens to be
available, enriches that map with how many of each the tenant holds. That frees `resource list` for its obvious
meaning: what the tenant actually contains, per resource, without downloading anything.

> **Prerequisite.** This assumes the resource-facing commands live under a `resource` parent, each subcommand
> its own package under `cmd/resource/`, with the flags they share declared once on the parent.
>
> **Why `types` and not `list` for the offline map.** That command enumerates the resource types this binary can
> handle — a property of the build, answerable without a subscription or a session — not anything in a tenant.
> Naming it `types` says so, and stops a bare `list` next to a grouped noun from inviting "list what?".
>
> **`resource list` keeps its spelling and changes its meaning, which is the more dangerous break.** A caller
> that kept working after the regrouping now gets the tenant's resources where it used to get the supported
> types — no error, just different output. Record it under `Breaking` in those terms and name `resource types`
> as the replacement; a rename that errors is kinder than a silent substitution, so this one has to be stated
> loudly.
>
> **The two questions have different dependencies, which is why they are different commands.** Supported types
> are a property of the build: no subscription, no sign-in, no network, and the answer is identical on every
> machine running the same binary. A tenant's contents need authentication, a listing call per type, and the
> answer differs per tenant and per minute. Collapsing them means the offline question inherits the online one's
> failure modes.
>
> **`resource list` must not become a second implementation of listing.** The download command already
> enumerates exactly this set to build its fetch requests, and reports it under `--dry-run`. The new command has
> to share that one path, so the two can never disagree about what is in scope; they differ only in framing —
> "what would this download write" versus "what is there".
>
> **The counts are one dataset in two presentations.** A per-type count is exactly the roll-up of what
> `resource list` enumerates, so it must come from the same listing call rendered differently — a per-type view
> versus a per-resource view — never from a second counting path.
>
> **Whether the counts appear is decided by the session, not by a flag.** If the tool can authenticate, it
> counts; if it cannot, it prints the type map alone. No opt-in switch, and explicitly not `--dry-run`, which
> means "write nothing" everywhere in this tool and must never change what is *reported* — the same flag has to
> keep meaning the same thing here as it does for a download or a drift check. Failing to authenticate is not an
> error for this command: the offline answer is complete and correct on its own terms, so it degrades with a
> note rather than a non-zero exit.
>
> **It must never prompt to obtain a session.** Counting is a convenience, so a credential that would require
> interaction — a device-code sign-in, a browser — counts as "not available" and yields the offline map. A
> command whose plain form is documentation of the binary must not be able to block on a login prompt.
>
> **The output must say which of the two it is.** Because the same invocation now prints different things on
> different machines, it has to state that counts were omitted and why, rather than leaving an operator to infer
> from absence that the tenant is empty. For the same reason the type map's own columns stay identical in both
> modes: the count is an added column, never a different layout.
>
> **An unlistable type has no count, and zero is not it.** A type whose listing was refused must render as
> unknown, distinct from a type that listed to zero resources. Showing `0` for a missing permission is the same
> error that would make `--prune` delete a live file, in a place where it merely misinforms.
>
> **Listing yields resource ids, not names.** Display names come from a fetch and a transform, so a listing
> alone cannot show them. Enriching from an existing export's `resources/metadata.yaml` is legitimate (it is
> already-recorded fact); fetching resources merely to prettify a listing is not.

**Plan.**

- Rename the moved supported-types command to `resource types`, and extend its output from a flat list of Azure
  types to the mapping an operator actually wants: each supported Azure type, the handler that implements it and
  the API it speaks, grouped so the ARM and Microsoft Graph surfaces are distinguishable at a glance. The map
  itself remains answerable with no session and no subscription, as it is today.
- Derive the handler identity from what the registry already knows rather than widening `ResourceHandler`; a new
  interface method would have to be implemented by every handler, which is a high price for a listing command.
  Only add one if deriving it proves genuinely unreadable.
- Append each type's resource count in the tenant whenever a session is available, rendered as the per-type
  roll-up of the same listing the other resource commands use. Decide it by attempting authentication
  non-interactively: a usable session counts, anything that would prompt or fail degrades to the type map plus a
  note saying counts were omitted and why. Never fail the command over it. Concretely: the Azure CLI credential
  already fails fast when no `az login` session exists, but the device-code credential *starts the sign-in flow
  inside `GetToken`* — exactly the prompt the invariant forbids — so construct it with azidentity's option that
  disables automatic authentication, and treat the resulting authentication-required error as "no session".
  Cover the would-prompt path with a fake credential in a unit test; a signed-in developer machine never
  exercises it, so without that test the blocking behaviour ships unnoticed.
- Add `resource list`, enumerating the tenant's resources for the selected types. With no selection it covers
  every registered type, exactly as a full download would.
- Route both through the same listing path the download command uses to build its fetch requests, so scope,
  filters and the treatment of unlistable types are shared code rather than a parallel implementation.
- Promote the selection flags — and the pipeline flags that genuinely affect listing concurrency — from
  `resource download` to the `resource` parent, now that every subcommand under it honours them: `resource types`
  filters its map by type offline and narrows what it counts online, and `resource list` selects what it
  enumerates. Counting or listing every registered type is the expensive half of a download's listing phase, so
  an operator narrowing the scope should pay only for that scope.
- Report per type: the resources found and their ids, and — when an export for the tenant exists — the display
  names joined from its `resources/metadata.yaml`, marking resources not present in the export as new. Never
  fetch to obtain a name.
- Keep the coverage distinction the rest of the tool makes in both commands: a type that could not be listed is
  reported as unknown and excluded from the counts, never conflated with a type that listed to zero resources,
  and never rendered as `0`.
- Write nothing at all in either command. Because neither has an output artifact, `--dry-run` is a no-op for
  them; say so rather than inventing a file for the flag to withhold.
- Exit zero even when some types are unknown or the counts were omitted, consistent with the rest of the tool:
  missing permissions and missing sessions warn, they never fail a run. Non-zero exits are reserved for real
  errors — an unreachable API mid-listing, not an absent privilege.
- Add tests for both commands, document them in `README.md` with example output, and record them in
  `CHANGELOG.md` — the new command under `Added`, the enriched type map under `Changed`, and the repurposing of
  the `list` spelling under `Breaking`.

## 3. Detect drift between the tenant and the export on disk

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
