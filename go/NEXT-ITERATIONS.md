# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work: it is written
and committed here before it is implemented, its plan items are struck through as they land, and **once it is
done it is archived** — moved with its full plan to `../.claude/archive/go/`, so the *how* survives for later
review while `CHANGELOG.md` records the what and why. Ideas that are deliberately not scheduled collect under
*Parked ideas* at the end, so they persist as the entries around them ship. `README.md` stays the single source
of truth for what the tool *does today*.

## 1. Attribute each drift finding to an actor and a time, from the tenant's Log Analytics audit tables

**Goal.** Answer *who changed this, and when* for every finding of a drift observation. `drift/metadata.yaml`
says a resource's bytes moved between the baseline and the observation; the change record naming the actor
already exists in the operator's Log Analytics workspace, keyed by the same object GUID the finding carries
and bounded by the same two timestamps. Join the two and record the result as a separate artifact,
`drift/audit.yaml`, so the drift-analysis agent — and a human reading the tree — can weigh a change against
who made it instead of reading it as anonymous. Attribution is enrichment: it never changes a verdict and
never fails a run.

> **Why a separate `drift/audit.yaml` and not `drift/metadata.yaml`.** (1) **Provenance differs** — the
> observation is computed from bytes this run fetched and can be verified against them; audit rows are copied
> from an external system with its own ingestion latency, retention and completeness. (2) **Determinism** —
> `findingsSha256` and the "identical bytes over an unchanged tenant except the timestamp" property survive
> only if the observation never contains what was ingested since. (3) **It must be allowed to fail** — no
> workspace, no grant or a window past retention may not invalidate the observation; a separate file can
> simply be absent. It sits at the drift root beside `metadata.yaml` and `analyze.md`, where no payload can be
> (payloads are ≥ 2 levels deep), so `drift.ClearTree` — the drift run's clear-and-rebuild and the
> re-baselining download — sweeps it. **No new delete path.**
>
> **The join key and the window already exist.** `Finding.ResourceID` is the Graph object GUID the audit
> tables record as the target; `Observation.Baseline.GeneratedAt .. Observation.ObservedAt` is exactly the
> interval the verdict claims the change fell in. No heuristic bounds; the whole window is queried in one go.
>
> **Facts only, and the gaps are facts too.** Record the actor (UPN or application display name), activity,
> result, event timestamp and correlation id — never "authorized" or "expected". Where the join cannot be made
> the entry says why with a distinct status, because *no event found* and *could not look* must never render
> the same: singletons and pseudo-ids (`organization`, `onPremisesSynchronization` = tenant GUID,
> `authorizationPolicy`, `authenticationMethodsPolicy`, `deviceManagement`, `organizationalBranding`,
> `applePushNotificationCertificate` = Apple ID, `roleScopeTags` whose ids are numeric strings) have no
> usable target id, while Intune ids that merely *embed* a GUID (`T_<guid>` app protections, `A_<guid>`
> targeted app configurations, `<guid>_DefaultPlatformRestrictions` enrollment configurations) do;
> a window starting before the table's earliest row is *unknown*, not *unchanged*; ingestion lag means a
> change observed minutes ago may not be queryable yet — which is why `resource audit` is re-runnable; and
> several events in one window are a list, never a single "who".
>
> **Failure semantics (settled).** In both entry points the lookup warns, records the per-table and
> per-finding status, writes whatever it could answer, and leaves the exit code to the drift comparison. A
> missing grant is detected through `azure.IsPermissionError` and degrades to `query-failed` with the grant
> named. Only the standalone `resource audit` adds one refusal of its own: exit 2 when *nothing* could be
> answered (no observation, superseded observation, tenant mismatch, no workspace configured, no table
> queried at all), because a scripted rerun must notice.
>
> **Coverage and routing (settled).** No field says Intune vs Entra; the routing is derived from what each
> registered handler already declares: `models.DetectAPIType` separates ARM from Graph, and every Graph
> handler's `RequiredPermissions` decides the table — any `DeviceManagement*` permission → `IntuneAuditLogs`
> (join on `Properties.TargetObjectIds`); `Policy.Read.All`, `Group.Read.All`, `Agreement.Read.All`,
> `Organization.Read.All`, `OrganizationalBranding.Read.All`, `OnPremDirectorySynchronization.Read.All` →
> `AuditLogs` (join on `TargetResources[].id`). ARM types and types not registered in this build are
> `not-queried` with a reason; `AzureActivity` can be added later behind the same per-finding shape. Both
> tables' field names are mapped onto **one** event shape and never leak into the artifact.
>
> **Where the workspace is configured (settled).** Tenant profiles exist (`<config-dir>/<domain>.yaml`,
> `internal/config` `keyScopes`), so the workspace is one tenant-scoped, config-only key,
> `audit-workspace-id`, holding the workspace **id** (GUID) — never a name (resolving one needs a
> subscription, Reader on its resource group and cross-subscription disambiguation). There is deliberately
> **no flag and no environment variable**: a value typed for one tenant and forgotten would silently apply to
> the next, and a wrong workspace does not fail loudly — it returns no rows, which reads as *nobody changed it*.
>
> **Decision.** How is attribution enabled in a drift run: by the presence of `audit-workspace-id` in the
> tenant profile — no flag; unset means off.
>
> **Decision.** Is `drift/audit.yaml` written when no workspace is configured: no — the file exists only when
> a workspace is configured, so *absent file = attribution off* (simplest for the browser; `ClearTree` sweeps
> an old one on the next drift run anyway). `resource audit` with no workspace refuses (exit 2) naming the key.
>
> **Contract.** `drift/audit.yaml` at the drift root, written atomically by `resource audit` and — only when
> `audit-workspace-id` is set — by `resource drift` after `metadata.yaml`; swept by `drift.ClearTree`; absent
> file = attribution off. Top level: `version: 1`; `observedAt` and `baselineGeneratedAt` copied from the
> observation it attributes (the browser renders it only when both equal the observation's); `tenant`,
> `toolVersion`, `queriedAt`, `workspaceId`; `window: {from: <baselineGeneratedAt>, to: <observedAt>}`;
> `tables` with **both** keys `IntuneAuditLogs` and `AuditLogs` always, each `{status: ok | failed, reason,
> earliest}`; `counts: {matched, noEventInWindow, noJoinKey, retentionExceeded, queryFailed, notQueried}`;
> `findings` keyed exactly like `drift/metadata.yaml`'s findings (`<type>/<name>.yaml`, every one), each
> `{status, table, reason, events}` — `status` ∈ `matched | no-event-in-window | no-join-key |
> retention-exceeded | query-failed | not-queried`, `table` ∈ `IntuneAuditLogs | AuditLogs | ""`, `reason`
> empty for `matched` / `no-event-in-window`, `events` (matched only, ≥ 1, newest first, then by
> correlationId) of `{at, actor (UPN or app display name), actorType: user | application | unknown, activity,
> result: success | failure | unknown, correlationId}`. Every timestamp is a quoted RFC3339 UTC string with
> whole seconds (`2026-09-30T10:00:00Z`); `earliest` may be `""`. Facts only, never a judgment; bytes are
> deterministic for a fixed query result except `queriedAt`.
>
> **Owner.** `.claude/rules/go-export-safety.md` (the drift-root file list gains `audit.yaml`, swept, never
> pruned — written at *done* with the README); every other file is under `go/`. No sequencing with the web side: the browser gates on `observedAt` / `baselineGeneratedAt` and
> treats an absent file as normal, so either side may ship first.
>
> **Implementer.** opus — new package, new SDK, new token audience.
>
> **Not regeneration-gated.** It touches no `doc-prompt.md` and no per-type template, so no `promptSha256`
> moves. The drift-analysis template (`analyze_drift_template.md`) is not hashed either.

**Plan.**

- ~~**File shape and lifecycle in `internal/drift` (`audit.go`)**, so `docs analyze-drift` can read it without
  an import cycle: `AuditFileName = "audit.yaml"`, `AuditPath(tenantDir)`, the `Attribution` /
  `AttributionFinding` / `AttributionEvent` / `TableStatus` / `AttributionCounts` types and the six status
  constants (yaml tags exactly as in the Contract), `LoadAttribution(tenantDir)` returning a distinct
  `ErrNoAttribution`, `(a *Attribution) Matches(obs Observation) bool` (observedAt and baseline generatedAt
  equal), and `WriteAttribution(tenantDir, *Attribution, dryRun) (string, error)` — never clears anything,
  never creates the tree (an absent `drift/` means no observation), writes atomically; under `dryRun` returns
  the path and writes nothing.~~
- ~~**One atomic-write helper instead of a third copy**: extract the temp-file-in-dir → write → `Chmod 0644` →
  `Rename` sequence duplicated in `docs.writeMetadata` and `drift.WriteObservation` into an exported
  `docs.WriteFileAtomic(path string, data []byte) error` (drift already imports docs; the temp file is
  `.<basename stem>-*<ext>` in the target's directory, removed on every failure) and use it in all three
  places; the callers wrap its error with their existing messages, so the existing observation and metadata
  tests keep passing unchanged.~~
- ~~**Extract the currency check** from `analyzePreflight` into exported
  `drift.CheckCurrent(tenantDir, expectDomain string) (Observation, docs.Metadata, error)`: `LoadObservation`
  → `docs.LoadExportMetadata` → both tenant cross-checks (`docs.ErrTenantMismatch`) → `obs.Baseline.GeneratedAt
  != meta.GeneratedAt` → `ErrObservationSuperseded`. `analyzePreflight` calls it and keeps the marker
  validation and `verifyPayloads`; an audit file can therefore never describe an observation the current
  baseline has replaced.~~
- ~~**Engine package `internal/audit`** (imports `drift`, `azure`, `handlers`, `models`; imported by
  `cmd/resource` only — never by `drift`):~~
  - ~~`Route(registry, key string, f drift.Finding) (table, status, reason string)`, the type taken from the
    key through a newly exported `drift.TypeOfKey` (today's private `typeOfKey`, `path.Dir`), builds the
    type → table map once from the registry (`models.PermissionScoped.RequiredPermissions()`: any permission
    with prefix `DeviceManagement` → `IntuneAuditLogs`, which wins over an Entra permission on the same type;
    the six Entra permissions → `AuditLogs`; `DetectAPIType` ≠ Graph → `not-queried` "ARM resource;
    AzureActivity is not consulted"; unregistered type → `not-queried` "type not registered in this build";
    a registered Graph type matching neither set → `not-queried` "no audit table mapped for this type"),
    then applies the join-key guard: an explicit singleton-type set (the types listed in the Notes), an empty
    `ResourceID`, and an id that does not match `^[A-Za-z0-9_-]{1,128}$` or contains no GUID →
    `no-join-key` with the reason. An id that embeds a GUID (`T_<guid>`, `<guid>_Suffix`) is queried as
    both the id and the embedded GUID, and a row matches the finding whose id or embedded GUID equals the
    target (case-insensitive). Only ids of that character set ever reach a query string, which is what makes
    the KQL interpolation safe.~~
  - ~~`Querier` interface (`Query(ctx, workspaceID, kql string, from, to *time.Time) ([]Row, error)`, `Row` a
    `map[string]any` keyed by column name, `nil` bounds = no `Timespan`) with the
    one real implementation over `sdk/monitor/query/azlogs` (`azlogs.NewClient(cred, nil)`,
    `QueryWorkspace(ctx, id, azlogs.QueryBody{Query, Timespan}, nil)`; the client requests the
    `https://api.loganalytics.io/.default` audience itself). New direct dependency added with `make deps`
    (verify the package path and `QueryBody`/`TimeInterval` names at that point — `azquery` is deprecated
    and must not be used).~~
  - ~~Queries, one per table per chunk of ≤ 200 ids over the whole window: `IntuneAuditLogs | where
    TimeGenerated between (from .. to) | extend P = parse_json(Properties) | mv-expand T = P.TargetObjectIds
    | where tostring(T) in~ (ids) | project …` and `AuditLogs | … | mv-expand T = TargetResources | where
    tostring(T.id) in~ (ids) | project …`; ids embedded as a quoted list. Row mapping onto
    `AttributionEvent`: Intune actor from `Properties.Actor.UPN` (user) else `Properties.Actor.ApplicationName`
    (application), activity `OperationName`, result `ResultType`; Entra actor from
    `InitiatedBy.user.userPrincipalName` else `InitiatedBy.app.displayName`, activity `ActivityDisplayName`,
    result `Result`; results lower-cased to `success | failure | unknown`; `correlationId` from
    `CorrelationId`; `at` from `TimeGenerated` in UTC, formatted `time.RFC3339` (fraction truncated). Column
    names are pinned by the fixtures, written from Microsoft's published table schemas and confirmed by the
    live check below, not by memory.~~
  - ~~Retention, data-driven and permission-free: for **both** tables, whether or not a finding routes to
    them (so `tables` always carries both keys and a missing diagnostic setting surfaces as `failed`), one
    unbounded `| summarize min(TimeGenerated)` (no `Timespan`) before the event queries; `earliest` recorded per table; when `window.from` precedes it,
    every finding of that table without an event becomes `retention-exceeded` (reason states both times);
    a table with no rows at all records `earliest: ""` and the same status with reason "table has no rows".~~
  - ~~Status mapping: routed + ≥ 1 event → `matched` (events sorted newest first, then by correlationId);
    routed, table `ok`, no event → `no-event-in-window` or `retention-exceeded`; table `failed` →
    `query-failed` with the table's reason (a permission error names the grant: *Log Analytics Reader* on the
    workspace, and `Data.Read` on the Log Analytics API for a dedicated app); a token that cannot be minted at
    all marks both tables `failed` and every routable finding `query-failed`. Exactly one entry per
    observation finding; `counts` tallies the statuses.~~
  - ~~`Attribute(ctx, q Querier, registry, obs drift.Observation, opts Options) *drift.Attribution` where
    `Options{WorkspaceID, ToolVersion, Now, Selection{Types, ResourceIDs, ResourceGroup}}`: findings outside
    the selection are `not-queried` "outside this run's selection" so the file stays complete; `Now` is
    injected for tests.~~
- ~~**Configuration key** `audit-workspace-id` → `ScopeTenant` in `internal/config/keys.go`, read with
  `viper.GetString`, no default; validated in `config.Load` after the merge as a GUID (an error naming the key
  and the profile file, surfaced by `initConfig` — a full ARM resource id or a workspace name is not
  accepted). `config.example.domain.yaml` gains the key **empty and commented** under a new "Audit
  attribution" heading explaining the grant and the diagnostic-settings prerequisite;
  `TestConfigExampleDomainIsNoOp` asserts it is unset, `TestConfigExamplesCoverThePartition` passes by
  construction, `TestPartitionIsEnforced` gains the case "audit-workspace-id in the base file", and a new
  case rejects a non-GUID value.~~
- ~~**Tenant-dir helper move**: relocate `resolveExportDir` (offline with `--domain`, else authenticate with the
  profile's credentials and resolve, else the single export) from `cmd/docs/generate_prompt.go` to
  `cmdutil.ResolveExportDir(ctx, baseOutput, declaredDomain string, cred azcore.TokenCredential) (tenantDir,
  expectDomain string, err error)`, resolving the domain through `azure.NewClientWithCredential(ctx, cred,
  subscription, tenant-id)` instead of building its own credential (a `nil` cred — the caller could not build
  one — takes today's warn-and-fall-back path), and collapsing `detectSingleExportDomain` onto the existing
  `cmdutil.ExportDomains`; the three `docs` subcommands build the credential with
  `azure.NewCredential(client-id, tenant-id)` and pass it, unchanged in behaviour. This is what lets
  `resource audit` sign in once and reuse the same credential for the Log Analytics query.~~
- ~~**`resource audit` command** (`cmd/resource/audit.go`, attached in `cmd/resource.go`; the parent's Long
  text no longer says "all four"): refuses (exit 2) on an empty `audit-workspace-id` before anything else,
  builds the credential once with `azure.NewCredential(client-id, tenant-id)` from the profile (never
  `runprep.Prepare` — no Graph probe, no dedicated-app prompt), resolves the tenant dir through
  `cmdutil.ResolveExportDir` with that credential, runs `drift.CheckCurrent`, refuses (exit 2) on
  `ErrNoObservation` / `ErrObservationSuperseded` / `ErrTenantMismatch`, builds a registry for routing
  with `handlers.NewRegistry(cred, "", false)` (no network: the credential is lazy), calls `audit.Attribute`, writes with `drift.WriteAttribution`, prints the per-table status and the status counts,
  and exits 2 when no table could be queried. Honours the inherited flags: `--type` narrows the attributed
  findings to those types, `--resource-id` / `--resource-group` filter the finding set (documented: ARM
  findings are never queried, so `--resource-group` yields an all-`not-queried` file and warns); no
  `--exit-code`. `--dry-run` runs the queries, reports, withholds the write and says an earlier `audit.yaml`
  was not refreshed. Extend `TestResourceGroupSharesFlags` and `TestRemovedFlagsAreGone` to include `audit`
  and to assert it does not offer `--exit-code`.~~
- ~~**Drift-run integration** in `cmd/resource/drift.go`: after `WriteObservation` returns (metadata written
  last), when `audit-workspace-id` is set, attribute the observation **as written** — today
  `WriteObservation` stamps `ObservedAt` / `ToolVersion` on a copy, so `rep.Observation.ObservedAt` stays
  empty; change it to return the stamped `Observation` alongside the path (also under `--dry-run`) and
  attribute that, so `audit.yaml`'s `observedAt` equals `metadata.yaml`'s byte for byte — with
  `prep.Client.GetCredential()`
  (the same credential minting a token for the Log Analytics audience), the run's selection as
  `Options.Selection`, and write `audit.yaml`; warn per failed table; the exit code stays the comparison's.
  The token request and the queries run under a context bounded by the run's `timeout`, so an interactive
  sign-in the new audience might trigger (a dedicated app without the Log Analytics consent) cannot hold the
  run open; its failure degrades to `query-failed`. Unset key → one info line, no file. Under `--dry-run`
  query and report, withhold the write, note the stale file. Update the command's Long text.~~
- ~~**Prompt splice** in `internal/drift` (`analyzeprompt*.go`, `analyze_drift_template.md` — not hashed, not
  regeneration-gated): `GenerateAnalyzePrompt` loads `audit.yaml` via `LoadAttribution` and uses it only when
  `Matches(obs)`; the `observation` block gains `- Attribution: <workspace, queriedAt, per-table status,
  counts>` or `- Attribution: none (no drift/audit.yaml — configure audit-workspace-id or run 'azure-rd
  resource audit')` or `… outdated (belongs to observation <observedAt>)`, plus the ingestion-lag caveat;
  `renderFinding` emits `- Changed by: <actor> (<actorType>) at <at> — <activity>, <result>, correlation
  <id>` per event (newest first) or `- Attribution unavailable (<status>: <reason>)`; step 2 gains a short
  2b' note (an attributed actor is a fact about *who*, never proof of intent; several events are listed, judge
  the latest; absence is not evidence); the ground rules add `drift/audit.yaml` to the never-touch list.~~
- ~~**Tests, no network**: recorded response fixtures for both table schemas (row → event mapping, actor
  precedence, result normalisation); routing of **every registered type** through the real registry
  (each lands on exactly one of the two tables or `not-queried`) plus the singleton/pseudo-id → `no-join-key`
  cases, the numeric `roleScopeTags` id and an empty `ResourceID` → `no-join-key`, and `T_<guid>` /
  `<guid>_Suffix` ids queried as id plus embedded GUID and matched by either; chunking (201 ids → two
  queries, only ids of the guarded character set in the KQL); every status path including retention
  (earliest after `window.from`, empty table) and permission failure (a fake `Querier` returning an
  `*azcore.ResponseError` 403 → `query-failed` naming the grant, token failure → both tables failed); the
  selection filter producing `not-queried`; `tables` carrying both keys when every finding routes to one
  table; `CheckCurrent` refusals; `WriteAttribution` writing after the observation and never under dry-run;
  the drift-run path writing an `audit.yaml` whose `observedAt` / `baselineGeneratedAt` equal the
  `metadata.yaml` just written (fake `Querier`); `resource audit` exit 2 without the key, without an
  observation and with a superseded one; byte determinism for a fixed fixture and injected `Now`;
  `LoadAttribution`/`Matches` staleness; the prompt splice with present, absent and outdated files
  (deterministic output); the config key partition and the example file's no-op promise; the flag-surface
  tests above, plus no command offering `--audit-workspace-id`.~~
- **Documentation** (written at *done*): README — new `resource audit` section (prerequisites: Intune and
  Entra diagnostic settings shipping `IntuneAuditLogs` / `AuditLogs` to one workspace, *Log Analytics
  Reader* on it, the profile key; flags honoured; the artifact and its statuses; re-runnability and
  ingestion lag; exit codes), `resource drift` (enabled by the key, written after the observation),
  `docs analyze-drift` (the spliced lines), Authentication (third audience `api.loganalytics.io`; the
  app-registration script adds the Log Analytics API delegated permission `Data.Read`), Configuration
  partition table (`audit-workspace-id` tenant-scoped), Output layout tree (`drift/audit.yaml`, swept);
  `CHANGELOG.md` under `[Unreleased]`; the drift-root file lists in `.windsurf/rules/04-security-and-ops.md`
  and `../.claude/rules/go-export-safety.md` naming `audit.yaml` as swept, never pruned; `go/CLAUDE.md`'s
  layout gains `internal/audit/` and `audit` in the `cmd/resource/` list.
- **Live check** (the user — no agent has a tenant): with Intune and Entra diagnostic settings shipping to one
  workspace, *Log Analytics Reader* on it and, for a dedicated app, the Log Analytics API `Data.Read`
  delegated permission consented, run `resource drift` and `resource audit` against a tenant with a known
  change; confirm the column names and actor paths the fixtures assume, and that an app protection
  (`T_<guid>`) or enrollment configuration change joins.

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
