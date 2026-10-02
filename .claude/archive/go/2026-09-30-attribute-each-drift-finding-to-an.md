---
title: Attribute each drift finding to an actor and a time, from the tenant's Log Analytics audit tables
project: go
status: done
started: 2026-09-29
finished: 2026-09-30
branch: feat/drift-attribution
pr: 33
changelog: 0.4.0
---
## Attribute each drift finding to an actor and a time, from the tenant's Log Analytics audit tables

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

- ✅ **File shape and lifecycle in `internal/drift` (`audit.go`)**, so `docs analyze-drift` can read it without
  an import cycle: `AuditFileName = "audit.yaml"`, `AuditPath(tenantDir)`, the `Attribution` /
  `AttributionFinding` / `AttributionEvent` / `TableStatus` / `AttributionCounts` types and the six status
  constants (yaml tags exactly as in the Contract), `LoadAttribution(tenantDir)` returning a distinct
  `ErrNoAttribution`, `(a *Attribution) Matches(obs Observation) bool` (observedAt and baseline generatedAt
  equal), and `WriteAttribution(tenantDir, *Attribution, dryRun) (string, error)` — never clears anything,
  never creates the tree (an absent `drift/` means no observation), writes atomically; under `dryRun` returns
  the path and writes nothing.
- ✅ **One atomic-write helper instead of a third copy**: extract the temp-file-in-dir → write → `Chmod 0644` →
  `Rename` sequence duplicated in `docs.writeMetadata` and `drift.WriteObservation` into an exported
  `docs.WriteFileAtomic(path string, data []byte) error` (drift already imports docs; the temp file is
  `.<basename stem>-*<ext>` in the target's directory, removed on every failure) and use it in all three
  places; the callers wrap its error with their existing messages, so the existing observation and metadata
  tests keep passing unchanged.
- ✅ **Extract the currency check** from `analyzePreflight` into exported
  `drift.CheckCurrent(tenantDir, expectDomain string) (Observation, docs.Metadata, error)`: `LoadObservation`
  → `docs.LoadExportMetadata` → both tenant cross-checks (`docs.ErrTenantMismatch`) → `obs.Baseline.GeneratedAt
  != meta.GeneratedAt` → `ErrObservationSuperseded`. `analyzePreflight` calls it and keeps the marker
  validation and `verifyPayloads`; an audit file can therefore never describe an observation the current
  baseline has replaced.
- ✅ **Engine package `internal/audit`** (imports `drift`, `azure`, `handlers`, `models`; imported by
  `cmd/resource` only — never by `drift`):
  - ✅ `Route(registry, key string, f drift.Finding) (table, status, reason string)`, the type taken from the
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
    the KQL interpolation safe.
  - ✅ `Querier` interface (`Query(ctx, workspaceID, kql string, from, to *time.Time) ([]Row, error)`, `Row` a
    `map[string]any` keyed by column name, `nil` bounds = no `Timespan`) with the
    one real implementation over `sdk/monitor/query/azlogs` (`azlogs.NewClient(cred, nil)`,
    `QueryWorkspace(ctx, id, azlogs.QueryBody{Query, Timespan}, nil)`; the client requests the
    `https://api.loganalytics.io/.default` audience itself). New direct dependency added with `make deps`
    (verify the package path and `QueryBody`/`TimeInterval` names at that point — `azquery` is deprecated
    and must not be used).
  - ✅ Queries, one per table per chunk of ≤ 200 ids over the whole window: `IntuneAuditLogs | where
    TimeGenerated between (from .. to) | extend P = parse_json(Properties) | mv-expand T = P.TargetObjectIds
    | where tostring(T) in~ (ids) | project …` and `AuditLogs | … | mv-expand T = TargetResources | where
    tostring(T.id) in~ (ids) | project …`; ids embedded as a quoted list. Row mapping onto
    `AttributionEvent`: Intune actor from `Properties.Actor.UPN` (user) else `Properties.Actor.ApplicationName`
    (application), activity `OperationName`, result `ResultType`; Entra actor from
    `InitiatedBy.user.userPrincipalName` else `InitiatedBy.app.displayName`, activity `ActivityDisplayName`,
    result `Result`; results lower-cased to `success | failure | unknown`; `correlationId` from
    `CorrelationId`; `at` from `TimeGenerated` in UTC, formatted `time.RFC3339` (fraction truncated). Column
    names are pinned by the fixtures, written from Microsoft's published table schemas and confirmed by the
    live check below, not by memory.
  - ✅ Retention, data-driven and permission-free: for **both** tables, whether or not a finding routes to
    them (so `tables` always carries both keys and a missing diagnostic setting surfaces as `failed`), one
    unbounded `| summarize min(TimeGenerated)` (no `Timespan`) before the event queries; `earliest` recorded per table; when `window.from` precedes it,
    every finding of that table without an event becomes `retention-exceeded` (reason states both times);
    a table with no rows at all records `earliest: ""` and the same status with reason "table has no rows".
  - ✅ Status mapping: routed + ≥ 1 event → `matched` (events sorted newest first, then by correlationId);
    routed, table `ok`, no event → `no-event-in-window` or `retention-exceeded`; table `failed` →
    `query-failed` with the table's reason (a permission error names the grant: *Log Analytics Reader* on the
    workspace, and `Data.Read` on the Log Analytics API for a dedicated app); a token that cannot be minted at
    all marks both tables `failed` and every routable finding `query-failed`. Exactly one entry per
    observation finding; `counts` tallies the statuses.
  - ✅ `Attribute(ctx, q Querier, registry, obs drift.Observation, opts Options) *drift.Attribution` where
    `Options{WorkspaceID, ToolVersion, Now, Selection{Types, ResourceIDs, ResourceGroup}}`: findings outside
    the selection are `not-queried` "outside this run's selection" so the file stays complete; `Now` is
    injected for tests.
- ✅ **Configuration key** `audit-workspace-id` → `ScopeTenant` in `internal/config/keys.go`, read with
  `viper.GetString`, no default; validated in `config.Load` after the merge as a GUID (an error naming the key
  and the profile file, surfaced by `initConfig` — a full ARM resource id or a workspace name is not
  accepted). `config.example.domain.yaml` gains the key **empty and commented** under a new "Audit
  attribution" heading explaining the grant and the diagnostic-settings prerequisite;
  `TestConfigExampleDomainIsNoOp` asserts it is unset, `TestConfigExamplesCoverThePartition` passes by
  construction, `TestPartitionIsEnforced` gains the case "audit-workspace-id in the base file", and a new
  case rejects a non-GUID value.
- ✅ **Tenant-dir helper move**: relocate `resolveExportDir` (offline with `--domain`, else authenticate with the
  profile's credentials and resolve, else the single export) from `cmd/docs/generate_prompt.go` to
  `cmdutil.ResolveExportDir(ctx, baseOutput, declaredDomain string, cred azcore.TokenCredential) (tenantDir,
  expectDomain string, err error)`, resolving the domain through `azure.NewClientWithCredential(ctx, cred,
  subscription, tenant-id)` instead of building its own credential (a `nil` cred — the caller could not build
  one — takes today's warn-and-fall-back path), and collapsing `detectSingleExportDomain` onto the existing
  `cmdutil.ExportDomains`; the three `docs` subcommands build the credential with
  `azure.NewCredential(client-id, tenant-id)` and pass it, unchanged in behaviour. This is what lets
  `resource audit` sign in once and reuse the same credential for the Log Analytics query.
- ✅ **`resource audit` command** (`cmd/resource/audit.go`, attached in `cmd/resource.go`; the parent's Long
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
  and to assert it does not offer `--exit-code`.
- ✅ **Drift-run integration** in `cmd/resource/drift.go`: after `WriteObservation` returns (metadata written
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
  query and report, withhold the write, note the stale file. Update the command's Long text.
- ✅ **Prompt splice** in `internal/drift` (`analyzeprompt*.go`, `analyze_drift_template.md` — not hashed, not
  regeneration-gated): `GenerateAnalyzePrompt` loads `audit.yaml` via `LoadAttribution` and uses it only when
  `Matches(obs)`; the `observation` block gains `- Attribution: <workspace, queriedAt, per-table status,
  counts>` or `- Attribution: none (no drift/audit.yaml — configure audit-workspace-id or run 'azure-rd
  resource audit')` or `… outdated (belongs to observation <observedAt>)`, plus the ingestion-lag caveat;
  `renderFinding` emits `- Changed by: <actor> (<actorType>) at <at> — <activity>, <result>, correlation
  <id>` per event (newest first) or `- Attribution unavailable (<status>: <reason>)`; step 2 gains a short
  2b' note (an attributed actor is a fact about *who*, never proof of intent; several events are listed, judge
  the latest; absence is not evidence); the ground rules add `drift/audit.yaml` to the never-touch list.
- ✅ **Tests, no network**: recorded response fixtures for both table schemas (row → event mapping, actor
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
  tests above, plus no command offering `--audit-workspace-id`.
- ✅ **Documentation** (written at *done*): README — new `resource audit` section (prerequisites: Intune and
  Entra diagnostic settings shipping `IntuneAuditLogs` / `AuditLogs` to one workspace, *Log Analytics
  Reader* on it, the profile key; flags honoured; the artifact and its statuses; re-runnability and
  ingestion lag; exit codes), `resource drift` (enabled by the key, written after the observation),
  `docs analyze-drift` (the spliced lines), Authentication (third audience `api.loganalytics.io`; the
  app-registration script adds the Log Analytics API delegated permission `Data.Read`), Configuration
  partition table (`audit-workspace-id` tenant-scoped), Output layout tree (`drift/audit.yaml`, swept);
  `CHANGELOG.md` under `[Unreleased]`; the drift-root file lists in `.windsurf/rules/04-security-and-ops.md`
  and `../.claude/rules/go-export-safety.md` naming `audit.yaml` as swept, never pruned; `go/CLAUDE.md`'s
  layout gains `internal/audit/` and `audit` in the `cmd/resource/` list.
- ✅ **Live check** (the user — no agent has a tenant): with Intune and Entra diagnostic settings shipping to one
  workspace, *Log Analytics Reader* on it and, for a dedicated app, the Log Analytics API `Data.Read`
  delegated permission consented, run `resource drift` and `resource audit` against a tenant with a known
  change; confirm the column names and actor paths the fixtures assume, and that an app protection
  (`T_<guid>`) or enrollment configuration change joins.
