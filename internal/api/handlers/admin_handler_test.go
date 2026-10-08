package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gostore/internal/api/handlers"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func TestAdminHandler_ExportAuditLogs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-admin-test-*")
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

	auditRepo := sqlite.NewAuditRepo(db)
	auditService := service.NewAuditService(auditRepo)
	defer auditService.Close()

	gcService := service.NewGarbageCollector(storagePath, db)
	backupService := service.NewBackupService(dbPath, storagePath, db)
	fileRepo := sqlite.NewFileRepository(db)
	bucketRepo := sqlite.NewBucketRepository(db)
	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	signer := service.NewURLSigner("key", cfg.BaseURL)
	imgProc := service.NewImageProcessor(storagePath)
	eventHub := service.NewEventHub()

	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imgProc, eventHub)
	adminHandler := handlers.NewAdminHandler(auditService, gcService, backupService, storageService)

	// 1. Record sample audit entries
	auditService.Record("UPLOAD", "default", "photos/cat.jpg", "admin", "127.0.0.1", "curl/8.0", 200, 15)
	auditService.Record("DOWNLOAD", "default", "photos/cat.jpg", "public", "127.0.0.1", "Mozilla/5.0", 200, 5)
	time.Sleep(50 * time.Millisecond) // Allow async audit buffer flush

	// 2. Test CSV Export
	r := chi.NewRouter()
	r.Get("/api/v1/admin/audit-logs/export", adminHandler.ExportAuditLogs)

	reqCSV := httptest.NewRequest("GET", "/api/v1/admin/audit-logs/export?format=csv", nil)
	recCSV := httptest.NewRecorder()
	r.ServeHTTP(recCSV, reqCSV)

	if recCSV.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for CSV export, got %d", recCSV.Code)
	}
	if !strings.Contains(recCSV.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("Expected text/csv content type, got %s", recCSV.Header().Get("Content-Type"))
	}
	csvBody := recCSV.Body.String()
	if !strings.Contains(csvBody, "ID,Action,Bucket,Path") {
		t.Errorf("Expected CSV header, got: %s", csvBody)
	}
	if !strings.Contains(csvBody, "UPLOAD") || !strings.Contains(csvBody, "photos/cat.jpg") {
		t.Errorf("Expected UPLOAD audit entry in CSV output, got: %s", csvBody)
	}

	// 3. Test JSON Export
	reqJSON := httptest.NewRequest("GET", "/api/v1/admin/audit-logs/export?format=json", nil)
	recJSON := httptest.NewRecorder()
	r.ServeHTTP(recJSON, reqJSON)

	if recJSON.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for JSON export, got %d", recJSON.Code)
	}
	if !strings.Contains(recJSON.Header().Get("Content-Type"), "application/json") {
		t.Errorf("Expected application/json content type, got %s", recJSON.Header().Get("Content-Type"))
	}
}

func TestShareHandler_HTMLPreviewAndOpenGraph(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-share-html-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to init SQLite: %v", err)
	}

	diskStorage, err := local.NewDiskStorage(filepath.Join(tempDir, "storage"))
	if err != nil {
		t.Fatalf("Failed to init storage: %v", err)
	}

	fileRepo := sqlite.NewFileRepository(db)
	shareRepo := sqlite.NewShareRepository(db)
	shareService := service.NewShareService(shareRepo, fileRepo, diskStorage, "http://localhost:8080")
	shareHandler := handlers.NewShareHandler(shareService)

	// Upload test file
	bucketRepo := sqlite.NewBucketRepository(db)
	cfg := &config.Config{BaseURL: "http://localhost:8080"}
	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, nil, nil, nil)

	isPublic := true
	file, err := storageService.Upload(context.Background(), service.UploadInput{
		Bucket:      "default",
		Path:        "share-test.png",
		ContentType: "image/png",
		IsPublic:    &isPublic,
		Reader:      strings.NewReader("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4"),
	})
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Create share link
	share, err := shareService.CreateShareLink(context.Background(), service.CreateShareRequest{
		Bucket: file.Bucket,
		Path:   file.Path,
	})
	if err != nil {
		t.Fatalf("CreateShareLink failed: %v", err)
	}

	r := chi.NewRouter()
	r.Get("/s/{token}", shareHandler.GetShareInfo)

	// Test Browser / Crawler with Accept: text/html
	req := httptest.NewRequest("GET", "/s/"+share.Token, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Expected text/html content type, got %s", rec.Header().Get("Content-Type"))
	}

	htmlBody := rec.Body.String()
	if !strings.Contains(htmlBody, "og:title") || !strings.Contains(htmlBody, "share-test.png") {
		t.Errorf("Expected OpenGraph metadata in HTML output, got: %s", htmlBody)
	}
	if !strings.Contains(htmlBody, "Download File") {
		t.Errorf("Expected Download File button in HTML output")
	}
}
