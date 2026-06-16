import type {
  OutputIndex,
  ResourceMeta,
  TenantIndex,
  TenantTree,
} from './types';
import { walkAll, walkTenant } from './walk';

function pickMeta(r: ResourceMeta): ResourceMeta {
  const m: ResourceMeta = { slug: r.slug };
  if (r.displayName !== undefined) m.displayName = r.displayName;
  if (r.id !== undefined) m.id = r.id;
  if (r.lastModifiedDateTime !== undefined)
    m.lastModifiedDateTime = r.lastModifiedDateTime;
  if (r.version !== undefined) m.version = r.version;
  return m;
}

/** Build the compact per-tenant index from a walked tree. */
export function buildTenantIndex(tree: TenantTree): TenantIndex {
  let total = 0;
  const providers = tree.providers.map((p) => ({
    provider: p.provider,
    resourceTypes: p.resourceTypes.map((rt) => {
      total += rt.resources.length;
      return {
        resourceType: rt.resourceType,
        count: rt.resources.length,
        resources: rt.resources.map(pickMeta),
      };
    }),
  }));
  return { tenant: tree.tenant, providers, totalResources: total };
}

/** Render a per-tenant `index.md` landing page from a tenant index. */
export function renderTenantIndexMarkdown(idx: TenantIndex): string {
  const lines: string[] = [];
  lines.push(`# ${idx.tenant}`);
  lines.push('');
  lines.push(`Total resources: **${idx.totalResources}**`);
  lines.push('');
  for (const p of idx.providers) {
    lines.push(`## ${p.provider}`);
    lines.push('');
    for (const rt of p.resourceTypes) {
      lines.push(`### ${rt.resourceType} (${rt.count})`);
      lines.push('');
      for (const r of rt.resources) {
        const name = r.displayName ?? r.slug;
        const link = `${p.provider}/${rt.resourceType}/${r.slug}.doc.md`;
        lines.push(`- [${name}](${link})`);
      }
      lines.push('');
    }
  }
  return `${lines.join('\n')}\n`;
}

/** Build the top-level index enumerating every tenant under the output root. */
export function buildOutputIndex(outputDir: string): OutputIndex {
  const trees: TenantTree[] = walkAll(outputDir);
  return {
    generatedAt: new Date().toISOString(),
    tenants: trees.map(buildTenantIndex),
  };
}

/** Build a single tenant's index by walking it fresh. */
export function buildTenantIndexFromDisk(
  outputDir: string,
  tenant: string,
): TenantIndex {
  return buildTenantIndex(walkTenant(outputDir, tenant));
}
