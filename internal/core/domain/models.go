package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringList) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return errors.New("failed to scan StringList: invalid type")
		}
		bytes = []byte(str)
	}
	if len(bytes) == 0 {
		*s = []string{}
		return nil
	}
	return json.Unmarshal(bytes, s)
}

type JSONMap map[string]string

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return json.Marshal(m)
}

func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]string)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		str, ok := value.(string)
		if !ok {
			return errors.New("failed to scan JSONMap: invalid type")
		}
		bytes = []byte(str)
	}
	if len(bytes) == 0 {
		*m = make(map[string]string)
		return nil
	}
	return json.Unmarshal(bytes, m)
}

// Project represents a multi-tenant project workspace
type Project struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Name        string    `gorm:"not null;size:128" json:"name"`
	Slug        string    `gorm:"uniqueIndex;not null;size:128" json:"slug"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// User represents an administrator / superuser / staff account
type User struct {
	ID               string     `gorm:"primaryKey;size:64" json:"id"`
	Username         string     `gorm:"uniqueIndex;not null;size:64" json:"username"`
	Email            string     `gorm:"uniqueIndex;size:128" json:"email,omitempty"`
	PasswordHash     string     `gorm:"not null;size:255" json:"-"`
	Role             string     `gorm:"default:'admin';size:32" json:"role"` // superadmin, admin, operator, viewer
	IsSuperuser      bool       `gorm:"default:true" json:"isSuperuser"`
	TwoFactorEnabled bool       `gorm:"default:false" json:"twoFactorEnabled"`
	TwoFactorSecret  string     `gorm:"size:128" json:"-"`
	Status           string     `gorm:"default:'active';size:32" json:"status"` // active, suspended
	LastLoginAt      *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt        time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

// Bucket represents a logical storage container with Quotas and SSE encryption
type Bucket struct {
	ID            string     `gorm:"primaryKey;size:64" json:"id"`
	ProjectID     string     `gorm:"index;default:'default';size:64" json:"projectId"`
	Name          string     `gorm:"uniqueIndex;not null;size:128" json:"name"`
	Description   string     `gorm:"type:text" json:"description,omitempty"`
	IsPublic      bool       `gorm:"default:false" json:"isPublic"`
	MaxFileSize   int64      `gorm:"default:0" json:"maxFileSize,omitempty"`
	MaxTotalBytes int64      `gorm:"default:0" json:"maxTotalBytes,omitempty"` // Quota limit in bytes (0 = unlimited)
	MaxFileCount  int64      `gorm:"default:0" json:"maxFileCount,omitempty"`  // Quota file count (0 = unlimited)
	EncryptAtRest bool       `gorm:"default:false" json:"encryptAtRest"`       // AES-256-GCM encryption
	AllowedMimes  StringList `gorm:"type:text" json:"allowedMimes,omitempty"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

// FileObject represents a stored file with Versioning, LQIP, Self-Destruct, and TTL
type FileObject struct {
	ID            string     `gorm:"primaryKey;size:255" json:"id"`
	Bucket        string     `gorm:"index:idx_bucket_path,priority:1;not null;size:128" json:"bucket"`
	Path          string     `gorm:"index:idx_bucket_path,priority:2;not null;size:512" json:"path"`
	Name          string     `gorm:"not null;size:255" json:"name"`
	Size          int64      `gorm:"not null" json:"size"`
	ContentType   string     `gorm:"not null;size:128" json:"contentType"`
	MD5Hash       string     `gorm:"size:64" json:"md5Hash,omitempty"`
	SHA256Hash    string     `gorm:"index;size:64" json:"sha256Hash,omitempty"`
	DownloadToken string     `gorm:"index;not null;size:64" json:"downloadToken,omitempty"`
	DownloadURL   string     `gorm:"-" json:"downloadUrl"`
	IsPublic      bool       `gorm:"default:false" json:"isPublic"`
	Version       int        `gorm:"default:1" json:"version"`
	LQIP          string     `gorm:"type:text" json:"lqip,omitempty"`          // Low-Quality Image Placeholder (Base64)
	DownloadCount int        `gorm:"default:0" json:"downloadCount"`          // Total downloads
	MaxDownloads  int        `gorm:"default:0" json:"maxDownloads,omitempty"` // Self-destruct if reached (0 = unlimited)
	Encrypted     bool       `gorm:"default:false" json:"encrypted"`          // Server-side encrypted flag
	Metadata      JSONMap    `gorm:"type:text" json:"metadata,omitempty"`
	ExpiresAt     *time.Time `gorm:"index" json:"expiresAt,omitempty"`        // TTL
	DeletedAt     *time.Time `gorm:"index" json:"deletedAt,omitempty"`        // Soft delete
	CreatedAt     time.Time  `gorm:"autoCreateTime;index" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

// FileVersion represents an immutable historical revision
type FileVersion struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	FileID      string    `gorm:"index;not null;size:255" json:"fileId"`
	Version     int       `gorm:"not null" json:"version"`
	Bucket      string    `gorm:"not null;size:128" json:"bucket"`
	Path        string    `gorm:"not null;size:512" json:"path"`
	Size        int64     `gorm:"not null" json:"size"`
	ContentType string    `gorm:"not null;size:128" json:"contentType"`
	MD5Hash     string    `gorm:"size:64" json:"md5Hash,omitempty"`
	SHA256Hash  string    `gorm:"size:64" json:"sha256Hash,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

// AuditLog tracks access and modifications for security & compliance
type AuditLog struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	Action     string    `gorm:"index;not null;size:64" json:"action"` // "UPLOAD", "DOWNLOAD", "DELETE", "RESTORE", "ROTATE"
	Bucket     string    `gorm:"index;size:128" json:"bucket"`
	Path       string    `gorm:"size:512" json:"path,omitempty"`
	Actor      string    `gorm:"size:128" json:"actor"`                // API key name or "public" / "anonymous"
	IPAddress  string    `gorm:"size:64" json:"ipAddress"`
	UserAgent  string    `gorm:"size:256" json:"userAgent"`
	Status     int       `gorm:"not null" json:"status"`
	DurationMs int64     `json:"durationMs"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"createdAt"`
}

// Webhook represents an outgoing HTTP notification listener
type Webhook struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	ProjectID      string     `gorm:"index;default:'default';size:64" json:"projectId"`
	Name           string     `gorm:"size:128" json:"name"`
	URL            string     `gorm:"not null;size:512" json:"url"`
	Secret         string     `gorm:"size:128" json:"secret,omitempty"`
	Events         StringList `gorm:"type:text" json:"events"`
	Enabled        bool       `gorm:"default:true" json:"enabled"`
	LastDeliveryAt *time.Time `json:"lastDeliveryAt,omitempty"`
	LastStatusCode int        `gorm:"default:0" json:"lastStatusCode"`
	SuccessCount   int64      `gorm:"default:0" json:"successCount"`
	FailureCount   int64      `gorm:"default:0" json:"failureCount"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

// WebhookDelivery tracks historic delivery attempts, status, response latency, and payload
type WebhookDelivery struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	WebhookID    string    `gorm:"index;not null;size:64" json:"webhookId"`
	ProjectID    string    `gorm:"index;size:64" json:"projectId"`
	Event        string    `gorm:"size:64" json:"event"`
	URL          string    `gorm:"size:512" json:"url"`
	StatusCode   int       `gorm:"not null" json:"statusCode"`
	DurationMs   int64     `json:"durationMs"`
	Success      bool      `gorm:"index" json:"success"`
	Error        string    `gorm:"type:text" json:"error,omitempty"`
	RequestBody  string    `gorm:"type:text" json:"requestBody,omitempty"`
	ResponseBody string    `gorm:"type:text" json:"responseBody,omitempty"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index" json:"createdAt"`
}

// APIKey represents an access key for client applications
type APIKey struct {
	ID                 string     `gorm:"primaryKey;size:64" json:"id"`
	ProjectID          string     `gorm:"index;default:'default';size:64" json:"projectId"`
	Key                string     `gorm:"uniqueIndex;not null;size:128" json:"key"`
	KeyPrefix          string     `gorm:"size:32" json:"keyPrefix"`
	Name               string     `gorm:"not null;size:128" json:"name"`
	Role               string     `gorm:"not null;default:'read-write';size:32" json:"role"`
	Permissions        StringList `gorm:"type:text" json:"permissions,omitempty"`
	AllowedBuckets     StringList `gorm:"type:text" json:"allowedBuckets,omitempty"`
	AllowedOrigins     StringList `gorm:"type:text" json:"allowedOrigins,omitempty"`
	RateLimitReqPerMin int        `gorm:"default:0" json:"rateLimitReqPerMin"` // 0 = unlimited
	RequestCount       int64      `gorm:"default:0" json:"requestCount"`
	LastUsedAt         *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt          *time.Time `gorm:"index" json:"expiresAt,omitempty"`
	Revoked            bool       `gorm:"default:false" json:"revoked"`
	CreatedBy          string     `gorm:"size:128" json:"createdBy,omitempty"`
	CreatedAt          time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime" json:"updatedAt"`
}

// ShareLink represents a password-protected, time-limited, download-metered public share link
type ShareLink struct {
	ID            string     `gorm:"primaryKey;size:64" json:"id"`
	Token         string     `gorm:"uniqueIndex;not null;size:64" json:"token"`
	Bucket        string     `gorm:"not null;size:128" json:"bucket"`
	Path          string     `gorm:"not null;size:512" json:"path"`
	PasswordHash  string     `gorm:"size:255" json:"-"`
	HasPassword   bool       `gorm:"default:false" json:"hasPassword"`
	MaxDownloads  int        `gorm:"default:0" json:"maxDownloads"` // 0 = unlimited
	DownloadCount int        `gorm:"default:0" json:"downloadCount"`
	ExpiresAt     *time.Time `gorm:"index" json:"expiresAt,omitempty"`
	CreatedBy     string     `gorm:"size:128" json:"createdBy,omitempty"`
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"createdAt"`
}

// LifecycleRule represents automated retention and cleanup policies for buckets
type LifecycleRule struct {
	ID             string    `gorm:"primaryKey;size:64" json:"id"`
	Bucket         string    `gorm:"index;not null;size:128" json:"bucket"`
	Prefix         string    `gorm:"size:255" json:"prefix,omitempty"`
	TrashDays      int       `gorm:"default:30" json:"trashDays"`     // Purge soft-deleted files after X days (0 = disabled)
	VersionLimit   int       `gorm:"default:5" json:"versionLimit"`   // Keep maximum X historical versions (0 = unlimited)
	ExpirationDays int       `gorm:"default:0" json:"expirationDays"` // Auto-delete objects after X days (0 = disabled)
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// WorkerJobRecord represents a persistent background task record
type WorkerJobRecord struct {
	ID          string     `gorm:"primaryKey;size:64" json:"id"`
	Type        string     `gorm:"index;size:64" json:"type"`
	Status      string     `gorm:"index;size:32" json:"status"`
	Priority    int        `gorm:"default:0" json:"priority"`
	Bucket      string     `gorm:"size:128" json:"bucket,omitempty"`
	Path        string     `gorm:"size:512" json:"path,omitempty"`
	Payload     string     `gorm:"type:text" json:"payload,omitempty"`
	WorkerID    int        `gorm:"default:0" json:"workerId"`
	Progress    int        `gorm:"default:0" json:"progress"`
	Error       string     `gorm:"type:text" json:"error,omitempty"`
	DurationMs  int64      `gorm:"default:0" json:"durationMs"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

func (Project) TableName() string         { return "projects" }
func (User) TableName() string            { return "users" }
func (Bucket) TableName() string          { return "buckets" }
func (FileObject) TableName() string      { return "files" }
func (FileVersion) TableName() string     { return "file_versions" }
func (AuditLog) TableName() string        { return "audit_logs" }
func (Webhook) TableName() string         { return "webhooks" }
func (WebhookDelivery) TableName() string { return "webhook_deliveries" }
func (APIKey) TableName() string          { return "api_keys" }
func (ShareLink) TableName() string       { return "share_links" }
func (LifecycleRule) TableName() string   { return "lifecycle_rules" }
func (WorkerJobRecord) TableName() string { return "worker_jobs" }

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

func (b *Bucket) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

func (f *FileObject) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return nil
}

func (v *FileVersion) BeforeCreate(tx *gorm.DB) error {
	if v.ID == "" {
		v.ID = uuid.New().String()
	}
	return nil
}

func (a *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

func (d *WebhookDelivery) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return nil
}

func (k *APIKey) BeforeCreate(tx *gorm.DB) error {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	return nil
}

func (s *ShareLink) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

func (l *LifecycleRule) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ListFilesFilter represents query parameters for listing objects
type ListFilesFilter struct {
	Bucket       string
	Prefix       string
	Delimiter    string
	Limit        int
	Offset       int
	Search       string
	IncludeTrash bool
	TrashOnly    bool
}

// ListFilesResult represents the response for a listing query
type ListFilesResult struct {
	Items         []FileObject `json:"items"`
	Prefixes      []string     `json:"prefixes,omitempty"`
	TotalCount    int64        `json:"totalCount"`
	NextPageToken string       `json:"nextPageToken,omitempty"`
}

type StorageStats struct {
	TotalFiles   int64 `json:"totalFiles"`
	TotalBytes   int64 `json:"totalBytes"`
	TotalBuckets int64 `json:"totalBuckets"`
	TotalTrash   int64 `json:"totalTrash"`
}

type DedupReport struct {
	TotalVirtualFiles int64   `json:"totalVirtualFiles"`
	UniqueBlobs       int64   `json:"uniqueBlobs"`
	VirtualBytes      int64   `json:"virtualBytes"`
	PhysicalBytes     int64   `json:"physicalBytes"`
	BytesSaved        int64   `json:"bytesSaved"`
	SavingsPercent    float64 `json:"savingsPercent"`
}

type CompressionReport struct {
	TotalFilesScanned    int64   `json:"totalFilesScanned"`
	TotalFilesCompressed int64   `json:"totalFilesCompressed"`
	RawBytes             int64   `json:"rawBytes"`
	CompressedBytes      int64   `json:"compressedBytes"`
	BytesSaved           int64   `json:"bytesSaved"`
	SavingsPercent       float64 `json:"savingsPercent"`
	AverageRatio         float64 `json:"averageRatio"`
}
