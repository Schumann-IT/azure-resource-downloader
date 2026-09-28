import { Injectable } from '@nestjs/common';
import { promises as fs } from 'fs';
import * as path from 'path';
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

// Reads what the tenant compare needs from disk: each export's
// `resources/metadata.yaml` (mtime-cached, so a re-download is reflected on the
// next request) and the two files of a paired key, which are only ever located
// through `resolveResource` — the existing boundary, called once per side.
// Read-only, like the rest of the app.
@Injectable()
export class CompareService {
  private readonly metadata = new FileCache<ResourcesMetadata | undefined>(MAX_METADATA_ENTRIES);

  // The export's resource metadata, or undefined when the file is absent or
  // malformed — either way the tenant cannot take part in a comparison.
  async metadataOf(info: TenantInfo): Promise<ResourcesMetadata | undefined> {
    return this.metadata.read(path.join(info.resourcesDir, RESOURCES_METADATA_FILE), (raw) =>
      parseResourcesMetadata(raw.toString('utf8')),
    );
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
