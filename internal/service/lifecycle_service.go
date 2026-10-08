package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type LifecycleService struct {
	db            *sqlite.DB
	lifecycleRepo *sqlite.LifecycleRepository
	fileRepo      *sqlite.FileRepository
	storagePath   string
	stopChan      chan struct{}
	mu            sync.Mutex
	isRunning     bool
}

type ScrubbingReport struct {
	TotalBlobsChecked int64     `json:"totalBlobsChecked"`
	CorruptedBlobs    int64     `json:"corruptedBlobs"`
	TotalBytesScrubbed int64    `json:"totalBytesScrubbed"`
	Errors            []string  `json:"errors,omitempty"`
	StartedAt         time.Time `json:"startedAt"`
	CompletedAt       time.Time `json:"completedAt"`
}

func NewLifecycleService(
	db *sqlite.DB,
	lifecycleRepo *sqlite.LifecycleRepository,
	fileRepo *sqlite.FileRepository,
	storagePath string,
) *LifecycleService {
	return &LifecycleService{
		db:            db,
		lifecycleRepo: lifecycleRepo,
		fileRepo:      fileRepo,
		storagePath:   storagePath,
		stopChan:      make(chan struct{}),
	}
}

func (s *LifecycleService) Start(interval time.Duration) {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				s.RunLifecycleSweep(ctx)
				cancel()
			case <-s.stopChan:
				return
			}
		}
	}()
}

func (s *LifecycleService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		close(s.stopChan)
		s.isRunning = false
	}
}

func (s *LifecycleService) RunLifecycleSweep(ctx context.Context) {
	log.Printf("[Lifecycle] Starting automated retention sweep...")
	now := time.Now().UTC()

	// 1. Purge expired TTL files
	var expiredFiles []domain.FileObject
	if err := s.db.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at < ?", now).Find(&expiredFiles).Error; err == nil {
		for _, f := range expiredFiles {
			_ = s.fileRepo.HardDelete(ctx, f.Bucket, f.Path)
			log.Printf("[Lifecycle] Purged TTL-expired object: %s/%s", f.Bucket, f.Path)
		}
	}

	// 2. Process per-bucket lifecycle rules
	rules, err := s.lifecycleRepo.ListAllEnabled(ctx)
	if err == nil {
		for _, rule := range rules {
			if rule.TrashDays > 0 {
				threshold := now.Add(-time.Duration(rule.TrashDays) * 24 * time.Hour)
				var trashToPurge []domain.FileObject
				tx := s.db.WithContext(ctx).Where("bucket = ? AND deleted_at IS NOT NULL AND deleted_at < ?", rule.Bucket, threshold)
				if rule.Prefix != "" {
					tx = tx.Where("path LIKE ?", rule.Prefix+"%")
				}
				if err := tx.Find(&trashToPurge).Error; err == nil {
					for _, f := range trashToPurge {
						_ = s.fileRepo.HardDelete(ctx, f.Bucket, f.Path)
						log.Printf("[Lifecycle] Auto-purged trash past %d days: %s/%s", rule.TrashDays, f.Bucket, f.Path)
					}
				}
			}

			// Prune historical versions beyond versionLimit
			if rule.VersionLimit > 0 {
				var filePaths []string
				s.db.WithContext(ctx).Model(&domain.FileVersion{}).
					Where("bucket = ?", rule.Bucket).
					Distinct("path").
					Pluck("path", &filePaths)

				for _, p := range filePaths {
					var versions []domain.FileVersion
					s.db.WithContext(ctx).Where("bucket = ? AND path = ?", rule.Bucket, p).
						Order("version DESC").
						Find(&versions)

					if len(versions) > rule.VersionLimit {
						excess := versions[rule.VersionLimit:]
						for _, v := range excess {
							s.db.WithContext(ctx).Delete(&v)
							log.Printf("[Lifecycle] Pruned old version v%d for %s/%s", v.Version, v.Bucket, v.Path)
						}
					}
				}
			}
		}
	}

	// 3. Default fallback: purge trash older than 30 days if no custom rule exists
	defaultTrashThreshold := now.Add(-30 * 24 * time.Hour)
	s.db.WithContext(ctx).Where("deleted_at IS NOT NULL AND deleted_at < ?", defaultTrashThreshold).Delete(&domain.FileObject{})

	log.Printf("[Lifecycle] Automated retention sweep completed successfully.")
}

func (s *LifecycleService) RunScrubber(ctx context.Context) (*ScrubbingReport, error) {
	started := time.Now().UTC()
	report := &ScrubbingReport{
		StartedAt: started,
	}

	blobDir := filepath.Join(s.storagePath, ".blobs")
	err := filepath.Walk(blobDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		expectedHash := info.Name()
		if len(expectedHash) != 64 { // Not a canonical SHA256 file
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			report.Errors = append(report.Errors, "failed to open: "+path)
			return nil
		}
		defer file.Close()

		hasher := sha256.New()
		n, err := io.Copy(hasher, file)
		if err != nil {
			report.Errors = append(report.Errors, "read error: "+path)
			return nil
		}

		report.TotalBlobsChecked++
		report.TotalBytesScrubbed += n

		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != expectedHash {
			report.CorruptedBlobs++
			report.Errors = append(report.Errors, "checksum mismatch: "+expectedHash+" vs "+actualHash)
			log.Printf("[Scrubber ERROR] Bitrot detected in CAS block: %s (computed %s)", expectedHash, actualHash)
		}

		return nil
	})

	report.CompletedAt = time.Now().UTC()
	return report, err
}
