---
name: archive
description: Read the archived backlog entries under .claude/archive/ — list them, show one, or search their text. Use when the user asks what was planned or how something was built, or before reworking an area a past entry covered.
---

# Read the plan archive

Argument: `$ARGUMENTS` = `list [go|web]` | `show <slug or part of a title>` | `search <text>`. Read-only:
this skill never edits or deletes an archive file.

- **list** — for each `.claude/archive/<project>/*.md`: finished date, status, title, `changelog:` version,
  from the frontmatter (`head -12`). Newest first. Do not read the bodies.
- **show** — print the matching file whole (frontmatter and body). Ask when several match.
- **search** — `grep -ril` across the archive, then the matching lines with two lines of context; offer
  `show` for a hit.

The archive is history, not instruction: it explains why something was built the way it was, and it may
describe code that has since changed. Verify against the current code before relying on a detail.
