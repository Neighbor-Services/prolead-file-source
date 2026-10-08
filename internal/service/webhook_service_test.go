package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
)

func TestWebhookService_TestAndCircuitBreaker(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-webhook-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to init SQLite: %v", err)
	}

	webhookRepo := sqlite.NewWebhookRepository(db)
	webhookService := service.NewWebhookService(webhookRepo)
	ctx := context.Background()

	// 1. Mock Receiver Server
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		sig := r.Header.Get("X-ProleadFile-Signature")
		if sig == "" {
			http.Error(w, "missing signature", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	}))
	defer server.Close()

	// 2. Register Webhook
	wh, err := webhookService.RegisterWebhook(ctx, service.RegisterWebhookInput{
		ProjectID: "p-default",
		Name:      "Test Endpoint",
		URL:       server.URL,
		Events:    []string{"file:uploaded"},
		Secret:    "test-secret-123",
	})
	if err != nil {
		t.Fatalf("Failed to register webhook: %v", err)
	}

	// 3. Test ping verification
	testResult, err := webhookService.TestWebhook(ctx, wh.ID)
	if err != nil {
		t.Fatalf("TestWebhook failed: %v", err)
	}
	if !testResult.Success {
		t.Errorf("Expected test webhook to succeed, got failure: %s", testResult.Error)
	}
	if testResult.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", testResult.StatusCode)
	}

	// 4. Test Circuit Breaker: Webhook with 25+ consecutive failures is tripped
	whBroken := &domain.Webhook{
		ID:           "wh-broken",
		ProjectID:    "p-default",
		Name:         "Broken Endpoint",
		URL:          "http://127.0.0.1:54321/dead-endpoint",
		Secret:       "sec",
		Events:       domain.StringList{"*"},
		Enabled:      true,
		FailureCount: 26,
	}
	if err := webhookRepo.Create(ctx, whBroken); err != nil {
		t.Fatalf("Failed to create broken webhook: %v", err)
	}

	eventHub := service.NewEventHub()
	webhookService.StartListening(ctx, eventHub)

	// Publish event - circuit breaker should skip broken webhook
	eventHub.Publish(service.EventFileUploaded, "default", "test.txt", nil)
	time.Sleep(50 * time.Millisecond)
}
