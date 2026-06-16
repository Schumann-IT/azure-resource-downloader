import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import {
  hashContent,
  loadManifest,
  needsRegen,
  saveManifest,
  type Manifest,
} from '../src/manifest';
import { buildDocPrompt } from '../src/prompt';

let dir: string;
beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), 'ard-manifest-'));
});
afterAll(() => {
  rmSync(dir, { recursive: true, force: true });
});

describe('manifest', () => {
  it('hashes content deterministically', () => {
    expect(hashContent('abc')).toBe(hashContent('abc'));
    expect(hashContent('abc')).not.toBe(hashContent('abd'));
  });

  it('round-trips to disk', () => {
    const path = join(dir, '.docgen-manifest.json');
    const m: Manifest = { 'a/b.yaml': { sourceHash: 'h1', generatedAt: 'now' } };
    saveManifest(path, m);
    expect(loadManifest(path)).toEqual(m);
  });

  it('returns empty manifest when file is missing', () => {
    expect(loadManifest(join(dir, 'nope.json'))).toEqual({});
  });

  it('decides regeneration incrementally', () => {
    const m: Manifest = { k: { sourceHash: 'h1', generatedAt: 'now' } };
    expect(needsRegen(m, 'k', 'h1')).toBe(false); // unchanged
    expect(needsRegen(m, 'k', 'h2')).toBe(true); // changed hash
    expect(needsRegen(m, 'new', 'h1')).toBe(true); // unseen key
    expect(needsRegen(m, 'k', 'h1', true)).toBe(true); // forced
  });
});

describe('buildDocPrompt', () => {
  it('appends a fenced yaml block to the verbatim prompt', () => {
    const out = buildDocPrompt('# Prompt\nbody', 'a: 1\nb: 2');
    expect(out).toContain('# Prompt');
    expect(out).toContain('```yaml\na: 1\nb: 2\n```');
  });
});
