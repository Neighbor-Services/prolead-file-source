export interface CacheEntry<T> {
  data: T;
  expiresAt: number;
  lastAccessed: number;
}

export interface CacheOptions {
  ttlMs?: number;
  maxEntries?: number;
}

/**
 * In-memory LRU cache with TTL expiration for Prolead File metadata & responses.
 */
export class ProleadMemoryCache {
  private cache = new Map<string, CacheEntry<any>>();
  private defaultTtlMs: number;
  private maxEntries: number;

  constructor(options: CacheOptions = {}) {
    this.defaultTtlMs = options.ttlMs ?? 5 * 60 * 1000; // 5 minutes default
    this.maxEntries = options.maxEntries ?? 500;
  }

  public get<T>(key: string): T | undefined {
    const entry = this.cache.get(key);
    if (!entry) return undefined;

    const now = Date.now();
    if (now > entry.expiresAt) {
      this.cache.delete(key);
      return undefined;
    }

    entry.lastAccessed = now;
    return entry.data as T;
  }

  public set<T>(key: string, data: T, ttlMs?: number): void {
    this.evictIfNeeded();
    const now = Date.now();
    this.cache.set(key, {
      data,
      expiresAt: now + (ttlMs ?? this.defaultTtlMs),
      lastAccessed: now,
    });
  }

  public has(key: string): boolean {
    return this.get(key) !== undefined;
  }

  public delete(key: string): boolean {
    return this.cache.delete(key);
  }

  public invalidatePrefix(prefix: string): void {
    for (const key of this.cache.keys()) {
      if (key.startsWith(prefix)) {
        this.cache.delete(key);
      }
    }
  }

  public clear(): void {
    this.cache.clear();
  }

  public get size(): number {
    return this.cache.size;
  }

  private evictIfNeeded(): void {
    if (this.cache.size < this.maxEntries) return;

    const now = Date.now();
    // 1. Evict expired entries
    for (const [key, entry] of this.cache.entries()) {
      if (now > entry.expiresAt) {
        this.cache.delete(key);
      }
    }

    // 2. If still exceeds, evict least recently accessed
    if (this.cache.size >= this.maxEntries) {
      let oldestKey: string | null = null;
      let oldestAccess = Infinity;

      for (const [key, entry] of this.cache.entries()) {
        if (entry.lastAccessed < oldestAccess) {
          oldestAccess = entry.lastAccessed;
          oldestKey = key;
        }
      }

      if (oldestKey) {
        this.cache.delete(oldestKey);
      }
    }
  }
}
