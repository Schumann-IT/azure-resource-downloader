import { Logger } from '@nestjs/common';
import { AnthropicProvider } from './anthropic.provider';
import { NoopProvider } from './noop.provider';
import { llmConfigFromEnv, type LlmProvider, type LlmProviderConfig } from './provider';

const log = new Logger('docgen:llm');

/**
 * Build the configured provider. Falls back to the noop provider when the
 * required credentials are absent, so --dry-run and tests never need a key.
 */
export function createProvider(cfg: LlmProviderConfig = llmConfigFromEnv()): LlmProvider {
  switch (cfg.provider) {
    case 'anthropic': {
      if (!cfg.apiKey) {
        log.warn('ANTHROPIC_API_KEY not set — falling back to the noop provider');
        return new NoopProvider();
      }
      return new AnthropicProvider(cfg.apiKey, cfg.model ?? 'claude-opus-4-8');
    }
    case 'noop':
      return new NoopProvider();
    // case 'openai': / case 'azure-openai': add here behind the same interface.
    default:
      log.warn(`Unknown LLM_PROVIDER "${cfg.provider}" — using noop provider`);
      return new NoopProvider();
  }
}
