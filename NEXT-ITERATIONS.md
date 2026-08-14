# Next iterations

Open work only. Everything delivered so far is recorded in `CHANGELOG.md`.

`DOC-GENERATION-PROMPT.md` remains untouched — it is the full-export procedure. The incremental flow in
section 4 uses its own template, embedded at `internal/docs/generate_prompt_template.md`.

Every item below was re-verified against the source on 2026-08-14; the file:line references are where the
open behaviour actually lives.

---

## 1. Two missing tests

Both were specified with the work they cover and are the only gaps left in it. Neither exists yet — no test
in the tree references `AZURE_RD_LOG_LEVEL`, `SetEnvKeyReplacer`, or byte-identity of `metadata.yaml`.

**The env-key-replacer guarantee.** `initConfig` maps hyphens to underscores (`cmd/root.go:92`) so
`log-level` resolves to `AZURE_RD_LOG_LEVEL`. The behaviour works; nothing pins it down, so the fix can
silently regress into the exact bug it replaced — an override that is documented, believed, and inert. Set
the variable, assert it is read.

**Determinism of `metadata.yaml`.** Two runs over an unchanged export must produce byte-identical output.
`summary.Results` arrives in worker-completion order, and slices are sorted on the way in, but nothing pins
that down. Without the test, a future field added as an unsorted slice churns the file in git on every run
and nobody notices until the diffs are unreadable.

Note that the determinism test shipped under `internal/docs` covers `generate.md`, not `metadata.yaml`.
They are separate artifacts with separate sort paths; one does not stand in for the other.

## 2. `list` requires authentication to print a compile-time registry

`runList` constructs an Azure client purely to "ensure authentication works" (`cmd/list.go:48`), exits `1`
if that fails, then prints `registry.GetAllTypes()` — a list baked into the binary. So `azure-rd list` fails
without `az login`, or offline, to answer a question that needs neither. If `handlers.NewRegistry` tolerates
zero-value credentials for enumeration, `list` becomes usable as documentation.

## 3. `rootCmd.Version` is hardcoded

`toolVersion()` correctly makes the root command's version the single source of truth for what
`metadata.yaml` records — but that version is the literal `"1.0.0"` (`cmd/root.go:41`) and never moves when
the binary does.

This defeats a specific thing the metadata was built for. `promptSha256` comes from
`handler.GetDocumentationPrompt()`, a compile-time constant, so a tool upgrade changes the prompt hash for
every affected type at once. `toolVersion` is what lets a later step attribute that to an upgrade rather than
report it as content drift — and it can only do that if it actually changes. Wire it to
`debug.ReadBuildInfo()` or an `-ldflags -X` value.

---

## 4. `docs generate-prompt` — remaining work

The command ships and decides **list 1**: the documents to generate. Tenant resolution, `--domain`, the
excluded types and referenced-groups exception, the reference map, template splicing with marker validation,
exit codes, the dry-run stale-prompt warning — all delivered, see `CHANGELOG.md` → Unreleased.

What is left is **list 2** and the two hashes that drive it. The `resplice` and `migrate` markers are filled
by `renderNotImplemented(...)` (`internal/docs/generateprompt.go:210-211`), which states the gap in the
emitted prompt rather than claiming "none".

### Two lists, because a marked block can go stale on its own

**Content staleness and splice staleness are different sets.** Conflating them either regenerates documents
that only needed a table swapped, or leaves marked blocks silently wrong. Implementing list 1 alone — which
is where the command stands today — produces a tool that silently never re-splices.

**List 2 — documents to re-splice.** Body untouched, one marked block replaced. Two independent causes, one
per direction:

*Reverse — a group's `Targeted by` block.* Re-point a policy from group G1 to G2 and the policy's YAML
changes, so it lands in list 1 — but G1's and G2's reverse indexes are now wrong while *neither group's YAML
moved*.

*Forward — a document's own assignments table.* Rename group G in the tenant and G's document regenerates,
but every policy targeting G still prints G's **old name**. Those policies store only the GUID, so their
YAML never moved and nothing in list 1 catches them. The same applies to a renamed assignment filter, to a
group changing from assigned to dynamic, and to a group leaving the export entirely (its name must become
`⚠️ not in export`).

Both are detected the same way, and neither needs a download.

### `assignmentsSha256`: the forward-direction hash

Every generated document that carries an assignments block records, in its frontmatter, a hash of the
**resolved inputs** that block was rendered from — not of the resource alone:

```
for each assignment entry, sorted by (direction, targetKind, groupId, filterId):
    direction | targetKind | groupId | resolvedGroupName | groupTypes | securityEnabled
              | filterId | resolvedFilterName | filterType | intent | source
```

Anything that would change a cell in the rendered table changes the hash: a rename, a filter rename, a group
type change, a target added or removed, a group disappearing from the export. Nothing else does.

`targetKind` is the target's `@odata.type` and it is **not optional**. Built-in targets carry no `groupId`
(`assignmentGroupIDs` at `generateprompt.go:336-352` skips them for exactly that reason), so without it
`allLicensedUsersAssignmentTarget` and `allDevicesAssignmentTarget` hash identically while §5c renders them
as different cells (**All users** vs **All devices**) — a swap between the two would never restale. It also
makes the sort key unique again; two rows keyed only by an empty `groupId` sort unstably, which alone is
enough to make the hash non-deterministic.

The two group inputs this hash needs — `groupTypes` and `securityEnabled` — are already recorded per group in
`ResourceMeta` (`internal/docs/metadata.go:117-123`, written at `internal/pipeline/writer.go:289-291`), so
they cost nothing to read.

This mirrors `targetedBySha256` for the reverse direction, so list 2 has one mechanism and one shape:

| Direction | Hash | Stale when |
|---|---|---|
| Forward — the document's own assignments table | `assignmentsSha256` | any referenced group or filter changed name, kind or presence |
| Reverse — a group document's `Targeted by` | `targetedBySha256` | the set of resources targeting that group changed |

Both fields already parse out of document frontmatter (`internal/docs/generateprompt.go:276-277`); nothing
computes them yet.

Rules that keep this from misfiring:

- **The command computes both hashes and hands them to the agent**, exactly as it does for `sourceSha256` and
  `promptSha256`. The agent never computes a hash. Two implementations of one hash is how you get a document
  that re-splices on every run forever.
- **Both hashes live in document frontmatter, not `metadata.yaml`** — they describe the state a document was
  written in, which is the document's business, not the export's.
- **A document already in list 1 is excluded from list 2**; §5c gives it a fresh block anyway.
- **Omit the field for types with no assignments concept.** Missing field on a type that *does* have
  assignments means "never spliced" → needs splicing. See the open question below: that predicate does not
  exist yet.
- The hash covers the resolved *inputs*, not the rendered markdown, so changing how the table is formatted
  will not restale existing blocks. If that ever matters, mix a template version into the hash.

### The reference map is half of what the hash needs

The forward hash is only sound if the command and the agent render from the *same* facts. Today they do not,
in two places.

**The map carries no group kind.** `renderRefmap` (`internal/docs/generateprompt_render.go:123-150`) emits
`GUID → [displayName](docPath)` and nothing else, so template §5a tells the agent to open the referenced
groups' YAML for `groupTypes` / `securityEnabled` (`internal/docs/generate_prompt_template.md:399-404`) —
while the hash would derive the same two values from `metadata.yaml`. One rendered cell, two sources. Add
both to the map row and rewrite that paragraph of §5a; the facts are already in `ResourceMeta`, so this is a
renderer change, not a download change. (`membershipRule` stays a YAML read — §5c does not render it, and
§5b's group document is regenerated by list 1 whenever the rule moves.)

**There is no filter half at all.** `assignmentGroupIDs` extracts only `target["groupId"]`; nothing reads
`deviceAndAppManagementAssignmentFilterId` / `…FilterType`. So the hash inputs `filterId`,
`resolvedFilterName` and `filterType` have no source, a renamed filter cannot be detected, and there is no
dangling-filter equivalent of `DanglingGroupIDs` even though a filter deleted from the tenant produces the
same `⚠️ not in export` cell. `assignmentFilters` is an in-scope type with a `displayName` in
`ResourceMeta`, so the resolution is a second lookup built the same way as `groupKeyByID`.

Note the asymmetry that stays: groups are in scope *only when referenced* (`inScope`,
`generateprompt.go:239-248`), filters are in scope unconditionally. Referenced-ness is needed for the filter
lookup and for dangling detection, not for scope.

### The work list already has an empty `assignmentsSha256` column

`renderWorklist` emits an `assignmentsSha256` header and separator, then writes every cell blank — the
format string carries no argument for it (`internal/docs/generateprompt_render.go:109-113`), and `WorkItem`
(`generateprompt.go:80-87`) has no field to carry one. So filling the column is a struct change plus the
computation, not a format-string fix.

Harmless today because nothing reads the column back, but it collides with the rule above the moment list 2
lands: a blank cell and a genuinely absent hash are the same signal, and that signal means "never spliced →
needs splicing". Either fill the column with the computed hash or drop it until there is one. Do not ship a
third state that means neither.

### Open question — what marks a type as having assignments

Three rules depend on it: omitting `assignmentsSha256` for types without assignments, deciding which
documents §5e must migrate, and deciding which documents are candidates for a forward re-splice. It is not
derivable from `metadata.yaml`: `AssignmentTargets` is per resource, so a type whose resources happen to have
no assignments is indistinguishable from a type with no assignments concept — and the first is the common
case in a small tenant. Nothing in `TypeMeta` (`metadata.go:96-101`) or the handler registry records it
today. It wants a capability on the handler, surfaced into `TypeMeta` by the export, and that is a `download`
change — the only part of list 2 that is not confined to `internal/docs`. Decide this before writing the
hash, because the three rules above all read it.

### §5e is conditional, and cheap where the command already reads

Marker migration only applies to documents written before the assignment markers existed — a document with an
assignments table and no `<!-- assignments:start -->`. `staleReason` (`generateprompt.go:252-269`) already
reads every in-scope document's bytes to parse frontmatter, so both the marker check and the list-2
frontmatter comparison belong in that same pass; a second walk of the tree would double the I/O for nothing.
Include §5e in the emitted prompt only when at least one such document exists, with those documents listed.

### Plumbing that has to move with it

`GeneratePromptResult` (`generateprompt.go:91-108`) carries only `ToGenerate`; list 2 needs the forward and
reverse re-splice sets and the migration set as their own fields, and `reportGeneratePrompt`
(`cmd/docs/generate_prompt.go:219-259`) needs to print them. **Decide what `--exit-code` means then**: it
fires today on `len(res.ToGenerate) > 0` alone (`cmd/docs/generate_prompt.go:142-144`), so a CI gate would
report clean while every assignments table on disk prints a stale group name. Either it covers all three
sets, or the flag needs to say which one it watches.

### Noted gap — a transform-config change looks like a tenant-wide change

Change the transformer config and re-download, and every resource's `sourceSha256` moves, so every document
goes stale at once. That is arguably correct — the YAML content genuinely changed — but the report can only
say "190 stale" without saying why. The export side already records what would answer it:
`RunMeta.TransformConfigSha256` (`metadata.go:82`). Distinguishing the two therefore needs only the document
side — a `transformConfigSha256` in frontmatter and the prompt amendment that writes it. Noted, not planned.

---

## 5. `download --dry-run` still performs the full download

Flagged by the symmetry with `docs generate-prompt`, and a change to shipped behaviour rather than a bug — so
it needs a decision before anyone acts on it.

`Pipeline.Execute` starts all three stages unconditionally (`internal/pipeline/pipeline.go:76-82`). `DryRun`
reaches only the writer (`pipeline.go:43`); the fetcher and transformer never see it. So every resource is
listed, fetched from Azure with its retries and rate limiting, and transformed, then discarded at the write
stage. A dry run of a full export costs the same API traffic and wall-clock time as a real one, and produces
no file.

If `--dry-run` means "list the resources that would be downloaded", the run can stop after
`Registry.BuildFetchRequests` — `requests` is fully materialised at `pipeline.go:58` before any stage starts,
so this is an early return, not a restructuring of the stage wiring. The per-type listing calls that build
the request set live in the registry, upstream of the fetcher, so they are preserved. That drops the
per-resource fetches and makes the two commands consistent: neither performs resource fetches under
`--dry-run`.

**The trade-off is smaller than it looks.** The argument for keeping today's behaviour is that a dry run
exercises the fetch and transform paths and so catches per-resource permission errors. True — but it stops
there: `yaml.Marshal` and `buildResourceFacts` both sit *inside* the `if !w.dryRun` branch
(`internal/pipeline/writer.go:188-214`), so a marshalling failure or a facts-extraction bug already slips
through a dry run untouched. If the fetch-path check is worth keeping, it belongs behind its own flag rather
than as the meaning of `--dry-run`.

**One consequence to handle either way.** A list-only dry run leaves `summary.Results` empty, so the
completeness accounting and the invariant at `pipeline.go:135` (`len(summary.Results) != TotalResources`)
need an explicit dry-run branch. Left as-is, that invariant fires on every dry run.
