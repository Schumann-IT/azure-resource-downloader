---
title: Ship the section-6 reference check and the section-7 signal sweep as scripts in the run prompt
project: go
status: done
started: 2026-10-02
finished: 2026-10-02
branch: prepare-documentation-agent-run
changelog: Unreleased
---
## Ship the section-6 reference check and the section-7 signal sweep as scripts in the run prompt

*Kind:* fix

**Goal.** Every documentation run checks its cross-document references and sweeps the export for the tenant
summary's signals the same way, with scripts the run prompt ships — as it already does for the structural checks
(section 4) and the summary check (section 7) — instead of each agent writing its own, so a run cannot fail, pass
or miss a finding because of a script it got wrong.

> **Why.** Section 6 of `internal/docs/generate_prompt_template.md` describes six checks in a table and tells the
> agent to "script them the same way", but ships no script. On the first documentation run with the harmonised
> templates (tenant `iis.mitarbeiterangebote-staging.de`, 2026-10-02) the agent's own script first reported a
> false failure: it counted the filter link in a group's `Targeted by` row (the *Filter* column) as a group
> back-reference. The agent corrected its script and the re-run was clean, but the next run writes a new script
> with new bugs.
>
> **Signal sweep (2026-10-02, tenant `cb-gmbh.com`).** Section 7 defines a five-signal sweep over `resources/` in
> prose and leaves the script to the agent. On that run the agent had to widen the credential-word list itself
> (it first missed `REMOTEOFFICEAUTHKEY`), the closed rules (a)–(d) miss a credential in a command line (a
> TeamViewer `APITOKEN=` in `mobileApps` `installCommandLine` — the document redacted it, the summary did not
> list it), and *Credentials near expiry* picked up update-ring pause expiries, which are not credentials. The
> agent excluded ISO timestamps and pause dates by hand. The same run's documents and section 6 were clean.
>
> **Not regeneration-gated.** The run prompt is not hashed; no document changes. Takes effect at the next
> `docs generate-prompt`. The summary's *configured but unassigned* count for default objects is a separate
> parked idea (*default objects are not "configured but unassigned"*). No other entry or parked idea needs to
> ride along.
>
> **Scope.** Update-ring pause windows (`featureUpdatesPauseExpiryDateTime`, `qualityUpdatesPauseExpiryDateTime`)
> and a Microsoft 365 group's `expirationDateTime` leave the sweep entirely: they are not credentials, the signal
> list stays closed at five (appendix E), and a paused ring is already described in its own document. The
> marker-pair check is shared by copy, not import: the agent pastes each script as its own file, so section 6
> carries a verbatim copy of the section-4 helper and a test keeps the two identical.
>
> **Contract.** No artefact the web project reads changes. Documents keep their frontmatter, their four marker
> blocks (`assignments`, `targeted-by`, `used-by`, `notifications`) and their table shapes; `docs/summary.md`
> keeps `# Tenant summary`, the four H2 headings, the `### Findings` / `### Recommendations` H3 pair and the
> `Severity | Finding | Affected | Documents` table with `critical` / `high` / `medium`. The new scripts only
> read: section 6 reads `docs/` (documents and the rendered `docs/generate.md`) and `chunks/mtimes.json`; the
> sweep reads `resources/` (YAML, `metadata.yaml`, `doc-prompt.md`, plist sidecars) and prints to stdout.
> Neither writes a file. The tightened section-4 marker check (order and nesting) fails only documents that
> already broke section 2's "never nest markers, never a start without its end" rule. The follow-up changes only
> the rendered prompt's `migrate` table (`docs/generate.md`, read by the agent and the section-6 script, never by
> the web project) and adds `notificationsSha256` to migrated documents' frontmatter — a key the frontmatter
> already defines.
>
> **Owner.** none — every change is under `go/`. No sequencing constraint.
>
> **Implementer.** sonnet — the remaining follow-up is a render change, a prompt sentence and tests.

**Plan.**

- ✅ Constraints for both new scripts (state them in the template prose once, above the section-6 script): Python ≥
  3.9 standard library only (no PyYAML — like sections 4 and 7, the agent's environment is not guaranteed to have
  it); run from the tenant folder; read-only (write no file); never print a credential value (print the resource
  path, the key path or argument name, the rule and the value's length). The script text must not contain the
  literal start or end comment of any tool-filled block (`export`, `worklist`, `refmap`, `usedbymap`, `resplice`,
  `migrate`, `expected`, `summary-facts`): `ValidateMarkers` requires each exactly once in the template, so build
  such strings from the name (`f"<!-- {name}:start -->"`), as the section-4 script already does.
- ✅ `internal/docs/generate_prompt_template.md` section 4: move the inline marker loop (`for marker in
  ("assignments", "targeted-by", "used-by", "notifications")`) into a function `def marker_problems(text):` that
  returns a list of messages and is called once per document with `fail(doc, msg)` for each. It keeps the
  existing *unbalanced* and *repeated* messages and adds *end before start* and *nested* (another block's start
  between a start and its end). The section-4 table's *Assignment markers* row and section 2's "never nest"
  sentence already state the rule; no prose change beyond that.
- ✅ `internal/docs/generate_prompt_template.md` section 6: a Python block in a four-backtick fence (as in sections 4
  and 7) after the table, docstring
  `"""Section 6 reference checks. Run from the tenant folder after section 5. Exit 1 if anything failed."""`,
  the same `fail(doc, msg)` / `Counter` summary / exit-code shape as section 4, walking every document under
  `docs/` except the root files. It implements every row of the table:
  - *Assignment resolution* — inside any of the four marked blocks, every GUID except the all-zero filter
    sentinel shares its table cell (or, outside a table, its line) with a Markdown link or `⚠️ not in export`.
  - *Link symmetry* — tables are read by their header row. Policy → group: links in the assignments table's
    *Target* column only, resolved relative to the document; the group document's `targeted-by` table must have
    a *Resource*-column link resolving back to the policy. Group → policy: links in the `targeted-by` *Resource*
    column only; that document's assignments *Target* column must link back. Links in the *Filter* column (either
    table) are never group references. Compliance policy ↔ notification template: every link in a policy's
    `notifications` block must be answered by a *Resource*-column link in the template's `used-by` table, and
    every such *Resource* link by a link in that policy's `notifications` block.
  - *Link targets exist* — every relative link inside a marked block (anchors and `http(s)` links ignored)
    resolves to a file under `docs/`.
  - *Marker pairs survived* — a verbatim copy of section 4's `marker_problems`.
  - *Hashes updated* — the script reads the rendered prompt (`docs/generate.md`, or the path given as its first
    argument) and, inside its `worklist`, `resplice` and `migrate` blocks, every table whose header has a
    `Document` column: for each row and each of `assignmentsSha256` / `notificationsSha256` / `usedBySha256` /
    `targetedBySha256` present as a column with a non-empty cell, the document's frontmatter must carry that key
    with that value. A missing prompt file fails.
  - *Nothing else touched* — against `chunks/mtimes.json` (missing → fail: section 4 has not passed): a document
    whose mtime moved, or that is not in the snapshot, must be named in the `worklist`, `resplice` or `migrate`
    block; a snapshot document that no longer exists fails.
  The table stays as the explanation; the sentence "Script them the same way" becomes: run the script after
  section 5, repair what it reports through the section-5 splice script (never by hand), and re-run until it
  exits 0. The *Marker pairs survived* row says the script carries the section-4 helper.
- ✅ `internal/docs/generate_prompt_template.md` section 7: ship the signal sweep as a Python block in a four-backtick fence
  (as in sections 4 and 7) under *Where the facts come from*, docstring `"""Section 7 signal sweep. Run from the tenant folder before writing
  docs/summary.md. Prints each signal with the resources it names."""`. It reads YAML with a small
  indentation-aware walker over the `yaml.v3` output shape (mapping scalars with their key path, `|` / `>` block
  scalars joined, the keys of each list item grouped), measures time against `metadata.yaml`'s `generatedAt`
  (never the clock), prints one section per signal with a count and deduplicated, sorted lines, and exits 0
  (1 only when `resources/metadata.yaml` is missing). It sweeps `resources/**/*.yaml` except `metadata.yaml`, plus
  `.mobileconfig` / `.plist` / `.xml` sidecars for rule (a) only, so the result does not depend on the
  `base64-decode` transformer's inline or file mode. Within it:
  - *Not in force* — top-level `state: enabledForReportingButNotEnforced`, `state: disabled` or
    `isEnabled: false`, counted per type with each resource's path.
  - *Configured but unassigned* — from `metadata.yaml`: entries with `presentInTenant: true` whose type has
    `hasAssignments: true` and that carry no `assignmentTargets`, with display name and derived document path —
    the same set the `summary-facts` block counts.
  - *Dangling targets* — from `metadata.yaml`: every `groupId` in a present entry's `assignmentTargets` that is
    not the `resourceId` of any `Microsoft.Graph/groups/…` entry, with the number of resources assigning it —
    the GUIDs the `refmap` block flags dangling.
  - *Credentials near expiry* — only types whose `doc-prompt.md` `doc-headings` marker lists `Expiry and renewal`
    (the credential family: Apple push certificate, VPP tokens, DEP onboarding settings), and only their
    top-level `expirationDateTime` / `tokenExpirationDateTime`, quoted or not: past, or within 180 days of the
    export timestamp, printed with the date and the days left.
  - one credential-word list, stated once and used by rules (a), (b), (c) and (e): `password`, `passwd`, `pwd`,
    `passphrase`, `secret`, `token`, `apikey`, `authkey`, `accesskey`, `privatekey`, `sharedkey`. A name matches
    when, lowercased with `_`, `-` and `.` removed and one trailing `value` / `text` / `string` dropped, it ends
    with a list word (`wifiPassword`, `preSharedKey`, `REMOTEOFFICEAUTHKEY`, `APITOKEN` match; `tokenName`,
    `passwordMinimumLength`, `tokenExpirationDateTime` do not). Rule (c) instead looks for a list word as a
    whole word in the free text.
  - *Plaintext credentials*, rules (a)–(d) as today, plus rule (e): in `installCommandLine`,
    `uninstallCommandLine` and any key ending in `CommandLine` (case-insensitive), a `NAME=value`, `/NAME value`,
    `/NAME:value` or `-NAME value` argument (quotes stripped) whose name matches the list and whose value is
    credential-shaped.
  - credential-shaped as today (≥ 10 characters; a hex run of 16+ first, else at least three of lowercase,
    uppercase, digit, other non-space), with these exclusions added to the existing ones: ISO-8601 dates and
    timestamps (`YYYY-MM-DD`, optionally `T…`) and any value containing whitespace are never credential-shaped.
  The table stays as the explanation and is updated to match: the *Credentials near expiry* row names the
  credential types and their two fields and says pause windows and group expiry are not credentials; the
  *Plaintext credentials* row says "exactly five rules" and adds (e); the credential-shaped paragraph gains the
  two exclusions. The rule (c) caution (free-text fields only) stays. The sentence introducing the sweep says to
  run the shipped script, not to write one.
- ✅ Tests (`internal/docs/generateprompt_test.go`), presence: the default template contains both docstrings,
  `def marker_problems(`, the *Target* / *Resource* column names the section-6 symmetry reads, the
  credential-word list, rule (e)'s `CommandLine` match and `Expiry and renewal`; and a `GeneratePrompt` run with
  the default template still succeeds (no tool-filled marker literal leaked into a script).
- ✅ Tests, helper identity: extract the two `def marker_problems` bodies from the template (by the section-4 and
  section-6 docstrings) and assert they are byte-identical.
- ✅ Tests, section-6 fixture run (skip with `t.Skip` when `exec.LookPath("python3")` fails; CI's
  `ubuntu-latest` has it): in `t.TempDir()`, a tenant with one compliance policy assigned to one group with an
  assignment filter and referencing one notification template; `docs/generate.md` produced by `GeneratePrompt`
  on that fixture so the parser is pinned to the real table shapes; hand-written documents whose blocks and
  frontmatter hashes match the work list; `chunks/mtimes.json` written from the files' mtimes. Extract the
  script from the rendered prompt, run it with the tenant as working directory. Clean → exit 0, including with
  the filter link in the group's `Targeted by` *Filter* column. Each planted defect → exit 1 with its message:
  the policy row removed from the group's `Targeted by`, a bare group GUID in an assignments block, a wrong
  `targetedBySha256`, a document outside the three lists touched (`os.Chtimes`), a `notifications` link with no
  `Used by` answer.
- ✅ Tests, sweep fixture run (same skip): a `resources/` tree with `metadata.yaml` (`generatedAt`
  `2026-10-02T00:00:00Z`) holding a win32 app with `installCommandLine: setup.exe APITOKEN=<planted>`, an iOS
  custom profile whose plist carries `<key>RemoteOfficeAuthKey</key><string><planted></string>`, an update ring
  with both pause expiries inside 180 days, a Microsoft 365 group with `expirationDateTime` inside 180 days, a VPP
  token expiring inside 180 days (with its credential-family `doc-prompt.md`), a `tokenName` with a
  three-class value, and one unassigned assignment-capable resource. Assert the output reports the `APITOKEN`
  argument (rule e), the plist key (rule a), the VPP expiry and the unassigned resource; does not report the
  pause expiries, the group expiry or `tokenName`; and never contains either planted value.
- ✅ Documentation at *done*: `README.md` *Documentation generation*, items 6 and 7 of the agent's steps (the
  shipped reference-check and signal-sweep scripts; command-line credentials in the summary); `CHANGELOG.md`
  `### Changed`.
- ✅ Follow-up (found while implementing): `renderMigrate` (`internal/docs/generateprompt_render.go`) prints only
  `| Document | Type | assignmentsSha256 |`, although `GeneratePrompt` also appends a `Migrate` item carrying
  `NotificationsSha256` (reason "document predates the noncompliance-notification markers"). A document migrated
  for its notification markers never receives that hash — the next run lists it as a notifications re-splice —
  and section 6's *Hashes updated* check has no column to verify. 5e also says "the row's reason names which",
  but no Reason column is rendered. A compliance policy missing both markers yields two items for the same
  document. Change `renderMigrate` to:
  - header `| Document | Type | Reason | assignmentsSha256 | notificationsSha256 |` (the re-splice tables'
    `Document | Type | Reason | <hash>` order, hashes in the work list's order);
  - **one row per document**: merge items sharing a `DocPath` — reasons joined with `; ` in item order
    (assignments first, as `GeneratePrompt` appends them), each hash cell from whichever item carries it — then
    sort by `DocPath`;
  - an empty cell (`hashCell("")`) for a hash the document is not migrated for, so an assignments-only row has a
    blank `notificationsSha256` and a notifications-only row a blank `assignmentsSha256`;
  - the empty-state sentence "_No documents need migrating — every current document already carries the markers
    its content needs._".
- ✅ Follow-up, `internal/docs/generate_prompt_template.md` (run prompt, not hashed): the header comment's `migrate`
  line reads "documents predating the assignment or noncompliance-notification markers; …"; 5e's paragraph after
  the `migrate` block gains, after "then apply 5c normally", the sentence "Then write each filled hash cell of the
  row — `assignmentsSha256`, `notificationsSha256` — into that document's frontmatter; a migrated document whose
  hash you did not write is re-spliced on every future run." No change to the section-6 script: it reads hash
  columns by header name and skips blank cells.
- ✅ Follow-up tests: a table test for `renderMigrate` in `generateprompt_test.go` — empty input gives the
  empty-state sentence; an assignments-only, a notifications-only and a two-item same-document input give the
  five-column header, the blank cells as above, one merged row with both reasons and both hashes, rows sorted by
  document. Extend `TestGeneratePromptNotificationsMigrate` (`notificationtemplates_test.go`) to assert the
  rendered `migrate` block (`DryRun` result or `docs/generate.md`) contains the item's `notificationsSha256` in
  the policy's row. Section-6 script (same `python3` skip): a variant of `referenceFixture` in which the policy
  document is written current (`sourceSha256` / `promptSha256` matching) but without either marker **before**
  `GeneratePrompt` runs, so it lands in `migrate` as one merged row; then rewrite it with the fixture body and
  both hashes from the `Migrate` items. Assert exit 0 on that tree, and exit 1 with
  "notificationsSha256 missing or not the value the prompt gives" when its `notificationsSha256` line is removed.
