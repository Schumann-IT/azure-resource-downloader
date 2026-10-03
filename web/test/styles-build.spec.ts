import { execFileSync } from 'child_process';
import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';

const POSITIONAL = /:(?:first-child|last-child|nth-child\(|nth-last-child\(|first-of-type|last-of-type|nth-of-type\()/;

// Splits a selector list on its top-level commas: commas inside `:where(...)`,
// `:not(...)` or an attribute selector stay together.
export function splitSelectorList(list: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let quote = '';
  let current = '';
  for (const ch of list) {
    if (quote) {
      if (ch === quote) quote = '';
    } else if (ch === '"' || ch === "'") quote = ch;
    else if (ch === '(' || ch === '[') depth++;
    else if (ch === ')' || ch === ']') depth--;
    else if (ch === ',' && depth === 0) {
      out.push(current.trim());
      current = '';
      continue;
    }
    current += ch;
  }
  if (current.trim()) out.push(current.trim());
  return out;
}

// Removes every `:where(...)` group, balanced, so what is left is the part of a
// selector that carries specificity.
function stripWhere(selector: string): string {
  let out = '';
  for (let i = 0; i < selector.length; i++) {
    if (selector.startsWith(':where(', i)) {
      let depth = 0;
      let j = i + ':where'.length;
      for (; j < selector.length; j++) {
        if (selector[j] === '(') depth++;
        else if (selector[j] === ')' && --depth === 0) break;
      }
      // eslint-disable-next-line sonarjs/updated-loop-counter -- skips the balanced :where(...) group
      i = j;
      continue;
    }
    out += selector[i];
  }
  return out;
}

// True when `selector` styles a table cell by its column position. Allowed: the
// contract tables (`.findings`, `.doc-metadata`), typography's zero-specificity
// edge-padding defaults (positional part inside `:where(...)`), and row or
// container positions (`tr:last-child`, `.doc-assignments > :first-child`).
export function isPositionalCellSelector(selector: string): boolean {
  if (/\.(?:findings|doc-metadata)(?![\w-])/.test(selector)) return false;
  const live = stripWhere(selector);
  // Compounds are what sits between combinators; a cell compound is one whose
  // type is td or th.
  return live
    .split(/[\s>+~]+/)
    .some((compound) => /^(?:td|th)(?![\w-])/.test(compound) && POSITIONAL.test(compound));
}

describe('positional table-column selector guard', () => {
  it('flags a positional td or th compound', () => {
    expect(
      isPositionalCellSelector('.prose .doc-section[data-section="at-a-glance"] td:last-child'),
    ).toBe(true);
    expect(isPositionalCellSelector('.prose table th:nth-of-type(2)')).toBe(true);
    expect(isPositionalCellSelector('.x td:nth-last-child(1)')).toBe(true);
  });

  it('allows typography defaults, contract tables and row or container positions', () => {
    expect(isPositionalCellSelector('.prose :where(thead th:first-child):not(:where([class~="not-prose"] *))')).toBe(false);
    expect(isPositionalCellSelector('.prose :where(tbody td:last-child)')).toBe(false);
    expect(isPositionalCellSelector('.prose table.findings td:nth-child(3)')).toBe(false);
    expect(isPositionalCellSelector('.prose table.doc-metadata th:first-child')).toBe(false);
    expect(isPositionalCellSelector('.prose table.findings.findings-drift td:first-child')).toBe(false);
    expect(isPositionalCellSelector('.doc-assignments>:first-child')).toBe(false);
    expect(isPositionalCellSelector('.prose tbody tr:last-child')).toBe(false);
  });

  it('splits a selector list on top-level commas only', () => {
    expect(splitSelectorList('a:where(b, c), d[x="1,2"], e')).toEqual([
      'a:where(b, c)',
      'd[x="1,2"]',
      'e',
    ]);
  });
});

// Reproduces the manual check that the custom <details>/<summary> CSS (which
// @tailwindcss/typography does NOT provide) actually survives into the compiled
// stylesheet. Compiles src/styles.css with the local Tailwind v4 CLI and greps
// the output. No network: uses the installed binary in node_modules/.bin.
describe('Tailwind stylesheet build', () => {
  let outFile: string;
  let css: string;

  beforeAll(async () => {
    const tmpDir = await fsp.mkdtemp(path.join(os.tmpdir(), 'css-'));
    outFile = path.join(tmpDir, 'app.css');
    const bin = path.join(
      process.cwd(),
      'node_modules',
      '.bin',
      process.platform === 'win32' ? 'tailwindcss.cmd' : 'tailwindcss',
    );
    execFileSync(bin, ['-i', './src/styles.css', '-o', outFile], {
      cwd: process.cwd(),
      stdio: 'pipe',
    });
    css = await fsp.readFile(outFile, 'utf8');
  }, 60_000);

  it('includes the custom summary styling', () => {
    expect(css).toMatch(/summary\s*\{[^}]*cursor:\s*pointer/);
    expect(css).toContain('::-webkit-details-marker');
  });

  it('includes the custom details container styling', () => {
    expect(css).toMatch(/\.prose\s+details/);
    expect(css).toMatch(/border-left/);
  });

  it('includes the sidebar navigation disclosure styling', () => {
    expect(css).toMatch(/\.nav-tree\s+summary/);
    expect(css).toMatch(/\.nav-tree\s+summary:focus-visible/);
  });

  it('includes dark-mode overrides for the disclosure blocks', () => {
    expect(css).toContain('prefers-color-scheme');
  });

  it('declares both colour schemes and a dark html background', () => {
    // Without these the UA paints scrollbars, form chrome and the overscroll
    // area light around a dark page. Asserted as its own case: the dark-mode
    // case above only greps `prefers-color-scheme` and would pass regardless.
    expect(css).toMatch(/html\s*\{[^}]*color-scheme:\s*light dark/);
    expect(css).toMatch(
      /@media \(prefers-color-scheme:\s*dark\)\s*\{\s*html\s*\{[^}]*background-color/,
    );
  });

  it('includes the YAML view rules (shiki variables, gutter, :target)', () => {
    expect(css).toContain('--shiki-light');
    expect(css).toContain('--shiki-dark');
    expect(css).toMatch(/\.yaml-view\s+\.line-no/);
    expect(css).toMatch(/user-select:\s*none/);
    expect(css).toMatch(/\.yaml-view\s+\.line:target/);
  });

  it('includes the findings table rules (wrapping opt-out and severity icons)', () => {
    // The shared .prose table rule sets `white-space: nowrap` for wide GUID
    // tables; the findings table must override it or its prose runs off-screen.
    expect(css).toMatch(/\.prose\s+table\.findings/);
    expect(css).toMatch(/white-space:\s*normal/);
    // Severity is drawn as a masked SVG, with the word kept for screen readers.
    expect(css).toContain('--sev-icon');
    expect(css).toContain('--sev-color');
    expect(css).toMatch(/\[data-severity=["']critical["']\]/);
    expect(css).toMatch(/\[data-severity=["']high["']\]/);
    expect(css).toMatch(/\[data-severity=["']medium["']\]/);
    expect(css).toMatch(/mask:\s*var\(--sev-icon\)/);
    expect(css).toMatch(/text-indent:\s*-9999px/);
  });

  it('includes the drift findings table rules (content-sized columns and verdict icons)', () => {
    expect(css).toMatch(/table\.findings\.findings-drift\s*\{[^}]*table-layout:\s*auto/);
    expect(css).toMatch(/mask:\s*var\(--verdict-icon\)/);
    for (const verdict of ['added', 'changed', 'renamed', 'removed']) {
      expect(css).toMatch(new RegExp(`\\[data-verdict=["']${verdict}["']\\]`));
    }
    // Severity has its own scale in this table, scoped so it outranks the summary's.
    for (const severity of ['high', 'medium', 'low', 'info']) {
      expect(css).toMatch(
        new RegExp(`\\.findings-drift\\s+\\[data-severity=["']${severity}["']\\]`),
      );
    }
  });

  it('includes the section identity rules (icons, roles, dark lift)', () => {
    expect(css).toContain('.doc-section-heading');
    expect(css).toContain('--section-icon');
    expect(css).toContain('--section-color');
    expect(css).toMatch(/mask:\s*var\(--section-icon\)/);
    // One declaration per section of the closed heading vocabulary.
    expect(css).toMatch(/\[data-section=["']security["']\]/);
    expect(css).toMatch(/\[data-section=["']settings["']\]/);
    expect(css).toMatch(/\[data-section=["']lifecycle-and-operations["']\]/);
    expect(css).toMatch(/\[data-section=["']management-summary["']\]/);
    // The four role hues are one token each, so dark mode is one place.
    expect(css).toContain('--sec-risk');
    expect(css).toContain('--sec-relation');
    // A sticky top bar would otherwise cover an anchored heading.
    expect(css).toMatch(/scroll-margin-top/);
  });

  it('includes the section wrapper panels and the density mode', () => {
    expect(css).toContain('.doc-section');
    // Risk sections get a rail; substance sections get their own density.
    expect(css).toMatch(/\.doc-section\[data-section=["']security["']\]/);
    expect(css).toMatch(/\.doc-section\[data-section=["']settings["']\]\s+details/);
    expect(css).toMatch(/\.doc-section\[data-section=["']references["']\]/);
  });

  it('styles Conditions as a relation section and fixes the density and rail gaps', () => {
    expect(css).toMatch(
      /\[data-section=["']conditions["']\][^{]*\{[^}]*--section-color:\s*var\(--sec-relation\)/,
    );
    // Nested details in a definition section get the depth rail.
    expect(css).toMatch(
      /\.doc-section\[data-section=["']definition["']\]\s+details\s+details/,
    );
    // Neither membership (prose) nor conditions (tables) gets the dense mode.
    expect(css).not.toMatch(/\.doc-section\[data-section=["']membership["']\]/);
    expect(css).not.toMatch(/\.doc-section\[data-section=["']conditions["']\]/);
  });

  it('includes the setting-block note treatments', () => {
    expect(css).toMatch(/details\[data-note=["']security["']\]/);
    expect(css).toMatch(/details\[data-note=["']inert["']\]/);
    // The chips are CSS-only, so the label has to come from a custom property.
    expect(css).toContain('--note-label');
    expect(css).toMatch(/content:\s*var\(--note-label\)/);
  });

  it('includes the tool-maintained marker blocks and the metadata table', () => {
    expect(css).toContain('.doc-assignments');
    expect(css).toContain('.doc-targeted-by');
    expect(css).toContain('.doc-used-by');
    expect(css).toContain('.doc-notifications');
    // The metadata table opts out of the shared wide-table `nowrap`.
    expect(css).toMatch(/\.prose\s+table\.doc-metadata/);
  });

  it('includes the print rules (chrome hidden, layout un-stuck, tables wrap)', () => {
    expect(css).toContain('@media print');
    const print = css.slice(css.indexOf('@media print'));
    expect(print).toMatch(/\.site-header/);
    expect(print).toMatch(/\.nav-tree/);
    expect(print).toMatch(/\.doc-layout\s*\{[^}]*display:\s*block/);
    // Wide tables scroll on screen; on paper there is nothing to scroll.
    expect(print).toMatch(/\.prose\s+table\s*\{[^}]*white-space:\s*normal/);
    // A :target tint would print as a band around the last anchor followed.
    expect(print).toMatch(/:target/);
  });

  it('includes the Changed by cell tones', () => {
    for (const tone of ['warning', 'quiet', 'matched']) {
      expect(css).toContain(`.findings-drift td[data-attribution="${tone}"]`);
    }
  });

  it('styles no table column by its position outside the contract tables', () => {
    const offenders: string[] = [];
    // eslint-disable-next-line sonarjs/cognitive-complexity -- one brace-matching scan; splitting hides the recursion
    const scan = (block: string): void => {
      let i = 0;
      while (i < block.length) {
        const open = block.indexOf('{', i);
        if (open < 0) break;
        const raw = block.slice(i, open);
        // Drop `;`-terminated at-statements (@import, @layer a,b;) before the `@` test.
        const prelude = raw.slice(raw.lastIndexOf(';') + 1).trim();
        let depth = 1;
        let j = open + 1;
        for (; j < block.length && depth > 0; j++) {
          if (block[j] === '{') depth++;
          else if (block[j] === '}') depth--;
        }
        const body = block.slice(open + 1, j - 1);
        if (prelude.startsWith('@')) {
          // A nested at-rule (media, supports, layer) holds rules of its own.
          if (/^@(?:media|supports|layer|container)/.test(prelude)) scan(body);
        } else {
          for (const selector of splitSelectorList(prelude)) {
            if (isPositionalCellSelector(selector)) offenders.push(selector);
          }
        }
        i = j;
      }
    };
    scan(css.replace(/\/\*[\s\S]*?\*\//g, ''));
    expect(offenders).toEqual([]);
  });

  it('right-aligns the at-a-glance count columns by content', () => {
    expect(css).toMatch(
      /\.prose \.doc-section\[data-section=["']at-a-glance["']\]\s+\[data-numeric\]\s*\{[^}]*text-align:\s*right/,
    );
  });

  it('still emits the typography prose classes', () => {
    expect(css).toMatch(/prose/);
  });
});
