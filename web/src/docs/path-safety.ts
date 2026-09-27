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
//   - only serves files ending in `ext` (exactly one extension per resolver, so
//     a document can never be served from the resources root, nor a resource
//     from docs/, nor a drift payload as a drift document or vice versa);
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

// Every drift path mirrors a resource key (`<APIType>/<endpoint>/<name>`), so it
// has at least two segments. Requiring that makes everything at the drift tree
// root — the CLI's `metadata.yaml`, the agent prompt `analyze.md` and the
// summary `index.md` — unreachable through the drift resolvers by construction
// rather than by a list of names. Empty and `.` segments are refused, so the
// depth cannot be faked (`./metadata`, `x//index`).
const MIN_DRIFT_SEGMENTS = 2;

function deepEnough(relPath: string): boolean {
  const segments = relPath.split('/');
  return (
    segments.length >= MIN_DRIFT_SEGMENTS &&
    segments.every((s) => s !== '' && s !== '.')
  );
}

// A drift document the analysis agent wrote inside a tenant's drift/ folder,
// beside the payload of the same key. `drift/` holds both extensions, so it is
// served by two resolvers pinned to one extension each — never one resolver
// with an extension list.
export function resolveDriftDocument(
  driftDir: string,
  relPath: string,
): string | null {
  if (!deepEnough(relPath)) return null;
  return resolveWithinRoot(driftDir, relPath, DOC_EXT);
}

// An observed payload `azure-rd resource drift` wrote inside a tenant's drift/
// folder. `.yaml` only, like the resources root it mirrors.
export function resolveDriftPayload(
  driftDir: string,
  relPath: string,
): string | null {
  if (!deepEnough(relPath)) return null;
  return resolveWithinRoot(driftDir, relPath, RESOURCE_EXT);
}
