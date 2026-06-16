// Thin typed client for the NestJS API. Types mirror @ard/core's output
// shapes; kept local so the frontend stays decoupled from the Node package.

export type ChangeStatus = 'added' | 'removed' | 'changed' | 'unchanged';

export interface ResourceMeta {
  slug: string;
  displayName?: string;
  id?: string;
  lastModifiedDateTime?: string;
  version?: string | number;
}

export interface ResourceEntry extends ResourceMeta {
  yamlPath: string;
  docPath?: string;
  hasDoc: boolean;
}

export interface ResourceTypeNode {
  provider: string;
  resourceType: string;
  docPromptPath?: string;
  resources: ResourceEntry[];
}

export interface ProviderNode {
  provider: string;
  resourceTypes: ResourceTypeNode[];
}

export interface TenantTree {
  tenant: string;
  providers: ProviderNode[];
}

export interface ResourcePayload {
  ref: { tenant: string; provider: string; resourceType: string; slug: string };
  yaml: string;
  parsed: unknown;
  doc: string | null;
}

export interface DiffSummary {
  added: number;
  removed: number;
  changed: number;
  unchanged: number;
}

export interface FileDiffEntry {
  slug: string;
  status: ChangeStatus;
  changes?: DiffSummary;
}

export interface TypeDiffGroup {
  provider: string;
  resourceType: string;
  summary: DiffSummary;
  entries: FileDiffEntry[];
}

export interface TenantDiffResult {
  left: string;
  right: string;
  ignoreFields: string[];
  summary: DiffSummary;
  groups: TypeDiffGroup[];
}

export interface ValueDiff {
  path: string;
  key: string;
  status: ChangeStatus;
  left?: unknown;
  right?: unknown;
  children?: ValueDiff[];
}

const BASE = '/api';

async function get<T>(path: string, params: Record<string, string> = {}): Promise<T> {
  const qs = new URLSearchParams(params).toString();
  const res = await fetch(`${BASE}${path}${qs ? `?${qs}` : ''}`);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${body}`);
  }
  return (await res.json()) as T;
}

export const api = {
  tenants: () => get<{ tenants: string[] }>('/tenants'),
  tree: (tenant: string) => get<TenantTree>('/tree', { tenant }),
  resource: (tenant: string, provider: string, type: string, slug: string) =>
    get<ResourcePayload>('/resource', { tenant, provider, type, slug }),
  diff: (left: string, right: string, ignore: string[]) =>
    get<TenantDiffResult>('/diff', { left, right, ignore: ignore.join(',') }),
  diffResource: (
    left: string,
    right: string,
    provider: string,
    type: string,
    slug: string,
    ignore: string[],
  ) =>
    get<ValueDiff>('/diff/resource', {
      left,
      right,
      provider,
      type,
      slug,
      ignore: ignore.join(','),
    }),
};
