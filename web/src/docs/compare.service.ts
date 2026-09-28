import { Injectable } from '@nestjs/common';
import { promises as fs, Stats } from 'fs';
import * as path from 'path';
import { FileDigest, fileDigest } from './compare-view';
import { FileCache } from './file-cache';
import { resolveResource } from './path-safety';
import {
  parseResourcesMetadata,
  RESOURCES_METADATA_FILE,
  ResourcesMetadata,
} from './resources-metadata';
import { TenantInfo } from './tenant-discovery.service';

// Bound on the metadata cache: one entry per tenant, so far above any real
// docs root while still bounded like every other cache.
const MAX_METADATA_ENTRIES = 100;

// Bound on the digest cache: one entry per resource file, a few dozen bytes
// each, so several large exports fit with room to spare.
const MAX_DIGEST_ENTRIES = 20_000;

// How many resource files one listing reads at a time. Normalising is CPU on
// one thread, so this only keeps the reads from queueing all at once.
const DIGEST_CONCURRENCY = 16;

interface DigestEntry {
  mtimeMs: number;
  size: number;
  metadataMtimeMs: number;
  metadataSize: number;
  digest: FileDigest;
}

// Reads what the tenant compare needs from disk: each export's
// `resources/metadata.yaml` (mtime-cached, so a re-download is reflected on the
// next request), the per-file digests behind the comparison pane's status, and
// the two files of a paired key. Resource files are only ever located through
// `resolveResource` — the existing boundary — and only for keys both exports
// list. Read-only, like the rest of the app.
@Injectable()
export class CompareService {
  private readonly metadata = new FileCache<ResourcesMetadata | undefined>(MAX_METADATA_ENTRIES);

  // Per resource file, not per pair: its hashes, validated by its own stat and
  // its export's metadata stat (the reference lookup the normalisation uses), so
  // a tenant compared against several others normalises each file once and an
  // edited file or re-downloaded export is picked up on the next request. A
  // sibling of `FileCache`, which validates by one file only.
  private readonly digestCache = new Map<string, DigestEntry>();

  // The export's resource metadata, or undefined when the file is absent or
  // malformed — either way the tenant cannot take part in a comparison.
  async metadataOf(info: TenantInfo): Promise<ResourcesMetadata | undefined> {
    return this.metadata.read(path.join(info.resourcesDir, RESOURCES_METADATA_FILE), (raw) =>
      parseResourcesMetadata(raw.toString('utf8')),
    );
  }

  // The digest of each key's file in one export, or null for a file that does
  // not resolve or cannot be read. Files are located only through
  // `resolveResource`; `metadata` is the export's parsed metadata, whose file is
  // stat'ed once here to validate every entry.
  async digests(
    info: TenantInfo,
    metadata: ResourcesMetadata,
    keys: string[],
  ): Promise<Map<string, FileDigest | null>> {
    const out = new Map<string, FileDigest | null>();
    let metadataStat: Stats;
    try {
      metadataStat = await fs.stat(path.join(info.resourcesDir, RESOURCES_METADATA_FILE));
    } catch {
      for (const key of keys) out.set(key, null);
      return out;
    }
    let next = 0;
    const worker = async (): Promise<void> => {
      while (next < keys.length) {
        const key = keys[next++];
        out.set(key, await this.digestOf(info, metadata, metadataStat, key));
      }
    };
    await Promise.all(Array.from({ length: Math.min(DIGEST_CONCURRENCY, keys.length) }, worker));
    return out;
  }

  private async digestOf(
    info: TenantInfo,
    metadata: ResourcesMetadata,
    metadataStat: Stats,
    key: string,
  ): Promise<FileDigest | null> {
    const file = resolveResource(info.resourcesDir, key);
    if (!file) return null;
    let stat: Stats;
    try {
      stat = await fs.stat(file);
    } catch {
      this.digestCache.delete(file);
      return null;
    }
    const hit = this.digestCache.get(file);
    if (
      hit &&
      hit.mtimeMs === stat.mtimeMs &&
      hit.size === stat.size &&
      hit.metadataMtimeMs === metadataStat.mtimeMs &&
      hit.metadataSize === metadataStat.size
    ) {
      return hit.digest;
    }
    let raw: string;
    try {
      raw = await fs.readFile(file, 'utf8');
    } catch {
      this.digestCache.delete(file);
      return null;
    }
    const digest = fileDigest(raw, metadata);
    this.digestCache.delete(file);
    this.digestCache.set(file, {
      mtimeMs: stat.mtimeMs,
      size: stat.size,
      metadataMtimeMs: metadataStat.mtimeMs,
      metadataSize: metadataStat.size,
      digest,
    });
    if (this.digestCache.size > MAX_DIGEST_ENTRIES) {
      const oldest = this.digestCache.keys().next().value;
      if (oldest !== undefined) this.digestCache.delete(oldest);
    }
    return digest;
  }

  // Both files of a key present in both exports, or null when either does not
  // resolve or cannot be read.
  async pair(
    a: TenantInfo,
    b: TenantInfo,
    key: string,
  ): Promise<{ left: string; right: string } | null> {
    const leftFile = resolveResource(a.resourcesDir, key);
    const rightFile = resolveResource(b.resourcesDir, key);
    if (!leftFile || !rightFile) return null;
    try {
      const [left, right] = await Promise.all([
        fs.readFile(leftFile, 'utf8'),
        fs.readFile(rightFile, 'utf8'),
      ]);
      return { left, right };
    } catch {
      return null;
    }
  }
}
