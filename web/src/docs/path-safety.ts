import * as fs from 'fs';
import * as path from 'path';

// Resolves a request path (relative to a tenant folder, without the `.md`
// suffix) to an absolute `.md` file on disk, or returns null if the path is
// unsafe or does not resolve to a Markdown file inside the tenant.
//
// This is the one security-relevant surface in the app: `*path` is an
// attacker-controllable filesystem path. Guarantees:
//   - rejects null bytes, absolute paths, and any `..` segment up front;
//   - only serves files ending in `.md`;
//   - verifies, after realpath resolution, that the target is still inside the
//     tenant folder, so a symlink cannot escape.
export function resolveWithinTenant(
  tenantDir: string,
  relPath: string,
): string | null {
  if (!relPath) return null;
  if (relPath.includes('\0')) return null;
  if (relPath.startsWith('/')) return null;

  const segments = relPath.split('/');
  if (segments.some((s) => s === '..')) return null;

  const withExt = relPath.toLowerCase().endsWith('.md')
    ? relPath
    : `${relPath}.md`;

  const candidate = path.resolve(tenantDir, withExt);

  let realTenant: string;
  let realCandidate: string;
  try {
    realTenant = fs.realpathSync(tenantDir);
  } catch {
    return null;
  }
  try {
    realCandidate = fs.realpathSync(candidate);
  } catch {
    return null; // does not exist
  }

  const prefix = realTenant.endsWith(path.sep)
    ? realTenant
    : realTenant + path.sep;
  if (!realCandidate.startsWith(prefix)) return null;
  if (!realCandidate.toLowerCase().endsWith('.md')) return null;

  return realCandidate;
}
