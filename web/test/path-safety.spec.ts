import { promises as fsp } from 'fs';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { resolveResource, resolveWithinTenant } from '../src/docs/path-safety';

describe('resolveWithinTenant', () => {
  let tenantDir: string;
  let outsideDir: string;

  beforeAll(async () => {
    tenantDir = await fsp.mkdtemp(path.join(os.tmpdir(), 'tenant-'));
    outsideDir = await fsp.mkdtemp(path.join(os.tmpdir(), 'outside-'));
    await fsp.mkdir(path.join(tenantDir, 'Microsoft.Graph', 'type'), {
      recursive: true,
    });
    await fsp.writeFile(
      path.join(tenantDir, 'Microsoft.Graph', 'type', 'foo.md'),
      '# foo',
    );
    await fsp.writeFile(path.join(tenantDir, 'index.md'), '# index');
    await fsp.writeFile(path.join(outsideDir, 'secret.md'), 'secret');
  });

  afterAll(async () => {
    await fsp.rm(tenantDir, { recursive: true, force: true });
    await fsp.rm(outsideDir, { recursive: true, force: true });
  });

  it('resolves a valid document (adding the .md suffix)', () => {
    const resolved = resolveWithinTenant(tenantDir, 'Microsoft.Graph/type/foo');
    expect(resolved).toBe(
      fs.realpathSync(path.join(tenantDir, 'Microsoft.Graph/type/foo.md')),
    );
  });

  it('rejects path traversal to /etc/passwd', () => {
    expect(resolveWithinTenant(tenantDir, '../../etc/passwd')).toBeNull();
    expect(resolveWithinTenant(tenantDir, '../../../../etc/passwd')).toBeNull();
  });

  it('rejects escaping into a sibling directory', () => {
    const rel = `../${path.basename(outsideDir)}/secret`;
    expect(resolveWithinTenant(tenantDir, rel)).toBeNull();
  });

  it('rejects null bytes and absolute paths', () => {
    expect(resolveWithinTenant(tenantDir, 'foo\u0000bar')).toBeNull();
    expect(resolveWithinTenant(tenantDir, '/etc/passwd')).toBeNull();
  });

  it('returns null for a non-existent document', () => {
    expect(resolveWithinTenant(tenantDir, 'Microsoft.Graph/type/missing')).toBeNull();
  });

  it('rejects a symlink that escapes the root', async () => {
    const link = path.join(tenantDir, 'escape.md');
    await fsp.symlink(path.join(outsideDir, 'secret.md'), link);
    expect(resolveWithinTenant(tenantDir, 'escape')).toBeNull();
    await fsp.rm(link, { force: true });
  });
});

// The resources root is the second served root. One extension per root is what
// keeps the two apart: a document can never be served from resources/, nor a
// source YAML from docs/.
describe('resolveResource', () => {
  let resourcesDir: string;
  let docsDir: string;

  beforeAll(async () => {
    const exportDir = await fsp.mkdtemp(path.join(os.tmpdir(), 'export-'));
    resourcesDir = path.join(exportDir, 'resources');
    docsDir = path.join(exportDir, 'docs');
    await fsp.mkdir(path.join(resourcesDir, 'Microsoft.Graph', 'type'), {
      recursive: true,
    });
    await fsp.mkdir(docsDir, { recursive: true });
    await fsp.writeFile(
      path.join(resourcesDir, 'Microsoft.Graph', 'type', 'foo.yaml'),
      'id: foo\n',
    );
    await fsp.writeFile(
      path.join(resourcesDir, 'Microsoft.Graph', 'type', 'notes.md'),
      '# not a resource',
    );
    await fsp.writeFile(path.join(docsDir, 'secret.md'), '# doc');
  });

  afterAll(async () => {
    await fsp.rm(path.dirname(resourcesDir), { recursive: true, force: true });
  });

  it('resolves a valid resource (adding the .yaml suffix)', () => {
    const resolved = resolveResource(resourcesDir, 'Microsoft.Graph/type/foo');
    expect(resolved).toBe(
      fs.realpathSync(
        path.join(resourcesDir, 'Microsoft.Graph/type/foo.yaml'),
      ),
    );
  });

  it('serves only .yaml — a Markdown file in resources/ is not reachable', () => {
    expect(
      resolveResource(resourcesDir, 'Microsoft.Graph/type/notes.md'),
    ).toBeNull();
    expect(
      resolveResource(resourcesDir, 'Microsoft.Graph/type/notes'),
    ).toBeNull();
  });

  it('does not serve .yml (exports only ever write .yaml)', () => {
    expect(
      resolveResource(resourcesDir, 'Microsoft.Graph/type/foo.yml'),
    ).toBeNull();
  });

  it('cannot cross into the sibling docs/ root', () => {
    expect(resolveResource(resourcesDir, '../docs/secret')).toBeNull();
    expect(resolveResource(resourcesDir, '../docs/secret.md')).toBeNull();
  });

  it('rejects null bytes and absolute paths', () => {
    expect(resolveResource(resourcesDir, 'foo\u0000bar')).toBeNull();
    expect(resolveResource(resourcesDir, '/etc/passwd')).toBeNull();
  });

  it('a source YAML is not reachable through the document guard', () => {
    expect(
      resolveWithinTenant(resourcesDir, 'Microsoft.Graph/type/foo.yaml'),
    ).toBeNull();
  });
});
