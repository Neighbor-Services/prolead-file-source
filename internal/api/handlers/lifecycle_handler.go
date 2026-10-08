package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
	"gostore/internal/service"
)

type LifecycleHandler struct {
	lifecycleService *service.LifecycleService
	lifecycleRepo    *sqlite.LifecycleRepository
}

func NewLifecycleHandler(lifecycleService *service.LifecycleService, lifecycleRepo *sqlite.LifecycleRepository) *LifecycleHandler {
	return &LifecycleHandler{
		lifecycleService: lifecycleService,
		lifecycleRepo:    lifecycleRepo,
	}
}

// GetRule gets lifecycle retention settings for a bucket
func (h *LifecycleHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rule, err := h.lifecycleRepo.GetByBucket(r.Context(), bucket)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if rule == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"bucket":         bucket,
			"trashDays":      30,
			"versionLimit":   5,
			"expirationDays": 0,
			"enabled":        true,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(rule)
}

// SetRule updates bucket lifecycle retention rules
func (h *LifecycleHandler) SetRule(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var rule domain.LifecycleRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule.Bucket = bucket

	if err := h.lifecycleRepo.SaveRule(r.Context(), &rule); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rule)
}

// TriggerSweep manually runs the retention and garbage collector sweep
func (h *LifecycleHandler) TriggerSweep(w http.ResponseWriter, r *http.Request) {
	go h.lifecycleService.RunLifecycleSweep(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Lifecycle retention sweep triggered in background"})
}

// TriggerScrubber runs the CAS bit-rot scrubber
func (h *LifecycleHandler) TriggerScrubber(w http.ResponseWriter, r *http.Request) {
	report, err := h.lifecycleService.RunScrubber(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "report": report})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}
