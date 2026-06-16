/** Pluggable LLM provider. Implementations must not hard-code secrets. */
export interface LlmProvider {
  readonly name: string;
  /** Generate Markdown documentation for the given fully-assembled prompt. */
  generate(prompt: string): Promise<string>;
}

export interface LlmProviderConfig {
  provider: string; // anthropic | noop (extend with openai | azure-openai)
  apiKey?: string;
  model?: string;
}

export function llmConfigFromEnv(): LlmProviderConfig {
  return {
    provider: (process.env.LLM_PROVIDER ?? 'anthropic').toLowerCase(),
    apiKey: process.env.ANTHROPIC_API_KEY,
    model: process.env.ANTHROPIC_MODEL ?? 'claude-opus-4-8',
  };
}
