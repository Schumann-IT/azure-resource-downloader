#!/usr/bin/env bash
# PreToolUse guard for the pipeline agents (.claude/agents/*.md): the one place
# that enforces "agents never commit — git belongs to the main session", plus
# the stage-specific rules. It reads the hook's JSON on stdin, looks at the Bash
# command an agent is about to run, and exits 2 (block, reason on stderr) when
# the command is off-limits. Everything else passes (exit 0), including the
# read-only git the reports and `make build` rely on (status, diff, log, show,
# describe, rev-parse, ls-files).
#
#   agent-bash-guard.sh               git writes, gh, release and gate targets
#   agent-bash-guard.sh --no-lint     … plus lint/fmt/check/ci (implementers)
#   agent-bash-guard.sh --no-baseline … plus the lint baseline rewrites (qa)
#
# Test it from the repository root:
#   printf '{"tool_input":{"command":"git commit -m x"}}' | .claude/hooks/agent-bash-guard.sh; echo $?
set -uo pipefail

no_lint=0
no_baseline=0
for arg in "$@"; do
  case "$arg" in
    --no-lint) no_lint=1 ;;
    --no-baseline) no_baseline=1 ;;
  esac
done

input=$(cat)
cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // empty' 2>/dev/null || true)
[[ -z "$cmd" ]] && exit 0
flat=$(printf '%s' "$cmd" | tr '\n' ' ')

deny() {
  echo "agent-bash-guard: blocked — $1. $2" >&2
  exit 2
}

# git <subcommand>, allowing `git -C dir` / `git -c key=val` before the verb, and
# the verb after `;`, `&&`, `|` or `(`. The read-only verbs are not in the list.
git_verbs='commit|add|push|pull|fetch|merge|rebase|reset|checkout|switch|restore|stash|tag|rm|mv|cherry-pick|revert|clean|worktree|am|apply|init|remote|notes|filter-branch|gc|prune'
if printf '%s' "$flat" | grep -Eq "(^|[;&|( ])git([[:space:]]+-[Cc][[:space:]]*[^[:space:]]+)*[[:space:]]+($git_verbs)([[:space:]]|$)"; then
  deny "git writes are off-limits to agents" "Report what should be committed; the main session commits."
fi
if printf '%s' "$flat" | grep -Eq "(^|[;&|( ])git[[:space:]]+branch[[:space:]]+-"; then
  deny "git branch changes are off-limits to agents" "Report instead."
fi
if printf '%s' "$flat" | grep -Eq "(^|[;&|( ])gh[[:space:]]+(pr|release|repo|api|issue)([[:space:]]|$)"; then
  deny "gh is off-limits to agents" "Pull requests and releases are the user's call."
fi
if printf '%s' "$flat" | grep -Eq "(^|[;&|( ])make([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+(release|release-status|release-ready(-go|-web)?|branch-ready(-go|-web)?)([[:space:]]|$)|npm[^;&|]*[[:space:]](branch-ready|release-ready)([[:space:]]|$)|release\.sh|branch-ready\.(sh|js)|release-ready\.(sh|js)"; then
  deny "release and branch gates are off-limits to agents" "The main session runs the gates after committing."
fi

if [[ "$no_lint" -eq 1 ]]; then
  if printf '%s' "$flat" | grep -Eq "golangci-lint|eslint|gofmt|(^|[;&|( ])make([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+(lint|lint-check|fmt|fmt-check|check|ci)([[:space:]]|$)|npm[^;&|]*[[:space:]]lint(:[a-z]+)?([[:space:]]|$)"; then
    deny "lint and format are not this stage's job" "Finish with build and tests only; QA lints afterwards."
  fi
fi

if [[ "$no_baseline" -eq 1 ]]; then
  if printf '%s' "$flat" | grep -Eq "lint:baseline|--suppress-all|--suppress-rule"; then
    deny "the lint baseline must never grow" "Fix the finding at its site, or silence it there with a reason."
  fi
fi

exit 0
