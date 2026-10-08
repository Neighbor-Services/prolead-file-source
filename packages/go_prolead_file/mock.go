package proleadfile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MockClient struct {
	mu             sync.RWMutex
	buckets        map[string]StorageBucket
	storage        map[string]map[string]mockObject
	shareLinks     map[string]ShareLink
	lifecycleRules map[string]LifecycleRule
	eventChan      chan StorageEvent
}

type mockObject struct {
	obj  StorageObject
	data []byte
}

func NewMockClient() *MockClient {
	m := &MockClient{
		buckets:        make(map[string]StorageBucket),
		storage:        make(map[string]map[string]mockObject),
		shareLinks:     make(map[string]ShareLink),
		lifecycleRules: make(map[string]LifecycleRule),
		eventChan:      make(chan StorageEvent, 50),
	}

	// Seed default bucket
	m.buckets["default"] = StorageBucket{
		ID:          "b-default",
		Name:        "default",
		Description: "Default Storage Bucket",
		IsPublic:    true,
		MaxFileSize: 104857600,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.storage["default"] = make(map[string]mockObject)

	return m
}

// -----------------------------------------------------------------------------
// BUCKET OPERATIONS
// -----------------------------------------------------------------------------

func (m *MockClient) ListBuckets(ctx context.Context) ([]StorageBucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []StorageBucket
	for _, b := range m.buckets {
		result = append(result, b)
	}
	return result, nil
}

func (m *MockClient) CreateBucket(ctx context.Context, bucket StorageBucket) (*StorageBucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := strings.ToLower(bucket.Name)
	if _, exists := m.buckets[clean]; exists {
		return nil, fmt.Errorf("bucket already exists: %s", clean)
	}

	bucket.ID = "b-" + clean
	bucket.Name = clean
	bucket.CreatedAt = time.Now()
	bucket.UpdatedAt = time.Now()

	m.buckets[clean] = bucket
	m.storage[clean] = make(map[string]mockObject)

	return &bucket, nil
}

func (m *MockClient) DeleteBucket(ctx context.Context, bucketName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := strings.ToLower(bucketName)
	delete(m.buckets, clean)
	delete(m.storage, clean)
	return nil
}

// -----------------------------------------------------------------------------
// OBJECT OPERATIONS
// -----------------------------------------------------------------------------

func (m *MockClient) UploadFile(ctx context.Context, bucket, objectPath string, reader io.Reader, contentType string, opts ...UploadOptions) (*StorageObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanPath := strings.TrimPrefix(objectPath, "/")
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	isPublic := true
	var metadata map[string]string
	if len(opts) > 0 {
		if opts[0].IsPublic != nil {
			isPublic = *opts[0].IsPublic
		}
		metadata = opts[0].Metadata
	}

	now := time.Now()
	obj := StorageObject{
		ID:            fmt.Sprintf("obj-%d", now.UnixNano()),
		Bucket:        bucket,
		Path:          cleanPath,
		Name:          filepath.Base(cleanPath),
		Size:          int64(len(data)),
		ContentType:   contentType,
		SHA256:        fmt.Sprintf("sha256-%d", len(data)),
		DownloadToken: fmt.Sprintf("tok-%d", now.UnixNano()),
		DownloadURL:   fmt.Sprintf("/v0/b/%s/o/%s?alt=media", bucket, url.PathEscape(cleanPath)),
		IsPublic:      isPublic,
		Metadata:      metadata,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if _, ok := m.storage[bucket]; !ok {
		m.storage[bucket] = make(map[string]mockObject)
	}
	m.storage[bucket][cleanPath] = mockObject{obj: obj, data: data}

	// Trigger event
	select {
	case m.eventChan <- StorageEvent{
		EventType:   "OBJECT_CREATED",
		Bucket:      bucket,
		Path:        cleanPath,
		Size:        obj.Size,
		ContentType: obj.ContentType,
		Timestamp:   now,
	}:
	default:
	}

	return &obj, nil
}

func (m *MockClient) UploadLocalFile(ctx context.Context, bucket, objectPath, localFilePath string, opts ...UploadOptions) (*StorageObject, error) {
	data, err := os.ReadFile(localFilePath)
	if err != nil {
		return nil, err
	}
	return m.UploadFile(ctx, bucket, objectPath, bytes.NewReader(data), "", opts...)
}

func (m *MockClient) CreateFolder(ctx context.Context, bucket, folderPath string) (*StorageObject, error) {
	clean := strings.Trim(folderPath, "/")
	return m.UploadFile(ctx, bucket, clean+"/.keep", bytes.NewReader([]byte{}), "application/octet-stream")
}

func (m *MockClient) ListFiles(ctx context.Context, filter ListFilesFilter) (*ListFilesResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucketStore, ok := m.storage[filter.Bucket]
	if !ok {
		return &ListFilesResult{Items: []StorageObject{}, Prefixes: []string{}, Total: 0}, nil
	}

	var items []StorageObject
	for _, entry := range bucketStore {
		if filter.TrashOnly && entry.obj.DeletedAt == nil {
			continue
		}
		if !filter.TrashOnly && entry.obj.DeletedAt != nil {
			continue
		}
		if filter.Prefix != "" && !strings.HasPrefix(entry.obj.Path, filter.Prefix) {
			continue
		}
		if filter.Search != "" && !strings.Contains(strings.ToLower(entry.obj.Path), strings.ToLower(filter.Search)) {
			continue
		}
		items = append(items, entry.obj)
	}

	total := int64(len(items))
	if filter.Offset > 0 && filter.Offset < len(items) {
		items = items[filter.Offset:]
	}
	if filter.Limit > 0 && len(items) > filter.Limit {
		items = items[:filter.Limit]
	}

	return &ListFilesResult{
		Items:    items,
		Prefixes: []string{},
		Total:    total,
	}, nil
}

func (m *MockClient) GetFileMetadata(ctx context.Context, bucket, objectPath string) (*StorageObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanPath := strings.TrimPrefix(objectPath, "/")
	entry, ok := m.storage[bucket][cleanPath]
	if !ok || entry.obj.DeletedAt != nil {
		return nil, fmt.Errorf("file not found: %s", objectPath)
	}
	objCopy := entry.obj
	return &objCopy, nil
}

func (m *MockClient) DownloadBytes(ctx context.Context, bucket, objectPath string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanPath := strings.TrimPrefix(objectPath, "/")
	entry, ok := m.storage[bucket][cleanPath]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", objectPath)
	}
	return entry.data, nil
}

func (m *MockClient) DeleteFile(ctx context.Context, bucket, objectPath string, permanent bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanPath := strings.TrimPrefix(objectPath, "/")
	if permanent {
		delete(m.storage[bucket], cleanPath)
	} else {
		entry, ok := m.storage[bucket][cleanPath]
		if ok {
			now := time.Now()
			entry.obj.DeletedAt = &now
			m.storage[bucket][cleanPath] = entry
		}
	}
	return nil
}

func (m *MockClient) RestoreFile(ctx context.Context, bucket, objectPath string) (*StorageObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanPath := strings.TrimPrefix(objectPath, "/")
	entry, ok := m.storage[bucket][cleanPath]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", objectPath)
	}
	entry.obj.DeletedAt = nil
	m.storage[bucket][cleanPath] = entry
	return &entry.obj, nil
}

func (m *MockClient) GenerateSignedURL(ctx context.Context, bucket, objectPath string, durationSeconds int) (*SignedUrlResult, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(durationSeconds) * time.Second)
	return &SignedUrlResult{
		SignedURL: fmt.Sprintf("/v0/b/%s/o/%s?sig=mockhmac&expires=%d", bucket, url.PathEscape(objectPath), expiresAt.Unix()),
		ExpiresAt: expiresAt,
	}, nil
}

func (m *MockClient) GetTransformedImageURL(rawDownloadURL string, opts ImageTransformOptions) string {
	u, err := url.Parse(rawDownloadURL)
	if err != nil {
		return rawDownloadURL
	}
	q := u.Query()
	if opts.Width > 0 {
		q.Set("w", strconv.Itoa(opts.Width))
	}
	if opts.Height > 0 {
		q.Set("h", strconv.Itoa(opts.Height))
	}
	if opts.Fit != "" {
		q.Set("fit", opts.Fit)
	}
	if opts.Format != "" {
		q.Set("format", opts.Format)
	}
	if opts.Quality > 0 {
		q.Set("q", strconv.Itoa(opts.Quality))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// -----------------------------------------------------------------------------
// SHARE LINKS
// -----------------------------------------------------------------------------

func (m *MockClient) CreateShareLink(ctx context.Context, bucket, objectPath string, durationHours int, password string, maxDownloads int) (*ShareLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token := fmt.Sprintf("sh-%d", time.Now().UnixNano())
	var expiresAt *time.Time
	if durationHours > 0 {
		t := time.Now().Add(time.Duration(durationHours) * time.Hour)
		expiresAt = &t
	}

	link := ShareLink{
		ID:              fmt.Sprintf("link-%d", time.Now().UnixNano()),
		Bucket:          bucket,
		Path:            objectPath,
		Token:           token,
		RequirePassword: password != "",
		Password:        password,
		MaxDownloads:    maxDownloads,
		DownloadCount:   0,
		ExpiresAt:       expiresAt,
		DownloadURL:     fmt.Sprintf("/api/v1/public/share/%s", token),
		CreatedAt:       time.Now(),
	}

	m.shareLinks[token] = link
	return &link, nil
}

func (m *MockClient) ListShareLinks(ctx context.Context, bucket string) ([]ShareLink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ShareLink
	for _, l := range m.shareLinks {
		if bucket == "" || l.Bucket == bucket {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *MockClient) RevokeShareLink(ctx context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.shareLinks, token)
	return nil
}

// -----------------------------------------------------------------------------
// LIFECYCLE RULES
// -----------------------------------------------------------------------------

func (m *MockClient) GetLifecycleRules(ctx context.Context, bucket string) ([]LifecycleRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []LifecycleRule
	for _, r := range m.lifecycleRules {
		if bucket == "" || r.Bucket == bucket {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *MockClient) CreateLifecycleRule(ctx context.Context, rule LifecycleRule) (*LifecycleRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule.ID = fmt.Sprintf("lc-%d", time.Now().UnixNano())
	m.lifecycleRules[rule.ID] = rule
	return &rule, nil
}

func (m *MockClient) DeleteLifecycleRule(ctx context.Context, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.lifecycleRules, ruleID)
	return nil
}

// -----------------------------------------------------------------------------
// TELEMETRY & EVENTS
// -----------------------------------------------------------------------------

func (m *MockClient) GetStats(ctx context.Context) (*StorageStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalFiles, totalBytes int64
	for _, bucketStore := range m.storage {
		for _, entry := range bucketStore {
			totalFiles++
			totalBytes += int64(len(entry.data))
		}
	}

	return &StorageStats{
		TotalFiles:         totalFiles,
		TotalLogicalBytes:  totalBytes,
		TotalPhysicalBytes: int64(float64(totalBytes) * 0.75),
		TotalSavedBytes:    int64(float64(totalBytes) * 0.25),
		DedupRatio:         1.33,
		ActiveBuckets:      int64(len(m.buckets)),
		ActiveVersions:     1,
		TrashFiles:         0,
	}, nil
}

func (m *MockClient) SubscribeEvents(ctx context.Context) (<-chan StorageEvent, <-chan error, error) {
	errChan := make(chan error)
	return m.eventChan, errChan, nil
}

func (m *MockClient) UploadTUS(ctx context.Context, bucket, objectPath string, reader io.ReaderAt, size int64, opts ...TUSUploadOptions) (*StorageObject, error) {
	buf := make([]byte, size)
	_, err := reader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return m.UploadFile(ctx, bucket, objectPath, bytes.NewReader(buf), "application/octet-stream")
}

func (m *MockClient) ExportAuditLogs(ctx context.Context, format string, writer io.Writer) error {
	_, err := writer.Write([]byte("id,user,action,timestamp\n1,admin,MOCK_ACTION," + time.Now().Format(time.RFC3339) + "\n"))
	return err
}

func (m *MockClient) TriggerGC(ctx context.Context) (*GCReport, error) {
	return &GCReport{DeletedBlobs: 2, FreedBytes: 1048576}, nil
}

func (m *MockClient) GetDedupReport(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{
		"totalLogicalBytes":  10485760,
		"totalPhysicalBytes": 5242880,
		"dedupRatio":         2.0,
	}, nil
}

