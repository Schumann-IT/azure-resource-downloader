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

Editing convention: the numbered sections are the procedure and stay short. Background — why a step exists,
what failure it prevents — goes in the appendix at the bottom. Put explanation there, never instructions.
-->

You are generating end-user documentation for an Azure/Entra/Intune tenant export produced by
**azure-resource-downloader**.

This is an **incremental** run. The inventory and change detection have already been done for you: the work
list below is complete and closed. Document exactly what it names — nothing more, nothing less. Do not walk
the export looking for other resources, do not hash anything to decide what changed, and do not re-litigate
which resource types are in scope.

## Run parameters (already decided — do not ask)

These are fixed for this prompt. Do **not** stop to ask the operator about any of them — begin immediately.

- **Always a single full run.** Process the entire work list to completion in one pass, however long it is.
  Do not checkpoint, do not pause for approval between batches, and do not ask whether to split, sample or
  resume later. The only reason to stop early is an unrecoverable error, which you report at the end.
- **Per-chunk generation agents use Claude Sonnet.** Sonnet-class models are the required choice for the
  fan-out subagents in section 3 — do not ask which model to use and do not substitute a larger model. The
  orchestrator (chunking, splicing and verification) is the session you are in now. This is stated once,
  here; sections below do not repeat it.
- **Deterministic steps are scripted, never hand-edited.** Verification (sections 4 and 6) and the
  assignment splice (section 5) are mechanical transformations over files. Write a script, run it over the
  tree, and read its output. Do not open documents one at a time and rewrite their text through your own
  output: a model retyping a generated table paraphrases names and breaks links, and doing it once per
  document across the whole tree is the single most expensive thing you could do in this run.

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
resources/<APIType>/<endpoint>/<n>.yaml     the source
resources/<APIType>/<endpoint>/doc-prompt.md   the spec for that whole type
docs/<APIType>/<endpoint>/<n>.md            the document you write
```

The mapping is mechanical: take the source path, swap the leading `resources/` for `docs/`, swap the `.yaml`
extension for `.md`. Every step below relies on that, so never improvise an output path.

All agents share this filesystem and run from the tenant folder. Nothing is ever transferred, copied, staged
or archived for them — a subagent reads its sources from these paths directly. Copies would only add work
and let a document's `source` path drift from the tree the frontmatter checks are verified against.

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
- **Never compute a hash.** Every hash you write comes verbatim from the work-list row that names the
  document. This is the only place that rule is stated; it holds everywhere below.
- **Write only the files named below**, plus the working files in `chunks/` that section 3 calls for. No
  index, no summary document, no other scratch files left behind.

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

The hashes on each row are the values to write into that document's frontmatter (section 2), given to you
precisely so you never have to compute one. `assignmentsSha256` is blank for types that have no assignments.

Note that `promptSha256` is a property of the *type*, not of the file: every row under one `###` heading
carries the same value. Read it once per type and reuse it — never copy it once per row.

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
assignmentsSha256: 7a41c0de…      # only for types that have assignments
generatedAt: 2026-08-13T08:31:00Z  # the export timestamp from above, verbatim — not the current time
---
```

`source` is a path relative to the tenant folder, not a bare filename — the resource and its document live in
different trees.

`generatedAt` copies the export timestamp because **the hashes alone decide staleness**. Stamping wall-clock
time instead would make every regeneration rewrite every file, turning a no-op run into a full-tree diff and
destroying the mtime evidence section 6 depends on.

`assignmentsSha256` covers the resolved contents of the assignments block — the target groups' names and
kinds, the filters' names — which is how the next run notices that a group was renamed even though this
resource never changed. Group documents additionally carry `targetedBySha256`; see 5d.

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

Bare GUIDs here are correct and expected — section 5 resolves them. Emit the markers even when a resource has
no assignments; put the sentence saying so between them. Omit them only for types with no concept of
assignments. Never nest markers, never emit a start without its end, never put anything that is not about
assignments between them. Section 5 depends on this being a deterministic splice.

---

## 3. Generate (parallel subagents)

A large work list is too much content for one context. Fan it out — but pack the chunks deliberately, because
every chunk costs one agent, one full spec read, and one round trip.

### 3.1 Chunk by expected output, not by input bytes

The constraint that actually bites is the agent's output budget: the defect section 4 checks for — a
truncated final block — is what happens when a chunk's documents do not fit in one response. Input size only
approximates that.

- **Measure, never read.** Size chunks from file metadata — `ls -l`, `wc -c`, `grep -c` over the resource
  tree — and never by loading a resource YAML into your own context to look at it. The only YAML the
  orchestrator ever reads itself is the group lookup in 5a, inside the splice script.
- **One type per chunk, and prefer one chunk per type.** Every extra chunk of the same type re-reads the same
  `doc-prompt.md` in full. Never mix types in a chunk.
- **Small types pack hard.** Assignment filters, role scope tags, notification templates and the like produce
  a page or two each: 20–30 files in a single chunk is appropriate. Do not give three small files their own
  agent.
- **Settings-heavy types split.** A settings-catalog policy expands into hundreds of `<details>` blocks, so
  the settings count is the real budget, not the byte count. Give any source over ~60 KB of YAML its own
  chunk, and split a large type into as many chunks as its output demands.
- Target roughly one chunk per type plus a handful of extra chunks for the few large types. Fewer,
  better-packed agents beat many small ones.

### 3.2 Write the shared rules once

Write `chunks/_common.md` a single time. It contains: the ground rules from section 0, the heading,
frontmatter and marker rules from section 2, the output-path mapping, and the self-check and receipt format
in 3.4. **Do not copy it into each chunk file** — every agent reads it directly.

### 3.3 Write one minimal chunk file per chunk

`chunks/NN.md` carries only what is specific to that chunk. The type's `promptSha256` is stated once in the
header, never per row:

```
type: Microsoft.Graph/assignmentFilters
spec: resources/Microsoft.Graph/assignmentFilters/doc-prompt.md
promptSha256: f4304780fca043cfe3836a68bc5466ce95dcc8f5ef8cf386498b50a9fa7a410a

resources/Microsoft.Graph/assignmentFilters/gbl_af_prd_d_mac_mgm_modernworkplace.yaml  3c465824019e0f567c1fcd78cae4b944ca16be4085ae25b1c616522977e3e10c
resources/Microsoft.Graph/assignmentFilters/gbl_af_prd_d_win_mgm_lenovo_devices.yaml   6012be0fb2700e870c59c728b9e950ed104ca51413436b19414f5d30fbd72ece
```

Copy each `sourceSha256` from its work-list row exactly. Output paths are derived, not listed — the mapping
above is unambiguous, and writing them out doubles the transcription risk for no benefit.

### 3.4 Spawn the agents

Keep about **10 agents in flight, refilling as each finishes** — do not wait for a whole batch to drain before
starting the next. Each agent's prompt is exactly:

> Read `chunks/_common.md`, then `chunks/NN.md`, and follow them exactly.

Keeping chunk contents out of the orchestrator's context is what makes a large run feasible.

These rules belong in `_common.md` verbatim, because they are what each agent must do to its own output:

- Read every assigned YAML **in full** before writing anything.
- Write exactly one document per source file, at the derived path, and nothing else.
- Close each `<details>` immediately after its content. Never leave one open across a heading boundary.
- Before finishing, count `<details>` against `</details>` in what you wrote and repair any imbalance
  yourself.
- Return one line and nothing more: `DONE <chunk> <files written> <open tags> <close tags>`.

The counts in the receipt make truncation and tag imbalance visible at the agent that caused them, while it
still has the context to fix them, instead of in a central repair pass that has to reconstruct intent.

---

## 4. Verify structure (mandatory — do not skip)

These checks test each document **in isolation**, so run them now, before assignments are touched. The checks
that compare documents *to each other* — GUID resolution and link symmetry — cannot pass yet: at this point
every assignments block still holds bare GUIDs by design. They are in section 6.

| Check | Expectation |
|---|---|
| Coverage | Every work-list entry produced a document at its derived path. Zero missing. Diff the source paths in `chunks/*.md` against what exists on disk. |
| Frontmatter | Every written document has valid frontmatter whose `sourceSha256` and `promptSha256` equal the values from its chunk file. A mismatch means an agent documented the wrong file. |
| Heading structure | Exactly one `#` heading per document. Count headings **outside fenced code blocks only** — shell scripts embedded in `deviceShellScripts` / `deviceManagementScripts` documents contain `##` comment lines that are not headings. |
| `<details>` balance | `<details>` count equals `</details>` count per file. Nesting is normal. |
| Assignment markers | Every document that should have them has exactly one matched `<!-- assignments:start -->` / `<!-- assignments:end -->` pair — never unbalanced, never nested, never repeated. |
| Stray artifacts | No leftover `DONE` receipts in document text, no truncated final block, every file ends cleanly. |
| Nothing else touched | No document outside the work list was created or modified. **Record every document's mtime once these checks pass** — section 5 legitimately rewrites the files it splices, and section 6 compares against this snapshot. |

Run them with a script; do not eyeball hundreds of files. This one covers the per-document checks:

````python
#!/usr/bin/env python3
"""Section 4 structural checks. Run from the tenant folder. Exit 1 if anything failed."""
import json, pathlib, re, sys, time
from collections import Counter

FENCE = re.compile(r"^\s*(```|~~~)")
problems, seen = Counter(), set()

def fail(doc, msg):
    problems[msg] += 1
    print(f"FAIL {doc}: {msg}")

# expected[source path] = (doc path, promptSha256, sourceSha256)
expected = {}
for chunk in sorted(pathlib.Path("chunks").glob("[0-9]*.md")):
    header, _, body = chunk.read_text(encoding="utf-8").partition("\n\n")
    meta = {}
    for line in header.splitlines():
        key, _, val = line.partition(":")
        if val:
            meta[key.strip()] = val.strip()
    for line in body.splitlines():
        if not line.strip():
            continue
        src, _, sha = line.strip().partition(" ")
        doc = re.sub(r"^resources/", "docs/", src.strip()).removesuffix(".yaml") + ".md"
        expected[src.strip()] = (doc, meta.get("promptSha256", ""), sha.strip())

for src, (docpath, prompt_sha, source_sha) in expected.items():
    doc = pathlib.Path(docpath)
    seen.add(doc)
    if not doc.is_file():
        fail(doc, "missing — no document written")
        continue
    text = doc.read_text(encoding="utf-8")

    if not text.startswith("---\n") or text.count("\n---\n") < 1:
        fail(doc, "missing or malformed frontmatter")
    else:
        fm = text.split("---\n", 2)[1]
        for key, want in (("sourceSha256", source_sha), ("promptSha256", prompt_sha)):
            got = re.search(rf"^{key}:\s*(\S+)", fm, re.M)
            if not got:
                fail(doc, f"frontmatter missing {key}")
            elif want and got.group(1) != want:
                fail(doc, f"{key} mismatch — wrong source documented")
        if not re.search(r"^source:\s*resources/", fm, re.M):
            fail(doc, "frontmatter source is not a resources/ path")

    in_fence, h1 = False, 0
    for line in text.splitlines():
        if FENCE.match(line):
            in_fence = not in_fence
        elif not in_fence and line.startswith("# "):
            h1 += 1
    if h1 != 1:
        fail(doc, f"expected exactly one H1, found {h1}")

    opens, closes = text.count("<details"), text.count("</details>")
    if opens != closes:
        fail(doc, f"<details> imbalance: {opens} open / {closes} close")

    for marker in ("assignments", "targeted-by"):
        s, e = text.count(f"<!-- {marker}:start -->"), text.count(f"<!-- {marker}:end -->")
        if s != e:
            fail(doc, f"{marker} markers unbalanced: {s} start / {e} end")
        elif s > 1:
            fail(doc, f"{marker} markers repeated ({s} pairs)")

    if re.search(r"^DONE ", text, re.M):
        fail(doc, "leftover DONE receipt")
    if not text.endswith("\n") or text.rstrip().endswith(("<details>", "|", "<summary>")):
        fail(doc, "file ends mid-block — likely truncated")

for doc in pathlib.Path("docs").rglob("*.md"):
    if doc not in seen:
        fail(doc, "document exists but is not in the work list")

pathlib.Path("chunks/mtimes.json").write_text(json.dumps(
    {str(p): p.stat().st_mtime for p in sorted(seen) if p.is_file()}, indent=0))
print(f"\nchecked {len(expected)} documents at {time.strftime('%H:%M:%S')}")
for msg, n in problems.most_common():
    print(f"{n:5d}  {msg}")
sys.exit(1 if problems else 0)
````

Imbalanced `<details>` tags are the most common defect and they silently break rendering in GitHub and most
viewers. Repair by walking the file, tracking depth outside code fences, closing anything still open before
the next heading or at EOF, and dropping unmatched closing tags. Re-run the checks and report what you
repaired.

---

## 5. Assignments

Documents name their assignment targets by raw GUID. A page saying
`groupId: 8964516b-c223-4f58-a866-232d3690c9b4` is accurate and useless. This step resolves those GUIDs and
links the two directions together. It is a **deterministic splice**: one script, run over the tree, replacing
the contents of marked blocks. Do not spawn agents for it and do not retype tables yourself.

### 5a. Reference map (given)

<!-- refmap:start -->
_Replaced by the tool: group GUID → display name, its document path and its kind
(assigned/dynamic · security/Microsoft 365), resolved from `metadata.yaml`. Includes any GUID with no
matching group in the export, flagged as dangling._
<!-- refmap:end -->

Two values need care:

- `00000000-0000-0000-0000-000000000000` as a **filter** ID is Intune's "no filter" sentinel. Render it as
  `none`. It is not an unresolvable reference and must never be reported as one.
- A group GUID flagged **dangling** above has no group in the export — usually deleted from the tenant while
  still assigned. Keep the raw GUID, mark it `⚠️ not in export`, and list every one in your final report.

The map already carries each group's name, document and kind — the `dynamic security group` /
`assigned Microsoft 365 group` annotation is given, not derived, so **drive every assignment table directly
from the map** and never open a group's YAML to recover it. The one thing the map does not carry is
`membershipRule`: read that from a dynamic group's YAML only when you are documenting that group (5b), not
when a policy merely targets it.

### 5b. Document referenced groups

Referenced groups needing a document appear in the work list like any other resource, with
`resources/Microsoft.Graph/groups/doc-prompt.md` as their spec. A dynamic group's `membershipRule` is what
actually decides who receives every policy assigned to it — explain it clause by clause and flag where it
keys on user-editable attributes.

### 5c. Rewrite assignment blocks

For every document you wrote, replace everything between `<!-- assignments:start -->` and
`<!-- assignments:end -->` with a table built from the YAML and the lookup above. The markers stay. Generate
fresh — never edit the previous table in place. This is part of the same script as everything else in
section 5; it runs over the whole tree in one pass.

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

A marked block can go stale while the document around it is perfectly current, because it is rendered from
information that lives outside the document's own resource — see appendix A for how that happens in both
directions. Such documents never appear in the work list. Each needs **one marked block replaced and its body
left completely alone**: do not regenerate them, do not re-read their specs.

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

Sort by resource type then name, and state the total above the table. Build it by inverting the same lookup
that produced the forward tables, so the two directions cannot disagree. Never hand this block to an agent —
it is generated data, and a model rewriting it will paraphrase names and break links.

### 5e. Migrate documents written before markers existed

Documents predating the markers need them inserted before 5c can splice anything; see appendix B.

<!-- migrate:start -->
_Replaced by the tool: documents that have an assignments table but no `<!-- assignments:start -->` marker.
Rendered as "none" when there are none._
<!-- migrate:end -->

For each, insert the markers around the existing block without altering anything else, then apply 5c
normally. This is a one-off per document; once migrated the markers persist.

---

## 6. Verify references (mandatory — do not skip)

Section 4 checked each document against itself. These checks compare documents to each other and to the
reference map, so they only become meaningful once section 5 has run. Script them the same way.

| Check | Expectation |
|---|---|
| Assignment resolution | No bare group GUID remains inside a marked block without a resolved name beside it or an explicit `⚠️ not in export`. |
| Link symmetry | Every group linked from a policy's assignment table has a document, and that document's **Targeted by** list contains that policy. Both directions come from one lookup, so a mismatch means the splice went wrong. |
| Link targets exist | Every relative link inside a marked block resolves to a file that exists under `docs/`. |
| Marker pairs survived | The splice left every `assignments` and `targeted-by` pair matched and unnested. Re-run that check from section 4 — a bad splice is exactly how a pair gets broken. |
| Hashes updated | Every document whose block was re-spliced carries the new `assignmentsSha256` / `targetedBySha256`. Any it kept from before will be re-spliced on every future run. |
| Nothing else touched | Compare mtimes against the snapshot taken at the end of section 4. Only work-list documents, re-spliced documents and migrated documents may have changed. |

---

## 7. Report

Finish with:

- documents written, and documents re-spliced
- checks passed at section 4 and at section 6, and anything you repaired
- every **dangling group reference** (an assigned GUID with no group in the export)
- how many documents were migrated to markers
- whether the export was marked incomplete, and why
- substantive findings the analysis surfaced: baseline deviations, disabled controls, secrets present,
  policies with no assignments

Findings are derived from the export alone. Say so, and recommend verifying anything security-relevant
against the live tenant.

---

## Appendix — background

Nothing here is an instruction. It exists so the procedure above can stay short.

### A. Why re-splicing exists (5d)

Two marked blocks are rendered from information that lives outside the document's own resource, so either can
go stale while the document around it is still correct:

- **Forward** — a document's own assignments table prints its target groups' *names*. The resource stores
  only GUIDs, so renaming a group in the tenant leaves every policy targeting it printing a stale name while
  no policy's YAML moved.
- **Reverse** — a group document's `Targeted by` block lists *other* resources' assignments. Re-point a
  policy from one group to another and both groups' reverse indexes are wrong while neither group's YAML
  moved.

Neither case can appear in the work list, because nothing about those documents' own resources changed. That
is the whole reason 5d exists as a separate step with its own list.

### B. Why marker migration exists (5e)

Documents written before the assignment markers were introduced have an assignments table but no markers
around it. The splice in 5c is defined as "replace what is between the markers", which is a no-op on those
documents — so they would keep an unresolved table forever. Inserting the markers once brings them into the
normal cycle.

### C. Why the verification is split across sections 4 and 6

The two sets of checks answer different questions and cannot both run at the same point. Section 4 asks
whether each document is *well-formed*, which is true the moment an agent finishes writing it and is worth
knowing before the splice runs over the whole tree. Section 6 asks whether the documents *agree with each other*,
which cannot be true until the splice has resolved every GUID and built both directions of the link graph.
Running the referential checks early reports a failure on every document in the tree; running the structural
checks only at the end means a truncated document is discovered after the splice has already processed it.