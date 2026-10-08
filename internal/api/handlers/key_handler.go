package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gostore/internal/service"
)

type KeyHandler struct {
	keyService *service.KeyService
}

func NewKeyHandler(keyService *service.KeyService) *KeyHandler {
	return &KeyHandler{keyService: keyService}
}

type CreateKeyRequest struct {
	ProjectID          string     `json:"projectId"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	Permissions        []string   `json:"permissions"`
	AllowedBuckets     []string   `json:"allowedBuckets"`
	AllowedOrigins     []string   `json:"allowedOrigins"`
	RateLimitReqPerMin int        `json:"rateLimitReqPerMin"`
	ExpiresInDays      int        `json:"expiresInDays"`
	ExpiresAt          *time.Time `json:"expiresAt"`
}

func (h *KeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "Key name is required")
		return
	}

	projectID := req.ProjectID
	if projectID == "" {
		projectID = r.Header.Get("X-Project-ID")
		if projectID == "" {
			projectID = r.URL.Query().Get("projectId")
		}
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	} else if req.ExpiresInDays > 0 {
		exp := time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	input := service.GenerateKeyInput{
		ProjectID:          projectID,
		Name:               req.Name,
		Role:               req.Role,
		Permissions:        req.Permissions,
		AllowedBuckets:     req.AllowedBuckets,
		AllowedOrigins:     req.AllowedOrigins,
		RateLimitReqPerMin: req.RateLimitReqPerMin,
		ExpiresAt:          expiresAt,
		CreatedBy:          "admin",
	}

	key, err := h.keyService.GenerateKey(r.Context(), input)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(key)
}

func (h *KeyHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.Header.Get("X-Project-ID")
	if projectID == "" {
		projectID = r.URL.Query().Get("projectId")
	}

	keys, err := h.keyService.ListKeys(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(keys)
}

func (h *KeyHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key, err := h.keyService.RotateKey(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(key)
}

func (h *KeyHandler) ToggleRevoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Revoked bool `json:"revoked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Revoked = true
	}

	if err := h.keyService.ToggleRevoke(r.Context(), id, req.Revoked); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"revoked": req.Revoked,
		"message": "Key status updated successfully",
	})
}

func (h *KeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.keyService.RevokeKey(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "API key revoked successfully",
	})
}

func (h *KeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.keyService.DeleteKey(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "API key deleted successfully",
	})
}
