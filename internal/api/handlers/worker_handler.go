package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gostore/internal/service"
)

type WorkerHandler struct {
	pipeline *service.WorkerPipeline
}

func NewWorkerHandler(pipeline *service.WorkerPipeline) *WorkerHandler {
	return &WorkerHandler{pipeline: pipeline}
}

func (h *WorkerHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	stats := h.pipeline.GetStats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (h *WorkerHandler) GetJobs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	jobs := h.pipeline.GetRecentJobs(limit)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

type TriggerJobRequest struct {
	Type     service.JobType `json:"type"`
	Bucket   string          `json:"bucket,omitempty"`
	Path     string          `json:"path,omitempty"`
	Priority int             `json:"priority,omitempty"`
	Payload  string          `json:"payload,omitempty"`
}

func (h *WorkerHandler) TriggerJob(w http.ResponseWriter, r *http.Request) {
	var req TriggerJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	if req.Type == "" {
		req.Type = service.JobTypeIntegrityScrub
	}

	jobID := h.pipeline.EnqueueJob(service.WorkerJob{
		Type:     req.Type,
		Bucket:   req.Bucket,
		Path:     req.Path,
		Priority: req.Priority,
		Payload:  req.Payload,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"jobId":   jobID,
		"message": "Background worker job queued successfully",
		"type":    req.Type,
	})
}

type ScaleWorkersRequest struct {
	Workers int `json:"workers"`
}

func (h *WorkerHandler) ScaleWorkers(w http.ResponseWriter, r *http.Request) {
	var req ScaleWorkersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	if req.Workers <= 0 || req.Workers > 32 {
		writeJSONError(w, http.StatusBadRequest, "Workers must be between 1 and 32")
		return
	}

	newCount := h.pipeline.ScaleWorkers(req.Workers)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"totalWorkers": newCount,
		"message":      "Worker pool concurrency updated",
	})
}

func (h *WorkerHandler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	h.pipeline.ClearHistory()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Worker jobs history cleared",
	})
}
