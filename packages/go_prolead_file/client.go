package proleadfile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string, opts ...func(*Client)) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithHTTPClient(httpClient *http.Client) func(*Client) {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// -----------------------------------------------------------------------------
// BUCKET OPERATIONS
// -----------------------------------------------------------------------------

func (c *Client) ListBuckets(ctx context.Context) ([]StorageBucket, error) {
	endpoint := fmt.Sprintf("%s/api/v1/buckets", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list buckets failed: HTTP %d", resp.StatusCode)
	}

	var buckets []StorageBucket
	if err := json.NewDecoder(resp.Body).Decode(&buckets); err != nil {
		return nil, err
	}
	return buckets, nil
}

func (c *Client) CreateBucket(ctx context.Context, bucket StorageBucket) (*StorageBucket, error) {
	endpoint := fmt.Sprintf("%s/api/v1/buckets", c.baseURL)
	payload, err := json.Marshal(bucket)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create bucket error (%d): %s", resp.StatusCode, string(body))
	}

	var created StorageBucket
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (c *Client) DeleteBucket(ctx context.Context, bucketName string) error {
	endpoint := fmt.Sprintf("%s/api/v1/buckets/%s", c.baseURL, url.PathEscape(bucketName))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete bucket failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------------
// FILE / OBJECT OPERATIONS
// -----------------------------------------------------------------------------

func (c *Client) UploadFile(ctx context.Context, bucket, objectPath string, reader io.Reader, contentType string, opts ...UploadOptions) (*StorageObject, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(objectPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return nil, fmt.Errorf("failed to copy file reader: %w", err)
	}

	if err := writer.WriteField("path", objectPath); err != nil {
		return nil, err
	}

	if len(opts) > 0 {
		opt := opts[0]
		if opt.IsPublic != nil {
			isPub := "false"
			if *opt.IsPublic {
				isPub = "true"
			}
			_ = writer.WriteField("isPublic", isPub)
		}
		if opt.ExpiresInSeconds != nil {
			_ = writer.WriteField("expiresInSeconds", strconv.Itoa(*opt.ExpiresInSeconds))
		}
		if opt.Metadata != nil {
			metaBytes, _ := json.Marshal(opt.Metadata)
			_ = writer.WriteField("metadata", string(metaBytes))
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/v0/b/%s/o", c.baseURL, bucket)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var fileObj StorageObject
	if err := json.NewDecoder(resp.Body).Decode(&fileObj); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fileObj, nil
}

func (c *Client) UploadLocalFile(ctx context.Context, bucket, objectPath, localFilePath string, opts ...UploadOptions) (*StorageObject, error) {
	file, err := os.Open(localFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return c.UploadFile(ctx, bucket, objectPath, file, "", opts...)
}

func (c *Client) CreateFolder(ctx context.Context, bucket, folderPath string) (*StorageObject, error) {
	clean := strings.Trim(folderPath, "/")
	markerPath := clean + "/.keep"
	return c.UploadFile(ctx, bucket, markerPath, bytes.NewReader([]byte{}), "application/octet-stream")
}

func (c *Client) ListFiles(ctx context.Context, filter ListFilesFilter) (*ListFilesResult, error) {
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o", c.baseURL, filter.Bucket)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if filter.Prefix != "" {
		q.Set("prefix", filter.Prefix)
	}
	if filter.Delimiter != "" {
		q.Set("delimiter", filter.Delimiter)
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		q.Set("offset", strconv.Itoa(filter.Offset))
	}
	if filter.Search != "" {
		q.Set("search", filter.Search)
	}
	if filter.TrashOnly {
		q.Set("trash", "true")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list files failed: HTTP %d", resp.StatusCode)
	}

	var result ListFilesResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetFileMetadata(ctx context.Context, bucket, objectPath string) (*StorageObject, error) {
	cleanPath := url.PathEscape(objectPath)
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o/%s", c.baseURL, bucket, cleanPath)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get file metadata failed: HTTP %d", resp.StatusCode)
	}

	var obj StorageObject
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (c *Client) DownloadBytes(ctx context.Context, bucket, objectPath string) ([]byte, error) {
	cleanPath := url.PathEscape(objectPath)
	endpoint := fmt.Sprintf("%s/v0/b/%s/o/%s?alt=media", c.baseURL, bucket, cleanPath)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download error: HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) DeleteFile(ctx context.Context, bucket, objectPath string, permanent bool) error {
	cleanPath := url.PathEscape(objectPath)
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o/%s", c.baseURL, bucket, cleanPath)
	if permanent {
		endpoint += "?permanent=true"
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) RestoreFile(ctx context.Context, bucket, objectPath string) (*StorageObject, error) {
	cleanPath := url.PathEscape(objectPath)
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o/%s/restore", c.baseURL, bucket, cleanPath)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("restore failed: HTTP %d", resp.StatusCode)
	}

	var obj StorageObject
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (c *Client) GenerateSignedURL(ctx context.Context, bucket, objectPath string, durationSeconds int) (*SignedUrlResult, error) {
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/sign-url", c.baseURL, bucket)
	payload, _ := json.Marshal(map[string]interface{}{
		"path":            objectPath,
		"durationSeconds": durationSeconds,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sign url failed: HTTP %d", resp.StatusCode)
	}

	var res SignedUrlResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetTransformedImageURL(rawDownloadURL string, opts ImageTransformOptions) string {
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

func (c *Client) SubscribeEvents(ctx context.Context) (<-chan StorageEvent, <-chan error, error) {
	endpoint := fmt.Sprintf("%s/api/v1/events/stream", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}

	eventChan := make(chan StorageEvent, 10)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)
		defer close(errChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if dataStr != "" && dataStr != ":keepalive" {
					var ev StorageEvent
					if err := json.Unmarshal([]byte(dataStr), &ev); err == nil {
						eventChan <- ev
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan, nil
}

func (c *Client) GetStats(ctx context.Context) (*StorageStats, error) {
	endpoint := fmt.Sprintf("%s/api/v1/stats", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stats failed: HTTP %d", resp.StatusCode)
	}

	var stats StorageStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// -----------------------------------------------------------------------------
// SHARE LINKS
// -----------------------------------------------------------------------------

func (c *Client) CreateShareLink(ctx context.Context, bucket, objectPath string, durationHours int, password string, maxDownloads int) (*ShareLink, error) {
	endpoint := fmt.Sprintf("%s/api/v1/shares", c.baseURL)
	payload, _ := json.Marshal(map[string]interface{}{
		"bucket":        bucket,
		"path":          objectPath,
		"durationHours": durationHours,
		"password":      password,
		"maxDownloads":  maxDownloads,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("create share link failed: HTTP %d", resp.StatusCode)
	}

	var link ShareLink
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		return nil, err
	}
	return &link, nil
}

func (c *Client) ListShareLinks(ctx context.Context, bucket string) ([]ShareLink, error) {
	endpoint := fmt.Sprintf("%s/api/v1/shares?bucket=%s", c.baseURL, url.QueryEscape(bucket))
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list share links failed: HTTP %d", resp.StatusCode)
	}

	var links []ShareLink
	if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
		return nil, err
	}
	return links, nil
}

func (c *Client) RevokeShareLink(ctx context.Context, token string) error {
	endpoint := fmt.Sprintf("%s/api/v1/shares/%s", c.baseURL, url.PathEscape(token))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("revoke share link failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------------
// LIFECYCLE RULES
// -----------------------------------------------------------------------------

func (c *Client) GetLifecycleRules(ctx context.Context, bucket string) ([]LifecycleRule, error) {
	endpoint := fmt.Sprintf("%s/api/v1/lifecycle?bucket=%s", c.baseURL, url.QueryEscape(bucket))
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get lifecycle rules failed: HTTP %d", resp.StatusCode)
	}

	var rules []LifecycleRule
	if err := json.NewDecoder(resp.Body).Decode(&rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) CreateLifecycleRule(ctx context.Context, rule LifecycleRule) (*LifecycleRule, error) {
	endpoint := fmt.Sprintf("%s/api/v1/lifecycle", c.baseURL)
	payload, _ := json.Marshal(rule)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("create lifecycle rule failed: HTTP %d", resp.StatusCode)
	}

	var created LifecycleRule
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	return &created, nil
}

func (c *Client) DeleteLifecycleRule(ctx context.Context, ruleID string) error {
	endpoint := fmt.Sprintf("%s/api/v1/lifecycle/%s", c.baseURL, url.PathEscape(ruleID))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete lifecycle rule failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------------
// RESUMABLE TUS 1.0.0 UPLOAD
// -----------------------------------------------------------------------------

func (c *Client) UploadTUS(ctx context.Context, bucket, objectPath string, reader io.ReaderAt, size int64, opts ...TUSUploadOptions) (*StorageObject, error) {
	chunkSize := int64(4 * 1024 * 1024) // 4MB default chunk size
	var userMeta map[string]string
	if len(opts) > 0 {
		if opts[0].ChunkSize > 0 {
			chunkSize = opts[0].ChunkSize
		}
		userMeta = opts[0].Metadata
	}

	// 1. Create TUS upload session (POST /api/v1/tus/upload)
	metaParts := []string{
		fmt.Sprintf("bucket %s", base64Encode(bucket)),
		fmt.Sprintf("path %s", base64Encode(objectPath)),
		fmt.Sprintf("filename %s", base64Encode(filepath.Base(objectPath))),
	}
	for k, v := range userMeta {
		metaParts = append(metaParts, fmt.Sprintf("%s %s", k, base64Encode(v)))
	}

	creationURL := fmt.Sprintf("%s/api/v1/tus/upload", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", creationURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	req.Header.Set("Upload-Metadata", strings.Join(metaParts, ","))
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TUS creation failed: HTTP %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("TUS server did not return Location header")
	}

	uploadURL := location
	if !strings.HasPrefix(uploadURL, "http://") && !strings.HasPrefix(uploadURL, "https://") {
		uploadURL = fmt.Sprintf("%s/%s", c.baseURL, strings.TrimPrefix(location, "/"))
	}

	// 2. Stream chunks with PATCH
	var offset int64 = 0
	buf := make([]byte, chunkSize)

	for offset < size {
		remaining := size - offset
		currChunkSize := chunkSize
		if remaining < currChunkSize {
			currChunkSize = remaining
		}

		n, err := reader.ReadAt(buf[:currChunkSize], offset)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("reading chunk at offset %d: %w", offset, err)
		}

		chunkReader := bytes.NewReader(buf[:n])
		patchReq, err := http.NewRequestWithContext(ctx, "PATCH", uploadURL, chunkReader)
		if err != nil {
			return nil, err
		}
		patchReq.Header.Set("Tus-Resumable", "1.0.0")
		patchReq.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
		patchReq.Header.Set("Content-Type", "application/offset+octet-stream")
		if c.apiKey != "" {
			patchReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		patchReq.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

		patchResp, err := c.httpClient.Do(patchReq)
		if err != nil {
			return nil, fmt.Errorf("TUS chunk upload failed: %w", err)
		}
		patchResp.Body.Close()

		if patchResp.StatusCode != http.StatusNoContent && patchResp.StatusCode != http.StatusOK && patchResp.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("TUS chunk upload failed: HTTP %d", patchResp.StatusCode)
		}

		newOffsetStr := patchResp.Header.Get("Upload-Offset")
		if newOffsetStr != "" {
			parsedOffset, _ := strconv.ParseInt(newOffsetStr, 10, 64)
			if parsedOffset > offset {
				offset = parsedOffset
			} else {
				offset += int64(n)
			}
		} else {
			offset += int64(n)
		}
	}

	// Fetch uploaded object metadata
	return c.GetFileMetadata(ctx, bucket, objectPath)
}

func base64Encode(s string) string {
	return url.QueryEscape(s)
}

// -----------------------------------------------------------------------------
// ADMIN & MAINTENANCE
// -----------------------------------------------------------------------------

func (c *Client) ExportAuditLogs(ctx context.Context, format string, writer io.Writer) error {
	if format == "" {
		format = "csv"
	}
	endpoint := fmt.Sprintf("%s/api/v1/admin/audit-logs/export?format=%s", c.baseURL, url.QueryEscape(format))
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("export audit logs failed: HTTP %d", resp.StatusCode)
	}

	_, err = io.Copy(writer, resp.Body)
	return err
}

func (c *Client) TriggerGC(ctx context.Context) (*GCReport, error) {
	endpoint := fmt.Sprintf("%s/api/v1/admin/gc", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("trigger GC failed: HTTP %d", resp.StatusCode)
	}

	var report GCReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, err
	}
	return &report, nil
}

func (c *Client) GetDedupReport(ctx context.Context) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/api/v1/admin/dedup-report", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Prolead-Client", "go-sdk-1.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get dedup report failed: HTTP %d", resp.StatusCode)
	}

	var report map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, err
	}
	return report, nil
}

