import { Injectable } from '@nestjs/common';
import { promises as fs } from 'fs';
import * as path from 'path';

export interface TenantInfo {
  // Route segment / picker id (the tenant folder name, e.g. "cb-gmbh.com").
  id: string;
  // Human-friendly name from the manifest (falls back to id).
  name: string;
  // Absolute path to the tenant folder.
  dir: string;
  // Count of documented, in-scope resources (sum of manifest types[*].resources).
  resourceCount: number;
  // When the export was generated, from the manifest.
  generatedAt: string | null;
}

const MANIFEST = '.doc-manifest.json';
const INDEX = 'index.md';
const MAX_DEPTH = 3;
const TTL_MS = 30_000;

// Discovers tenant folders under DOCS_ROOT. A tenant is any directory that
// contains BOTH index.md and .doc-manifest.json. Results are cached with a
// short TTL so a newly generated tenant appears without a restart.
@Injectable()
export class TenantDiscoveryService {
  private readonly root = path.resolve(
    process.cwd(),
    process.env.DOCS_ROOT || '../output',
  );
  private cache?: { at: number; tenants: TenantInfo[] };

  getRoot(): string {
    return this.root;
  }

  async list(): Promise<TenantInfo[]> {
    if (this.cache && Date.now() - this.cache.at < TTL_MS) {
      return this.cache.tenants;
    }
    const tenants = await this.scan();
    this.cache = { at: Date.now(), tenants };
    return tenants;
  }

  async get(id: string): Promise<TenantInfo | undefined> {
    return (await this.list()).find((t) => t.id === id);
  }

  private async scan(): Promise<TenantInfo[]> {
    const found: TenantInfo[] = [];
    await this.walk(this.root, 0, found);
    found.sort((a, b) => a.id.localeCompare(b.id));
    return found;
  }

  private async walk(
    dir: string,
    depth: number,
    found: TenantInfo[],
  ): Promise<void> {
    if (depth > MAX_DEPTH) return;

    let entries;
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      return;
    }

    const names = new Set(entries.filter((e) => e.isFile()).map((e) => e.name));
    if (names.has(INDEX) && names.has(MANIFEST)) {
      const info = await this.readTenant(dir);
      if (info) found.push(info);
      // A tenant owns its whole subtree — do not descend looking for more.
      return;
    }

    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      // Skip housekeeping/hidden directories (e.g. _to_delete/).
      if (entry.name.startsWith('_') || entry.name.startsWith('.')) continue;
      await this.walk(path.join(dir, entry.name), depth + 1, found);
    }
  }

  private async readTenant(dir: string): Promise<TenantInfo | undefined> {
    try {
      const raw = await fs.readFile(path.join(dir, MANIFEST), 'utf8');
      const manifest = JSON.parse(raw);
      const types = manifest.types || {};
      let resourceCount = 0;
      for (const key of Object.keys(types)) {
        const resources = types[key]?.resources || {};
        resourceCount += Object.keys(resources).length;
      }
      return {
        id: path.basename(dir),
        name: manifest.tenant || path.basename(dir),
        dir,
        resourceCount,
        generatedAt: manifest.generatedAt || null,
      };
    } catch {
      return undefined;
    }
  }
}
