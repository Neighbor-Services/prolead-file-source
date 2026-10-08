export interface ProleadFileObject {
  id: string;
  bucket: string;
  path: string;
  name: string;
  size: number;
  contentType: string;
  sha256: string;
  downloadToken: string;
  downloadUrl: string;
  isPublic: boolean;
  expiresAt?: string;
  deletedAt?: string;
  metadata?: Record<string, any>;
  version: number;
  lqip?: string;
  isCompressed?: boolean;
  originalSize?: number;
  compressedSize?: number;
  compressionRatio?: number;
  savingsPercent?: number;
  createdAt: string;
  updatedAt: string;
}

export interface ProleadFileVersion {
  id: string;
  bucket: string;
  path: string;
  version: number;
  size: number;
  sha256: string;
  downloadToken: string;
  createdAt: string;
}

export interface ProleadBucket {
  id: string;
  name: string;
  description: string;
  isPublic: boolean;
  maxFileSize: number;
  allowedMimes: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ProleadStats {
  totalFiles: number;
  totalLogicalBytes: number;
  totalPhysicalBytes: number;
  totalSavedBytes: number;
  dedupRatio: number;
  activeBuckets: number;
  activeVersions: number;
  trashFiles: number;
}

export interface ListFilesOptions {
  prefix?: string;
  delimiter?: string;
  limit?: number;
  offset?: number;
  search?: string;
  trashOnly?: boolean;
}

export interface ListFilesResult {
  items: ProleadFileObject[];
  prefixes: string[];
  total: number;
}

export interface ProleadImageTransform {
  width?: number;
  height?: number;
  fit?: 'cover' | 'contain' | 'fill' | 'scale';
  format?: 'webp' | 'jpeg' | 'png' | 'gif';
  quality?: number;
}

export interface SignedUrlResult {
  signedUrl: string;
  expiresAt: string;
}

export interface UploadOptions {
  bucket?: string;
  path: string;
  isPublic?: boolean;
  expiresInSeconds?: number;
  metadata?: Record<string, string>;
  contentType?: string;
  onProgress?: (progress: { loaded: number; total: number; percent: number }) => void;
}

export interface TUSUploadOptions {
  bucket?: string;
  path: string;
  chunkSize?: number;
  metadata?: Record<string, string>;
  isPublic?: boolean;
  onProgress?: (progress: { loaded: number; total: number; percent: number }) => void;
}

export interface ShareLink {
  id: string;
  bucket: string;
  path: string;
  token: string;
  requirePassword: boolean;
  password?: string;
  maxDownloads: number;
  downloadCount: number;
  expiresAt?: string;
  downloadUrl: string;
  createdAt: string;
}

export interface CreateShareOptions {
  durationHours?: number;
  password?: string;
  maxDownloads?: number;
}

export interface GCReport {
  deletedBlobs: number;
  freedBytes: number;
}

export interface DedupReport {
  totalLogicalBytes: number;
  totalPhysicalBytes: number;
  totalSavedBytes: number;
  dedupRatio: number;
  activeBlobs: number;
  filesReferenced: number;
}

export interface ProleadEvent {
  eventType: string;
  bucket: string;
  path: string;
  size: number;
  contentType: string;
  timestamp: string;
}

export interface ClientConfig {
  baseUrl: string;
  apiKey?: string;
}

