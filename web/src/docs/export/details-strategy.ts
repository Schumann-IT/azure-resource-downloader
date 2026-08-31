// How a `<details>`/`<summary>` settings block is represented in an export.
//
// Confluence's HTML import does not list `<details>` among the elements it
// preserves and does not say what it does with it either, and no instance is
// available to settle it — so v1 passes the block through unchanged and the
// export is documented as provisional. This module exists so that the
// alternatives (a bold paragraph plus blockquote, headings by depth, one table
// per settings section) land in one place with one shared fixture, instead of
// being threaded through the serialiser after the fact.
export type DetailsStrategy = 'passthrough';

export const DEFAULT_DETAILS_STRATEGY: DetailsStrategy = 'passthrough';

export const DETAILS_STRATEGIES: DetailsStrategy[] = ['passthrough'];

// Parses the strategy from a request value. Returns null for anything not
// implemented, so the caller can answer 404 rather than silently exporting
// something else than what was asked for.
export function parseDetailsStrategy(
  value: string | undefined,
): DetailsStrategy | null {
  if (value === undefined || value === '') return DEFAULT_DETAILS_STRATEGY;
  return (DETAILS_STRATEGIES as string[]).includes(value)
    ? (value as DetailsStrategy)
    : null;
}

// What the serialiser does with a `<details>` or `<summary>` tag: keep it, or
// drop the tag and keep its children. Only `passthrough` exists today, so this
// is the single point a transform has to change.
export function detailsTagAction(strategy: DetailsStrategy): 'keep' | 'unwrap' {
  switch (strategy) {
    case 'passthrough':
      return 'keep';
  }
}
