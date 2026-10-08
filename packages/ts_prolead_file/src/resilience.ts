export interface RetryOptions {
  maxRetries?: number;
  initialDelayMs?: number;
  maxDelayMs?: number;
  backoffFactor?: number;
  retryIf?: (error: any) => boolean;
}

/**
 * Execute an async operation with exponential backoff and jitter.
 */
export async function executeWithRetry<T>(
  fn: () => Promise<T>,
  options: RetryOptions = {}
): Promise<T> {
  const maxRetries = options.maxRetries ?? 3;
  const initialDelayMs = options.initialDelayMs ?? 300;
  const maxDelayMs = options.maxDelayMs ?? 10000;
  const backoffFactor = options.backoffFactor ?? 2.0;

  let attempt = 0;
  let delay = initialDelayMs;

  while (true) {
    try {
      attempt++;
      return await fn();
    } catch (error: any) {
      if (attempt > maxRetries) {
        throw error;
      }

      if (options.retryIf && !options.retryIf(error)) {
        throw error;
      }

      // Add full jitter (0 to 100ms)
      const jitter = Math.random() * 100;
      await new Promise((res) => setTimeout(res, delay + jitter));

      delay = Math.min(delay * backoffFactor, maxDelayMs);
    }
  }
}

export interface ChunkProgress {
  chunkIndex: number;
  totalChunks: number;
  bytesUploaded: number;
  totalBytes: number;
  percent: number;
}

/**
 * Resilient chunked uploader helper for large files with pause and retry capabilities.
 */
export class ResilientUploader {
  private aborted = false;
  private paused = false;

  constructor(
    private fileData: Uint8Array | ArrayBuffer,
    private chunkSize: number = 5 * 1024 * 1024 // 5MB default chunk
  ) {}

  public pause(): void {
    this.paused = true;
  }

  public resume(): void {
    this.paused = false;
  }

  public abort(): void {
    this.aborted = true;
  }

  public async upload(
    uploadChunkFn: (chunk: Uint8Array, index: number, total: number) => Promise<void>,
    onProgress?: (progress: ChunkProgress) => void
  ): Promise<void> {
    const buffer = this.fileData instanceof Uint8Array ? this.fileData : new Uint8Array(this.fileData);
    const totalBytes = buffer.byteLength;
    const totalChunks = Math.max(1, Math.ceil(totalBytes / this.chunkSize));

    let bytesUploaded = 0;

    for (let i = 0; i < totalChunks; i++) {
      if (this.aborted) {
        throw new Error('Upload aborted by user');
      }

      while (this.paused) {
        await new Promise((resolve) => setTimeout(resolve, 200));
        if (this.aborted) throw new Error('Upload aborted while paused');
      }

      const start = i * this.chunkSize;
      const end = Math.min(start + this.chunkSize, totalBytes);
      const chunk = buffer.subarray(start, end);

      await executeWithRetry(
        async () => {
          await uploadChunkFn(chunk, i, totalChunks);
        },
        { maxRetries: 4, initialDelayMs: 500 }
      );

      bytesUploaded += chunk.byteLength;
      if (onProgress) {
        onProgress({
          chunkIndex: i,
          totalChunks,
          bytesUploaded,
          totalBytes,
          percent: Math.min(100, Math.round((bytesUploaded / totalBytes) * 100)),
        });
      }
    }
  }
}
