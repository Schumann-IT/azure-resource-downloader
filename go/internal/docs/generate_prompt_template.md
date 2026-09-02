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
  usedbymap  notification-template GUID -> template name/document and the resources that reference it
  resplice   documents needing one marked block re-rendered though their own resource did not change:
             assignments tables whose target names moved, "Targeted by" blocks whose targeting moved,
             "Used by" blocks whose referencing resources moved, and noncompliance-notification blocks
             whose referenced template was renamed
  migrate    documents predating the assignment markers; rendered as "none" when there are none
  summary-facts  tenant-wide counts, platforms, assignment posture and coverage for the summary (section 7)

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
- **Write only the files named below**, plus `docs/summary.md` (section 7) and the working files in
  `chunks/` that section 3 calls for. No index, no other scratch files left behind.

---

## 1. What to document

The list is authoritative. Each entry gives the source YAML, the document to write, and why it is listed.

<!-- worklist:start -->
_Replaced by the tool. Shape:_

### Microsoft.Graph/deviceCompliancePolicies — spec: `resources/Microsoft.Graph/deviceCompliancePolicies/doc-prompt.md`

| Source | Document | Reason | sourceSha256 | promptSha256 | assignmentsSha256 | notificationsSha256 | usedBySha256 |
|---|---|---|---|---|---|---|---|
| `resources/…/gbl_c_prd_d_win_os_validation.yaml` | `docs/…/gbl_c_prd_d_win_os_validation.md` | resource changed | `5d6b32f8…` | `95cb34be…` | `7a41c0de…` | `a3f8b91e…` | |

_…one section per resource type, then a tally: N documents to write across M types._
<!-- worklist:end -->

The hashes on each row are the values to write into that document's frontmatter (section 2), given to you
precisely so you never have to compute one. `assignmentsSha256` is blank for types that have no assignments.
`notificationsSha256` is blank unless the resource references a notification template in a noncompliance
action. `usedBySha256` is blank unless the resource is a notification message template.

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
resource never changed. Group documents additionally carry `targetedBySha256`, notification message
template documents carry `usedBySha256`, and documents that reference a notification template in a
noncompliance action carry `notificationsSha256`; see 5d.

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

The same contract applies to two more marked blocks, wherever the spec calls for them: `<!-- targeted-by -->`
around a group document's list of the resources that assign it, and `<!-- notifications:start -->` /
`<!-- notifications:end -->` around a policy's reference to the notification message template its noncompliance
actions send through. Unlike assignments, the notifications block is emitted **only** when the resource
actually references a template — omit it entirely otherwise, since there is nothing to re-splice.

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
| Heading vocabulary | Every `##` **outside fenced code blocks and outside `<!-- …:start -->`/`<!-- …:end -->` marker pairs** is in the closed set declared for that document's type — the `doc-headings` list in its `doc-prompt.md` — spelled exactly, in order, without duplicates. The marker-pair exemption is what lets the tool-spliced `## Targeted by` block (section 5) live in group documents without being part of the authored contract. Same fenced-code caveat as *Heading structure*. |
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

MARKER = re.compile(r"^<!--\s*[\w-]+:(start|end)\s*-->\s*$")
HEADING = re.compile(r"^(#+)\s+\S")
INLINE = re.compile(r"`[^`]*`")
_contracts = {}

def heading_contract(src):
    """Ordered closed H2 set for src's type, read from its doc-prompt.md."""
    spec = pathlib.Path(src).parent / "doc-prompt.md"
    if spec not in _contracts:
        val = None
        if spec.is_file():
            m = re.search(r"<!--\s*doc-headings:\s*(.+?)\s*-->", spec.read_text(encoding="utf-8"))
            if m:
                val = [h.strip() for h in m.group(1).split("|") if h.strip()]
        _contracts[spec] = val
    return _contracts[spec]

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

    in_fence, in_marker, h1, h2s, opens, closes = False, False, 0, [], 0, 0
    for line in text.splitlines():
        if FENCE.match(line):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        bare = INLINE.sub("", line)  # a <details> mentioned in inline code is prose, not a tag
        opens += bare.count("<details")
        closes += bare.count("</details>")
        mk = MARKER.match(line)
        if mk:
            in_marker = mk.group(1) == "start"
            continue
        if in_marker:
            continue
        h = HEADING.match(line)
        if h:
            level = len(h.group(1))
            if level == 1:
                h1 += 1
            elif level == 2:
                h2s.append(line.lstrip("#").strip())
    if h1 != 1:
        fail(doc, f"expected exactly one H1, found {h1}")

    contract = heading_contract(src)
    if contract is None:
        # Tolerated, not failed: the type's doc-prompt.md predates the
        # doc-headings marker (e.g. a type that could not be listed and was not
        # refreshed this run). There is nothing to validate against.
        print(f"NOTE {doc}: no doc-headings contract in its doc-prompt.md — heading vocabulary not checked")
    else:
        allowed = set(contract)
        extra = [h for h in dict.fromkeys(h2s) if h not in allowed]
        if extra:
            fail(doc, f"unexpected H2 heading(s): {', '.join(extra)}")
        dupes = [h for h in dict.fromkeys(h2s) if h2s.count(h) > 1]
        if dupes:
            fail(doc, f"duplicate H2 heading(s): {', '.join(dupes)}")
        it = iter(contract)
        if not all(h in it for h in h2s if h in allowed):
            fail(doc, "H2 headings out of contract order")

    if opens != closes:
        fail(doc, f"<details> imbalance: {opens} open / {closes} close")

    for marker in ("assignments", "targeted-by", "used-by", "notifications"):
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
    # Tool- and agent-owned files (generate.md, summary.md, report-*.md) live at
    # the docs root; real documents are always at least two levels deep.
    if doc.parent == pathlib.Path("docs"):
        continue
    if doc in seen:
        continue
    # A document outside the work list is normal: an incremental run rewrites
    # only what changed, and a document is legitimately retained when its type
    # was not regenerated this run (e.g. a type that could not be listed). It is
    # a problem only when it is a stray (no frontmatter tying it to a resource)
    # or misplaced (its content belongs at a different derived path).
    body = doc.read_text(encoding="utf-8")
    fm = body.split("---\n", 2)[1] if body.startswith("---\n") and body.count("\n---\n") >= 1 else ""
    m = re.search(r"^source:\s*(resources/\S+\.yaml)\s*$", fm, re.M)
    if not m:
        fail(doc, "untracked document with no resource frontmatter")
        continue
    want = pathlib.Path(re.sub(r"^resources/", "docs/", m.group(1)).removesuffix(".yaml") + ".md")
    if doc != want:
        fail(doc, f"misplaced document — its source maps to {want}")

# Baseline mtimes for section 6, covering the whole document tree (not just the
# work list) so a retained document is present and shows unchanged rather than
# as an extra. Write it once: re-running section 4 after the section-5 splice
# must not overwrite the pre-splice baseline it compares against.
snapshot = pathlib.Path("chunks/mtimes.json")
if not snapshot.exists():
    tree = [p for p in pathlib.Path("docs").rglob("*.md") if p.parent != pathlib.Path("docs")]
    snapshot.write_text(json.dumps(
        {str(p): p.stat().st_mtime for p in sorted(tree) if p.is_file()}, indent=0))
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
_Replaced by the tool, in four groups: documents whose **assignments** block must be re-rendered (with the
resolved targets and the new `assignmentsSha256`), group documents whose **Targeted by** block must be
re-rendered (with the targeting resources and the new `targetedBySha256`), notification message template
documents whose **Used by** block must be re-rendered (with the referencing resources and the new
`usedBySha256`), and policy documents whose **noncompliance-notification** block must be re-rendered (with the
resolved template name and the new `notificationsSha256`). Rendered as "none" when nothing needs re-splicing._
<!-- resplice:end -->

After splicing, update that document's frontmatter with the hash the tool gave you — `assignmentsSha256` for
a forward re-splice, `targetedBySha256` for a reverse one, `usedBySha256` for a used-by one,
`notificationsSha256` for a notifications one. A document whose block you re-rendered but whose hash you did
not update will be re-spliced again on every future run.

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

The notification message template `Used by` block works exactly the same way, one direction over: it lists
the compliance policies that reference the template in a noncompliance action, built from the template
reference map in 5f. Wrap it in its own markers:

```markdown
<!-- used-by:start -->
## Used by

3 resources reference this template in a noncompliance action.

| Resource | Type |
|---|---|
| [GBL_C_PRD_D_WIN_OS_validation](../deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.md) | deviceCompliancePolicies |
<!-- used-by:end -->
```

State the total above the table and sort by resource type then name. A template no resource references gets
one sentence between the markers: *"No resource references this template in a noncompliance action."* — a
finding worth stating plainly, exactly like an unassigned resource. Drive it from the reference map in 5f,
never from re-reading compliance policies, and never hand it to an agent.

The **forward** counterpart lives inside a *policy* document, not a template document: a compliance policy
whose noncompliance actions send through a notification template names that template in its Settings section,
wrapped in its own markers so a template rename re-splices just that reference — not the whole page:

```markdown
<!-- notifications:start -->
Noncompliance actions notify through [Compliance email](../notificationMessageTemplates/compliance_email.md).
<!-- notifications:end -->
```

Resolve the template name and its document from the same 5f reference map used for the reverse block, so the
two directions cannot disagree; keep the raw GUID and mark it `⚠️ not in export` for a dangling reference.
This block appears **only** for policies that actually reference a template.

### 5e. Migrate documents written before markers existed

Documents predating the markers need them inserted before 5c can splice anything; see appendix B.

<!-- migrate:start -->
_Replaced by the tool: current documents missing a marker their content needs — an assignments table with no
`<!-- assignments:start -->` marker, or a template-referencing policy with no `<!-- notifications:start -->`
marker. The row's reason names which. Rendered as "none" when there are none._
<!-- migrate:end -->

For each, insert the markers around the existing block without altering anything else, then apply 5c
normally. This is a one-off per document; once migrated the markers persist.

### 5f. Notification template reference map (given)

<!-- usedbymap:start -->
_Replaced by the tool: notification message template GUID → display name, its document path, and the
resources that reference it in a noncompliance action (name, document, type), resolved from `metadata.yaml`.
A template no resource references is listed as such; a referenced GUID with no template in the export is
flagged dangling._
<!-- usedbymap:end -->

This is the used-by counterpart of the assignment reference map in 5a: it already carries every template's
name, document and referencing resources, so build each `Used by` block (and, if a compliance policy
document mentions the template it notifies through, the forward mention) directly from it. A GUID flagged
**dangling** has no template in the export — usually deleted from the tenant while still referenced; keep the
raw GUID, mark it `⚠️ not in export`, and list every one in your final report.

---

## 6. Verify references (mandatory — do not skip)

Section 4 checked each document against itself. These checks compare documents to each other and to the
reference map, so they only become meaningful once section 5 has run. Script them the same way.

| Check | Expectation |
|---|---|
| Assignment resolution | No bare group GUID remains inside a marked block without a resolved name beside it or an explicit `⚠️ not in export`. |
| Link symmetry | Every group linked from a policy's assignment table has a document, and that document's **Targeted by** list contains that policy. Every template a compliance policy references — both from that policy's **noncompliance-notification** block and from the template's **Used by** list — agrees in both directions. Each direction comes from one lookup, so a mismatch means the splice went wrong. |
| Link targets exist | Every relative link inside a marked block resolves to a file that exists under `docs/`. |
| Marker pairs survived | The splice left every `assignments`, `targeted-by`, `used-by` and `notifications` pair matched and unnested. Re-run that check from section 4 — a bad splice is exactly how a pair gets broken. |
| Hashes updated | Every document whose block was re-spliced carries the new `assignmentsSha256` / `targetedBySha256` / `usedBySha256` / `notificationsSha256`. Any it kept from before will be re-spliced on every future run. |
| Nothing else touched | Compare mtimes against the snapshot taken at the end of section 4. Only work-list documents, re-spliced documents and migrated documents may have changed. A document outside the work list — e.g. one retained under a type that could not be listed — is in the snapshot and must be unchanged. |

---

## 7. Tenant summary (write `docs/summary.md`)

After every document is written and both verification passes are green, write one overview of the tenant's
Intune/Entra management posture to `docs/summary.md`. It is the tenant's landing page in the documentation
frontend — prose for an operator and the manager they report to; the machine-readable index lives in
`index.yaml`. This is the only file besides the documents this run produces, and the only place section 0's
"no summary document" rule is lifted.

**The length is fixed: about one page, 600–900 words.** Do not ask the operator how long, how deep or how
formal it should be — this line settles it. When something does not fit, cut detail, never a section.

Above the first H2, write the page preamble in this fixed shape, and emit nothing else there:

- A single H1 title, verbatim: `# Tenant summary`. This is the file's only H1.
- Directly below it, without a heading, one orientation sentence: name the tenant (its Entra default domain,
  which is this export's folder name) and state that the page summarizes the **exported** Intune/Entra
  configuration, not the live tenant. Keep judgements out of it — posture, findings and recommendations
  belong in **Management summary** below.

Then write these four sections, in this order:

**1. Management summary** — the top half of the page, and the only part that judges anything. Open with one
paragraph on the overall posture (no heading): what is managed, how consistently, and whether the
configuration that exists is actually in force. Then two H3 subsections, in this order, each heading written
verbatim:

- `### Findings` — at most six, rendered as a table, one finding per row, sorted by severity with the most
  serious first (all `critical` rows, then `high`, then `medium`). The table has these columns, in this
  order: **Severity**, **Finding**, **Affected**, **Documents**.
  - **Severity** — one value from a closed set, `critical`, `high` or `medium`, and no other; the frontend
    ranks and filters on it, and the row order must match it.
  - **Finding** — the fact and its consequence in one line, e.g. *"All Conditional Access policies are in
    report-only mode — no identity control in this tenant is currently enforced."* Never reprint a credential
    value: name the resource and the field that holds it.
  - **Affected** — the number of resources the finding covers as a bare number, or `—` when it is not
    resource-scoped.
  - **Documents** — a relative link (or links) to the documents that carry the detail.
- `### Recommendations` — at most four, each tied to a finding above and naming what to act on. Prefer the
  concrete instruction (*"resolve or remove the 9 assignments pointing at the deleted group
  `06f19a9f-…`"*) over the generic principle. Where acting needs information the export does not hold, say
  what to check in the live tenant rather than guessing.

**2. At a glance** — which platforms are managed and which management areas are present, with the per-type
counts grouped into areas (compliance, device configuration / settings catalog, app protection, app
deployment, enrollment / Autopilot, updates, scripts, Entra policies) in your own words, as prose or one
small table. The block gives raw types; the grouping is yours. Name an area absent rather than omitting it
silently, and make the area counts add up to the block's total.

**3. Assignment posture** — assigned versus configured-but-unassigned, the balance of dynamic versus
assigned groups and All users / All devices targets, and how many distinct groups actually carry the
assignments. A tenant whose policies all hang off two or three membership rules is a different tenant from
one that spreads across thirty, and the reader cannot see that from the totals alone.

**4. Coverage caveats** — whether the export was incomplete and why, any type that could not be listed, any
type that listed empty, and how many resources are retained but no longer in the tenant.

These four H2 headings are a closed set and a machine contract, exactly as in the per-type documents: the
documentation frontend styles and deep-links `summary.md` by heading slug the same way it does every
document. Write them verbatim — `## Management summary`, `## At a glance`, `## Assignment posture`,
`## Coverage caveats` — with that exact wording and casing, no numbering, in this order, and emit no other
H2. The numbers above label the sections for this instruction only; they are not part of the heading text.

The H3 sub-vocabulary is closed the same way: the only H3 headings anywhere in the file are `### Findings`
and `### Recommendations`, both inside **Management summary** and in that order. Write them verbatim and
emit no other H3 — the other three sections carry no subheadings. Deeper structure inside a subsection (H4,
lists, one small table) is free.

Links in this file are relative to `docs/`, the directory it sits in — `Microsoft.Graph/groups/x.md`. That
is neither the `../groups/x.md` form the documents use between themselves nor the `docs/`-prefixed form the
reference map (5a) prints: when you lift a link from 5a, drop its leading `docs/`.

### Where the facts come from

Three inputs, and nothing else. Never re-read the document tree to count or conclude anything, and never
carry over an observation an agent made earlier in this session — the summary must come out the same on a
run that generated 148 documents and on a run that generated none.

- The `summary-facts` block below: every count, platform and posture number.
- The reference map in 5a: which groups are actually targeted, and how many there are.
- A **signal sweep**: one script over `resources/`, run once, covering exactly these five checks and no
  others. Report each signal with the resources it names, deduplicated.

  | Signal | What to collect |
  |---|---|
  | Not in force | Resources whose own state field says they are not enforced — `state: enabledForReportingButNotEnforced`, `state: disabled`, `isEnabled: false` — counted per type. |
  | Configured but unassigned | Assignment-capable resources with no assignment: display name and document path. |
  | Dangling targets | Assigned group GUIDs with no group in the reference map, and how many resources assign each. |
  | Credentials near expiry | Any expiry field (`expirationDateTime`, `tokenExpirationDateTime`, …) within 180 days of the export timestamp, and any already past it. Values are quoted in the YAML — match single, double and unquoted alike. |
  | Plaintext credentials | A credential-shaped value the service did **not** mask, found by exactly three rules: (a) an XML/plist `<key>` naming a credential whose `<string>` holds one; (b) a scalar whose own key names a credential; (c) a `description` or `notes` value where a credential-shaped token follows a credential word. |

  A value is **credential-shaped** when it is at least 10 characters and either a hex run of 16+ characters
  or a mix of at least three character classes. Test the hex case first: a long hex key is
  lowercase-alphanumeric and an identifier exclusion would otherwise swallow it. Not credential-shaped, and
  never a finding: anything the service masked (`encryptedValueToken`, `*****`, redacted certificates),
  GUIDs, URLs, and identifiers — `device_vendor_msft_…`, `com.apple.…`, bare snake_case, bare camelCase.
  Do not widen rule (c) beyond free-text fields: run over a whole file it matches every camelCase setting
  name in the export and buries the real hits.

Nothing outside these three inputs belongs in the management summary. Deeper per-resource analysis — a
baseline deviation, a contradictory condition, an unreachable policy — stays in that resource's own document
and in the section 8 report; the summary points at the documents instead of restating them.

<!-- summary-facts:start -->
_Replaced by the tool: tenant-wide counts per type, platforms, assignment posture and coverage, computed
from `resources/metadata.yaml`._
<!-- summary-facts:end -->

### Rules

- Every finding and every recommendation names the count, resource or type it rests on. If the export
  cannot support a judgement, say so plainly instead of hedging it into the text.
- Close by naming the checks that ran. Absence of a finding is not a clean bill of health, and a bare list
  of findings reads as a security review when it is not one.
- No scoring, no maturity rating, no percentage the fact block does not contain, and no claim about
  compliance with a benchmark — the export shows configuration, not effectiveness.
- Findings are derived from the export alone; recommend verifying anything security-relevant against the
  live tenant.

### Verify the summary (mandatory)

`summary.md` is written after section 4's sweep has already run, and it lives at the `docs/` root the sweep
skips — so it needs its own check. Run this once, after writing the file, and fix any failure before
section 8. It enforces the contract declared above: the `# Tenant summary` preamble, the closed H2 set and
order, the `### Findings` / `### Recommendations` H3 sub-vocabulary, and the findings table's columns, closed
`Severity` values and severity ordering.

````python
#!/usr/bin/env python3
"""Section 7 summary check. Run from the tenant folder after docs/summary.md is written. Exit 1 on any failure."""
import pathlib, re, sys

FENCE = re.compile(r"^\s*(```|~~~)")
MARKER = re.compile(r"^<!--\s*[\w-]+:(start|end)\s*-->\s*$")
HEADING = re.compile(r"^(#+)\s+(.*\S)\s*$")

H1 = "Tenant summary"
H2 = ["Management summary", "At a glance", "Assignment posture", "Coverage caveats"]
H3 = ["Findings", "Recommendations"]
SEV = ["critical", "high", "medium"]
COLS = ["Severity", "Finding", "Affected", "Documents"]

problems = []
def fail(msg):
    problems.append(msg)
    print(f"FAIL summary.md: {msg}")

path = pathlib.Path("docs/summary.md")
if not path.is_file():
    print("FAIL summary.md: missing — not written")
    sys.exit(1)
lines = path.read_text(encoding="utf-8").splitlines()

# Heading walk, skipping fenced code and marker-pair spans.
in_fence = in_marker = False
h1s, h2s, h3s, cur_h2 = [], [], [], -1
for ln in lines:
    if FENCE.match(ln):
        in_fence = not in_fence
        continue
    if in_fence:
        continue
    mk = MARKER.match(ln)
    if mk:
        in_marker = mk.group(1) == "start"
        continue
    if in_marker:
        continue
    h = HEADING.match(ln)
    if not h:
        continue
    level, title = len(h.group(1)), h.group(2).strip()
    if level == 1:
        h1s.append(title)
    elif level == 2:
        h2s.append(title)
        cur_h2 += 1
    elif level == 3:
        h3s.append((cur_h2, title))

if h1s != [H1]:
    fail(f"preamble H1 must be exactly one '# {H1}', got {h1s}")
if h2s != H2:
    fail(f"H2 headings must be {H2} verbatim and in order, got {h2s}")
misplaced = [t for i, t in h3s if i != 0]
if misplaced:
    fail(f"H3 headings are only allowed under Management summary; found: {misplaced}")
found_h3 = [t for i, t in h3s if i == 0]
if found_h3 != H3:
    fail(f"H3 sub-vocabulary must be {H3} verbatim and in order, got {found_h3}")

# Findings table: rows between '### Findings' and the next heading.
grab, header, rows = False, None, []
for ln in lines:
    h = HEADING.match(ln)
    if h:
        if grab and len(h.group(1)) <= 3:
            break
        grab = len(h.group(1)) == 3 and h.group(2).strip() == "Findings"
        continue
    if grab and "|" in ln:
        cells = [c.strip() for c in ln.strip().strip("|").split("|")]
        if set("".join(cells)) <= set("-: "):
            continue
        if header is None:
            header = cells
        else:
            rows.append(cells)

if header is None:
    fail("Findings section has no table")
elif header != COLS:
    fail(f"Findings table columns must be {COLS}, got {header}")
else:
    sev = [r[0].strip().lower().strip("*[] ") for r in rows if r]
    bad = [s for s in sev if s not in SEV]
    if bad:
        fail(f"Findings Severity must be one of {SEV}; got {bad}")
    rank = [SEV.index(s) for s in sev if s in SEV]
    if rank != sorted(rank):
        fail("Findings rows not sorted by severity (critical, then high, then medium)")

print(f"summary.md: {len(problems)} problem(s)" if problems else "summary.md OK")
sys.exit(1 if problems else 0)
````

---

## 8. Report

Write the run report to a new file `docs/report-<UTC-date>-<UTC-time>.md` — the run's finish time in UTC,
e.g. `docs/report-2026-08-31-200617.md` (`report-YYYY-MM-DD-HHMMSS`) — and print the same text as the run's
final output. Writing the file to disk is mandatory, not optional: printing the report is never a substitute
for saving it, and a run has not finished until the file exists. One report per run: never overwrite an
earlier report and never hash-track it. Like `summary.md` it lives at the `docs/` root, so the section-4
sweep leaves it untouched.

The report must contain:

- **the plan the run executed** — reproduce the section 1 work list in full (every document to generate and
  to re-splice, grouped by type, with the reason each was listed). Include it even though the plan was an
  input to the run and was never explicitly asked for in this list: the report must stand alone as the record
  of what the run set out to do and what it did, so a reader never has to go back to the prompt to see the plan.
- documents written, and documents re-spliced
- that `docs/summary.md` was written and passed its section 7 check
- checks passed at sections 4, 6 and 7, and anything you repaired
- every **dangling group reference** (an assigned GUID with no group in the export)
- every **dangling notification template reference** (a template GUID a noncompliance action references with no template in the export)
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

Four marked blocks are rendered from information that lives outside the document's own resource, in three
directions, so any of them can go stale while the document around it is still correct:

- **Forward** — a document's own assignments table prints its target groups' *names*, and a compliance
  policy's noncompliance-notification block prints its notification template's *name*. The resource stores
  only GUIDs, so renaming a group or a template in the tenant leaves every referencing policy printing a
  stale name while no policy's YAML moved. `assignmentsSha256` catches the first, `notificationsSha256` the
  second.
- **Reverse** — a group document's `Targeted by` block lists *other* resources' assignments. Re-point a
  policy from one group to another and both groups' reverse indexes are wrong while neither group's YAML
  moved.
- **Used-by** — a notification message template document's `Used by` block lists the *other* resources
  (compliance policies) that reference the template in a noncompliance action. Point a policy's noncompliance
  action at a different template, or rename a referencing policy, and both templates' used-by indexes are
  wrong while neither template's YAML moved. This is the reverse case one type over: groups are targeted by
  assignments, templates are used by noncompliance actions.

None of these cases can appear in the work list, because nothing about those documents' own resources
changed. That is the whole reason 5d exists as a separate step with its own list.

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

### D. Why the summary is tool-fed and written last (7)

The summary is tenant-wide, but this prompt is incremental: the work list is only the documents that changed,
so counting it would describe the delta, not the tenant, and a no-op run would summarise nothing. The
`summary-facts` block is computed from `metadata.yaml`, which is complete every run, so the summary stays
correct regardless of how little changed. It is written last because it names the state the documents
describe, and it is regenerated every run and never hash-tracked — it has no source of its own to compare
against, so there is nothing to make it incremental.

### E. Why the management summary is fed, not observed (7)

The summary is regenerated on every run, including a run whose work list is empty and which therefore read
no resource at all. If its findings came from the orchestrator's own reading, a no-op run would produce a
thinner page than a full one, and the tenant's landing page would change meaning for reasons that have
nothing to do with the tenant. Feeding it from the fact block and a closed signal list makes the page a
function of the export, not of how much work the run happened to do. The length is fixed in the section for
the same reason: a landing page that grows every run stops being one.

The five signals are the ones that are both tenant-wide and mechanically decidable. Everything an operator
would also want — whether a baseline deviation was deliberate, whether a policy's conditions can ever
match — needs the judgement that went into the individual document, and belongs there.
