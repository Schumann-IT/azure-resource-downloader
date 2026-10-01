---
title: Check access before listing, and refuse a run that would be mostly refused
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/plan-dependency-updates
changelog: Unreleased
---
## Check access before listing, and refuse a run that would be mostly refused

**Goal.** A run finds out before it lists anything whether the signed-in account can actually read what was
selected. When it cannot — an expired PIM-activated Global Administrator, a missing Intune role, a missing Entra
role — it stops with one clear message naming what is missing, instead of listing everything, collecting dozens of
refusals and ending as a mostly empty, incomplete run.

> **Why.** On 2026-10-01 a `resource download --dry-run` on cb-gmbh.com ran after the operator's Global
> Administrator role had expired: 44 of 50 types were refused while listing — Intune answered `HTTP 401` with a
> nested `{"ErrorCode":"Forbidden", …}` body on every device-management and app endpoint, Entra answered `403`
> (`Authorization_RequestDenied`, `accessDenied`, `AccessDenied` naming the roles needed, `UnauthorizedUserRole`).
> The token itself was fine: the cached dedicated-app session carried the scopes. This is a **role** problem, which
> a scope check (the `--debug` coverage of the sign-in entry) cannot see — only a live request can. Today the run
> lists every type, logs each refusal twice (warn and debug) with the full nested JSON, and continues; a real run
> would then write an incomplete export.
>
> **Decision.** When a probe is refused: refuse the whole run before listing — exit non-zero, nothing written; to
> run with less, the operator narrows the selection (`--type`, `type:`, `exclude-type`).
>
> **Scope, settled at review (2026-10-01).** The check runs in `resource download`, `resource drift` and
> `resource list` — the commands that list by type — and only when the run lists by type: a `--resource-id` or
> `--resource-group` run lists nothing (`Registry.BuildFetchRequests` returns those requests directly), so it is
> not probed and keeps today's per-resource handling. `resource types` (counts with a non-interactive credential,
> documented as never failing on access), `resource audit` (Log Analytics, no listing) and `--debug` (writes and
> refuses nothing) never run it. ARM types are probed only when the run has a subscription; without one they are
> skipped without a network call, as today. `--dry-run` probes and refuses like a real run: the probe only reads.
> Consequence of the decision worth knowing: a full run by an account without the subscription's Reader role is now
> refused (ARM group refused) where today it skips the ARM types; the refusal says to exclude them. `resource list`
> changes the same way — its help says an unlistable type "does not fail the command"; a 401/403 probe now does.
>
> **Where it sits, and the sign-in.** At the end of `runprep.Prepare` — after credential resolution, `NewClient`,
> tenant resolution (a tenant mismatch still refuses first) and the real registry — and, in `resource list`, after
> its registry and before `BuildFetchRequests`. Correction to the premise the entry was handed: the cached
> device-code credential signs in at the **first token request**, which in `Prepare` is the ARM subscription
> auto-detect inside `NewClient` (or, with `subscription` configured, the ARM tenant lookup in `resolveTenantDir`)
> — both before the probe. The probe never causes the prompt and never adds one; it runs on the session that
> already exists.
>
> **Classification, settled at review.** Only a typed status decides (`azure.HTTPStatus`, `azure.GraphErrorCode`,
> the ARM `ErrorCode`): 401 and 403 are *refused*; an azidentity credential failure
> (`*azidentity.AuthenticationFailedError`, `*azidentity.AuthenticationRequiredError`,
> `*azidentity.CredentialUnavailableError` — e.g. a cancelled device-code sign-in or a dedicated app without ARM
> consent) is *refused* as "sign-in failed", because every listing would fail the same way; 404 (an unconfigured
> singleton), 400, 408, 429, 5xx, a probe timeout and any untyped error are *not refusals* — logged at debug and
> left to the listing as today; context cancellation (Ctrl+C) returns the context error. Throttling is already
> retried by the SDK pipelines (azcore's retry policy for ARM, Kiota's retry handler for Graph, both honouring
> `Retry-After`); the probe adds no retry layer and must **not** use `retry.Config.IsRetryable` or
> `azure.IsPermissionError`'s text markers to classify — both match substrings, and an Intune body's Activity ID or
> URL can contain `403` or `429`.
>
> **Coupling.** Not regeneration-gated: no template, no `promptSha256`. Permission groups are derived at run time
> from each handler's declared `RequiredPermissions`, so the permission corrections of the per-handler metadata
> entry move the groups by themselves; both entries edit Graph constructors under `internal/handlers/graph/`, so on
> a shared branch the second one rebases over the first. Changing `azure.ErrorSummary` changes the
> `notListed.reasons` strings in `resources/metadata.yaml` for Intune types only; reasons are not hashed, gate no
> drift comparability, and the ARM summary (with its request line) stays exactly as it is.
>
> **Contract.** None with `web/`: `web/` does not read `notListed.reasons` or any field this entry touches, and
> no file name, path, key or exit code under the export tree changes. Within `go/`: `resources/metadata.yaml`
> keeps its shape; only the text of an Intune type's `notListed.reasons` value becomes the one-line
> `HTTP 401 Forbidden: <operation> (hint: …)`; ARM reasons keep `HTTP <status> <code>: GET https://…`. A refused
> run writes nothing anywhere (no `resources/`, `docs/` or `drift/` change, `drift/` not cleared). Exit codes:
> `resource download` and `resource list` 1, `resource drift` 2 ("cannot answer"); `resource types`, `resource
> audit` and `--debug` are unchanged. No new flag and no new config key.
>
> **Owner.** none — no file outside `go/`. Sequencing: none with `web/`; see the coupling note for the per-handler
> metadata entry.
>
> **Implementer.** opus — it adds a concurrent step in front of every listing command and must not misread a 404
> or a throttle as a refusal.

**Plan.**

- ✅ Permission groups and the probe plan (pure, `internal/runprep/access.go`): `PermissionGroup(perm string)` strips
  a trailing `.Read.All` / `.ReadWrite.All` (`DeviceManagementConfiguration.ReadWrite.All` →
  `DeviceManagementConfiguration`, so `resolve-secrets` does not split a group). A type's group is the group of its
  first declared permission (`models.PermissionScoped.RequiredPermissions()`); an ARM type (`models.DetectAPIType`)
  belongs to the single group `AzureRBAC`; a type with neither is not probed. `PlanProbes(types []TypeInfo)
  []Probe` — `TypeInfo{Type, Group, HasAccessProbe}`, `Probe{Group, ProbeType, Blocks []string}` — returns one
  probe per group, sorted by group; the probed type is the first type in sorted order with `HasAccessProbe`, else
  the first in sorted order; `Blocks` lists every selected type of the group, sorted. Input is the effective
  selection (`Prepared.EffectiveTypes`, `TypeSelection.Effective` in `list`), minus ARM types when the subscription
  is empty; a `--resource-id` or `--resource-group` run plans nothing.
- ✅ Probe surface (`internal/models`): a new optional interface `AccessProber { HasAccessProbe() bool;
  ProbeAccess(ctx context.Context) error }`. `GraphCollectionHandler` gains an optional `probe func(ctx) error`
  closure and implements it: `HasAccessProbe` reports `probe != nil`, `ProbeAccess` calls `probe`, else `listIDs`
  (result discarded). A handler without the interface (ARM) is probed by its `List`. Give a `probe` closure — one
  GET of the collection's first page with `Top: 1` (and `Select: []string{"id"}` where the request builder offers
  it), returning the SDK error unwrapped except for the handler's usual context and `(hint: …)` — to at least one
  collection type in each of `DeviceManagementConfiguration`, `DeviceManagementApps`,
  `DeviceManagementServiceConfig`, `DeviceManagementScripts`, `DeviceManagementRBAC`,
  `DeviceManagementManagedDevices` (device categories), `Policy`, `Agreement` (terms of use) and `Group`; the
  singleton groups (`Organization`, `OrganizationalBranding`, `OnPremDirectorySynchronization`) are probed by their
  normal listing (one or two requests).
- ✅ Classifier (pure, `internal/runprep/access.go`): `ClassifyProbe(err error) ProbeOutcome` → `Allowed` (nil),
  `Refused` (typed 401/403, or an azidentity `AuthenticationFailedError` / `AuthenticationRequiredError` /
  `CredentialUnavailableError` anywhere in the chain, marked as a sign-in failure), `Inconclusive` (everything
  else), and `context.Canceled` passed through. No substring matching, no `IsPermissionError`, no
  `retry.IsRetryable`.
- ✅ Prober (`runprep.CheckAccess(ctx, registry, probes, timeout, concurrency) error`): runs the planned probes
  concurrently, bounded by `ListingConcurrency`, each under `context.WithTimeout` of the run's `timeout`; results
  land in per-probe slots (no shared mutable state, deterministic order). Inconclusive probes log one debug line
  with the summary and full error and do not refuse. With one or more refused groups it logs, per refused group
  in group order, one `log.Error("Access refused", …)` with `group`, `probed_type`, `status` and service `code`
  (or `sign-in failed`), `blocks` (the selected types of the group) and `hint` — `DeviceManagement*` groups: "no
  Intune role for this account (a PIM-activated role may have expired)"; other Graph groups: the service's own
  message (its first line, which names the required roles when the service sends them); `AzureRBAC`: "no Azure RBAC
  Reader role on subscription <id>"; sign-in failure: "sign-in failed: <ErrorSummary>" — plus the full error at
  debug (Activity ID and URL live there), then returns an error wrapping a new sentinel `runprep.ErrAccessRefused`:
  "access check refused <n> of <m> permission groups; narrow the run with --type, the type: list or exclude-type in
  the tenant profile".
- ✅ Wiring: `Prepare` calls `CheckAccess` last (after the real registry is built), only when the run lists by type;
  `runDrift` maps `errors.Is(err, runprep.ErrAccessRefused)` to `cmdutil.WithExitCode(driftExitCannotAnswer, …)`
  beside the tenant-dir refusals; `runDownload` returns it unchanged (exit 1); `runList` plans and calls
  `CheckAccess` after building its registry and before `BuildFetchRequests` (exit 1), and its `Long` help no longer
  says an unlistable type never fails the command — a refused access check does; a type whose listing still fails
  afterwards is reported as unknown as today. `resource types`, `resource audit` and `runDebugReport` do not call
  it.
- ✅ Error summaries (`azure.ErrorSummary`): when the message (hint stripped) contains a JSON object that decodes
  with `encoding/json` to an object with a string `ErrorCode` (Intune's nested body; its `Message` is itself a
  JSON string carrying `_version`, the Activity ID and the URL), the summary keeps only the text before the object
  (trimmed of ` :{`) as the operation and uses the inner `ErrorCode` as the code, replacing the outer OData code —
  typed: `HTTP 401 Forbidden: failed to list device configurations (hint: requires
  'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)` (hint per the existing 401/403 rule);
  untyped: the same without the `HTTP` prefix. Nothing of the nested message (Activity ID, URL) reaches the
  summary; callers already log the full error at debug. Every other summary is byte-for-byte unchanged — in
  particular the ARM one with its `GET https://…` request line, which `notListed.reasons` keeps.
- ✅ Tests (no network, `make test` and `make test-race` — the prober is concurrent): `PermissionGroup`
  (`Read.All`, `ReadWrite.All`, no suffix); `PlanProbes` (groups from a mixed selection, one probe per group,
  preference for a type with an access probe, fallback to the first type, `Blocks` sorted, ARM dropped without a
  subscription, nothing for `--resource-id` / `--resource-group`); `ClassifyProbe` table — typed beta `ODataError`
  401 with the nested Intune body, v1.0 403 `Authorization_RequestDenied`, ARM 403 `AuthorizationFailed`, 404,
  400, 429, 500, 503, each azidentity error type, `context.DeadlineExceeded`, `context.Canceled`, and an untyped
  error whose text contains `403` and `429` (inconclusive); `CheckAccess` with stub probers (all allowed → nil;
  one refused → `ErrAccessRefused`, other probes still complete; inconclusive → nil); `ErrorSummary` with the
  nested Intune body typed (outer code empty and `UnknownError`) and untyped, plus the existing ARM case
  unchanged; runprep / command integration with a stub prober — refused → no `BuildFetchRequests` call, nothing
  written under a temp output (dry-run and real), download/list exit 1, drift exit 2; all clear → unchanged run;
  `--resource-id` run → prober never called.
- ✅ Follow-up, seen in the cached-session runs (2026-10-01): "Authentication successful" is logged before the lazy
  device-code sign-in actually happens; log that the credential is ready instead, and "Signed in" (with the
  account) once the first token has been obtained. Concretely: `Prepare` and `runList` build the credential with
  `azure.NewCredential`, wrap it in a new `azure.WithSignInLog(cred)` decorator — on the first successful
  `GetToken` (exactly once under concurrency, `sync.Once` entered only on success) it decodes the token's identity
  claims with the existing `parseIdentityClaims` and logs `Signed in` with `user` and `tenant_id`, never the token
  — and pass it to `NewClientWithCredential`; `Prepare`'s "Authentication successful" becomes `Credential ready`
  (with the subscription). No extra token request, no extra prompt; `--debug` keeps its own "Signed in" line and
  does not use the decorator. Tests: logs once for N concurrent callers, not on error, then once on a later
  success.
- ✅ Follow-up: an empty type is reported twice — a per-type `No resources found` warning whose note still offers
  "(2) Insufficient permissions", and the summary's `Empty type` line. With the access check refusing a run without
  access, an empty listing is a real empty: log it once at info level, without the permissions note; the summary
  line stays. (`internal/handlers/requests.go`: `log.Info("No resources found", "type", …)` with no `note`.)
- ✅ Follow-up: "Secret resolution enabled" (and its debug explanation) is logged twice per run — once during
  configuration loading and once during run preparation; log it once. Cause: `registerDefaults` logs it, and
  `Prepare` builds the registry twice (offline for the selection, then with the real credential). Move both lines
  out of `registerDefaults` into `Prepare`, after the real registry, when `ResolveSecrets` is set; name the key
  correctly (`"key", "resolve-secrets"` — it is config-only, there is no `--resolve-secrets` flag). A test asserts
  one occurrence per `Prepare`.
- ✅ Documentation at *done*: `README.md` — the access check before listing (what it probes, what refuses, how to
  narrow a run) in the sign-in / permissions section and the exit codes; `CHANGELOG.md` `### Added` (the access
  check) and `### Changed` (the readable Intune error summary).
