// The single lint truth for this project: `npm run lint` runs ESLint with this
// file, and WebStorm's default "Automatic ESLint configuration" runs the same
// ESLint from node_modules against the same file, so an editor squiggle and a
// `npm run lint` finding are the same thing. The IDE's own bundled TypeScript
// inspections are a separate engine that cannot be expressed here; where the two
// overlap (unused symbols, unreachable code, `require` in an ES module) the rule
// sets below cover it, and anything only the bundled inspections report stays an
// editor hint rather than a merge gate.
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import globals from 'globals';

export default tseslint.config(
  // public/app.css is generated, dist/ is build output, and neither is source.
  { ignores: ['dist/**', 'node_modules/**', 'public/**', 'coverage/**'] },

  // A stale `eslint-disable` is itself a finding: it claims a rule is enforced
  // when it is not, which is exactly the mismatch this config exists to end.
  { linterOptions: { reportUnusedDisableDirectives: 'error' } },

  {
    files: ['src/**/*.ts', 'test/**/*.ts'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
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

  // The readiness scripts are plain CommonJS Node, not TypeScript: without
  // sourceType and the Node globals, `require`/`module`/`process` would all be
  // reported as undefined. They print their reports, so `no-console` is not
  // enabled here.
  {
    files: ['scripts/**/*.js'],
    extends: [js.configs.recommended],
    languageOptions: { sourceType: 'commonjs', globals: { ...globals.node } },
  },

  // This file is ESM and must not inherit the CommonJS block above.
  {
    files: ['eslint.config.mjs'],
    extends: [js.configs.recommended],
    languageOptions: { globals: { ...globals.node } },
  },
);
