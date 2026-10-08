package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type BucketHandler struct {
	bucketService *service.BucketService
}

func NewBucketHandler(bucketService *service.BucketService) *BucketHandler {
	return &BucketHandler{bucketService: bucketService}
}

func (h *BucketHandler) Create(w http.ResponseWriter, r *http.Request) {
	var b domain.Bucket
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload: "+err.Error())
		return
	}

	if b.ProjectID == "" {
		b.ProjectID = r.Header.Get("X-Project-ID")
		if b.ProjectID == "" {
			b.ProjectID = r.URL.Query().Get("projectId")
		}
	}

	created, err := h.bucketService.CreateBucket(r.Context(), &b)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (h *BucketHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.Header.Get("X-Project-ID")
	if projectID == "" {
		projectID = r.URL.Query().Get("projectId")
	}

	buckets, err := h.bucketService.ListBuckets(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buckets)
}

func (h *BucketHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "bucket")
	bucket, err := h.bucketService.GetBucket(r.Context(), name)
	if err != nil {
		if errors.Is(err, service.ErrBucketNotFound) {
			writeJSONError(w, http.StatusNotFound, "Bucket not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(bucket)
}

func (h *BucketHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "bucket")
	var b domain.Bucket
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	b.Name = name

	updated, err := h.bucketService.UpdateBucket(r.Context(), &b)
	if err != nil {
		if errors.Is(err, service.ErrBucketNotFound) {
			writeJSONError(w, http.StatusNotFound, "Bucket not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *BucketHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "bucket")
	if err := h.bucketService.DeleteBucket(r.Context(), name); err != nil {
		if errors.Is(err, service.ErrBucketNotFound) {
			writeJSONError(w, http.StatusNotFound, "Bucket not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Bucket deleted successfully",
	})
}

func (h *BucketHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.bucketService.GetStats(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
