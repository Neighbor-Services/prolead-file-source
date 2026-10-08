package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func TestCompressionService_CompressFileAndReport(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-comp-test-*")
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
		t.Fatalf("Failed to init storage: %v", err)
	}

	bucketRepo := sqlite.NewBucketRepository(db)
	fileRepo := sqlite.NewFileRepository(db)
	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	signer := service.NewURLSigner("test-secret", cfg.BaseURL)
	imgProc := service.NewImageProcessor(storagePath)
	eventHub := service.NewEventHub()

	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imgProc, eventHub)
	compressionService := service.NewCompressionService(diskStorage, fileRepo, db)
	ctx := context.Background()

	// 1. Test IsCompressible
	if !compressionService.IsCompressible("application/json", "data.json") {
		t.Errorf("Expected data.json to be compressible")
	}
	if !compressionService.IsCompressible("text/plain", "logs.txt") {
		t.Errorf("Expected logs.txt to be compressible")
	}
	if compressionService.IsCompressible("image/png", "photo.png") {
		t.Errorf("Expected photo.png to NOT be compressible")
	}

	// 2. Upload a large compressible JSON file (> 1KB with high redundancy)
	jsonPayload := `{"items": [` + strings.Repeat(`{"id": 12345, "name": "GoStore Object Storage Engine", "status": "active"},`, 50) + `{"id": 99999, "name": "End", "status": "done"}]}`
	isPublic := true
	fileObj, err := storageService.Upload(ctx, service.UploadInput{
		Bucket:      "default",
		Path:        "reports/analytics.json",
		ContentType: "application/json",
		IsPublic:    &isPublic,
		Reader:      strings.NewReader(jsonPayload),
	})
	if err != nil {
		t.Fatalf("Failed to upload test JSON file: %v", err)
	}

	origSize := fileObj.Size

	// 3. Run Compression on file
	result, err := compressionService.CompressFile(ctx, "default", "reports/analytics.json")
	if err != nil {
		t.Fatalf("CompressFile failed: %v", err)
	}

	if result.Skipped {
		t.Fatalf("Expected file to be compressed, but it was skipped: %s", result.Reason)
	}

	if result.CompressedSize >= origSize {
		t.Errorf("Expected compressed size (%d) to be smaller than original (%d)", result.CompressedSize, origSize)
	}

	if result.SavingsPercent < 50.0 {
		t.Errorf("Expected >50%% savings on repetitive JSON, got %.2f%%", result.SavingsPercent)
	}

	// Verify metadata was updated in DB
	updatedFile, err := fileRepo.GetByPath(ctx, "default", "reports/analytics.json")
	if err != nil {
		t.Fatalf("GetByPath failed: %v", err)
	}
	if updatedFile.Metadata["compressed"] != "gzip" {
		t.Errorf("Expected metadata['compressed'] to be 'gzip', got %v", updatedFile.Metadata["compressed"])
	}

	// 4. Test Second Run skips already compressed file
	secondResult, err := compressionService.CompressFile(ctx, "default", "reports/analytics.json")
	if err != nil {
		t.Fatalf("Second CompressFile failed: %v", err)
	}
	if !secondResult.Skipped {
		t.Errorf("Expected second compression to be skipped as already compressed")
	}

	// 5. Test Batch Compression & Compression Report
	report, err := compressionService.GetCompressionReport(ctx)
	if err != nil {
		t.Fatalf("GetCompressionReport failed: %v", err)
	}
	if report.TotalFilesCompressed != 1 {
		t.Errorf("Expected 1 compressed file in report, got %d", report.TotalFilesCompressed)
	}
	if report.BytesSaved <= 0 {
		t.Errorf("Expected BytesSaved > 0, got %d", report.BytesSaved)
	}

	// 6. Test Worker Pipeline Integration with JobTypeFileCompression
	wp := service.NewWorkerPipeline(diskStorage, fileRepo, 2)
	defer wp.Stop()
	wp.SetServices(db, nil, nil, nil, nil, nil, compressionService)

	jobID := wp.EnqueueJob(service.WorkerJob{
		Type:     service.JobTypeFileCompression,
		Bucket:   "default",
		Priority: 1,
	})
	if jobID == "" {
		t.Errorf("Expected job ID from EnqueueJob")
	}
}
