#!/usr/bin/env bash
# SessionStart hook: tell the session up front whether GitHub work is possible,
# so a request for a pull request or a release fails fast instead of half-way.
# Prints one line; never fails the session.
if ! command -v gh >/dev/null 2>&1; then
  echo "gh: not installed — pull requests and releases (/pull-request, /release) are unavailable; install the GitHub CLI and run 'gh auth login'"
  exit 0
fi
if gh auth status >/dev/null 2>&1; then
  user=$(gh api user --jq .login 2>/dev/null || echo "unknown user")
  echo "gh: authenticated as $user — pull requests and releases are possible"
else
  echo "gh: not logged in — pull requests and releases (/pull-request, /release) will fail; run 'gh auth login' first"
fi
exit 0
