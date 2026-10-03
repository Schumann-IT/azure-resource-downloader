---
title: Fix the Firewall CSP bridge between custom OMA-URIs and the Settings Catalog
project: go
status: done
started: 2026-10-03
finished: 2026-10-03
branch: feat/go-consistency-catalog
pr: 58
changelog: Unreleased
---
## Fix the Firewall CSP bridge between custom OMA-URIs and the Settings Catalog

*Kind:* fix

**Goal.** A Windows Firewall setting configured through a custom OMA-URI profile and the same setting configured
through the Settings Catalog are recognised as one setting, so the consistency analysis reports their conflicts and
duplicates instead of silently missing them.

> **Why.** `normaliseOMAURI` (`internal/consistency/index.go`) reads `./Vendor/MSFT/…` as `device/vendor/msft/…`,
> so `./Vendor/MSFT/Firewall/MdmStore/PublicProfile/EnableFirewall` becomes
> `device_vendor_msft_firewall_mdmstore_publicprofile_enablefirewall`. The Settings Catalog id of that node is
> `vendor_msft_firewall_mdmstore_publicprofile_enablefirewall` — no `device_` prefix — so the two never join. Found
> by the Claude Cowork catalog review (2026-10-03); confirmed against the code.
>
> **Scope.** Only the CSPs whose Settings Catalog ids carry no `device_` / `user_` scope prefix — Firewall is the
> one confirmed today. A root joins the table only with the Settings Catalog id it was observed as (Learn or a real
> export's `settingDefinitionId`), cited in the table's comment; nothing is guessed. Changing a key form moves which
> members of a configured catalog match.
>
> **Seed and example.** The tracked seed (`go/config-tailored-intune.yaml`) is already right and stays untouched:
> its three `windows-firewall-*` equivalences name the `vendor_msft_firewall_…` form (until this fix they never met
> a custom OMA-URI setting), and its `firewall` topic matches the bare regex `firewall`, which covers both forms.
> `config.example.yaml` names no firewall key. The reviewed catalog that *Replace the seed consistency catalog with
> the reviewed catalog* ships carries a `^device_vendor_msft_firewall_mdmstore_…` topic regex next to the
> `^vendor_msft_firewall_…` one; after this fix it matches nothing and can be dropped there (topic regexes are not
> validated against the canonical form, so it is dead, not an error).
>
> **Contract.** None with `web/`: the browser reads no `consistency/` output. Inside `go/` the invariant is the
> shared key builder — every key the index emits passes member validation, and validation proposes exactly the key
> the index emits. After the fix: `./Vendor/MSFT/Firewall/<path>` and `./Device/Vendor/MSFT/Firewall/<path>` (any
> case, leading `./` or `/`) index as `vendor_msft_firewall_<path>` (lowercase, `/` → `_`); a catalog member or
> selector key `device_vendor_msft_firewall_<path>` or a raw firewall OMA-URI is refused with exit `2` and
> `use "vendor_msft_firewall_<path>"`; every other `./Vendor/MSFT/…` keeps `device_vendor_msft_…`, and
> `./User/Vendor/MSFT/…` keeps `user_vendor_msft_…`. Operator-visible effect: firewall settings from custom profiles
> appear in `consistency/mechanical.yaml` under the `vendor_msft_firewall_…` key, and a catalog that names the old
> `device_vendor_msft_firewall_…` form now fails validation instead of silently matching only custom profiles.
>
> **Owner.** none — every change is under `go/`. No sequencing constraint; landing it before *Replace the seed
> consistency catalog with the reviewed catalog* lets that entry's check run see the firewall equivalences meet
> custom profiles.
>
> **Implementer.** sonnet
>
> **Not regeneration-gated.** The consistency analysis reads no prompt template; no `promptSha256` moves.

**Plan.**

- ✅ `internal/consistency/index.go`: add a package-level table of CSP roots whose Settings Catalog ids carry no scope
  prefix (only `firewall`, with a comment citing `vendor_msft_firewall_mdmstore_…` as the observed Settings Catalog
  id). In `normaliseOMAURI`, after the existing steps (lowercase, trim, `./` and `/` dropped, `vendor/msft/` read
  as `device/vendor/msft/`, `/` → `_`), rewrite a key starting with `device_vendor_msft_<root>_` to
  `vendor_msft_<root>_…` — on the `_` form, so `./Vendor/…`, `./Device/Vendor/…` and an already-underscored
  `device_vendor_msft_firewall_…` member all land on the same key, and the root match needs the trailing `_` (no
  `firewallfoo` false match). `user_` keys are left alone. Update the doc comments of `normaliseOMAURI` and
  `validMemberKey` to name the exception. Member validation follows automatically (it shares the builder).
- ✅ `internal/consistency/index_test.go` `TestNormaliseOMAURI`: add `./Vendor/MSFT/Firewall/MdmStore/PublicProfile/EnableFirewall`,
  `./Device/Vendor/MSFT/Firewall/MdmStore/DomainProfile/EnableFirewall` and a mixed-case variant →
  `vendor_msft_firewall_mdmstore_…`; keep the existing `./Device/Vendor/MSFT/Policy/…`, `./Vendor/MSFT/Policy/…` →
  `device_vendor_msft_policy_…` and `./User/…` → `user_vendor_msft_…` cases unchanged.
- ✅ `internal/consistency/catalog_test.go`: the two cases that pin the old canonical firewall form are updated to the
  new one — this is the fixed behaviour, not a weakened assertion: in `TestValidMemberKey`,
  `./Vendor/MSFT/Firewall/MdmStore/DomainProfile/EnableFirewall` → `(false, "vendor_msft_firewall_mdmstore_domainprofile_enablefirewall")`,
  plus a new case `device_vendor_msft_firewall_mdmstore_domainprofile_enablefirewall` → `(false, "vendor_msft_firewall_…")`;
  in the `CompileCatalog` refusal table, the "rule key not canonical" case expects
  `use "vendor_msft_firewall_mdmstore_domainprofile_enablefirewall"`, and a new equivalence-member case in the
  `device_vendor_msft_firewall_…` form is refused with the same message.
- ✅ `internal/consistency/catalog_test.go` `TestIndexKeysAreValidMembers`: add a Windows custom profile
  (`#microsoft.graph.windows10CustomConfiguration`) to the fixture with a `./Vendor/MSFT/Firewall/…` OMA setting,
  so the round-trip invariant covers the exception.
- ✅ `internal/consistency/consistency_test.go`: a new test on a synthetic tenant — a custom OMA-URI profile setting
  `./Vendor/MSFT/Firewall/MdmStore/PublicProfile/EnableFirewall` to `false` and a Settings Catalog policy choosing
  `vendor_msft_firewall_mdmstore_publicprofile_enablefirewall_true` for the same scope yield exactly one `conflict`
  on `vendor_msft_firewall_mdmstore_publicprofile_enablefirewall`; with the OMA value `true` they yield one
  `duplicate` instead.
- ✅ Documentation at *done*: `README.md` (the *Joined on* row for custom OMA-URI settings); `CHANGELOG.md`
  `### Fixed`.
