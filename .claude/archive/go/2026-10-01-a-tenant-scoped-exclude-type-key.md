---
title: A tenant-scoped exclude-type key
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/plan-exclude-type
changelog: Unreleased
---
## A tenant-scoped exclude-type key

**Goal.** A tenant profile can name resource types that are never listed for that tenant — typically the ARM types
of an Intune/Entra-only tenant whose account holds no subscription role — and runs that leave them out stay
**complete**, so absence, prune and drift removals keep working. A drift run can never compare silently across a
changed exclusion.

> **Why.** On cb-gmbh.com `virtualMachines`, `storageAccounts` and `resourceGroups` fail listing with `HTTP 403
> AuthorizationFailed` on every run (2026-10-01), which keeps every export and drift run incomplete although the
> tenant's documentation never covers ARM. The only workaround is the general `type:` allow-list in the base file:
> it applies to every tenant in the config directory, must list all ~50 wanted types, and silently stops exporting
> any type a later release registers.
>
> **What is true now.** `filters` is tenant-scoped, `type` general (`internal/config/keys.go:54-92`); each file is
> validated against its side before merging (`internal/config/load.go:89-117`). `--type` is bound to the same viper
> key as `type:` (`cmd/resource.go:56`), so it *replaces* the config list — the flag help
> (`internal/cmdutil/flags.go:54`) wrongly says "narrows". Requests are built in `Registry.BuildFetchRequests`
> (`internal/handlers/requests.go:26-69`, precedence id → group → types → all); the dedicated-app probe mirrors it
> in `SelectedTypeNames` (`internal/runprep/runprep.go:331-352`). Unknown types are caught only lazily during
> listing, and registry lookup is case-sensitive while filters and audit compare case-insensitively. Coverage is
> what was actually listed (`coveredTypes`, `internal/docs/metadata.go:518-537`, mirrored in
> `internal/drift/drift.go:473-489`): a type never requested is neither covered nor skipped, so leaving it out keeps
> a run complete with no change to `MarkCompleteness` (`internal/pipeline/pipeline.go:240-257`). Comparability:
> `filtersSha256` / `transformConfigSha256` refuse a drift run on mismatch (`drift.go:56-90`, `ErrNotComparable`,
> exit 2); the type scope is never compared today.
>
> **Decision.** A `--type` or `type:` naming a type the profile excludes: refuse with an error naming the type and
> the profile — the profile is the tenant's record, a one-off flag does not override it.
>
> **Decision.** Drift against a baseline downloaded with a different exclusion: refuse like `filters` — the exclusion
> is recorded in the export metadata and any difference is not comparable (exit 2); a baseline without the field
> counts as excluding nothing.
>
> **Contract.** None with the browser. It reads `run.complete` / `run.incompleteReason` from `resources/metadata.yaml`
> and the drift observation and never reads `run.scope`; the new `run.scope.excludedTypes` key is additive, omitted
> when empty, and an export without exclusions stays byte-identical. Unrelated to the `counts.excluded` of
> `docs/index.yaml` (types the taxonomy leaves out of the documentation), which this entry does not touch — the
> README must keep the two "excluded" apart.
>
> **Owner.** none — every file is under `go/`. No sequencing constraint.
>
> Not regeneration-gated: no prompt template and no `promptSha256` is touched.
>
> **Implementer.** opus — it touches coverage, the drift preflight and config validation.

**Plan.**

- ✅ Config key: `exclude-type` in `keyScopes` as **tenant-scoped** (`internal/config/keys.go`), a list read with
  `viper.GetStringSlice`. `internal/config` validates only the shape at load (`validateValues`, next to the
  `audit-workspace-id` check): a list of non-empty strings, anything else a fatal error naming the profile file —
  `internal/config` does not import `internal/handlers`. Per `/add-config-option`: `config.example.domain.yaml` gets
  a commented-out example with the three ARM types (the file stays a no-op), `config.example.yaml` a pointer to the
  profile, `cmd/config_test.go` the partition and no-op coverage.
- ✅ Selection, one pure function in `internal/runprep` taking the registered type names, the allow-list (`--type`,
  else config `type:`), the `exclude-type` list, `--resource-id` and `--resource-group`. It resolves each excluded
  name against the registered types case-insensitively to its registered spelling (unknown = error naming the type
  and the profile; duplicates collapse) and returns the sorted exclusion and the effective types to list: the
  allow-list, else all registered types, minus the exclusion. A clash refuses with an error naming the type and the
  profile: an allow-list entry that is excluded, and likewise a type derived from the selection the way
  `SelectedTypeNames` derives it — a parseable `--resource-id` of an excluded type, or `--resource-group` while
  `Microsoft.Resources/resourceGroups` is excluded; unparseable ids (bare Graph GUIDs) carry no type and pass.
- ✅ `runprep.Prepare` calls it first, before session verification and sign-in, with the type names of an offline
  registry (lazy credential, as the dedicated-app probe already builds one). `Prepared` keeps `SelectedTypes` as
  the allow-list as asked and gains `ExcludedTypes` plus the effective list; `BuildRequests` passes the effective
  list to `BuildFetchRequests` and the dedicated-app probe passes it to `SelectedTypeNames`, so the scope prompt
  never asks for an excluded type's scopes. With exclusions the excluded types are logged once at info level.
- ✅ `resource list` (`cmd/resource/list.go`) calls the same function before signing in and lists only the effective
  types. `resource types` (`cmd/resource/types.go`) keeps listing every registered type — it is the catalogue an
  operator picks names from — but marks an excluded type `excluded: exclude-type` and leaves it out of the live
  `tenantCounts` listing, so it never reports a 403 for it; a clash refuses there too.
- ✅ Metadata: `docs.RunScope` gains `ExcludedTypes` and `ScopeMeta` gains `excludedTypes` (`yaml:
  "excludedTypes,omitempty"`, sorted by `scopeMeta`), filled from `prep.ExcludedTypes` in `cmd/resource/download.go`
  and `cmd/resource/drift.go` on every run — including `--type`, `--resource-id` and `--resource-group` runs,
  because the exclusion belongs to the profile, not to the selection. `RunScope.Types` stays the allow-list as
  asked, never the effective list, so `coverageLabel` still says `full` for an otherwise full run with exclusions.
  Coverage needs no change: excluded types are never requested, so they are neither covered nor skipped, and their
  earlier `types`/`resources` entries and files stay untouched (prune never removes them; deleting them is the
  operator's call).
- ✅ Drift comparability: `drift.Preflight` gains the current exclusion as a parameter and compares it, as a set, with
  the baseline's `run.scope.excludedTypes` (missing = empty); any difference returns an error wrapping
  `ErrNotComparable` — `runDrift` already maps it to exit 2 — naming the added and removed types and saying to run
  `resource download` first. The drift observation records the exclusion in its `run.scope` (`drift.go`, where
  `obs.Run.Scope` is built). `resource audit` needs no change — it only filters drift findings, which never contain
  excluded types.
- ✅ `--type` flag help and the comment above it (`internal/cmdutil/flags.go:36-38, 54`): it *replaces* the configured
  types for this run, not "narrows"; excluded types are still removed.
- ✅ Tests: config partition, shape errors, no-op examples; the selection function table-tested — all minus
  excluded; `type:` minus excluded; case-insensitive normalisation and duplicates; an unknown excluded name errors;
  an allow-list clash, an excluded `--resource-id` type and an excluded `resourceGroups` with `--resource-group`
  refuse; an unparseable id and an unrelated id/group pass; `SelectedTypeNames` fed the effective list contains no
  excluded type; `resource types` marks an excluded type and does not count it; metadata `excludedTypes` round-trip,
  sorted, omitted when empty (an existing fixture stays byte-identical), and a full run with exclusions is
  `complete` with `lastCoveredBy: full` and the excluded types' entries untouched; drift preflight — same set
  passes, different set refuses with `ErrNotComparable` naming added and removed, a baseline without the field plus
  a current exclusion refuses, neither passes; the observation's `run.scope.excludedTypes` is written.
- ✅ Documentation at *done*: `README.md` — profile keys and partition table, a selection subsection (allow-list, then
  exclusion, then the clash with `--type` / `type:` / `--resource-id` / `--resource-group`), `resource types`
  marking, the coverage rules and the `run.scope.excludedTypes` metadata example, the drift "must match" list and
  comparability paragraph; `CHANGELOG.md` `### Added`, with **after adding or changing `exclude-type`, run
  `resource download` before the next `resource drift`** in bold, and `### Fixed` for the `--type` help text.
