# TODO — backlog review

Snapshot taken 2026-09-28 from `go/NEXT-ITERATIONS.md` and `web/NEXT-ITERATIONS.md` (previous snapshot
2026-09-08). Those two files stay authoritative; this is a cross-project reading of them. Both hold parked
ideas only again: the two Go entries scheduled since the last snapshot — the drift-impact analysis prompt
(`docs analyze-drift`, per-finding drift documents, `drift/index.md`) and its scoping/header-stripping
follow-up — shipped in full, are recorded under `[Unreleased]` in `go/CHANGELOG.md`, and were cleared from the
backlog today. The web side shipped the matching **Drift view** in the same period (`web/CHANGELOG.md`
`[Unreleased]`).

## 1. Backlog overview

### Go CLI (`go/NEXT-ITERATIONS.md`)

Scheduled work: **none**. Parked ideas only (three new since the last snapshot, marked *new*):

- **Routine dependency updates, and consolidating on one Microsoft Graph SDK** *(new)* — a `go get -u` pass,
  and moving the seven v1.0-only handlers to the beta SDK so `msgraph-sdk-go` can go. Parked: a bump is not
  hash-neutral by construction (SDK model changes move `sourceSha256` for unchanged resources), the switch
  trades v1.0's stability contract for uniformity, and it moves hashes once. Its first revisit condition — "when
  the drift command has shipped, as the verification tool" — **is now met**; the second (a forced SDK bump or
  build time becoming a felt cost) is not. The idea's prose still speaks of the drift command in the future
  tense; see Housekeeping.
- **Per-finding severity in `Security` sections** — tag each callout `[risk]` / `[review]` / `[ok]` so the web
  can colour or filter. Parked: subjective model-only judgement, made 400+ times, unvalidatable.
  **Regeneration-gated** (touches all seven templates). Note the contrast the shipped drift analysis drew
  explicitly: severity *is* assigned per finding there, because a drift report is a handful of judgments in
  disposable artifacts — the blast-radius argument, not the principle, is what parks this one.
- **Resolve Graph ids to names inside the YAML** — `groupId: <guid> (name)` at export time. Parked:
  `docs generate-prompt` already resolves incrementally via splice blocks; embedding a name turns a fact into a
  decision and moves `sourceSha256` on every group rename. Revisit only for a non-doc consumer of `resources/`.
- **Bootstrap taxonomy from per-document LLM suggestions** — harvest programme labels into
  `docs/taxonomy-suggestions.yaml`. Parked: taxonomy is small today. **Regeneration-gated** (suggestion
  instruction lives in per-type templates).
- **Review the sign-in surface** *(new)* — can `az login --scope https://graph.microsoft.com/.default` replace
  the dedicated-app device-code path (`--client-id`/`--tenant-id`, the `PermissionScoped` probe, the
  interactive prompt, the app-registration setup)? Parked: the answer lives in tenant consent policy and
  Microsoft's first-party-app rules, not in this repository; one tenant's result does not generalise; removal
  is a breaking change to the auth surface. If promoted, soften first (recommend the exact scoped re-login,
  keep device code as fallback), remove only on a major.
- **Clear the `gocognit` baseline** *(new)* — split the 26 functions named in `.golangci.yml`'s exclusions
  one at a time, deleting each entry as it goes. Parked: the baseline already forces every new function to
  comply, several listed functions carry invariants a refactor must not disturb (prune guards, completeness
  accounting), and Go's error handling inflates the metric. Shrinks opportunistically, when a listed function
  is touched for another reason.

### Web docs browser (`web/NEXT-ITERATIONS.md`)

Scheduled work: **none**; the *Fixes* section reads *None outstanding.*

Standing decision: whole-tenant exports live on the picker card as plain `<a download>`; partial exports live
next to the thing exported; Confluence REST sync needs POST and is therefore not offered at all.

Parked ideas, grouped:

- **Presentation**: per-document identity on `<article>`; summary TOC; syntax highlighting inside documents;
  explicit dark-mode toggle.
- **Navigation**: name filter + per-item context in sidebar; sidebar spine by taxonomy axis; clickable
  breadcrumb segments; resource landing page; two-group nav tree; browsable excluded bulk types;
  multi-segment tenants.
- **Findings**: actionable findings block (severity filter via `:target`).
- **Search / diff**: tenant-wide search; tenant diff.
- **Export**: further formats + partial exports; media / YAML as attachments; Confluence REST sync; single
  export button → `GET /:tenant/_export` options page.
- **Infra**: watch-based cache invalidation.
- **Rule change**: drop the no-client-side-JavaScript rule (tiers: keep / progressive enhancement / full lift).
- **Debt** *(new)*: clear the ESLint suppressions baseline (16 findings in 9 files; a debt ledger, not a
  policy — the mirror of Go's `gocognit` baseline idea, and shrinking by the same opportunistic rule).

## 2. Inter-project dependencies

Reassessed 2026-09-28 against the code, not the backlog prose: each claim below was re-checked in
`go/internal/docs/` (`generateindex.go`, `generate_prompt_template.md`), `go/internal/drift/`
(`analyze_drift_template.md`) and `web/src/docs/` (`tenant-index.ts`, `findings-table.ts`, `drift-view.ts`).
Intra-project couplings that the authoritative backlogs already state in full (the no-JS rule's blast radius,
the export chain, the CSP/`script-src` coupling) are **not** repeated here — they drift when copied.

### New since the last snapshot: the drift tree is a second Go → web contract

Go now writes a third tree, `<tenant>/drift/`, and the web renders it: `drift/metadata.yaml` (verdicts,
dotted-path deltas, baseline and payload hashes), payloads at `drift/<key>.yaml`, the analysis prompt
`drift/analyze.md`, and — written by the analysis agent, never by `azure-rd` — one drift document per
in-scope finding at `drift/<key>.md` plus `drift/index.md`. The web joins all of it by the shared
`<type>/<name>` key, gates validity on the observation's `baseline.generatedAt` against
`resources/metadata.yaml`, and shows a payload only after its hash matches. This is exactly the shape the
`index.yaml` contract has — Go emits facts and a self-describing header, the web reads and never derives —
and it carries the same obligations: a change to the observation schema, to the per-finding frontmatter
(`verdict`, `severity`, `observedAt`, `baselineGeneratedAt`) or to the index's `severities:` line is a
cross-project change, and neither backlog currently says so. **Recommendation:** when either side next edits
its rules, name `drift/` alongside `index.yaml` as a versioned Go → web contract; the observation file carries
a `toolVersion` (which binary wrote it) but no schema `version:` today — the field the index learned to need.

One concrete mismatch found on this pass: **two severity vocabularies.** The tenant summary's Findings table
uses `critical / high / medium`; the drift analysis uses `high / medium / low / info` (`info` reserved for
tool-fed inventory rows). The web's table tagging (`findings-table.ts`, `SEVERITIES`) knows only the summary's
set, so in a drift index table `high`/`medium` rows get the icon treatment while `low`/`info` rows fall back to
the plain word — graceful by design (an unknown value must never become a wrong icon), but visibly uneven in
the one table where all four appear. The drift *page* header (`drift-view.ts`) already maps all four. **Web-only
fix**, small: give drift tables their own closed set, or unify the two into one. Unifying on the Go side would
touch the summary template (regeneration-gated for `docs/summary.md` only — a single file, cheap) and the drift
template (not gated); the web fix needs neither.

### The one hard dependency, and it is tracked by neither backlog

**Go's prompt template never asks for `summary:`** — re-verified unchanged on 2026-09-28. The plumbing on both
sides is complete: `docFrontmatter` has a `Summary` field, `GenerateIndex` copies it into each `index.yaml`
resource (`IndexResource.Summary`, `omitempty`), and the web renders it as per-item context when present. But
the template's *Frontmatter (required)* section still lists `source`, `sourceSha256`, `promptSha256`,
`platformGroup`, `functionGroup`, `generatedAt` — and nothing else. The web backlog's "0 of 263 and 0 of 148
resources carry it" is therefore **by construction**, not model behaviour, and the web idea *name filter and
per-item context* is blocked on a Go change that `go/NEXT-ITERATIONS.md` still does not list — three weeks and
two shipped Go entries later. **Recommendation (unchanged, now overdue):** add it to Go's backlog as a
regeneration-gated item (one frontmatter line in the template plus its rule text), explicitly batched with the
two existing regeneration-gated ideas — it must not ship alone, because any template edit moves `promptSha256`
for every type. Note that the frontmatter line itself rides `generate_prompt_template.md`, which is **not**
hashed; what makes it regeneration-gated is that no existing document carries the field, so filling it means
regenerating every document anyway — the hash argument and the coverage argument land in the same place.

A second finding on the same evidence: the template already **requires** `platformGroup` / `functionGroup`
(chosen from each type's `<!-- doc-groups -->` marker), yet the web reports both **empty in both reference
exports**. So the reference exports predate the current template. The next regeneration fills them with no
further Go change, and the web's existing badge rendering lights up on its own.

### Regeneration is the single cross-project lever

Four things ride the next documentation regeneration, and none of them is currently written down on the Go
side as *scheduled* work: `summary:` (above — **not tracked at all**), refreshed `platformGroup`/`functionGroup`
(free), *per-finding severity* (Go parked) and *taxonomy bootstrap* (Go parked). The Go-internal rule stands —
promoting any one obliges surveying the others, because the cost (a full regeneration of every non-`record`
type) is paid once — and *taxonomy bootstrap* must ride the non-hashed `docs/generate.md`, never a per-type
`doc-prompt.md`, or it couples a cheap taxonomy edit to that expensive regeneration.

The drift-analysis work is the precedent for the other direction: it was designed **not** to be
regeneration-gated (no per-type drift templates; `doc-prompt.md` reused as the lens; header stripping done at
render time because neither generated prompt is hashed) and shipped without moving a single `promptSha256`.
That is the bar for anything else that wants to ship between regenerations.

A fifth thing now rides the regeneration indirectly: the **dependency update / SDK consolidation** idea. Its
one-time hash movement (seven types re-fetched through beta) should be batched with a regeneration scheduled
for another reason — and `resource drift` is now available to prove a bump content-neutral first, which was the
missing verification tool.

### Facets and taxonomy (Go → web) — verified compatible

- Go emits **`index.yaml` version 4**: `facets` is the sole grouping surface; the transitional `programmes` /
  `groups` mirrors of v3 are gone. The web accepts any version ≥ 1, reads `facets` first and synthesises a
  single programme axis from `programmes` only for a v2 index, so v4 is read correctly today. The web backlog's
  *sidebar by taxonomy axis* still says "v3" — wording only, harmless, fix at next edit.
- A **function-shaped spine needs no Go change**: `TaxonomyConfig` accepts arbitrary `axes`, each value matched
  by rules over the exported facts (`name`, `type`, `odataType`, `platforms` as regex; `scope` exact). A
  `function` axis is an operator-authored config section, not a feature. Go's *taxonomy bootstrap* would help
  author it from evidence, which is its only bearing on the web spine — it enlarges the *values* an operator can
  promote, not the axis mechanism.
- `functionGroup` as the alternative spine carrier is weaker than the config axis: it is single-valued,
  label-only and LLM-chosen, whereas an axis is id-bearing, ordered by the header and deterministic — the
  properties the web idea says a spine needs.

### Mutual waits with no owner

- **Per-finding severity.** Go revisits "when the web side needs per-item severity"; the web's *actionable
  findings block* revisits "when the findings table stops being readable in one pass" (15 rows today). A
  deadlock by design, and a correct one while the table is short. The trigger belongs to the **web** side
  (reader demand), and it is regeneration-gated on the Go side, so it queues behind the lever above. The web
  now has a second severity-bearing table (the drift index) that the same `:target` construction would apply
  to — if the findings block is ever built, build it once for both.
- **Tenant diff — narrowed, not resolved.** The *temporal* half is now owned and shipped: the CLI compares a
  tenant against its own export baseline (`resource drift`, verdicts and dotted-path deltas, the analysis
  prompt), and the web renders it, including a **server-rendered YAML line diff** (`yaml-diff.ts`) of baseline
  versus observed bytes. That disproves one of the web idea's four obstacles for the per-resource case — a
  readable diff of a large document did not need client-side interaction after all — and gives a *cross-tenant*
  version a shape to reuse (verdict vocabulary, delta paths, per-finding document beside the payload). What
  remains unowned is exactly the stage-vs-prod use case: identity does not survive across tenants, every web
  route is single-`:tenant`, and the prerequisite — a **stateable, testable normalisation of tenant-local
  identity** (GUIDs, group ids, names) — is still work neither backlog lists. Until a stage/prod pair is
  actually exported into one docs root, leaving that half unowned is still the right call; the web idea's
  text should be reconciled against what shipped (see Housekeeping).

### One-directional, verified stable

- **Browsable excluded bulk types** (web) rests on Go's `counts.excluded` (emitted per type) and the in-scope
  rule that documents a group only when an assignment references it (`inScope` in `generateindex.go`). Both are
  stable contracts; if picked up it is web-only work.
- **Resolve Graph ids in YAML** (Go) is **not** a dependency, despite its revisit trigger naming "a consumer of
  `resources/` other than the doc pipeline". The web YAML view is such a consumer and shows raw GUIDs **by
  design** — the two backlogs made the same choice from opposite ends (names belong in the documentation, facts
  stay facts). Nothing to reconcile.

### Housekeeping in the authoritative files

- `web/NEXT-ITERATIONS.md` still cites "the scheduled axis-index entry above" twice (in *one export button per
  tenant*); that entry shipped as `EXPORT_INDEX` and is gone. Rephrase at next edit — describe the work, do not
  cite an entry. (Still open from the last snapshot.)
- Same file, *sidebar by taxonomy axis*: "v3" → v4 (see above). (Still open.)
- Same file, *tenant diff*: the idea says "it is not settled whether the comparison belongs here at all rather
  than in the CLI" and lists a readable diff as needing client-side interaction — both overtaken by the shipped
  drift machinery and the server-rendered YAML diff. Reconcile at next edit: the open question is cross-tenant
  identity only.
- `go/NEXT-ITERATIONS.md`, *dependency updates / one Graph SDK*: "Until the drift command exists there is no
  cheap way to prove a bump content-neutral; after it ships …" — it has shipped. Rewrite the rationale in the
  present tense and drop the satisfied half of the revisit condition, leaving the SDK-bump-or-build-time half.
- Both `06-next-iterations.md` rule files still describe the same lifecycle; the Go one says strikeouts are
  cleared at *branch close*, the web one at *release time*. Today's clearing followed the Go wording. Harmless,
  but the two projects should say the same thing.

### Observations

- Neither project has scheduled work left. Both now carry a *debt-ledger* parked idea of the same shape
  (`gocognit` baseline in Go, `eslint-suppressions.json` in web) with the same rule — shrink opportunistically
  when touching a listed file, never grow — so the next branch on either side starts from a green gate with the
  debt visible rather than hidden.
- The **regeneration is the lever**, and the `summary:` template gap is still the one concrete, cheap Go change
  waiting to be scheduled for it. It is also still the only cross-project blocker where one side is finished
  and the other side has not written the work down — and it survived a full Go feature cycle unlisted, which is
  the failure mode this reassessment exists to catch. Adding it to `go/NEXT-ITERATIONS.md` is the one action
  this file recommends outright.
- **Drift** is the first feature delivered end to end across both projects since the index (`resource drift`
  → `docs analyze-drift` → web Drift view), and it added a Go → web contract that neither project's rules yet
  name as one. The severity-vocabulary mismatch above is the first symptom; a `version:` on the observation
  would be the cheapest guard.
- The **no-JS rule** stays the pivotal web decision; the CSP hardens it and roughly a third of parked web ideas
  wait on relaxing it. The drift YAML diff shipped server-side within the rule, which slightly strengthens the
  "keep it" tier. Its full blast radius is stated in the web backlog and is not repeated here.

## 3. Development workflow — draft and review

Recorded 2026-09-28. The draft is the user's, verbatim; the review and the enforcement proposal follow it. To be
done **after** the 0.3.0 releases are out, as its own branch — and dogfooded: written as `## 1.` in both
`NEXT-ITERATIONS.md` files before any script is touched.

### The draft (as stated)

Sync the dev workflows on both web and go:

1. When wanting to implement an existing plan item or promoting an idea, make sure the worktree is on an
   appropriate branch (not main).
2. Let the user work on an idea promotion and plan refinements.
3. Make sure that NEXT-ITERATIONS has been committed, before starting to implement.
4. When implementation is done, never remove a planned item, only strike them through and give the user a chance
   to define follow-ups.
5. Never implement stuff that is not in NEXT-ITERATIONS. The user can only say "implement item N" to start
   implementation; all other requests open a new idea, update an item, or add a follow-up to an item.
6. The user says "item N is done", then the changelog must be updated. But the item should not be removed, but
   archived, so that other sessions can review what has been implemented.
7. Assess branch readiness through `make branch-ready` / `npm run branch-ready`.
8. Commit.
9. Branch is done and can be pushed and merged.

### Review

The workflow is sound as a state machine — *idea → planned entry → committed → in progress → struck →
done/archived → released* — and it closes the gap section 2 keeps catching (work shipping without ever being
written down). Four things must be decided first, because they contradict what the tooling does today.

**Where it conflicts with the current gates**

- **Steps 4 and 6 break `branch-ready` and `release-ready`.** Both currently *fail* on any `~~` in
  `NEXT-ITERATIONS.md` (`go/scripts/branch-ready.sh`, `web/scripts/branch-ready.js`, and the release-time
  backstop in both `release-ready` scripts). "Archive instead of delete" means strikeouts must become *legal in
  one place* and illegal everywhere else. Concretely: add a trailing `## Archive` section; the gate then fails on
  a strikeout **outside** it (work done but not archived) and on an archive entry whose title is **not** struck.
- **Steps 7 and 8 are in the wrong order.** Both gates refuse to run on a dirty tree (`working-tree-clean.*`), so
  the real order is: commit → `branch-ready` → fix → commit → rerun. That is deliberate: the verdict describes
  the commit that gets merged.
- **Numbering vs archive.** Archived entries cannot keep numbers if active ones must stay `1..N`. Cheapest:
  archived entries lose their number and gain a stamp — `### ~~Title~~ — done 2026-09-28` — so
  `readEntryNumbers` / `entry_numbers` keep working unchanged.
- **The archive grows forever** (the Go file is already ~400 lines with parked ideas alone). Two options: keep
  archived entries only until the release that ships them (`release-ready` then reports them as clearable — the
  changelog is the permanent record, and "other sessions can review" is satisfied for the whole unreleased
  window), or keep the archive as title + goal + pointer to the changelog entry and drop the struck plan bullets.
  Recommendation: the first.

**Points to push back on**

- **Step 6 moves the changelog to "item N is done".** Today the rule is *changelog in the same edit as the
  strikeout*, which is what makes forgetting hard. If the changelog waits for a verbal "done", the only thing
  catching an omission is a gate check — doable (below), but better: keep writing the changelog at strikeout
  time and treat "done" as *archive + user reviews the entry*. Follow-ups amend the changelog entry.
- **Step 5 has no exemption for trivial fixes.** A one-line bug still needs an entry. That is right — `web/`
  already has the *Fixes* section for exactly this, and the ceremony is the point — but the fix-entry format
  should be deliberately tiny (title + one-line goal + one plan bullet) so the rule is not bypassed out of
  irritation.
- **"Never remove" needs one exception:** a planned entry the user *abandons*. Delete, or archive with
  `— dropped`? Decide.
- **Step 1 vs the release flow.** Root README step 4 commits the changelog close directly on `main`. Fine — that
  is the release, not a branch — but the rule must say "no *implementation* on `main`", not "no commits on
  `main`".

### Enforcement — four layers, soft to hard

**1. One shared rules file, not two.** `go/.windsurf/rules/06-next-iterations.md` and
`web/.windsurf/rules/06-next-iterations.md` already disagree (strikeouts cleared at *branch close* vs at
*release time*). Put the workflow once at the repository root (`.windsurf/rules/`), keeping only the
`make`/`npm` spellings per project. State the protocol as *trigger phrases → allowed actions*:

- **Any request that is not `implement item N`** → the assistant may only edit `NEXT-ITERATIONS.md` (new idea,
  promotion, refinement, follow-up). No code.
- **`implement item N`** → run the start gate first (layer 2); refuse if it fails.
- **Implementation complete** → strike plan items in place, write the changelog, *stop and ask for follow-ups*.
- **`item N is done`** → move the entry to the archive.

This layer is the only one that can enforce the conversational part (steps 2, 4, 5, 6); it is soft by nature,
so it is backed by the next three.

**2. A start gate — the missing piece.** `make start-item N` / `npm run start-item -- N`, both delegating to a
script mirroring `branch-ready`: current branch ≠ `RELEASE_BRANCH`; tree clean; entry `N` exists in **`HEAD`'s**
`NEXT-ITERATIONS.md` (`git show HEAD:NEXT-ITERATIONS.md`, not the working copy — that is what makes step 3
checkable); the entry has a `**Plan.**` with at least one unstruck bullet. Prints the plan. The rule says the
assistant runs it before the first code edit, which turns steps 1 and 3 into a yes/no.

**3. Extend `branch-ready`** (the hard gate; both projects, same checks):

- current branch ≠ `RELEASE_BRANCH` (the one git call the gate already makes can grow this);
- `git diff <base>...HEAD -- NEXT-ITERATIONS.md` non-empty — under step 5 *every* branch touches the backlog, so
  a code branch that did not is exactly the violation;
- strikeouts only inside `## Archive`; every archive entry struck;
- if the archive grew vs base, `## [Unreleased]` grew vs base (catches a "done" without a changelog);
- keep: numbering `1..N`, `version` untouched (web).

`release-ready` keeps its strikeout backstop, scoped to "outside Archive".

**4. Make it unbypassable: GitHub.** Nothing local is enforcement — `git commit` by hand always works. Branch
protection on `main` requiring a PR plus a status check that runs `make branch-ready` is what actually gates
steps 7–9. A local `pre-commit` hook refusing commits on `main` is possible but fights the release flow, which
commits on `main`; skip it.

### Open decisions before implementing

- Archive lifetime: until the shipping release, or forever?
- Changelog at strikeout time (current rule) or at "done" (draft step 6)?
- Abandoned entries: delete or `— dropped`?
- Where `todo.md` itself lives under the new rule: it is covered by neither gate. Either a sanctioned root file
  with its own refresh trigger (e.g. at every release), or it goes.
