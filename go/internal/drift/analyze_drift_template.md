# Drift impact analysis prompt (template)

<!--
This file is a TEMPLATE, not a prompt to paste as-is.

`azure-rd docs analyze-drift` reads it, replaces each marked block below with values computed from the
latest drift observation (drift/metadata.yaml) and the export baseline (resources/metadata.yaml), and
writes the finished prompt to drift/analyze.md. Paste the OUTPUT into a fresh agent session.

Override with `--prompt <file>` to use a different template.

Marked blocks the tool replaces (start/end markers stay, content between them is regenerated):

  observation  what was observed and when, against which baseline, with the verdict counts and caveats
  worklist     the drifted resources, grouped by type, each with its files and recorded field deltas
  refmap       GUID -> name/document for assignment target groups, filters and notification templates

Everything outside the markers is prose you can edit freely. Keep the markers matched and never nested.

Editing convention: the numbered sections are the procedure and stay short. Background — why a step exists,
what failure it prevents — goes in the appendix at the bottom. Put explanation there, never instructions.
-->

You are analyzing **configuration drift** in an Azure/Entra/Intune tenant: the differences between an
export baseline produced by **azure-resource-downloader** and the tenant's current state as observed by
`azure-rd resource drift`. Your output is a single **impact report** — what changed, who is affected, and
what it means for security, compliance and lifecycle — for a human operator deciding what to do next.

The change detection has already been done. The findings below are complete and closed: analyze exactly
what they name — nothing more, nothing less. Do not walk the export looking for other differences, do not
hash or diff anything to decide *whether* something changed, and do not re-litigate the verdicts.

## Observation under analysis

<!-- observation:start -->
- Tenant folder: `output/<tenant>/`
- Baseline (read-only): `output/<tenant>/resources/`
- Observed payloads (read-only): `output/<tenant>/drift/`
- Your report (the only file you write): `output/<tenant>/drift/report.md`
- Observed at: `<timestamp>`
- Baseline generated at: `<timestamp>`
- Drift run complete: `<true|false — reason>`
- Verdicts: changed N · added N · removed N · renamed N (unchanged N of M compared)
- Types whose drift is unknown (listing failed): none
- Removals suppressed (incomplete run could not assert absence): false
- Entries not comparable (config attestation): none
<!-- observation:end -->

Every path below is relative to the tenant folder. Three trees matter, and they mirror each other exactly:

```
resources/<APIType>/<endpoint>/<name>.yaml   the baseline (old bytes)
drift/<APIType>/<endpoint>/<name>.yaml       the observed current state (new bytes)
docs/<APIType>/<endpoint>/<name>.md          the document describing the pre-change state
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
- **Everything is read-only except your report.** You write exactly one file: `drift/report.md`. Never
  modify anything under `resources/` or `docs/`, and never touch `drift/metadata.yaml` or the payloads.
- **Do not update documentation.** The documents under `docs/` describe the baseline and must keep doing
  so; they are refreshed by a separate documentation pass after the operator re-baselines (appendix A).

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
- Field deltas (values truncated; open both files when context is needed):
  - `scheduledActionsForRule[0].scheduledActionConfigurations[0].gracePeriodHours`: `24` → `0`

_…one section per resource type, then a tally: N finding(s) across M type(s)._
<!-- worklist:end -->

---

## 2. Analyze each finding

Work through every finding, one at a time:

- **2a. Read the type's lens.** Before the first finding of a type, read that type's `doc-prompt.md` in
  full — it states what the type is for, its key settings and their security relevance. Where the heading
  above marks the spec missing, analyze from the YAML alone and flag the reduced confidence in the report.
- **2b. Establish what changed — facts only.** Changed/renamed: start from the recorded deltas; open the
  baseline and payload files when a delta needs surrounding context. Added: read the payload in full.
  Removed: read the baseline file, and its document for what the resource was doing.
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
  cosmetic, a rename, a description edit, or a change that is clearly benign in context.

---

## 3. Reference map

Resolve GUIDs through this map only; a GUID that resolves nowhere is dangling and worth naming as such.

<!-- refmap:start -->
_Replaced by the tool: assignment target groups, assignment filters and notification message templates as
`GUID → name/document` lists, with dangling references flagged._
<!-- refmap:end -->

---

## 4. Write the report

Write `drift/report.md` — and nothing else — with this frontmatter, copied from the observation block:

```markdown
---
observedAt: <observed at>
baselineGeneratedAt: <baseline generated at>
findings: <count>
severities: high <n> · medium <n> · low <n>
---
```

Then four parts, in order:

1. **Executive summary** — 3–6 sentences: the overall shape of the drift, the highest-severity items, and
   whether anything demands action before the next re-baseline.
2. **Findings, ordered by severity** (high first, then by resource name). Per finding, a heading
   `<severity> — <verdict>: <name>` and four short paragraphs or bullet groups: *what changed* (facts,
   citing the delta paths), *who is affected* (resolved names, never bare GUIDs), *why it matters* (the 2d
   dimensions that apply), and *suggested follow-up* — accept and re-baseline, investigate with the owner,
   or revert in the tenant.
3. **Not analyzed** — the observation block's caveats, restated: types whose drift is unknown, entries not
   comparable, suppressed removals, and findings whose type spec was missing. Absence from this report is
   not evidence of no drift.
4. **Ephemerality note** (the last line): this report describes the observation named in its frontmatter;
   the next `resource drift` or `resource download` run deletes it. Archive a copy if it must be kept.

---

## Appendix A — background (explanation, never instructions)

- **Why the report is ephemeral.** The `drift/` tree holds exactly one observation — the latest — and is
  cleared by the next drift run, and by a re-baselining download (a new baseline supersedes the
  observation by definition). The report lives with the observation it analyzes, so it can never describe
  a baseline that no longer exists. Deliberately, there is no drift history.
- **Why you never touch `docs/`.** The documents mirror the baseline and are regenerated incrementally by
  `azure-rd docs generate-prompt` after the operator re-baselines with `resource download`. A report that
  edited documents would make them disagree with the baseline their frontmatter hashes attest.
- **Severity is judgment; the rest is facts.** The verdicts, deltas and hashes were computed by the tool;
  your contribution is the impact assessment and the severity. The report is advisory prose for a human —
  nothing downstream parses it, so favour clarity over structure.
- **Renames.** A renamed resource is a content change whose payload landed at a new path; the finding
  carries both names and both files. Judge the rename itself (is the old name referenced anywhere? does
  naming encode policy?) as well as any other deltas it carries.
