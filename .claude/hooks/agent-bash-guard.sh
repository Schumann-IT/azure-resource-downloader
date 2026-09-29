#!/usr/bin/env bash
# PreToolUse guard for the pipeline agents (.claude/agents/*.md): the one place
# that enforces "agents never commit — git belongs to the main session", plus
# the stage-specific rules. It reads the hook's JSON on stdin, looks at the Bash
# command an agent is about to run, and exits 2 (block, reason on stderr) when
# the command is off-limits. Everything else passes (exit 0), including the
# read-only git the reports and `make build` rely on (status, diff, log, show,
# describe, rev-parse, ls-files).
#
#   agent-bash-guard.sh                        git writes, gh, release and gate targets
#   agent-bash-guard.sh --no-lint              … plus lint/fmt/check/ci (implementers)
#   agent-bash-guard.sh --no-baseline          … plus the lint baseline rewrites (qa)
#   agent-bash-guard.sh --allow-backlog-commit … but `git add <…NEXT-ITERATIONS.md>` and a plain
#                                              `git commit` are allowed (plan-reviewer)
#
# Commands joined with `&&`, `||`, `;` or `|` are judged segment by segment.
# Test it from the repository root:
#   printf '{"tool_input":{"command":"git commit -m x"}}' | .claude/hooks/agent-bash-guard.sh; echo $?
set -uo pipefail

no_lint=0
no_baseline=0
allow_backlog=0
for arg in "$@"; do
  case "$arg" in
    --no-lint) no_lint=1 ;;
    --no-baseline) no_baseline=1 ;;
    --allow-backlog-commit) allow_backlog=1 ;;
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

# The verb of a `git …` segment, allowing `git -C dir` / `git -c key=val` first.
git_verb() {
  printf '%s' "$1" | sed -E 's/^[[:space:]]*git([[:space:]]+-[Cc][[:space:]]*[^[:space:]]+)*[[:space:]]+([a-z-]+).*/\2/'
}

# A backlog-only `git add`: every argument is a path ending in NEXT-ITERATIONS.md.
backlog_add_ok() {
  local seg="$1" tok
  for tok in $(printf '%s' "$seg" | sed -E 's/^[[:space:]]*git[[:space:]]+add[[:space:]]*//'); do
    [[ "$tok" == *NEXT-ITERATIONS.md ]] || return 1
  done
  return 0
}

# A plain `git commit`: no -a/--all/-i/--include/--amend (also inside combined
# flags such as -am), and no pathspec other than NEXT-ITERATIONS.md.
backlog_commit_ok() {
  local seg="$1" tok after_dashes=0 skip_next=0 stripped
  # Quoted strings are messages, never paths: drop them before tokenising.
  stripped=$(printf '%s' "$seg" | sed -E "s/'[^']*'//g; s/\"[^\"]*\"//g; s/^[[:space:]]*git[[:space:]]+commit[[:space:]]*//")
  for tok in $stripped; do
    if [[ "$skip_next" -eq 1 && "$tok" != -* ]]; then
      skip_next=0
      continue
    fi
    skip_next=0
    if [[ "$after_dashes" -eq 1 ]]; then
      [[ "$tok" == *NEXT-ITERATIONS.md ]] || return 1
      continue
    fi
    case "$tok" in
      --) after_dashes=1 ;;
      --all|--include|--amend|--interactive|--patch) return 1 ;;
      -m|-F|--message|--file) skip_next=1 ;;
      --message=*|--file=*) ;;
      --*) ;;
      -*[aip]*) return 1 ;;
      -*) ;;
      *) return 1 ;;   # a bare pathspec before `--`: only NEXT-ITERATIONS.md may be committed
    esac
  done
  return 0
}

git_verbs='commit|add|push|pull|fetch|merge|rebase|reset|checkout|switch|restore|stash|tag|rm|mv|cherry-pick|revert|clean|worktree|am|apply|init|remote|notes|filter-branch|gc|prune'

# Judge each command segment on its own.
while IFS= read -r seg; do
  [[ -z "${seg// }" ]] && continue
  s=" $seg"
  if printf '%s' "$s" | grep -Eq "(^|[( ])git([[:space:]]+-[Cc][[:space:]]*[^[:space:]]+)*[[:space:]]+($git_verbs)([[:space:]]|$)"; then
    verb=$(git_verb "$seg")
    if [[ "$allow_backlog" -eq 1 && "$verb" == "add" ]] && backlog_add_ok "$seg"; then :
    elif [[ "$allow_backlog" -eq 1 && "$verb" == "commit" ]] && backlog_commit_ok "$seg"; then :
    elif [[ "$allow_backlog" -eq 1 && ( "$verb" == "add" || "$verb" == "commit" ) ]]; then
      deny "only NEXT-ITERATIONS.md may be added and committed by this agent (plain 'git commit -m …', no -a)" "Stage and commit the backlog file only."
    else
      deny "git writes are off-limits to agents" "Report what should be committed; the main session commits."
    fi
  fi
  if printf '%s' "$s" | grep -Eq "(^|[( ])git[[:space:]]+branch[[:space:]]+-"; then
    deny "git branch changes are off-limits to agents" "Report instead."
  fi
  if printf '%s' "$s" | grep -Eq "(^|[( ])gh[[:space:]]+(pr|release|repo|api|issue)([[:space:]]|$)"; then
    deny "gh is off-limits to agents" "Pull requests and releases are the user's call."
  fi
  if printf '%s' "$s" | grep -Eq "(^|[( ])make([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+(release|release-status|release-ready(-go|-web)?|branch-ready(-report)?(-go|-web)?)([[:space:]]|$)|npm[^;&|]*[[:space:]](branch-ready(:report)?|release-ready)([[:space:]]|$)|release\.sh|branch-ready\.(sh|js)|release-ready\.(sh|js)"; then
    deny "release and branch gates are off-limits to agents" "The main session runs the gates after committing."
  fi
  if [[ "$no_lint" -eq 1 ]] && printf '%s' "$s" | grep -Eq "golangci-lint|eslint|gofmt|(^|[( ])make([[:space:]]+-C[[:space:]]+[^[:space:]]+)?[[:space:]]+(lint|lint-check|fmt|fmt-check|check|ci)([[:space:]]|$)|npm[^;&|]*[[:space:]]lint(:[a-z]+)?([[:space:]]|$)"; then
    deny "lint and format are not this stage's job" "Finish with build and tests only; QA lints afterwards."
  fi
  if [[ "$no_baseline" -eq 1 ]] && printf '%s' "$s" | grep -Eq "lint:baseline|--suppress-all|--suppress-rule"; then
    deny "the lint baseline must never grow" "Fix the finding at its site, or silence it there with a reason."
  fi
done < <(printf '%s\n' "$flat" | sed -E 's/&&|\|\||;|\|/\n/g')

exit 0
