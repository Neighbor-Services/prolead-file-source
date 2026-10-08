package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gostore/internal/service"
)

type WebhookHandler struct {
	webhookService *service.WebhookService
}

func NewWebhookHandler(webhookService *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookService: webhookService}
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterWebhookInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "Webhook URL is required")
		return
	}

	projectID := req.ProjectID
	if projectID == "" {
		projectID = r.Header.Get("X-Project-ID")
		if projectID == "" {
			projectID = chi.URLParam(r, "projectId")
			if projectID == "" {
				projectID = r.URL.Query().Get("projectId")
			}
		}
	}
	req.ProjectID = projectID

	wh, err := h.webhookService.RegisterWebhook(r.Context(), req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(wh)
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.Header.Get("X-Project-ID")
	if projectID == "" {
		projectID = chi.URLParam(r, "projectId")
		if projectID == "" {
			projectID = r.URL.Query().Get("projectId")
		}
	}

	webhooks, err := h.webhookService.ListWebhooks(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webhooks)
}

func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	wh, err := h.webhookService.GetWebhook(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "Webhook not found: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(wh)
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req service.RegisterWebhookInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	wh, err := h.webhookService.UpdateWebhook(r.Context(), id, req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(wh)
}

func (h *WebhookHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if err := h.webhookService.ToggleEnabled(r.Context(), id, req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"enabled": req.Enabled,
		"message": "Webhook status updated",
	})
}

func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.webhookService.TestWebhook(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Test ping failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	deliveries, err := h.webhookService.ListDeliveries(r.Context(), id, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(deliveries)
}

func (h *WebhookHandler) Redeliver(w http.ResponseWriter, r *http.Request) {
	deliveryID := chi.URLParam(r, "deliveryId")
	result, err := h.webhookService.Redeliver(r.Context(), deliveryID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Redelivery failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.webhookService.DeleteWebhook(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Webhook deleted successfully",
	})
}
