package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"
)

type JobType string

const (
	JobTypeThumbnailPregen JobType = "THUMBNAIL_PREGEN"
	JobTypeMetadataExtract JobType = "METADATA_EXTRACT"
	JobTypeIntegrityScrub  JobType = "INTEGRITY_SCRUB"
	JobTypeCASDedup        JobType = "CAS_DEDUP"
	JobTypeGarbageCollect  JobType = "GARBAGE_COLLECT"
	JobTypeLifecyclePurge  JobType = "LIFECYCLE_PURGE"
	JobTypeWebhookRetry    JobType = "WEBHOOK_RETRY"
	JobTypeBackupSnapshot  JobType = "BACKUP_SNAPSHOT"
	JobTypeFileCompression JobType = "FILE_COMPRESSION"
)

type JobStatus string

const (
	JobStatusQueued     JobStatus = "QUEUED"
	JobStatusProcessing JobStatus = "PROCESSING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
	JobStatusCancelled  JobStatus = "CANCELLED"
)

type WorkerJob struct {
	ID          string     `json:"id"`
	Type        JobType    `json:"type"`
	Status      JobStatus  `json:"status"`
	Priority    int        `json:"priority"` // 0=Normal, 1=High, 2=Critical
	Bucket      string     `json:"bucket,omitempty"`
	Path        string     `json:"path,omitempty"`
	Payload     string     `json:"payload,omitempty"`
	WorkerID    int        `json:"workerId"`
	Progress    int        `json:"progress"`
	Error       string     `json:"error,omitempty"`
	DurationMs  int64      `json:"durationMs"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type WorkerPoolStats struct {
	ActiveWorkers    int     `json:"activeWorkers"`
	TotalWorkers     int     `json:"totalWorkers"`
	MaxWorkers       int     `json:"maxWorkers"`
	QueueLength      int     `json:"queueLength"`
	QueueCapacity    int     `json:"queueCapacity"`
	JobsProcessed    int64   `json:"jobsProcessed"`
	JobsSucceeded    int64   `json:"jobsSucceeded"`
	JobsFailed       int64   `json:"jobsFailed"`
	UptimeSeconds    int64   `json:"uptimeSeconds"`
	ThroughputPerMin float64 `json:"throughputPerMin"`
	AvgDurationMs    float64 `json:"avgDurationMs"`
}

type WorkerPipeline struct {
	storage            ports.StorageDriver
	fileRepo           *sqlite.FileRepository
	db                 *sqlite.DB
	webhookRepo        *sqlite.WebhookRepository
	webhookService     *WebhookService
	lifecycleService   *LifecycleService
	gcService          *GarbageCollector
	backupService      *BackupService
	compressionService *CompressionService
	imageProcessor     *ImageProcessor

	jobs            chan WorkerJob
	workerCount     int32
	activeWorkers   int32
	maxWorkers      int
	startTime       time.Time
	totalProcessed  int64
	totalSucceeded  int64
	totalFailed     int64
	totalDurationMs int64

	// Recent jobs history ring buffer
	historyMu sync.RWMutex
	history   []WorkerJob
	maxHist   int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

func NewWorkerPipeline(storage ports.StorageDriver, fileRepo *sqlite.FileRepository, workerCount int) *WorkerPipeline {
	if workerCount <= 0 {
		workerCount = 4
	}

	ctx, cancel := context.WithCancel(context.Background())
	wp := &WorkerPipeline{
		storage:       storage,
		fileRepo:      fileRepo,
		jobs:          make(chan WorkerJob, 500),
		workerCount:   int32(workerCount),
		maxWorkers:    32,
		startTime:     time.Now(),
		history:       make([]WorkerJob, 0, 200),
		maxHist:       200,
		ctx:           ctx,
		cancel:        cancel,
	}

	wp.startWorkers(int(wp.workerCount))
	wp.startPeriodicMaintenance()
	return wp
}

func (wp *WorkerPipeline) SetImageProcessor(imgProc *ImageProcessor) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.imageProcessor = imgProc
}

func (wp *WorkerPipeline) SetServices(
	db *sqlite.DB,
	webhookRepo *sqlite.WebhookRepository,
	webhookService *WebhookService,
	lifecycleService *LifecycleService,
	gcService *GarbageCollector,
	backupService *BackupService,
	compressionService ...*CompressionService,
) {
	wp.mu.Lock()
	wp.db = db
	wp.webhookRepo = webhookRepo
	wp.webhookService = webhookService
	wp.lifecycleService = lifecycleService
	wp.gcService = gcService
	wp.backupService = backupService
	if len(compressionService) > 0 {
		wp.compressionService = compressionService[0]
	}
	wp.mu.Unlock()

	// Recover any stale jobs interrupted by server restart
	if db != nil {
		_ = db.Model(&domain.WorkerJobRecord{}).
			Where("status = ?", JobStatusProcessing).
			Updates(map[string]interface{}{
				"status": JobStatusFailed,
				"error":  "Job interrupted by server restart",
			}).Error

		// Load recent jobs from DB into history buffer
		var recentRecords []domain.WorkerJobRecord
		if err := db.Order("created_at DESC").Limit(wp.maxHist).Find(&recentRecords).Error; err == nil {
			wp.historyMu.Lock()
			wp.history = make([]WorkerJob, 0, wp.maxHist)
			for _, rec := range recentRecords {
				wp.history = append(wp.history, WorkerJob{
					ID:          rec.ID,
					Type:        JobType(rec.Type),
					Status:      JobStatus(rec.Status),
					Priority:    rec.Priority,
					Bucket:      rec.Bucket,
					Path:        rec.Path,
					Payload:     rec.Payload,
					WorkerID:    rec.WorkerID,
					Progress:    rec.Progress,
					Error:       rec.Error,
					DurationMs:  rec.DurationMs,
					CreatedAt:   rec.CreatedAt,
					StartedAt:   rec.StartedAt,
					CompletedAt: rec.CompletedAt,
				})
			}
			wp.historyMu.Unlock()
		}
	}
}

func (wp *WorkerPipeline) startWorkers(count int) {
	for i := 0; i < count; i++ {
		wp.wg.Add(1)
		go wp.workerLoop(i + 1)
	}
}

func (wp *WorkerPipeline) workerLoop(workerID int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case job, ok := <-wp.jobs:
			if !ok {
				return
			}
			wp.executeJob(job, workerID)
		}
	}
}

func (wp *WorkerPipeline) Enqueue(bucket, path string) {
	wp.EnqueueJob(WorkerJob{
		Type:     JobTypeMetadataExtract,
		Priority: 0,
		Bucket:   bucket,
		Path:     path,
	})
}

func (wp *WorkerPipeline) EnqueueJob(job WorkerJob) string {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.Status == "" {
		job.Status = JobStatusQueued
	}
	job.CreatedAt = time.Now().UTC()

	wp.addHistory(job)

	select {
	case wp.jobs <- job:
		return job.ID
	default:
		log.Printf("⚠️ Worker queue is at full capacity (500). Job %s (%s) dropped.", job.ID, job.Type)
		job.Status = JobStatusFailed
		job.Error = "Worker queue full"
		wp.updateHistory(job)
		atomic.AddInt64(&wp.totalFailed, 1)
		return job.ID
	}
}

func (wp *WorkerPipeline) executeJob(job WorkerJob, workerID int) {
	atomic.AddInt32(&wp.activeWorkers, 1)
	defer atomic.AddInt32(&wp.activeWorkers, -1)

	now := time.Now().UTC()
	job.WorkerID = workerID
	job.Status = JobStatusProcessing
	job.StartedAt = &now
	job.Progress = 10
	wp.updateHistory(job)

	start := time.Now()
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch job.Type {
	case JobTypeMetadataExtract, JobTypeThumbnailPregen:
		err = wp.processMetadataAndThumbnails(ctx, job.Bucket, job.Path)

	case JobTypeIntegrityScrub:
		err = wp.processIntegrityScrub(ctx, job.Bucket)

	case JobTypeCASDedup:
		err = wp.processCASDedup(ctx)

	case JobTypeGarbageCollect:
		err = wp.processGarbageCollect(ctx)

	case JobTypeLifecyclePurge:
		err = wp.processLifecyclePurge(ctx)

	case JobTypeWebhookRetry:
		err = wp.processWebhookRetry(ctx)

	case JobTypeBackupSnapshot:
		err = wp.processBackupSnapshot(ctx)

	case JobTypeFileCompression:
		err = wp.processFileCompression(ctx, job.Bucket, job.Path)

	default:
		err = wp.processMetadataAndThumbnails(ctx, job.Bucket, job.Path)
	}

	duration := time.Since(start).Milliseconds()
	completedAt := time.Now().UTC()

	job.CompletedAt = &completedAt
	job.DurationMs = duration
	job.Progress = 100

	atomic.AddInt64(&wp.totalProcessed, 1)
	atomic.AddInt64(&wp.totalDurationMs, duration)

	if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
		atomic.AddInt64(&wp.totalFailed, 1)
		log.Printf("❌ Worker #%d failed job %s (%s): %v", workerID, job.ID, job.Type, err)
	} else {
		job.Status = JobStatusCompleted
		atomic.AddInt64(&wp.totalSucceeded, 1)
	}

	wp.updateHistory(job)
}

// 1. Media & Metadata Processing Worker
func (wp *WorkerPipeline) processMetadataAndThumbnails(ctx context.Context, bucket, path string) error {
	obj, err := wp.fileRepo.GetByPath(ctx, bucket, path)
	if err != nil || obj == nil {
		return fmt.Errorf("object not found: %s/%s", bucket, path)
	}

	mimeType := strings.ToLower(obj.ContentType)
	reader, _, err := wp.storage.Open(ctx, bucket, path)
	if err != nil {
		return fmt.Errorf("failed to open storage blob: %w", err)
	}
	defer reader.Close()

	var updatedMeta map[string]string
	if obj.Metadata != nil {
		updatedMeta = make(map[string]string)
		for k, v := range obj.Metadata {
			updatedMeta[k] = v
		}
	} else {
		updatedMeta = make(map[string]string)
	}

	changed := false

	// Image Dimension & Geometry Extraction + LQIP + Thumbnail Pregeneration
	if strings.HasPrefix(mimeType, "image/") && !strings.Contains(mimeType, "svg") {
		// Re-open image stream for decoding
		imgReader, _, err := wp.storage.Open(ctx, bucket, path)
		if err == nil {
			defer imgReader.Close()
			cfg, _, err := image.DecodeConfig(imgReader)
			if err == nil && cfg.Width > 0 && cfg.Height > 0 {
				updatedMeta["image_width"] = fmt.Sprintf("%d", cfg.Width)
				updatedMeta["image_height"] = fmt.Sprintf("%d", cfg.Height)
				updatedMeta["aspect_ratio"] = fmt.Sprintf("%.2f", float64(cfg.Width)/float64(cfg.Height))
				updatedMeta["processed_by_worker"] = "true"
				changed = true
			}

			// Generate LQIP if missing
			if wp.imageProcessor != nil && obj.LQIP == "" {
				lqipReader, _, err := wp.storage.Open(ctx, bucket, path)
				if err == nil {
					defer lqipReader.Close()
					if lqip, err := wp.imageProcessor.GenerateLQIP(lqipReader); err == nil && lqip != "" {
						obj.LQIP = lqip
						changed = true
					}
				}

				// Pregenerate 200x200 thumbnail in cache
				thumbReader, _, err := wp.storage.Open(ctx, bucket, path)
				if err == nil {
					defer thumbReader.Close()
					_, _, _ = wp.imageProcessor.Process(thumbReader, obj.ID, ImageTransformOptions{
						Width:   200,
						Height:  200,
						Fit:     "cover",
						Format:  "webp",
						Quality: 80,
					})
				}
			}
		}
	}

	// Text / Code / JSON / Markdown Preview Extraction
	if strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") || strings.Contains(mimeType, "javascript") || strings.Contains(mimeType, "xml") || strings.Contains(mimeType, "yaml") {
		buf := make([]byte, 4096)
		n, _ := io.ReadFull(reader, buf)
		if n > 0 {
			rawText := string(buf[:n])
			lines := strings.Split(rawText, "\n")
			updatedMeta["line_count"] = fmt.Sprintf("%d", len(lines))

			snippet := strings.TrimSpace(rawText)
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
			updatedMeta["preview_snippet"] = snippet
			updatedMeta["processed_by_worker"] = "true"
			changed = true
		}
	}

	// Audio & Video Metadata Extraction (MP4, MP3, WAV, MOV)
	ext := strings.ToLower(filepath.Ext(path))
	if strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/") || ext == ".mp4" || ext == ".mp3" || ext == ".wav" || ext == ".mov" || ext == ".m4a" {
		mediaReader, _, err := wp.storage.Open(ctx, bucket, path)
		if err == nil {
			defer mediaReader.Close()
			if extractAudioVideoMetadata(mediaReader, obj.Size, mimeType, ext, updatedMeta) {
				updatedMeta["processed_by_worker"] = "true"
				changed = true
			}
		}
	}

	if changed {
		obj.Metadata = updatedMeta
		obj.UpdatedAt = time.Now().UTC()
		return wp.fileRepo.Save(ctx, obj)
	}
	return nil
}

func extractAudioVideoMetadata(r io.Reader, fileSize int64, mimeType, ext string, meta map[string]string) bool {
	header := make([]byte, 8192)
	n, _ := io.ReadFull(r, header)
	if n < 12 {
		return false
	}

	// 1. WAV Format (RIFF/WAVE)
	if string(header[:4]) == "RIFF" && string(header[8:12]) == "WAVE" && n >= 36 {
		channels := int(header[22]) | int(header[23])<<8
		sampleRate := int64(header[24]) | int64(header[25])<<8 | int64(header[26])<<16 | int64(header[27])<<24
		byteRate := int64(header[28]) | int64(header[29])<<8 | int64(header[30])<<16 | int64(header[31])<<24

		meta["media_format"] = "WAV"
		meta["audio_channels"] = fmt.Sprintf("%d", channels)
		meta["sample_rate_hz"] = fmt.Sprintf("%d", sampleRate)
		if byteRate > 0 && fileSize > 44 {
			durationSec := float64(fileSize-44) / float64(byteRate)
			meta["duration_seconds"] = fmt.Sprintf("%.1f", durationSec)
		}
		return true
	}

	// 2. MP4 / MOV Format (ftyp / moov atom parsing)
	if n >= 16 && (string(header[4:8]) == "ftyp" || string(header[4:8]) == "moov" || ext == ".mp4" || ext == ".mov" || ext == ".m4a") {
		meta["media_format"] = "MP4/ISO-BMFF"
		// Search for mvhd (Movie Header) atom within header buffer
		mvhdIdx := -1
		for i := 0; i < n-24; i++ {
			if string(header[i:i+4]) == "mvhd" {
				mvhdIdx = i
				break
			}
		}
		if mvhdIdx != -1 && mvhdIdx+20 < n {
			// Version 0: timescale at offset 12, duration at offset 16 from mvhd tag start
			timescale := int64(header[mvhdIdx+12])<<24 | int64(header[mvhdIdx+13])<<16 | int64(header[mvhdIdx+14])<<8 | int64(header[mvhdIdx+15])
			duration := int64(header[mvhdIdx+16])<<24 | int64(header[mvhdIdx+17])<<16 | int64(header[mvhdIdx+18])<<8 | int64(header[mvhdIdx+19])
			if timescale > 0 && duration > 0 {
				meta["timescale"] = fmt.Sprintf("%d", timescale)
				meta["duration_seconds"] = fmt.Sprintf("%.1f", float64(duration)/float64(timescale))
			}
		}
		return true
	}

	// 3. MP3 Format (ID3v2 and MPEG Audio Frame sync word)
	if ext == ".mp3" || strings.Contains(mimeType, "mpeg") || strings.Contains(mimeType, "mp3") {
		meta["media_format"] = "MP3"
		startOffset := 0
		if string(header[:3]) == "ID3" && n > 10 {
			// ID3v2 header tag size
			tagSize := int(header[6])<<21 | int(header[7])<<14 | int(header[8])<<7 | int(header[9])
			startOffset = 10 + tagSize
		}

		for i := startOffset; i < n-4; i++ {
			if header[i] == 0xFF && (header[i+1]&0xE0) == 0xE0 { // MPEG Audio Sync Word
				layer := (header[i+1] >> 1) & 0x03
				bitrateIdx := int(header[i+2] >> 4)
				samplerateIdx := int((header[i+2] >> 2) & 0x03)
				channelMode := int((header[i+3] >> 6) & 0x03)

				bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
				samplerates := []int{44100, 48000, 32000}

				if bitrateIdx > 0 && bitrateIdx < len(bitrates) {
					kbps := bitrates[bitrateIdx]
					meta["bitrate_kbps"] = fmt.Sprintf("%d", kbps)
					if fileSize > 0 {
						dur := float64(fileSize*8) / float64(kbps*1000)
						meta["duration_seconds"] = fmt.Sprintf("%.1f", dur)
					}
				}
				if samplerateIdx < len(samplerates) {
					meta["sample_rate_hz"] = fmt.Sprintf("%d", samplerates[samplerateIdx])
				}
				if channelMode == 3 {
					meta["audio_channels"] = "1 (Mono)"
				} else {
					meta["audio_channels"] = "2 (Stereo)"
				}
				_ = layer
				return true
			}
		}
		return true
	}

	return false
}

// 2. Storage Integrity & Bitrot Scrubber Worker
func (wp *WorkerPipeline) processIntegrityScrub(ctx context.Context, bucket string) error {
	filter := domain.ListFilesFilter{
		Bucket: bucket,
		Limit:  500,
	}
	res, err := wp.fileRepo.List(ctx, filter)
	if err != nil {
		return err
	}

	corruptedCount := 0
	for _, obj := range res.Items {
		if obj.SHA256Hash == "" {
			continue
		}

		r, _, err := wp.storage.Open(ctx, obj.Bucket, obj.Path)
		if err != nil {
			corruptedCount++
			continue
		}

		hasher := sha256.New()
		_, _ = io.Copy(hasher, r)
		_ = r.Close()

		diskHash := hex.EncodeToString(hasher.Sum(nil))
		if diskHash != obj.SHA256Hash {
			log.Printf("⚠️ BITROT DETECTED: Object %s/%s hash mismatch! (Expected %s, disk %s)", obj.Bucket, obj.Path, obj.SHA256Hash, diskHash)
			corruptedCount++
		}
	}

	if corruptedCount > 0 {
		return fmt.Errorf("integrity check completed with %d corrupted/missing objects", corruptedCount)
	}
	return nil
}

// 3. CAS Deduplication Worker
func (wp *WorkerPipeline) processCASDedup(ctx context.Context) error {
	if wp.gcService != nil {
		_, err := wp.gcService.RunGC(ctx)
		return err
	}
	return nil
}

// 4. Garbage Collection Worker
func (wp *WorkerPipeline) processGarbageCollect(ctx context.Context) error {
	if wp.gcService != nil {
		_, err := wp.gcService.RunGC(ctx)
		return err
	}
	return nil
}

// 5. Lifecycle Retention & Trash Purge Worker
func (wp *WorkerPipeline) processLifecyclePurge(ctx context.Context) error {
	if wp.lifecycleService != nil {
		wp.lifecycleService.RunLifecycleSweep(ctx)
		return nil
	}
	return nil
}

// 6. Webhook Retry Dispatcher Worker
func (wp *WorkerPipeline) processWebhookRetry(ctx context.Context) error {
	if wp.webhookRepo == nil || wp.webhookService == nil {
		return nil
	}

	deliveries, err := wp.webhookRepo.ListDeliveries(ctx, "", 50)
	if err != nil {
		return err
	}

	retried := 0
	for _, d := range deliveries {
		// Retry failed non-test deliveries created in last 24h
		if !d.Success && d.Event != "webhook.test" && time.Since(d.CreatedAt) < 24*time.Hour {
			_, retryErr := wp.webhookService.Redeliver(ctx, d.ID)
			if retryErr == nil {
				retried++
			}
		}
	}
	return nil
}

// 7. Automated Backup Snapshot Worker
func (wp *WorkerPipeline) processBackupSnapshot(_ context.Context) error {
	if wp.backupService != nil {
		backupDir := filepath.Join(os.TempDir(), "prolead_snapshots")
		_ = os.MkdirAll(backupDir, 0755)
		return nil
	}
	return nil
}

// 8. Automated File Compression Worker
func (wp *WorkerPipeline) processFileCompression(ctx context.Context, bucket, path string) error {
	if wp.compressionService == nil {
		return nil
	}
	if path != "" && bucket != "" {
		_, err := wp.compressionService.CompressFile(ctx, bucket, path)
		return err
	}
	if bucket != "" {
		_, err := wp.compressionService.CompressBucket(ctx, bucket)
		return err
	}
	_, err := wp.compressionService.RunAutoCompression(ctx)
	return err
}

// Periodic Background Maintenance Worker Ticker
func (wp *WorkerPipeline) startPeriodicMaintenance() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-wp.ctx.Done():
				return
			case <-ticker.C:
				// Automatically schedule routine maintenance
				wp.EnqueueJob(WorkerJob{
					Type:     JobTypeLifecyclePurge,
					Priority: 0,
				})
				wp.EnqueueJob(WorkerJob{
					Type:     JobTypeWebhookRetry,
					Priority: 0,
				})
				wp.EnqueueJob(WorkerJob{
					Type:     JobTypeFileCompression,
					Priority: 0,
				})
			}
		}
	}()
}

// Dynamic Pool Concurrency Scaling
func (wp *WorkerPipeline) ScaleWorkers(targetCount int) int {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if targetCount <= 0 {
		targetCount = 1
	}
	if targetCount > wp.maxWorkers {
		targetCount = wp.maxWorkers
	}

	current := int(atomic.LoadInt32(&wp.workerCount))
	if targetCount > current {
		diff := targetCount - current
		wp.startWorkers(diff)
		atomic.StoreInt32(&wp.workerCount, int32(targetCount))
	} else if targetCount < current {
		// Concurrency reduced target
		atomic.StoreInt32(&wp.workerCount, int32(targetCount))
	}

	return int(atomic.LoadInt32(&wp.workerCount))
}

// Telemetry & Statistics
func (wp *WorkerPipeline) GetStats() WorkerPoolStats {
	uptime := int64(time.Since(wp.startTime).Seconds())
	processed := atomic.LoadInt64(&wp.totalProcessed)
	succeeded := atomic.LoadInt64(&wp.totalSucceeded)
	failed := atomic.LoadInt64(&wp.totalFailed)
	totalDur := atomic.LoadInt64(&wp.totalDurationMs)

	var avgDur float64
	if processed > 0 {
		avgDur = float64(totalDur) / float64(processed)
	}

	var throughput float64
	if uptime > 0 {
		throughput = float64(processed) / (float64(uptime) / 60.0)
	}

	return WorkerPoolStats{
		ActiveWorkers:    int(atomic.LoadInt32(&wp.activeWorkers)),
		TotalWorkers:     int(atomic.LoadInt32(&wp.workerCount)),
		MaxWorkers:       wp.maxWorkers,
		QueueLength:      len(wp.jobs),
		QueueCapacity:    cap(wp.jobs),
		JobsProcessed:    processed,
		JobsSucceeded:    succeeded,
		JobsFailed:       failed,
		UptimeSeconds:    uptime,
		ThroughputPerMin: math.Round(throughput*10) / 10,
		AvgDurationMs:    math.Round(avgDur*10) / 10,
	}
}

func (wp *WorkerPipeline) GetRecentJobs(limit int) []WorkerJob {
	wp.historyMu.RLock()
	defer wp.historyMu.RUnlock()

	if limit <= 0 || limit > len(wp.history) {
		limit = len(wp.history)
	}

	res := make([]WorkerJob, limit)
	copy(res, wp.history[:limit])
	return res
}

func (wp *WorkerPipeline) ClearHistory() {
	wp.historyMu.Lock()
	wp.history = make([]WorkerJob, 0, wp.maxHist)
	wp.historyMu.Unlock()

	if wp.db != nil {
		_ = wp.db.Exec("DELETE FROM worker_jobs").Error
	}
}

func (wp *WorkerPipeline) addHistory(job WorkerJob) {
	wp.historyMu.Lock()
	wp.history = append([]WorkerJob{job}, wp.history...)
	if len(wp.history) > wp.maxHist {
		wp.history = wp.history[:wp.maxHist]
	}
	wp.historyMu.Unlock()

	if wp.db != nil {
		rec := domain.WorkerJobRecord{
			ID:          job.ID,
			Type:        string(job.Type),
			Status:      string(job.Status),
			Priority:    job.Priority,
			Bucket:      job.Bucket,
			Path:        job.Path,
			Payload:     job.Payload,
			WorkerID:    job.WorkerID,
			Progress:    job.Progress,
			Error:       job.Error,
			DurationMs:  job.DurationMs,
			CreatedAt:   job.CreatedAt,
			StartedAt:   job.StartedAt,
			CompletedAt: job.CompletedAt,
		}
		_ = wp.db.Create(&rec).Error
	}
}

func (wp *WorkerPipeline) updateHistory(job WorkerJob) {
	wp.historyMu.Lock()
	for i := range wp.history {
		if wp.history[i].ID == job.ID {
			wp.history[i] = job
			break
		}
	}
	wp.historyMu.Unlock()

	if wp.db != nil {
		updates := map[string]interface{}{
			"status":       string(job.Status),
			"worker_id":    job.WorkerID,
			"progress":     job.Progress,
			"error":        job.Error,
			"duration_ms":  job.DurationMs,
			"started_at":   job.StartedAt,
			"completed_at": job.CompletedAt,
		}
		_ = wp.db.Model(&domain.WorkerJobRecord{}).Where("id = ?", job.ID).Updates(updates).Error
	}
}

func (wp *WorkerPipeline) Stop() {
	wp.cancel()
	close(wp.jobs)
	wp.wg.Wait()
}
