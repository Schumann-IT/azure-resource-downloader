import { stringify } from 'yaml';
import type { YamlValue } from './types';

/**
 * Build the final LLM input for a resource: the verbatim `doc-prompt.md` for
 * the resource type, followed by the resource YAML in a fenced ```yaml block.
 * The prompt is never rewritten.
 */
export function buildDocPrompt(docPrompt: string, resourceYaml: string): string {
  const trimmed = docPrompt.replace(/\s+$/, '');
  return `${trimmed}\n\n---\n\nResource configuration:\n\n\`\`\`yaml\n${resourceYaml.replace(/\s+$/, '')}\n\`\`\`\n`;
}

/** Serialize a parsed value back to YAML (used when only a parsed doc is held). */
export function toYaml(value: YamlValue): string {
  return stringify(value);
}
