package handlers_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"gostore/internal/api/handlers"
	"gostore/internal/config"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
	"gostore/internal/storage/local"
)

func setupTUSTest(t *testing.T) (*chi.Mux, func()) {
	tmpDir, err := os.MkdirTemp("", "prolead-tus-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		StoragePath:  tmpDir,
		DatabasePath: tmpDir + "/test.db",
		DBType:       "sqlite",
		MasterAPIKey: "test-key",
		MaxUploadMB:  100,
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
	signer := service.NewURLSigner("test-key", "http://localhost:8080")
	imgProc := service.NewImageProcessor(tmpDir)
	eventHub := service.NewEventHub()

	storageService := service.NewStorageService(cfg, diskStorage, fileRepo, bucketRepo, signer, imgProc, eventHub)
	chunkService := service.NewChunkService(tmpDir, storageService)
	tusHandler := handlers.NewTUSHandler(chunkService, storageService)

	r := chi.NewRouter()
	r.Route("/api/v1/tus/files", func(r chi.Router) {
		r.Options("/", tusHandler.Options)
		r.Post("/", tusHandler.CreateUpload)
		r.Options("/{uploadId}", tusHandler.Options)
		r.Head("/{uploadId}", tusHandler.HeadUpload)
		r.Patch("/{uploadId}", tusHandler.PatchUpload)
		r.Delete("/{uploadId}", tusHandler.TerminateUpload)
	})

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return r, cleanup
}

func TestTUSOptions(t *testing.T) {
	router, cleanup := setupTUSTest(t)
	defer cleanup()

	req := httptest.NewRequest("OPTIONS", "/api/v1/tus/files", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 No Content, got %d", rec.Code)
	}
	if rec.Header().Get("Tus-Resumable") != "1.0.0" {
		t.Errorf("expected Tus-Resumable 1.0.0, got %s", rec.Header().Get("Tus-Resumable"))
	}
}

func TestTUSUploadWorkflow(t *testing.T) {
	router, cleanup := setupTUSTest(t)
	defer cleanup()

	payload := []byte("Prolead File TUS Resumable Upload Test Content")
	totalSize := int64(len(payload))

	// 1. Create Upload Session (POST)
	filenameEncoded := base64.StdEncoding.EncodeToString([]byte("resumable_doc.txt"))
	req := httptest.NewRequest("POST", "/api/v1/tus/files", nil)
	req.Header.Set("Tus-Resumable", "1.0.0")
	req.Header.Set("Upload-Length", fmt.Sprintf("%d", totalSize))
	req.Header.Set("Upload-Metadata", fmt.Sprintf("filename %s,bucket %s,isPublic dHJ1ZQ==", filenameEncoded, base64.StdEncoding.EncodeToString([]byte("default"))))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatalf("expected Location header in response")
	}

	// 2. HEAD Request to check offset
	headReq := httptest.NewRequest("HEAD", location, nil)
	headReq.Header.Set("Tus-Resumable", "1.0.0")
	headRec := httptest.NewRecorder()
	router.ServeHTTP(headRec, headReq)

	if headRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK on HEAD, got %d", headRec.Code)
	}
	if headRec.Header().Get("Upload-Offset") != "0" {
		t.Errorf("expected offset 0, got %s", headRec.Header().Get("Upload-Offset"))
	}

	// 3. PATCH Request to upload content
	patchReq := httptest.NewRequest("PATCH", location, bytes.NewReader(payload))
	patchReq.Header.Set("Tus-Resumable", "1.0.0")
	patchReq.Header.Set("Content-Type", "application/offset+octet-stream")
	patchReq.Header.Set("Upload-Offset", "0")

	patchRec := httptest.NewRecorder()
	router.ServeHTTP(patchRec, patchReq)

	if patchRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 No Content on PATCH, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	if patchRec.Header().Get("Upload-Offset") != fmt.Sprintf("%d", totalSize) {
		t.Errorf("expected offset %d, got %s", totalSize, patchRec.Header().Get("Upload-Offset"))
	}
}
