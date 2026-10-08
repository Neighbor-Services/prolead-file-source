package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gostore/internal/api"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func main() {
	cfg := config.LoadConfig()

	// 1. Initialize Database with GORM (PostgreSQL or SQLite with WAL mode)
	db, err := sqlite.NewDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database (%s): %v", cfg.DBType, err)
	}

	// 2. Initialize Local Disk Storage Driver (with AES-256 At-Rest Disk Encryption, CAS Deduplication & LRU Hot Cache)
	diskStorage, err := local.NewDiskStorage(cfg.StoragePath, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to initialize local storage engine: %v", err)
	}

	// 3. Initialize Repositories
	bucketRepo := sqlite.NewBucketRepository(db)
	fileRepo := sqlite.NewFileRepository(db)
	keyRepo := sqlite.NewAPIKeyRepository(db)
	webhookRepo := sqlite.NewWebhookRepository(db)
	auditRepo := sqlite.NewAuditRepo(db)
	projectRepo := sqlite.NewProjectRepository(db)
	userRepo := sqlite.NewUserRepository(db)
	shareRepo := sqlite.NewShareRepository(db)
	lifecycleRepo := sqlite.NewLifecycleRepository(db)

	// 4. Initialize Core & Advanced Services
	auditService := service.NewAuditService(auditRepo)
	defer auditService.Close()

	authService := service.NewAuthService(userRepo, cfg.MasterAPIKey)
	projectService := service.NewProjectService(projectRepo, bucketRepo)

	signer := service.NewURLSigner(cfg.MasterAPIKey, cfg.BaseURL)
	imageProcessor := service.NewImageProcessor(cfg.StoragePath)
	eventHub := service.NewEventHub()

	workerPipeline := service.NewWorkerPipeline(diskStorage, fileRepo, 4)
	defer workerPipeline.Stop()
	workerPipeline.SetImageProcessor(imageProcessor)

	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imageProcessor, eventHub)
	storageService.SetAuditService(auditService)
	storageService.SetWorkerPipeline(workerPipeline)
	chunkService := service.NewChunkService(cfg.StoragePath, storageService)
	bucketService := service.NewBucketService(bucketRepo, fileRepo, projectRepo, diskStorage)
	keyService := service.NewKeyService(keyRepo, cfg.MasterAPIKey)
	zipStreamer := service.NewZipStreamer(diskStorage, fileRepo)
	backupService := service.NewBackupService(cfg.DatabasePath, cfg.StoragePath, db)
	shareService := service.NewShareService(shareRepo, fileRepo, diskStorage, cfg.BaseURL)

	lifecycleService := service.NewLifecycleService(db, lifecycleRepo, fileRepo, cfg.StoragePath)
	lifecycleService.Start(1 * time.Hour)
	defer lifecycleService.Stop()

	// 5. Initialize Webhook Dispatcher
	webhookService := service.NewWebhookService(webhookRepo)
	webhookService.StartListening(context.Background(), eventHub)

	// 6. Initialize Garbage Collector, Compression Worker & Lifecycle Worker
	gcService := service.NewGarbageCollector(cfg.StoragePath, db)
	gcService.StartBackgroundWorker(context.Background(), 1*time.Hour)

	compressionService := service.NewCompressionService(diskStorage, fileRepo, db)

	// Wire full service ecosystem into Worker Engine
	workerPipeline.SetServices(db, webhookRepo, webhookService, lifecycleService, gcService, backupService, compressionService)

	// 7. Build HTTP Router
	router := api.NewRouter(
		cfg,
		storageService,
		bucketService,
		keyService,
		chunkService,
		gcService,
		zipStreamer,
		webhookService,
		auditService,
		backupService,
		authService,
		projectService,
		shareService,
		lifecycleService,
		lifecycleRepo,
		workerPipeline,
	)

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	displayBaseURL := cfg.BaseURL
	if displayBaseURL == "" {
		displayBaseURL = fmt.Sprintf("http://localhost:%s (dynamic origin enabled)", cfg.Port)
	}

	// Server banner
	fmt.Println("==================================================================")
	fmt.Println(" Prolead File (Enterprise On-Premise Storage Appliance) is Running!")
	fmt.Println("==================================================================")
	fmt.Printf(" Web Dashboard:    %s\n", displayBaseURL)
	fmt.Printf(" Storage Engine:   Local Disk (%s) [CAS Deduplication + LRU Cache]\n", cfg.StoragePath)
	fmt.Printf(" Metadata DB:      %s (driver: %s)\n", cfg.DBType, db.DriverName)
	fmt.Printf(" S3 API Gateway:   %s/s3\n", displayBaseURL)
	fmt.Printf(" Master API Key:   %s\n", cfg.MasterAPIKey)
	fmt.Printf(" Maximum Upload:   %d MB\n", cfg.MaxUploadMB)
	fmt.Println("==================================================================")

	// Graceful shutdown handling
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listen failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Prolead File server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced shutdown: %v", err)
	}

	log.Println("Prolead File server exited successfully.")
}
