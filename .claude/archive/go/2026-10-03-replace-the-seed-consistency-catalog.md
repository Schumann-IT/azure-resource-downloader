---
title: Replace the seed consistency catalog with the reviewed catalog
project: go
status: done
started: 2026-10-03
finished: 2026-10-03
branch: feat/go-consistency-catalog
pr: 58
changelog: Unreleased
---
## Replace the seed consistency catalog with the reviewed catalog

*Kind:* feat

**Goal.** The worked example ships the consistency catalog that was checked against Microsoft Learn and the two
real tenant exports, instead of the blind seed, so an operator who copies it gets correct members, current
references, documented enforcement and the rules Learn actually supports.

> **Source.** The Claude Cowork review of 2026-10-03: `Claude outputs/consistency-catalog-review.md` (the
> reasoning, per entry) and `Claude outputs/consistency-catalog-proposed.yaml` (the catalog), both git-ignored. The
> proposal: 37 equivalences (17 fixed seed entries, 20 additions; the three minimum-OS equivalences dropped as
> unmatchable), 13 topics (`assignment-filters` dropped; `windows-feature-update-policy`,
> `windows-driver-update-policy`, `conditional-access` added), 29 rules (R6 and R7 removed — they move to the
> scope model, see *R6 and R7 as assignment checks in the scope model*); 2 equivalences and 7 rules `verified`,
> the rest `verify`; every reference moved to the current Learn addresses.
>
> **Checked (Claude Code, 2026-10-03).** `go/azure-rd docs analyze-consistency` with the proposal against scratch
> copies of both exports: exit 0 for both; only `macos-password-complex-characters` matches in neither tenant
> (kept for future tenants, as the review says). An independent leak check of both files against both exports:
> clean (every raw hit a Microsoft-defined id, a product name or a Learn example name). The detector already
> handles the review's engine questions on shared members (no double reporting; scope rules out cross-platform
> pairs), so the iOS entries stay.
>
> **Value conditions** in rule `violation` text (e.g. "deferral > 0") are judged by *The consistency analysis job*;
> no rule schema change here.
>
> **Not regeneration-gated.** Operator action at done: copy the new section into the live base file under
> `go/.config/`.
>
> **Contract.** No Go → web artefact changes: the catalog is operator configuration read only by
> `docs analyze-consistency`; it changes what lands in `consistency/` (mechanical findings, `metadata.yaml`
> catalog counts and `unmatchedMembers`), which the browser does not read today. Schema unchanged
> (`consistency.version: 1`). Firewall members are already in the bridged `vendor_msft_firewall_…` form (the
> three `windows-firewall-*` equivalences); no member or `keys:` selector uses the refused
> `device_vendor_msft_firewall_…` form — only the `firewall` topic regex does, and the Plan drops it.
>
> **Owner.** none outside `go/`. Files: `go/config-tailored-intune.yaml`, `go/cmd/docs/analyze_consistency_test.go`;
> never `go/.config/`, never `output/`. No sequencing (the Firewall CSP bridge fix is already on this branch).
>
> **Implementer.** sonnet

**Plan.**

- ✅ `go/config-tailored-intune.yaml`: replace everything from the `consistency:` key to the end of the file with the
  proposal's `consistency:` block (`version: 1`, then `equivalences`, `topics`, `rules`); do not copy the
  proposal's four leading comment lines (they name git-ignored files and a Python validation). Re-indent to the
  file's style (list items indented under their key, a blank line between sections) and add a short comment per
  group, as the seed has (e.g. `# --- macOS password: …`); never touch `go/.config/`.
- ✅ Rewrite the comment block above `consistency:` (today "EVERY ENTRY IS 'status: verify'" and "'enforced' is
  deliberately left out everywhere"), which the new catalog makes false: the catalog was reviewed against
  Microsoft Learn on 2026-10-03; 2 equivalences and 7 rules are `verified`, the rest stay `verify` (Apple key
  mappings rest on Apple's payload documentation, not Learn); `enforced` is set only on the password equivalences,
  which Learn documents as remediated on Windows, macOS and iOS; R6/R7 (assignment-filter and include/exclude
  kind checks) left the catalog for the scope model; the minimum-OS comparison is blocked on indexing. Keep the
  opening paragraph (what the catalog is, copy into your base file, canonical keys).
- ✅ Drop the proposal's `^device_vendor_msft_firewall_mdmstore_…` alternative from the `firewall` topic's key regex:
  after *Fix the Firewall CSP bridge between custom OMA-URIs and the Settings Catalog* custom firewall OMA-URIs index
  as `vendor_msft_firewall_…`, so that form matches nothing (harmless, but dead).
- ✅ Tests in `go/cmd/docs/analyze_consistency_test.go`: `TestTailoredConfigCatalogCompiles` keeps compiling the
  section and additionally pins `Counts()` = 37 equivalences / 13 topics / 29 rules; asserts no rule id starting
  `r6-` or `r7-`, no topic `assignment-filters`, no equivalence id containing `minimum-os`; and asserts the
  `status: verified` set is exactly `windows-defender-cloud-protection-maps`, `windows-defender-realtime-admx`
  (equivalences) and `r5-deferral-with-feature-profile`, `r5b-ring-feature-pause-with-feature-profile`,
  `r5c-ring-driver-exclusion-with-driver-policy`, `r5d-overlapping-feature-update-policies`,
  `r5e-apple-enforcement-ignores-update-settings`, `r-whfb-user-over-device-enable`,
  `r-whfb-user-over-device-minpinlength` (rules).
- ✅ Test: no member, topic key regex or rule `keys:` entry in the tracked catalog starts with
  `device_vendor_msft_firewall_` (regexes: after the leading `^`; guards the bridge form against a later catalog round).
- ✅ Keep `TestTailoredConfigSeedsThePasswordLengthEquivalence` unchanged (members and `>=` relation are identical in
  the proposal); update the "seed" wording in both tests' doc comments and failure messages to "tracked catalog".
- ✅ Documentation at *done*: `README.md` (*The catalog* — the seed paragraph: counts, what is verified, R6/R7 moved
  to the scope model, the minimum-OS comparison blocked on indexing); `CHANGELOG.md` `### Changed` with the
  operator action in bold.
