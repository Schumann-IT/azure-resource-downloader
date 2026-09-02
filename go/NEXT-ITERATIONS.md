# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. Redact free-text secrets

**Goal.** Bring every tenant's generated documentation back in sync with the prompts after a security fix to
the per-type prompt templates: a full one-shot regeneration across every tenant.

> **Why the templates changed.** Real tenant runs found credential-shaped secrets — a plist
> `REMOTEOFFICEAUTHKEY`, a profile-removal password in a `description` — that the service returned unmasked
> and the per-resource documents then reprinted verbatim, widening exposure from Intune readers to anyone
> with read access to the docs. The fix teaches the prompts to redact such values: a free-text
> credential-redaction rule was added to six per-type templates — `internal/models/documentation_prompt.tmpl`
> (default) and the `singleton`, `group`, `credential`, `referenced` and `arm` overrides — so a
> credential-shaped value in a free-text field (`description`/`notes`) or a decoded payload (plist/XML/base64)
> that the service did not mask is rendered as `«redacted — secret present in source»`, still documented and
> flagged in `Security`, with the literal value left only in the source YAML.
>
> **Why that forces a regeneration.** Editing a `.tmpl` changes that type's assembled `doc-prompt.md`, and
> therefore its `promptSha256`. The incremental engine hashes source + prompt + assignments, so on the next
> `azure-rd docs generate-prompt` run every document of an affected type is flagged stale (prompt-hash
> mismatch) and lands in the work list automatically — no manual list to maintain.
>
> **Scope.** Every type except the `record` family (`windowsAutopilotDeviceIdentities`, `deviceCategories`,
> `ndesConnectors`, `mobileThreatDefenseConnectors`): its template was deliberately left unchanged, so those
> documents are not reissued and keep their current `promptSha256`.
>
> **Not hash-affecting, but shipped in the same run.** The other edits to
> `internal/docs/generate_prompt_template.md` — the §7 `summary.md` contract check, the §8 on-disk run report
> (`docs/report-<date>-<time>.md`), and the three §4 checker fixes (`<details>` counting, retained-doc
> tolerance, write-once mtime snapshot) — live in the *generation* prompt, not in any type's `doc-prompt.md`,
> so they do not change `promptSha256`. They simply take effect on this same regeneration run.
>
> **Also riding this regeneration: the model-authored group axes.** A `<!-- doc-groups: platform=… |
> function=… -->` marker is now appended to every non-`record` type's `doc-prompt.md`, so it changes the same
> `promptSha256` set this regeneration already covers (record types opt out via `OmitGroupAxes` and keep
> theirs, exactly matching the redaction scope above). As each document is reissued it must additionally
> carry a `platformGroup` and a `functionGroup` in frontmatter, each a value from that marker's closed set
> (`n/a` where the axis does not apply); the §4 check enforces this and the §8 report surfaces the axis
> `uncategorised` count. So this one regeneration brings the docs back in sync with the redaction fix *and*
> populates the grouping axes `docs generate-index` already emits into `index.yaml`.

**Plan.**
- Run the documentation regeneration once per tenant (`azure-rd docs generate-prompt`, then the doc pass).
- Confirm completion via the run's own §4/§6/§7 checks and the new on-disk report; until it is done, the
  on-disk documents lag the prompts for every non-`record` type.
- Confirm each reissued document carries `platformGroup`/`functionGroup` and that the §8 report's axis
  `uncategorised` count is only the `record` family (the types that legitimately have no `doc-groups` marker).

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are removed. Each records why it is parked and what would make it worth doing.

### Idea: per-finding severity in document `Security` sections

Tag every individual security callout *inside each resource's document* with `**[risk]**` / `**[review]**` /
`**[ok]**`, so the web side can colour or filter them. **Not planned — parked deliberately**, for three
reasons:

- **It is a subjective, model-only judgement.** Nothing in the export can compute or validate whether a given
  setting is risk / review / ok.
- **It is made 400+ times** (once per callout across every document), so a bad or inconsistent batch is
  likely — and the only fix is regenerating everything.
- **Low marginal benefit.** Section-level styling (the `Security` H2 slug) already gives the frontend most of
  the visual win without the per-item risk.

This is the opposite trade-off from the tenant-summary findings severity, which is decided once per tenant on
at most six findings — tiny blast radius, easy to eyeball — and was therefore done.

**Revisit only if both hold:** (1) the closed-heading contract has proven stable across a real regeneration,
with no drift observed in practice; and (2) the web side needs per-item severity that section-level styling
cannot deliver. If promoted, treat it as its own one-shot: extend the `Security:` instruction across all
seven templates and regenerate every document — and accept that it cannot be automatically validated.
