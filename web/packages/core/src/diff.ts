import type { YamlValue } from './types';

export type ChangeStatus = 'added' | 'removed' | 'changed' | 'unchanged';

/** A node in the structured (key-by-key) diff tree. */
export interface ValueDiff {
  /** Dotted path; array elements use `[i]`. Root is `''`. */
  path: string;
  /** The map key or array index for this node (empty at root). */
  key: string;
  status: ChangeStatus;
  /** Present for scalar/leaf comparisons. */
  left?: YamlValue;
  right?: YamlValue;
  /** Present for objects/arrays. */
  children?: ValueDiff[];
}

export interface DiffOptions {
  /**
   * Field paths or leaf key names to ignore (volatile fields). A value matches
   * if it equals the full dotted path OR the trailing key name. Defaults to
   * none; the API/UI typically passes ['id', 'lastModifiedDateTime'].
   */
  ignoreFields?: string[];
}

export interface DiffSummary {
  added: number;
  removed: number;
  changed: number;
  unchanged: number;
}

function isObject(v: YamlValue): v is { [k: string]: YamlValue } {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

/** Stable, order-independent deep equality for parsed YAML values. */
export function deepEqual(a: YamlValue, b: YamlValue): boolean {
  if (a === b) return true;
  if (a === null || b === null) return a === b;
  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false;
    return a.every((v, i) => deepEqual(v, b[i]));
  }
  if (isObject(a) && isObject(b)) {
    const ak = Object.keys(a).sort();
    const bk = Object.keys(b).sort();
    if (ak.length !== bk.length) return false;
    if (!ak.every((k, i) => k === bk[i])) return false;
    return ak.every((k) => deepEqual(a[k], b[k]));
  }
  return false;
}

function shouldIgnore(path: string, key: string, opts: DiffOptions): boolean {
  const set = opts.ignoreFields;
  if (!set || set.length === 0) return false;
  return set.includes(path) || set.includes(key);
}

function rollup(children: ValueDiff[]): ChangeStatus {
  return children.every((c) => c.status === 'unchanged') ? 'unchanged' : 'changed';
}

function diffNode(
  path: string,
  key: string,
  left: YamlValue,
  right: YamlValue,
  opts: DiffOptions,
): ValueDiff {
  if (isObject(left) && isObject(right)) {
    const children: ValueDiff[] = [];
    const keys = Array.from(
      new Set([...Object.keys(left), ...Object.keys(right)]),
    ).sort((a, b) => a.localeCompare(b));
    for (const k of keys) {
      const childPath = path ? `${path}.${k}` : k;
      if (shouldIgnore(childPath, k, opts)) continue;
      const inL = k in left;
      const inR = k in right;
      if (inL && !inR) {
        children.push({ path: childPath, key: k, status: 'removed', left: left[k] });
      } else if (!inL && inR) {
        children.push({ path: childPath, key: k, status: 'added', right: right[k] });
      } else {
        children.push(diffNode(childPath, k, left[k], right[k], opts));
      }
    }
    return { path, key, status: rollup(children), children };
  }

  if (Array.isArray(left) && Array.isArray(right)) {
    const children: ValueDiff[] = [];
    const len = Math.max(left.length, right.length);
    for (let i = 0; i < len; i++) {
      const childPath = `${path}[${i}]`;
      const inL = i < left.length;
      const inR = i < right.length;
      if (inL && !inR) {
        children.push({ path: childPath, key: String(i), status: 'removed', left: left[i] });
      } else if (!inL && inR) {
        children.push({ path: childPath, key: String(i), status: 'added', right: right[i] });
      } else {
        children.push(diffNode(childPath, String(i), left[i], right[i], opts));
      }
    }
    return { path, key, status: rollup(children), children };
  }

  const equal = deepEqual(left, right);
  return {
    path,
    key,
    status: equal ? 'unchanged' : 'changed',
    left,
    right,
  };
}

/** Structured, semantic diff of two parsed YAML documents. */
export function diffValues(
  left: YamlValue,
  right: YamlValue,
  opts: DiffOptions = {},
): ValueDiff {
  return diffNode('', '', left, right, opts);
}

/** Count leaf changes by status across a diff tree. */
export function summarizeValueDiff(node: ValueDiff): DiffSummary {
  const summary: DiffSummary = { added: 0, removed: 0, changed: 0, unchanged: 0 };
  const visit = (n: ValueDiff) => {
    if (n.children && n.children.length > 0) {
      n.children.forEach(visit);
    } else if (n.path !== '') {
      summary[n.status] += 1;
    }
  };
  visit(node);
  return summary;
}

/** One entry in a file-set (folder) comparison. */
export interface FileDiffEntry {
  slug: string;
  status: ChangeStatus;
  /** Per-field counts, present only when status is 'changed'. */
  changes?: DiffSummary;
}

export interface FileSetDiff {
  entries: FileDiffEntry[];
  summary: DiffSummary;
}

/**
 * Compare two maps of slug -> parsed YAML. Surfaces added (right-only),
 * removed (left-only), changed, and unchanged files. Pure: callers build the
 * maps from disk.
 */
export function diffFileSets(
  left: Record<string, YamlValue>,
  right: Record<string, YamlValue>,
  opts: DiffOptions = {},
): FileSetDiff {
  const slugs = Array.from(
    new Set([...Object.keys(left), ...Object.keys(right)]),
  ).sort((a, b) => a.localeCompare(b));

  const entries: FileDiffEntry[] = [];
  const summary: DiffSummary = { added: 0, removed: 0, changed: 0, unchanged: 0 };

  for (const slug of slugs) {
    const inL = slug in left;
    const inR = slug in right;
    if (inL && !inR) {
      entries.push({ slug, status: 'removed' });
      summary.removed += 1;
    } else if (!inL && inR) {
      entries.push({ slug, status: 'added' });
      summary.added += 1;
    } else {
      const d = diffValues(left[slug], right[slug], opts);
      if (d.status === 'unchanged') {
        entries.push({ slug, status: 'unchanged' });
        summary.unchanged += 1;
      } else {
        entries.push({ slug, status: 'changed', changes: summarizeValueDiff(d) });
        summary.changed += 1;
      }
    }
  }

  return { entries, summary };
}
