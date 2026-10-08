package client

import (
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
	"strings"
	"time"

	"gostore/internal/core/domain"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// UploadOptions allows passing extra metadata or public visibility overrides
type UploadOptions struct {
	IsPublic *bool
	Metadata map[string]string
}

// UploadFile uploads an io.Reader stream to GoStore
func (c *Client) UploadFile(ctx context.Context, bucket, objectPath string, reader io.Reader, contentType string, opts ...UploadOptions) (*domain.FileObject, error) {
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var fileObj domain.FileObject
	if err := json.NewDecoder(resp.Body).Decode(&fileObj); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fileObj, nil
}

// UploadLocalFile reads a file from the local path and uploads it
func (c *Client) UploadLocalFile(ctx context.Context, bucket, objectPath, localFilePath string, opts ...UploadOptions) (*domain.FileObject, error) {
	file, err := os.Open(localFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return c.UploadFile(ctx, bucket, objectPath, file, "", opts...)
}

// GetDownloadStream opens a stream directly from the GoStore server
func (c *Client) GetDownloadStream(ctx context.Context, downloadURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, 0, err
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("download error (status %d)", resp.StatusCode)
	}

	return resp.Body, resp.ContentLength, nil
}

// DeleteFile deletes a file from a bucket
func (c *Client) DeleteFile(ctx context.Context, bucket, objectPath string) error {
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o/%s", c.baseURL, bucket, url.PathEscape(objectPath))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListFiles lists files in a bucket
func (c *Client) ListFiles(ctx context.Context, bucket, prefix string) (*domain.ListFilesResult, error) {
	endpoint := fmt.Sprintf("%s/api/v1/b/%s/o?prefix=%s", c.baseURL, bucket, url.QueryEscape(prefix))
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list error (status %d): %s", resp.StatusCode, string(body))
	}

	var res domain.ListFilesResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &res, nil
}
