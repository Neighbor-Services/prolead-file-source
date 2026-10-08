package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type WebhookService struct {
	repo       *sqlite.WebhookRepository
	httpClient *http.Client
}

func NewWebhookService(repo *sqlite.WebhookRepository) *WebhookService {
	return &WebhookService{
		repo: repo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type RegisterWebhookInput struct {
	ProjectID string   `json:"projectId"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret"`
	Enabled   *bool    `json:"enabled,omitempty"`
}

type TestWebhookResult struct {
	Success      bool   `json:"success"`
	StatusCode   int    `json:"statusCode"`
	DurationMs   int64  `json:"durationMs"`
	RequestBody  string `json:"requestBody"`
	ResponseBody string `json:"responseBody"`
	Signature    string `json:"signature"`
	Error        string `json:"error,omitempty"`
	DeliveryID   string `json:"deliveryId"`
}

func (s *WebhookService) RegisterWebhook(ctx context.Context, input RegisterWebhookInput) (*domain.Webhook, error) {
	if input.Secret == "" {
		secretBytes := make([]byte, 16)
		_, _ = rand.Read(secretBytes)
		input.Secret = "whsec_" + hex.EncodeToString(secretBytes)
	}
	if input.ProjectID == "" {
		input.ProjectID = "p-default"
	}
	if input.Name == "" {
		input.Name = "Webhook Listener"
	}
	if len(input.Events) == 0 {
		input.Events = []string{"*"}
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	wh := &domain.Webhook{
		ID:        uuid.New().String(),
		ProjectID: input.ProjectID,
		Name:      input.Name,
		URL:       input.URL,
		Secret:    input.Secret,
		Events:    input.Events,
		Enabled:   enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, wh); err != nil {
		return nil, err
	}
	return wh, nil
}

func (s *WebhookService) GetWebhook(ctx context.Context, id string) (*domain.Webhook, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *WebhookService) ListWebhooks(ctx context.Context, projectID string) ([]domain.Webhook, error) {
	return s.repo.List(ctx, projectID)
}

func (s *WebhookService) UpdateWebhook(ctx context.Context, id string, input RegisterWebhookInput) (*domain.Webhook, error) {
	wh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name != "" {
		wh.Name = input.Name
	}
	if input.URL != "" {
		wh.URL = input.URL
	}
	if input.Secret != "" {
		wh.Secret = input.Secret
	}
	if len(input.Events) > 0 {
		wh.Events = input.Events
	}
	if input.Enabled != nil {
		wh.Enabled = *input.Enabled
	}

	if err := s.repo.Update(ctx, wh); err != nil {
		return nil, err
	}
	return wh, nil
}

func (s *WebhookService) ToggleEnabled(ctx context.Context, id string, enabled bool) error {
	return s.repo.ToggleEnabled(ctx, id, enabled)
}

func (s *WebhookService) DeleteWebhook(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *WebhookService) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error) {
	return s.repo.ListDeliveries(ctx, webhookID, limit)
}

// TestWebhook triggers a synthetic verification ping payload to the target URL
func (s *WebhookService) TestWebhook(ctx context.Context, id string) (*TestWebhookResult, error) {
	wh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	testPayload := map[string]interface{}{
		"event":       "webhook.test",
		"timestamp":   now.Format(time.RFC3339),
		"webhookId":   wh.ID,
		"projectId":   wh.ProjectID,
		"description": "Synthetic verification ping dispatched from Prolead File Engine.",
		"payload": map[string]interface{}{
			"bucket":    "default",
			"fileName":  "test-ping.txt",
			"fileSize":  1024,
			"mimeType":  "text/plain",
			"triggered": "Manual Dashboard Ping",
		},
	}

	payloadBytes, err := json.MarshalIndent(testPayload, "", "  ")
	if err != nil {
		return nil, err
	}

	deliveryID := uuid.New().String()
	timestampStr := strconv.FormatInt(now.Unix(), 10)
	sig := s.computeSignature(payloadBytes, wh.Secret)

	req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ProleadFile-Webhook-Engine/1.0")
	req.Header.Set("X-ProleadFile-Event", "webhook.test")
	req.Header.Set("X-ProleadFile-Delivery", deliveryID)
	req.Header.Set("X-ProleadFile-Timestamp", timestampStr)
	req.Header.Set("X-ProleadFile-Signature", fmt.Sprintf("sha256=%s", sig))
	req.Header.Set("X-GoStore-Signature", fmt.Sprintf("sha256=%s", sig))

	start := time.Now()
	resp, sendErr := s.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()

	result := &TestWebhookResult{
		DeliveryID:  deliveryID,
		RequestBody: string(payloadBytes),
		Signature:   fmt.Sprintf("sha256=%s", sig),
		DurationMs:  duration,
	}

	var statusCode int
	var respBodyStr string
	var success bool

	if sendErr != nil {
		statusCode = 0
		success = false
		result.Error = sendErr.Error()
	} else {
		statusCode = resp.StatusCode
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		respBodyStr = string(respBytes)
		success = statusCode >= 200 && statusCode < 300
		result.StatusCode = statusCode
		result.ResponseBody = respBodyStr
		result.Success = success
		if !success {
			result.Error = fmt.Sprintf("HTTP %d: %s", statusCode, http.StatusText(statusCode))
		}
	}

	// Record delivery log
	delivery := &domain.WebhookDelivery{
		ID:           deliveryID,
		WebhookID:    wh.ID,
		ProjectID:    wh.ProjectID,
		Event:        "webhook.test",
		URL:          wh.URL,
		StatusCode:   statusCode,
		DurationMs:   duration,
		Success:      success,
		Error:        result.Error,
		RequestBody:  string(payloadBytes),
		ResponseBody: respBodyStr,
		CreatedAt:    now,
	}
	_ = s.repo.SaveDelivery(context.Background(), delivery)
	_ = s.repo.RecordDeliveryStats(context.Background(), wh.ID, statusCode, success)

	return result, nil
}

// Redeliver replays an existing webhook delivery attempt
func (s *WebhookService) Redeliver(ctx context.Context, deliveryID string) (*TestWebhookResult, error) {
	delivery, err := s.repo.GetDeliveryByID(ctx, deliveryID)
	if err != nil {
		return nil, err
	}

	wh, err := s.repo.GetByID(ctx, delivery.WebhookID)
	if err != nil {
		return nil, err
	}

	newDeliveryID := uuid.New().String()
	now := time.Now().UTC()
	payloadBytes := []byte(delivery.RequestBody)
	sig := s.computeSignature(payloadBytes, wh.Secret)

	req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ProleadFile-Webhook-Engine/1.0 (Redelivery)")
	req.Header.Set("X-ProleadFile-Event", delivery.Event)
	req.Header.Set("X-ProleadFile-Delivery", newDeliveryID)
	req.Header.Set("X-ProleadFile-Timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("X-ProleadFile-Signature", fmt.Sprintf("sha256=%s", sig))
	req.Header.Set("X-GoStore-Signature", fmt.Sprintf("sha256=%s", sig))

	start := time.Now()
	resp, sendErr := s.httpClient.Do(req)
	duration := time.Since(start).Milliseconds()

	result := &TestWebhookResult{
		DeliveryID:  newDeliveryID,
		RequestBody: string(payloadBytes),
		Signature:   fmt.Sprintf("sha256=%s", sig),
		DurationMs:  duration,
	}

	var statusCode int
	var respBodyStr string
	var success bool

	if sendErr != nil {
		statusCode = 0
		success = false
		result.Error = sendErr.Error()
	} else {
		statusCode = resp.StatusCode
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		respBodyStr = string(respBytes)
		success = statusCode >= 200 && statusCode < 300
		result.StatusCode = statusCode
		result.ResponseBody = respBodyStr
		result.Success = success
		if !success {
			result.Error = fmt.Sprintf("HTTP %d: %s", statusCode, http.StatusText(statusCode))
		}
	}

	// Record delivery log
	newDel := &domain.WebhookDelivery{
		ID:           newDeliveryID,
		WebhookID:    wh.ID,
		ProjectID:    wh.ProjectID,
		Event:        delivery.Event,
		URL:          wh.URL,
		StatusCode:   statusCode,
		DurationMs:   duration,
		Success:      success,
		Error:        result.Error,
		RequestBody:  string(payloadBytes),
		ResponseBody: respBodyStr,
		CreatedAt:    now,
	}
	_ = s.repo.SaveDelivery(context.Background(), newDel)
	_ = s.repo.RecordDeliveryStats(context.Background(), wh.ID, statusCode, success)

	return result, nil
}

// StartListening subscribes to EventHub and dispatches outgoing webhooks asynchronously
func (s *WebhookService) StartListening(ctx context.Context, hub *EventHub) {
	eventChan := hub.Subscribe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventChan:
				go s.dispatchEvent(event)
			}
		}
	}()
}

func (s *WebhookService) dispatchEvent(event Event) {
	webhooks, err := s.repo.List(context.Background(), "")
	if err != nil || len(webhooks) == 0 {
		return
	}

	payloadBytes, err := json.Marshal(event)
	if err != nil {
		return
	}

	eventTypeStr := string(event.Type)

	for _, wh := range webhooks {
		if !wh.Enabled {
			continue
		}

		// Check if webhook is subscribed to this event type
		matched := false
		for _, e := range wh.Events {
			if e == eventTypeStr || e == "*" || (e == "file:uploaded" && eventTypeStr == "file.created") || (e == "file:deleted" && eventTypeStr == "file.deleted") {
				matched = true
				break
			}
		}

		if matched {
			go s.sendWebhook(wh, eventTypeStr, payloadBytes)
		}
	}
}

func (s *WebhookService) sendWebhook(wh domain.Webhook, eventType string, payload []byte) {
	// Circuit Breaker: skip if failed 25+ consecutive times to prevent resource exhaustion
	if wh.FailureCount >= 25 {
		log.Printf("⚠️ Webhook %s (%s) tripped circuit breaker (failures >= 25). Skipping delivery.", wh.ID, wh.URL)
		return
	}

	maxRetries := 3
	var finalStatusCode int
	var finalDuration int64
	var finalSuccess bool
	var finalErrMsg string
	var finalRespBody string
	var deliveryID string

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s
			backoff := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
			time.Sleep(backoff)
		}

		deliveryID = uuid.New().String()
		now := time.Now().UTC()
		sig := s.computeSignature(payload, wh.Secret)

		req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(payload))
		if err != nil {
			finalErrMsg = err.Error()
			break
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "ProleadFile-Webhook-Engine/1.0")
		req.Header.Set("X-ProleadFile-Event", eventType)
		req.Header.Set("X-ProleadFile-Delivery", deliveryID)
		req.Header.Set("X-ProleadFile-Timestamp", strconv.FormatInt(now.Unix(), 10))
		req.Header.Set("X-ProleadFile-Signature", fmt.Sprintf("sha256=%s", sig))
		req.Header.Set("X-GoStore-Signature", fmt.Sprintf("sha256=%s", sig))

		start := time.Now()
		resp, sendErr := s.httpClient.Do(req)
		finalDuration = time.Since(start).Milliseconds()

		if sendErr != nil {
			finalStatusCode = 0
			finalSuccess = false
			finalErrMsg = sendErr.Error()
			continue
		}

		finalStatusCode = resp.StatusCode
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		_ = resp.Body.Close()
		finalRespBody = string(respBytes)
		finalSuccess = finalStatusCode >= 200 && finalStatusCode < 300
		if finalSuccess {
			finalErrMsg = ""
			break
		} else {
			finalErrMsg = fmt.Sprintf("HTTP %d", finalStatusCode)
			// Only retry on server errors (5xx) or timeout, not 4xx client errors
			if finalStatusCode < 500 {
				break
			}
		}
	}

	// Record delivery history
	delivery := &domain.WebhookDelivery{
		ID:           deliveryID,
		WebhookID:    wh.ID,
		ProjectID:    wh.ProjectID,
		Event:        eventType,
		URL:          wh.URL,
		StatusCode:   finalStatusCode,
		DurationMs:   finalDuration,
		Success:      finalSuccess,
		Error:        finalErrMsg,
		RequestBody:  string(payload),
		ResponseBody: finalRespBody,
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.repo.SaveDelivery(context.Background(), delivery)
	_ = s.repo.RecordDeliveryStats(context.Background(), wh.ID, finalStatusCode, finalSuccess)

	// If failure count reached threshold after this delivery, trip circuit breaker and disable
	if !finalSuccess && wh.FailureCount+1 >= 25 {
		log.Printf("🛑 Webhook %s (%s) exceeded 25 consecutive failures. Automatically disabling webhook.", wh.ID, wh.URL)
		_ = s.repo.ToggleEnabled(context.Background(), wh.ID, false)
	}
}

func (s *WebhookService) computeSignature(payload []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
