# CLAUDE.md — Azure/Entra/Intune export workspace

This folder is the export tree of **azure-resource-downloader** (`azure-rd`). One subfolder per tenant,
named after the tenant's Entra default domain:

```
<tenant>/
├─ resources/   downloaded configuration, one YAML per resource + metadata.yaml + per-type doc-prompt.md
├─ docs/        the documentation (Markdown), plus generate.md, index.yaml, summary.md, report-*.md
├─ drift/       the latest drift observation: metadata.yaml, observed payloads, analyze.md
└─ chunks/      scratch working files of a documentation run
```

You do two jobs in this workspace, and both are driven by a **prompt file the tool generated**:

| Job | Prompt file | Asked for with |
|---|---|---|
| Write/refresh the documentation | `<tenant>/docs/generate.md` | "document `<tenant>`" |
| Judge the impact of the latest drift | `<tenant>/drift/analyze.md` | "analyze drift for `<tenant>`" |

Either phrase starts **phase 1 only** (preflight and plan) — see the next section. Neither ever starts a run.

**One tenant per session.** Never read or write another tenant's folder in the same run.

## Every job runs in two phases, and you stop in between

A job is asked for in two steps, never one. **Phase 1 writes nothing.** Phase 2 starts only after the
operator confirms.

### Phase 1 — preflight and plan (always; writes nothing)

1. Read the whole prompt file — it is long and it is the procedure.
2. Run the preflight checks below. If one fails, report it, name the CLI command that fixes it, and stop.
3. Present the plan (shape below) and **stop. Ask for confirmation and wait.**

In this phase you may only read files and inspect metadata (`ls -l`, `wc -c`, `grep -c`). Create no
document, no `chunks/` file, no script output, no directory. If the operator's request sounds like "just do
it", it still means phase 1 — confirmation is a separate turn, always.

### Phase 2 — execute (only on explicit confirmation)

4. **Follow the prompt exactly, end to end, in one pass.** Its numbered sections are instructions; its
   appendix is background. It already decided the run parameters (single full run, no checkpointing, no
   approval between batches) — from here on, do not pause again and do not ask which model to use. The
   confirmation gate is the *only* interruption, and it happens before the run starts, so the prompt's
   no-checkpointing rule is untouched.
5. **The prompt file outranks this file** on anything about the job itself: what to write, where, in what
   shape. This file only covers what the prompt cannot know — where the workspace root is, what is off
   limits, who runs the CLI, and the gate above.
6. Do not summarise, condense or "improve" the procedure. Skipping a verification section is a failed run,
   not a shortcut.

Confirmation covers **one job on one tenant**. A second job, a re-run, or the same job on another tenant
needs its own phase 1.

### What the plan must contain

Keep it to a screen — the operator is deciding whether to spend a long run, not reading the prompt back.

- The job, the tenant, and the prompt file's path.
- The preflight verdict: every check, passed or failed, with the values compared.
- The export/observation facts that bound the run: the timestamps, whether the export or drift run is marked
  **incomplete** and why, types that could not be listed, entries not comparable, suppressed removals.
- The work: counts from the work list, grouped by resource type — documents to generate, blocks to
  re-splice, documents to migrate; or findings by verdict (changed/added/removed/renamed).
- For a documentation run, the intended chunking: how many chunks, the types they cover, and which large
  types get their own chunk.
- Everything you will write, by path pattern, and confirmation that nothing else will be touched.
- Anything the prompt asks you to flag that is already visible: missing type specs, dangling references,
  an empty work list.

If the preflight passes but there is genuinely nothing to do, say that in the plan instead of asking for
confirmation — and read the empty-work-list note below before concluding it.

### Two traps in the prompt files

- **An older prompt file may still open with "This file is a TEMPLATE, not a prompt to paste as-is."** Current
  CLI versions strip that header; if you still see it, it is left over from the template the tool splices. The
  file you are reading *is* the finished prompt: its marked blocks (`observation`, `worklist`, `refmap`,
  `export`, …) are already filled in with real values. Ignore that header and the `--prompt` note; everything
  below it applies to you.
- **Path prefixes in the first block are wrong for this workspace.** Lines such as
  `Tenant folder: ../output/<tenant>` record where the CLI ran, not where you are. In this workspace the
  tenant folder is `<tenant>/` at the root. Strip any `output/` or `../output/` prefix, then treat **every
  other path in the prompt as relative to the tenant folder** — `resources/…`, `docs/…`, `drift/…`,
  `chunks/…`. Work from the tenant folder and never improvise a path: the trees mirror each other, and the
  prompt states the mapping.

## Preflight checks (phase 1)

A prompt file describes the export as it was when the CLI ran. If the export moved on, the prompt is stale
and running it would document or judge something that no longer exists. Verify, then stop and ask the
operator to re-run the CLI if a check fails — never patch the prompt yourself, and never proceed to phase 2
on a failed check even if the operator confirms.

- **The prompt file exists and is the current one.** No `docs/generate.md` → operator runs
  `docs generate-prompt`. No `drift/analyze.md` (or no `drift/metadata.yaml` at all) → operator runs
  `resource drift`, then `docs analyze-drift`.
- **`docs/generate.md`**: its `Export generated at` must equal `generatedAt` in
  `<tenant>/resources/metadata.yaml`. Mismatch → operator re-runs `docs generate-prompt`.
- **`drift/analyze.md`**: its `Observed at` must equal `observedAt` in `<tenant>/drift/metadata.yaml`, and
  its `Baseline generated at` must equal `generatedAt` in `<tenant>/resources/metadata.yaml`. Mismatch →
  the observation was superseded by a newer download; operator re-runs `resource drift`, then
  `docs analyze-drift`.
- **You can run scripts.** The prompts' verification and splice sections are Python scripts. Check that
  `python3 --version` works in this workspace; if it does not, say so in the plan and stop — the job cannot be
  run by hand.

**An empty work list is not a no-op run.** `docs/generate.md` can list nothing to generate while still
requiring the re-splice, the tenant summary and the report. Run every section and let the prompt decide
what is empty.

**Documents you do not rewrite stay as they are.** The prompt requires a one-line, double-quoted `summary:`
in the frontmatter of every document you write, and its checks enforce it for those documents only. A
document kept from an earlier run may have no `summary` — that is expected; it gets one when its resource or
spec changes. Never add or edit frontmatter in a document the work list does not name.

## What you may write

| Allowed to write | Never touch |
|---|---|
| `<tenant>/docs/<APIType>/<endpoint>/<name>.md` — the documents | Anything under `<tenant>/resources/` — the source of truth, including `metadata.yaml` and every `doc-prompt.md` |
| `<tenant>/docs/summary.md` and `<tenant>/docs/report-<UTC>.md` | `<tenant>/docs/generate.md` and `<tenant>/docs/index.yaml` — written by `azure-rd` |
| `<tenant>/chunks/**` — scratch for a documentation run | `<tenant>/drift/metadata.yaml`, `<tenant>/drift/analyze.md` and every `drift/**/*.yaml` payload |
| `<tenant>/drift/<APIType>/<endpoint>/<name>.md` — one drift document per finding | `<tenant>/docs/**` during a **drift** run — the documents describe the baseline and must keep doing so |
| `<tenant>/drift/index.md` — the drift summary | Any other tenant's folder; anything outside this workspace |

Never delete anything. Never rename a tenant folder. `.DS_Store` files are noise — ignore them.

## Script the mechanical steps

The prompt's verification sections and its assignment splice are deterministic transformations over many
files, and it says so: **write a script, run it, read its output.** Do not open documents one at a time and
retype generated tables through your own output — that paraphrases names, breaks links, and is the most
expensive possible way to run this. If you cannot execute a shell or Python script in this workspace, stop
and say so rather than hand-editing the tree.

Likewise: **never compute a hash.** Every hash you write is copied verbatim from the row in the prompt (or
the chunk file) that names the document.

## Subagents (documentation run only)

`docs/generate.md` section 3 fans the work list out to per-chunk subagents and fixes the rules: one type per
chunk, chunk by expected *output* size, shared rules written once to `chunks/_common.md`, roughly ten agents
in flight refilling as each finishes, and each agent's prompt is only the one line the section gives. You
stay the orchestrator: you chunk, you splice, you verify. Keep chunk contents out of your own context.

The drift run has no fan-out — work the findings yourself, in order.

## You never call Azure

Nothing in this workspace requires a sign-in, and you have no credentials. Do not run `az`, `azure-rd` or
any network fetch, and never try to "check the tenant" — the export and the observation are closed inputs.
When a prompt file is missing or stale, ask the operator to run the CLI from the `go/` folder of the
`azure-resource-downloader` checkout:

```bash
./azure-rd resource download     --output ../output --domain <tenant>    # re-baseline (clears drift/)
./azure-rd docs generate-prompt  --output ../output --domain <tenant>    # refresh docs/generate.md
./azure-rd docs generate-index   --output ../output --domain <tenant>    # refresh docs/index.yaml
./azure-rd resource drift        --output ../output --domain <tenant>    # new observation
./azure-rd docs analyze-drift    --output ../output --domain <tenant>    # refresh drift/analyze.md
```

Always name the tenant you are working on in `<tenant>`. When the operator keeps per-tenant profiles in a
config directory, the `download` and `drift` lines also need `--config-dir <dir>`.

## When a job finishes

- **Documentation run** — after the report is written, tell the operator to run `docs generate-index`: the
  navigation index the docs browser reads is built from your documents' frontmatter and is not refreshed by
  you.
- **Drift run** — after `drift/index.md` is written, remind the operator that the `drift/` tree is
  **ephemeral**: the next `resource drift` and any re-baselining `resource download` delete it, your
  documents included. If the analysis must be kept, the operator copies the tree to
  `_archive/<tenant>-<observed-at>/` first — folders starting with `_` are ignored by the docs browser and by
  tenant discovery. You suggest it; you never create or write `_archive/` yourself.
- Report what you did, what you repaired, and every caveat the prompt asked you to carry forward (dangling
  references, an incomplete export or drift run, types that could not be listed, missing type specs).
  Absence of a finding is not a clean bill of health, and everything here is derived from the export alone —
  recommend verifying anything security-relevant against the live tenant.
