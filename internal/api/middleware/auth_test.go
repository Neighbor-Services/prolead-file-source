package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gostore/internal/api/middleware"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
)

func TestCheckBucketPermission(t *testing.T) {
	ctx := context.Background()

	// 1. Unauthenticated read is allowed
	if err := middleware.CheckBucketPermission(ctx, "default", false); err != nil {
		t.Errorf("Expected unauthenticated read to be allowed, got: %v", err)
	}

	// 2. Unauthenticated write is rejected
	if err := middleware.CheckBucketPermission(ctx, "default", true); err == nil {
		t.Errorf("Expected unauthenticated write to be rejected, but it was allowed!")
	}

	// 3. Read-only API Key write is rejected
	readOnlyKey := &domain.APIKey{
		ID:        "key-ro",
		Role:      "read-only",
		CreatedAt: time.Now(),
	}
	ctxRO := context.WithValue(ctx, middleware.APIKeyContextKey, readOnlyKey)
	if err := middleware.CheckBucketPermission(ctxRO, "default", true); err == nil {
		t.Errorf("Expected read-only key write to be rejected, but it was allowed!")
	}

	// 4. Admin API Key write is allowed
	adminKey := &domain.APIKey{
		ID:        "key-admin",
		Role:      "admin",
		CreatedAt: time.Now(),
	}
	ctxAdmin := context.WithValue(ctx, middleware.APIKeyContextKey, adminKey)
	if err := middleware.CheckBucketPermission(ctxAdmin, "default", true); err != nil {
		t.Errorf("Expected admin key write to be allowed, got: %v", err)
	}

	// 5. Whitelisted bucket check
	scopedKey := &domain.APIKey{
		ID:             "key-scoped",
		Role:           "read-write",
		AllowedBuckets: domain.StringList{"images"},
		CreatedAt:      time.Now(),
	}
	ctxScoped := context.WithValue(ctx, middleware.APIKeyContextKey, scopedKey)
	if err := middleware.CheckBucketPermission(ctxScoped, "images", true); err != nil {
		t.Errorf("Expected allowed bucket to pass, got: %v", err)
	}
	if err := middleware.CheckBucketPermission(ctxScoped, "videos", true); err == nil {
		t.Errorf("Expected non-whitelisted bucket to fail, but it passed!")
	}
}

func TestRequireAdmin(t *testing.T) {
	adminHandler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// 1. Unauthenticated request should be forbidden
	req := httptest.NewRequest("GET", "/api/v1/admin/gc", nil)
	rec := httptest.NewRecorder()
	adminHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", rec.Code)
	}

	// 2. Read-only user request should be forbidden
	reqRO := httptest.NewRequest("GET", "/api/v1/admin/gc", nil)
	ctxRO := context.WithValue(reqRO.Context(), middleware.APIKeyContextKey, &domain.APIKey{
		Role: "read-write",
	})
	recRO := httptest.NewRecorder()
	adminHandler.ServeHTTP(recRO, reqRO.WithContext(ctxRO))
	if recRO.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 Forbidden, got %d", recRO.Code)
	}

	// 3. Admin user request should be allowed
	reqAdmin := httptest.NewRequest("GET", "/api/v1/admin/gc", nil)
	ctxAdmin := context.WithValue(reqAdmin.Context(), middleware.APIKeyContextKey, &domain.APIKey{
		Role: "admin",
	})
	recAdmin := httptest.NewRecorder()
	adminHandler.ServeHTTP(recAdmin, reqAdmin.WithContext(ctxAdmin))
	if recAdmin.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %d", recAdmin.Code)
	}
}

func TestRequireAuth(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-auth-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to init SQLite: %v", err)
	}

	userRepo := sqlite.NewUserRepository(db)
	keyRepo := sqlite.NewAPIKeyRepository(db)
	authService := service.NewAuthService(userRepo, "test-master-key")
	keyService := service.NewKeyService(keyRepo, "test-master-key")

	protectedHandler := middleware.RequireAuth(keyService, authService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authenticated"))
	}))

	// 1. Missing Token
	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rec := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized for missing token, got %d", rec.Code)
	}

	// 2. Master Key Token
	reqMaster := httptest.NewRequest("GET", "/api/v1/projects", nil)
	reqMaster.Header.Set("Authorization", "Bearer test-master-key")
	recMaster := httptest.NewRecorder()
	protectedHandler.ServeHTTP(recMaster, reqMaster)
	if recMaster.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK for master key, got %d", recMaster.Code)
	}
}
