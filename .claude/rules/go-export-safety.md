---
paths:
  - "go/cmd/**"
  - "go/internal/**"
---

# Export safety: output layout, metadata, prune, drift, dry-run (`go/`)

## Output layout
Everything lives under `<output>/<tenant>/`, `<tenant>` being the Entra default domain resolved by
`internal/tenantdir` (never a bare-output fallback), in sibling trees that mirror each other:
- `resources/` — **written by `resource download` only.** `metadata.yaml` at its root, then
  `<APIType>/<endpoint>/<name>.yaml` (+ decoded sidecar artifacts, + the type's `doc-prompt.md`).
- `docs/` — the agent's tree. `azure-rd` writes exactly two files there, both at the root where no document
  can be: `generate.md` (`docs generate-prompt`) and `index.yaml` (`docs generate-index`). Documents are
  always `<APIType>/<endpoint>/<name>.md`, derived by swapping tree root and extension — never store a
  document path in `metadata.yaml`.
- `drift/` — owned by `resource drift`: `metadata.yaml` (the observation) at the root, payloads of
  added/changed/renamed resources at paths mirroring `resources/`; `resource audit` (and a drift run whose
  tenant profile sets `audit-workspace-id`, after the observation) writes `audit.yaml` — the attribution,
  facts only, never folded into the observation — atomically at the root; `docs analyze-drift` writes
  `analyze.md` at the root; the analysis agent writes `<key>.md` beside each payload and `index.md` at the root. Each drift
  run clears and rebuilds the tree, and a re-baselining `resource download` clears it too (`drift.ClearTree`,
  after a successful metadata write, never under dry-run) — `audit.yaml` included: swept with the tree, never
  pruned, no delete path of its own. No history, by design. Drift never writes under
  `resources/` or `docs/`, never updates the export's `metadata.yaml`, never prunes.
- `consistency/` — owned by `docs analyze-consistency`: `mechanical.yaml` (the same-setting findings) and
  `metadata.yaml` (the export it describes, `exportGeneratedAt`, and counts) at the root, written atomically,
  findings first and metadata last; other files there are left alone. It reads only `resources/` and describes
  one export, so a re-baselining `resource download` clears it (`consistency.ClearTree`, after a successful
  metadata write, never under dry-run). A resolved secret never reaches it: secret values are indexed as
  unknown and never written or logged.
- File names: display name sanitised (lowercase, `[a-z0-9_]`, `resource_` prefix for a leading digit,
  `unnamed` fallback); collisions resolved by lowest resource id + `sha256(id)` suffix — decided by id, never by
  finish order.

## `resources/metadata.yaml` — facts, and the prune contract
`prune` (config-only, no flag) is the **only** delete path inside the export; the only other deletes go
through `drift.ClearTree` and `consistency.ClearTree` and touch only `drift/` or `consistency/`. These rules
make that safe — never relax them:
- It describes the **export directory, not the tenant**. Never remove an entry while its file exists on disk;
  a resource gone from the tenant becomes `presentInTenant: false` with facts and hash retained. Only a prune
  that actually deleted the file removes an entry.
- **An incomplete run may not mark anything absent** — it cannot tell deleted from unreached.
- **"Covered" means the listing succeeded**, not that it returned resources: `EmptyTypes` is covered,
  `SkippedTypes` is not. Collapsing them turns a missing permission into a deletion.
- Prune refuses unless the run is `Complete` and `FailedResources == 0`; deletes only within covered types;
  never leaves `resources/`; never deletes `metadata.yaml`; removes a type's `doc-prompt.md` only when the
  type emptied out; never reaches into `docs/`, `drift/` or `consistency/` (an orphaned document is reported, not deleted).
  Preview and real path share one eligibility decision (`prunableKeys`). Every deletion is logged, with a total.
- **Partial runs merge, never truncate**: a `--type`-scoped run keeps entries and `lastCoveredAt` of types it
  did not cover. Skipped/filtered resources are re-observed (`skipped`/`filtered: true`), not rewritten.
- **Facts only.** Record a value only if it is read from the resource or computed from its bytes. Grouping,
  classification, change buckets and counts belong to the `docs` commands.
- `promptSha256` hashes the **assembled `doc-prompt.md` bytes on disk** (`content.String()` in
  `Writer.writePromptFiles`), never the raw prompt string — otherwise every later comparison reports every
  type changed.
- Every written entry records the `transformConfigSha256` of the run that produced it; the run records
  `filtersSha256`. `resource drift` refuses (exit 2) across a transform/filter config change, treats entries
  without a matching per-entry hash as *unattested* (reported, never counted as drift), gates removals on the
  complete-and-covered rule, and hashes the same marshalled bytes the writer hashes
  (`pipeline.MarshalResourceYAML` + the shared `NamePlanner`).
- `metadata.yaml` is written atomically (temp file + rename in `writeMetadata`); resource writes deliberately
  are not. Files `0644`, directories `0755`.

## Dry-run
`--dry-run` writes nothing and never updates `metadata.yaml`, and nothing downstream is recalculated to
compensate. A command whose real work is a download may skip the download and answer only what is answerable
offline — worded as a subset, never as a preview. `resource drift --dry-run` still fetches and compares in full
and withholds only the `drift/` tree (clearing nothing, reporting an earlier observation as not refreshed).
With `prune` configured, dry-run lists exactly what a real run would delete, from the same selection.

## Config example files
`config.example.yaml` (general) and `config.example.domain.yaml` (tenant-scoped) are reference schemas with
a no-op promise: loading either unmodified behaves **byte-for-byte** like running without it, every hash
included. Update both on **any** option change; every active key sets the built-in default; keys whose mere
presence changes behaviour (`workers`) or whose defaults would move a hash (`transformers` sub-settings) are
illustrated in comments only. `cmd/config_test.go` asserts every partition key appears in the right file.

## Secrets and sensitive output
- `resolve-secrets` (config-only) writes decrypted Intune OMA-URI secrets to disk **in plaintext**; it must
  log a warning and needs `DeviceManagementConfiguration.ReadWrite.All`.
- Never log tokens, client secrets or resolved values. ARM handlers drop `adminPassword`, keys and connection
  strings in `Transform()`.
- Regeneration-gated changes: editing `internal/models/documentation_prompt.tmpl` or any `*_prompt.tmpl`
  moves `promptSha256` and forces a documentation regeneration for every affected type. Batch such changes
  (see `.claude/rules/next-iterations.md`); prefer changes that ride the non-hashed `docs/generate.md` or
  `drift/analyze.md` templates.
