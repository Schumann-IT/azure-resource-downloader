.PHONY: help release release-status release-ready-go release-ready-web branch-ready branch-ready-go branch-ready-web \
	branch-ready-report branch-ready-report-go branch-ready-report-web \
	sonarqube-start sonarqube-stop sonarqube-clean sonarqube-preflight \
	sonarqube-analyze sonarqube-analyze-go sonarqube-analyze-web \
	sonarqube-report sonarqube-report-go sonarqube-report-web

# Release entry points for the monorepo. Closing a project's changelog into an
# undated `## [X.Y.Z]` heading (and, for web/, bumping package.json) is done by
# hand and committed first; the per-project targets below only REPORT whether a
# release can be cut, and `make release` stamps the date, commits, tags and
# publishes every project whose newest heading is still undated. The
# repository-wide branch and working-tree checks live in `make release` only.
# See README.md#development-workflow (step 4, Release).
# The `branch-ready*` targets are the ones here that are not part of releasing:
# they report whether a feature or fix branch is ready to be merged.

# Branch releases are cut from; `make release` refuses on any other branch.
RELEASE_BRANCH ?= main
export RELEASE_BRANCH

# Local SonarQube server the analysis targets talk to. Exported so the scanner
# picks it up: the URL is environment-specific and deliberately NOT committed
# into either project's sonar-project.properties.
SONAR_HOST_URL ?= http://localhost:9000
export SONAR_HOST_URL

# Release readiness of go/ (delegates to go/Makefile). Reports only, changes nothing.
release-ready-go:
	@$(MAKE) -C go release-ready

# Release readiness of web/ (delegates to the npm `release-ready` script). Reports only.
release-ready-web:
	@npm --prefix web run release-ready

# Is a feature or fix branch ready to ship? Each project's gate reports only
# (read-only git: the branch, its merge-base with RELEASE_BRANCH, the backlog and
# archive diffs), but unlike the release targets above it exits non-zero if any
# check failed, so it can gate a merge, and it refuses to run while its own
# folder has uncommitted changes. Not release targets and not prerequisites of
# one. The start gate for one entry is per project: `make -C go start-item N=<n>`
# / `npm --prefix web run start-item -- <n>`.
branch-ready-go:
	@$(MAKE) -C go branch-ready

branch-ready-web:
	@npm --prefix web run branch-ready

# Both gates; a branch touching one project only can run just that one.
branch-ready: branch-ready-go branch-ready-web

# The report half of each gate without its pipeline (clean tree + branch
# report, seconds). CI runs the pipelines on every push (ci-go, ci-web) and
# these on the pull request (branch-ready-go, branch-ready-web): together they
# are the merge gate. Locally, closing a branch runs these once CI is green;
# the full gates above stay for offline use.
branch-ready-report-go:
	@$(MAKE) -C go branch-ready-report

branch-ready-report-web:
	@npm --prefix web run branch-ready:report

branch-ready-report: branch-ready-report-go branch-ready-report-web

# Readiness of both projects (prerequisites), then check branch + working tree,
# stamp the date onto every undated version heading, commit, tag, push and
# create the GitHub releases (needs gh).
release: release-ready-go release-ready-web
	@scripts/release.sh publish

# Readiness of both projects, then show which would be published by
# `make release`; changes nothing.
release-status: release-ready-go release-ready-web
	@scripts/release.sh status

# Local SonarQube (server + database), data under ./.sonar. The server needs
# about a minute to start Elasticsearch, and a scan against a server that is
# not yet UP fails, so this waits instead of returning early. The wait polls
# from the host rather than using a container healthcheck so it does not depend
# on which HTTP client the image happens to ship.
sonarqube-start:
	@docker compose -f sonar-compose.yaml --env-file sonar.env up -d
	@printf '⏳ Waiting for SonarQube at $(SONAR_HOST_URL) '
	@for i in $$(seq 1 60); do \
		if curl -fsS $(SONAR_HOST_URL)/api/system/status 2>/dev/null | grep -q '"status":"UP"'; then \
			echo ' ✅ UP'; \
			exit 0; \
		fi; \
		printf '.'; \
		sleep 5; \
	done; \
	echo ' ❌ still not UP after 5 minutes — check: docker logs sonarqube' >&2; \
	exit 1

sonarqube-stop:
	@docker compose -f sonar-compose.yaml --env-file sonar.env down --remove-orphans

# Stop the stack, THEN delete its data. Removing ./.sonar while the containers
# run pulls the database directory out from under a live Postgres.
sonarqube-clean:
	@$(MAKE) --no-print-directory sonarqube-stop
	@rm -rf .sonar .scannerwork go/.scannerwork web/.scannerwork
	@echo "✅ SonarQube data and scanner work directories removed"

# Shared preflight for the analysis targets: the scanner CLI and an analysis
# token. The token is a secret, so it lives in the environment only — never in
# sonar.env (which is committed) or in a properties file.
sonarqube-preflight:
	@command -v sonar-scanner >/dev/null 2>&1 || { \
		echo "❌ sonar-scanner not found — install it with: npm install -g sonarqube-scanner" >&2; \
		exit 1; \
	}
	@if [ -z "$$SONAR_TOKEN" ]; then \
		echo "❌ SONAR_TOKEN is not set. Create an analysis token in the UI" >&2; \
		echo "   ($(SONAR_HOST_URL) → My Account → Security) and export it:" >&2; \
		echo "     export SONAR_TOKEN=<token>" >&2; \
		exit 1; \
	fi
	@curl -fsS $(SONAR_HOST_URL)/api/system/status 2>/dev/null | grep -q '"status":"UP"' || { \
		echo "❌ No SonarQube at $(SONAR_HOST_URL) — run 'make sonarqube-start'" >&2; \
		exit 1; \
	}

# Each project is its own Sonar project, analysed from its own folder: the two
# have different toolchains, coverage formats and release lines, and the scanner
# reads sonar-project.properties from its working directory. Coverage is
# produced first — without the report the analysis silently lands at 0%.
sonarqube-analyze-go: sonarqube-preflight
	@$(MAKE) --no-print-directory -C go test-coverage
	@cd go && sonar-scanner

sonarqube-analyze-web: sonarqube-preflight
	@npm --prefix web test -- --coverage --coverageReporters=lcov --coverageDirectory=coverage
	@cd web && sonar-scanner

# Both projects; a branch touching one project only can run just that one.
sonarqube-analyze: sonarqube-analyze-go sonarqube-analyze-web
	@echo "✅ Analysis complete — results at $(SONAR_HOST_URL)/projects"

# Community Edition cannot export a report, so the findings of the last
# analysis are pulled from the Web API into ./.sonar-reports (raw JSON plus a
# Markdown breakdown grouped by rule). Read-only, and separate from ./.sonar so
# `sonarqube-clean` does not take the reports with the server data.
sonarqube-report-go: sonarqube-preflight
	@scripts/sonar-report.sh azure-rd-go

sonarqube-report-web: sonarqube-preflight
	@scripts/sonar-report.sh azure-rd-web

sonarqube-report: sonarqube-report-go sonarqube-report-web

help:
	@echo "Azure Resource Downloader - monorepo targets:"
	@echo ""
	@echo "  make branch-ready-go    - Gate: is a go/ branch ready to ship? (clean tree, ci, entries archived, backlog touched, changelog written, not on main)"
	@echo "  make branch-ready-web   - Gate: is a web/ branch ready to ship? (same, plus package.json version untouched)"
	@echo "  make branch-ready       - Both gates"
	@echo "  make branch-ready-report-go / -web / branch-ready-report - The gates without their pipelines (what CI runs on the pull request)"
	@echo ""
	@echo "  make release-ready-go   - Report whether go/ is ready to release (changelog closed, no strikeouts)"
	@echo "  make release-ready-web  - Report whether web/ is ready to release (same, plus package.json version)"
	@echo "  make release-status     - Run readiness, then show which projects have an undated version heading"
	@echo "  make release            - Run readiness, check branch + tree, then stamp date + archive versions, commit, tag, push, GitHub release (needs gh)"
	@echo ""
	@echo "  make sonarqube-start    - Start the local SonarQube stack and wait until it is UP"
	@echo "  make sonarqube-stop     - Stop the local SonarQube stack"
	@echo "  make sonarqube-clean    - Stop it, then remove ./.sonar and the scanner work dirs"
	@echo "  make sonarqube-analyze-go  - Coverage + scan go/  (needs SONAR_TOKEN)"
	@echo "  make sonarqube-analyze-web - Coverage + scan web/ (needs SONAR_TOKEN)"
	@echo "  make sonarqube-analyze     - Both scans"
	@echo "  make sonarqube-report      - Download the last analysis' findings to ./.sonar-reports"
	@echo ""
	@echo "  RELEASE_BRANCH=<name>   - Branch releases are cut from (default: main)"
	@echo "  SONAR_HOST_URL=<url>    - SonarQube server (default: http://localhost:9000)"
	@echo "  SONAR_TOKEN=<token>     - Analysis token; required by the analyze targets"
	@echo ""
	@echo "Build, test and lint each project from its own folder: 'make -C go help', 'npm --prefix web run'."
	@echo "Start gate for one backlog entry: 'make -C go start-item N=<n>' / 'npm --prefix web run start-item -- <n>'."

.DEFAULT_GOAL := help
