'use strict';
// The one place web/'s tooling shells out. `git` is resolved from PATH on
// purpose: there is no fixed install location across platforms, every argument
// list is fixed by the callers (never user input), and every call is read-only
// (`status`, `rev-parse`, `merge-base`, `diff`, `show`). A missing git surfaces
// as an ENOENT error so callers can report a skip instead of a failure.

const { spawnSync } = require('child_process');

// Runs git with a fixed argument list. Never throws on a non-zero exit — callers
// read `status` — and throws only when git itself cannot be started.
function git(args, { cwd }) {
  // eslint-disable-next-line sonarjs/no-os-command-from-path -- fixed read-only argument lists; git is located via PATH by design (see the file comment)
  const r = spawnSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  if (r.error) throw r.error;
  return { status: r.status ?? 1, stdout: r.stdout ?? '', stderr: r.stderr ?? '' };
}

// Is `cwd` inside a git work tree? False when git is missing.
function isRepo(cwd) {
  try {
    return git(['rev-parse', '--is-inside-work-tree'], { cwd }).status === 0;
  } catch {
    return false;
  }
}

// The checked-out branch, or 'HEAD' when detached.
function currentBranch(cwd) {
  const r = git(['rev-parse', '--abbrev-ref', 'HEAD'], { cwd });
  return r.status === 0 ? r.stdout.trim() : 'HEAD';
}

// Where HEAD left the release branch: the merge-base with `rb`, else with
// `origin/<rb>`, else null.
function mergeBase(cwd, rb) {
  for (const ref of [rb, `origin/${rb}`]) {
    const r = git(['merge-base', 'HEAD', ref], { cwd });
    if (r.status === 0 && r.stdout.trim() !== '') return r.stdout.trim();
  }
  return null;
}

// Uncommitted changes under `cwd` (porcelain lines), scoped to that folder so an
// edit elsewhere in the repository cannot block this project's gate.
function dirtyFiles(cwd) {
  const r = git(['status', '--porcelain', '--', cwd], { cwd });
  return r.stdout.split('\n').filter((line) => line.trim() !== '');
}

// A file's content at `rev` (`./relPath` is resolved against `cwd`), or null
// when the revision or the path does not exist.
function showFile(cwd, rev, relPath) {
  const r = git(['show', `${rev}:./${relPath}`], { cwd });
  return r.status === 0 ? r.stdout : null;
}

module.exports = { git, isRepo, currentBranch, mergeBase, dirtyFiles, showFile };
