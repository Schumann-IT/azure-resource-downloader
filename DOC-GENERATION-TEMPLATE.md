# Incremental documentation generation prompt (template)

<!--
This file is a TEMPLATE, not a prompt to paste as-is.

`azure-rd docs generate-prompt` reads it, replaces each marked block below with values computed from
`resources/metadata.yaml` and the documents already present under `docs/`, and writes the finished prompt.
Paste the OUTPUT into a fresh agent session.

Override with `--prompt <file>` to use a different template.

Marked blocks the tool replaces (start/end markers stay, content between them is regenerated):

  export     paths of the export being documented, plus its freshness
  worklist   the resources to document, grouped by type, with the reason each is listed
  refmap     assignment target GUID -> group name/document, resolved from metadata.yaml
  resplice   documents needing one marked block re-rendered though their own resource did not change:
             assignments tables whose target names moved, and "Targeted by" blocks whose targeting moved
  migrate    documents predating the assignment markers; rendered as "none" when there are none

Everything outside the markers is prose you can edit freely. Keep the markers matched and never nested.
The existing `DOC-GENERATION-PROMPT.md` is the full-export procedure and is not used by this command.
-->

You are generating end-user documentation for an Azure/Entra/Intune tenant export produced by
**azure-resource-downloader**.

This is an **incremental** run. The inventory and change detection have already been done for you: the work
list below is complete and closed. Document exactly what it names — nothing more, nothing less. Do not walk
the export looking for other resources, do not hash anything to decide what changed, and do not re-litigate
which resource types are in scope.

## Export under documentation

<!-- export:start -->
- Tenant folder: `output/<tenant>/`
- Resources (read-only source of truth): `output/<tenant>/resources/`
- Documents (your output): `output/<tenant>/docs/`
- Export generated at: `<timestamp>`
- Export complete: `<true|false — reason>`
<!-- export:end -->

Every path below is relative to the tenant folder. A resource and its document mirror each other:

```
resources/<APIType>/<endpoint>/<name>.yaml     the source
resources/<APIType>/<endpoint>/doc-prompt.md   the spec for that whole type
docs/<APIType>/<endpoint>/<name>.md            the document you write
```

If the export is marked incomplete, say so in your final report: it means the last download did not reach
every resource, so the export itself may lag the tenant. It does not stop you documenting what is here.

---

## 0. Ground rules (apply to every step)

- **Never invent.** Only document properties, values, IDs and assignments that literally appear in the
  YAML. If something is absent, say it is absent — do not fill it in from general knowledge of the product.
- **Masked values are not misconfigurations.** Secrets exported as `valueState: encryptedValueToken`,
  redacted certificates, opaque tokens, `*****` and so on are expected service behaviour. State that the
  value is masked by the service and move on. Never flag it as a finding, never guess the plaintext.
- **Unmappable enums stay unmapped.** If a numeric or opaque enum cannot be confidently resolved to a human
  label from the YAML alone, document it with an explicit "verify against <CSP/API reference>" caveat rather
  than guessing.
- **The resource's own `description` field is authoritative context.** If it documents a deliberate
  deviation from a baseline, honour that — describe it as intentional, do not report it as a defect.
- **Prefer the reference links in each `doc-prompt.md`** over recalled knowledge.
- **Never modify anything under `resources/`.** It is the source of truth and is read-only for this task.
  That includes `metadata.yaml` — it belongs to `azure-rd`, not to you.
- **Write only the files named below.** No index, no summary document, no scratch files left behind.

---

## 1. What to document

The list is authoritative. Each entry gives the source YAML, the document to write, and why it is listed.

<!-- worklist:start -->
_Replaced by the tool. Shape:_

### Microsoft.Graph/deviceCompliancePolicies — spec: `resources/Microsoft.Graph/deviceCompliancePolicies/doc-prompt.md`

| Source | Document | Reason | sourceSha256 | promptSha256 | assignmentsSha256 |
|---|---|---|---|---|---|
| `resources/…/gbl_c_prd_d_win_os_validation.yaml` | `docs/…/gbl_c_prd_d_win_os_validation.md` | resource changed | `5d6b32f8…` | `95cb34be…` | `7a41c0de…` |

_…one section per resource type, then a tally: N documents to write across M types._
<!-- worklist:end -->

The hashes on each row are the values to write into that document's frontmatter (section 2). They are given
to you precisely so you never have to compute them — do not hash anything, ever. `assignmentsSha256` is
blank for types that have no assignments.

---

## 2. Read the spec

For each resource type in the work list, read its `doc-prompt.md` **in full** before writing anything for
that type. It defines the required layout for that type: title, summary paragraph, metadata table, the exact
H2 sections in order, and a `Properties` or `Settings` section requiring every remaining property as a
collapsed HTML `<details>` block whose `<summary>` carries the property path and configured value.

Follow it exactly. Specs differ meaningfully between types — an `assignmentFilters` document is a short rule
explanation with no settings payload; a `deviceManagementConfigurationPolicies` document may carry hundreds
of settings. Never substitute a generic template.

### Headings

`#` is the resource display name, `##` its sections. Directly under the title, put the source YAML filename
in backticks on its own line so any statement can be traced to its source.

### Frontmatter (required)

Every document starts with YAML frontmatter, before the `#` title:

```yaml
---
source: resources/Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.yaml
sourceSha256: 5d6b32f8…
promptSha256: 95cb34be…
assignmentsSha256: 7a41c0de…   # only for types that have assignments
generatedAt: 2026-08-13T08:31:00Z
---
```

`source` is a path relative to the tenant folder, not a bare filename — the resource and its document live in
different trees.

**Every hash comes from the work-list row, verbatim. Never compute one yourself.** `assignmentsSha256`
covers the resolved contents of the assignments block — the target groups' names and kinds, the filters'
names — which is how the next run notices that a group was renamed even though this resource never changed.
Group documents additionally carry `targetedBySha256`; see 5d.

This frontmatter is how the next incremental run knows whether your document is still current. A document
without it is treated as stale and regenerated from scratch every time.

### Assignment markers (required)

Wherever the spec calls for an assignments or targeting table, wrap the whole block in HTML comment markers
on their own lines:

```markdown
<!-- assignments:start -->

| Direction | Target | Filter |
|---|---|---|
| Include | `8964516b-c223-4f58-a866-232d3690c9b4` | none |

<!-- assignments:end -->
```

Emit the markers even when a resource has no assignments — put the sentence saying so between them. Omit them
only for types with no concept of assignments. Never nest markers, never emit a start without its end, never
put anything that is not about assignments between them. Section 5 depends on this being a deterministic
splice.

---

## 3. Generate (parallel subagents)

A large work list is too much content for one context. Fan it out:

1. **Chunk the work.** Group each type's listed YAML files into chunks of roughly **≤10 files or ≤110 KB of
   YAML**, whichever is smaller. A single 200 KB settings-catalog policy is its own chunk. A type with one
   small resource is one chunk.
2. **Write a self-contained instruction file per chunk** (e.g. `chunks/NN.txt`) containing: the resource
   type, the path to its `doc-prompt.md`, the path of every YAML in the chunk, the two hashes to embed in
   each document's frontmatter, the ground rules from section 0, the heading and frontmatter rules from
   section 2, the exact output path to write, and an instruction to return only a one-line
   `DONE <path> <count>` receipt.
3. **Spawn one subagent per chunk**, in batches of about 10 concurrently. Each agent's prompt is just *"Read
   `chunks/NN.txt` and follow its instructions exactly."* Keeping chunk contents out of the orchestrator's
   context is what makes a large run feasible.
4. Instruct each agent to **read every assigned YAML in full** before writing, and to write exactly one
   output file and nothing else.

Sonnet-class models are sufficient for per-chunk generation. Reserve the orchestrator for chunking,
splicing and verification.

---

## 4. Verify (mandatory — do not skip)

Run these mechanically, with a script, over what you produced. Do not eyeball it.

| Check | Expectation |
|---|---|
| Coverage | Every work-list entry produced a document at its stated path. Zero missing. |
| Nothing else touched | No document outside the work list and the re-splice list was created or modified. Check mtimes. |
| Frontmatter | Every written document has valid frontmatter whose `sourceSha256` and `promptSha256` equal the values from its work-list row. A mismatch means an agent documented the wrong file. |
| Heading structure | Exactly one `#` heading per document. Count headings **outside fenced code blocks only** — shell scripts embedded in `deviceShellScripts` / `deviceManagementScripts` documents contain `##` comment lines that are not headings. |
| `<details>` balance | `<details>` count equals `</details>` count per file. Nesting is normal. |
| Stray artifacts | No leftover `DONE` receipts, no truncated final block, every file ends cleanly. |
| Assignment markers | Every document that should have them has a matched `<!-- assignments:start -->` / `<!-- assignments:end -->` pair — never unbalanced, never nested. Every referenced-group document has a matched `<!-- targeted-by:start -->` / `<!-- targeted-by:end -->` pair. |
| Assignment resolution | No bare group GUID remains inside a marked block without a resolved name beside it or an explicit `⚠️ not in export`. |
| Link symmetry | Every group linked from a policy's assignment table has a document, and that document's **Targeted by** list contains that policy. Both directions come from one map, so a mismatch means the splice went wrong. |

Imbalanced `<details>` tags are the most common defect and they silently break rendering in GitHub and most
viewers. Repair by walking the file, tracking depth outside code fences, closing anything still open before
the next heading or at EOF, and dropping unmatched closing tags. Re-run the checks and report what you
repaired.

---

## 5. Assignments

Documents name their assignment targets by raw GUID. A page saying
`groupId: 8964516b-c223-4f58-a866-232d3690c9b4` is accurate and useless. This step resolves those GUIDs and
links the two directions together. It is a **deterministic splice** — do not spawn agents for it.

### 5a. Reference map (given)

<!-- refmap:start -->
_Replaced by the tool: group GUID → display name, its document path, and the resources that target it,
resolved from `metadata.yaml`. Includes any GUID with no matching group in the export, flagged as dangling._
<!-- refmap:end -->

Two values need care:

- `00000000-0000-0000-0000-000000000000` as a **filter** ID is Intune's "no filter" sentinel. Render it as
  `none`. It is not an unresolvable reference and must never be reported as one.
- A group GUID flagged **dangling** above has no group in the export — usually deleted from the tenant while
  still assigned. Keep the raw GUID, mark it `⚠️ not in export`, and list every one in your final report.

The map does not carry `groupTypes`, `securityEnabled` or `membershipRule`. Read those from the referenced
groups' YAML when you need the inline `dynamic security group` annotation — that is a few dozen files, and
reading them is not the same as documenting them.

### 5b. Document referenced groups

Referenced groups needing a document appear in the work list like any other resource, with
`resources/Microsoft.Graph/groups/doc-prompt.md` as their spec. A dynamic group's `membershipRule` is what
actually decides who receives every policy assigned to it — explain it clause by clause and flag where it
keys on user-editable attributes.

### 5c. Rewrite assignment blocks

For every document you wrote, replace everything between `<!-- assignments:start -->` and
`<!-- assignments:end -->` with a table built from the YAML and the map above. The markers stay. Generate
fresh — never edit the previous table in place.

```markdown
| Direction | Target | Filter | Intent |
|---|---|---|---|
| Include | [M365-CO-DYN-INTUNE-AUTOPILOT-DEFAULT](../groups/m365_co_dyn_intune_autopilot_default.md) · dynamic security group · `8964516b-c223-4f58-a866-232d3690c9b4` | include [GBL_AF_PRD_D_WIN_MGM_Lenovo_devices](../assignmentFilters/gbl_af_prd_d_win_mgm_lenovo_devices.md) | required |
| Exclude | [M365-CB-Admin](../groups/m365_cb_admin.md) · assigned security group · `e0c6f42d-…` | none | — |
```

- **Name first, GUID last.** The name is what a reader needs; the GUID stays for traceability and grepping.
  Never truncate a GUID someone might search for.
- Links are **relative from one document to another**, within `docs/`, so they work on disk, in VS Code, on
  GitHub and in the web frontend.
- Annotate each group inline with `dynamic`/`assigned` and `security`/`Microsoft 365`.
- **Drop columns that are empty for every row.** `Intent` applies only to apps.
- `allLicensedUsersAssignmentTarget` → **All users**, `allDevicesAssignmentTarget` → **All devices**. These
  are built-in targets with no group page; never invent a link.
- No assignments at all → one sentence between the markers: *"This resource has no assignments — it is
  configured but not targeted at anything."* That is a finding worth stating plainly.

### 5d. Re-splice documents whose blocks went stale on their own

A marked block can be wrong while the document around it is perfectly current, because both blocks are
rendered from information that lives outside the document's own resource:

- **Forward** — a document's own assignments table prints its target groups' *names*. The resource stores
  only GUIDs, so renaming a group in the tenant leaves every policy targeting it printing a stale name while
  no policy's YAML moved.
- **Reverse** — a group document's `Targeted by` block lists *other* resources' assignments. Re-point a
  policy from one group to another and both groups' reverse indexes are wrong while neither group's YAML
  moved.

Neither case appears in the work list, because nothing about those documents' own resources changed. The
documents below need one marked block replaced and **their bodies left completely alone** — do not
regenerate them, do not re-read their specs:

<!-- resplice:start -->
_Replaced by the tool, in two groups: documents whose **assignments** block must be re-rendered (with the
resolved targets and the new `assignmentsSha256`), and group documents whose **Targeted by** block must be
re-rendered (with the targeting resources and the new `targetedBySha256`). Rendered as "none" when nothing
needs re-splicing._
<!-- resplice:end -->

After splicing, update that document's frontmatter with the hash the tool gave you — `assignmentsSha256` for
a forward re-splice, `targetedBySha256` for a reverse one. A document whose block you re-rendered but whose
hash you did not update will be re-spliced again on every future run.

The `Targeted by` block is wrapped in its own markers:

```markdown
<!-- targeted-by:start -->
## Targeted by

53 resources assign this group.

| Resource | Type | Direction | Filter |
|---|---|---|---|
| [GBL_CP_PRD_D_CIS_WIN_Firewall_L1](../deviceManagementConfigurationPolicies/gbl_cp_prd_d_cis_win_firewall_l1.md) | Settings Catalog | Include | none |
<!-- targeted-by:end -->
```

Sort by resource type then name, and state the total above the table. Never hand this block to an agent — it
is generated data, and a model rewriting it will paraphrase names and break links.

### 5e. Migrate documents written before markers existed

<!-- migrate:start -->
_Replaced by the tool: documents that have an assignments table but no `<!-- assignments:start -->` marker.
Rendered as "none" when there are none._
<!-- migrate:end -->

For each, insert the markers around the existing block without altering anything else, then apply 5c
normally. This is a one-off per document; once migrated the markers persist.

---

## 6. Report

Finish with:

- documents written, and documents re-spliced
- checks passed, and anything you repaired
- every **dangling group reference** (an assigned GUID with no group in the export)
- how many documents were migrated to markers
- whether the export was marked incomplete, and why
- substantive findings the analysis surfaced: baseline deviations, disabled controls, secrets present,
  policies with no assignments

Findings are derived from the export alone. Say so, and recommend verifying anything security-relevant
against the live tenant.
