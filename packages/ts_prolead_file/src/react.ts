import type { ProleadFileClient } from './client.js';
import type { ProleadBucket, ProleadFileObject, UploadOptions } from './types.js';

export interface UseProleadUploadState {
  isUploading: boolean;
  progress: number;
  error: Error | null;
  result: ProleadFileObject | null;
}

export interface UseProleadBucketState {
  buckets: ProleadBucket[];
  isLoading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

export interface UseProleadFilesState {
  files: ProleadFileObject[];
  prefixes: string[];
  total: number;
  isLoading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

/**
 * Creates reusable controller hooks compatible with React, Preact, Vue, and Svelte component state.
 */
export function createProleadUploadController(client: ProleadFileClient) {
  return async (
    file: Blob | Buffer | Uint8Array | string,
    options: UploadOptions,
    onProgressUpdate?: (percent: number) => void
  ): Promise<ProleadFileObject> => {
    return client.upload(file, {
      ...options,
      onProgress: (p) => {
        if (options.onProgress) options.onProgress(p);
        if (onProgressUpdate) onProgressUpdate(p.percent);
      },
    });
  };
}

/**
 * Controller helper to manage reactive file lists with pagination and prefix filters.
 */
export function createProleadFilesController(client: ProleadFileClient, bucket: string = 'default') {
  return async (options?: { prefix?: string; search?: string; limit?: number }) => {
    return client.listFiles(bucket, options);
  };
}

export interface ProleadImageBlurUpProps {
  src: string;
  lqip?: string;
  alt?: string;
  width?: number;
  height?: number;
  className?: string;
}

/**
 * Computes image style tokens for smooth LQIP blur-up transitions.
 */
export function getProleadBlurUpStyles(lqip?: string, isLoaded: boolean = false) {
  return {
    backgroundImage: lqip ? `url(${lqip})` : 'none',
    backgroundSize: 'cover',
    backgroundPosition: 'center',
    filter: isLoaded ? 'none' : 'blur(10px)',
    transition: 'filter 0.3s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.3s ease',
    opacity: isLoaded ? 1 : 0.85,
  };
}


