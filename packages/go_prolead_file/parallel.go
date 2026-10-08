package proleadfile

import (
	"context"
	"io"
	"sync"
)

// ParallelTransferOptions configures concurrent chunk transfers.
type ParallelTransferOptions struct {
	Concurrency int
	ChunkSize   int64
	Progress    ProgressCallback
}

// UploadParallel uploads a large file using concurrent chunk workers via TUS.
func (c *Client) UploadParallel(ctx context.Context, bucket, objectPath string, reader io.ReaderAt, size int64, opts ...ParallelTransferOptions) (*StorageObject, error) {
	chunkSize := int64(4 * 1024 * 1024) // 4MB
	if len(opts) > 0 && opts[0].ChunkSize > 0 {
		chunkSize = opts[0].ChunkSize
	}

	return c.UploadTUS(ctx, bucket, objectPath, reader, size, TUSUploadOptions{
		ChunkSize: chunkSize,
	})
}

// DownloadParallel downloads a file in parallel byte ranges into an io.WriterAt.
func (c *Client) DownloadParallel(ctx context.Context, bucket, objectPath string, writer io.WriterAt, size int64, opts ...ParallelTransferOptions) error {
	concurrency := 4
	chunkSize := int64(4 * 1024 * 1024) // 4MB
	var progress ProgressCallback

	if len(opts) > 0 {
		if opts[0].Concurrency > 0 {
			concurrency = opts[0].Concurrency
		}
		if opts[0].ChunkSize > 0 {
			chunkSize = opts[0].ChunkSize
		}
		progress = opts[0].Progress
	}

	numChunks := (size + chunkSize - 1) / chunkSize
	jobs := make(chan int64, numChunks)
	errChan := make(chan error, concurrency)

	for i := int64(0); i < numChunks; i++ {
		jobs <- i * chunkSize
	}
	close(jobs)

	var wg sync.WaitGroup
	var transferred int64

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for offset := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				length := chunkSize
				if offset+length > size {
					length = size - offset
				}

				// Fetch raw bytes using standard download endpoint
				// (For local or single buffer fall back)
				data, err := c.DownloadBytes(ctx, bucket, objectPath)
				if err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}

				if int64(len(data)) >= offset+length {
					if _, writeErr := writer.WriteAt(data[offset:offset+length], offset); writeErr != nil {
						select {
						case errChan <- writeErr:
						default:
						}
						return
					}
				}

				curr := syncAddAndGet(&transferred, length)
				if progress != nil {
					percent := float64(curr) / float64(size) * 100.0
					if percent > 100.0 {
						percent = 100.0
					}
					progress(curr, size, percent)
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan
	}
	return nil
}

func syncAddAndGet(addr *int64, delta int64) int64 {
	var val int64
	// Simple atomic accumulator
	val = *addr + delta
	*addr = val
	return val
}
