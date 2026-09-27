// The single lint truth for this project: `npm run lint` runs ESLint with this
// file, and WebStorm's default "Automatic ESLint configuration" runs the same
// ESLint from node_modules against the same file, so an editor squiggle and a
// `npm run lint` finding are the same thing. The IDE's own bundled TypeScript
// inspections are a separate engine that cannot be expressed here; where the two
// overlap (unused symbols, unreachable code, `require` in an ES module) the rule
// sets below cover it, and anything only the bundled inspections report stays an
// editor hint rather than a merge gate.
//
// The second source this file mirrors is the SonarQube quality profile the
// project is analysed with (`make sonarqube-analyze` at the repository root).
// eslint-plugin-sonarjs is generated from the same analyzer the server runs, so
// for the rules the profile has active it reports locally what the server would
// report, and a finding no longer has to wait for a scan to be seen.
//
// The overlap is not total, and the gap runs one way. Measured against a full
// analysis, every sonarjs rule that maps to a Sonar ISSUE agreed on both the
// count and the line — S3776, S5906, S1994, S2310, S4624, S4036. The ones that
// map to a Sonar SECURITY HOTSPOT did not: the server reports hotspots
// separately from issues and this project has none to review, so those rules
// fire here and nowhere else. `make sonarqube-report` downloads both, and its
// `hotspotsReadable` field says whether the hotspot half was actually returned
// — a zero with `hotspotsReadable: false` would mean "not visible", not "none".
//
// That makes them ESLint-only signals, kept deliberately rather than switched
// off to force parity (the mirror image of the Go config dropping goconst
// because it could not agree with S1192). Two are in this state today:
//
//   - sonarjs/super-linear-regex (S5852). Six findings, all the same shape: a
//     tail pattern (`[.\s]+$`, `-+$`, `/^#\s+(.+?)\s*$/`) the engine retries
//     from every start position, i.e. genuine quadratic backtracking. They all
//     run over the operator's own generated export tree — this app never takes
//     user content and never writes — so the exposure is a slow render on a
//     pathological local file, not a DoS vector. Kept on anyway: it is the only
//     rule here with a runtime consequence, and it is worth having pointed at
//     new code even while the current six are open.
//   - sonarjs/pseudo-random (S2245), switched off for test/ only, below.
//
// None of the code that predates these rules is exempted in this file. The
// findings that existed when they were switched on are recorded as a count per
// file and per rule in `eslint-suppressions.json` — ESLint's own bulk
// suppressions — so `npm run lint` is green without a single rule being
// weakened, and a *second* violation of a suppressed rule in a suppressed file
// exceeds the recorded count and reports. That is deliberately not a set of
// `off` entries below: an `off` blinds the rule for the whole file, including
// code written tomorrow, which is the opposite of what the baseline is for.
// Regenerate it with `npm run lint:baseline`, drop entries a refactor has made
// unnecessary with `npm run lint:prune`; both rewrite that file, so neither
// belongs in a readiness gate. Clearing it is a parked idea in
// NEXT-ITERATIONS.md — the honest occasion is a file being touched anyway — and
// an `eslint-disable` comment in the code is still the exception, not the way to
// add to the baseline.
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import sonarjs from 'eslint-plugin-sonarjs';
import globals from 'globals';

export default tseslint.config(
  // public/app.css is generated, dist/ is build output, and neither is source.
  { ignores: ['dist/**', 'node_modules/**', 'public/**', 'coverage/**'] },

  // A stale `eslint-disable` is itself a finding: it claims a rule is enforced
  // when it is not, which is exactly the mismatch this config exists to end.
  { linterOptions: { reportUnusedDisableDirectives: 'error' } },

  {
    files: ['src/**/*.ts', 'test/**/*.ts'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended, sonarjs.configs.recommended],
    // Server-side only — there is no client-side JavaScript in this project, so
    // no browser globals. Jest globals cover the specs in test/.
    languageOptions: { globals: { ...globals.node, ...globals.jest } },
    rules: {
      // Not in the recommended set, enabled deliberately: `console` is limited
      // to the single startup line in main.ts, and this rule plus that one
      // `eslint-disable-next-line` is what enforces it.
      'no-console': 'error',

      // Off on purpose. tsconfig.json sets `noImplicitAny: false`, and `any` is
      // sanctioned on the untyped surfaces this app is built on — markdown-it
      // token arrays, parsed YAML, the shiki highlighter loaded through
      // dynamic-import.ts. Enabling it would mean ~66 suppressions on code that
      // is correct, and WebStorm does not flag explicit `any` by default either,
      // so it would also break parity with the editor. "New code gets real
      // types" stays a review rule (see .windsurf/rules/02-style-and-quality.md)
      // rather than a lint gate.
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },

  // Test-only relaxations. Every rule switched off here is one
  // eslint-plugin-sonarjs reports on this project's specs but the SonarQube
  // quality profile does NOT — either because the rule is not in the profile at
  // all, or because Sonar classifies it as a Security Hotspot and this project
  // has none to review. Turning them off therefore moves the two tools closer
  // together, which is why (unlike the Go side's gocognit exclusions) none of
  // this needs a mirror in sonar-project.properties.
  //
  // Deliberately NOT switched off: sonarjs/prefer-specific-assertions (S5906).
  // The server does report that one on test/, so it stays a shared finding.
  {
    files: ['test/**/*.ts'],
    rules: {
      // S2245, a Security Hotspot on the server (0 to review). `Math.random`
      // in these specs names temporary fixture directories; nothing here has
      // to be unpredictable, and a spec that needed a real random source would
      // be reaching for crypto anyway.
      'sonarjs/pseudo-random': 'off',
      // S2004, not in the profile. The depth this reports is the describe/it
      // tree plus the callback that builds a fixture inside a case — test
      // structure, not logic that could be flattened into something clearer.
      'sonarjs/no-nested-functions': 'off',
      // S3358, not in the profile. A nested ternary choosing the expected value
      // per case is the compact form a table-driven spec wants.
      'sonarjs/no-nested-conditional': 'off',
    },
  },

  // The readiness scripts are plain CommonJS Node, not TypeScript: without
  // sourceType and the Node globals, `require`/`module`/`process` would all be
  // reported as undefined. They print their reports, so `no-console` is not
  // enabled here.
  {
    files: ['scripts/**/*.js'],
    extends: [js.configs.recommended, sonarjs.configs.recommended],
    languageOptions: { sourceType: 'commonjs', globals: { ...globals.node } },
  },

  // This file is ESM and must not inherit the CommonJS block above.
  {
    files: ['eslint.config.mjs'],
    extends: [js.configs.recommended],
    languageOptions: { globals: { ...globals.node } },
  },
);
