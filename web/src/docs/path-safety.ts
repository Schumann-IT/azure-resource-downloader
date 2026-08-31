import * as fs from 'fs';
import * as path from 'path';

export const DOC_EXT = '.md';
export const RESOURCE_EXT = '.yaml';

// Resolves a request path (relative to a served root, with the extension
// optional) to an absolute file on disk, or returns null if the path is unsafe
// or does not resolve to a file of exactly `ext` inside that root.
//
// This is the one security-relevant surface in the app: `*path` is an
// attacker-controllable filesystem path. Guarantees:
//   - rejects null bytes, absolute paths, and any `..` segment up front;
//   - only serves files ending in `ext` (one extension per root, so a document
//     can never be served from the resources root, nor a resource from docs/);
//   - verifies, after realpath resolution, that the target is still inside the
//     root, so a symlink cannot escape.
export function resolveWithinRoot(
  rootDir: string,
  relPath: string,
  ext: string,
): string | null {
  if (!relPath) return null;
  if (relPath.includes('\0')) return null;
  if (relPath.startsWith('/')) return null;

  const segments = relPath.split('/');
  if (segments.some((s) => s === '..')) return null;

  const withExt = relPath.toLowerCase().endsWith(ext)
    ? relPath
    : `${relPath}${ext}`;

  const candidate = path.resolve(rootDir, withExt);

  let realRoot: string;
  let realCandidate: string;
  try {
    realRoot = fs.realpathSync(rootDir);
  } catch {
    return null;
  }
  try {
    realCandidate = fs.realpathSync(candidate);
  } catch {
    return null; // does not exist
  }

  const prefix = realRoot.endsWith(path.sep) ? realRoot : realRoot + path.sep;
  if (!realCandidate.startsWith(prefix)) return null;
  if (!realCandidate.toLowerCase().endsWith(ext)) return null;

  return realCandidate;
}

// A document inside a tenant's docs/ folder.
export function resolveWithinTenant(
  tenantDir: string,
  relPath: string,
): string | null {
  return resolveWithinRoot(tenantDir, relPath, DOC_EXT);
}

// A source resource inside a tenant's resources/ folder. Exports only ever
// write `.yaml`, so `.yml` is deliberately not served.
export function resolveResource(
  resourcesDir: string,
  relPath: string,
): string | null {
  return resolveWithinRoot(resourcesDir, relPath, RESOURCE_EXT);
}
