# Documentation generation prompt (azure-resource-downloader)

<!-- Paste this whole file into a fresh agent session, then name the export folder you want documented. -->

You are generating end-user documentation for an Azure/Entra/Intune tenant export produced by
**azure-resource-downloader**.

**Target folder:** `output/<tenant>/` — the user names it. Everything below is relative to that folder.

Regeneration is **incremental**: on a re-run you only apply the documentation prompt to resources whose
YAML actually changed. See section 1.

**Excluded resource types.** These are never documented in bulk:

- `groups` — **except those referenced by an assignment** (see section 5)
- `windowsAutopilotDeviceIdentities` — no exceptions

They are bulk directory records — thin, highly repetitive, and typically 80%+ of all files in an export
(470 + 270 of 898 in a representative tenant). Documenting them costs enormously and produces near-identical
pages that nobody reads. Skip them at the inventory stage: do not hash them, do not bucket them, do not
count them toward totals, and never spawn an agent for them. Report their counts once, as excluded.

**The referenced-groups exception.** A group that some policy, app or profile actually assigns is not a bulk
record — it is the answer to "who gets this?". Those groups *are* documented, in section 5, after the main
generation pass has revealed which ones they are. In a representative tenant this is **17 of 470 groups**.
Everything else in `groups` stays excluded.

Only document an otherwise-excluded resource if the user asks for it explicitly and by name in the current
session. Never infer it from "document everything", and never re-litigate the exclusion in your report.

---

## 0. Ground rules (apply to every step)

- **Never invent.** Only document properties, values, IDs and assignments that literally appear in the
  YAML. If something is absent, say it is absent — do not fill it in from general knowledge of the
  product.
- **Masked values are not misconfigurations.** Secrets exported as `valueState: encryptedValueToken`,
  redacted certificates, opaque tokens, `*****` etc. are expected service behaviour. State that the
  value is masked by the service and move on. Never flag it as a finding, and never guess the plaintext.
- **Unmappable enums stay unmapped.** If a numeric or opaque enum value cannot be confidently resolved
  to a human label from the YAML alone, document it with an explicit "verify against <CSP/API reference>"
  caveat rather than guessing a label.
- **The resource's own `description` field is authoritative context.** If it documents a deliberate
  deviation from a baseline, honour that — describe it as intentional, don't report it as a defect.
- **Prefer the reference links given in each `doc-prompt.md`** over recalled knowledge.
- **Never modify a `.yaml` file.** The export is the source of truth and is read-only for this task.

---

## 1. Inventory and change detection

The export is laid out as:

```
output/<tenant>/
  .doc-manifest.json               # generation state (created by this process — see below)
  <provider>/                      # e.g. Microsoft.Graph
    <resourceType>/                # e.g. conditionalAccessPolicies
      doc-prompt.md                # the per-type documentation spec — NOT a resource
      <resource-name>.yaml         # one file per resource
      <resource-name>.md           # generated documentation
      ...
```

### 1a. Walk and hash

1. Walk the folder recursively. Directory listings of a large export can exceed a single tool result —
   if so, dump the listing to a file and parse it with a script instead of reading it into context.
2. **Drop the excluded types immediately** (`groups`, `windowsAutopilotDeviceIdentities`). Record only
   their resource counts for the report, then exclude them from every later step. Everything below refers
   to the remaining, in-scope types.
3. Find every `doc-prompt.md`. There is exactly one per resource type directory, and it is the
   authoritative spec for how that type must be documented. It is **not** a resource — never count or
   document it.
4. Compute **SHA-256 of the raw bytes** of every in-scope `*.yaml` file, and of every in-scope
   `doc-prompt.md`.

**Change detection is by content hash only. Do not use `lastModifiedDateTime`, `modifiedDateTime`,
`version` or file mtime to decide whether a resource changed.** Those fields are unreliable here: they
are absent entirely from roughly a third of exported resources (in a representative export, all
`mobileApps`, all `deviceShellScripts`, `deviceManagementScripts`, `organization`, `deviceManagement`,
`roleScopeTags` and others carry no modified timestamp at all), and where present they are inconsistently
named. A hash of the YAML works uniformly for every resource type and is the only signal you should
branch on. Timestamps may still be *documented* as content where the spec asks for them — just never
used to skip work.

### 1b. Diff against the manifest

Read `.doc-manifest.json` at the tenant folder root if it exists. Schema:

```json
{
  "version": 2,
  "tenant": "cb-gmbh.com",
  "generatedAt": "2026-08-13T08:31:00Z",
  "types": {
    "Microsoft.Graph/conditionalAccessPolicies": {
      "promptSha256": "<sha256 of that type's doc-prompt.md>",
      "resources": {
        "ca01_highprivsignins.yaml": {
          "sha256": "<sha256 of the yaml>",
          "doc": "ca01_highprivsignins.md",
          "generatedAt": "2026-08-13T08:31:00Z"
        }
      }
    },
    "Microsoft.Graph/groups": {
      "promptSha256": "<sha256 of groups/doc-prompt.md>",
      "resources": {
        "m365_co_dyn_intune_autopilot_default.yaml": {
          "sha256": "<sha256 of the group yaml>",
          "doc": "m365_co_dyn_intune_autopilot_default.md",
          "generatedAt": "2026-08-13T08:31:00Z",
          "targetedBySha256": "<sha256 of this group's slice of the reference map — see 5d>"
        }
      }
    }
  }
}
```

`Microsoft.Graph/groups` holds **only the referenced groups** (section 5b), never the whole directory.
`targetedBySha256` appears only on group entries.

**Version handling.** A manifest with `version: 1` predates the assignment work. Do not treat that as
invalidating anything: keep every resource's bucket as the hashes dictate, treat the missing
`targetedBySha256` fields as absent (which forces one re-splice of the reverse indexes, not a
regeneration), and write `version: 2` back out. Never regenerate documents just because the schema moved.

Classify every resource into exactly one bucket:

| Bucket | Condition | Action |
|---|---|---|
| **new** | not in the manifest | generate |
| **changed** | manifest hash != current hash | regenerate |
| **prompt-changed** | its type's `promptSha256` != current `doc-prompt.md` hash | regenerate — **all** resources of that type |
| **missing-doc** | in manifest and unchanged, but its `.md` is absent | regenerate |
| **unchanged** | hash matches, doc exists, prompt hash matches | **skip — do not read the YAML, do not spawn an agent** |
| **orphan** | doc exists but its `.yaml` is gone | ignore; leave in place, say nothing |

Referenced groups (section 5b) go through this same table, under `Microsoft.Graph/groups`. Their generated
`Targeted by` block is **not** covered by these buckets — it is spliced separately in 5d and never causes a
regeneration.

A changed `doc-prompt.md` invalidates every document of that type — the spec itself changed, so the
existing docs no longer conform even though their YAML didn't move.

If there is no manifest, everything is **new** — this is a first full run.

### 1c. Report before generating

Report a table: resource type, total resources, and the count in each bucket, plus the overall total to
generate. State plainly how many are being skipped as unchanged. Add one line below the table giving the
excluded types and their counts, so the numbers reconcile against the raw file count — then move on.

Also report anything that looks like a **false positive change**: a resource whose only diff is a
regenerated `id`, a sync timestamp, or a usage counter. If a resource's `id` differs between exports it is
not an edit — either it is a different object, or the exporter is synthesising IDs, in which case that type
will show as changed on every run forever and incremental regeneration is defeated for it. Flag it; that is
an exporter bug to fix, not something to paper over here.

**Output shape** — ask only when there is no manifest and no existing docs (a genuine first run):
one `.md` per `.yaml`, written next to it (recommended — best for diffing and for incremental
regeneration); one `.md` per resource type; or a separate mirrored `docs/` tree. Otherwise infer the shape
from what already exists and proceed without asking.

If the session is unattended, use *one `.md` per `.yaml`* and state that assumption at the top of your
output.

> **Note on the per-type output shape:** it composes badly with incremental regeneration, because one
> changed resource forces the whole type's file to be rewritten. If the user picks it, regenerate only the
> changed resources' sections and splice them into the existing file rather than re-running the whole type.

---

## 2. Read the spec

For each resource type that has work to do, read its `doc-prompt.md` **in full**. It defines the required
layout for that type — title, summary paragraph, metadata table, the exact H2 sections in order, and a
`Properties` or `Settings` section requiring every remaining property as a collapsed HTML `<details>`
block whose `<summary>` carries the property path and configured value.

Follow that spec exactly. It differs meaningfully between types (an `assignmentFilters` doc is a short rule
explanation with no settings payload; a `deviceManagementConfigurationPolicies` doc may have hundreds of
settings). Do not substitute a generic template.

### Heading levels

- **One file per resource** (recommended shape): use the spec's levels verbatim — `#` = display name,
  `##` = its sections.
- **One file per resource type**: demote every heading one level (spec's `#` → `##`, `##` → `###`) so the
  combined file has a single `#` title, and separate resources with `---`.

Directly under each resource's title, put the source YAML filename in backticks on its own line, so a
reader can trace any statement back to its source and so the file can be mechanically split or recombined
later.

### Assignment markers (required)

Wherever the spec calls for an assignments / targeting table, **wrap the entire block in HTML comment
markers**, on their own lines:

```markdown
<!-- assignments:start -->

| Direction | Target | Filter |
|---|---|---|
| Include | `8964516b-c223-4f58-a866-232d3690c9b4` | none |

<!-- assignments:end -->
```

Emit the markers even when the resource has no assignments — put the sentence saying so between them.
Omit them only for resource types that have no concept of assignments at all.

The markers make section 5 a deterministic splice. Without them, resolving group IDs to names means parsing
free-form prose, and the tables vary between resource types. Never nest markers, never emit a start without
a matching end, and never put content that is not about assignments between them.

### Frontmatter (required)

Every generated `.md` starts with YAML frontmatter, before the `#` title:

```yaml
---
source: ca01_highprivsignins.yaml
sourceSha256: 9f2b…
promptSha256: 4ad1…
generatedAt: 2026-08-13T08:31:00Z
---
```

This makes each document self-describing: if `.doc-manifest.json` is lost, deleted or never committed,
the state can be rebuilt by reading the frontmatter of the existing docs. The manifest is the fast path;
the frontmatter is the durable copy. Keep them consistent — if you ever find them disagreeing, trust the
frontmatter and rewrite the manifest from it.

---

## 3. Generate (parallel subagents)

Only resources in the **new / changed / prompt-changed / missing-doc** buckets reach this step.

A large export is far too much content for one context. Fan the work out:

1. **Chunk the work.** Group each type's to-generate YAML files into chunks of roughly **≤10 files or
   ≤110 KB of YAML**, whichever is smaller. A single 200 KB settings-catalog policy is its own chunk.
   Types with one small resource are one chunk.
2. **Write a self-contained instruction file per chunk** (e.g. `chunks/NN.txt`) containing: the resource
   type, the absolute path to its `doc-prompt.md`, the absolute path of every YAML in the chunk, the
   precomputed hashes to embed in frontmatter, the ground rules from section 0, the heading and
   frontmatter rules from section 2, the exact output path to write, and an instruction to return only a
   one-line `DONE <path> <count>` receipt.
3. **Spawn one subagent per chunk**, launched in batches of ~10 concurrently. Each agent's prompt is just
   *"Read `chunks/NN.txt` and follow its instructions exactly."* — this keeps chunk file lists out of the
   orchestrator's context, which is the main thing that makes a 150+ resource run feasible.
4. Instruct each agent to **read every assigned YAML in full** before writing, and to write exactly one
   output file and nothing else.

Sonnet-class models are sufficient for the per-chunk generation work; reserve the orchestrator for
inventory, hashing, chunking and verification.

---

## 4. Verify (mandatory — do not skip)

Run these mechanically, with a script, over the generated output. Do not eyeball it.

| Check | Expectation |
|---|---|
| Coverage | Every YAML in the to-generate set produced a document. Zero missing. |
| Count | Within in-scope types only: number of `.md` files == number of `.yaml` files (one-per-resource shape), or number of resource sections == number of YAML files (per-type shape). Counts include skipped-unchanged docs. Excluded types are expected to have zero `.md` files — that is not a coverage failure. |
| Frontmatter | Every regenerated doc has valid frontmatter whose `sourceSha256` matches the hash you computed for its YAML in step 1a. A mismatch means the agent documented the wrong file. |
| Untouched | Files in the **unchanged** bucket still exist and their mtime did not change. Nothing was rewritten that shouldn't have been. |
| Heading structure | Exactly one `#` heading per file (one-per-resource shape). Count headings **outside fenced code blocks only** — shell scripts embedded in `deviceShellScripts` / `deviceManagementScripts` docs contain `##` comment lines that are not headings. |
| `<details>` balance | `<details>` count == `</details>` count per file. Nested blocks are normal and expected. |
| Stray artifacts | No leftover `DONE`, no truncated final block, file ends cleanly. |
| Assignment markers | Every document that should have them has a matched `<!-- assignments:start -->` / `<!-- assignments:end -->` pair — never unbalanced, never nested. Every referenced-group page has a matched `<!-- targeted-by:start/end -->` pair. |
| Group manifest | Every referenced group has a `Microsoft.Graph/groups` manifest entry whose `sha256` matches its YAML on disk and whose `targetedBySha256` matches the reference map you just built. A group page with no manifest entry would be regenerated from scratch every run. |
| Assignment resolution | No bare group GUID remains inside a marked block without an accompanying resolved name or an explicit `⚠️ not in export`. |
| Link symmetry | Every group linked from a policy's assignment table has a page, and that page's **Targeted by** list contains that policy. The two directions are generated from one map, so a mismatch means the splice went wrong. |
| Index coverage | Every in-scope document — including referenced-group pages — is linked from `index.md` **exactly once**: no duplicates, no omissions. Check by comparing the set of linked paths against the set of documents on disk. |
| Index links | Every link in `index.md` resolves to a file that exists. Relative paths, not absolute. |

Imbalanced `<details>` tags are the most common generation defect and they silently break rendering in
GitHub and most viewers. Repair them by walking the file, tracking depth outside code fences, closing any
still-open blocks before the next resource heading or at EOF, and dropping unmatched closing tags. Then
re-run the checks. Report what you repaired.

Finish by reporting: resources regenerated, resources skipped as unchanged, checks passed, anything
repaired, any resource that fell to the index's **inferred fallback** (so a classification rule can be
added), **any dangling group reference** (assigned GUID with no group in the export) and how many documents
were migrated to markers, and any substantive findings the analysis surfaced (baseline deviations, disabled
controls, secrets present, policies with no assignments). Findings are derived from the export only — say
so, and recommend verifying against the live tenant.

---

## 5. Resolve assignments

Documents at this point name their assignment targets by raw GUID. A page saying
`groupId: 8964516b-c223-4f58-a866-232d3690c9b4` is technically accurate and practically useless. This step
resolves those GUIDs to names, documents the groups that are actually used, and links the two directions
together.

This step is **deterministic** — it splices generated tables into marked regions. Do not spawn agents for
the rewrite itself; the only LLM work here is documenting the referenced groups (5b).

### 5a. Build the reference map

Parse the YAML of every **in-scope** resource and collect, for each assignment entry:

- `target.@odata.type` → `groupAssignmentTarget` (include), `exclusionGroupAssignmentTarget` (exclude),
  `allLicensedUsersAssignmentTarget` (all users), `allDevicesAssignmentTarget` (all devices)
- `target.groupId`
- `target.deviceAndAppManagementAssignmentFilterId` and `...FilterType` (`none` / `include` / `exclude`)
- `intent` (apps only: `required`, `available`, `uninstall`, `availableWithoutEnrollment`)
- `source` (`direct`, `policySets`)

Then build two lookups by reading the YAML of the excluded `groups` directory and of `assignmentFilters`:

- **group id → { displayName, file, groupTypes, securityEnabled, membershipRule }**
- **filter id → { displayName, file, platform }**

Reading the 470 group YAMLs to build a lookup is cheap and is **not** documenting them — do it with a
script, never with agents.

Two values need special handling:

- **`00000000-0000-0000-0000-000000000000` as a filter ID is Intune's "no filter" sentinel.** Render it as
  `none`. It is not an unresolvable reference and must not be reported as one.
- A group ID with no matching group YAML is a genuine dangling reference — most often a group deleted from
  the tenant while still assigned, or a group outside the export's scope. Keep the raw GUID, mark it
  `⚠️ not in export`, and **list every one in your final report**. Do not silently drop it.

### 5b. Document the referenced groups

The set of group IDs collected in 5a — and only that set — gets documented. Apply
`groups/doc-prompt.md` to each, exactly as section 3 does for any other resource: same chunking, same
frontmatter, same output path (`Microsoft.Graph/groups/<name>.md`).

These pages matter more than their file count suggests. A dynamic group's `membershipRule` is what actually
decides who receives every policy assigned to it, and the group prompt requires that rule be explained
clause by clause and flagged where it keys on user-editable attributes.

**Referenced groups are tracked in `.doc-manifest.json` exactly like any other resource** — under
`Microsoft.Graph/groups`, with the group YAML's `sha256`, its `doc`, and `generatedAt`, and with the
`promptSha256` of `groups/doc-prompt.md`. All six buckets from section 1b apply unchanged. A group whose
YAML is unchanged and whose page exists is **unchanged**: its YAML is not read, no agent is spawned, and the
page is left untouched. Only unreferenced groups stay out of the manifest.

The referenced set can change between runs:

- A group newly referenced by an assignment is **new** — generate it and add it to the manifest.
- A group no longer referenced keeps its page and its manifest entry. The page is an orphan; per the orphan
  rule it is left in place and not mentioned. Keeping the entry means that if the group is referenced again
  later it is correctly classified **unchanged**, not regenerated.

Because groups live in the manifest under a normal type key, a first run after adding this step generates
all referenced groups once; every later run regenerates none of them unless a group's own YAML or
`groups/doc-prompt.md` changed.

### 5c. Rewrite the assignment blocks

For every in-scope document, replace everything between `<!-- assignments:start -->` and
`<!-- assignments:end -->` with a freshly generated table built from the YAML and the 5a lookups. The
markers themselves stay. Generate from YAML — never edit the previous table in place.

Canonical table:

```markdown
| Direction | Target | Filter | Intent |
|---|---|---|---|
| Include | [M365-CO-DYN-INTUNE-AUTOPILOT-DEFAULT](../groups/m365_co_dyn_intune_autopilot_default.md) · dynamic security group · `8964516b-c223-4f58-a866-232d3690c9b4` | include [GBL_AF_PRD_D_WIN_MGM_Lenovo_devices](../assignmentFilters/gbl_af_prd_d_win_mgm_lenovo_devices.md) | required |
| Exclude | [M365-CB-Admin](../groups/m365_cb_admin.md) · assigned security group · `e0c6f42d-…` | none | — |
```

Rules:

- **Name first, GUID last.** The name is what a reader needs; the GUID stays for traceability and for
  grepping the export. Keep it complete — never truncate a GUID a reader might search for.
- Links are **relative from the document to the group page**, so they work on disk, in VS Code, on GitHub
  and in the web frontend.
- Annotate each group inline with `dynamic` / `assigned` and `security` / `Microsoft 365` — a dynamic group
  behaves differently enough to be worth seeing without following the link.
- `allLicensedUsersAssignmentTarget` → **All users**, `allDevicesAssignmentTarget` → **All devices**. These
  are built-in targets with no group page; do not invent a link.
- **Drop columns that are empty for every row.** `Intent` applies only to apps; `Source` is worth a column
  only when some row is not `direct`.
- No assignments at all → between the markers write one sentence: *"This resource has no assignments — it
  is configured but not targeted at anything."* An unassigned policy doing nothing is a finding worth
  stating plainly, not an empty table.

### 5d. Reverse index on group pages

Append a **`## Targeted by`** section to each referenced group's page, listing every resource that assigns
it. Generate it from the same map — it is the inverse of 5c and must be built from the YAML, not by reading
the policy documents.

**Wrap it in its own markers**, exactly as assignment blocks are wrapped:

```markdown
<!-- targeted-by:start -->
## Targeted by
…
<!-- targeted-by:end -->
```

This matters for incremental correctness. A group page has **two independent sources of truth**:

| Part of the page | Depends on | Regenerated when |
|---|---|---|
| The documented body (membership rule, security notes, properties) | the group's own YAML + `groups/doc-prompt.md` | its manifest hash changes — an LLM pass |
| The `Targeted by` block | *other resources'* assignments | the set of resources targeting it changes — a deterministic splice |

A policy re-pointed at a different group changes nothing in either group's YAML, so a hash check on the
group alone would leave both reverse indexes stale and silently wrong. Handle it by treating the two parts
separately: hash the group's slice of the 5a reference map (the sorted list of resources targeting it, with
direction and filter) into `targetedBySha256`, store it in the manifest, and **re-splice the marked block
whenever that hash differs — without regenerating the body**. Splicing is free; regenerating is not.

Never let the marker block drift outside the markers, and never hand the `Targeted by` block to an agent —
it is generated data, and an LLM rewriting it will paraphrase resource names and break the links.

```markdown
## Targeted by

53 resources assign this group.

| Resource | Type | Direction | Filter |
|---|---|---|---|
| [GBL_CP_PRD_D_CIS_WIN_Firewall_L1](../deviceManagementConfigurationPolicies/gbl_cp_prd_d_cis_win_firewall_l1.md) | Settings Catalog | Include | none |
```

Sort by resource type then name, and state the total above the table. This is the section that makes the
group pages worth having: it turns "what is this group?" into "here is exactly what it controls", which
today can only be answered by grepping the export for a GUID.

### 5e. Migrating documents generated before markers existed

Documents produced by an earlier version of this prompt have assignment tables but no markers. Detect this
(document has an assignments-like table, no `<!-- assignments:start -->`) and run a **one-off** migration:
a subagent per chunk that inserts the markers around the existing block without altering anything else.
Then run 5c normally. Report how many documents were migrated.

Once migrated, the markers persist — this is a one-time cost, not a per-run pass.

---

## 6. Generate the index

Write **`index.md` at the tenant folder root** (`output/<tenant>/index.md`). Regenerate it whenever any
document was generated or regenerated, or when it is missing. If nothing changed and it already exists,
leave it alone.

Build it from the **generated documents**, not from the YAML: each doc's frontmatter gives the source file
and hashes, its `#` title gives the display name, and its summary paragraph gives the purpose line. This
keeps index generation cheap — no second pass over the export.

### 6a. Classify every documented resource

Two independent axes. Apply each list in order and stop at the first match.

**Axis 1 — platform group:**

| # | Rule | Group |
|---|---|---|
| 1 | `platforms:` field contains `macOS` / `windows` | macOS / Windows |
| 2 | any `@odata.type` starts with `macOS` or `depMacOS` | macOS |
| 3 | any `@odata.type` starts with `windows`, `win32LobApp`, `winGetApp`, `officeSuiteApp`, `windowsUniversalAppX`, `microsoftStoreForBusinessApp` | Windows |
| 4 | any `@odata.type` starts with `ios` or `android` | Mobile |
| 5 | endpoint is `groups` | Assignment targets |
| 6 | endpoint is `conditionalAccessPolicies`, `authenticationStrengthPolicies`, `authenticationMethodsPolicy`, `authorizationPolicy`, `organization`, `onPremisesSynchronization` | Identity & access |
| 7 | endpoint is `deviceManagement`, `intuneBrandingProfiles`, `mobileThreatDefenseConnectors`, `roleScopeTags` | Tenant & Intune infrastructure |
| 8 | endpoint name starts with `windows`, or is `deviceManagementScripts` (PowerShell) or `groupPolicyConfigurations` (ADMX) | Windows |
| 9 | endpoint is `applePushNotificationCertificate`, `depOnboardingSettings`, `deviceShellScripts`, `deviceCustomAttributeShellScripts` | macOS |
| 10 | filename contains `_mac_` / `_win_` | macOS / Windows |

**Axis 2 — function group** (within a platform):

| # | Rule | Group |
|---|---|---|
| 1 | endpoint `deviceManagementConfigurationPolicies` **and** filename contains `_cis_` | Baseline & hardening (CIS) |
| 2 | `deviceCompliancePolicies` | Compliance |
| 3 | `deviceManagementConfigurationPolicies` (non-CIS) | Configuration — Settings Catalog |
| 4 | `deviceConfigurations` | Configuration — custom & legacy profiles |
| 5 | `groupPolicyConfigurations` | Configuration — ADMX / Group Policy |
| 6 | `mobileApps` | Apps |
| 7 | `deviceShellScripts`, `deviceManagementScripts`, `deviceCustomAttributeShellScripts` | Scripts & custom attributes |
| 8 | `windowsAutopilotDeploymentProfiles`, `deviceEnrollmentConfigurations`, `depOnboardingSettings`, `applePushNotificationCertificate` | Enrollment & provisioning |
| 9 | `windowsDriverUpdateProfiles`, `windowsFeatureUpdateProfiles` | Updates |
| 10 | `assignmentFilters`, `notificationMessageTemplates` | Targeting & notifications |
| 11 | `conditionalAccessPolicies` | Conditional Access |
| 12 | `authenticationStrengthPolicies`, `authenticationMethodsPolicy` | Authentication strengths & methods |
| 13 | `authorizationPolicy`, `organization`, `onPremisesSynchronization` | Directory |
| 14 | `groups` | Groups |
| 15 | `deviceManagement`, `intuneBrandingProfiles`, `mobileThreatDefenseConnectors`, `roleScopeTags` | Tenant settings |

A platform group whose resources all land in a single function group renders as a **flat list** — emit the
entries directly under the `<summary>` with no `###` heading. A lone heading repeating the group name is
noise.

**Inferred fallback.** Anything the tables don't match — a new endpoint, an unfamiliar `@odata.type` — is
placed where it best fits by judgement, marked in the index with a trailing `_(unclassified)_`, and
**listed explicitly in your final report** so a rule can be added. Never silently drop a resource, and
never invent a new top-level group for a single stray resource.

A resource appears **exactly once** in the index.

### 6b. Structure

```markdown
# <Tenant display name> — Intune & Entra ID configuration

<intro — see 6c>

## How to read this
<naming-convention table, only if the export uses a consistent convention — see 6c>

## At a glance
| Area | Resources |   ← one row per platform group, plus a total

<details open>
<summary><strong>Windows</strong> — 79 resources</summary>

### Baseline & hardening (CIS) — 23
- [DISPLAY NAME](Microsoft.Graph/<endpoint>/<file>.md) — <purpose> · <scope> · <assignments>
...
### Compliance — 5
...
</details>

<details>
<summary><strong>macOS</strong> — 55 resources</summary>
...
</details>
```

Rules:

- Platform groups in this order: **Windows, macOS, Mobile, Identity & access, Assignment targets, Tenant &
  Intune infrastructure**, omitting any that are empty. Open the two largest by default (`<details open>`),
  collapse the rest.
- In the **Assignment targets** group, give each entry its assignment count instead of a scope/assignment
  suffix — e.g. `— dynamic security group; all Autopilot-enrolled devices · targeted by 53 resources`.
  That number is the reason the group is in the index at all.
- Function sections in the Axis-2 table order. **Omit empty sections** — never emit a heading with nothing
  under it. The same rules must produce a sane index for a Windows-only tenant.
- Every heading carries its resource count.
- Entries sorted by display name within a section.
- Links are **relative** to the tenant folder so the index works on disk, in VS Code and on GitHub.

**Entry format** — `- [Display name](relative/path.md) — purpose · scope · assignments`

- *purpose*: one clause, ≤120 characters, taken from the document's summary paragraph. Not a restatement
  of the title.
- *scope*: `device` or `user`, from the `_d_`/`_u_` filename token, else from the resource itself. Omit
  when it doesn't apply (apps, identity policies).
- *assignments*: `unassigned`, `N groups`, or `all users` / `all devices`, read from the document's
  assignments section. Omit for resource types that carry no assignments. **`unassigned` is worth
  surfacing** — an unassigned policy is configured but doing nothing.

### 6c. Intro and conventions

Open with, adapted to the tenant:

> This is the generated configuration documentation for the **&lt;tenant&gt;** tenant, produced from an
> `azure-resource-downloader` export on &lt;date&gt;. Each entry links to a page describing one configured
> resource: what it does, how it is targeted, and every setting it carries.
>
> These pages describe what **is** configured, not what should be. They are derived from the export alone —
> verify anything security-relevant against the live tenant before acting on it.
>
> **Why this isn't grouped like the folders.** On disk the export mirrors the Microsoft Graph API — one
> folder per endpoint. That layout answers "which API returned this?", which is rarely the question.
> "What hardens our Macs?" spans five endpoints. This index regroups the same pages by platform, then by
> purpose.

Then, **only if the export uses a consistent resource naming convention**, add a short decode table for it.
Derive the convention from the actual filenames — do not assume the one below, and omit the section
entirely if names are ad hoc. For reference, one observed convention is:

`gbl` (scope) · `af` assignment filter / `c` compliance / `cp` configuration profile · `prd` (environment) ·
`d` device / `u` user · optional `cis` (CIS baseline) · `mac` / `win` · category · name

State how many resources follow the convention and note that apps and identity objects typically don't.

Close the index with a short **Not documented** line: the excluded types and their counts, and the fact that
the groups actually used by assignments *are* documented — say how many of how many (e.g. "17 of 470 groups
are referenced by an assignment and documented; the rest are unused by any policy in this export").

---

## 7. Write back and update the manifest

- Write each `.md` next to its source `.yaml` (or into the shape the user chose), and `index.md` at the
  tenant folder root.
- `index.md` is fully derived — it has no frontmatter, no manifest entry, and is safe to delete and
  regenerate at any time.
- **Rewrite `.doc-manifest.json` last**, only after verification passes, and only from hashes that were
  actually verified. Carry forward entries for skipped-unchanged resources verbatim — including their
  `targetedBySha256`. Never write a manifest entry for a document that failed a check — leaving it out
  means the next run retries it, which is the safe failure mode.
- Update `targetedBySha256` for every group whose reverse index you re-spliced, and only those.
- Do not remove orphan entries or orphan documents.
- If the agent works in a sandbox separate from the user's disk, transfer via a single archive rather
  than file-by-file — it is dramatically cheaper than per-file transfer.
- Clean up any transfer artifacts. If the environment cannot delete on the user's disk, move them into a
  `_to_delete/` folder and tell the user which files are there.
- Add `.doc-manifest.json` to version control. It is what makes the next run cheap; if it is gitignored,
  every fresh clone does a full regeneration (recoverable from frontmatter, but slower to discover).

The manifest covers in-scope types only, **plus the referenced groups documented in section 5** — those are
tracked under `Microsoft.Graph/groups` like any other type, so they regenerate only when their YAML or the
group prompt changes. Unreferenced groups and `windowsAutopilotDeviceIdentities` never appear in it, so they
never show up as **new** on a later run.

### Forcing a full regeneration

Delete `.doc-manifest.json`, or ask for it explicitly. Every in-scope resource then falls into the **new**
bucket.
