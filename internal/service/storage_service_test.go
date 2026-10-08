package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/storage/local"
)

func TestStorageService_UploadAndDownload(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-svc-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	storagePath := filepath.Join(tempDir, "storage")

	db, err := sqlite.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init SQLite: %v", err)
	}

	diskStorage, err := local.NewDiskStorage(storagePath)
	if err != nil {
		t.Fatalf("Failed to init disk storage: %v", err)
	}

	bucketRepo := sqlite.NewBucketRepository(db)
	fileRepo := sqlite.NewFileRepository(db)

	cfg := &config.Config{
		BaseURL: "http://localhost:8080",
	}

	signer := NewURLSigner("test-secret-key", cfg.BaseURL)
	imageProcessor := NewImageProcessor(storagePath)
	eventHub := NewEventHub()

	svc := NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imageProcessor, eventHub)
	ctx := context.Background()

	// 1. Upload File
	content := "Hello World Test File Content"
	isPublicFalse := false
	uploadInput := UploadInput{
		Bucket:      "default",
		Path:        "test/hello.txt",
		ContentType: "text/plain",
		IsPublic:    &isPublicFalse,
		Reader:      strings.NewReader(content),
	}

	fileObj, err := svc.Upload(ctx, uploadInput)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if fileObj.Size != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), fileObj.Size)
	}
	if fileObj.DownloadToken == "" {
		t.Errorf("Expected download token to be generated")
	}

	// 2. Open Download Stream with Permanent Token
	stream, err := svc.OpenDownload(ctx, "default", "test/hello.txt", fileObj.DownloadToken, "", "", false)
	if err != nil {
		t.Fatalf("OpenDownload failed: %v", err)
	}
	defer stream.Reader.Close()

	readBytes, err := io.ReadAll(stream.Reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(readBytes) != content {
		t.Errorf("Expected content %q, got %q", content, string(readBytes))
	}

	// 3. Test Soft Delete & Restore
	if err := svc.Delete(ctx, "default", "test/hello.txt", true); err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// Should be not found after soft delete
	_, err = svc.GetFileMetadata(ctx, "default", "test/hello.txt")
	if err == nil {
		t.Errorf("Expected file to be hidden after soft delete")
	}

	// Restore from trash
	restored, err := svc.Restore(ctx, "default", "test/hello.txt")
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restored == nil || restored.Path != "test/hello.txt" {
		t.Errorf("Expected file restored properly")
	}

	// 4. Test HMAC Expiring Signed URL
	signedURL, exp, sig, err := svc.GenerateSignedURL("default", "test/hello.txt", 10*time.Minute)
	if err != nil {
		t.Fatalf("GenerateSignedURL failed: %v", err)
	}
	if signedURL == "" || sig == "" || exp == 0 {
		t.Errorf("Invalid signed URL output")
	}

	// 5. Rotate Token
	rotated, err := svc.RotateToken(ctx, "default", "test/hello.txt")
	if err != nil {
		t.Fatalf("RotateToken failed: %v", err)
	}
	if rotated.DownloadToken == fileObj.DownloadToken {
		t.Errorf("Expected new token after rotation")
	}

	// 6. Verify old token is rejected
	_, err = svc.OpenDownload(ctx, "default", "test/hello.txt", fileObj.DownloadToken, "", "", false)
	if err == nil {
		t.Errorf("Expected unauthorized error with old token, got nil")
	}
}
