# Drift impact analysis prompt (template)

<!--
This file is a TEMPLATE, not a prompt to paste as-is.

`azure-rd docs analyze-drift` reads it, replaces each marked block below with values computed from the
latest drift observation (drift/metadata.yaml) and the export baseline (resources/metadata.yaml), and
writes the finished prompt to drift/analyze.md. Paste the OUTPUT into a fresh agent session.

Override with `--prompt <file>` to use a different template.

Marked blocks the tool replaces (start/end markers stay, content between them is regenerated):

  observation  what was observed and when, against which baseline, with the verdict counts, caveats and
               the attribution status (drift/audit.yaml)
  worklist     the in-scope drifted resources, grouped by type, each with its files and recorded field deltas
  inventory    out-of-scope added/removed resources (unreferenced groups, autopilot identities), index-only
  refmap       GUID -> name/document for assignment target groups, filters and notification templates

Everything outside the markers is prose you can edit freely. Keep the markers matched and never nested.

Editing convention: the numbered sections are the procedure and stay short. Background — why a step exists,
what failure it prevents — goes in the appendix at the bottom. Put explanation there, never instructions.
-->

You are analyzing **configuration drift** in an Azure/Entra/Intune tenant: the differences between an
export baseline produced by **azure-resource-downloader** and the tenant's current state as observed by
`azure-rd resource drift`. Your output is one **drift document per in-scope finding** — what changed in
that resource, who is affected, and what it means for security, compliance and lifecycle — plus one summary
**index**, for a human operator deciding what to do next.

The change detection has already been done. The findings below are complete and closed: analyze exactly
what they name — nothing more, nothing less. Do not walk the export looking for other differences, do not
hash or diff anything to decide *whether* something changed, and do not re-litigate the verdicts.

## Observation under analysis

<!-- observation:start -->
- Tenant folder: `output/<tenant>/`
- Baseline (read-only): `output/<tenant>/resources/`
- Observed payloads (read-only): `output/<tenant>/drift/`
- Your drift documents (one per finding; each worklist entry names its destination): `output/<tenant>/drift/<APIType>/<endpoint>/<name>.md`
- Your index (the summary, written last): `output/<tenant>/drift/index.md`
- Observed at: `<timestamp>`
- Baseline generated at: `<timestamp>`
- Drift run complete: `<true|false — reason>`
- Verdicts: changed N · added N · removed N · renamed N (unchanged N of M compared)
- Types whose drift is unknown (listing failed): none
- Removals suppressed (incomplete run could not assert absence): false
- Entries not comparable (config attestation): none
- Attribution: workspace `<id>`, queried at `<timestamp>`; tables: `AuditLogs` ok · `IntuneAuditLogs` ok — or none / outdated
<!-- observation:end -->

Every path below is relative to the tenant folder. Three trees matter, and they mirror each other exactly —
your drift document joins them at the same path, so per resource all four files carry the same key:

```
resources/<APIType>/<endpoint>/<name>.yaml   the baseline (old bytes)
drift/<APIType>/<endpoint>/<name>.yaml       the observed current state (new bytes)
docs/<APIType>/<endpoint>/<name>.md          the document describing the pre-change state
drift/<APIType>/<endpoint>/<name>.md         YOUR drift document for this resource (you write this)
```

---

## 0. Ground rules (apply to every step)

- **Never invent.** Every claim about a change must be traceable to a finding's recorded deltas or to the
  baseline and payload YAML. If something is absent, say it is absent — do not fill it in from general
  knowledge of the product.
- **Masked values are not misconfigurations.** Secrets exported as `valueState: encryptedValueToken`,
  redacted certificates, opaque tokens, `*****` and so on are expected service behaviour. A delta between
  two masked values usually means the service re-issued the token, not that the underlying secret changed —
  say so, never guess plaintext, and never rate a masked value itself as a finding.
- **The resource's own `description` field is authoritative context.** A change consistent with a
  documented, deliberate baseline deviation is intentional — describe it as such, not as a defect.
- **Everything is read-only except your drift documents and index.** You write exactly the per-finding
  drift documents named by the worklist plus `drift/index.md` — nothing else. Never modify anything under
  `resources/` or `docs/`, and never touch `drift/metadata.yaml`, `drift/analyze.md`, `drift/audit.yaml`
  or the payloads.
- **Do not update documentation.** The documents under `docs/` describe the baseline and must keep doing
  so; they are refreshed by a separate documentation pass after the operator re-baselines (appendix A).
- **Inventory rows are not findings.** The resources under "Out of scope — inventory only" (section 1) are
  never analyzed: copy their rows into the index's findings table verbatim — severity `info`, one-liner
  "inventory change — not analyzed", no document link — and write no drift document for them.

---

## 1. What drifted

The list is authoritative. Each finding names the files to read and the recorded field deltas.

<!-- worklist:start -->
_Replaced by the tool. Shape:_

### Microsoft.Graph/deviceCompliancePolicies — spec: `resources/Microsoft.Graph/deviceCompliancePolicies/doc-prompt.md`

#### changed: GBL_C_PRD_D_WIN_OS_Validation

- Resource id: `…`
- Baseline (old bytes): `resources/…/gbl_c_prd_d_win_os_validation.yaml`
- Current (new bytes): `drift/…/gbl_c_prd_d_win_os_validation.yaml`
- Document (describes the pre-change state): `docs/…/gbl_c_prd_d_win_os_validation.md`
- Your drift document (write it): `drift/…/gbl_c_prd_d_win_os_validation.md`
- Field deltas (values truncated; open both files when context is needed):
  - `scheduledActionsForRule[0].scheduledActionConfigurations[0].gracePeriodHours`: `24` → `0`

_…one section per resource type, then a tally: N finding(s) across M type(s)._
<!-- worklist:end -->

### Out of scope — inventory only

Resources of types outside the documentation scope — groups no assignment references and Autopilot device
identities — are not analyzed (appendix A). Their added/removed findings are listed here for the index
only; their changed/renamed findings are only counted, in the observation block's caveats above.

<!-- inventory:start -->
_Replaced by the tool: `` `info` · <verdict>: <name> (<type>) `` rows, or "None."_
<!-- inventory:end -->

---

## 2. Analyze each finding

Work through every finding, one at a time:

- **2a. Read the type's lens.** Before the first finding of a type, read that type's `doc-prompt.md` in
  full — it states what the type is for, its key settings and their security relevance. Where the heading
  above marks the spec missing, analyze from the YAML alone and flag the reduced confidence in the report.
- **2b. Establish what changed — facts only.** Changed/renamed: start from the recorded deltas; open the
  baseline and payload files when a delta needs surrounding context. Added: read the payload in full.
  Removed: read the baseline file, and its document for what the resource was doing.
- **2b'. Weigh who made the change.** Where the finding lists `Changed by:` lines (read from the tenant's
  audit logs, newest first), name the actor in the report. An actor is a fact about *who*, never proof of
  intent: do not call a change authorized or unauthorized because of who made it. Several events are all
  listed; judge the latest against the observed bytes. `Attribution unavailable` is not evidence that
  nobody changed the resource — say that the actor is unknown and why.
- **2c. Establish who is affected.** Deltas under assignment paths change *who receives* the
  configuration; resolve every group and filter GUID through the reference map (section 3). For added and
  removed resources the whole assignment scope appears or disappears with them.
- **2d. Judge the impact**, where applicable per finding: **security posture** (weakened or hardened
  controls, widened or narrowed scope, disabled protections), **compliance** (rules that stop or start
  being enforced, grace periods, noncompliance actions), **lifecycle** (expiring or renewed credentials
  and certificates, version pins, deprecated settings), and **user/device impact** (who notices, what
  breaks or unlocks).
- **2e. Assign a severity**: `high` — security weakened, protections disabled, or enforcement scope
  changed broadly; `medium` — behaviour or scope changed in a way that needs a human decision; `low` —
  cosmetic, a rename, a description edit, or a change that is clearly benign in context. `info` is
  reserved for the tool-fed inventory rows — never assign it to a finding you analyzed.

---

## 3. Reference map

Resolve GUIDs through this map only; a GUID that resolves nowhere is dangling and worth naming as such.

<!-- refmap:start -->
_Replaced by the tool: assignment target groups, assignment filters and notification message templates as
`GUID → name/document` lists, with dangling references flagged._
<!-- refmap:end -->

---

## 4. Write one drift document per finding

As you finish each finding's analysis (section 2), write its drift document at the destination its
worklist entry names — the finding's own path under `drift/`, `.md` beside the payload's `.yaml` — with
this frontmatter:

```markdown
---
observedAt: <observed at>
baselineGeneratedAt: <baseline generated at>
verdict: <added|changed|renamed|removed>
severity: <high|medium|low>
---
```

Then four short parts, per document: *what changed* (facts, citing the delta paths), *who is affected*
(resolved names, never bare GUIDs), *why it matters* (the 2d dimensions that apply), and *suggested
follow-up* — accept and re-baseline, investigate with the owner, or revert in the tenant. One resource per
document — a frontend renders it beside the resource's YAML diff, so never discuss another resource except
by linking its drift document.

---

## 5. Write the index

When every drift document exists, write `drift/index.md` — the summary — with this frontmatter:

```markdown
---
observedAt: <observed at>
baselineGeneratedAt: <baseline generated at>
findings: <count>
severities: high <n> · medium <n> · low <n> · info <n>
---
```

Then four parts, in order:

1. **Management summary** — open with one bold sentence, the bottom line: does anything demand action
   before the next re-baseline, and if so, what. Then one H3 section per theme, most severe first,
   at most five. A theme is a group of findings that share a cause or a purpose (one admin change
   across several policies, a rollout, a service-side schema change), not a resource type. Name the
   heading after the theme itself (e.g. "Trusted-location exception in Conditional Access"), not a
   generic category. Each section is 2–4 sentences of prose, no lists or tables: what changed and
   when, the actor if attributed, the highest severity it carries, and the recommended action.
   Link the findings' drift documents inline where you name them. Put every remaining low-severity
   finding in a final section, "Everything else", of one or two sentences. Keep evidence and
   tables for part 3 — this part says *what to do*, part 3 shows *why*.
2. **Findings, ordered by severity** (high first, then by resource name), each linking to its drift
   document by relative path (`<APIType>/<endpoint>/<name>.md`) with a one-line judgment. A table
   (severity · verdict · resource · one-liner) fits well; prose is fine too. Append the inventory rows
   (section 1) verbatim — severity `info`, one-liner "inventory change — not analyzed", no link.
3. **Security, compliance and lifecycle issues** — the cross-resource view: themes that only appear when
   the findings are read together (the same protection weakened in several places, scope shifts that
   compound, expiring credentials clustering). Prose or tables, whatever fits; skip axes with nothing to
   say rather than padding them.
4. **Not analyzed** — the observation block's caveats, restated: types whose drift is unknown, entries not
   comparable, suppressed removals, the out-of-scope changed/renamed count, and findings whose type spec
   was missing. Absence from this report is not evidence of no drift. End with the **ephemerality note**
   (the last line): these documents describe the observation named in their frontmatter; the next
   `resource drift` or `resource download` run deletes them. Archive copies if they must be kept.

---

## Appendix A — background (explanation, never instructions)

- **Why one document per resource.** The drift document sits at the payload's path with the extension
  swapped, so per resource the baseline YAML, the observed YAML, the documentation and your judgment all
  share one key — a frontend can show the YAML diff and your document side by side without any lookup
  table. That only works if each document confines itself to its own resource; the index carries
  everything cross-cutting.
- **Why the documents are ephemeral.** The `drift/` tree holds exactly one observation — the latest — and
  is cleared by the next drift run, and by a re-baselining download (a new baseline supersedes the
  observation by definition). Your documents live with the observation they analyze, so they can never
  describe a baseline that no longer exists. Deliberately, there is no drift history.
- **Why you never touch `docs/`.** The documents mirror the baseline and are regenerated incrementally by
  `azure-rd docs generate-prompt` after the operator re-baselines with `resource download`. A drift pass
  that edited them would make them disagree with the baseline their frontmatter hashes attest.
- **Severity is judgment; the rest is facts.** The verdicts, deltas and hashes were computed by the tool;
  your contribution is the impact assessment and the severity. The documents and index are advisory prose
  for a human — nothing downstream parses them, so favour clarity over structure.
- **Renames.** A renamed resource is a content change whose payload landed at a new path; the finding
  carries both names and both files. Judge the rename itself (is the old name referenced anywhere? does
  naming encode policy?) as well as any other deltas it carries.
- **Why some findings are inventory only.** The documentation deliberately skips bulk directory records
  (Autopilot device identities) and groups nothing assigns — the same scope applies here, with one
  refinement: a group a drifted payload newly assigns counts as referenced. Arrivals and departures of
  out-of-scope resources still matter as inventory, so the index lists them; but with no documentation
  lens and no assignment surface of their own, analyzing them would be guesswork.
