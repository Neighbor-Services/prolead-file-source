package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"gostore/internal/api/handlers"
	"gostore/internal/api/middleware"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func setupS3Test(t *testing.T) (*chi.Mux, *service.StorageService, func()) {
	tmpDir, err := os.MkdirTemp("", "gostore-s3-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		StoragePath:  filepath.Join(tmpDir, "storage"),
		DatabasePath: filepath.Join(tmpDir, "test.db"),
		DBType:       "sqlite",
		MasterAPIKey: "test-master-key",
		MaxUploadMB:  100,
		BaseURL:      "http://localhost:8080",
	}

	db, err := sqlite.NewDatabase(cfg)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	diskStorage, err := local.NewDiskStorage(cfg.StoragePath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	bucketRepo := sqlite.NewBucketRepository(db)
	fileRepo := sqlite.NewFileRepository(db)
	projectRepo := sqlite.NewProjectRepository(db)
	keyRepo := sqlite.NewAPIKeyRepository(db)
	userRepo := sqlite.NewUserRepository(db)
	signer := service.NewURLSigner("test-key", cfg.BaseURL)
	imgProc := service.NewImageProcessor(tmpDir)
	eventHub := service.NewEventHub()

	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imgProc, eventHub)
	bucketService := service.NewBucketService(bucketRepo, fileRepo, projectRepo, diskStorage)
	keyService := service.NewKeyService(keyRepo, cfg.MasterAPIKey)
	authService := service.NewAuthService(userRepo, cfg.MasterAPIKey)
	s3Handler := handlers.NewS3Handler(storageService, bucketService)

	r := chi.NewRouter()
	r.Route("/s3/{bucket}", func(r chi.Router) {
		r.Use(middleware.OptionalAuth(keyService, authService))
		r.HandleFunc("/", s3Handler.HandleBucketOrObject)
		r.HandleFunc("/*", s3Handler.HandleBucketOrObject)
	})

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return r, storageService, cleanup
}

func TestS3_PutAndGetObject(t *testing.T) {
	router, _, cleanup := setupS3Test(t)
	defer cleanup()

	content := []byte("S3 API Compatibility Unit Test Payload")

	// 1. Put Object with Master Key Auth (PUT /s3/default/test.txt)
	req := httptest.NewRequest("PUT", "/s3/default/test.txt", bytes.NewReader(content))
	req.Header.Set("Authorization", "Bearer test-master-key")
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for S3 PutObject, got %d. Body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") == "" {
		t.Errorf("Expected ETag header in S3 PutObject response")
	}

	// 2. Get Object (GET /s3/default/test.txt)
	getReq := httptest.NewRequest("GET", "/s3/default/test.txt", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 OK for S3 GetObject, got %d", getRec.Code)
	}
	if getRec.Body.String() != string(content) {
		t.Errorf("Expected body %q, got %q", string(content), getRec.Body.String())
	}

	// 3. Unauthenticated Put Object should fail with 403 AccessDenied
	unauthReq := httptest.NewRequest("PUT", "/s3/default/forbidden.txt", bytes.NewReader(content))
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusForbidden {
		t.Fatalf("Expected status 403 Forbidden for unauthenticated S3 PutObject, got %d", unauthRec.Code)
	}
}
