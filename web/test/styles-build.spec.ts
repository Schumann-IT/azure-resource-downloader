import { execFileSync } from 'child_process';
import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';

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

  it('still emits the typography prose classes', () => {
    expect(css).toMatch(/prose/);
  });
});
