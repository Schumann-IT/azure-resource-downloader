/**
 * Shared domain types describing the azure-resource-downloader `output/` tree.
 *
 * Layout (authoritative, file-based — no database):
 *   output/<tenant-domain>/<provider>/<resourceType>/doc-prompt.md
 *   output/<tenant-domain>/<provider>/<resourceType>/<slug>.yaml
 *   output/<tenant-domain>/<provider>/<resourceType>/<slug>.doc.md   (generated, optional)
 */

/** A YAML value parsed from disk. Keys may contain dots/`@`; kept as plain map keys. */
export type YamlValue =
  | string
  | number
  | boolean
  | null
  | YamlValue[]
  | { [key: string]: YamlValue };

/** Identity/metadata fields pulled from a resource YAML for navigation/listing. */
export interface ResourceMeta {
  /** snake_case slug derived from the file name (without `.yaml`). */
  slug: string;
  displayName?: string;
  id?: string;
  createdDateTime?: string;
  lastModifiedDateTime?: string;
  version?: string | number;
}

/** One resource instance (a single `<slug>.yaml`). */
export interface ResourceEntry extends ResourceMeta {
  /** Path to the source YAML, relative to the output root (POSIX separators). */
  yamlPath: string;
  /** Path to the generated `<slug>.doc.md`, relative to the output root, if it exists. */
  docPath?: string;
  /** True when a sibling `<slug>.doc.md` exists. */
  hasDoc: boolean;
}

/** A resource-type folder (e.g. `deviceCompliancePolicies`). */
export interface ResourceTypeNode {
  provider: string;
  resourceType: string;
  /** Path to the folder's single `doc-prompt.md`, relative to the output root, if present. */
  docPromptPath?: string;
  resources: ResourceEntry[];
}

/** A provider namespace (e.g. `Microsoft.Graph`). */
export interface ProviderNode {
  provider: string;
  resourceTypes: ResourceTypeNode[];
}

/** The full tree for one tenant. */
export interface TenantTree {
  tenant: string;
  providers: ProviderNode[];
}

/** Compact navigation/index for one tenant. */
export interface TenantIndex {
  tenant: string;
  providers: Array<{
    provider: string;
    resourceTypes: Array<{
      resourceType: string;
      count: number;
      resources: ResourceMeta[];
    }>;
  }>;
  totalResources: number;
}

/** Top-level index enumerating every tenant. */
export interface OutputIndex {
  generatedAt: string;
  tenants: TenantIndex[];
}

/** Reference to a single resource within the tree. */
export interface ResourceRef {
  tenant: string;
  provider: string;
  resourceType: string;
  slug: string;
}
