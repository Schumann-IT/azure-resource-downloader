import * as path from 'path';

export interface LinkEnv {
  tenant?: string;
  // Directory of the current document relative to the tenant root ('' = root).
  docDir?: string;
}

// Rewrites a Markdown link href into an application route.
//
// Only *relative* `.md` links are rewritten. They are resolved against the
// current document's directory within the tenant and prefixed with the tenant
// segment, producing an absolute app route with the `.md` suffix stripped. This
// deliberately resolves at render time (rather than relying on the browser's
// relative resolution) so that `../groups/x.md` from a policy page and
// `Microsoft.Graph/type/x.md` from index.md both land on the right route
// regardless of trailing slashes.
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

  // The link escaped the tenant root — do not turn it into a route.
  if (resolved === '..' || resolved.startsWith('../')) return null;

  const tenant = env.tenant || '';
  return `/${tenant}/${resolved}${anchor}`;
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
