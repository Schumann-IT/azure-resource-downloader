import { Injectable } from '@nestjs/common';
import { createHash } from 'crypto';
import { promises as fs } from 'fs';
import * as path from 'path';
import {
  DriftFinding,
  DriftObservation,
  parseBaselineGeneratedAt,
  parseObservation,
} from './drift-observation';
import { resolveDriftPayload, resolveResource } from './path-safety';
import { TenantInfo } from './tenant-discovery.service';
import { TenantIndex } from './tenant-index';

// The export's own metadata, whose top-level `generatedAt` is the baseline an
// observation is valid against.
const RESOURCES_METADATA_FILE = 'metadata.yaml';

// Bound on the per-file hash cache, like the render caches.
const MAX_HASH_ENTRIES = 500;

interface CacheEntry<T> {
  mtimeMs: number;
  size: number;
  value: T;
}

// The files a finding's page may show, each only once it is verified against
// the hash the observation recorded for it. `intact` is false when a file the
// verdict needs is missing or no longer matches: the comparison is then
// withheld rather than shown against bytes it was not decided on.
export interface DriftFiles {
  baseline: string | null;
  payload: string | null;
  intact: boolean;
}

// Reads what `azure-rd resource drift` left on disk: the observation, the
// baseline timestamp it must be checked against, and hashes of the files a
// finding names. Every read is cached by mtime + size from a per-request
// `stat()`, and an entry whose `stat()` fails is dropped — the drift tree is
// deleted wholesale by the next drift run or re-baselining download, and that
// must show on the next request without a restart. Read-only, like the rest of
// the app.
@Injectable()
export class DriftService {
  private readonly observations = new Map<string, CacheEntry<DriftObservation | undefined>>();
  private readonly baselines = new Map<string, CacheEntry<string | undefined>>();
  private readonly hashes = new Map<string, CacheEntry<string>>();

  // The tenant's observation, or undefined when there is none or it does not
  // parse.
  async observation(info: TenantInfo): Promise<DriftObservation | undefined> {
    return this.cached(this.observations, info.driftObservationPath, (raw) =>
      parseObservation(raw.toString('utf8')),
    );
  }

  // The export's baseline timestamp, from `resources/metadata.yaml` — the same
  // value `azure-rd docs analyze-drift` checks against, so the two tools never
  // disagree about one export. The index's `generatedAt` is only the fallback
  // for an export copied without that file: between a re-download and the next
  // `generate-index` it is stale, and gating on it would send the operator to
  // re-run drift when the index is what needs regenerating.
  async baselineGeneratedAt(
    info: TenantInfo,
    index: TenantIndex | undefined,
  ): Promise<string | undefined> {
    const file = path.join(info.resourcesDir, RESOURCES_METADATA_FILE);
    if (!(await exists(file))) return index?.generatedAt ?? undefined;
    return this.cached(this.baselines, file, (raw) =>
      parseBaselineGeneratedAt(raw.toString('utf8')),
    );
  }

  // The baseline file and the observed payload of a finding, each verified
  // against the hash recorded for it. A payload is only considered when the
  // observation names it.
  async files(
    info: TenantInfo,
    observation: DriftObservation,
    finding: DriftFinding,
  ): Promise<DriftFiles> {
    const baseline = finding.baselineKey
      ? await this.verified(
          resolveResource(info.resourcesDir, finding.baselineKey),
          finding.baselineSha256,
        )
      : null;
    const payload = observation.payloads.has(finding.key)
      ? await this.verified(
          resolveDriftPayload(info.driftDir, finding.key),
          finding.payloadSha256,
        )
      : null;
    return { baseline, payload, intact: intact(finding, baseline, payload) };
  }

  private async verified(
    file: string | null,
    expected: string,
  ): Promise<string | null> {
    if (!file || !expected) return null;
    const actual = await this.cached(this.hashes, file, (raw) =>
      createHash('sha256').update(raw).digest('hex'),
    );
    return actual === expected ? file : null;
  }

  private async cached<T>(
    cache: Map<string, CacheEntry<T>>,
    file: string,
    parse: (raw: Buffer) => T,
  ): Promise<T | undefined> {
    let stat;
    try {
      stat = await fs.stat(file);
    } catch {
      cache.delete(file);
      return undefined;
    }
    const hit = cache.get(file);
    if (hit && hit.mtimeMs === stat.mtimeMs && hit.size === stat.size) {
      return hit.value;
    }

    let raw: Buffer;
    try {
      raw = await fs.readFile(file);
    } catch {
      cache.delete(file);
      return undefined;
    }
    const value = parse(raw);
    cache.delete(file);
    cache.set(file, { mtimeMs: stat.mtimeMs, size: stat.size, value });
    if (cache.size > MAX_HASH_ENTRIES) {
      const oldest = cache.keys().next().value;
      if (oldest !== undefined) cache.delete(oldest);
    }
    return value;
  }
}

// Whether every file the verdict is decided on is present and verified. A
// removal needs none: its baseline file may legitimately have been pruned since.
function intact(
  finding: DriftFinding,
  baseline: string | null,
  payload: string | null,
): boolean {
  switch (finding.verdict) {
    case 'changed':
    case 'renamed':
      return baseline !== null && payload !== null;
    case 'added':
      return payload !== null;
    default:
      return true;
  }
}

async function exists(file: string): Promise<boolean> {
  try {
    await fs.stat(file);
    return true;
  } catch {
    return false;
  }
}
