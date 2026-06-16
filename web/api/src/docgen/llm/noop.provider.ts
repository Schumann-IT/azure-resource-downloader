import type { LlmProvider } from './provider';

/**
 * Provider used when no API key is configured or LLM_PROVIDER=noop. It produces
 * a deterministic placeholder so the pipeline can be exercised end-to-end
 * (e.g. in tests) without spending API calls.
 */
export class NoopProvider implements LlmProvider {
  readonly name = 'noop';
  async generate(prompt: string): Promise<string> {
    const firstLine = prompt.split('\n', 1)[0] ?? '';
    return `# Generated documentation (noop provider)\n\n> No LLM provider configured. Set ANTHROPIC_API_KEY to generate real docs.\n\nPrompt preview: ${firstLine}\n`;
  }
}
