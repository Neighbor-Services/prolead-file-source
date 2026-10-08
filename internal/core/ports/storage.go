package ports

import (
	"context"
	"io"
)

// ReadSeekCloser combines io.Reader, io.Seeker, and io.Closer
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// StorageDriver defines the contract for physical object storage persistence
type StorageDriver interface {
	// Save streams data to storage atomically, returning total bytes and computed hashes
	Save(ctx context.Context, bucket, objectPath string, reader io.Reader) (bytesWritten int64, md5Sum, sha256Sum string, err error)

	// Open opens the stored file for reading and seeking (enabling HTTP range requests)
	Open(ctx context.Context, bucket, objectPath string) (ReadSeekCloser, int64, error)

	// Delete removes the physical file from the storage backend
	Delete(ctx context.Context, bucket, objectPath string) error

	// Exists checks if the physical file exists
	Exists(ctx context.Context, bucket, objectPath string) (bool, error)

	// DeleteBucket removes the entire bucket directory
	DeleteBucket(ctx context.Context, bucket string) error
}
