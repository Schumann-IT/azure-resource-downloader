#!/usr/bin/env bash
#
# Download the open findings of one Sonar project into a local report.
#
# SonarQube Community Edition has no report export, so this reads the Web API
# and writes two files per project: the raw JSON (so nothing is lost) and a
# Markdown breakdown grouped by rule, which is the view needed to decide
# whether a finding class should become a lint rule in the project's own
# toolchain or be fixed case by case.
#
# Usage: scripts/sonar-report.sh <projectKey> [outputDir]
#   SONAR_HOST_URL       server (default http://localhost:9000)
#   SONAR_REPORT_TOKEN   token used for reading; falls back to SONAR_TOKEN
#
# Reading needs a USER token (squ_…): an analysis token (sqa_…/sqp_…) carries
# 'Execute Analysis' only, so it authenticates and can list issues but is
# refused on project metadata and security hotspots. Those two are therefore
# optional here — a report is still produced, with the gaps stated in it.
#
# Read-only: it performs GET requests only and touches nothing in the export or
# in either project's source tree.

set -euo pipefail

PROJECT_KEY=${1:-}
OUT_DIR=${2:-.sonar-reports}
SONAR_HOST_URL=${SONAR_HOST_URL:-http://localhost:9000}
PAGE_SIZE=500
# The API refuses p*ps beyond 10000 results; stop before it errors out.
MAX_RESULTS=10000

if [ -z "$PROJECT_KEY" ]; then
	echo "usage: scripts/sonar-report.sh <projectKey> [outputDir]" >&2
	exit 2
fi

for tool in curl jq; do
	command -v "$tool" >/dev/null 2>&1 || {
		echo "❌ $tool is required" >&2
		exit 1
	}
done

TOKEN=${SONAR_REPORT_TOKEN:-${SONAR_TOKEN:-}}
if [ -z "$TOKEN" ]; then
	echo "❌ No token — create one at $SONAR_HOST_URL (My Account → Security) and export SONAR_REPORT_TOKEN" >&2
	exit 1
fi

case "$TOKEN" in
sqa_* | sqp_*)
	echo "⚠️  This is an ANALYSIS token, which grants 'Execute Analysis' only:" >&2
	echo "   project metadata and security hotspots will be refused (403) and left" >&2
	echo "   out of the report. For a complete report create a User Token at" >&2
	echo "   $SONAR_HOST_URL (My Account → Security → type 'User Token')" >&2
	echo "   and export it as SONAR_REPORT_TOKEN." >&2
	;;
esac

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

api() {
	curl -fsS -u "$TOKEN:" "$SONAR_HOST_URL/api/$1"
}

# Fetch every page of a paginated endpoint into $TMP_DIR/<prefix>-<n>.json.
# `total` is read from the response so the loop stops without a second call.
fetch_pages() {
	local prefix=$1 path=$2 page=1 fetched=0 total got
	while :; do
		api "$path&ps=$PAGE_SIZE&p=$page" >"$TMP_DIR/$prefix-$page.json" || return 1
		total=$(jq -r '.total // (.paging.total // 0)' "$TMP_DIR/$prefix-$page.json")
		got=$(jq -r "(.$prefix | length)" "$TMP_DIR/$prefix-$page.json")
		fetched=$((fetched + got))
		if [ "$got" -eq 0 ] || [ "$fetched" -ge "$total" ]; then
			break
		fi
		page=$((page + 1))
		if [ $((page * PAGE_SIZE)) -gt "$MAX_RESULTS" ]; then
			echo "⚠️  more than $MAX_RESULTS $prefix — report truncated; narrow it with a filter" >&2
			break
		fi
	done
}

echo "⬇️  Downloading findings for $PROJECT_KEY from $SONAR_HOST_URL"

# Cosmetic only, and refused by analysis tokens: fall back to the key.
PROJECT_NAME=$(api "components/show?component=$PROJECT_KEY" 2>/dev/null | jq -r '.component.name' || true)
case "$PROJECT_NAME" in
"" | null) PROJECT_NAME=$PROJECT_KEY ;;
esac

# The issues are the report; without them there is nothing to write.
# additionalFields=rules is required: without it the response carries rule keys
# but no rule names, and the report degrades to a list of `go:S1234`.
fetch_pages issues "issues/search?projects=$PROJECT_KEY&resolved=false&additionalFields=rules" || {
	echo "❌ Could not read the issues of $PROJECT_KEY — the token needs 'Browse' on the project" >&2
	exit 1
}

# Hotspots are a separate permission, so a token that can list issues may still
# be refused here. Report the gap rather than failing the whole run.
HOTSPOTS_READABLE=true
if ! fetch_pages hotspots "hotspots/search?projectKey=$PROJECT_KEY&status=TO_REVIEW" 2>/dev/null; then
	HOTSPOTS_READABLE=false
	rm -f "$TMP_DIR"/hotspots-*.json
	echo '{"hotspots":[],"paging":{"total":0}}' >"$TMP_DIR/hotspots-1.json"
	echo "⚠️  Security hotspots not readable with this token — omitted from the report" >&2
fi

mkdir -p "$OUT_DIR"
JSON_OUT="$OUT_DIR/$PROJECT_KEY-findings.json"
MD_OUT="$OUT_DIR/$PROJECT_KEY-findings.md"

# One document per project: the issues with their rule metadata, plus the
# hotspots. `component` keys are shortened to repository-relative paths so the
# report can be read (and grepped) next to the code.
jq -s \
	--arg key "$PROJECT_KEY" \
	--arg name "$PROJECT_NAME" \
	--arg host "$SONAR_HOST_URL" \
	--arg generated "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	--argjson hotspotsReadable "$HOTSPOTS_READABLE" \
	'{
		project: $key,
		projectName: $name,
		server: $host,
		generatedAt: $generated,
		hotspotsReadable: $hotspotsReadable,
		issues: (map(.issues // []) | add | map(. + {file: (.component | sub("^\($key):"; ""))})),
		rules: (map(.rules // []) | add | unique_by(.key)),
		hotspots: (map(.hotspots // []) | add | map(. + {file: (.component | sub("^\($key):"; ""))}))
	}' "$TMP_DIR"/issues-*.json "$TMP_DIR"/hotspots-*.json >"$JSON_OUT"

jq -r '
	def impacts: if (.impacts // []) | length > 0
		then (.impacts | map("\(.softwareQuality) \(.severity)") | join(", "))
		else (.severity // "—") end;
	def ruleName($k): (.rules // []) | map(select(.key == $k)) | (.[0].name // $k);
	# Issues raised on a file or module as a whole carry no line.
	def at: "\(.file)\(if .line then ":\(.line)" else "" end)";

	. as $r
	| [
		"# Sonar findings — \($r.projectName) (`\($r.project)`)",
		"",
		"Downloaded \($r.generatedAt) from \($r.server). **\($r.issues | length) open issues**, \(if $r.hotspotsReadable then "**\($r.hotspots | length) security hotspots to review**" else "hotspots not readable with the token used" end).",
		"",
		"Regenerate with `make sonarqube-report`. Browse a finding at \($r.server)/project/issues?id=\($r.project)&open=<issueKey>.",
		"",
		"## Open issues by rule",
		"",
		"Highest count first — the top rows are the ones worth encoding as a lint rule rather than fixing one by one.",
		"",
		"| Count | Rule | Name | Impact |",
		"| ----: | ---- | ---- | ------ |"
	]
	+ ($r.issues
		| group_by(.rule)
		| sort_by(-length)
		| map(.[0].rule as $rk
			| "| \(length) | `\($rk)` | \($r | ruleName($rk)) | \(.[0] | impacts) |"))
	+ [
		"",
		"## Open issues by file",
		"",
		"| Count | File |",
		"| ----: | ---- |"
	]
	+ ($r.issues
		| group_by(.file)
		| sort_by(-length)
		| map("| \(length) | `\(.[0].file)` |"))
	+ [
		"",
		"## Open issues in detail",
		""
	]
	+ ($r.issues
		| group_by(.rule)
		| sort_by(-length)
		| map(.[0].rule as $rk
			| ["### `\($rk)` — \($r | ruleName($rk)) (\(length))", ""]
			+ (sort_by(.file, (.line // 0))
				| map("- `\(at)` — \(.message | gsub("\n"; " "))"))
			+ [""]
		)
		| add // [])
	+ (if $r.hotspotsReadable | not then
		[
			"## Security hotspots to review",
			"",
			"_Not included: the token used grants Execute Analysis only. Re-run with a User Token (`squ_…`) exported as `SONAR_REPORT_TOKEN`, or review them at \($r.server)/project/security_hotspots?id=\($r.project)._"
		]
	elif ($r.hotspots | length) > 0 then
		[
			"## Security hotspots to review",
			"",
			"| Location | Category | Probability | Message |",
			"| -------- | -------- | ----------- | ------- |"
		]
		+ ($r.hotspots
			| sort_by(.file, (.line // 0))
			| map("| `\(at)` | \(.securityCategory // "—") | \(.vulnerabilityProbability // "—") | \(.message | gsub("\n"; " ")) |"))
	else [] end)
	| join("\n")
' "$JSON_OUT" >"$MD_OUT"

echo "✅ $MD_OUT"
echo "   $JSON_OUT"
