package service_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func TestWorkerPipeline_DBPersistenceAndRecovery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-worker-test-*")
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

	fileRepo := sqlite.NewFileRepository(db)
	webhookRepo := sqlite.NewWebhookRepository(db)
	webhookService := service.NewWebhookService(webhookRepo)
	gcService := service.NewGarbageCollector(storagePath, db)
	backupService := service.NewBackupService(dbPath, storagePath, db)

	// 1. Simulate an ungraceful shutdown leaving a job in PROCESSING state
	staleJob := domain.WorkerJobRecord{
		ID:        "stale-job-123",
		Type:      string(service.JobTypeIntegrityScrub),
		Status:    string(service.JobStatusProcessing),
		Priority:  1,
		CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
	}
	if err := db.Create(&staleJob).Error; err != nil {
		t.Fatalf("Failed to create stale job record: %v", err)
	}

	// 2. Initialize Worker Pipeline
	wp := service.NewWorkerPipeline(diskStorage, fileRepo, 2)
	defer wp.Stop()

	wp.SetServices(db, webhookRepo, webhookService, nil, gcService, backupService)

	// Verify stale job was recovered and marked FAILED
	var recovered domain.WorkerJobRecord
	if err := db.Where("id = ?", "stale-job-123").First(&recovered).Error; err != nil {
		t.Fatalf("Failed to find stale job record: %v", err)
	}
	if recovered.Status != string(service.JobStatusFailed) {
		t.Errorf("Expected stale job to be marked FAILED on startup, got %s", recovered.Status)
	}

	// 3. Enqueue and execute a new job
	jobID := wp.EnqueueJob(service.WorkerJob{
		Type:     service.JobTypeCASDedup,
		Priority: 0,
	})
	if jobID == "" {
		t.Fatalf("Expected job ID to be generated")
	}

	// Give worker time to execute
	time.Sleep(200 * time.Millisecond)

	stats := wp.GetStats()
	if stats.MaxWorkers == 0 {
		t.Errorf("Expected MaxWorkers to be > 0")
	}

	// Verify job was persisted in database
	var jobRecord domain.WorkerJobRecord
	if err := db.Where("id = ?", jobID).First(&jobRecord).Error; err != nil {
		t.Fatalf("Expected job %s to be persisted in DB: %v", jobID, err)
	}
}
