import { Command, CommandRunner, Option } from 'nest-commander';
import { DocgenService, type DocgenOptions } from './docgen.service';

interface CliOptions {
  tenant?: string;
  force?: boolean;
  dryRun?: boolean;
  concurrency?: number;
  provider?: string;
}

@Command({
  name: 'docgen',
  description:
    'Generate <slug>.doc.md documentation next to each resource YAML using its doc-prompt.md.',
  options: { isDefault: true },
})
export class DocgenCommand extends CommandRunner {
  constructor(private readonly svc: DocgenService) {
    super();
  }

  async run(_passed: string[], options: CliOptions = {}): Promise<void> {
    const opts: DocgenOptions = {
      tenant: options.tenant,
      force: options.force ?? false,
      dryRun: options.dryRun ?? false,
      concurrency: options.concurrency,
      provider: options.provider,
    };
    await this.svc.run(opts);
  }

  @Option({ flags: '-t, --tenant <tenant>', description: 'Limit generation to one tenant domain' })
  parseTenant(v: string): string {
    return v;
  }

  @Option({ flags: '-f, --force', description: 'Regenerate all docs, ignoring the manifest' })
  parseForce(): boolean {
    return true;
  }

  @Option({ flags: '-d, --dry-run', description: 'List what would be generated; no LLM calls, no writes' })
  parseDryRun(): boolean {
    return true;
  }

  @Option({ flags: '-c, --concurrency <n>', description: 'Max concurrent LLM calls (default 4)' })
  parseConcurrency(v: string): number {
    return Number.parseInt(v, 10);
  }

  @Option({ flags: '-p, --provider <name>', description: 'Override LLM_PROVIDER (anthropic | noop)' })
  parseProvider(v: string): string {
    return v;
  }
}
