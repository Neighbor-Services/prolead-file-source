package proleadfile

import (
	"context"
	"io"
)

// ClientInterface defines the contract for Prolead File SDK operations.
type ClientInterface interface {
	// Buckets
	ListBuckets(ctx context.Context) ([]StorageBucket, error)
	CreateBucket(ctx context.Context, bucket StorageBucket) (*StorageBucket, error)
	DeleteBucket(ctx context.Context, bucketName string) error

	// Objects
	UploadFile(ctx context.Context, bucket, objectPath string, reader io.Reader, contentType string, opts ...UploadOptions) (*StorageObject, error)
	UploadLocalFile(ctx context.Context, bucket, objectPath, localFilePath string, opts ...UploadOptions) (*StorageObject, error)
	CreateFolder(ctx context.Context, bucket, folderPath string) (*StorageObject, error)
	ListFiles(ctx context.Context, filter ListFilesFilter) (*ListFilesResult, error)
	GetFileMetadata(ctx context.Context, bucket, objectPath string) (*StorageObject, error)
	DownloadBytes(ctx context.Context, bucket, objectPath string) ([]byte, error)
	DeleteFile(ctx context.Context, bucket, objectPath string, permanent bool) error
	RestoreFile(ctx context.Context, bucket, objectPath string) (*StorageObject, error)
	GenerateSignedURL(ctx context.Context, bucket, objectPath string, durationSeconds int) (*SignedUrlResult, error)
	GetTransformedImageURL(rawDownloadURL string, opts ImageTransformOptions) string

	// Share Links
	CreateShareLink(ctx context.Context, bucket, objectPath string, durationHours int, password string, maxDownloads int) (*ShareLink, error)
	ListShareLinks(ctx context.Context, bucket string) ([]ShareLink, error)
	RevokeShareLink(ctx context.Context, token string) error

	// Lifecycle Rules
	GetLifecycleRules(ctx context.Context, bucket string) ([]LifecycleRule, error)
	CreateLifecycleRule(ctx context.Context, rule LifecycleRule) (*LifecycleRule, error)
	DeleteLifecycleRule(ctx context.Context, ruleID string) error

	// Telemetry & Events
	GetStats(ctx context.Context) (*StorageStats, error)
	SubscribeEvents(ctx context.Context) (<-chan StorageEvent, <-chan error, error)

	// Resumable Upload (TUS 1.0.0)
	UploadTUS(ctx context.Context, bucket, objectPath string, reader io.ReaderAt, size int64, opts ...TUSUploadOptions) (*StorageObject, error)

	// Admin & Operations
	ExportAuditLogs(ctx context.Context, format string, writer io.Writer) error
	TriggerGC(ctx context.Context) (*GCReport, error)
	GetDedupReport(ctx context.Context) (map[string]interface{}, error)
}
