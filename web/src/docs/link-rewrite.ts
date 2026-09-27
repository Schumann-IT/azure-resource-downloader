import * as path from 'path';

export interface LinkEnv {
  tenant?: string;
  // Directory of the current document relative to the tenant root ('' = root).
  docDir?: string;
  // Route prefix the links of this document resolve under, for a document that
  // lives in a served root other than docs/ (the drift documents, `_drift`).
  // Unset for documentation, whose links are therefore byte-identical.
  routeBase?: string;
}

// The documentation root, as seen from a sibling root such as drift/. The drift
// documents link the resource's documentation as `../../../docs/<type>/<name>.md`,
// which escapes their own root by exactly this prefix.
const SIBLING_DOCS_PREFIX = '../docs/';

// Rewrites a Markdown link href into an application route.
//
// Only *relative* `.md` links are rewritten. They are resolved against the
// current document's directory within the tenant and prefixed with the tenant
// segment, producing an absolute app route with the `.md` suffix stripped. This
// deliberately resolves at render time (rather than relying on the browser's
// relative resolution) so that `../groups/x.md` from a policy page and
// `Microsoft.Graph/type/x.md` from a docs-root document both land on the right route
// regardless of trailing slashes.
//
// With a `routeBase`, the resolved path is prefixed with it, so drift documents
// link each other inside the drift view, and a link that escapes into the
// sibling docs/ tree becomes the documentation route.
//
// Returns null when the link should be left untouched (anchors, absolute URLs,
// external schemes, protocol-relative, non-`.md` targets, or links that escape
// the tenant root).
export function rewriteHref(href: string, env: LinkEnv): string | null {
  if (!href) return null;
  if (href.startsWith('#')) return null; // same-page anchor
  if (href.startsWith('//')) return null; // protocol-relative
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) return null; // has a scheme (http:, mailto:, ...)
  if (href.startsWith('/')) return null; // already an absolute app route

  const hashIdx = href.indexOf('#');
  const anchor = hashIdx >= 0 ? href.slice(hashIdx) : '';
  const pathPart = hashIdx >= 0 ? href.slice(0, hashIdx) : href;

  if (!pathPart.toLowerCase().endsWith('.md')) return null;

  const noExt = pathPart.slice(0, -'.md'.length);
  const dir = env.docDir || '';
  const resolved = path.posix.normalize(path.posix.join(dir, noExt));

  const route = routeFor(resolved, env);
  return route === null ? null : `/${env.tenant || ''}/${route}${anchor}`;
}

// The tenant-relative route for a resolved link path, or null when it escaped
// its root. Escaping is final for documentation; from a sibling root only the
// docs/ tree is still a route.
function routeFor(resolved: string, env: LinkEnv): string | null {
  const base = env.routeBase ? `${env.routeBase}/` : '';
  if (resolved !== '..' && !resolved.startsWith('../')) {
    return `${base}${resolved}`;
  }
  const doc = resolved.startsWith(SIBLING_DOCS_PREFIX)
    ? resolved.slice(SIBLING_DOCS_PREFIX.length)
    : '';
  return base !== '' && doc !== '' ? doc : null;
}

// Extracts the first ATX H1 (`# Title`) outside of fenced code blocks. Embedded
// shell scripts in some docs contain `#` comment lines inside fences, which are
// not headings.
export function extractTitle(markdown: string): string {
  const lines = markdown.split(/\r?\n/);
  let inFence = false;
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed.startsWith('```') || trimmed.startsWith('~~~')) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const match = /^#\s+(.+?)\s*$/.exec(line);
    if (match) return match[1];
  }
  return '';
}
