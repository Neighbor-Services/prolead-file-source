export interface StorageObject {
  kind: string;
  id: string;
  name: string;
  path: string;
  bucket: string;
  size: number;
  contentType: string;
  md5Hash?: string;
  sha256Hash?: string;
  downloadToken: string;
  downloadUrl: string;
  isPublic: boolean;
  version?: number;
  expiresAt?: string;
  deletedAt?: string;
  metadata?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface FileVersion {
  id: string;
  fileId: string;
  version: number;
  bucket: string;
  path: string;
  size: number;
  contentType: string;
  md5Hash?: string;
  sha256Hash?: string;
  createdAt: string;
}

export interface WebhookConfig {
  id: string;
  projectId?: string;
  name?: string;
  url: string;
  secret?: string;
  events: string[];
  enabled: boolean;
  lastDeliveryAt?: string;
  lastStatusCode?: number;
  successCount?: number;
  failureCount?: number;
  createdAt: string;
  updatedAt?: string;
}

export interface WebhookDelivery {
  id: string;
  webhookId: string;
  projectId?: string;
  event: string;
  url: string;
  statusCode: number;
  durationMs: number;
  success: boolean;
  error?: string;
  requestBody?: string;
  responseBody?: string;
  createdAt: string;
}

export interface TestWebhookResult {
  success: boolean;
  statusCode: number;
  durationMs: number;
  requestBody: string;
  responseBody: string;
  signature: string;
  error?: string;
  deliveryId: string;
}

export interface ListFilesResult {
  items: StorageObject[];
  prefixes?: string[];
  totalCount: number;
  nextPageToken?: string;
}

export interface Project {
  id: string;
  name: string;
  slug: string;
  description?: string;
  createdAt: string;
  updatedAt: string;
}

export interface UserProfile {
  id: string;
  username: string;
  email?: string;
  role?: string;
  isSuperuser: boolean;
  twoFactorEnabled?: boolean;
  status?: string;
}

export interface StorageBucket {
  id: string;
  projectId?: string;
  name: string;
  description?: string;
  isPublic: boolean;
  maxFileSize?: number;
  allowedMimes?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface StorageStats {
  totalFiles: number;
  totalBytes: number;
  totalBuckets: number;
  totalTrash?: number;
}

export interface UploadProgressEvent {
  status: 'uploading' | 'completed' | 'error';
  progress: number; // 0 to 100%
  result?: StorageObject;
  error?: string;
}

export interface ImageTransformOptions {
  width?: number;
  height?: number;
  fit?: 'cover' | 'contain' | 'fill' | 'scale';
  format?: 'webp' | 'jpeg' | 'png' | 'gif';
  quality?: number; // 1 to 100
}

export interface SignedUrlResult {
  signedUrl: string;
  expires: number;
  signature: string;
}

export interface StorageEvent {
  type: 'file:uploaded' | 'file:deleted' | 'file:restored' | 'token:rotated' | 'bucket:created';
  bucket: string;
  path?: string;
  payload?: any;
  timestamp: string;
}

export interface DedupReport {
  totalVirtualFiles: number;
  uniqueBlobs: number;
  virtualBytes: number;
  physicalBytes: number;
  bytesSaved: number;
  savingsPercent: number;
}

export interface AuditLogEntry {
  id: string;
  action: string;
  bucket: string;
  path?: string;
  actor: string;
  ipAddress: string;
  userAgent: string;
  status: number;
  durationMs: number;
  createdAt: string;
}

export interface APIKey {
  id: string;
  projectId: string;
  key: string;
  keyPrefix?: string;
  name: string;
  role: 'admin' | 'read-write' | 'read-only' | string;
  permissions?: string[];
  allowedBuckets?: string[];
  allowedOrigins?: string[];
  rateLimitReqPerMin?: number;
  requestCount?: number;
  lastUsedAt?: string;
  expiresAt?: string;
  revoked: boolean;
  createdBy?: string;
  createdAt: string;
  updatedAt?: string;
}

export interface Feature50Item {
  id: number;
  category: string;
  title: string;
  description: string;
  endpoint: string;
  status: 'active' | 'tested';
}

export interface ShareLink {
  id: string;
  token: string;
  bucket: string;
  path: string;
  hasPassword: boolean;
  maxDownloads: number;
  downloadCount: number;
  expiresAt?: string;
  shareUrl: string;
  createdAt: string;
}

export interface LifecycleRule {
  id?: string;
  bucket: string;
  prefix?: string;
  trashDays: number;
  versionLimit: number;
  expirationDays: number;
  enabled: boolean;
}

export type JobType = 
  | 'THUMBNAIL_PREGEN'
  | 'METADATA_EXTRACT'
  | 'INTEGRITY_SCRUB'
  | 'CAS_DEDUP'
  | 'GARBAGE_COLLECT'
  | 'LIFECYCLE_PURGE'
  | 'WEBHOOK_RETRY'
  | 'BACKUP_SNAPSHOT';

export type JobStatus = 
  | 'QUEUED'
  | 'PROCESSING'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED';

export interface WorkerJobRecord {
  id: string;
  type: JobType;
  status: JobStatus;
  priority: number;
  bucket?: string;
  path?: string;
  payload?: string;
  workerId: number;
  progress: number;
  error?: string;
  durationMs: number;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface StaffUser {
  id: string;
  username: string;
  email?: string;
  role: 'superadmin' | 'admin' | 'operator' | 'viewer';
  isSuperuser: boolean;
  twoFactorEnabled: boolean;
  status: 'active' | 'suspended';
  lastLoginAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface TwoFactorSetupResult {
  secret: string;
  otpAuthUri: string;
  recoveryCodes: string[];
}

export interface LoginResult {
  token?: string;
  user?: StaffUser;
  isAdmin: boolean;
  require2fa?: boolean;
  tempToken?: string;
}

export interface WorkerPoolStats {
  activeWorkers: number;
  totalWorkers: number;
  maxWorkers: number;
  queueLength: number;
  queueCapacity: number;
  jobsProcessed: number;
  jobsSucceeded: number;
  jobsFailed: number;
  uptimeSeconds: number;
  throughputPerMin: number;
  avgDurationMs: number;
}



