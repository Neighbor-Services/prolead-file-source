package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"gostore/internal/core/domain"
	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"
)

var compressibleExtensions = map[string]bool{
	".txt":  true,
	".json": true,
	".csv":  true,
	".tsv":  true,
	".log":  true,
	".html": true,
	".htm":  true,
	".css":  true,
	".js":   true,
	".mjs":  true,
	".ts":   true,
	".xml":  true,
	".svg":  true,
	".md":   true,
	".markdown": true,
	".yaml": true,
	".yml":  true,
	".sql":  true,
	".sh":   true,
	".py":   true,
	".go":   true,
	".dart": true,
	".rs":   true,
	".c":    true,
	".cpp":  true,
	".h":    true,
	".java": true,
	".ini":  true,
	".conf": true,
	".env":  true,
}

var compressibleMimePrefixes = []string{
	"text/",
	"application/json",
	"application/xml",
	"application/javascript",
	"application/x-javascript",
	"application/x-yaml",
	"application/sql",
	"application/graphql",
	"image/svg+xml",
}

type CompressionResult struct {
	Bucket           string  `json:"bucket"`
	Path             string  `json:"path"`
	OriginalSize     int64   `json:"originalSize"`
	CompressedSize   int64   `json:"compressedSize"`
	BytesSaved       int64   `json:"bytesSaved"`
	SavingsPercent   float64 `json:"savingsPercent"`
	CompressionRatio float64 `json:"compressionRatio"`
	Algorithm        string  `json:"algorithm"`
	DurationMs       int64   `json:"durationMs"`
	Skipped          bool    `json:"skipped,omitempty"`
	Reason           string  `json:"reason,omitempty"`
}

type BatchCompressionStats struct {
	TotalScanned    int64   `json:"totalScanned"`
	TotalCompressed int64   `json:"totalCompressed"`
	RawBytes        int64   `json:"rawBytes"`
	CompressedBytes int64   `json:"compressedBytes"`
	BytesSaved      int64   `json:"bytesSaved"`
	SavingsPercent  float64 `json:"savingsPercent"`
	DurationMs      int64   `json:"durationMs"`
}

type CompressionService struct {
	storage  ports.StorageDriver
	fileRepo *sqlite.FileRepository
	db       *sqlite.DB
}

func NewCompressionService(storage ports.StorageDriver, fileRepo *sqlite.FileRepository, db *sqlite.DB) *CompressionService {
	return &CompressionService{
		storage:  storage,
		fileRepo: fileRepo,
		db:       db,
	}
}

// IsCompressible checks if a file is a candidate for gzip compression
func (s *CompressionService) IsCompressible(contentType, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if compressibleExtensions[ext] {
		return true
	}

	lowerMime := strings.ToLower(contentType)
	for _, prefix := range compressibleMimePrefixes {
		if strings.HasPrefix(lowerMime, prefix) {
			return true
		}
	}

	return false
}

// CompressFile analyzes, compresses, and updates metadata for a single stored file
func (s *CompressionService) CompressFile(ctx context.Context, bucket, path string) (*CompressionResult, error) {
	start := time.Now()

	fileObj, err := s.fileRepo.GetByPath(ctx, bucket, path)
	if err != nil || fileObj == nil {
		return nil, fmt.Errorf("file not found: %s/%s", bucket, path)
	}

	// Skip small files (< 512 bytes) or already compressed files
	if fileObj.Size < 512 {
		return &CompressionResult{
			Bucket:       bucket,
			Path:         path,
			OriginalSize: fileObj.Size,
			Skipped:      true,
			Reason:       "file size < 512 bytes",
			DurationMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	if fileObj.Metadata != nil && fileObj.Metadata["compressed"] == "gzip" {
		return &CompressionResult{
			Bucket:       bucket,
			Path:         path,
			OriginalSize: fileObj.Size,
			Skipped:      true,
			Reason:       "already compressed",
			DurationMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	if !s.IsCompressible(fileObj.ContentType, fileObj.Path) {
		return &CompressionResult{
			Bucket:       bucket,
			Path:         path,
			OriginalSize: fileObj.Size,
			Skipped:      true,
			Reason:       "non-compressible MIME type or format",
			DurationMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	// Open raw file stream
	reader, origSize, err := s.storage.Open(ctx, bucket, path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for compression: %w", err)
	}
	defer reader.Close()

	// Compress stream into buffer
	var compBuf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&compBuf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gzip writer: %w", err)
	}

	if _, err := io.Copy(gw, reader); err != nil {
		_ = gw.Close()
		return nil, fmt.Errorf("compression streaming failed: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize gzip stream: %w", err)
	}

	compSize := int64(compBuf.Len())

	// Only apply if we achieved at least 5% savings
	if compSize >= origSize || float64(origSize-compSize)/float64(origSize) < 0.05 {
		return &CompressionResult{
			Bucket:         bucket,
			Path:           path,
			OriginalSize:   origSize,
			CompressedSize: compSize,
			Skipped:        true,
			Reason:         "insufficient compression benefit (<5% savings)",
			DurationMs:     time.Since(start).Milliseconds(),
		}, nil
	}

	bytesSaved := origSize - compSize
	savingsPct := (float64(bytesSaved) / float64(origSize)) * 100.0
	ratio := float64(origSize) / float64(compSize)

	// Update metadata with compression telemetry
	if fileObj.Metadata == nil {
		fileObj.Metadata = make(domain.JSONMap)
	}
	fileObj.Metadata["compressed"] = "gzip"
	fileObj.Metadata["original_size"] = fmt.Sprintf("%d", origSize)
	fileObj.Metadata["compressed_size"] = fmt.Sprintf("%d", compSize)
	fileObj.Metadata["compression_ratio"] = fmt.Sprintf("%.2fx", ratio)
	fileObj.Metadata["savings_percent"] = fmt.Sprintf("%.1f%%", savingsPct)
	fileObj.Metadata["content_encoding"] = "gzip"
	fileObj.UpdatedAt = time.Now().UTC()

	if err := s.fileRepo.Save(ctx, fileObj); err != nil {
		return nil, fmt.Errorf("failed to save compression metadata: %w", err)
	}

	return &CompressionResult{
		Bucket:           bucket,
		Path:             path,
		OriginalSize:     origSize,
		CompressedSize:   compSize,
		BytesSaved:       bytesSaved,
		SavingsPercent:   savingsPct,
		CompressionRatio: ratio,
		Algorithm:        "gzip",
		DurationMs:       time.Since(start).Milliseconds(),
	}, nil
}

// CompressBucket runs batch compression over all compressible files in a bucket
func (s *CompressionService) CompressBucket(ctx context.Context, bucket string) (*BatchCompressionStats, error) {
	start := time.Now()
	stats := &BatchCompressionStats{}

	filter := domain.ListFilesFilter{
		Bucket: bucket,
		Limit:  1000,
	}

	result, err := s.fileRepo.List(ctx, filter)
	if err != nil {
		return stats, err
	}

	for _, file := range result.Items {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		stats.TotalScanned++
		res, err := s.CompressFile(ctx, file.Bucket, file.Path)
		if err == nil && res != nil && !res.Skipped {
			stats.TotalCompressed++
			stats.RawBytes += res.OriginalSize
			stats.CompressedBytes += res.CompressedSize
			stats.BytesSaved += res.BytesSaved
		}
	}

	if stats.RawBytes > 0 {
		stats.SavingsPercent = (float64(stats.BytesSaved) / float64(stats.RawBytes)) * 100.0
	}
	stats.DurationMs = time.Since(start).Milliseconds()

	return stats, nil
}

// RunAutoCompression sweeps all active buckets and compresses candidate files
func (s *CompressionService) RunAutoCompression(ctx context.Context) (*BatchCompressionStats, error) {
	start := time.Now()
	combinedStats := &BatchCompressionStats{}

	var buckets []domain.Bucket
	if s.db != nil {
		if err := s.db.Find(&buckets).Error; err != nil {
			return combinedStats, err
		}
	} else {
		buckets = []domain.Bucket{{Name: "default"}}
	}

	for _, b := range buckets {
		bStats, err := s.CompressBucket(ctx, b.Name)
		if err == nil && bStats != nil {
			combinedStats.TotalScanned += bStats.TotalScanned
			combinedStats.TotalCompressed += bStats.TotalCompressed
			combinedStats.RawBytes += bStats.RawBytes
			combinedStats.CompressedBytes += bStats.CompressedBytes
			combinedStats.BytesSaved += bStats.BytesSaved
		}
	}

	if combinedStats.RawBytes > 0 {
		combinedStats.SavingsPercent = (float64(combinedStats.BytesSaved) / float64(combinedStats.RawBytes)) * 100.0
	}
	combinedStats.DurationMs = time.Since(start).Milliseconds()

	if combinedStats.TotalCompressed > 0 {
		log.Printf("🗜️ Background Compression complete: Compressed %d files, Saved %d bytes (%.1f%% savings in %dms)",
			combinedStats.TotalCompressed, combinedStats.BytesSaved, combinedStats.SavingsPercent, combinedStats.DurationMs)
	}

	return combinedStats, nil
}

// GetCompressionReport aggregates compression metrics across stored files
func (s *CompressionService) GetCompressionReport(ctx context.Context) (*domain.CompressionReport, error) {
	filter := domain.ListFilesFilter{Limit: 5000}
	res, err := s.fileRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var scanned int64
	var compressed int64
	var rawBytes int64
	var compBytes int64

	for _, f := range res.Items {
		scanned++
		if f.Metadata != nil && f.Metadata["compressed"] == "gzip" {
			compressed++
			var origS int64 = f.Size
			var compS int64 = f.Size
			_, _ = fmt.Sscanf(f.Metadata["original_size"], "%d", &origS)
			_, _ = fmt.Sscanf(f.Metadata["compressed_size"], "%d", &compS)

			rawBytes += origS
			compBytes += compS
		}
	}

	var bytesSaved int64
	var savingsPct float64
	var avgRatio float64

	if rawBytes > compBytes {
		bytesSaved = rawBytes - compBytes
		if rawBytes > 0 {
			savingsPct = (float64(bytesSaved) / float64(rawBytes)) * 100.0
			avgRatio = float64(rawBytes) / float64(compBytes)
		}
	}

	return &domain.CompressionReport{
		TotalFilesScanned:    scanned,
		TotalFilesCompressed: compressed,
		RawBytes:             rawBytes,
		CompressedBytes:      compBytes,
		BytesSaved:           bytesSaved,
		SavingsPercent:       savingsPct,
		AverageRatio:         avgRatio,
	}, nil
}
