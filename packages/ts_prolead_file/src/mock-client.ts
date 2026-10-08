import {
  ProleadBucket,
  ProleadFileObject,
  ProleadFileVersion,
  ProleadStats,
  ListFilesOptions,
  ListFilesResult,
  UploadOptions,
  SignedUrlResult,
  ProleadImageTransform,
  ProleadEvent,
} from './types.js';

interface MockFileEntry {
  obj: ProleadFileObject;
  data: Uint8Array;
}

/**
 * In-memory Mock implementation of ProleadFileClient for Node.js / TypeScript test suites.
 */
export class MockProleadFileClient {
  public baseUrl: string;
  public apiKey?: string;
  private buckets: Map<string, ProleadBucket> = new Map();
  private storage: Map<string, Map<string, MockFileEntry>> = new Map();
  public simulatedDelayMs: number = 0;

  constructor(baseUrl: string = 'http://localhost:8080', apiKey?: string) {
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.apiKey = apiKey;

    // Default bucket
    this.buckets.set('default', {
      id: 'b-default',
      name: 'default',
      description: 'Default Bucket',
      isPublic: true,
      maxFileSize: 104857600,
      allowedMimes: [],
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    });
    this.storage.set('default', new Map());
  }

  private async delay(): Promise<void> {
    if (this.simulatedDelayMs > 0) {
      await new Promise((r) => setTimeout(r, this.simulatedDelayMs));
    }
  }

  public async listBuckets(): Promise<ProleadBucket[]> {
    await this.delay();
    return Array.from(this.buckets.values());
  }

  public async getBucket(name: string): Promise<ProleadBucket> {
    await this.delay();
    const b = this.buckets.get(name);
    if (!b) throw new Error(`Bucket not found: ${name}`);
    return b;
  }

  public async createBucket(
    name: string,
    options: Partial<Omit<ProleadBucket, 'id' | 'name' | 'createdAt' | 'updatedAt'>> = {}
  ): Promise<ProleadBucket> {
    await this.delay();
    const clean = name.trim().toLowerCase();
    if (this.buckets.has(clean)) {
      throw new Error(`Bucket already exists: ${clean}`);
    }
    const bucket: ProleadBucket = {
      id: `b-${clean}`,
      name: clean,
      description: options.description || '',
      isPublic: options.isPublic ?? true,
      maxFileSize: options.maxFileSize || 0,
      allowedMimes: options.allowedMimes || [],
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    this.buckets.set(clean, bucket);
    this.storage.set(clean, new Map());
    return bucket;
  }

  public async deleteBucket(name: string): Promise<void> {
    await this.delay();
    this.buckets.delete(name);
    this.storage.delete(name);
  }

  public async upload(
    file: Blob | Buffer | Uint8Array | string,
    options: UploadOptions
  ): Promise<ProleadFileObject> {
    await this.delay();
    const bucket = options.bucket || 'default';
    const cleanPath = options.path.replace(/^\/+/, '');
    const filename = cleanPath.split('/').pop() || 'file';

    let data: Uint8Array;
    if (typeof file === 'string') {
      data = new TextEncoder().encode(file);
    } else if (file instanceof Uint8Array) {
      data = file;
    } else if (typeof Buffer !== 'undefined' && Buffer.isBuffer(file)) {
      data = new Uint8Array(file);
    } else if (file instanceof Blob) {
      const arr = await file.arrayBuffer();
      data = new Uint8Array(arr);
    } else {
      data = new Uint8Array();
    }

    if (options.onProgress) {
      options.onProgress({ loaded: data.byteLength, total: data.byteLength, percent: 100 });
    }

    const fileObj: ProleadFileObject = {
      id: `obj-${Date.now()}`,
      bucket,
      path: cleanPath,
      name: filename,
      size: data.byteLength,
      contentType: options.contentType || 'application/octet-stream',
      sha256: `mocksha-${data.byteLength}`,
      downloadToken: `mock-tok-${Date.now()}`,
      downloadUrl: `${this.baseUrl}/v0/b/${bucket}/o/${encodeURIComponent(cleanPath)}?alt=media`,
      isPublic: options.isPublic ?? true,
      metadata: options.metadata || {},
      version: 1,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };

    if (!this.storage.has(bucket)) {
      this.storage.set(bucket, new Map());
    }
    this.storage.get(bucket)!.set(cleanPath, { obj: fileObj, data });

    return fileObj;
  }

  public async listFiles(bucket: string, options: ListFilesOptions = {}): Promise<ListFilesResult> {
    await this.delay();
    const bucketFiles = Array.from(this.storage.get(bucket)?.values() || []).map((e) => e.obj);
    const prefix = options.prefix || '';
    const search = options.search?.toLowerCase() || '';

    let filtered = bucketFiles.filter((f) => {
      if (options.trashOnly && !f.deletedAt) return false;
      if (!options.trashOnly && f.deletedAt) return false;
      if (prefix && !f.path.startsWith(prefix)) return false;
      if (search && !f.path.toLowerCase().includes(search) && !f.name.toLowerCase().includes(search)) return false;
      return true;
    });

    const offset = options.offset || 0;
    const limit = options.limit || 100;
    const items = filtered.slice(offset, offset + limit);

    return {
      items,
      prefixes: [],
      total: filtered.length,
    };
  }

  public async getFileMetadata(bucket: string, path: string): Promise<ProleadFileObject> {
    await this.delay();
    const cleanPath = path.replace(/^\/+/, '');
    const entry = this.storage.get(bucket)?.get(cleanPath);
    if (!entry) throw new Error(`File not found: ${path}`);
    return entry.obj;
  }

  public async downloadBuffer(bucket: string, path: string): Promise<Uint8Array> {
    await this.delay();
    const cleanPath = path.replace(/^\/+/, '');
    const entry = this.storage.get(bucket)?.get(cleanPath);
    if (!entry) throw new Error(`File not found: ${path}`);
    return entry.data;
  }

  public async deleteFile(bucket: string, path: string, permanent: boolean = false): Promise<void> {
    await this.delay();
    const cleanPath = path.replace(/^\/+/, '');
    const bucketStore = this.storage.get(bucket);
    if (permanent) {
      bucketStore?.delete(cleanPath);
    } else {
      const entry = bucketStore?.get(cleanPath);
      if (entry) {
        entry.obj.deletedAt = new Date().toISOString();
      }
    }
  }

  public async restoreFile(bucket: string, path: string): Promise<ProleadFileObject> {
    await this.delay();
    const cleanPath = path.replace(/^\/+/, '');
    const entry = this.storage.get(bucket)?.get(cleanPath);
    if (!entry) throw new Error(`File not found: ${path}`);
    delete entry.obj.deletedAt;
    return entry.obj;
  }

  public async generateSignedUrl(
    bucket: string,
    path: string,
    durationSeconds: number = 3600
  ): Promise<SignedUrlResult> {
    await this.delay();
    return {
      signedUrl: `${this.baseUrl}/v0/b/${bucket}/o/${encodeURIComponent(path)}?sig=mockhmac&expires=${Date.now() + durationSeconds * 1000}`,
      expiresAt: new Date(Date.now() + durationSeconds * 1000).toISOString(),
    };
  }

  public getTransformedImageUrl(rawDownloadUrl: string, options: ProleadImageTransform): string {
    const url = new URL(rawDownloadUrl.startsWith('http') ? rawDownloadUrl : `${this.baseUrl}${rawDownloadUrl}`);
    if (options.width) url.searchParams.set('w', options.width.toString());
    if (options.height) url.searchParams.set('h', options.height.toString());
    if (options.fit) url.searchParams.set('fit', options.fit);
    if (options.format) url.searchParams.set('format', options.format);
    if (options.quality) url.searchParams.set('q', options.quality.toString());
    return url.toString();
  }

  public async getStats(): Promise<ProleadStats> {
    await this.delay();
    let totalFiles = 0;
    let totalBytes = 0;
    for (const bucketStore of this.storage.values()) {
      for (const entry of bucketStore.values()) {
        totalFiles++;
        totalBytes += entry.data.byteLength;
      }
    }
    return {
      totalFiles,
      totalLogicalBytes: totalBytes,
      totalPhysicalBytes: Math.floor(totalBytes * 0.75),
      totalSavedBytes: Math.floor(totalBytes * 0.25),
      dedupRatio: 1.33,
      activeBuckets: this.buckets.size,
      activeVersions: 1,
      trashFiles: 0,
    };
  }

  public async uploadTUS(
    file: Blob | { size: number; slice: (start: number, end: number) => any },
    options: { bucket?: string; path: string; onProgress?: (p: any) => void }
  ): Promise<ProleadFileObject> {
    options.onProgress?.({ loaded: file.size, total: file.size, percent: 100 });
    return this.upload(file as any, options);
  }

  public async createShareLink(
    bucket: string,
    path: string,
    options?: { durationHours?: number; password?: string; maxDownloads?: number }
  ) {
    await this.delay();
    return {
      id: 'mock-share-1',
      bucket,
      path,
      token: 'mock-share-token-xyz',
      requirePassword: !!options?.password,
      maxDownloads: options?.maxDownloads || 0,
      downloadCount: 0,
      downloadUrl: `${this.baseUrl}/s/mock-share-token-xyz`,
      createdAt: new Date().toISOString(),
    };
  }

  public async listShareLinks(bucket?: string) {
    await this.delay();
    return [
      {
        id: 'mock-share-1',
        bucket: bucket || 'default',
        path: 'test.png',
        token: 'mock-share-token-xyz',
        requirePassword: false,
        maxDownloads: 0,
        downloadCount: 0,
        downloadUrl: `${this.baseUrl}/s/mock-share-token-xyz`,
        createdAt: new Date().toISOString(),
      },
    ];
  }

  public async revokeShareLink(token: string): Promise<void> {
    await this.delay();
  }

  public async exportAuditLogs(options?: { format?: 'csv' | 'json' }): Promise<Blob> {
    await this.delay();
    return new Blob(['id,userId,action,status\n1,mock-user,UPLOAD,SUCCESS\n'], {
      type: 'text/csv',
    });
  }

  public async triggerGC() {
    await this.delay();
    return { deletedBlobs: 5, freedBytes: 10485760 };
  }

  public async getDedupReport() {
    await this.delay();
    return {
      totalLogicalBytes: 104857600,
      totalPhysicalBytes: 52428800,
      totalSavedBytes: 52428800,
      dedupRatio: 2.0,
      activeBlobs: 45,
      filesReferenced: 90,
    };
  }
}
