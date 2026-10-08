package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gostore/internal/api/middleware"
	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type StorageHandler struct {
	storageService *service.StorageService
	chunkService   *service.ChunkService
	zipStreamer    *service.ZipStreamer
	maxUploadBytes int64
}

func NewStorageHandler(
	storageService *service.StorageService,
	chunkService *service.ChunkService,
	zipStreamer *service.ZipStreamer,
	maxUploadMB int64,
) *StorageHandler {
	return &StorageHandler{
		storageService: storageService,
		chunkService:   chunkService,
		zipStreamer:    zipStreamer,
		maxUploadBytes: maxUploadMB * 1024 * 1024,
	}
}

// Upload handles standard multipart/form-data and direct binary streams
func (h *StorageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	if bucket == "" {
		bucket = "default"
	}

	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}

	contentType := r.Header.Get("Content-Type")
	var input service.UploadInput
	input.Bucket = bucket
	input.Metadata = make(map[string]string)

	for k, v := range r.Header {
		lowerK := strings.ToLower(k)
		if strings.HasPrefix(lowerK, "x-meta-") {
			input.Metadata[strings.TrimPrefix(lowerK, "x-meta-")] = v[0]
		} else if strings.HasPrefix(lowerK, "x-goog-meta-") {
			input.Metadata[strings.TrimPrefix(lowerK, "x-goog-meta-")] = v[0]
		}
	}

	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse multipart upload: %v", err))
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, handler, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing 'file' field in multipart form")
			return
		}
		defer file.Close()

		destPath := r.FormValue("path")
		if destPath == "" {
			destPath = r.FormValue("name")
		}
		if destPath == "" {
			destPath = handler.Filename
		}

		if isPubStr := r.FormValue("isPublic"); isPubStr != "" {
			isPub := isPubStr == "true" || isPubStr == "1"
			input.IsPublic = &isPub
		}

		if ttlSecStr := r.FormValue("expiresInSeconds"); ttlSecStr != "" {
			if ttlSec, err := strconv.Atoi(ttlSecStr); err == nil && ttlSec > 0 {
				dur := time.Duration(ttlSec) * time.Second
				input.ExpiresIn = &dur
			}
		}

		if metaStr := r.FormValue("metadata"); metaStr != "" {
			var customMeta map[string]string
			if err := json.Unmarshal([]byte(metaStr), &customMeta); err == nil {
				for k, v := range customMeta {
					input.Metadata[k] = v
				}
			}
		}

		input.Path = destPath
		input.ContentType = handler.Header.Get("Content-Type")
		input.Reader = file
	} else {
		destPath := r.URL.Query().Get("name")
		if destPath == "" {
			destPath = chi.URLParam(r, "*")
		}
		if destPath == "" {
			writeJSONError(w, http.StatusBadRequest, "Object destination name is required")
			return
		}

		if isPubStr := r.URL.Query().Get("isPublic"); isPubStr != "" {
			isPub := isPubStr == "true" || isPubStr == "1"
			input.IsPublic = &isPub
		}

		if ttlSecStr := r.URL.Query().Get("expiresInSeconds"); ttlSecStr != "" {
			if ttlSec, err := strconv.Atoi(ttlSecStr); err == nil && ttlSec > 0 {
				dur := time.Duration(ttlSec) * time.Second
				input.ExpiresIn = &dur
			}
		}

		input.Path = destPath
		input.ContentType = contentType
		input.Reader = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	}

	fileObj, err := h.storageService.Upload(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBucketNotFound):
			writeJSONError(w, http.StatusNotFound, "Bucket not found")
		case errors.Is(err, service.ErrFileTooLarge):
			writeJSONError(w, http.StatusRequestEntityTooLarge, "File exceeds maximum permitted size")
		case errors.Is(err, service.ErrMimeNotAllowed):
			writeJSONError(w, http.StatusBadRequest, "File MIME type not allowed in this bucket")
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(fileObj)
}

// Download serves file content with support for Image Resizing, Range requests, and Signed URLs
func (h *StorageHandler) Download(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rawPath := chi.URLParam(r, "*")
	if rawPath == "" {
		rawPath = chi.URLParam(r, "path")
	}

	filePath, err := url.PathUnescape(rawPath)
	if err != nil {
		filePath = rawPath
	}

	alt := r.URL.Query().Get("alt")
	if alt != "media" && r.URL.Query().Get("media") == "" && !strings.HasPrefix(r.URL.Path, "/v0/b/") {
		h.GetMetadata(w, r)
		return
	}

	token := r.URL.Query().Get("token")
	expires := r.URL.Query().Get("expires")
	sig := r.URL.Query().Get("sig")
	isAuth := middleware.GetAPIKeyFromContext(r.Context()) != nil

	stream, err := h.storageService.OpenDownload(r.Context(), bucket, filePath, token, expires, sig, isAuth)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFileNotFound):
			writeJSONError(w, http.StatusNotFound, "File not found")
		case errors.Is(err, service.ErrUnauthorized):
			writeJSONError(w, http.StatusForbidden, "Access denied: Valid download token or HMAC signature required")
		default:
			writeJSONError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	defer stream.Reader.Close()

	reqWidth, _ := strconv.Atoi(r.URL.Query().Get("w"))
	reqHeight, _ := strconv.Atoi(r.URL.Query().Get("h"))
	reqFit := r.URL.Query().Get("fit")
	reqFormat := r.URL.Query().Get("format")
	reqQuality, _ := strconv.Atoi(r.URL.Query().Get("q"))

	if (reqWidth > 0 || reqHeight > 0 || reqFormat != "") && strings.HasPrefix(stream.FileObject.ContentType, "image/") {
		opts := service.ImageTransformOptions{
			Width:   reqWidth,
			Height:  reqHeight,
			Fit:     reqFit,
			Format:  reqFormat,
			Quality: reqQuality,
		}
		imgBytes, outMime, err := h.storageService.TransformImage(stream.Reader, stream.FileObject.ID, opts)
		if err == nil {
			hash := md5.Sum(imgBytes)
			etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))
			w.Header().Set("ETag", etag)
			w.Header().Set("Content-Type", outMime)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

			if match := r.Header.Get("If-None-Match"); match != "" {
				if match == etag || match == hex.EncodeToString(hash[:]) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}

			w.Header().Set("Content-Length", strconv.Itoa(len(imgBytes)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imgBytes)
			return
		}
	}

	w.Header().Set("Content-Type", stream.FileObject.ContentType)
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, stream.FileObject.MD5Hash))
	w.Header().Set("Accept-Ranges", "bytes")

	if stream.FileObject.IsPublic {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}

	if match := r.Header.Get("If-None-Match"); match != "" {
		if match == fmt.Sprintf(`"%s"`, stream.FileObject.MD5Hash) || match == stream.FileObject.MD5Hash {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	fileName := filepath.Base(stream.FileObject.Path)
	http.ServeContent(w, r, fileName, stream.FileObject.UpdatedAt, stream.Reader)
}

// DownloadZip streams multiple selected files as a single on-the-fly ZIP archive
func (h *StorageHandler) DownloadZip(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Paths) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON or empty 'paths' list")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-archive.zip\"", bucket))

	if err := h.zipStreamer.StreamZip(r.Context(), bucket, req.Paths, w); err != nil {
		// Headers already sent, log or handle error
		return
	}
}

// RestoreFile un-deletes a file from trash
func (h *StorageHandler) RestoreFile(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rawPath := chi.URLParam(r, "*")
	if rawPath == "" {
		rawPath = r.URL.Query().Get("path")
	}
	filePath, _ := url.PathUnescape(rawPath)

	restored, err := h.storageService.Restore(r.Context(), bucket, filePath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(restored)
}

// ListVersions lists historical revisions of a file
func (h *StorageHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rawPath := chi.URLParam(r, "*")
	if rawPath == "" {
		rawPath = r.URL.Query().Get("path")
	}
	filePath, _ := url.PathUnescape(rawPath)

	versions, err := h.storageService.ListVersions(r.Context(), bucket, filePath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(versions)
}

// ScrubIntegrity performs an SHA-256 data integrity check on physical files
func (h *StorageHandler) ScrubIntegrity(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	res, err := h.storageService.ScrubIntegrity(r.Context(), bucket)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// SignURL generates a time-limited HMAC signed URL for private files
func (h *StorageHandler) SignURL(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		Path            string `json:"path"`
		DurationSeconds int    `json:"durationSeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	duration := time.Duration(req.DurationSeconds) * time.Second
	if duration <= 0 {
		duration = 1 * time.Hour
	}

	signedURL, expires, sig, err := h.storageService.GenerateSignedURL(bucket, req.Path, duration)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			writeJSONError(w, http.StatusNotFound, "File not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"signedUrl": signedURL,
		"expires":   expires,
		"signature": sig,
	})
}

// Resumable Chunk Upload Handlers
func (h *StorageHandler) InitChunkUpload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}

	var req struct {
		Path        string            `json:"path"`
		ContentType string            `json:"contentType"`
		TotalSize   int64             `json:"totalSize"`
		IsPublic    bool              `json:"isPublic"`
		Metadata    map[string]string `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	session, err := h.chunkService.InitUpload(bucket, req.Path, req.ContentType, req.TotalSize, req.IsPublic, req.Metadata)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(session)
}

func (h *StorageHandler) AppendChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	offsetStr := r.Header.Get("Upload-Offset")
	if offsetStr == "" {
		offsetStr = r.URL.Query().Get("offset")
	}
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)

	bytesReceived, err := h.chunkService.AppendChunk(uploadID, offset, r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"uploadId":      uploadID,
		"bytesReceived": bytesReceived,
	})
}

func (h *StorageHandler) AbortChunkUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	if err := h.chunkService.AbortUpload(uploadID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Upload aborted successfully",
	})
}

func (h *StorageHandler) CompleteChunkUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	fileObj, err := h.chunkService.CompleteUpload(r.Context(), uploadID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(fileObj)
}

// EventsSSE streams real-time Server-Sent Events to connected Angular apps
func (h *StorageHandler) EventsSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	hub := h.storageService.GetEventHub()
	if hub == nil {
		http.Error(w, "Event hub inactive", http.StatusServiceUnavailable)
		return
	}

	eventChan := hub.Subscribe()
	defer hub.Unsubscribe(eventChan)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case event := <-eventChan:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
			flusher.Flush()
		}
	}
}

// GetMetadata retrieves object metadata
func (h *StorageHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rawPath := chi.URLParam(r, "*")
	if rawPath == "" {
		rawPath = chi.URLParam(r, "path")
	}

	filePath, err := url.PathUnescape(rawPath)
	if err != nil {
		filePath = rawPath
	}

	fileObj, err := h.storageService.GetFileMetadata(r.Context(), bucket, filePath)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			writeJSONError(w, http.StatusNotFound, "File not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fileObj)
}

// List handles object listing including trash options
func (h *StorageHandler) List(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	if err := middleware.CheckBucketPermission(r.Context(), bucket, false); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}

	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")
	search := r.URL.Query().Get("search")
	trashOnly := r.URL.Query().Get("trash") == "true" || r.URL.Query().Get("trash") == "1"

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := domain.ListFilesFilter{
		Bucket:    bucket,
		Prefix:    prefix,
		Delimiter: delimiter,
		Limit:     limit,
		Offset:    offset,
		Search:    search,
		TrashOnly: trashOnly,
	}

	res, err := h.storageService.List(r.Context(), filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// Delete removes an object (supporting soft delete / trash)
func (h *StorageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	rawPath := chi.URLParam(r, "*")
	if rawPath == "" {
		rawPath = chi.URLParam(r, "path")
	}

	filePath, err := url.PathUnescape(rawPath)
	if err != nil {
		filePath = rawPath
	}

	// Default to soft delete (move to trash) unless permanent=true
	permanent := r.URL.Query().Get("permanent") == "true" || r.URL.Query().Get("purge") == "true"

	if err := h.storageService.Delete(r.Context(), bucket, filePath, !permanent); err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			writeJSONError(w, http.StatusNotFound, "File not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "File removed successfully",
		"isTrash": !permanent,
	})
}

// Copy duplicates an object to destination bucket/path
func (h *StorageHandler) Copy(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		SrcPath   string `json:"srcPath"`
		DstBucket string `json:"dstBucket"`
		DstPath   string `json:"dstPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.DstBucket == "" {
		req.DstBucket = bucket
	}
	if req.SrcPath == "" || req.DstPath == "" {
		writeJSONError(w, http.StatusBadRequest, "srcPath and dstPath are required")
		return
	}

	obj, err := h.storageService.CopyObject(r.Context(), bucket, req.SrcPath, req.DstBucket, req.DstPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(obj)
}

// Move moves an object to destination bucket/path
func (h *StorageHandler) Move(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		SrcPath   string `json:"srcPath"`
		DstBucket string `json:"dstBucket"`
		DstPath   string `json:"dstPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.DstBucket == "" {
		req.DstBucket = bucket
	}
	if req.SrcPath == "" || req.DstPath == "" {
		writeJSONError(w, http.StatusBadRequest, "srcPath and dstPath are required")
		return
	}

	obj, err := h.storageService.MoveObject(r.Context(), bucket, req.SrcPath, req.DstBucket, req.DstPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(obj)
}

// Rename renames an object
func (h *StorageHandler) Rename(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		OldPath string `json:"oldPath"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.OldPath == "" || req.NewName == "" {
		writeJSONError(w, http.StatusBadRequest, "oldPath and newName are required")
		return
	}

	obj, err := h.storageService.RenameObject(r.Context(), bucket, req.OldPath, req.NewName)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(obj)
}

// DeleteFolder deletes all objects under a folder prefix
func (h *StorageHandler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var req struct {
		Prefix    string `json:"prefix"`
		Permanent bool   `json:"permanent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.Prefix == "" {
		writeJSONError(w, http.StatusBadRequest, "prefix is required")
		return
	}

	count, err := h.storageService.DeleteFolder(r.Context(), bucket, req.Prefix, req.Permanent)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": count,
		"prefix":  req.Prefix,
	})
}

// RotateToken revokes old download token and issues a new one
func (h *StorageHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		rawPath = chi.URLParam(r, "*")
	}
	if rawPath == "" {
		rawPath = chi.URLParam(r, "path")
	}

	filePath, err := url.PathUnescape(rawPath)
	if err != nil {
		filePath = rawPath
	}

	if filePath == "" {
		writeJSONError(w, http.StatusBadRequest, "Object path is required (?path=...)")
		return
	}

	fileObj, err := h.storageService.RotateToken(r.Context(), bucket, filePath)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			writeJSONError(w, http.StatusNotFound, "File not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fileObj)
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    statusCode,
			"message": message,
		},
	})
}
