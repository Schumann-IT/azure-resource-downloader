import { diffLines } from 'diff';

// Unchanged lines kept around each change, as in a unified diff.
export const DIFF_CONTEXT = 3;

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

export interface DiffHunk {
  header: string;
  lines: DiffLine[];
}

export interface YamlDiff {
  hunks: DiffHunk[];
  added: number;
  removed: number;
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
    return { hunks: [], added: 0, removed: 0, tooLarge: true };
  }
  const lines = toLines(baseline, observed);
  return {
    hunks: toHunks(lines, context),
    added: lines.filter((l) => l.added).length,
    removed: lines.filter((l) => l.removed).length,
    tooLarge: false,
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
// lines longer than twice the context are collapsed between hunks.
function toHunks(lines: DiffLine[], context: number): DiffHunk[] {
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
  return groups.map((group) => ({ header: hunkHeader(group), lines: group }));
}

function hunkHeader(lines: DiffLine[]): string {
  const range = (numbers: number[]): string =>
    `${numbers.length > 0 ? numbers[0] : 0},${numbers.length}`;
  const oldNos = lines.map((l) => l.oldNo).filter((n): n is number => n !== null);
  const newNos = lines.map((l) => l.newNo).filter((n): n is number => n !== null);
  return `@@ -${range(oldNos)} +${range(newNos)} @@`;
}
