package proleadfile

import (
	"strconv"
	"time"
)

type StorageObject struct {
	ID                 string            `json:"id"`
	Bucket             string            `json:"bucket"`
	Path               string            `json:"path"`
	Name               string            `json:"name"`
	Size               int64             `json:"size"`
	ContentType        string            `json:"contentType"`
	SHA256             string            `json:"sha256"`
	DownloadToken      string            `json:"downloadToken"`
	DownloadURL        string            `json:"downloadUrl"`
	IsPublic           bool              `json:"isPublic"`
	ExpiresAt          *time.Time        `json:"expiresAt,omitempty"`
	DeletedAt          *time.Time        `json:"deletedAt,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Version            int               `json:"version"`
	LQIP               string            `json:"lqip,omitempty"`
	IsCompressed       bool              `json:"isCompressed,omitempty"`
	OriginalSize       int64             `json:"originalSize,omitempty"`
	CompressedSize     int64             `json:"compressedSize,omitempty"`
	CompressionRatio   float64           `json:"compressionRatio,omitempty"`
	SavingsPercent     float64           `json:"savingsPercent,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type FileVersion struct {
	ID            string    `json:"id"`
	Bucket        string    `json:"bucket"`
	Path          string    `json:"path"`
	Version       int       `json:"version"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	DownloadToken string    `json:"downloadToken"`
	CreatedAt     time.Time `json:"createdAt"`
}

type StorageBucket struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsPublic     bool      `json:"isPublic"`
	MaxFileSize  int64     `json:"maxFileSize"`
	AllowedMimes []string  `json:"allowedMimes"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type StorageStats struct {
	TotalFiles         int64   `json:"totalFiles"`
	TotalLogicalBytes  int64   `json:"totalLogicalBytes"`
	TotalPhysicalBytes int64   `json:"totalPhysicalBytes"`
	TotalSavedBytes    int64   `json:"totalSavedBytes"`
	DedupRatio         float64 `json:"dedupRatio"`
	ActiveBuckets      int64   `json:"activeBuckets"`
	ActiveVersions     int64   `json:"activeVersions"`
	TrashFiles         int64   `json:"trashFiles"`
}

type ListFilesFilter struct {
	Bucket       string
	Prefix       string
	Delimiter    string
	Limit        int
	Offset       int
	Search       string
	TrashOnly    bool
	IncludeTrash bool
}

type ListFilesResult struct {
	Items    []StorageObject `json:"items"`
	Prefixes []string        `json:"prefixes"`
	Total    int64           `json:"total"`
}

type SignedUrlResult struct {
	SignedURL string    `json:"signedUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ImageTransformOptions struct {
	Width   int
	Height  int
	Fit     string // cover, contain, fill, scale
	Format  string // webp, jpeg, png, gif
	Quality int    // 1-100
}

type StorageEvent struct {
	EventType   string    `json:"eventType"`
	Bucket      string    `json:"bucket"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	Timestamp   time.Time `json:"timestamp"`
}

type UploadOptions struct {
	IsPublic         *bool
	ExpiresInSeconds *int
	Metadata         map[string]string
}

type ShareLink struct {
	ID               string     `json:"id"`
	Bucket           string     `json:"bucket"`
	Path             string     `json:"path"`
	Token            string     `json:"token"`
	RequirePassword  bool       `json:"requirePassword"`
	Password         string     `json:"password,omitempty"`
	MaxDownloads     int        `json:"maxDownloads"`
	DownloadCount    int        `json:"downloadCount"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	DownloadURL      string     `json:"downloadUrl"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type LifecycleRule struct {
	ID             string `json:"id"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	ExpireDays     int    `json:"expireDays"`
	TrashPurgeDays int    `json:"trashPurgeDays"`
	MaxVersions    int    `json:"maxVersions"`
	Enabled        bool   `json:"enabled"`
}

type TUSUploadOptions struct {
	ChunkSize int64
	Metadata  map[string]string
	IsPublic  *bool
}

type AuditLogEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	IPAddress string    `json:"ipAddress"`
	Status    string    `json:"status"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"createdAt"`
}

type GCReport struct {
	DeletedBlobs int64 `json:"deletedBlobs"`
	FreedBytes   int64 `json:"freedBytes"`
}

// MediaInfo parses audio/video characteristics from metadata map.
type MediaInfo struct {
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
	Bitrate         int     `json:"bitrate,omitempty"`
	SampleRate      int     `json:"sampleRate,omitempty"`
	Channels        int     `json:"channels,omitempty"`
	Codec           string  `json:"codec,omitempty"`
}

func (o *StorageObject) MediaInfo() *MediaInfo {
	if o.Metadata == nil {
		return nil
	}
	info := &MediaInfo{}
	hasInfo := false
	if d, ok := o.Metadata["duration_seconds"]; ok {
		if sec, err := strconv.ParseFloat(d, 64); err == nil {
			info.DurationSeconds = sec
			hasInfo = true
		}
	}
	if c, ok := o.Metadata["codec"]; ok {
		info.Codec = c
		hasInfo = true
	}
	if !hasInfo {
		return nil
	}
	return info
}

