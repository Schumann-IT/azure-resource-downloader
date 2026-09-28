import { diffChars, diffLines } from 'diff';

// Unchanged lines kept around each change, as in a unified diff.
export const DIFF_CONTEXT = 3;

// The context that keeps every line: the whole file as one hunk, the way an
// IDE compares two files. Same computation, unbounded context.
export const WHOLE_FILE = Number.POSITIVE_INFINITY;

// Above this many diff lines a whole-file request is served as hunks instead:
// a 3,000-row two-pane table is already one to two megabytes of HTML.
export const MAX_FULL_LINES = 3000;

// A modified row gets character parts only when at most this share of the
// longer line changed. `pairRows` zips runs by position, so some modified rows
// pair two unrelated lines, and a character diff of those is confetti.
export const MAX_PART_RATIO = 0.5;

// Lines longer than this skip the character pass, for the same reason and to
// keep its cost bounded.
export const MAX_PART_LINE = 500;

// Above this combined size the diff is not computed: a line diff of
// multi-megabyte payloads would stall the request for no real benefit, the
// same trade-off the highlighter's size cap makes.
export const MAX_DIFF_BYTES = 1024 * 1024;

// One line of the diff. Exactly one of the numbers is null for an added or a
// removed line; both are set for a context line.
export interface DiffLine {
  text: string;
  added: boolean;
  removed: boolean;
  oldNo: number | null;
  newNo: number | null;
}

// One visual row of the side-by-side layout: a context line on both sides, or
// a removed line beside the added line it pairs with. Either side is null on
// the overhang of an uneven change.
export interface PairedRow {
  context: boolean;
  left: DiffLine | null;
  right: DiffLine | null;
}

// A stretch of one side of a modified row: equal to the other side, or not.
// Plain data, escaped by the template like every other diff value.
export interface DiffPart {
  text: string;
  changed: boolean;
}

// The link from the first row of one difference to the next one (the last
// wraps to the first), so a reader can walk the differences without a script.
export interface NextChange {
  n: number;
  label: string;
  first: boolean;
}

// A paired row as the partial renders it. `modified` marks a removed line
// beside the added line it pairs with; its parts say which characters differ,
// or are null when too much differs to be worth marking. `change` numbers the
// difference a row opens.
export interface DiffRow extends PairedRow {
  modified: boolean;
  leftParts: DiffPart[] | null;
  rightParts: DiffPart[] | null;
  change: number | null;
  next: NextChange | null;
}

export interface DiffHunk {
  header: string;
  lines: DiffLine[];
  rows: DiffRow[];
}

export interface YamlDiff {
  hunks: DiffHunk[];
  added: number;
  removed: number;
  // Blocks of adjacent changed rows — what a reader counts as a difference.
  changes: number;
  changesLabel: string;
  // True when the one hunk is the whole file, so its header is noise.
  wholeFile: boolean;
  // True when the whole file was asked for but served as hunks, over the cap.
  capped: boolean;
  tooLarge: boolean;
}

// A unified line diff of the baseline against the observed payload, as plain
// data for the template to escape — no HTML is produced here, so nothing from
// either file can reach the page unescaped. Pure and Nest-free.
export function diffYaml(
  baseline: string,
  observed: string,
  context = DIFF_CONTEXT,
): YamlDiff {
  if (
    Buffer.byteLength(baseline) + Buffer.byteLength(observed) >
    MAX_DIFF_BYTES
  ) {
    return emptyDiff(true);
  }
  const lines = toLines(baseline, observed);
  const whole = context === WHOLE_FILE;
  const capped = whole && lines.length > MAX_FULL_LINES;
  const hunks = toHunks(lines, capped ? DIFF_CONTEXT : context);
  const changes = numberChanges(hunks);
  return {
    hunks,
    added: lines.filter((l) => l.added).length,
    removed: lines.filter((l) => l.removed).length,
    changes,
    changesLabel: `${changes} difference${changes === 1 ? '' : 's'}`,
    wholeFile: whole && !capped,
    capped,
    tooLarge: false,
  };
}

function emptyDiff(tooLarge: boolean): YamlDiff {
  return {
    hunks: [],
    added: 0,
    removed: 0,
    changes: 0,
    changesLabel: '0 differences',
    wholeFile: false,
    capped: false,
    tooLarge,
  };
}

function toLines(baseline: string, observed: string): DiffLine[] {
  const out: DiffLine[] = [];
  let oldNo = 1;
  let newNo = 1;
  for (const change of diffLines(baseline, observed)) {
    for (const text of splitLines(change.value)) {
      const added = !!change.added;
      const removed = !!change.removed;
      out.push({
        text,
        added,
        removed,
        oldNo: added ? null : oldNo++,
        newNo: removed ? null : newNo++,
      });
    }
  }
  return out;
}

function splitLines(value: string): string[] {
  const parts = value.split('\n');
  if (parts[parts.length - 1] === '') parts.pop();
  return parts;
}

// Groups the changed lines with their context into hunks; runs of unchanged
// lines longer than twice the context are collapsed between hunks. Unbounded
// context keeps every line in one hunk — or none, when nothing changed.
function toHunks(lines: DiffLine[], context: number): DiffHunk[] {
  if (!Number.isFinite(context)) {
    return lines.some((l) => l.added || l.removed) ? [toHunk(lines)] : [];
  }
  const keep = new Array<boolean>(lines.length).fill(false);
  lines.forEach((line, i) => {
    if (!line.added && !line.removed) return;
    const end = Math.min(lines.length - 1, i + context);
    for (let j = Math.max(0, i - context); j <= end; j++) keep[j] = true;
  });

  const groups: DiffLine[][] = [];
  let current: DiffLine[] | null = null;
  lines.forEach((line, i) => {
    if (!keep[i]) {
      current = null;
      return;
    }
    if (!current) {
      current = [];
      groups.push(current);
    }
    current.push(line);
  });
  return groups.map(toHunk);
}

function toHunk(group: DiffLine[]): DiffHunk {
  return {
    header: hunkHeader(group),
    lines: group,
    rows: pairRows(group).map(markRow),
  };
}

// A paired row with what the partial needs to mark it: whether it is a
// modification, and if so which characters differ on each side.
function markRow(row: PairedRow): DiffRow {
  const modified = !row.context && row.left !== null && row.right !== null;
  const parts = modified && row.left && row.right ? charParts(row.left.text, row.right.text) : null;
  return {
    ...row,
    modified,
    leftParts: parts ? parts.left : null,
    rightParts: parts ? parts.right : null,
    change: null,
    next: null,
  };
}

// The two sides of a modified line split into equal and changed stretches, or
// null when either line is over the length cap or more than `MAX_PART_RATIO` of
// the longer line changed — the whole row then reads as changed. Pure.
export function charParts(
  left: string,
  right: string,
): { left: DiffPart[]; right: DiffPart[] } | null {
  if (left.length > MAX_PART_LINE || right.length > MAX_PART_LINE) return null;
  const longer = Math.max(left.length, right.length);
  if (longer === 0) return null;
  const out = { left: [] as DiffPart[], right: [] as DiffPart[] };
  let removed = 0;
  let added = 0;
  for (const change of diffChars(left, right)) {
    if (change.added) {
      out.right.push({ text: change.value, changed: true });
      added += change.value.length;
    } else if (change.removed) {
      out.left.push({ text: change.value, changed: true });
      removed += change.value.length;
    } else {
      out.left.push({ text: change.value, changed: false });
      out.right.push({ text: change.value, changed: false });
    }
  }
  return Math.max(removed, added) / longer > MAX_PART_RATIO ? null : out;
}

// Numbers the differences — each run of adjacent changed rows — across all
// hunks, and links each one's first row to the next, the last back to the
// first. A single difference gets no link: it would only point at itself.
// Returns the count.
function numberChanges(hunks: DiffHunk[]): number {
  const starts: DiffRow[] = [];
  for (const hunk of hunks) {
    hunk.rows.forEach((row, i) => {
      if (row.context) return;
      if (i > 0 && !hunk.rows[i - 1].context) return;
      starts.push(row);
      row.change = starts.length;
    });
  }
  if (starts.length > 1) {
    starts.forEach((row, i) => {
      const last = i === starts.length - 1;
      row.next = last
        ? { n: 1, label: 'first difference', first: true }
        : { n: i + 2, label: 'next difference', first: false };
    });
  }
  return starts.length;
}

// Pairs a hunk's lines for the side-by-side layout, so a modified value is one
// row rather than a removed row followed by an added one: each run of removed
// or added lines is zipped against the run of the other kind that immediately
// follows it, whichever comes first. Pure; the flat lines stay the unified
// view's input.
export function pairRows(lines: DiffLine[]): PairedRow[] {
  const rows: PairedRow[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.added && !line.removed) {
      rows.push({ context: true, left: line, right: line });
      i++;
      continue;
    }
    const first = takeRun(lines, i, line.added);
    i += first.length;
    const second =
      i < lines.length && (lines[i].added || lines[i].removed)
        ? takeRun(lines, i, lines[i].added)
        : [];
    i += second.length;
    const removed = line.removed ? first : second;
    const added = line.added ? first : second;
    for (let k = 0; k < Math.max(removed.length, added.length); k++) {
      rows.push({
        context: false,
        left: removed[k] ?? null,
        right: added[k] ?? null,
      });
    }
  }
  return rows;
}

function takeRun(lines: DiffLine[], start: number, added: boolean): DiffLine[] {
  let end = start;
  while (
    end < lines.length &&
    (added ? lines[end].added : lines[end].removed)
  ) {
    end++;
  }
  return lines.slice(start, end);
}

function hunkHeader(lines: DiffLine[]): string {
  const range = (numbers: number[]): string =>
    `${numbers.length > 0 ? numbers[0] : 0},${numbers.length}`;
  const oldNos = lines.map((l) => l.oldNo).filter((n): n is number => n !== null);
  const newNos = lines.map((l) => l.newNo).filter((n): n is number => n !== null);
  return `@@ -${range(oldNos)} +${range(newNos)} @@`;
}
