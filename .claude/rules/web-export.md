---
paths:
  - "web/src/docs/export/**"
  - "web/src/docs/docs.controller.ts"
  - "web/views/**"
  - "web/NEXT-ITERATIONS.md"
---

# Exports (`web/src/docs/export/`, export entry points)

A standing design decision, not a work item: it constrains the export ideas in `web/NEXT-ITERATIONS.md`, so the
next one does not relitigate it. Changing it is a rule change (here and in the Windsurf twin,
`web/.windsurf/rules/03-exports.md`), with a `CHANGELOG.md` entry when an operator sees the difference.

## The export seam
- A second format is a second `ExportService` method plus its own format module under `src/docs/export/`; the
  controller and the HTML serialiser stay untouched.
- A PDF of anything reuses `pdfmake` and the HTML → PDF content walker (`src/docs/export/pdf-content.ts`) the
  drift report brought — never a print stylesheet or a second engine.

## Entry points live on the tenant picker

**Decision.** Every export a *whole tenant* produces is offered on the tenant picker (`GET /`), on that
tenant's card, as a plain `<a download>` with the one-way-publish caveat beside it. Not on the tenant
landing page, not in the top bar, not on document pages.

**Why.** The landing page belongs to `docs/summary.md` — it is documentation, and the view adds no chrome of
its own to it. The picker is where a tenant is chosen *as a whole*, which is exactly the scope an export
operates on, so the button sits with the noun it applies to and stays out of the reading flow. It also means
one place to look per tenant instead of a control repeated on every page.

**How it extends to the planned types.**

- **Further whole-tenant formats** (single-file HTML, DOCX, PDF, Markdown bundle — see the parked idea in `web/NEXT-ITERATIONS.md`):
  additional sibling links on the same card, under one `Export:` label once there is more than one. **No
  dropdown, no picker widget** — that needs client-side JavaScript, which is a non-negotiable (dropping that
  rule is its own parked idea; until it is actually dropped, this decision stands as written). If the row of
  formats ever stops fitting, the answer is a per-tenant export *page* (`GET /:tenant/_export`) listing the
  formats, not a control that needs scripting. A format that grows **options** is a second, earlier reason to
  reach for that page — see the parked idea on a single export button per tenant.
- **Partial exports** (one resource type, one document, summary only): these are the one case that must
  *not* be on the picker, because the picker cannot express the scope. Their entry point belongs next to the
  thing being exported — the document top bar next to the **Documentation | YAML** switcher for a single
  document, a sidebar section header for one type — and the picker keeps whole-tenant formats only.
- **Scoped reports** (the drift report PDF): an export of something other than the tenant's documentation
  is the same case as a partial export — its link sits on the page of the thing it exports (the tenant drift
  page, next to the **Summary | Drift** switch), shown only when there is something current to export, and
  never on the picker.
- **Media and source YAML as attachments**: no entry point of its own. It changes what an existing export
  *contains*, never where it is offered.
- **Confluence REST API synchronisation**: not a download, and it mutates a remote system, so it cannot be
  an `<a>` at all — it needs a POST, and no route may mutate state today. It gets no control until that
  design change is actually made, and if it ever does it must be visibly distinct from a download rather
  than sitting in the same row.

**What any new entry point has to keep.** A plain anchor with `download` and no client-side JavaScript; the
route shape `/:tenant/_export/<format>` behind the `_export` representation prefix, declared before the
document catch-all; the one-way caveat rendered next to the link rather than only in the README;
no anchor nested inside another anchor (the picker card is a wrapper element for exactly this reason); and a
dark-mode variant plus a visible `:focus-visible` outline.
