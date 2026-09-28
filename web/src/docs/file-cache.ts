import { promises as fs } from 'fs';

interface CacheEntry<T> {
  mtimeMs: number;
  size: number;
  value: T;
}

// A parsed-file cache keyed by path and validated by mtime + size from a
// per-call `stat()`, so a regenerated file is picked up on the next request
// without a restart. An entry whose `stat()` or read fails is dropped: the
// trees these files live in (drift/, resources/) can be deleted wholesale, and
// that must read as "absent", never as the last value seen. Bounded: the oldest
// entry is evicted past `maxEntries`. Shared by the drift and compare readers so
// the two cannot disagree about when `resources/metadata.yaml` changed.
export class FileCache<T> {
  private readonly entries = new Map<string, CacheEntry<T>>();

  constructor(private readonly maxEntries: number) {}

  async read(file: string, parse: (raw: Buffer) => T): Promise<T | undefined> {
    let stat;
    try {
      stat = await fs.stat(file);
    } catch {
      this.entries.delete(file);
      return undefined;
    }
    const hit = this.entries.get(file);
    if (hit && hit.mtimeMs === stat.mtimeMs && hit.size === stat.size) {
      return hit.value;
    }

    let raw: Buffer;
    try {
      raw = await fs.readFile(file);
    } catch {
      this.entries.delete(file);
      return undefined;
    }
    const value = parse(raw);
    this.entries.delete(file);
    this.entries.set(file, { mtimeMs: stat.mtimeMs, size: stat.size, value });
    if (this.entries.size > this.maxEntries) {
      const oldest = this.entries.keys().next().value;
      if (oldest !== undefined) this.entries.delete(oldest);
    }
    return value;
  }
}
