package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gostore/internal/config"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func TestGarbageCollector_PreservesVersionsAndSoftDeletes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-gc-test-*")
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
	gcService := service.NewGarbageCollector(storagePath, db)
	ctx := context.Background()

	// 1. Upload File Version 1
	v1Content := "Version 1 Content for GC Test"
	isPublic := true
	f1, err := storageService.Upload(ctx, service.UploadInput{
		Bucket:      "default",
		Path:        "docs/file.txt",
		ContentType: "text/plain",
		IsPublic:    &isPublic,
		Reader:      strings.NewReader(v1Content),
	})
	if err != nil {
		t.Fatalf("Upload v1 failed: %v", err)
	}
	v1Hash := f1.SHA256Hash

	// 2. Create a version record for v1
	err = db.Create(&domain.FileVersion{
		ID:          "ver-1",
		FileID:      f1.ID,
		Version:     1,
		Bucket:      "default",
		Path:        "docs/file.txt",
		Size:        f1.Size,
		ContentType: f1.ContentType,
		SHA256Hash:  v1Hash,
	}).Error
	if err != nil {
		t.Fatalf("Failed to create version 1 record: %v", err)
	}

	// 3. Upload File Version 2 (overwrites active file)
	v2Content := "Version 2 Modified Content for GC Test"
	f2, err := storageService.Upload(ctx, service.UploadInput{
		Bucket:      "default",
		Path:        "docs/file.txt",
		ContentType: "text/plain",
		IsPublic:    &isPublic,
		Reader:      strings.NewReader(v2Content),
	})
	if err != nil {
		t.Fatalf("Upload v2 failed: %v", err)
	}
	v2Hash := f2.SHA256Hash

	if v1Hash == v2Hash {
		t.Fatalf("v1 and v2 hashes must differ")
	}

	// 4. Run GC
	stats, err := gcService.RunGC(ctx)
	if err != nil {
		t.Fatalf("RunGC failed: %v", err)
	}

	if stats.OrphanedBlobs > 0 {
		t.Errorf("Expected 0 orphaned blobs, but GC removed %d blobs!", stats.OrphanedBlobs)
	}

	// Verify both v1 and v2 blobs still exist on disk
	blob1Path := filepath.Join(storagePath, ".blobs", v1Hash[:2], v1Hash[2:4], v1Hash)
	if _, err := os.Stat(blob1Path); os.IsNotExist(err) {
		t.Errorf("Version 1 blob was erroneously deleted by GC!")
	}

	blob2Path := filepath.Join(storagePath, ".blobs", v2Hash[:2], v2Hash[2:4], v2Hash)
	if _, err := os.Stat(blob2Path); os.IsNotExist(err) {
		t.Errorf("Version 2 blob was erroneously deleted by GC!")
	}

	// 5. Test Dedup Report calculates version items
	report, err := gcService.CalculateDedupReport()
	if err != nil {
		t.Fatalf("CalculateDedupReport failed: %v", err)
	}
	if report.TotalVirtualFiles < 2 {
		t.Errorf("Expected at least 2 virtual files (active + version), got %d", report.TotalVirtualFiles)
	}
}
