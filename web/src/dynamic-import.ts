// Loads a module via a *native* dynamic `import()` even though this project is
// compiled to CommonJS. TypeScript would otherwise down-level `import()` to
// `require()`, which cannot load the ESM-only packages we depend on
// (markdown-it-anchor v9 is pure ESM). Wrapping it in `new Function` hides the
// call from the compiler so it survives to runtime as a real dynamic import.
export const dynamicImport = new Function(
  'specifier',
  'return import(specifier)',
) as <T = any>(specifier: string) => Promise<T>;
