import { readFileSync } from 'node:fs';
import { parse } from 'yaml';
import type { ResourceMeta, YamlValue } from './types';

/**
 * Parse a YAML string. Keys with dots and `@` (e.g. `'@odata.type'`) are kept
 * verbatim as map keys — the `yaml` library does this natively.
 */
export function parseYaml(content: string): YamlValue {
  return parse(content) as YamlValue;
}

/** Read and parse a YAML file from disk. */
export function parseYamlFile(absPath: string): YamlValue {
  return parseYaml(readFileSync(absPath, 'utf8'));
}

function asString(v: YamlValue | undefined): string | undefined {
  if (v == null) return undefined;
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return undefined;
}

/**
 * Extract common identity fields from a parsed resource document for listing
 * and navigation. Missing fields are simply omitted. Never invents values.
 */
export function extractMeta(doc: YamlValue, slug: string): ResourceMeta {
  const meta: ResourceMeta = { slug };
  if (doc && typeof doc === 'object' && !Array.isArray(doc)) {
    const o = doc as Record<string, YamlValue>;
    meta.displayName = asString(o.displayName) ?? asString(o.name);
    meta.id = asString(o.id);
    meta.createdDateTime = asString(o.createdDateTime);
    meta.lastModifiedDateTime = asString(o.lastModifiedDateTime);
    const version = o.version;
    if (typeof version === 'string' || typeof version === 'number') {
      meta.version = version;
    }
  }
  // Strip undefined keys for clean JSON output.
  return Object.fromEntries(
    Object.entries(meta).filter(([, v]) => v !== undefined),
  ) as ResourceMeta;
}
