package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gostore/internal/service"
)

type TUSHandler struct {
	chunkService   *service.ChunkService
	storageService *service.StorageService
}

func NewTUSHandler(chunkService *service.ChunkService, storageService *service.StorageService) *TUSHandler {
	return &TUSHandler{
		chunkService:   chunkService,
		storageService: storageService,
	}
}

func (h *TUSHandler) Options(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Tus-Resumable", "1.0.0")
	w.Header().Set("Tus-Version", "1.0.0")
	w.Header().Set("Tus-Extension", "creation,termination,checksum,expiration")
	w.Header().Set("Tus-Max-Size", "107374182400") // 100 GB
	w.WriteHeader(http.StatusNoContent)
}

func (h *TUSHandler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Tus-Resumable", "1.0.0")

	lengthStr := r.Header.Get("Upload-Length")
	if lengthStr == "" {
		http.Error(w, "Missing Upload-Length header", http.StatusBadRequest)
		return
	}

	totalSize, err := strconv.ParseInt(lengthStr, 10, 64)
	if err != nil || totalSize < 0 {
		http.Error(w, "Invalid Upload-Length", http.StatusBadRequest)
		return
	}

	metadataHeader := r.Header.Get("Upload-Metadata")
	meta := parseTusMetadata(metadataHeader)

	bucket := meta["bucket"]
	if bucket == "" {
		bucket = "default"
	}

	filename := meta["filename"]
	if filename == "" {
		filename = "unnamed_upload"
	}

	path := meta["path"]
	if path == "" {
		path = filename
	}

	contentType := meta["filetype"]
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	isPublic := meta["isPublic"] == "true"

	session, err := h.chunkService.InitUpload(bucket, path, contentType, totalSize, isPublic, meta)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to initialize TUS session: %v", err), http.StatusInternalServerError)
		return
	}

	uploadLocation := fmt.Sprintf("/api/v1/tus/files/%s", session.ID)
	w.Header().Set("Location", uploadLocation)
	w.WriteHeader(http.StatusCreated)
}

func (h *TUSHandler) HeadUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	w.Header().Set("Tus-Resumable", "1.0.0")
	w.Header().Set("Cache-Control", "no-store")

	session, exists := h.chunkService.GetSession(uploadID)
	if !exists {
		http.Error(w, "TUS upload session not found or completed", http.StatusNotFound)
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(session.BytesSoFar, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(session.TotalSize, 10))
	w.WriteHeader(http.StatusOK)
}

func (h *TUSHandler) PatchUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	w.Header().Set("Tus-Resumable", "1.0.0")

	session, exists := h.chunkService.GetSession(uploadID)
	if !exists {
		http.Error(w, "TUS upload session not found", http.StatusNotFound)
		return
	}

	offsetStr := r.Header.Get("Upload-Offset")
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil || offset != session.BytesSoFar {
		http.Error(w, fmt.Sprintf("Offset mismatch: expected %d, got %s", session.BytesSoFar, offsetStr), http.StatusConflict)
		return
	}

	newTotal, err := h.chunkService.AppendChunk(uploadID, offset, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to append chunk: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Upload-Offset", strconv.FormatInt(newTotal, 10))

	// Finalize if complete
	if session.TotalSize > 0 && newTotal >= session.TotalSize {
		_, err := h.chunkService.CompleteUpload(r.Context(), uploadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to finalize upload: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TUSHandler) TerminateUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	w.Header().Set("Tus-Resumable", "1.0.0")

	if err := h.chunkService.AbortUpload(uploadID); err != nil {
		http.Error(w, "Upload not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseTusMetadata(header string) map[string]string {
	result := make(map[string]string)
	if header == "" {
		return result
	}

	pairs := strings.Split(header, ",")
	for _, pair := range pairs {
		trimmed := strings.TrimSpace(pair)
		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
			if err == nil {
				result[k] = string(decoded)
			} else {
				result[k] = parts[1]
			}
		} else if len(parts) == 1 {
			result[parts[0]] = ""
		}
	}
	return result
}
