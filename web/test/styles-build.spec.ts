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

  it('still emits the typography prose classes', () => {
    expect(css).toMatch(/prose/);
  });
});
