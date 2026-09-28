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
import { FileCache } from './file-cache';
import { resolveDriftPayload, resolveResource } from './path-safety';
import { RESOURCES_METADATA_FILE } from './resources-metadata';
import { TenantInfo } from './tenant-discovery.service';
import { TenantIndex } from './tenant-index';

// Bound on the per-file hash cache, like the render caches.
const MAX_HASH_ENTRIES = 500;

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
  private readonly observations = new FileCache<DriftObservation | undefined>(MAX_HASH_ENTRIES);
  private readonly baselines = new FileCache<string | undefined>(MAX_HASH_ENTRIES);
  private readonly hashes = new FileCache<string>(MAX_HASH_ENTRIES);

  // The tenant's observation, or undefined when there is none or it does not
  // parse.
  async observation(info: TenantInfo): Promise<DriftObservation | undefined> {
    return this.observations.read(info.driftObservationPath, (raw) =>
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
    return this.baselines.read(file, (raw) =>
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

  // The baseline and observed texts for the diff view — only for a finding
  // whose comparison is intact and has both sides, so a diff is never shown
  // against bytes the verdict was not decided on. Null when either is missing
  // or vanished since it was verified.
  async diffSources(
    files: DriftFiles,
  ): Promise<{ baseline: string; observed: string } | null> {
    if (!files.intact || !files.baseline || !files.payload) return null;
    try {
      const [baseline, observed] = await Promise.all([
        fs.readFile(files.baseline, 'utf8'),
        fs.readFile(files.payload, 'utf8'),
      ]);
      return { baseline, observed };
    } catch {
      return null;
    }
  }

  private async verified(
    file: string | null,
    expected: string,
  ): Promise<string | null> {
    if (!file || !expected) return null;
    const actual = await this.hashes.read(file, (raw) =>
      createHash('sha256').update(raw).digest('hex'),
    );
    return actual === expected ? file : null;
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
