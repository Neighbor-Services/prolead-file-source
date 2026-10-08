import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpEvent, HttpEventType, HttpHeaders, HttpParams, HttpRequest } from '@angular/common/http';
import { Observable, map } from 'rxjs';
import { 
  StorageObject, 
  FileVersion,
  WebhookConfig,
  ListFilesResult, 
  StorageBucket, 
  StorageStats,
  UploadProgressEvent, 
  ImageTransformOptions, 
  SignedUrlResult, 
  StorageEvent, 
  DedupReport,
  AuditLogEntry,
  APIKey,
  Project,
  UserProfile,
  ShareLink,
  LifecycleRule,
  TestWebhookResult,
  WebhookDelivery,
  WorkerJobRecord,
  WorkerPoolStats
} from './gostore.models';
import { signal } from '@angular/core';

@Injectable({
  providedIn: 'root'
})
export class GoStoreService {
  private http = inject(HttpClient);

  private baseUrl = typeof window !== 'undefined' && window.location.port === '4200'
    ? 'http://localhost:8080'
    : (typeof window !== 'undefined' ? window.location.origin : '');
  private apiKey = 'gostore-master-secret-key';

  currentUser = signal<UserProfile | null>(null);
  currentProject = signal<Project>({
    id: 'p-default',
    name: 'Default Project',
    slug: 'default',
    description: 'Main production workspace',
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString()
  });

  constructor() {
    const savedKey = localStorage.getItem('gostore_api_key');
    if (savedKey) {
      this.apiKey = savedKey;
    }
    const savedUser = localStorage.getItem('gostore_user');
    if (savedUser) {
      try {
        this.currentUser.set(JSON.parse(savedUser));
      } catch (e) {}
    }
    const savedProject = localStorage.getItem('gostore_project');
    if (savedProject) {
      try {
        this.currentProject.set(JSON.parse(savedProject));
      } catch (e) {}
    }
  }

  login(username: string, password: string): Observable<{ token: string; user: UserProfile; isAdmin: boolean }> {
    return this.http.post<{ token: string; user: UserProfile; isAdmin: boolean }>(
      `${this.baseUrl}/api/v1/auth/login`,
      { username, password }
    ).pipe(
      map(res => {
        this.setConfig(this.baseUrl, res.token);
        this.currentUser.set(res.user);
        localStorage.setItem('gostore_user', JSON.stringify(res.user));
        return res;
      })
    );
  }

  logout(): void {
    if (this.apiKey) {
      this.http.post(`${this.baseUrl}/api/v1/auth/logout`, {}, { headers: this.getHeaders() }).subscribe({
        error: () => {}
      });
    }
    this.currentUser.set(null);
    localStorage.removeItem('gostore_user');
    this.apiKey = '';
    localStorage.removeItem('gostore_api_key');
  }

  listProjects(): Observable<Project[]> {
    return this.http.get<Project[]>(`${this.baseUrl}/api/v1/projects`, {
      headers: this.getHeaders()
    });
  }

  createProject(name: string, description: string): Observable<Project> {
    return this.http.post<Project>(
      `${this.baseUrl}/api/v1/projects`,
      { name, description },
      { headers: this.getHeaders() }
    );
  }

  deleteProject(id: string): Observable<{ success: boolean; message: string }> {
    return this.http.delete<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/projects/${id}`,
      { headers: this.getHeaders() }
    );
  }

  selectProject(p: Project): void {
    this.currentProject.set(p);
    localStorage.setItem('gostore_project', JSON.stringify(p));
  }

  getBaseUrl(): string {
    return this.baseUrl;
  }

  getApiKey(): string {
    return this.apiKey;
  }

  setConfig(baseUrl: string, apiKey?: string): void {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    if (apiKey !== undefined) {
      this.apiKey = apiKey;
      localStorage.setItem('gostore_api_key', apiKey);
    }
  }

  private getHeaders(): HttpHeaders {
    let headers = new HttpHeaders();
    if (this.apiKey) {
      headers = headers.set('Authorization', `Bearer ${this.apiKey}`);
    }
    const proj = this.currentProject();
    if (proj?.id) {
      headers = headers.set('X-Project-ID', proj.id);
    }
    return headers;
  }

  getStats(): Observable<StorageStats> {
    return this.http.get<StorageStats>(`${this.baseUrl}/api/v1/stats`, {
      headers: this.getHeaders()
    });
  }

  uploadWithProgress(
    file: File,
    path: string,
    bucket = 'default',
    isPublic = true,
    expiresInSeconds?: number,
    metadata?: Record<string, string>
  ): Observable<UploadProgressEvent> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('path', path);
    formData.append('isPublic', isPublic ? 'true' : 'false');

    if (expiresInSeconds) {
      formData.append('expiresInSeconds', expiresInSeconds.toString());
    }

    if (metadata) {
      formData.append('metadata', JSON.stringify(metadata));
    }

    const req = new HttpRequest('POST', `${this.baseUrl}/v0/b/${bucket}/o`, formData, {
      headers: this.getHeaders(),
      reportProgress: true
    });

    return this.http.request<StorageObject>(req).pipe(
      map((event: HttpEvent<StorageObject>): UploadProgressEvent => {
        switch (event.type) {
          case HttpEventType.UploadProgress: {
            const total = event.total ?? file.size;
            const progress = total > 0 ? Math.round((100 * event.loaded) / total) : 0;
            return { status: 'uploading', progress };
          }
          case HttpEventType.Response: {
            return {
              status: 'completed',
              progress: 100,
              result: event.body as StorageObject
            };
          }
          default:
            return { status: 'uploading', progress: 0 };
        }
      })
    );
  }

  getTransformedImageUrl(downloadUrl: string, opts: ImageTransformOptions): string {
    if (!downloadUrl) return '';
    try {
      const base = this.baseUrl || (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080');
      const url = new URL(downloadUrl, base);
      if (opts.width) url.searchParams.set('w', opts.width.toString());
      if (opts.height) url.searchParams.set('h', opts.height.toString());
      if (opts.fit) url.searchParams.set('fit', opts.fit);
      if (opts.format) url.searchParams.set('format', opts.format);
      if (opts.quality) url.searchParams.set('q', opts.quality.toString());
      return url.toString();
    } catch (e) {
      return downloadUrl;
    }
  }

  generateSignedUrl(
    bucket: string,
    path: string,
    durationSeconds = 3600
  ): Observable<SignedUrlResult> {
    return this.http.post<SignedUrlResult>(
      `${this.baseUrl}/api/v1/b/${bucket}/sign-url`,
      { path, durationSeconds },
      { headers: this.getHeaders() }
    );
  }

  downloadZip(bucket: string, paths: string[]): Observable<Blob> {
    return this.http.post(
      `${this.baseUrl}/api/v1/b/${bucket}/download-zip`,
      { paths },
      {
        headers: this.getHeaders(),
        responseType: 'blob'
      }
    );
  }

  listenToEvents(): Observable<StorageEvent> {
    return new Observable<StorageEvent>(observer => {
      const eventSource = new EventSource(`${this.baseUrl}/api/v1/events`);

      const handleEvent = (event: MessageEvent) => {
        try {
          const parsed = JSON.parse(event.data) as StorageEvent;
          observer.next(parsed);
        } catch (err) {
          // ignore keepalive
        }
      };

      eventSource.addEventListener('file:uploaded', handleEvent);
      eventSource.addEventListener('file:deleted', handleEvent);
      eventSource.addEventListener('file:restored', handleEvent);
      eventSource.addEventListener('token:rotated', handleEvent);
      eventSource.addEventListener('bucket:created', handleEvent);

      eventSource.onerror = (err) => {
        // EventSource will automatically attempt reconnection; do not terminate stream
        console.debug('SSE connection state changed (reconnecting)...', err);
      };

      return () => {
        eventSource.close();
      };
    });
  }

  listFiles(
    bucket = 'default',
    prefix = '',
    delimiter = '/',
    limit = 100,
    offset = 0,
    search = '',
    trashOnly = false
  ): Observable<ListFilesResult> {
    let params = new HttpParams()
      .set('prefix', prefix)
      .set('delimiter', delimiter)
      .set('limit', limit.toString())
      .set('offset', offset.toString());

    if (search) {
      params = params.set('search', search);
    }
    if (trashOnly) {
      params = params.set('trash', 'true');
    }

    return this.http.get<ListFilesResult>(`${this.baseUrl}/api/v1/b/${bucket}/o`, {
      headers: this.getHeaders(),
      params
    });
  }

  restoreFile(bucket: string, path: string): Observable<StorageObject> {
    let params = new HttpParams().set('path', path);
    return this.http.post<StorageObject>(
      `${this.baseUrl}/api/v1/b/${bucket}/restore`,
      {},
      { headers: this.getHeaders(), params }
    );
  }

  listVersions(bucket: string, path: string): Observable<FileVersion[]> {
    let params = new HttpParams().set('path', path);
    return this.http.get<FileVersion[]>(
      `${this.baseUrl}/api/v1/b/${bucket}/versions`,
      { headers: this.getHeaders(), params }
    );
  }

  rotateDownloadToken(bucket: string, path: string): Observable<StorageObject> {
    let params = new HttpParams().set('path', path);
    return this.http.post<StorageObject>(
      `${this.baseUrl}/api/v1/b/${bucket}/rotate-token`,
      {},
      { headers: this.getHeaders(), params }
    );
  }

  deleteFile(bucket: string, path: string, permanent = false): Observable<{ success: boolean; message: string }> {
    let params = new HttpParams();
    if (permanent) {
      params = params.set('permanent', 'true');
    }
    return this.http.delete<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/b/${bucket}/o/${encodeURIComponent(path)}`,
      { headers: this.getHeaders(), params }
    );
  }

  copyFile(bucket: string, srcPath: string, dstBucket: string, dstPath: string): Observable<StorageObject> {
    return this.http.post<StorageObject>(
      `${this.baseUrl}/api/v1/b/${bucket}/copy`,
      { srcPath, dstBucket, dstPath },
      { headers: this.getHeaders() }
    );
  }

  moveFile(bucket: string, srcPath: string, dstBucket: string, dstPath: string): Observable<StorageObject> {
    return this.http.post<StorageObject>(
      `${this.baseUrl}/api/v1/b/${bucket}/move`,
      { srcPath, dstBucket, dstPath },
      { headers: this.getHeaders() }
    );
  }

  renameFile(bucket: string, oldPath: string, newName: string): Observable<StorageObject> {
    return this.http.post<StorageObject>(
      `${this.baseUrl}/api/v1/b/${bucket}/rename`,
      { oldPath, newName },
      { headers: this.getHeaders() }
    );
  }

  deleteFolder(bucket: string, prefix: string, permanent = false): Observable<{ success: boolean; deleted: number; prefix: string }> {
    return this.http.post<{ success: boolean; deleted: number; prefix: string }>(
      `${this.baseUrl}/api/v1/b/${bucket}/delete-folder`,
      { prefix, permanent },
      { headers: this.getHeaders() }
    );
  }

  listBuckets(projectId?: string): Observable<StorageBucket[]> {
    const pid = projectId || this.currentProject()?.id || 'p-default';
    let params = new HttpParams();
    if (pid) params = params.set('projectId', pid);

    return this.http.get<StorageBucket[]>(`${this.baseUrl}/api/v1/buckets`, {
      headers: this.getHeaders(),
      params
    });
  }

  createBucket(bucket: { name: string; description?: string; isPublic: boolean; projectId?: string }): Observable<StorageBucket> {
    const payload = {
      ...bucket,
      projectId: bucket.projectId || this.currentProject()?.id || 'p-default'
    };
    return this.http.post<StorageBucket>(`${this.baseUrl}/api/v1/buckets`, payload, {
      headers: this.getHeaders()
    });
  }

  listWebhooks(projectId?: string): Observable<WebhookConfig[]> {
    const pid = projectId || this.currentProject().id;
    let params = new HttpParams();
    if (pid) params = params.set('projectId', pid);

    return this.http.get<WebhookConfig[]>(`${this.baseUrl}/api/v1/webhooks`, {
      headers: this.getHeaders(),
      params
    });
  }

  registerWebhook(
    reqOrUrl: string | {
      projectId?: string;
      name?: string;
      url: string;
      events?: string[];
      secret?: string;
      enabled?: boolean;
    },
    events?: string[]
  ): Observable<WebhookConfig> {
    const payload = typeof reqOrUrl === 'string'
      ? {
          projectId: this.currentProject().id,
          name: 'Webhook Endpoint',
          url: reqOrUrl,
          events: events || ['*'],
          enabled: true
        }
      : {
          projectId: reqOrUrl.projectId || this.currentProject().id,
          name: reqOrUrl.name || 'Webhook Endpoint',
          url: reqOrUrl.url,
          events: reqOrUrl.events || ['*'],
          secret: reqOrUrl.secret,
          enabled: reqOrUrl.enabled !== undefined ? reqOrUrl.enabled : true
        };

    return this.http.post<WebhookConfig>(
      `${this.baseUrl}/api/v1/webhooks`,
      payload,
      { headers: this.getHeaders() }
    );
  }

  updateWebhook(id: string, req: Partial<WebhookConfig>): Observable<WebhookConfig> {
    return this.http.put<WebhookConfig>(
      `${this.baseUrl}/api/v1/webhooks/${id}`,
      req,
      { headers: this.getHeaders() }
    );
  }

  toggleWebhook(id: string, enabled: boolean): Observable<{ success: boolean; enabled: boolean; message: string }> {
    return this.http.post<{ success: boolean; enabled: boolean; message: string }>(
      `${this.baseUrl}/api/v1/webhooks/${id}/toggle`,
      { enabled },
      { headers: this.getHeaders() }
    );
  }

  testWebhook(id: string): Observable<TestWebhookResult> {
    return this.http.post<TestWebhookResult>(
      `${this.baseUrl}/api/v1/webhooks/${id}/test`,
      {},
      { headers: this.getHeaders() }
    );
  }

  listWebhookDeliveries(id: string, limit = 50): Observable<WebhookDelivery[]> {
    const params = new HttpParams().set('limit', limit.toString());
    return this.http.get<WebhookDelivery[]>(
      `${this.baseUrl}/api/v1/webhooks/${id}/deliveries`,
      {
        headers: this.getHeaders(),
        params
      }
    );
  }

  redeliverWebhook(id: string, deliveryId: string): Observable<TestWebhookResult> {
    return this.http.post<TestWebhookResult>(
      `${this.baseUrl}/api/v1/webhooks/${id}/deliveries/${deliveryId}/redeliver`,
      {},
      { headers: this.getHeaders() }
    );
  }

  deleteWebhook(id: string): Observable<{ success: boolean; message: string }> {
    return this.http.delete<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/webhooks/${id}`,
      { headers: this.getHeaders() }
    );
  }

  getDedupReport(): Observable<DedupReport> {
    return this.http.get<DedupReport>(`${this.baseUrl}/api/v1/admin/dedup-report`, {
      headers: this.getHeaders()
    });
  }

  getAuditLogs(limit = 50, offset = 0, action = ''): Observable<{ items: AuditLogEntry[]; totalCount: number }> {
    let params = new HttpParams()
      .set('limit', limit.toString())
      .set('offset', offset.toString());
    if (action) {
      params = params.set('action', action);
    }
    return this.http.get<{ items: AuditLogEntry[]; totalCount: number }>(`${this.baseUrl}/api/v1/admin/audit-logs`, {
      headers: this.getHeaders(),
      params
    });
  }

  runGC(): Observable<any> {
    return this.http.post(`${this.baseUrl}/api/v1/admin/gc`, {}, {
      headers: this.getHeaders()
    });
  }

  scrubIntegrity(bucket = 'default'): Observable<any> {
    let params = new HttpParams().set('bucket', bucket);
    return this.http.post(`${this.baseUrl}/api/v1/admin/scrub`, {}, {
      headers: this.getHeaders(),
      params
    });
  }

  downloadBackup(): Observable<Blob> {
    return this.http.post(
      `${this.baseUrl}/api/v1/admin/backup`,
      {},
      {
        headers: this.getHeaders(),
        responseType: 'blob'
      }
    );
  }

  restoreBackup(file: File): Observable<{ success: boolean; message: string }> {
    const formData = new FormData();
    formData.append('backup', file);
    return this.http.post<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/admin/restore`,
      formData,
      { headers: this.getHeaders() }
    );
  }

  updateBucket(name: string, bucket: Partial<StorageBucket>): Observable<StorageBucket> {
    return this.http.put<StorageBucket>(
      `${this.baseUrl}/api/v1/buckets/${name}`,
      bucket,
      { headers: this.getHeaders() }
    );
  }

  listAPIKeys(projectId?: string): Observable<APIKey[]> {
    const pid = projectId || this.currentProject().id;
    let params = new HttpParams();
    if (pid) params = params.set('projectId', pid);

    return this.http.get<APIKey[]>(`${this.baseUrl}/api/v1/keys`, {
      headers: this.getHeaders(),
      params
    });
  }

  createAPIKey(
    reqOrName: string | {
      projectId?: string;
      name: string;
      role?: string;
      permissions?: string[];
      allowedBuckets?: string[];
      allowedOrigins?: string[];
      rateLimitReqPerMin?: number;
      expiresInDays?: number;
    },
    role?: string
  ): Observable<APIKey> {
    const payload = typeof reqOrName === 'string'
      ? {
          projectId: this.currentProject().id,
          name: reqOrName,
          role: role || 'read-write',
          permissions: [],
          allowedBuckets: [],
          allowedOrigins: [],
          rateLimitReqPerMin: 0,
          expiresInDays: 0
        }
      : {
          projectId: reqOrName.projectId || this.currentProject().id,
          name: reqOrName.name,
          role: reqOrName.role || 'read-write',
          permissions: reqOrName.permissions || [],
          allowedBuckets: reqOrName.allowedBuckets || [],
          allowedOrigins: reqOrName.allowedOrigins || [],
          rateLimitReqPerMin: reqOrName.rateLimitReqPerMin || 0,
          expiresInDays: reqOrName.expiresInDays || 0
        };

    return this.http.post<APIKey>(
      `${this.baseUrl}/api/v1/keys`,
      payload,
      { headers: this.getHeaders() }
    );
  }

  rotateAPIKey(id: string): Observable<APIKey> {
    return this.http.post<APIKey>(
      `${this.baseUrl}/api/v1/keys/${id}/rotate`,
      {},
      { headers: this.getHeaders() }
    );
  }

  toggleRevokeAPIKey(id: string, revoked: boolean): Observable<{ success: boolean; revoked: boolean; message: string }> {
    return this.http.post<{ success: boolean; revoked: boolean; message: string }>(
      `${this.baseUrl}/api/v1/keys/${id}/toggle-revoke`,
      { revoked },
      { headers: this.getHeaders() }
    );
  }

  revokeAPIKey(id: string): Observable<{ success: boolean; message: string }> {
    return this.http.post<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/keys/${id}/revoke`,
      {},
      { headers: this.getHeaders() }
    );
  }

  deleteAPIKey(id: string): Observable<{ success: boolean; message: string }> {
    return this.http.delete<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/keys/${id}`,
      { headers: this.getHeaders() }
    );
  }

  createShareLink(req: { bucket: string; path: string; password?: string; maxDownloads?: number; expiresAt?: string }): Observable<ShareLink> {
    return this.http.post<ShareLink>(
      `${this.baseUrl}/api/v1/share`,
      req,
      { headers: this.getHeaders() }
    );
  }

  listShareLinks(bucket: string): Observable<ShareLink[]> {
    return this.http.get<ShareLink[]>(
      `${this.baseUrl}/api/v1/share/bucket/${bucket}`,
      { headers: this.getHeaders() }
    );
  }

  deleteShareLink(token: string): Observable<{ message: string }> {
    return this.http.delete<{ message: string }>(
      `${this.baseUrl}/api/v1/share/${token}`,
      { headers: this.getHeaders() }
    );
  }

  getLifecycleRule(bucket: string): Observable<LifecycleRule> {
    return this.http.get<LifecycleRule>(
      `${this.baseUrl}/api/v1/buckets/${bucket}/lifecycle`,
      { headers: this.getHeaders() }
    );
  }

  updateLifecycleRule(bucket: string, rule: Partial<LifecycleRule>): Observable<LifecycleRule> {
    return this.http.put<LifecycleRule>(
      `${this.baseUrl}/api/v1/buckets/${bucket}/lifecycle`,
      rule,
      { headers: this.getHeaders() }
    );
  }

  triggerLifecycleSweep(): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(
      `${this.baseUrl}/api/v1/lifecycle/sweep`,
      {},
      { headers: this.getHeaders() }
    );
  }

  triggerScrubberReport(): Observable<any> {
    return this.http.post<any>(
      `${this.baseUrl}/api/v1/lifecycle/scrubber`,
      {},
      { headers: this.getHeaders() }
    );
  }

  // Background Worker Pool & Job Queue Engine API
  getWorkerStats(): Observable<WorkerPoolStats> {
    return this.http.get<WorkerPoolStats>(
      `${this.baseUrl}/api/v1/workers/status`,
      { headers: this.getHeaders() }
    );
  }

  getWorkerJobs(limit = 50): Observable<WorkerJobRecord[]> {
    const params = new HttpParams().set('limit', limit.toString());
    return this.http.get<WorkerJobRecord[]>(
      `${this.baseUrl}/api/v1/workers/jobs`,
      {
        headers: this.getHeaders(),
        params
      }
    );
  }

  triggerWorkerJob(
    type: string,
    bucket = '',
    path = '',
    priority = 0,
    payload = ''
  ): Observable<{ success: boolean; jobId: string; message: string; type: string }> {
    return this.http.post<{ success: boolean; jobId: string; message: string; type: string }>(
      `${this.baseUrl}/api/v1/workers/trigger`,
      { type, bucket, path, priority, payload },
      { headers: this.getHeaders() }
    );
  }

  scaleWorkerPool(workers: number): Observable<{ success: boolean; totalWorkers: number; message: string }> {
    return this.http.post<{ success: boolean; totalWorkers: number; message: string }>(
      `${this.baseUrl}/api/v1/workers/scale`,
      { workers },
      { headers: this.getHeaders() }
    );
  }

  clearWorkerHistory(): Observable<{ success: boolean; message: string }> {
    return this.http.post<{ success: boolean; message: string }>(
      `${this.baseUrl}/api/v1/workers/clear-history`,
      {},
      { headers: this.getHeaders() }
    );
  }
}
