// Which index the Confluence export writes onto `Overview.html`.
//
// An operator default rather than a per-request option: the export must stay a
// plain `<a download>` the tenant picker can produce, so the choice is made
// through the one configuration mechanism this project has — an environment
// variable, `EXPORT_INDEX`. The ids are fixed now so that if a per-tenant export
// page is ever built they can become the values of a query parameter with this
// variable as its default.
export type ExportIndexMode = 'type' | 'both' | 'axis';

// `type` — the by-type Pages list alone, which is today's export byte for byte.
// It is the default, so the axis index is opt-in and an operator who upgrades and
// changes nothing gets the same space they got before.
export const DEFAULT_EXPORT_INDEX_MODE: ExportIndexMode = 'type';

const MODES: readonly string[] = ['type', 'both', 'axis'];

// Parses the configured value. Anything unset, empty or unrecognised means
// `type` — the same leniency `parseFacetSelection()` applies to a bad selection,
// landing on the mode that changes nothing, because an operator typo must never
// fail an export or silently alter what it contains.
export function parseExportIndexMode(value: unknown): ExportIndexMode {
  if (typeof value !== 'string') return DEFAULT_EXPORT_INDEX_MODE;
  const id = value.trim().toLowerCase();
  return MODES.includes(id)
    ? (id as ExportIndexMode)
    : DEFAULT_EXPORT_INDEX_MODE;
}
