import { Injectable } from '@nestjs/common';
import { promises as fs } from 'fs';
import * as path from 'path';
import { parseTenantIndex, TenantIndex } from './tenant-index';

export interface TenantInfo {
  // Route segment / picker id (the export folder name, e.g. "cb-gmbh.com").
  id: string;
  // Human-friendly name from the index (falls back to id).
  name: string;
  // Absolute path to the folder documents are resolved against: <export>/docs.
  // Relative `.md` links inside the documents are relative to this folder.
  dir: string;
  // Absolute path to that folder's index.yaml.
  indexPath: string;
  // Absolute path to the tenant-wide management summary the generation agent
  // writes at the docs root. Optional: it is not a discovery marker and its
  // existence is checked at render time, so a summary written after discovery
  // was cached still shows up on the next request.
  summaryPath: string;
  // In-scope resource counts, from the index (never by walking the tree).
  documented: number;
  pending: number;
  // When the export was generated, from the index (mirrors the export).
  generatedAt: string | null;
}

export const DOCS_DIR = 'docs';
export const INDEX_FILE = 'index.yaml';
export const SUMMARY_FILE = 'summary.md';
const MAX_DEPTH = 3;
const TTL_MS = 30_000;

interface IndexCacheEntry {
  mtimeMs: number;
  size: number;
  index: TenantIndex;
}

// Discovers tenant folders under DOCS_ROOT. A tenant is any directory that
// contains `docs/index.yaml` — the navigation index written by
// `azure-rd docs generate-index`. Results are cached with a short TTL so a
// newly generated tenant appears without a restart, and the parsed index is
// cached by mtime + size so a regenerated index is picked up on the next
// request.
@Injectable()
export class TenantDiscoveryService {
  private readonly root = path.resolve(
    process.cwd(),
    process.env.DOCS_ROOT || '../output',
  );
  private cache?: { at: number; tenants: TenantInfo[] };
  private readonly indexCache = new Map<string, IndexCacheEntry>();

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

  // Reads and parses a tenant's index.yaml, cached by mtime + size so a
  // regenerated index shows up without a restart. Returns undefined when the
  // file has become unreadable or malformed.
  async getIndex(info: TenantInfo): Promise<TenantIndex | undefined> {
    return this.readIndex(info.indexPath);
  }

  private async readIndex(file: string): Promise<TenantIndex | undefined> {
    let stat;
    try {
      stat = await fs.stat(file);
    } catch {
      return undefined;
    }

    const cached = this.indexCache.get(file);
    if (cached && cached.mtimeMs === stat.mtimeMs && cached.size === stat.size) {
      return cached.index;
    }

    let raw: string;
    try {
      raw = await fs.readFile(file, 'utf8');
    } catch {
      return undefined;
    }

    const index = parseTenantIndex(raw);
    if (!index) {
      this.indexCache.delete(file);
      return undefined;
    }

    this.indexCache.set(file, {
      mtimeMs: stat.mtimeMs,
      size: stat.size,
      index,
    });
    return index;
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

    const hasDocsDir = entries.some(
      (e) => e.isDirectory() && e.name === DOCS_DIR,
    );
    if (hasDocsDir) {
      const info = await this.readTenant(dir);
      if (info) {
        found.push(info);
        // A tenant owns its whole subtree — do not descend looking for more.
        return;
      }
    }

    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      // Skip housekeeping/hidden directories (e.g. _to_delete/).
      if (entry.name.startsWith('_') || entry.name.startsWith('.')) continue;
      await this.walk(path.join(dir, entry.name), depth + 1, found);
    }
  }

  private async readTenant(dir: string): Promise<TenantInfo | undefined> {
    const docsDir = path.join(dir, DOCS_DIR);
    const indexPath = path.join(docsDir, INDEX_FILE);
    const index = await this.readIndex(indexPath);
    if (!index) return undefined;

    const id = path.basename(dir);
    return {
      id,
      name: index.tenant || id,
      dir: docsDir,
      indexPath,
      summaryPath: path.join(docsDir, SUMMARY_FILE),
      documented: index.counts.documented,
      pending: index.counts.pending,
      generatedAt: index.generatedAt,
    };
  }
}
