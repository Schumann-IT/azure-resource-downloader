// Page titles for a Confluence HTML import.
//
// The importer takes the *file name* as the page title, so a title has to be a
// legal zip entry name as well as a legal Confluence title. Titles are built as
// `<type leaf> — <display name>`: the space is flat (there is no page
// hierarchy), so the type prefix is what keeps an alphabetical page list
// readable and what makes cross-type name clashes impossible by construction.
//
// Pure and Nest-free on purpose — this is the module that decides what every
// link in the export points at, so it is unit tested on its own.

// Characters that are illegal in a file name (Windows, and therefore in a zip
// entry that has to survive extraction) or in a Confluence page title. A run is
// replaced by a single `-`, so `Win10/11: baseline` does not grow a dash per
// character.
const ILLEGAL = /[\\/:*?"<>|]+/g;

// Control characters have no business in a file name either.
const CONTROL = /[\u0000-\u001f\u007f]/g;

// Well below Confluence's 255-character title limit, leaving room for the
// deduplication suffix and the `.html` extension.
export const MAX_TITLE_LENGTH = 200;

// Used when a resource carries no usable name at all, so a page is still
// addressable instead of the export failing.
export const FALLBACK_TITLE = 'Untitled';

export interface PageSource {
  // Document path as the index records it, relative to docs/ (`<type>/<name>.md`).
  doc: string;
  // Azure/Graph resource type (`Microsoft.Graph/groups`).
  type: string;
  // Human name from the index; may be empty.
  displayName: string;
  // The document's own H1, used only when the index has no display name.
  h1?: string;
}

export interface PageName {
  // Extensionless document path — the key every link rewrite looks up.
  docPath: string;
  // Confluence page title.
  title: string;
  // Zip entry file name within the space folder.
  file: string;
}

// Last segment of a resource type: `Microsoft.Graph/groups` -> `groups`.
export function typeLeaf(type: string): string {
  const parts = type.split('/').filter(Boolean);
  return parts.length ? parts[parts.length - 1] : '';
}

// Makes a title safe as a file name and a Confluence title: illegal characters
// become `-`, whitespace collapses, and leading/trailing dots and spaces go
// (a trailing dot is not extractable on Windows). Returns '' when nothing
// usable is left, so the caller can fall back.
export function sanitizeTitle(raw: string): string {
  const cleaned = (raw || '')
    .replace(CONTROL, ' ')
    .replace(ILLEGAL, '-')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/^[.\s]+/, '')
    .replace(/[.\s]+$/, '');
  // A name made only of illegal characters sanitises to punctuation, which is a
  // legal file name but not a usable page title — report it as unusable so the
  // caller falls back.
  if (/^[-\s]*$/.test(cleaned)) return '';
  return cleaned.length > MAX_TITLE_LENGTH
    ? cleaned.slice(0, MAX_TITLE_LENGTH).trim().replace(/[.\s]+$/, '')
    : cleaned;
}

// Drops the `.md` suffix from an index document path.
export function stripDocExtension(doc: string): string {
  return doc.replace(/\.md$/i, '');
}

// `<type leaf> — <name>`, with the name taken from the index display name, then
// the document's H1, then the file's base name.
export function pageTitle(source: PageSource): string {
  const docPath = stripDocExtension(source.doc);
  const base = docPath.split('/').filter(Boolean).pop() || '';
  const name = sanitizeTitle(source.displayName || source.h1 || base);
  const leaf = sanitizeTitle(typeLeaf(source.type));
  if (!name) return leaf || FALLBACK_TITLE;
  if (!leaf) return name;
  return sanitizeTitle(`${leaf} — ${name}`);
}

// Builds the document-path -> page-name map the whole export keys off.
//
// Titles are deduplicated deterministically: sources are processed in document
// path order and a repeat gets a ` (2)`, ` (3)` suffix. Confluence treats page
// titles case-insensitively and zip extraction on a case-insensitive filesystem
// would overwrite, so the collision check ignores case. Never fails and never
// overwrites — a whole-tenant export must not die on two resources that happen
// to sanitise the same.
export function buildPageNames(sources: PageSource[]): Map<string, PageName> {
  const ordered = [...sources].sort((a, b) =>
    stripDocExtension(a.doc).localeCompare(stripDocExtension(b.doc)),
  );

  const taken = new Set<string>();
  const names = new Map<string, PageName>();

  for (const source of ordered) {
    const docPath = stripDocExtension(source.doc);
    if (!docPath || names.has(docPath)) continue;

    const wanted = pageTitle(source);
    let title = wanted;
    for (let n = 2; taken.has(title.toLowerCase()); n++) {
      title = `${wanted} (${n})`;
    }
    taken.add(title.toLowerCase());
    names.set(docPath, { docPath, title, file: `${title}.html` });
  }

  return names;
}
