import { resolve } from 'node:path';

/**
 * Resolve the azure-resource-downloader `output/` directory.
 * Precedence: ARD_OUTPUT_DIR (resolved from cwd) > default at the repo root.
 *
 * This file lives at web/api/src/config.ts (and web/api/dist/config.js); both
 * are three levels below the repo root, so ../../../output points at the Go
 * tool's output tree in either case.
 */
export function resolveOutputDir(): string {
  const env = process.env.ARD_OUTPUT_DIR;
  if (env && env.trim()) return resolve(process.cwd(), env.trim());
  return resolve(__dirname, '..', '..', '..', 'output');
}

export const API_PREFIX = 'api';

export function resolvePort(): number {
  const p = Number.parseInt(process.env.PORT ?? '', 10);
  return Number.isFinite(p) && p > 0 ? p : 3001;
}
