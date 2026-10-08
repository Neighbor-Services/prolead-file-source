package service

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

type GCStats struct {
	BlobsScanned     int   `json:"blobsScanned"`
	OrphanedBlobs    int   `json:"orphanedBlobs"`
	BytesReclaimed   int64 `json:"bytesReclaimed"`
	TempFilesCleaned int   `json:"tempFilesCleaned"`
	DurationMs       int64 `json:"durationMs"`
}

type GarbageCollector struct {
	storageBasePath string
	db              *sqlite.DB
}

func NewGarbageCollector(storageBasePath string, db *sqlite.DB) *GarbageCollector {
	return &GarbageCollector{
		storageBasePath: storageBasePath,
		db:              db,
	}
}

// StartBackgroundWorker runs GC periodically
func (gc *GarbageCollector) StartBackgroundWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats, err := gc.RunGC(ctx)
				if err != nil {
					log.Printf("⚠️ Garbage collection warning: %v", err)
				} else if stats.OrphanedBlobs > 0 || stats.TempFilesCleaned > 0 {
					log.Printf("🧹 GC completed: Reclaimed %d bytes, Removed %d orphaned blobs, %d temp files (in %dms)",
						stats.BytesReclaimed, stats.OrphanedBlobs, stats.TempFilesCleaned, stats.DurationMs)
				}
			}
		}
	}()
}

// RunGC performs a full scan and sweep of unreferenced blobs and stale chunks
func (gc *GarbageCollector) RunGC(ctx context.Context) (*GCStats, error) {
	start := time.Now()
	stats := &GCStats{}

	// 1. Get all currently referenced SHA-256 hashes from both active files, soft-deleted files, and historical versions
	var fileHashes []string
	if err := gc.db.Table("files").Pluck("DISTINCT sha256_hash", &fileHashes).Error; err != nil {
		return stats, err
	}

	var versionHashes []string
	if err := gc.db.Table("file_versions").Pluck("DISTINCT sha256_hash", &versionHashes).Error; err == nil {
		fileHashes = append(fileHashes, versionHashes...)
	}

	activeSet := make(map[string]bool, len(fileHashes))
	for _, h := range fileHashes {
		if h != "" {
			activeSet[h] = true
		}
	}

	// 2. Scan .blobs directory
	blobDir := filepath.Join(gc.storageBasePath, ".blobs")
	_ = filepath.WalkDir(blobDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}

		fileName := d.Name()
		// SHA256 hex string has 64 characters
		if len(fileName) == 64 && !strings.HasPrefix(fileName, ".upload-") {
			stats.BlobsScanned++
			if !activeSet[fileName] {
				// Blob is orphaned! Delete it
				if info, err := d.Info(); err == nil {
					stats.BytesReclaimed += info.Size()
				}
				// Ensure write permission before removal if blob was set read-only
				_ = os.Chmod(path, 0600)
				if err := os.Remove(path); err == nil {
					stats.OrphanedBlobs++
				}
			}
		}
		return nil
	})

	// 3. Clean up stale upload chunks older than 24 hours
	chunkDir := filepath.Join(gc.storageBasePath, ".tmp-chunks")
	now := time.Now()
	_ = filepath.WalkDir(chunkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if now.Sub(info.ModTime()) > 24*time.Hour {
				_ = os.Chmod(path, 0600)
				if err := os.Remove(path); err == nil {
					stats.TempFilesCleaned++
				}
			}
		}
		return nil
	})

	stats.DurationMs = time.Since(start).Milliseconds()
	return stats, nil
}

// CalculateDedupReport calculates the space saved by content-addressable deduplication
func (gc *GarbageCollector) CalculateDedupReport() (*domain.DedupReport, error) {
	var totalVirtualFiles int64
	var virtualBytes int64

	// Sum virtual files and sizes from files (active + soft-deleted)
	row := gc.db.Table("files").Select("COUNT(id), COALESCE(SUM(size), 0)").Row()
	if err := row.Scan(&totalVirtualFiles, &virtualBytes); err != nil {
		return nil, err
	}

	// Also sum historical revisions in file_versions
	var versionCount int64
	var versionBytes int64
	rowVersion := gc.db.Table("file_versions").Select("COUNT(id), COALESCE(SUM(size), 0)").Row()
	if err := rowVersion.Scan(&versionCount, &versionBytes); err == nil {
		totalVirtualFiles += versionCount
		virtualBytes += versionBytes
	}

	// Calculate unique physical blobs in .blobs/
	var uniqueBlobs int64
	var physicalBytes int64

	blobDir := filepath.Join(gc.storageBasePath, ".blobs")
	_ = filepath.WalkDir(blobDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if len(name) == 64 && !strings.HasPrefix(name, ".upload-") {
			uniqueBlobs++
			if info, err := d.Info(); err == nil {
				physicalBytes += info.Size()
			}
		}
		return nil
	})

	var bytesSaved int64
	var savingsPercent float64
	if virtualBytes > physicalBytes {
		bytesSaved = virtualBytes - physicalBytes
		if virtualBytes > 0 {
			savingsPercent = (float64(bytesSaved) / float64(virtualBytes)) * 100.0
		}
	}

	return &domain.DedupReport{
		TotalVirtualFiles: totalVirtualFiles,
		UniqueBlobs:       uniqueBlobs,
		VirtualBytes:      virtualBytes,
		PhysicalBytes:     physicalBytes,
		BytesSaved:        bytesSaved,
		SavingsPercent:    savingsPercent,
	}, nil
}

