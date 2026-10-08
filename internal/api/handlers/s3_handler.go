package handlers

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gostore/internal/api/middleware"
	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type S3Handler struct {
	storageService *service.StorageService
	bucketService  *service.BucketService
}

func NewS3Handler(storageService *service.StorageService, bucketService *service.BucketService) *S3Handler {
	return &S3Handler{
		storageService: storageService,
		bucketService:  bucketService,
	}
}

// S3 XML Structs
type S3ListBucketsResult struct {
	XMLName xml.Name      `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListAllMyBucketsResult"`
	Owner   S3Owner       `xml:"Owner"`
	Buckets S3BucketsList `xml:"Buckets"`
}

type S3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type S3BucketsList struct {
	Bucket []S3BucketItem `xml:"Bucket"`
}

type S3BucketItem struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type S3ListBucketResult struct {
	XMLName        xml.Name           `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`
	Name           string             `xml:"Name"`
	Prefix         string             `xml:"Prefix"`
	Delimiter      string             `xml:"Delimiter,omitempty"`
	MaxKeys        int                `xml:"MaxKeys"`
	IsTruncated    bool               `xml:"IsTruncated"`
	Contents       []S3ObjectItem     `xml:"Contents"`
	CommonPrefixes []S3CommonPrefixes `xml:"CommonPrefixes,omitempty"`
}

type S3ObjectItem struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type S3CommonPrefixes struct {
	Prefix string `xml:"Prefix"`
}

type S3ErrorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestId string   `xml:"RequestId"`
}

// HandleBucketOrObject dispatches S3 requests based on HTTP Method and path
func (h *S3Handler) HandleBucketOrObject(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	key := chi.URLParam(r, "*")

	if key == "" || key == "/" {
		switch r.Method {
		case http.MethodGet:
			h.ListObjectsV2(w, r, bucket)
		case http.MethodHead:
			h.HeadBucket(w, r, bucket)
		case http.MethodPut:
			h.CreateBucket(w, r, bucket)
		case http.MethodDelete:
			h.DeleteBucket(w, r, bucket)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
		}
		return
	}

	// Object operations
	key = strings.TrimPrefix(key, "/")
	switch r.Method {
	case http.MethodGet:
		h.GetObject(w, r, bucket, key)
	case http.MethodHead:
		h.HeadObject(w, r, bucket, key)
	case http.MethodPut:
		h.PutObject(w, r, bucket, key)
	case http.MethodDelete:
		h.DeleteObject(w, r, bucket, key)
	default:
		h.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed")
	}
}

// ListBuckets handles GET /s3
func (h *S3Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.bucketService.ListBuckets(r.Context(), "")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}

	var items []S3BucketItem
	for _, b := range buckets {
		items = append(items, S3BucketItem{
			Name:         b.Name,
			CreationDate: b.CreatedAt.Format(time.RFC3339),
		})
	}

	res := S3ListBucketsResult{
		Owner: S3Owner{ID: "prolead-admin", DisplayName: "Prolead Admin"},
		Buckets: S3BucketsList{
			Bucket: items,
		},
	}

	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(res)
}

// ListObjectsV2 lists objects inside a bucket in S3 XML format
func (h *S3Handler) ListObjectsV2(w http.ResponseWriter, r *http.Request, bucket string) {
	prefix := r.URL.Query().Get("prefix")
	delimiter := r.URL.Query().Get("delimiter")
	maxKeysStr := r.URL.Query().Get("max-keys")
	maxKeys := 1000
	if maxKeysStr != "" {
		if k, err := strconv.Atoi(maxKeysStr); err == nil && k > 0 {
			maxKeys = k
		}
	}

	res, err := h.storageService.ListFiles(r.Context(), domain.ListFilesFilter{
		Bucket:    bucket,
		Prefix:    prefix,
		Delimiter: delimiter,
		Limit:     maxKeys,
	})
	if err != nil {
		h.writeError(w, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
		return
	}

	var objects []S3ObjectItem
	for _, f := range res.Items {
		if strings.HasSuffix(f.Name, ".keep") || strings.HasSuffix(f.Path, ".keep") {
			continue
		}
		objects = append(objects, S3ObjectItem{
			Key:          f.Path,
			LastModified: f.UpdatedAt.Format(time.RFC3339),
			ETag:         fmt.Sprintf("\"%s\"", f.SHA256Hash),
			Size:         f.Size,
			StorageClass: "STANDARD",
		})
	}

	var prefixes []S3CommonPrefixes
	for _, p := range res.Prefixes {
		prefixes = append(prefixes, S3CommonPrefixes{Prefix: p})
	}

	result := S3ListBucketResult{
		Name:           bucket,
		Prefix:         prefix,
		Delimiter:      delimiter,
		MaxKeys:        maxKeys,
		IsTruncated:    false,
		Contents:       objects,
		CommonPrefixes: prefixes,
	}

	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(result)
}

func (h *S3Handler) HeadBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	b, err := h.bucketService.GetBucket(r.Context(), bucket)
	if err != nil || b == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) CreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		h.writeError(w, http.StatusForbidden, "AccessDenied", err.Error())
		return
	}
	_, err := h.bucketService.CreateBucket(r.Context(), &domain.Bucket{
		Name:     bucket,
		IsPublic: true,
	})
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "BucketAlreadyExists", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) DeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		h.writeError(w, http.StatusForbidden, "AccessDenied", err.Error())
		return
	}
	_ = h.bucketService.DeleteBucket(r.Context(), bucket)
	w.WriteHeader(http.StatusNoContent)
}

func (h *S3Handler) PutObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		h.writeError(w, http.StatusForbidden, "AccessDenied", err.Error())
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	isPublic := true
	obj, err := h.storageService.Upload(r.Context(), service.UploadInput{
		Bucket:      bucket,
		Path:        key,
		ContentType: contentType,
		IsPublic:    &isPublic,
		Reader:      r.Body,
	})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}

	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", obj.SHA256Hash))
	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) GetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	stream, err := h.storageService.OpenDownload(r.Context(), bucket, key, "", "", "", true)
	if err != nil || stream == nil {
		h.writeError(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	defer stream.Reader.Close()

	w.Header().Set("Content-Type", stream.FileObject.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(stream.FileObject.Size, 10))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", stream.FileObject.SHA256Hash))
	w.Header().Set("Last-Modified", stream.FileObject.UpdatedAt.Format(http.TimeFormat))

	http.ServeContent(w, r, stream.FileObject.Name, stream.FileObject.UpdatedAt, stream.Reader)
}

func (h *S3Handler) HeadObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	obj, err := h.storageService.GetFileMetadata(r.Context(), bucket, key)
	if err != nil || obj == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", obj.SHA256Hash))
	w.Header().Set("Last-Modified", obj.UpdatedAt.Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
}

func (h *S3Handler) DeleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if err := middleware.CheckBucketPermission(r.Context(), bucket, true); err != nil {
		h.writeError(w, http.StatusForbidden, "AccessDenied", err.Error())
		return
	}
	_ = h.storageService.Delete(r.Context(), bucket, key, false)
	w.WriteHeader(http.StatusNoContent)
}

func (h *S3Handler) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_ = xml.NewEncoder(w).Encode(S3ErrorResponse{
		Code:      code,
		Message:   message,
		RequestId: fmt.Sprintf("%d", time.Now().UnixNano()),
	})
}
