import { promises as fsp } from 'fs';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { resolveWithinTenant } from '../src/docs/path-safety';

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
});
