import { promises as fsp } from 'fs';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import {
  resolveDriftDocument,
  resolveDriftPayload,
  resolveResource,
  resolveWithinTenant,
} from '../src/docs/path-safety';

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

// drift/ holds both extensions, so it is served by two resolvers pinned to one
// extension each, plus a depth guard that keeps the tree root unreachable.
describe('resolveDriftDocument / resolveDriftPayload', () => {
  let driftDir: string;
  let exportDir: string;

  beforeAll(async () => {
    exportDir = await fsp.mkdtemp(path.join(os.tmpdir(), 'export-'));
    driftDir = path.join(exportDir, 'drift');
    const typeDir = path.join(driftDir, 'Microsoft.Graph', 'type');
    await fsp.mkdir(typeDir, { recursive: true });
    await fsp.mkdir(path.join(exportDir, 'resources'), { recursive: true });
    await fsp.writeFile(path.join(typeDir, 'foo.yaml'), 'id: foo\n');
    await fsp.writeFile(path.join(typeDir, 'foo.md'), '# drift of foo');
    await fsp.writeFile(path.join(driftDir, 'metadata.yaml'), 'observedAt: x\n');
    await fsp.writeFile(path.join(driftDir, 'analyze.md'), '# prompt');
    await fsp.writeFile(path.join(driftDir, 'index.md'), '# summary');
    await fsp.writeFile(path.join(exportDir, 'resources', 'bar.yaml'), 'id: bar\n');
  });

  afterAll(async () => {
    await fsp.rm(exportDir, { recursive: true, force: true });
  });

  it('resolves a document and a payload of the same key, each by its own extension', () => {
    expect(resolveDriftDocument(driftDir, 'Microsoft.Graph/type/foo')).toBe(
      fs.realpathSync(path.join(driftDir, 'Microsoft.Graph/type/foo.md')),
    );
    expect(resolveDriftPayload(driftDir, 'Microsoft.Graph/type/foo')).toBe(
      fs.realpathSync(path.join(driftDir, 'Microsoft.Graph/type/foo.yaml')),
    );
  });

  it('never serves a .md as a payload, nor a .yaml as a document', () => {
    expect(resolveDriftPayload(driftDir, 'Microsoft.Graph/type/foo.md')).toBeNull();
    expect(resolveDriftDocument(driftDir, 'Microsoft.Graph/type/foo.yaml')).toBeNull();
  });

  it('keeps everything at the tree root unreachable through either resolver', () => {
    for (const name of ['metadata', 'metadata.yaml', 'analyze', 'analyze.md', 'index', 'index.md']) {
      expect(resolveDriftDocument(driftDir, name)).toBeNull();
      expect(resolveDriftPayload(driftDir, name)).toBeNull();
    }
    // The depth cannot be faked with empty or `.` segments.
    for (const name of ['./metadata', './index', 'x//metadata', '/metadata']) {
      expect(resolveDriftDocument(driftDir, name)).toBeNull();
      expect(resolveDriftPayload(driftDir, name)).toBeNull();
    }
  });

  it('rejects traversal out of the drift root', () => {
    expect(resolveDriftPayload(driftDir, '../resources/bar')).toBeNull();
    expect(resolveDriftPayload(driftDir, 'Microsoft.Graph/../../resources/bar')).toBeNull();
    expect(resolveDriftDocument(driftDir, 'Microsoft.Graph/../index')).toBeNull();
    expect(resolveDriftPayload(driftDir, 'a/b\u0000c')).toBeNull();
  });
});
