# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work: it is written
and committed here before it is implemented, its plan items are struck through as they land, and **once it is
done it is archived** — moved with its full plan to `../.claude/archive/go/`, so the *how* survives for later
review while `CHANGELOG.md` records the what and why. Ideas that are deliberately not scheduled collect under
*Parked ideas* at the end, so they persist as the entries around them ship. `README.md` stays the single source
of truth for what the tool *does today*.

## 1. Attribute each drift finding to an actor and a time, from the tenant's Log Analytics audit tables

**Goal.** Answer *who changed this, and when* for every drifted resource. A drift observation today says a
resource's bytes moved somewhere between the baseline and the observation; the change record that names the
actor already exists in the operator's Log Analytics workspace, keyed by the same object GUID the finding
carries. Join the two and record the result as a new, separate artifact at the drift tree root — so the
drift-analysis agent, and a human reading the tree, can tell a deliberate administrative change from an
unexplained one.

> **Why a separate `drift/audit.yaml` and not `drift/metadata.yaml`.** Three structural reasons, not
> presentation: (1) **provenance differs** — the observation is computed from bytes this run fetched and can be
> verified against them, while audit rows are copied from an external system with its own ingestion latency,
> retention and completeness, so folding them in makes the observation partly unverifiable; (2) it would
> **break determinism** — `findingsSha256` and the "identical bytes over an unchanged tenant except the
> timestamp" property would be lost, because a re-run sees whatever has been ingested since; (3) it **must be
> allowed to fail** — no workspace, no permission or a window past retention may not invalidate the
> observation, and a separate artifact can simply be absent. Lifecycle is free: the file sits at the drift tree
> root beside `metadata.yaml` and `analyze.md`, where no payload can be, so it is swept by the drift run's
> existing clear-and-rebuild and by a re-baselining `resource download`. **No new delete path.**
>
> **The join key and the window already exist.** `Finding.ResourceID` is the Graph object GUID the audit
> tables record as the target, and `Observation.Baseline.GeneratedAt` → `Observation.ObservedAt` is exactly the
> interval the verdict claims the change happened in — so the query needs no heuristic bounds.
>
> **Facts only, and the gaps are facts too.** Record the actor (UPN / application name), the activity, its
> result, the event timestamp and the correlation id — never a judgment such as "authorized" or "expected".
> Where the join cannot be made the entry says so explicitly with a distinct status, because *no event found*
> and *could not look* must never render as the same thing: singleton types (`organization`,
> `authorizationPolicy`, `deviceManagementSettings`, …) have no GUID target to join on; a window starting
> before the workspace's retention is *unknown*, not *unchanged*; ingestion latency means a change observed
> seconds ago may not be queryable yet; and several events in one window are a list, never a single "who".
>
> **Failure semantics (settled).** The lookup is enrichment and **never fails a run** — in either entry point
> it warns, records the per-finding status, writes whatever it could answer, and leaves the exit code to the
> drift comparison alone. This mirrors how a permission error skips a type instead of failing a download.
>
> **Not regeneration-gated.** It touches no `doc-prompt.md` and no per-type template, so no `promptSha256`
> moves and no documentation regeneration is forced. The drift-analysis template is not hashed either.
>
> **Coverage (settled).** `IntuneAuditLogs` for the Intune/device-management types plus `AuditLogs` (Entra
> directory audit) for conditional access, groups, named locations and the authentication policies. The three
> ARM types are out of scope for this entry and report their status as not queried; `AzureActivity` can be
> added later behind the same per-finding shape.
>
> **Where the workspace is configured (settled), and the ordering that follows.** The workspace is a
> **per-tenant fact**, like the tenant domain itself — not a per-invocation choice — so it is one plain,
> config-only key in **that tenant's own configuration file**. This entry therefore depends on the
> per-tenant configuration profiles planned ahead of it (a config directory of `<domain>.yaml` files selected
> by `--domain`): with profiles, the setting is a single scalar in the file that already describes the
> tenant; without them it would have to be a domain-keyed map inside a shared file, which is strictly worse
> and would have to be migrated afterwards. **Do not implement this entry first.**
>
> Two consequences are deliberate. The key holds the workspace **id** (the GUID `azquery` queries by; a full
> ARM resource id may be accepted as an alternative spelling), never a display name — resolving a name would
> need a subscription, Reader on the workspace's resource group and disambiguation across subscriptions, a
> whole second permission surface to save pasting a GUID once per tenant. And there is deliberately **no
> flag and no `AZURE_RD_*` variable**: both beat the config file in the precedence order, so a value passed or
> exported for one tenant and forgotten would silently apply to the next — and a wrong workspace does not fail
> loudly, it returns no matching rows, which reads as *nobody changed it*. Binding the setting to the tenant's
> own profile makes that mistake unrepresentable.

**Plan.**

- Add an `internal/audit` engine (imported by `internal/drift`, keeping the dependency direction that already
  holds for `drift` → `docs`) that takes an `Observation` plus a workspace id and returns one attribution per
  finding. Query `IntuneAuditLogs` and `AuditLogs` through `sdk/monitor/query/azquery` (new direct dependency,
  added with `make deps`) over the observation's window, and map both schemas onto **one** finding-shaped
  record — the two tables' field names must not leak into the artifact.
- Route each finding to a table by its `APIType`/resource type, not by trying both: Intune types to
  `IntuneAuditLogs` (joining `Properties.TargetObjectIds` against the finding's `resourceId`), Entra types to
  `AuditLogs` (joining `TargetResources[].id`), everything else straight to a stated not-queried status.
- Batch the lookup: one query per table for the whole finding set (a GUID `in (…)` set), not one query per
  finding — a drift observation can carry hundreds.
- Write `drift/audit.yaml` atomically, last, the way `WriteObservation` writes the observation. It is
  self-describing: a schema `version:` from day one (the field `drift/metadata.yaml` lacks and `index.yaml`
  learned to need), the workspace queried, the exact window, the tables consulted, the `observedAt` and
  baseline `generatedAt` it belongs to, and a per-finding status of `matched` / `no-event-in-window` /
  `no-join-key` / `retention-exceeded` / `query-failed` / `not-queried`.
- Refuse to attribute a superseded observation: reuse the `docs analyze-drift` preflight rule (baseline
  `generatedAt` mismatch) and the tenant cross-check, so an audit file can never describe an observation the
  current baseline has replaced.
- Expose it twice, over the one engine: a standalone `azure-rd resource audit` that enriches the observation
  already on disk (re-runnable without re-fetching the tenant — the usual case, since ingestion lags), and a
  drift run that calls the same engine after comparing. Per the option model established by the
  configuration entry above, the latter is enabled by a config key, not a flag — it changes what a run
  produces — while the standalone command remains the explicit, visible entry point. Both honour `--dry-run`
  by withholding only the write and saying an earlier `audit.yaml` was not refreshed.
- Add one config-only option, `audit-workspace-id:`, read from the tenant's configuration profile and
  registered on the tenant-scoped side of the key partition (so it is rejected in a base file). Empty or
  absent means the lookup is off and every finding's status says so — never a silent no-op. Add it to
  `config.example.domain.yaml` **empty and commented**, preserving that file's no-op promise.
- Record the workspace actually queried in `drift/audit.yaml`, so an attribution can always be traced back to
  the source it came from and a profile mix-up is visible after the fact rather than only at the time.
- Note the new audience in the auth surface: the workspace token is issued for `api.loganalytics.io`, so the
  operator needs *Log Analytics Reader* on the workspace, and the dedicated-app path needs the Log Analytics
  API permission added. Detect a missing permission through `azure.IsPermissionError` and degrade to
  `query-failed` with a warning naming the grant required.
- Splice the attribution into `docs analyze-drift`: each finding's section gains the actor and timestamp when
  one is known, and an explicit "attribution unavailable (<status>)" line when it is not, so the analysis
  agent can weigh a change against who made it instead of reading it as anonymous.
- Tests: schema mapping for both tables from recorded fixtures (no network), routing by resource type, the
  batching query construction, the preflight refusals, every status path including retention and permission
  failure, determinism of the written bytes for a fixed input, and the dry-run withholding. Cover the
  off-by-default case (no workspace configured) reporting a stated status rather than an empty result. Extend
  the drift command's flag-surface test so `--audit` is offered only where honoured.
- Documentation: a README section on attributing drift (prerequisites — Intune and Entra diagnostic settings
  shipping to one workspace, the RBAC grant — the two entry points, the profile key that enables it, the
  artifact and its statuses, and the ephemerality it shares with the rest of the drift tree), plus the
  output-layout list gaining `drift/audit.yaml`;
  `CHANGELOG.md` under `[Unreleased]`; and the drift-tree lifecycle rule in
  `.windsurf/rules/04-security-and-ops.md` naming the new root file as swept, never pruned.

## 2. Enforce the branch gate on GitHub

**Goal.** Make the branch gate unbypassable: a pull request into `main` cannot be merged until
`make branch-ready-go` has passed on it. Everything local — the start gate, `branch-ready`, the archive
checks — is a report a `git commit` by hand can walk past; branch protection with a required status check is
the only layer that actually gates.

> **Scope.** A GitHub Actions workflow at the repository root running `make branch-ready-go` on every pull
> request into `main` (Go per `go.mod`, golangci-lint v2 pinned at the version used locally, the pull
> request's branch checked out by name with full history so `git rev-parse --abbrev-ref HEAD`,
> `git merge-base` and `git describe` work), and branch protection on `main` requiring it. The workflow has
> no `paths:` filter: a required check that never reports blocks the merge for good, and the gate already
> skips its diff checks when `go/` is unchanged; running both pipelines on every pull request costs a few
> minutes and is accepted. Fork pull requests are out of scope (single-maintainer repository). The web half
> is the mirror entry in `../web/NEXT-ITERATIONS.md`; one workflow file carries both jobs, and this entry
> creates it. A `pre-commit` hook refusing commits on `main` is deliberately not part of this: the release
> flow commits on `main`.
>
> **Not regeneration-gated.**

**Plan.**

- ~~Confirm in the runner that the gate needs no script change. The clean-tree preflight must see a fresh
  `go/` (`make ci` writes only the gitignored `azure-rd`, after the preflight). The branch check must read
  the pull request's branch name. The merge-base must resolve through the existing `origin/$RELEASE_BRANCH`
  fallback, and `git describe --match 'go/v*'` must find the tags. Verify this on this branch's own pull
  request, where `branch-ready-go` must report and pass. Only if one of these fails, fix
  `scripts/branch-ready.sh` and cover the fix in `scripts/lib/changelog_test.sh`.~~
- Enable branch protection on `main` requiring the `branch-ready-go` and `branch-ready-web` status checks and
  requiring branches to be up to date before merging (the jobs gate the pull request's head, not the merge
  result). This is a repository setting done by hand; leave the bullet unstruck until the user confirms it.
- ~~Run the `go` job only when the pull request touches something its gate judges. A first job `changes`
  (checkout with full history, `git diff --name-only origin/<base>...HEAD`) exposes the outputs `go` and
  `web`; the `go` job takes `needs: changes` and `if: needs.changes.outputs.go == 'true'`, where `go` is true
  for any change under `go/`, `.claude/archive/go/`, the root `Makefile` (it defines the gate targets) or the
  workflow file itself. Job-level conditions, not a `paths:` filter on the trigger: a skipped job counts as
  passed for a required status check and so keeps reporting. Sources alone would be too narrow — the gate
  also checks the backlog, the changelog, the archive and the commit subjects — so every file under `go/`
  counts. Cover the rule with a comment in the workflow naming both lists.~~

## 3. Split CI into pipeline checks and the close report

**Goal.** Make GitHub Actions the quality authority and keep the local cycle fast. The pipeline — format,
lint, tests, build — runs on every push so a red result arrives while the work is still in hand, and the
branch report — strikeouts, numbering, changelog, archive, commit subjects — runs on the pull request in
seconds as the merge gate. Locally, implementation runs build and tests before anything is committed, the
review and fix agents no longer run lint themselves, and closing a branch waits for CI instead of repeating
the pipeline.

> **Why.** `branch-ready` bundles two things with different rhythms. The pipeline belongs on every push;
> the close report is only meaningful at the end and is *expected* to be red while entries are still struck.
> Running both as one required check makes the pull request red for most of its life and makes the local
> gate slow. Splitting them keeps every check reporting (skipped jobs count as passed), keeps the merge gate
> strict, and lets the local tooling skip what CI already proves.
>
> **Scope.** The workflow and the Makefile targets. `make branch-ready` with the full pipeline stays for
> offline use. The pipeline agents and the skills change alongside (Claude setup, no entry).
>
> **Not regeneration-gated.**

**Plan.**

- ~~`Makefile`: a `branch-ready-report` target running only the clean-tree preflight and `scripts/branch-ready.sh`
  (no `ci`), with the `help` line; root `Makefile` gains `branch-ready-report-go`, `-web` and
  `branch-ready-report` beside the full targets, and the comments say which is the local full gate and which
  the CI merge gate.~~
- ~~`.github/workflows/branch-ready.yml`: trigger on `push` (every branch) and `pull_request` into `main`. The
  `changes` job diffs against `origin/<base>` where `<base>` is the pull request's base branch or `main` on a
  push. Job `ci-go` (`name: ci-go`, on both events, `if` go changed): checkout by name with full history,
  Go from `go.mod`, golangci-lint v2.11.4 via the action pointed at `go/`, then `make -C go ci` with
  `RELEASE_BRANCH` set. Job `branch-ready-go` (pull requests only, `if` go changed): checkout only, then
  `make branch-ready-report-go` with `RELEASE_BRANCH: ${{ github.base_ref }}`. The header comment states the
  two rhythms and that branch protection requires `ci-go`, `ci-web`, `branch-ready-go`, `branch-ready-web`.~~
- Verify on this branch's pull request: `ci-go` green on push and on the pull request; `branch-ready-go` red on
  the strikeout check only while entries are struck, green after the close.
- Branch protection on `main` requires all four checks (by hand; replaces the two-check setting of the
  previous entry). Leave unstruck until the user confirms.
- `CHANGELOG.md` under `[Unreleased]` (amending the entry about the GitHub gate); `README.md` Development
  section naming `make branch-ready-report` and the two CI jobs; root `README.md` step 3.

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are archived. Each records why it is parked and what would make it worth doing.

### Idea: emit `summary:` in the generated document frontmatter

The prompt template's *Frontmatter (required)* section lists `source`, `sourceSha256`, `promptSha256`,
`platformGroup`, `functionGroup` and `generatedAt` — and never asks for `summary:`. The plumbing on both
sides is complete: `docFrontmatter` has a `Summary` field, `GenerateIndex` copies it into each `index.yaml`
resource, and the browser renders it as per-item context when present — so it is absent from every
document by construction, not by model behaviour, and the web idea *a name filter and per-item context in
the sidebar* is blocked on this one template line. **Not planned — parked deliberately**, because the change
is **regeneration-gated**: the line itself rides `generate_prompt_template.md`, which is not hashed, but no
existing document carries the field, so filling it means regenerating every document anyway. It must not
ship alone.

**Revisit when** a documentation regeneration is scheduled for another reason; fold it in with the other
regeneration-gated ideas below (per-finding severity, taxonomy bootstrap) so they share one regeneration.
When promoted: one frontmatter line plus its rule text in the template, and a note that `platformGroup` /
`functionGroup` — already required by the template but empty in the reference exports, which predate it —
fill in on the same regeneration with no further change.

### Idea: version the drift observation, and name `drift/` a Go → web contract

`drift/metadata.yaml`, the payloads at `drift/<key>.yaml`, the analysis prompt and the agent-written
`drift/<key>.md` and `drift/index.md` are read by the browser, joined by the shared `<type>/<name>` key,
gated on `baseline.generatedAt` and verified by hash — exactly the shape of the `index.yaml` contract, with
the same obligations: a change to the observation schema, to the per-finding frontmatter (`verdict`,
`severity`, `observedAt`, `baselineGeneratedAt`) or to the index's `severities:` line is a cross-project
change. The observation carries a `toolVersion` but no schema `version:` — the field `index.yaml` learned to
need. **Not planned — parked deliberately**: nothing has broken, and adding the field alone is cheap but
pointless until a consumer branches on it.

**Revisit when** the observation schema or the per-finding frontmatter next changes for another reason: add
`version: 1` to `drift/metadata.yaml` in that same change, have the browser accept `>= 1`, and name `drift/`
beside `index.yaml` in both projects' rules as a versioned contract. Related, web-only: the drift index
table uses `high / medium / low / info` while the tenant summary's Findings table uses
`critical / high / medium` (see the web backlog's *Fixes*).

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
  not change in the tenant, which reads as mass drift and forces documentation regeneration. `resource drift`
  is the verification tool: "update, run drift against a fixture tenant, it must report zero findings" is the
  standard check, so a bump is provable content-neutral before it lands.
- **Moving the seven v1.0 types to beta trades a stability contract for uniformity.** v1.0 responses are
  contractually stable; beta responses may change shape at Microsoft's discretion. Today the most stable,
  most-referenced types (groups, conditional access) deliberately sit on the stable endpoint. Consolidation
  buys shorter builds but makes every type's bytes hostage to beta churn.
- **The switch itself moves hashes once.** Re-fetching those seven types through the beta endpoint will change
  their YAML (beta models carry extra properties), so their `sourceSha256` values move and their documents
  regenerate — a one-time cost that should ride a regeneration scheduled for another reason, not force its own.

**Revisit when** a security advisory forces an SDK bump anyway, or build time becomes a felt cost. If
consolidation is picked up:
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

### Idea: `resource compare` — an offline comparison of two exports, and the home of the cross-tenant identity rule

Compare two tenants' exports on disk — stage against prod — the way `resource drift` compares one tenant
against its own export: a verdict per resource key (only in A, only in B, same, different), dotted-path deltas
for the different ones, payloads, and an `analyze.md` so the drift-analysis agent can judge the **impact** of
each difference (security posture, compliance, lifecycle, who is affected) rather than only that bytes differ.
No Azure call: both inputs are export trees, so the command is offline like `docs analyze-drift`.

**Why this exists as a Go idea now.** The web project has shipped a **web-only proof of concept** of exactly
this comparison (the docs browser's tenant compare, described in `web/README.md`: two-click tenant
selection on the picker, a listing from the two `resources/metadata.yaml` files, and a per-resource YAML diff).
To make equal configuration compare equal across tenants, that PoC **normalises tenant-local identity in the browser**:
drops every `id` or `sourceId` at any depth whose value contains a GUID (ids without one — settings
ordinals, `all_users`, authentication method names, the all-zero sentinels — are content and stay), every
`*@odata.context` key at any depth, `createdDateTime`, `lastModifiedDateTime`, `version` and the group
identity fields (`mail`, `mailNickname`, `proxyAddresses`, `securityIdentifier`, `renewedDateTime`), and
resolves `assignments[].target.groupId`, `assignments[].target.deviceAndAppManagementAssignmentFilterId` and
`notificationTemplateId` to the display name of the matching `resourceId` in the same tenant's
`metadata.yaml` — a set-valued lookup, since neither `resourceId` nor `displayName` is unique in real exports:
a reference resolves only when every entry with that id agrees on the name, otherwise the GUID stays and is
flagged ambiguous or unresolved, and a zero-sentinel reference means *none* and passes through. Measured
against the two reference exports, that rule leaves 19 of 114 paired resources identical, 36 once
`assignments` is removed as well — the difference being the same policy targeting a differently named group
in each tenant, which the web diff page reports as *differs only in audience*. That rule is a **judgment,
not a fact**, and it was accepted on the web side under one condition, recorded there: it is provisional and
**moves to the CLI once it is stable**. The PoC's job is to find out what the rule is against real pairs;
this idea's job is to receive it. Until then the browser computes what the CLI should be emitting — the same
disagreement risk the taxonomy rules exist to prevent (a rule derived in one consumer can disagree with every
other), tolerated here only because there is no other consumer yet.

**Not planned — parked deliberately**, until the PoC has produced a rule that can be *stated and tested* —
a fixed drop list plus a reference-resolution table per field, with fixtures from a real stage/prod pair —
rather than guessed per resource type. Promoting it before that would freeze a guess into a contract the
browser then depends on.

**Shape when promoted.**

- **Engine** beside the drift engine (`internal/drift` or a sibling `internal/compare`), reusing the verdict
  and delta machinery over normalised documents; the normalisation is one exported, table-driven, unit-tested
  function — the *single* truth the browser stops duplicating.
- **Command** `azure-rd resource compare` taking two export domains (both resolved the way `--domain` is
  today), offline, `--dry-run` withholding only the writes like `resource drift`.
- **Output tree — placement is the open design question.** Every tree today lives under `<output>/<tenant>/`
  and a comparison belongs to neither tenant. Candidates: a sibling root `<output>/compare/<a>__<b>/`, or
  under the left tenant `<output>/<a>/compare/<b>/`. Whichever is chosen, the tree mirrors `resources/` keys
  the way `drift/` does (metadata at the root, payloads ≥ 2 levels deep, agent-written documents beside them),
  is cleared and rebuilt per run with no history, and carries a schema `version:` from day one — the field
  `drift/metadata.yaml` lacks and `index.yaml` learned to need.
- **Analysis prompt** as a third template beside `generate_prompt_template.md` and
  `analyze_drift_template.md`, with `doc-prompt.md` as the per-type lens again — **not regeneration-gated**,
  moving no `promptSha256`.
- The browser then renders the tree as it renders `drift/` and deletes its own normaliser.

**Relation to the idea above.** This does **not** trigger *resolve Graph object ids to names inside the
exported YAML*: the comparison reads names from `metadata.yaml`, which is exactly the consumer that idea says
does not need them in the YAML. The facts stay facts.

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
