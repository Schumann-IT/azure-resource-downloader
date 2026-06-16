import { existsSync, readdirSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import type {
  ProviderNode,
  ResourceEntry,
  ResourceTypeNode,
  TenantTree,
} from './types';
import { extractMeta, parseYamlFile } from './yaml';

const DOC_PROMPT = 'doc-prompt.md';

/** Convert an absolute path to a POSIX-style path relative to the output root. */
function rel(outputDir: string, abs: string): string {
  return relative(outputDir, abs).split(sep).join('/');
}

function dirsIn(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name)
    .sort((a, b) => a.localeCompare(b));
}

/** List tenant domains (top-level directories) under the output root. */
export function listTenants(outputDir: string): string[] {
  return dirsIn(outputDir);
}

/**
 * Walk a single resource-type folder, returning its resources and the
 * `doc-prompt.md` location. Each `<slug>.yaml` is parsed for identity metadata;
 * a parse failure on one file does not abort the others.
 */
export function walkResourceType(
  outputDir: string,
  tenant: string,
  provider: string,
  resourceType: string,
): ResourceTypeNode {
  const folder = join(outputDir, tenant, provider, resourceType);
  const node: ResourceTypeNode = { provider, resourceType, resources: [] };

  const promptAbs = join(folder, DOC_PROMPT);
  if (existsSync(promptAbs)) node.docPromptPath = rel(outputDir, promptAbs);

  if (!existsSync(folder)) return node;

  const files = readdirSync(folder)
    .filter((f) => f.endsWith('.yaml'))
    .sort((a, b) => a.localeCompare(b));

  for (const file of files) {
    const slug = file.slice(0, -'.yaml'.length);
    const yamlAbs = join(folder, file);
    const docAbs = join(folder, `${slug}.doc.md`);
    const hasDoc = existsSync(docAbs);

    let entry: ResourceEntry = {
      slug,
      yamlPath: rel(outputDir, yamlAbs),
      hasDoc,
      ...(hasDoc ? { docPath: rel(outputDir, docAbs) } : {}),
    };

    try {
      const meta = extractMeta(parseYamlFile(yamlAbs), slug);
      entry = { ...meta, ...entry };
    } catch {
      // Keep the entry with file info even if YAML is unparseable.
    }
    node.resources.push(entry);
  }

  return node;
}

/** Walk every provider/resource-type under one tenant. */
export function walkTenant(outputDir: string, tenant: string): TenantTree {
  const tenantDir = join(outputDir, tenant);
  const providers: ProviderNode[] = [];

  for (const provider of dirsIn(tenantDir)) {
    const resourceTypes: ResourceTypeNode[] = [];
    for (const resourceType of dirsIn(join(tenantDir, provider))) {
      resourceTypes.push(
        walkResourceType(outputDir, tenant, provider, resourceType),
      );
    }
    providers.push({ provider, resourceTypes });
  }

  return { tenant, providers };
}

/** Walk all tenants under the output root. */
export function walkAll(outputDir: string): TenantTree[] {
  return listTenants(outputDir).map((t) => walkTenant(outputDir, t));
}

/** True when the path points at an existing directory. */
export function isDir(p: string): boolean {
  return existsSync(p) && statSync(p).isDirectory();
}
