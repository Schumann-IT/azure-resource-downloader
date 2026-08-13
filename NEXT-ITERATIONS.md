# Next iterations — a reliable export and an export metadata file

Implementation prompt for the Go side. Describes work that is **not yet done**.

`DOC-GENERATION-PROMPT.md` is **not touched by this work**, and nothing here decides where generated
documentation lives — that is deferred to a later reshaping of the post-processing step.

Four changes, in dependency order:

1. **Fix the CLI surface** — two bugs and the flag structure, before anything builds on it. — **done**
2. **Move downloaded files under `resources/`** — a directory the tool exclusively owns. — **done**
3. **Make the run's own completeness knowable** — without it, metadata cannot be trusted and prune is
   unsafe. — **done, tested**
4. **Emit `metadata.yaml`**, and add **`download --prune`** on top of it. — **done**

---

## Status — reviewed against the implementation

All four sections are implemented. What follows stays in this file as the rationale record; the sections are
annotated with what landed. Verified in `cmd/flags.go`, `cmd/root.go`, `cmd/list.go`, `cmd/download.go`,
`internal/pipeline/*`, `internal/models/types.go`, `internal/docs/metadata.go`.

### Remaining work

**Tests — the completeness gap is now closed.** `internal/pipeline/pipeline_test.go` covers the section 3
work: `MarkCompleteness` (complete, missing-results, cancelled, skipped-types, and multiple reasons joined),
the drain-on-cancel behaviour in all three stages (fetcher, transformer, writer each emit exactly one result
per input under a cancelled context), the upstream-status propagation each stage performs, and — via
`TestPipelineStagesAccountForEveryRequestOnCancel` — the three stages wired together producing exactly one
result per request under cancellation, which is the guarantee the `len(Results) == TotalResources` assertion
in `Execute` rests on. Run under `-race` (`make test-race`) since it is concurrent code.
`internal/docs/metadata_test.go` covers merge, absence, prune and prune-refusal well.

Two smaller test gaps this plan asked for by name:

- the env-key-replacer guarantee (set `AZURE_RD_LOG_LEVEL`, assert it is read) — no test references it
- determinism: two runs over an unchanged export producing byte-identical `metadata.yaml`

**`--dry-run --prune` does not list what it would remove.** — **closed.** `WriteExportMetadata` now runs the
merge in-memory before the dry-run branch and calls `previewPrune`, which shares `prunableKeys` with the real
prune so the preview can never diverge from an actual deletion. Dry-run write results (YAMLPath set, no facts)
are counted as present in `mergeMetadata`, so absence detection — and the preview built on it — is accurate.

**Prune logs each deletion but no total.** — **closed.** `pruneCovered` now logs
`Prune: complete deleted=<n> candidates=<m>`, and `previewPrune` logs `would_delete=<n>`.

**Still deferred, unchanged:** `list` requires authentication to print a compile-time registry, and
`rootCmd.Version` is still the hardcoded `"1.0.0"`. `toolVersion()` at least makes it a single source of
truth for what lands in `metadata.yaml`, but it will not move when the binary does — which defeats the
"this type re-hashed because you upgraded" attribution `promptSha256` was meant to support.

### Landed beyond this plan

- `--write-prompts` became `--no-prompt`: documentation prompts are now written by default.
- `cmd/prompt.go` — an interactive prompt for the dedicated app registration (client/tenant ID), driven by a
  per-type permission probe that decides when the Azure CLI app is insufficient. Not part of this plan.
- `config.example.yaml` documents `resource-id`, `no-prompt` and `prune`, and the `timeout` comment now says
  per-operation — which closes the three-way description conflict this plan had deferred.

---

## 1. CLI surface: two bugs and one structural fix — DONE

Both bugs fixed and the flag structure rebuilt as specified. `cmd/flags.go` holds `addAzureAuthFlags` /
`addSelectionFlags` / `addPipelineFlags` plus a per-execution `bindFlags`, with the defaults as named
constants so the value-sniffing bug cannot recur. Root keeps only `--config`, `--output`, `--dry-run` and
`--log-level`; `list` opts into auth alone. Outstanding: no test for the env-key-replacer guarantee.

### Bug: environment overrides are silently broken for every hyphenated key

`initConfig` sets `viper.SetEnvPrefix("AZURE_RD")` and `viper.AutomaticEnv()` but never
`SetEnvKeyReplacer`. Viper therefore looks up `AZURE_RD_` + `ToUpper(key)` — so key `log-level` resolves to
`AZURE_RD_LOG-LEVEL`, which is not a name a POSIX shell can export.

Working today: `subscription`, `output`, `workers`, `type`, `timeout`.
**Not working:** `log-level`, `dry-run`, `client-id`, `tenant-id`, `resource-group`, `resolve-secrets`,
`write-prompts`.

Both `root.go`'s own comment and `download.go`'s promise the opposite ("precedence: flag > env > config >
default"). Fix:

```go
viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
```

Add a test that sets `AZURE_RD_LOG_LEVEL` and asserts it is read, so the guarantee stops being a comment.

### Bug: `--workers 5` is indistinguishable from not passing the flag

`determineWorkerCount` detects explicit use by comparing against the default literal:

```go
// Priority 1: Check if --workers CLI flag was explicitly set (highest priority)
if workersFlag != 5 {
    return workersFlag
}
```

Two problems. You cannot explicitly request 5 workers for an ARM type — `workerConfig.GetWorkerCount` returns
the API default of 20 and your request is discarded. And the default `5` is duplicated here as a magic
literal from `root.go`, so changing it there silently inverts this branch.

Fix: pass explicitness rather than sniffing the value — `cmd.Flags().Changed("workers")` at the call site,
threaded into `determineWorkerCount` as a `bool`. That also makes the existing precedence comments true
instead of approximately true.

### Structural: the selection triad is declared three different ways

`--resource-id`, `--type` and `--resource-group` do one job — choose what to download — and are declared
inconsistently:

| Flag | Where | viper-bound | Read via |
|---|---|---|---|
| `--type` | root, persistent | yes | `viper.GetStringSlice("type")` |
| `--resource-group` | root, persistent | yes | `viper.GetString("resource-group")` |
| `--resource-id` | download, local | **no** | the package variable `flagResourceIDs` |

Consequences: `type:` and `resource-group:` can be set in a config file but `resource-id:` cannot (it is
absent from `config.example.yaml` for exactly this reason); `runDownload` reads one of the three differently
from its siblings; and `azure-rd list --help` advertises `--type` and `--resource-group`, which `list`
silently ignores — a flag that looks like it should filter the output and doesn't.

Fix: all three become download-local and all three are viper-bound. Add `resource-id` to
`config.example.yaml`.

### Structural: replace persistent-by-default with explicit flag groups

Five persistent flags are read only by `download`. Every future command — prune, post-processing — inherits
them regardless of meaning. Replace inheritance with opt-in:

```go
// cmd/flags.go
func addAzureAuthFlags(cmd *cobra.Command)   // subscription, client-id, tenant-id
func addSelectionFlags(cmd *cobra.Command)   // resource-id, type, resource-group
func addPipelineFlags(cmd *cobra.Command)    // workers, timeout
```

- **Root keeps** `--config`, `--log-level`, `--output`, `--dry-run`.
- **`download`** calls all three helpers, plus its own `--resolve-secrets`, `--write-prompts` and later
  `--prune`.
- **`list`** calls `addAzureAuthFlags` only.
- Later commands opt into what they need.

This also gives the `BindPFlag` calls one home instead of two, and lets `--client-id` / `--tenant-id` get the
`MarkFlagsRequiredTogether` their help text already claims ("required with `--client-id`") but nothing
enforces.

`--output` and `--dry-run` stay global deliberately: every planned command needs the export root, and
`--dry-run` is a safety flag worth having uniformly available. It is inert on `list`, which is harmless —
unlike `--type` on `list`, which is misleading.

### One compatibility note

Persistent flags may be given **before** the subcommand; local flags may not. So moving `--type`,
`--resource-group` and `--workers` down to `download` breaks `azure-rd --type X download`, which works today.
`azure-rd download --type X` is unaffected. Worth a line in the changelog and a check of any scripts or
Makefile targets that use the leading form.

### Deferred to a later pass

`--timeout`'s three conflicting descriptions (covered in section 3, where its semantics are decided), `list`
requiring authentication to print a compile-time registry, and `Version: "1.0.0"` being hardcoded while
`metadata.yaml` is about to record `toolVersion`.

---

## 2. Export layout — DONE

`models.ResourcesDirName` with `Writer.resourcesDir()` used by both `writeResource` and `writePromptFiles`;
`metadata.yaml` written at `resources/metadata.yaml`.

```
output/<domain>/
  resources/metadata.yaml
  resources/<APIType>/<endpoint>/<resourcename>.yaml
  resources/<APIType>/<endpoint>/doc-prompt.md
```

Concretely:

```
output/cb-gmbh.com/resources/Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.yaml
output/cb-gmbh.com/resources/Microsoft.Graph/deviceCompliancePolicies/doc-prompt.md
```

Sidecar artifacts (decoded payloads written by `Writer.writeResource`) follow their YAML into `resources/`.

Everything under `resources/` is written exclusively by `azure-rd`. `doc-prompt.md` belongs there because the
tool generates it — it reads like documentation but is not authored by anyone else. `metadata.yaml` sits at
the root of that tree for the same reason, and because it describes exactly that tree and nothing else. That
ownership boundary is what later makes `--prune` safe to reason about: prune operates only inside a tree
nothing else writes to.

### Go changes

Two `filepath.Join` calls in `internal/pipeline/writer.go`:

- `writeResource`: `filepath.Join(w.outputDir, transformResult.ResourceType)` →
  `filepath.Join(w.outputDir, "resources", transformResult.ResourceType)`
- `writePromptFiles`: the same substitution for `resourceTypeDir`

---

## 3. Make the run's completeness knowable — DONE

All four required changes landed: every stage drains on cancel and emits a `Cancelled` result per request,
`Execute` returns an error if `len(Results) != TotalResources`, `ExecutionSummary` gained
`Complete` / `IncompleteReason` / `CancelledResources` with `MarkCompleteness`, and `--timeout` is now applied
per fetch inside `Fetcher` rather than around the whole pipeline. Tests now cover all of it in
`internal/pipeline/pipeline_test.go` (see the status block above); run with `make test-race`.

**This was the prerequisite for everything else.** A metadata file is only as trustworthy as the run's own
knowledge of what it did and did not fetch. Right now a run cannot tell you it was incomplete, and there is
a concrete path to silent data loss once prune exists.

### The gap: cancelled requests produce no result at all

All three pipeline stages share this shape (`fetcher.go:63`, `transformer.go:61`, `writer.go:74`):

```go
select {
case <-ctx.Done():
    results <- &models.FetchResult{ResourceID: req.ResourceID, Error: ctx.Err()}
    return          // <- abandons the rest of the input channel
default:
    ...
}
```

On cancellation each worker emits **one** error result and returns, abandoning whatever is still queued.
`requestsChan` is buffered with every request up front, so once all workers have returned, the remaining
requests are never read and **never produce a result of any kind**. `wg.Wait()` completes, the output channel
closes, and the run reports success.

The consequence: `summary.TotalResources` is `len(requests)`, but `len(summary.Results)` can be smaller, and
nothing compares them. `PrintSummary` shows totals that do not add up, and no code path notices.

### Why this becomes data loss

With `metadata.yaml` and `--prune` in place, a resource that produced no `WriteResult` looks exactly like a
resource that was deleted in the tenant — absent from a covered type. It would be marked
`presentInTenant: false` and then **deleted from disk**. A timeout would silently delete resources that exist.

Note that guarding prune on `summary.FailedResources > 0` does **not** protect against this: unprocessed
requests produce no failure, they produce nothing at all.

### The timeout that triggers it

`--timeout` is documented as *"per-operation timeout in seconds"* and defaults to 300. It is not
per-operation: `Pipeline.Execute` applies it as a single `context.WithTimeout` around the **entire run**.

```go
if p.config.Timeout > 0 {
    ctx, cancel = context.WithTimeout(ctx, p.config.Timeout)
}
```

905 resources, a bounded worker pool, Graph rate limits and `retry.DefaultConfig()` backoff — which consumes
the same global deadline — makes 300 seconds for the whole export easy to exceed. So the trigger for the bug
above is not exotic; it is the default configuration on a real tenant.

### Required changes

1. **Account for every request.** When a stage is cancelled, drain its input channel and emit a result marked
   cancelled for each remaining item, rather than returning early. Every request must produce exactly one
   result.
2. **Assert the invariant.** After `Execute`, `len(summary.Results)` must equal `summary.TotalResources`. If
   it does not, that is a bug in the pipeline, not a condition to tolerate — fail loudly.
3. **Add a `Complete bool` to `ExecutionSummary`**, plus the reason when false. A run is complete when every
   request produced a result, no stage was cancelled, and no type failed to list. `PrintSummary` states it
   plainly.
4. **Make `--timeout` per-operation**, matching its documentation: apply it around `handler.Fetch` inside
   `fetchResource` rather than around the whole pipeline. If a whole-run budget is still wanted, it should be
   a separate, explicitly named flag — not the one whose help text promises per-operation semantics.

### Completeness is per type, not just per run

Two existing fields already carry the distinction that matters, and metadata depends on it:

| Field | Meaning | Can absence be trusted? |
|---|---|---|
| `summary.EmptyTypes` | listing succeeded, returned zero resources | **Yes** — the type genuinely has no resources |
| `summary.SkippedTypes` | listing failed (permissions, no subscription) | **No** — the resource count is unknown |

Collapsing these into "nothing came back" is what turns a missing permission into a deletion.

---

## 4. `metadata.yaml` — DONE

`internal/docs/metadata.go` implements the shape, the merge and the coverage rules as specified:
`coveredTypes` treats `EmptyTypes` as covered and removes `SkippedTypes`, absence is only detected on a
complete run, and skipped/filtered resources retain their prior facts by resource-id lookup. Facts are built
in `Writer.buildResourceFacts` from the marshalled bytes; `promptSHAByType` hashes the assembled
`doc-prompt.md` content. Missing: a determinism test.

### Purpose

Make it possible for a later post-processing step to determine what changed since the last export without
re-reading and re-parsing every file — and to know whether the export it is looking at is complete.

### The split: facts vs decisions

| | Facts | Decisions |
|---|---|---|
| Examples | `sourceSha256`, `displayName`, `odataType`, raw assignment targets, artifact filenames | grouping, classification, change buckets, counts |
| Change when | the resource changes | **you** change your mind |
| Need | the YAML bytes / the parsed map, in memory | a rule table and the complete export |
| Computed | **in the pipeline** (this iteration) | in a post-process command (later) |

`metadata.yaml` contains **facts only**. If a field represents a choice you might revise, it does not belong
here — otherwise revising the choice means re-downloading a tenant.

### What the pipeline can supply, and from where

All of this is already in memory at write time. Verified against the current code.

| Fact | Source | Note |
|---|---|---|
| `sourceSha256` | `sha256.Sum256(yamlData)` in `Writer.writeResource` | The marshalled bytes exist only here. Recovering it later means re-reading every file. |
| resource path (the entry key) | `filepath.Rel(resourcesDir, yamlPath)` | `WriteResult.YAMLPath` already carries the absolute path. Relative to `resources/`, not to the tenant root. |
| `resourceType` | `TransformResult.ResourceType` | |
| `displayName` | `TransformResult.DisplayName` | **Not recoverable from disk.** It comes from `handler.Transform`, so the five singletons whose YAML has no `displayName` field — `applePushNotificationCertificate`, `depOnboardingSettings`, `deviceManagement`, `mobileThreatDefenseConnectors`, `onPremisesSynchronization` — already have a name here. `deviceManagementConfigurationPolicies` uses `name` rather than `displayName`; the handler normalises that too. |
| `resourceId` | `TransformResult.ResourceID` | Lets a later diff distinguish a genuine edit from a resource whose `id` was regenerated. |
| `odataType`, `platforms`, `technologies` | `CleanedData` map lookup | No re-parse. `@odata.type` was absent from 370 of 905 files in the last full export — treat as nullable, never as a primary key. |
| raw assignment targets | `CleanedData["assignments"]` | Per-resource only. Resolving GUIDs to names is a cross-resource join and belongs post-process. |
| artifact filenames | `TransformResult.Artifacts` | **Only the pipeline knows these** — which sidecar files exist for a resource is otherwise a directory guess. |
| `promptSha256` per type | `Writer.writePromptFiles` | Lets a later step tell whether a type's generated spec changed. See gotchas. |
| skipped / filtered + reason | already on `WriteResult` | So counts reconcile against the raw file count. |

### Run context, available only in `runDownload`

Tenant domain (already resolved via `azureClient.GetTenantDomain`, and it is the output folder name),
subscription, tool version, timestamp, the **run scope** (`--type` / `--resource-id` / `--resource-group`),
the effective transformer and filter config, `--resolve-secrets`, `--write-prompts`, `--dry-run`, the
completeness verdict from section 3, and `summary.SkippedTypes` / `summary.EmptyTypes`.

The config-shaped values earn their place: **transformer config and `--resolve-secrets` change the YAML
bytes.** Flip either and every hash in the export moves. Recording a hash of the effective transform config
lets a later diff report *"every resource hashes differently because the transform config changed"* instead
of presenting a mass edit that never happened.

### Explicitly out of scope for the pipeline

The previous `metadata.yaml`, anything about generated documentation, the assignment reference map, and every
classification decision. All are either state this run did not produce, or judgements that must stay
re-runnable without a re-download.

### Shape

Written at `resources/metadata.yaml` — the root of the tree it describes. YAML, to match the tool's own
output and because this file should diff in git between exports.

Resource keys are paths **relative to `metadata.yaml`'s own directory**, so the file and the tree it
describes move together and no key encodes the word `resources`. That also makes a separate `source:` field
redundant: the key *is* the path.

```yaml
generatedAt: 2026-08-13T11:19:37Z
tenant: cb-gmbh.com
toolVersion: azure-rd 0.x
run:
  complete: true                  # every request produced a result; no type failed to list
  incompleteReason: ""
  scope:
    types: []                     # empty = no --type filter, i.e. a full export
    resourceIds: []
    resourceGroup: ""
  transformConfigSha256: "…"      # explains mass hash movement
  resolveSecrets: true
  writePrompts: true
  pruned: false
types:
  Microsoft.Graph/deviceCompliancePolicies:
    promptSha256: "…"             # over the assembled doc-prompt.md bytes — see gotchas
    promptFileWritten: true
    lastCoveredAt: 2026-08-13T11:19:37Z
    lastCoveredBy: full           # or: --type, --resource-id, --resource-group
resources:
  Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.yaml:
    resourceId: c490b0b8-bd4c-48d6-866f-45e2fba2be7c
    displayName: GBL_C_PRD_D_WIN_OS-Validation
    sourceSha256: "5d6b32f8…"
    odataType: "#microsoft.graph.windows10CompliancePolicy"
    artifacts: []
    presentInTenant: true         # false = file still on disk, resource gone from the tenant
    lastSeenAt: 2026-08-13T11:19:37Z
    assignmentTargets:
      - {type: groupAssignmentTarget, groupId: e0c6f42d-…, filterType: none}
notListed:
  types: []                       # summary.SkippedTypes — permissions; count unknown
  empty: []                       # summary.EmptyTypes
```

**Deterministic output.** `summary.Results` arrives in worker-completion order, which is nondeterministic.
Sort every map key and every slice before marshalling, or the file churns in git on every run for no reason.
A test should assert two runs over an unchanged tenant produce byte-identical output.

### Two gotchas that fail silently

**`promptSha256` must hash the assembled file, not the prompt string.** `writePromptFiles` writes
`"# Documentation prompt for <type>\n\n"` + an HTML comment + `prompt` + `"\n"`. Hash `content.String()`, not
`TransformResult.DocumentationPrompt` — otherwise the pipeline's hash never matches the file on disk, and
every later comparison reports every type as changed.

**`handler.GetDocumentationPrompt()` is a compile-time constant per handler.** So `promptSha256` only moves
when the binary does: a tool upgrade changes it for every affected type at once. Record `toolVersion`
alongside it so that can be attributed rather than read as content drift.

### Go changes

1. **`internal/models/types.go`** — add a `Facts *ResourceFacts` field to `WriteResult`; define
   `ResourceFacts` in the same package. `TransformResult` needs no change; it already carries everything.
2. **`internal/pipeline/writer.go`** — in `writeResource`, hash `yamlData` and populate `Facts`; in
   `writePromptFiles`, hash the assembled content per type into a new accumulator, in the exact shape of the
   existing `promptsByType` map guarded by `w.mu`.
3. **`internal/pipeline`** — the completeness work from section 3.
4. **`cmd/download.go`** — one call near the end of `runDownload`:

```go
if err := docs.MergeExportMetadata(output, summary, runScope); err != nil {
    log.Warn("Export metadata not written", "error", err)
}
```

5. **`internal/docs`** (new package) — owns the file format, the merge, the prune set, and marshalling.

**Do not refactor `runDownload` for this.** It is long, but it is long with orchestration, and its
sub-builders (`buildWorkerConfig`, `buildTransformerConfigs`, `buildResourceFilters`) are already extracted.
It should grow by one call. Anything that reads or parses a file does not belong in it.

### Behaviour requirements

- **Never fail the download for metadata.** A successful fetch of 905 resources followed by a marshalling
  error still exits 0 with a warning. The YAML is the valuable output.
- **Write metadata before the failure exit.** `runDownload` calls `os.Exit(1)` when
  `summary.FailedResources > 0`. Metadata must be written above that call and the run marked incomplete — a
  partially failed download is exactly when a record of what did land is most useful.
- **`--dry-run` writes nothing**, and says what it would have written.
- **Partial runs merge, never truncate.**

### Merge semantics for partial runs

A `--type mobileApps` run against a 29-type export must not produce a `metadata.yaml` describing one type.
Read the existing file, apply this run's results, write the union back.

Every type carries `lastCoveredAt` and `lastCoveredBy`. Without them a union file is indistinguishable from a
fresh one, and a later step cannot tell that some entries describe an export state several runs old.

**"Covered" means the type's listing succeeded in this run** — not that it produced resources:

| Situation | `ExecutionSummary` | Covered? | Effect on existing entries |
|---|---|---|---|
| Type fetched normally | `Results` | yes | Replace this type's resource entries with what this run produced. |
| Listing succeeded, zero resources | `EmptyTypes` | **yes** | Mark this type's resource entries `presentInTenant: false`. Do not remove them. |
| Listing failed (permissions, no subscription) | `SkippedTypes` | **no** | Retain everything untouched. Do not advance `lastCoveredAt`. |
| Type not in this run's scope | — | no | Retain everything untouched. Do not advance `lastCoveredAt`. |

**If the run is not complete, no entry may be marked `presentInTenant: false` at all.** An incomplete run
cannot distinguish "deleted in the tenant" from "never got to it".

Within a covered type, on a complete run:

- A resource present before and absent now was **deleted in the tenant**. Set `presentInTenant: false`,
  **retain its previous facts including `sourceSha256`**, and leave `lastSeenAt` at the last run that saw it.
- A resource excluded by a configured filter is `Filtered`, not absent. If it has a previous entry, retain its
  facts and mark it filtered — its YAML from an earlier run is still on disk. If it has no previous entry it
  was never written, so record it with its reason and no hash.
- A resource the signed-in user could not read is `Skipped`. **Retain its previous facts unchanged** and mark
  it — its YAML from an earlier run is still on disk and still valid, so its hash is still correct. Dropping
  it would report a spurious change; dropping only the hash would report a spurious unknown.

#### Never remove an entry for a file that still exists

**`metadata.yaml` describes the export directory, not the tenant.** That is the rule the three cases above
follow from.

Without `--prune`, nothing deletes files — there is no `os.Remove` anywhere in `cmd/` or `internal/` today.
So a resource deleted in the tenant keeps its `.yaml`; a resource newly excluded by a `filters:` change keeps
its too, it merely stops being rewritten; and a `transformers:` change that alters name sanitization
*renames*, leaving the old file beside the new one.

If such an entry were removed from metadata, the next run would find a YAML on disk with no entry describing
it and treat it as new — forever, on every run. Change detection compares hash-of-file against
hash-in-metadata, so metadata must account for every file present, including files that should not be. "Gone
from the tenant" is an attribute of an entry, never a reason to delete one.

The corollary: **when `--prune` does delete a file, its entry goes too** — the file no longer exists, and the
rule is about describing the directory.

---

## 5. `download --prune` — DONE

`pruneCovered` runs after the merge, refuses on an incomplete run or any failure, deletes only within covered
types inside `resources/`, removes sidecar artifacts, and removes `doc-prompt.md` plus the type directory only
when a type empties out. Metadata is written once, without the pruned entries, with `run.pruned` set. Both
earlier gaps are now closed: `--dry-run --prune` enumerates exactly what a real run would delete (via
`previewPrune`, sharing `prunableKeys` with the real prune), and the real prune logs a deletion total. Covered
by `TestDryRunPrunePreviewTouchesNothing` and `TestPrunableKeysSelectsAbsentCoveredEntries`.

Opt-in flag. Removes files under `resources/` that this run establishes are no longer in the tenant, so the
export stops accumulating.

### It runs after the download, not before

The end state is identical either way, and prune-first has no recovery path. Any failure between the delete
and a successful fetch leaves an emptied `resources/` with nothing to restore from — and section 3 shows that
failure is not exotic, since the default 300-second whole-run deadline is reachable on a real tenant.

The decisive case is one the codebase already models: **`summary.SkippedTypes`**. Prune-first with no
`--type` would delete a type's files, then discover the type cannot be listed, and they are gone until
someone re-grants the permission.

Prune-after also needs no scoping logic of its own: **the delete set is exactly the entries the merge marked
`presentInTenant: false` in covered types.** That sidesteps a problem prune-first has — `filters:` are
per-resource property regexes applied *after* fetch, so "delete everything matching my filters" would require
parsing every existing YAML to find out which ones match. After the download you simply know what this run
produced.

### Rules

- **Only ever deletes inside `resources/`.** Never the tenant root, never any tree the tool does not own —
  and never `resources/metadata.yaml` itself, which lives in the pruned tree but is not part of it.
- **Refuses to run unless `summary.Complete` is true.** This is the guard that matters — an incomplete run
  cannot tell a deleted resource from one it never reached.
- **Only within covered types.** A type in `summary.SkippedTypes` is never pruned.
- **Skips entirely when `summary.FailedResources > 0`.** A resource that failed to fetch is not evidence that
  it was deleted.
- Deletes the resource `.yaml` and its sidecar artifacts. `doc-prompt.md` is deleted only when an entire type
  directory is being removed — otherwise the run rewrites it.
- **`--dry-run --prune` deletes nothing** and prints the list it would remove.
- Log every deletion at info level and report the total. This is the first delete path in the codebase; it
  should be loud.
- After a successful prune, write `metadata.yaml` **without** the pruned entries, and set `run.pruned: true`.
  One write, not two.

---

## Later, deliberately not decided here

Where generated documentation lives, how the post-processing step consumes `metadata.yaml`, and any change to
`DOC-GENERATION-PROMPT.md`. Nothing in this iteration constrains those beyond keeping downloaded files inside
`resources/`.
