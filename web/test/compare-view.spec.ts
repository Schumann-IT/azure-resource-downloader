import { interleaveRows, ListingCell } from '../src/docs/compare-view';

const cell = (name: string): ListingCell => ({ name, href: `/${name}` });

describe('interleaveRows', () => {
  it('orders pairs first, then left-only, then right-only, each alphabetical', () => {
    const rows = interleaveRows(
      [
        [cell('Zeta'), cell('Zeta (prod)')],
        [cell('Alpha'), cell('Alpha')],
      ],
      [cell('Only left B'), cell('Only left A')],
      [cell('Only right')],
    );
    expect(rows.map((r) => [r.left?.name ?? null, r.right?.name ?? null])).toEqual([
      ['Alpha', 'Alpha'],
      ['Zeta', 'Zeta (prod)'],
      ['Only left A', null],
      ['Only left B', null],
      [null, 'Only right'],
    ]);
  });

  it('does not reorder its inputs in place', () => {
    const onlyA = [cell('b'), cell('a')];
    interleaveRows([], onlyA, []);
    expect(onlyA.map((c) => c.name)).toEqual(['b', 'a']);
  });
});
