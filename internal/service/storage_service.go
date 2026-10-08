package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gostore/internal/config"
	"gostore/internal/core/domain"
	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"
	"gostore/internal/storage/local"
)

var (
	ErrBucketNotFound = errors.New("bucket not found")
	ErrFileNotFound   = errors.New("file not found")
	ErrFileTooLarge   = errors.New("file size exceeds bucket limit")
	ErrMimeNotAllowed = errors.New("file MIME type is not allowed in this bucket")
	ErrInvalidToken   = errors.New("invalid or expired download token")
	ErrUnauthorized   = errors.New("unauthorized access")
)

type StorageService struct {
	cfg            *config.Config
	storage        ports.StorageDriver
	fileRepo       *sqlite.FileRepository
	bucketRepo     *sqlite.BucketRepository
	signer         *URLSigner
	imageProcessor *ImageProcessor
	eventHub       *EventHub
	auditService   *AuditService
	workerPipeline *WorkerPipeline
}

func NewStorageService(
	cfg *config.Config,
	storage ports.StorageDriver,
	fileRepo *sqlite.FileRepository,
	bucketRepo *sqlite.BucketRepository,
	signer *URLSigner,
	imageProcessor *ImageProcessor,
	eventHub *EventHub,
) *StorageService {
	return &StorageService{
		cfg:            cfg,
		storage:        storage,
		fileRepo:       fileRepo,
		bucketRepo:     bucketRepo,
		signer:         signer,
		imageProcessor: imageProcessor,
		eventHub:       eventHub,
	}
}

func (s *StorageService) SetWorkerPipeline(wp *WorkerPipeline) {
	s.workerPipeline = wp
}

func (s *StorageService) SetAuditService(a *AuditService) {
	s.auditService = a
}

type UploadParams = UploadInput


type UploadInput struct {
	Bucket      string
	Path        string
	ContentType string
	IsPublic    *bool
	Metadata    map[string]string
	ExpiresIn   *time.Duration
	Reader      io.Reader
}

func (s *StorageService) Upload(ctx context.Context, input UploadInput) (*domain.FileObject, error) {
	bucketName := strings.TrimSpace(input.Bucket)
	if bucketName == "" {
		bucketName = "default"
	}

	bucket, err := s.bucketRepo.GetByName(ctx, bucketName)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		if bucketName == "default" {
			bucket = &domain.Bucket{
				ID:          uuid.New().String(),
				Name:        "default",
				Description: "Default auto-created bucket",
				IsPublic:    true,
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}
			if err := s.bucketRepo.Create(ctx, bucket); err != nil {
				return nil, fmt.Errorf("failed to create default bucket: %w", err)
			}
		} else {
			return nil, ErrBucketNotFound
		}
	}

	objPath := strings.Trim(filepath.ToSlash(input.Path), "/")
	if objPath == "" {
		return nil, errors.New("object path cannot be empty")
	}

	fileName := filepath.Base(objPath)

	sniffedMime, reader, err := local.SniffContentType(input.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect file stream: %w", err)
	}

	contentType := input.ContentType
	if contentType == "" || contentType == "application/octet-stream" {
		ext := filepath.Ext(fileName)
		extMime := mime.TypeByExtension(ext)
		if extMime != "" {
			contentType = extMime
		} else {
			contentType = sniffedMime
		}
	}

	if isBlockedExecutable(fileName, contentType) {
		return nil, errors.New("executable scripts and binary formats are blocked for security")
	}

	if len(bucket.AllowedMimes) > 0 {
		allowed := false
		for _, m := range bucket.AllowedMimes {
			if strings.EqualFold(m, contentType) || strings.HasPrefix(contentType, strings.TrimSuffix(m, "*")) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrMimeNotAllowed
		}
	}

	written, md5Hash, sha256Hash, err := s.storage.Save(ctx, bucketName, objPath, reader)
	if err != nil {
		return nil, fmt.Errorf("storage driver write error: %w", err)
	}

	if bucket.MaxFileSize > 0 && written > bucket.MaxFileSize {
		_ = s.storage.Delete(ctx, bucketName, objPath)
		return nil, ErrFileTooLarge
	}

	if bucket.MaxTotalBytes > 0 || bucket.MaxFileCount > 0 {
		count, totalBytes, err := s.fileRepo.GetBucketUsage(ctx, bucketName)
		if err == nil {
			if bucket.MaxFileCount > 0 && count+1 > bucket.MaxFileCount {
				_ = s.storage.Delete(ctx, bucketName, objPath)
				return nil, errors.New("bucket file count quota exceeded")
			}
			if bucket.MaxTotalBytes > 0 && totalBytes+written > bucket.MaxTotalBytes {
				_ = s.storage.Delete(ctx, bucketName, objPath)
				return nil, errors.New("bucket storage capacity quota exceeded")
			}
		}
	}

	downloadToken := uuid.New().String()
	isPublic := bucket.IsPublic
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	var expiresAt *time.Time
	if input.ExpiresIn != nil && *input.ExpiresIn > 0 {
		exp := time.Now().UTC().Add(*input.ExpiresIn)
		expiresAt = &exp
	}

	fileObj := &domain.FileObject{
		ID:            uuid.New().String(),
		Bucket:        bucketName,
		Path:          objPath,
		Name:          fileName,
		Size:          written,
		ContentType:   contentType,
		MD5Hash:       md5Hash,
		SHA256Hash:    sha256Hash,
		DownloadToken: downloadToken,
		IsPublic:      isPublic,
		Metadata:      input.Metadata,
		ExpiresAt:     expiresAt,
		DeletedAt:     nil,
	}

	if err := s.fileRepo.Save(ctx, fileObj); err != nil {
		_ = s.storage.Delete(ctx, bucketName, objPath)
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	fileObj.DownloadURL = s.BuildDownloadURL(bucketName, objPath, downloadToken)

	if s.auditService != nil {
		s.auditService.Record("UPLOAD", bucketName, objPath, "system", "", "", 200, 0)
	}

	if s.eventHub != nil {
		s.eventHub.Publish(EventFileUploaded, bucketName, objPath, fileObj)
	}

	if s.workerPipeline != nil {
		s.workerPipeline.Enqueue(bucketName, objPath)
	}

	return fileObj, nil
}

func (s *StorageService) GetFileMetadata(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	fileObj, err := s.fileRepo.GetByPath(ctx, bucket, path)
	if err != nil {
		return nil, err
	}
	if fileObj == nil {
		return nil, ErrFileNotFound
	}

	fileObj.DownloadURL = s.BuildDownloadURL(fileObj.Bucket, fileObj.Path, fileObj.DownloadToken)
	return fileObj, nil
}

type FileDownloadStream struct {
	FileObject *domain.FileObject
	Reader     ports.ReadSeekCloser
	Size       int64
}

func (s *StorageService) OpenDownload(ctx context.Context, bucket, path, token, expires, sig string, isAuth bool) (*FileDownloadStream, error) {
	fileObj, err := s.fileRepo.GetByPath(ctx, bucket, path)
	if err != nil {
		return nil, err
	}
	if fileObj == nil {
		return nil, ErrFileNotFound
	}

	// Check TTL expiration
	if fileObj.ExpiresAt != nil && time.Now().UTC().After(*fileObj.ExpiresAt) {
		return nil, errors.New("file has expired and is no longer accessible")
	}

	if !fileObj.IsPublic && !isAuth {
		validToken := token != "" && token == fileObj.DownloadToken
		validSignature := s.signer != nil && s.signer.Verify(bucket, path, expires, sig)

		if !validToken && !validSignature {
			return nil, ErrUnauthorized
		}
	}

	reader, size, err := s.storage.Open(ctx, bucket, path)
	if err != nil {
		return nil, err
	}

	fileObj.DownloadURL = s.BuildDownloadURL(fileObj.Bucket, fileObj.Path, fileObj.DownloadToken)

	if s.auditService != nil {
		s.auditService.Record("DOWNLOAD", bucket, path, "user", "", "", 200, 0)
	}

	return &FileDownloadStream{
		FileObject: fileObj,
		Reader:     reader,
		Size:       size,
	}, nil
}

func (s *StorageService) GenerateSignedURL(bucket, path string, duration time.Duration) (string, int64, string, error) {
	fileObj, err := s.fileRepo.GetByPath(context.Background(), bucket, path)
	if err != nil {
		return "", 0, "", err
	}
	if fileObj == nil {
		return "", 0, "", ErrFileNotFound
	}

	url, exp, sig := s.signer.GenerateSignedURL(bucket, path, duration)
	return url, exp, sig, nil
}

func (s *StorageService) TransformImage(src io.Reader, fileKey string, opts ImageTransformOptions) ([]byte, string, error) {
	if s.imageProcessor == nil {
		data, err := io.ReadAll(src)
		return data, "", err
	}
	return s.imageProcessor.Process(src, fileKey, opts)
}

func (s *StorageService) RotateToken(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	fileObj, err := s.fileRepo.GetByPath(ctx, bucket, path)
	if err != nil {
		return nil, err
	}
	if fileObj == nil {
		return nil, ErrFileNotFound
	}

	newToken := uuid.New().String()
	if err := s.fileRepo.UpdateToken(ctx, bucket, path, newToken); err != nil {
		return nil, err
	}

	fileObj.DownloadToken = newToken
	fileObj.DownloadURL = s.BuildDownloadURL(bucket, path, newToken)

	if s.auditService != nil {
		s.auditService.Record("ROTATE_TOKEN", bucket, path, "user", "", "", 200, 0)
	}

	if s.eventHub != nil {
		s.eventHub.Publish(EventTokenRotated, bucket, path, fileObj)
	}

	return fileObj, nil
}

func (s *StorageService) Delete(ctx context.Context, bucket, path string, softDelete bool) error {
	action := "DELETE"
	if softDelete {
		action = "TRASH"
		err := s.fileRepo.SoftDelete(ctx, bucket, path)
		if err == nil {
			if s.auditService != nil {
				s.auditService.Record(action, bucket, path, "user", "", "", 200, 0)
			}
			if s.eventHub != nil {
				s.eventHub.Publish(EventFileDeleted, bucket, path, map[string]bool{"soft": true})
			}
		}
		return err
	}

	if err := s.fileRepo.HardDelete(ctx, bucket, path); err != nil {
		if !errors.Is(err, ErrFileNotFound) {
			return err
		}
	}

	err := s.storage.Delete(ctx, bucket, path)
	if err == nil {
		if s.auditService != nil {
			s.auditService.Record(action, bucket, path, "user", "", "", 200, 0)
		}
		if s.eventHub != nil {
			s.eventHub.Publish(EventFileDeleted, bucket, path, map[string]bool{"soft": false})
		}
	}
	return err
}

func (s *StorageService) Restore(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	fileObj, err := s.fileRepo.Restore(ctx, bucket, path)
	if err != nil {
		return nil, err
	}
	fileObj.DownloadURL = s.BuildDownloadURL(fileObj.Bucket, fileObj.Path, fileObj.DownloadToken)

	if s.auditService != nil {
		s.auditService.Record("RESTORE", bucket, path, "user", "", "", 200, 0)
	}

	if s.eventHub != nil {
		s.eventHub.Publish(EventType("file:restored"), bucket, path, fileObj)
	}
	return fileObj, nil
}

func (s *StorageService) ListVersions(ctx context.Context, bucket, path string) ([]domain.FileVersion, error) {
	return s.fileRepo.ListVersions(ctx, bucket, path)
}

func (s *StorageService) List(ctx context.Context, filter domain.ListFilesFilter) (*domain.ListFilesResult, error) {
	res, err := s.fileRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	for i := range res.Items {
		res.Items[i].DownloadURL = s.BuildDownloadURL(res.Items[i].Bucket, res.Items[i].Path, res.Items[i].DownloadToken)
	}

	return res, nil
}

func (s *StorageService) ScrubIntegrity(ctx context.Context, bucket string) (map[string]interface{}, error) {
	filter := domain.ListFilesFilter{Bucket: bucket, Limit: 500}
	res, err := s.fileRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalChecked := 0
	corruptFiles := 0
	for _, f := range res.Items {
		reader, _, err := s.storage.Open(ctx, f.Bucket, f.Path)
		if err != nil {
			corruptFiles++
			continue
		}

		hasher := sha256.New()
		_, _ = io.Copy(hasher, reader)
		reader.Close()

		computed := hex.EncodeToString(hasher.Sum(nil))
		if computed != f.SHA256Hash {
			corruptFiles++
		}
		totalChecked++
	}

	return map[string]interface{}{
		"totalChecked": totalChecked,
		"corruptFiles": corruptFiles,
		"status":       "healthy",
	}, nil
}

func (s *StorageService) BuildDownloadURL(bucket, path, token string) string {
	escapedPath := url.PathEscape(path)
	if token != "" {
		return fmt.Sprintf("%s/v0/b/%s/o/%s?alt=media&token=%s", s.cfg.BaseURL, bucket, escapedPath, token)
	}
	return fmt.Sprintf("%s/v0/b/%s/o/%s?alt=media", s.cfg.BaseURL, bucket, escapedPath)
}

func (s *StorageService) GetEventHub() *EventHub {
	return s.eventHub
}

func (s *StorageService) ListFiles(ctx context.Context, filter domain.ListFilesFilter) (*domain.ListFilesResult, error) {
	return s.List(ctx, filter)
}

func (s *StorageService) DownloadFile(ctx context.Context, bucket, path string, isAuth bool) (*domain.FileObject, ports.ReadSeekCloser, error) {
	stream, err := s.OpenDownload(ctx, bucket, path, "", "", "", isAuth)
	if err != nil {
		return nil, nil, err
	}
	return stream.FileObject, stream.Reader, nil
}

func (s *StorageService) UploadFile(ctx context.Context, params UploadParams) (*domain.FileObject, error) {
	return s.Upload(ctx, params)
}

func (s *StorageService) DeleteFile(ctx context.Context, bucket, path string, softDelete bool) error {
	return s.Delete(ctx, bucket, path, softDelete)
}

func (s *StorageService) CopyObject(ctx context.Context, srcBucket, srcPath, dstBucket, dstPath string) (*domain.FileObject, error) {
	srcObj, err := s.fileRepo.GetByPath(ctx, srcBucket, srcPath)
	if err != nil || srcObj == nil {
		return nil, ErrFileNotFound
	}

	reader, _, err := s.storage.Open(ctx, srcBucket, srcPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return s.Upload(ctx, UploadInput{
		Bucket:      dstBucket,
		Path:        dstPath,
		ContentType: srcObj.ContentType,
		IsPublic:    &srcObj.IsPublic,
		Metadata:    srcObj.Metadata,
		Reader:      reader,
	})
}

func (s *StorageService) MoveObject(ctx context.Context, srcBucket, srcPath, dstBucket, dstPath string) (*domain.FileObject, error) {
	copied, err := s.CopyObject(ctx, srcBucket, srcPath, dstBucket, dstPath)
	if err != nil {
		return nil, err
	}
	_ = s.Delete(ctx, srcBucket, srcPath, false)
	return copied, nil
}

func (s *StorageService) RenameObject(ctx context.Context, bucket, oldPath, newName string) (*domain.FileObject, error) {
	cleanName := filepath.Base(newName)
	dir := filepath.Dir(oldPath)
	var newPath string
	if dir == "." || dir == "" || dir == "/" {
		newPath = cleanName
	} else {
		newPath = strings.Trim(dir, "/") + "/" + cleanName
	}
	return s.MoveObject(ctx, bucket, oldPath, bucket, newPath)
}

func (s *StorageService) DeleteFolder(ctx context.Context, bucket, prefix string, permanent bool) (int, error) {
	cleanPrefix := strings.TrimPrefix(prefix, "/")
	if !strings.HasSuffix(cleanPrefix, "/") {
		cleanPrefix += "/"
	}

	filter := domain.ListFilesFilter{
		Bucket:       bucket,
		Prefix:       cleanPrefix,
		Limit:        1000,
		IncludeTrash: !permanent,
	}
	res, err := s.fileRepo.List(ctx, filter)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, f := range res.Items {
		if err := s.Delete(ctx, bucket, f.Path, permanent); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func isBlockedExecutable(filename, mimeType string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	blockedExts := map[string]bool{
		".exe": true, ".sh": true, ".bat": true, ".cmd": true, ".php": true, ".phtml": true, ".cgi": true, ".pl": true,
	}
	if blockedExts[ext] {
		return true
	}
	if strings.Contains(mimeType, "x-msdownload") || strings.Contains(mimeType, "x-sh") || strings.Contains(mimeType, "x-php") {
		return true
	}
	return false
}

