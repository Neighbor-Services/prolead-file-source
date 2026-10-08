import {
  ProleadFileObject,
  ProleadFileVersion,
  ProleadBucket,
  ProleadStats,
  ListFilesOptions,
  ListFilesResult,
  ProleadImageTransform,
  SignedUrlResult,
  UploadOptions,
  TUSUploadOptions,
  ShareLink,
  CreateShareOptions,
  GCReport,
  DedupReport,
  ProleadEvent,
  ClientConfig
} from './types.js';

export class ProleadFileClient {
  private readonly baseUrl: string;
  private readonly apiKey?: string;

  constructor(config: ClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/+$/, '');
    this.apiKey = config.apiKey;
  }

  private getHeaders(extra?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
      'X-Prolead-Client': 'ts-sdk-1.0.0',
      ...extra
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    return headers;
  }

  // ---------------------------------------------------------------------------
  // BUCKETS
  // ---------------------------------------------------------------------------

  async listBuckets(): Promise<ProleadBucket[]> {
    const res = await fetch(`${this.baseUrl}/api/v1/buckets`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to list buckets: ${res.statusText}`);
    return res.json();
  }

  async getBucket(name: string): Promise<ProleadBucket> {
    const res = await fetch(`${this.baseUrl}/api/v1/buckets/${encodeURIComponent(name)}`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to get bucket ${name}: ${res.statusText}`);
    return res.json();
  }

  async createBucket(data: {
    name: string;
    description?: string;
    isPublic?: boolean;
    maxFileSize?: number;
    allowedMimes?: string[];
  }): Promise<ProleadBucket> {
    const res = await fetch(`${this.baseUrl}/api/v1/buckets`, {
      method: 'POST',
      headers: this.getHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        name: data.name.trim().toLowerCase(),
        description: data.description || '',
        isPublic: data.isPublic ?? true,
        maxFileSize: data.maxFileSize || 0,
        allowedMimes: data.allowedMimes || []
      })
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error?.message || `Failed to create bucket: ${res.statusText}`);
    }
    return res.json();
  }

  async updateBucket(
    name: string,
    data: {
      description?: string;
      isPublic?: boolean;
      maxFileSize?: number;
      allowedMimes?: string[];
    }
  ): Promise<ProleadBucket> {
    const res = await fetch(`${this.baseUrl}/api/v1/buckets/${encodeURIComponent(name)}`, {
      method: 'PUT',
      headers: this.getHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(data)
    });
    if (!res.ok) throw new Error(`Failed to update bucket ${name}: ${res.statusText}`);
    return res.json();
  }

  async deleteBucket(name: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/api/v1/buckets/${encodeURIComponent(name)}`, {
      method: 'DELETE',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to delete bucket ${name}: ${res.statusText}`);
  }

  // ---------------------------------------------------------------------------
  // OBJECTS & FILES
  // ---------------------------------------------------------------------------

  /**
   * Uploads content (Blob, Buffer, Uint8Array, or string) to a bucket.
   */
  async upload(
    content: Blob | Buffer | Uint8Array | string,
    options: UploadOptions
  ): Promise<ProleadFileObject> {
    const bucket = options.bucket || 'default';
    const form = new FormData();

    let blob: Blob;
    if (typeof content === 'string') {
      blob = new Blob([content], { type: options.contentType || 'text/plain; charset=utf-8' });
    } else if (content instanceof Blob) {
      blob = content;
    } else {
      blob = new Blob([content as BlobPart], { type: options.contentType || 'application/octet-stream' });
    }

    const filename = options.path.split('/').pop() || 'file';
    form.append('file', blob, filename);
    form.append('path', options.path);
    form.append('isPublic', (options.isPublic ?? true).toString());

    if (options.expiresInSeconds) {
      form.append('expiresInSeconds', options.expiresInSeconds.toString());
    }
    if (options.metadata) {
      form.append('metadata', JSON.stringify(options.metadata));
    }

    const res = await fetch(`${this.baseUrl}/v0/b/${encodeURIComponent(bucket)}/o`, {
      method: 'POST',
      headers: this.getHeaders(),
      body: form
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error?.message || `Upload failed: ${res.statusText}`);
    }

    return res.json();
  }

  /**
   * Creates a folder marker file (.keep) to materialize a folder prefix.
   */
  async createFolder(bucket: string = 'default', folderPath: string): Promise<ProleadFileObject> {
    const clean = folderPath.replace(/^\/+|\/+$/g, '');
    return this.upload('', {
      bucket,
      path: `${clean}/.keep`,
      contentType: 'application/octet-stream',
      isPublic: true
    });
  }

  async listFiles(bucket: string = 'default', options: ListFilesOptions = {}): Promise<ListFilesResult> {
    const params = new URLSearchParams();
    if (options.prefix) params.set('prefix', options.prefix);
    if (options.delimiter) params.set('delimiter', options.delimiter);
    if (options.limit) params.set('limit', options.limit.toString());
    if (options.offset) params.set('offset', options.offset.toString());
    if (options.search) params.set('search', options.search);
    if (options.trashOnly) params.set('trash', 'true');

    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o?${params.toString()}`, {
      headers: this.getHeaders()
    });

    if (!res.ok) throw new Error(`Failed to list files: ${res.statusText}`);
    return res.json();
  }

  async getFileMetadata(bucket: string, path: string): Promise<ProleadFileObject> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to get metadata for ${path}: ${res.statusText}`);
    return res.json();
  }

  async download(bucket: string, path: string): Promise<Blob> {
    const res = await fetch(`${this.baseUrl}/v0/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}?alt=media`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Download failed: ${res.statusText}`);
    return res.blob();
  }

  async downloadText(bucket: string, path: string): Promise<string> {
    const blob = await this.download(bucket, path);
    return blob.text();
  }

  async deleteFile(bucket: string, path: string, permanent: boolean = false): Promise<void> {
    const params = permanent ? '?permanent=true' : '';
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}${params}`, {
      method: 'DELETE',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to delete ${path}: ${res.statusText}`);
  }

  async restoreFile(bucket: string, path: string): Promise<ProleadFileObject> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}/restore`, {
      method: 'POST',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to restore ${path}: ${res.statusText}`);
    return res.json();
  }

  async rotateToken(bucket: string, path: string): Promise<ProleadFileObject> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}/rotate-token`, {
      method: 'POST',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to rotate token: ${res.statusText}`);
    return res.json();
  }

  async listVersions(bucket: string, path: string): Promise<ProleadFileVersion[]> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(path)}/versions`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to list versions: ${res.statusText}`);
    return res.json();
  }

  async generateSignedUrl(bucket: string, path: string, durationSeconds: number = 3600): Promise<SignedUrlResult> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/sign-url`, {
      method: 'POST',
      headers: this.getHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ path, durationSeconds })
    });
    if (!res.ok) throw new Error(`Failed to generate signed url: ${res.statusText}`);
    return res.json();
  }

  async downloadZip(bucket: string, paths: string[]): Promise<Blob> {
    const res = await fetch(`${this.baseUrl}/api/v1/b/${encodeURIComponent(bucket)}/download-zip`, {
      method: 'POST',
      headers: this.getHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ paths })
    });
    if (!res.ok) throw new Error(`Download zip failed: ${res.statusText}`);
    return res.blob();
  }

  // ---------------------------------------------------------------------------
  // IMAGE TRANSFORMS
  // ---------------------------------------------------------------------------

  getTransformedImageUrl(rawDownloadUrl: string, options: ProleadImageTransform): string {
    const url = new URL(rawDownloadUrl.startsWith('http') ? rawDownloadUrl : `${this.baseUrl}${rawDownloadUrl.startsWith('/') ? '' : '/'}${rawDownloadUrl}`);
    if (options.width) url.searchParams.set('w', options.width.toString());
    if (options.height) url.searchParams.set('h', options.height.toString());
    if (options.fit) url.searchParams.set('fit', options.fit);
    if (options.format) url.searchParams.set('format', options.format);
    if (options.quality) url.searchParams.set('q', options.quality.toString());
    return url.toString();
  }

  // ---------------------------------------------------------------------------
  // TELEMETRY & SSE
  // ---------------------------------------------------------------------------

  async getStats(): Promise<ProleadStats> {
    const res = await fetch(`${this.baseUrl}/api/v1/stats`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to get stats: ${res.statusText}`);
    return res.json();
  }

  /**
   * Subscribe to real-time storage events using EventSource or streaming fetch.
   */
  subscribeEvents(onEvent: (event: ProleadEvent) => void, onError?: (err: any) => void): () => void {
    const url = `${this.baseUrl}/api/v1/events/stream`;
    
    if (typeof EventSource !== 'undefined') {
      const es = new EventSource(url);
      es.onmessage = (e) => {
        if (e.data && e.data !== ':keepalive') {
          try {
            const parsed = JSON.parse(e.data);
            onEvent(parsed);
          } catch (_) {}
        }
      };
      es.onerror = (e) => onError?.(e);
      return () => es.close();
    }
    
    const controller = new AbortController();
    fetch(url, {
      headers: this.getHeaders({ Accept: 'text/event-stream' }),
      signal: controller.signal
    }).then(async (res) => {
      const reader = res.body?.getReader();
      if (!reader) return;
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          if (line.startsWith('data:')) {
            const data = line.slice(5).trim();
            if (data && data !== ':keepalive') {
              try {
                onEvent(JSON.parse(data));
              } catch (_) {}
            }
          }
        }
      }
    }).catch((err) => {
      if (!controller.signal.aborted) onError?.(err);
    });

    return () => controller.abort();
  }

  // ---------------------------------------------------------------------------
  // RESUMABLE TUS 1.0.0 UPLOAD
  // ---------------------------------------------------------------------------

  async uploadTUS(
    file: Blob | { size: number; slice: (start: number, end: number) => any },
    options: TUSUploadOptions
  ): Promise<ProleadFileObject> {
    const bucket = options.bucket || 'default';
    const path = options.path;
    const filename = path.split('/').pop() || 'file.bin';
    const size = file.size;
    const chunkSize = options.chunkSize || 4 * 1024 * 1024; // 4MB

    const metaObj: Record<string, string> = {
      bucket: encodeURIComponent(bucket),
      path: encodeURIComponent(path),
      filename: encodeURIComponent(filename),
      ...(options.metadata || {})
    };
    const uploadMetadata = Object.entries(metaObj)
      .map(([k, v]) => `${k} ${typeof btoa !== 'undefined' ? btoa(v) : Buffer.from(v).toString('base64')}`)
      .join(',');

    const createRes = await fetch(`${this.baseUrl}/api/v1/tus/upload`, {
      method: 'POST',
      headers: this.getHeaders({
        'Tus-Resumable': '1.0.0',
        'Upload-Length': size.toString(),
        'Upload-Metadata': uploadMetadata
      })
    });

    if (!createRes.ok && createRes.status !== 201) {
      throw new Error(`Failed to create TUS upload: ${createRes.statusText}`);
    }

    const location = createRes.headers.get('Location');
    if (!location) throw new Error('Missing TUS Location header');
    const uploadUrl = location.startsWith('http')
      ? location
      : `${this.baseUrl}${location.startsWith('/') ? '' : '/'}${location}`;

    let offset = 0;
    while (offset < size) {
      const chunk = file.slice(offset, offset + chunkSize);
      const patchRes = await fetch(uploadUrl, {
        method: 'PATCH',
        headers: this.getHeaders({
          'Tus-Resumable': '1.0.0',
          'Upload-Offset': offset.toString(),
          'Content-Type': 'application/offset+octet-stream'
        }),
        body: chunk as any
      });

      if (!patchRes.ok && patchRes.status !== 204 && patchRes.status !== 200) {
        throw new Error(`TUS chunk failed at offset ${offset}: ${patchRes.statusText}`);
      }

      const newOffsetStr = patchRes.headers.get('Upload-Offset');
      if (newOffsetStr) {
        offset = parseInt(newOffsetStr, 10);
      } else {
        offset += (chunk as any).size || (chunk as any).length || chunkSize;
      }

      options.onProgress?.({
        loaded: offset,
        total: size,
        percent: Math.min(100, Math.round((offset / size) * 100))
      });
    }

    return this.getFileMetadata(bucket, path);
  }

  // ---------------------------------------------------------------------------
  // SHARE LINKS
  // ---------------------------------------------------------------------------

  async createShareLink(
    bucket: string,
    path: string,
    options?: CreateShareOptions
  ): Promise<ShareLink> {
    const res = await fetch(`${this.baseUrl}/api/v1/shares`, {
      method: 'POST',
      headers: this.getHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        bucket,
        path,
        durationHours: options?.durationHours || 24,
        password: options?.password || '',
        maxDownloads: options?.maxDownloads || 0
      })
    });
    if (!res.ok) throw new Error(`Failed to create share link: ${res.statusText}`);
    return res.json();
  }

  async listShareLinks(bucket?: string): Promise<ShareLink[]> {
    const query = bucket ? `?bucket=${encodeURIComponent(bucket)}` : '';
    const res = await fetch(`${this.baseUrl}/api/v1/shares${query}`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to list share links: ${res.statusText}`);
    return res.json();
  }

  async revokeShareLink(token: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/api/v1/shares/${encodeURIComponent(token)}`, {
      method: 'DELETE',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to revoke share link: ${res.statusText}`);
  }

  // ---------------------------------------------------------------------------
  // ADMIN & MAINTENANCE
  // ---------------------------------------------------------------------------

  async exportAuditLogs(options?: { format?: 'csv' | 'json' }): Promise<Blob> {
    const format = options?.format || 'csv';
    const res = await fetch(`${this.baseUrl}/api/v1/admin/audit-logs/export?format=${encodeURIComponent(format)}`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to export audit logs: ${res.statusText}`);
    return res.blob();
  }

  async triggerGC(): Promise<GCReport> {
    const res = await fetch(`${this.baseUrl}/api/v1/admin/gc`, {
      method: 'POST',
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to trigger GC: ${res.statusText}`);
    return res.json();
  }

  async getDedupReport(): Promise<DedupReport> {
    const res = await fetch(`${this.baseUrl}/api/v1/admin/dedup-report`, {
      headers: this.getHeaders()
    });
    if (!res.ok) throw new Error(`Failed to get dedup report: ${res.statusText}`);
    return res.json();
  }
}
