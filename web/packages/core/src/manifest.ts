import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';

/** One manifest record per source YAML. */
export interface ManifestEntry {
  sourceHash: string;
  generatedAt: string;
}

export type Manifest = Record<string, ManifestEntry>;

/** Content hash used for incremental docgen (sha256 of the source YAML). */
export function hashContent(content: string): string {
  return createHash('sha256').update(content, 'utf8').digest('hex');
}

export function loadManifest(path: string): Manifest {
  if (!existsSync(path)) return {};
  try {
    return JSON.parse(readFileSync(path, 'utf8')) as Manifest;
  } catch {
    return {};
  }
}

export function saveManifest(path: string, manifest: Manifest): void {
  writeFileSync(path, `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
}

/**
 * Decide whether a resource needs regeneration. Returns true when forced, when
 * there is no prior entry, or when the source hash changed since last run.
 */
export function needsRegen(
  manifest: Manifest,
  key: string,
  sourceHash: string,
  force = false,
): boolean {
  if (force) return true;
  const entry = manifest[key];
  return !entry || entry.sourceHash !== sourceHash;
}
