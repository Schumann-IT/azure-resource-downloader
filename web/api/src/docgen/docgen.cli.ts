import 'reflect-metadata';
import { CommandFactory } from 'nest-commander';
import { DocgenModule } from './docgen.module';

/**
 * Standalone CLI entrypoint for docgen.
 * Usage: npm run docgen -- --dry-run [--tenant <domain>] [--force] [--concurrency 4]
 */
async function main() {
  await CommandFactory.run(DocgenModule, ['warn', 'error', 'log']);
}

void main();
