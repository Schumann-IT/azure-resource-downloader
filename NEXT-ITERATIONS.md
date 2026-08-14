# Next iterations

Open work only. Everything delivered so far is recorded in `CHANGELOG.md`.

`DOC-GENERATION-PROMPT.md` remains untouched — it is the full-export procedure. The incremental flow in
section 4 uses its own template, embedded at `internal/docs/generate_prompt_template.md`.

---

## 1. Two missing tests

Both were specified with the work they cover and are the only gaps left in it.

**The env-key-replacer guarantee.** `initConfig` maps hyphens to underscores so `log-level` resolves to
`AZURE_RD_LOG_LEVEL`. Nothing tests it, so the fix can silently regress into the exact bug it replaced — an
override that is documented, believed, and inert. Set the variable, assert it is read.

**Determinism of `metadata.yaml`.** Two runs over an unchanged export must produce byte-identical output.
`summary.Results` arrives in worker-completion order, and slices are sorted on the way in, but nothing pins
that down. Without the test, a future field added as an unsorted slice churns the file in git on every run
and nobody notices until the diffs are unreadable.

## 2. `list` requires authentication to print a compile-time registry

`runList` constructs an Azure client purely to "ensure authentication works", then prints
`registry.GetAllTypes()` — a list baked into the binary. So `azure-rd list` fails without `az login`, or
offline, to answer a question that needs neither. If `handlers.NewRegistry` tolerates zero-value credentials
for enumeration, `list` becomes usable as documentation.

## 3. `rootCmd.Version` is hardcoded

`toolVersion()` correctly makes the root command's version the single source of truth for what
`metadata.yaml` records — but that version is the literal `"1.0.0"` and never moves when the binary does.

This defeats a specific thing the metadata was built for. `promptSha256` comes from
`handler.GetDocumentationPrompt()`, a compile-time constant, so a tool upgrade changes the prompt hash for
every affected type at once. `toolVersion` is what lets a later step attribute that to an upgrade rather than
report it as content drift — and it can only do that if it actually changes. Wire it to
`debug.ReadBuildInfo()` or an `-ldflags -X` value.

---

## 4. New command: `docs generate-prompt`

Emits a ready-to-use documentation prompt covering exactly the resources whose documentation is missing or
out of date.

**It never fetches a resource and never touches `resources/`.** The workflow is two independent steps:

```
1. azure-rd download               # tenant -> export   (refreshes resources/ and metadata.yaml)
2. azure-rd docs generate-prompt   # export -> docs     (writes one prompt file; reads everything else)
```

It authenticates (see below) but downloads nothing: no per-resource fetches, no rate limiting, no retries,
nothing in the export mutated. Run it as often as you like against an export of any age — the answer depends
only on files already on disk.

### Which tenant

The command signs in with the Azure CLI session exactly as `download` does — `az login` first — and resolves
the tenant's Entra default domain, which *is* the export folder name. That guarantees both commands agree on
the folder with no flag to get wrong or out of sync.

Wire it with `addAzureAuthFlags` from `cmd/flags.go`, plus `bindFlags(cmd)` at the top of `RunE`, so
`--subscription` / `--client-id` / `--tenant-id` behave as they do elsewhere. It needs nothing beyond the
domain, so the plain CLI credential is enough — no dedicated app-registration prompt.

**Suggested escape hatch: `--domain <domain>`, which skips authentication entirely.** The command's inputs
are otherwise all local, so with the folder named explicitly it runs offline, in CI, and without credentials.
Worth having, since sign-in would otherwise be the only reason it needs a network at all. If `output/` holds
exactly one directory containing `resources/metadata.yaml`, defaulting to it is also reasonable — but do not
guess when there are several.

A tenant directory is one containing `resources/metadata.yaml`. That file's `tenant:` field must match the
resolved domain; if it does not, stop rather than document the wrong export.

### What is documented

`metadata.yaml` describes **every** exported resource, most of which are not worth a page. Two types are
excluded from documentation:

- `groups` — **except those referenced by an assignment**
- `windowsAutopilotDeviceIdentities` — no exceptions

They are bulk directory records: thin, near-identical, and in the reference tenant **740 of 905 exported
objects**. Documenting them costs enormously and produces pages nobody reads.

The referenced-groups exception is not optional. A group some policy actually assigns is the answer to "who
gets this?", and every assignment table links to one — exclude them all and every link points at nothing. In
the reference tenant that was 21 of 470 groups. The referenced set is computable from `assignmentTargets` in
`metadata.yaml`, so the command decides it; the agent never does.

Hardcode the exclusion list as a named constant in `internal/docs` rather than adding config surface — these
are structural facts about the Graph API, not per-tenant preferences. If a one-off override is ever needed,
`--include-type` is the smaller change.

### Outputs

| | without `--dry-run` | with `--dry-run` |
|---|---|---|
| `download` | writes resource YAML files and `metadata.yaml` | writes nothing; lists the resources it would download |
| `docs generate-prompt` | writes a complete documentation prompt for the stale resources | writes nothing; lists the resources whose documentation needs refreshing |

Both modes run the identical comparison; `--dry-run` only withholds the prompt file.

The deliverable is a **written prompt file**, not a printed list: everything a documentation run needs in one
artifact — the resources to document, each one's source path, and the `doc-prompt.md` that governs its type.

It is written to **`output/<tenant>/docs/generate.md`**, overwritten on every run, with an `--out` override.
That is the one file `azure-rd` writes under `docs/`, and it sits at the root of that tree where no document
can ever be — documents are always at least two levels deep, under `<APIType>/<endpoint>/`. `--prune` still
never touches `docs/`, and the documentation run must treat `generate.md` as its input, never as something
to edit or clean up.

### Where documentation lives (decided)

`docs/` mirrors `resources/` exactly, as a sibling under the tenant directory:

```
output/<domain>/resources/<APIType>/<endpoint>/<name>.yaml   # written by azure-rd
output/<domain>/resources/<APIType>/<endpoint>/doc-prompt.md # written by azure-rd
output/<domain>/docs/<APIType>/<endpoint>/<name>.md          # written by the documentation run
```

So a document's path is its metadata key with the tree root and extension swapped — no `doc:` field in
`metadata.yaml`, nothing to keep in sync, and the existence check is a single `os.Stat`.

Two consequences worth knowing. **`docs/` is not tool-owned**, so `--prune` must never reach into it; a
pruned resource leaves its document behind as an orphan, to be reported and not deleted. And **doc→doc
relative links survive**: every link in a generated document is of the form `../<endpoint>/<name>.md`,
resolved relative to the document itself, so a mirrored tree keeps all of them valid.

### How staleness is determined

No history is stored anywhere, because **both endpoints of the comparison are self-describing**:

- `resources/metadata.yaml` records the export's *current* state — `sourceSha256` per resource,
  `promptSha256` per type, and every resource's `assignmentTargets`.
- every generated document's frontmatter records the state it *was generated from* — `sourceSha256`,
  `promptSha256`, `generatedAt`, plus `assignmentsSha256` and, on group documents, `targetedBySha256`.

Stale is simply: those two disagree.

That is deliberately stateless across runs. Download five times, or download, hand-edit a document, download
again — the command still names exactly the stale documents, because it never relied on having observed the
intermediate steps. **Do not add change tracking to `metadata.yaml` to support this**: a change record is
derived state, and the facts-only rule keeps it out of that file.

| Condition | Action |
|---|---|
| No document under `docs/` | **generate** |
| Frontmatter `sourceSha256` ≠ metadata's | **generate** — the resource changed since the document was written |
| Frontmatter `promptSha256` ≠ the type's | **generate** — the per-type spec changed, so the document no longer conforms to it |
| Frontmatter missing or unparseable | **generate** — never skip on a signal you could not read |
| Entry marked `presentInTenant: false` | do **not** generate; report as an orphaned document |
| Type's `doc-prompt.md` absent from `resources/` | report — it is the *input* to generation, so no document of that type can be produced (e.g. the export ran with `--no-prompt`) |
| Excluded type, and not a referenced group | ignore entirely — not documented, not reported as missing |

The `promptSha256` row is the only path by which a `doc-prompt.md` change — in practice a tool upgrade —
reaches the documents it governs.

**This table decides list 1 only.** A document can be perfectly current by every row above and still carry a
stale marked block — see "Two lists" below, which decides list 2 from `assignmentsSha256` and
`targetedBySha256`. Implementing this table alone produces a command that silently never re-splices.

### Contract: document frontmatter is an interface

The template (`internal/docs/generate_prompt_template.md`) §2 requires `source`, `sourceSha256`,
`promptSha256` and `generatedAt` on every generated document, plus `assignmentsSha256` where the type has
assignments and `targetedBySha256` on group documents. That is not housekeeping: it is the only reason the next run can tell a current document from a
stale one. Without those hashes this command degrades to detecting wholly missing documents.

Every one of those hashes is handed to the agent in its work-list row. No document should ever be written
with a hash the agent computed itself — two implementations of one hash never agree for long.

### What it must not do

- **Never fetch a resource.** It authenticates only to resolve the tenant domain; there is no fetch,
  transform or write pipeline in this command.
- **Never write into `resources/`**, and never write or update `metadata.yaml`.
- **Never delete anything.** Orphaned documents are reported, not removed.
- **Write exactly one file**, `docs/generate.md` (or `--out`). It never touches a generated document —
  deciding that one is stale is not the same as being allowed to change it.

### Reporting

Print `metadata.yaml`'s `generatedAt`, `run.complete` and `run.incompleteReason` alongside the result. The
cost of splitting the workflow is that step 1 can be forgotten; this is what makes that visible. An
incomplete export is a **warning, not a refusal** — nothing destructive happens here, and documenting what
you do have is still useful.

### Output shape (decided)

One prompt file, rendered from a **template embedded in the binary**:
`internal/docs/generate_prompt_template.md`, adapted from `DOC-GENERATION-PROMPT.md` §0–§5. Override it with
`--prompt <file>`.

`DOC-GENERATION-PROMPT.md` itself stays untouched — it remains the full-export procedure, and it is not read
by this command. The template is a separate file precisely so the incremental flow can drop what does not
apply to it rather than accumulate conditionals in a document that has another job.

The command replaces five marked blocks in the template and writes the result. Markers follow the convention
already used for assignment tables (`<!-- x:start -->` / `<!-- x:end -->`), so the template stays readable on
its own and the splice is deterministic:

| Marker | Injected content |
|---|---|
| `export` | the tenant, `resources/` and `docs/` paths, the export's `generatedAt`, and whether the last run was complete |
| `worklist` | the resources to document, grouped by type, each row giving source path, target document path, the reason it is listed, and every hash to write into its frontmatter |
| `refmap` | assignment target GUID → group name and document path, resolved from `metadata.yaml`, with dangling GUIDs flagged |
| `resplice` | documents needing one marked block re-rendered though their own resource did not change — assignments tables whose resolved target names moved, and `Targeted by` blocks whose targeting moved |
| `migrate` | documents predating the assignment markers; rendered as "none" when there are none |

Everything outside the markers is editable prose. What the template drops relative to the original: the
inventory and hashing of §1, the six-bucket classification of §1b, the excluded-types argument (the work
list is closed — the agent documents exactly what it names), the manifest read/write of §1b and §7, and
§5a's "parse every YAML to build the reference map", which `metadata.yaml` already answers.

What it adds: the source paths are tenant-relative rather than bare filenames, the splice hashes
(`assignmentsSha256`, `targetedBySha256`) are recorded in document frontmatter rather than in a manifest, and
the report states whether the export itself was complete.

### Two lists, because a marked block can go stale on its own

**Content staleness and splice staleness are different sets.** Conflating them either regenerates documents
that only needed a table swapped, or leaves marked blocks silently wrong.

**List 1 — documents to generate.** Everything from the classification table above, grouped by resource type,
each entry giving its source YAML path, its target document path, why it is listed, and the hashes to write
into its frontmatter. Newly referenced groups land here too: a group some policy now assigns and that has no
document yet is simply a missing document.

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
for each assignment entry, sorted by (direction, groupId, filterId):
    direction | groupId | resolvedGroupName | groupTypes | securityEnabled
              | filterId | resolvedFilterName | filterType | intent | source
```

Anything that would change a cell in the rendered table changes the hash: a rename, a filter rename, a group
type change, a target added or removed, a group disappearing from the export. Nothing else does.

This mirrors `targetedBySha256` for the reverse direction, so list 2 has one mechanism and one shape:

| Direction | Hash | Stale when |
|---|---|---|
| Forward — the document's own assignments table | `assignmentsSha256` | any referenced group or filter changed name, kind or presence |
| Reverse — a group document's `Targeted by` | `targetedBySha256` | the set of resources targeting that group changed |

Rules that keep this from misfiring:

- **The command computes both hashes and hands them to the agent**, exactly as it does for `sourceSha256` and
  `promptSha256`. The agent never computes a hash. Two implementations of one hash is how you get a document
  that re-splices on every run forever.
- **Both hashes live in document frontmatter, not `metadata.yaml`** — they describe the state a document was
  written in, which is the document's business, not the export's.
- **A document already in list 1 is excluded from list 2**; §5c gives it a fresh block anyway.
- **Omit the field for types with no assignments concept.** Missing field on a type that *does* have
  assignments means "never spliced" → needs splicing.
- The hash covers the resolved *inputs*, not the rendered markdown, so changing how the table is formatted
  will not restale existing blocks. If that ever matters, mix a template version into the hash.

### Two facts to add to `ResourceMeta`

The forward hash needs `groupTypes` and `securityEnabled` for each referenced group, and `ResourceMeta`
carries neither today. Both come straight from the group YAML at download time and are plain facts, so they
belong there under the facts-only rule. Record them raw — do not store a rendered `"dynamic security group"`
string; that phrasing is a decision the renderer makes.

Adding them pays twice: it completes the forward hash, and it removes the last reason for the agent to open
a group YAML at all, so §5a's reference map becomes fully precomputable from `metadata.yaml`.

### §5e is conditional

Marker migration only applies to documents written before the assignment markers existed. The command can
detect that offline — a document with an assignments table and no `<!-- assignments:start -->` — and include
§5e in the emitted prompt only when at least one such document exists, with those documents listed.

### Operational contract

- **Exit codes.** `0` when every document is current, `0` with the prompt written when work was found, and a
  distinct non-zero code when the command could not answer — no `resources/metadata.yaml`, an ambiguous
  tenant, an unreadable template. CI wants "stale documents exist" to be distinguishable from "everything is
  current", so make that a flag (`--exit-code` or similar) rather than overloading the default.
- **First run.** `docs/` will not exist. Create it — writing `generate.md` is the one exception to "never
  writes into `docs/`", and it is the reason the directory has to be there at all.
- **A stale `generate.md` outlives a `--dry-run`.** Dry-run writes nothing, so a prompt file from an earlier
  run stays on disk and someone will paste it. Print its path and age whenever one exists and the run did not
  replace it.
- **Template validation.** Every marker the command intends to fill must be present, matched and unmatched-
  free before anything is written. A `--prompt` file missing `<!-- worklist:end -->` must fail naming that
  marker, not splice into nothing and emit a prompt with no work list.
- **Determinism.** `generate.md` is diffed and committed like any other artifact: sort every list, and two
  runs over an unchanged export must produce byte-identical output.

### Noted gap — a transform-config change looks like a tenant-wide change

Change the transformer config and re-download, and every resource's `sourceSha256` moves, so every document
goes stale at once. That is arguably correct — the YAML content genuinely changed — but the report can only
say "190 stale" without saying why. Distinguishing the two would mean recording `transformConfigSha256` in
document frontmatter as well, which is an extra field and a prompt amendment. Noted, not planned.

---

## 5. `download --dry-run` still performs the full download

Flagged by the symmetry above, and a change to shipped behaviour rather than a bug — so it needs a decision
before anyone acts on it.

Today `--dry-run` suppresses only the *writes*. `Pipeline.Execute` still runs all three stages, so every
resource is listed, fetched from Azure, transformed, and then discarded. A dry run of a full export costs the
same API traffic, rate limiting and wall-clock time as a real one, and produces no file.

If `--dry-run` means "list the resources that would be downloaded", the run can stop after
`Registry.BuildFetchRequests` — which is where the list already exists — and print it. That keeps the
per-type listing calls needed to build the list, drops the per-resource fetches, and makes the two commands
consistent: neither performs resource fetches under `--dry-run`.

The trade-off is that today's dry run does exercise the fetch and transform paths, so it catches per-resource
permission errors that a list-only version would not. If that is worth keeping, it belongs behind its own
flag rather than as the meaning of `--dry-run`.
