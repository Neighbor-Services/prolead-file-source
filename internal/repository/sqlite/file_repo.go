package sqlite

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gostore/internal/core/domain"
)

type FileRepository struct {
	db *DB
}

func NewFileRepository(db *DB) *FileRepository {
	return &FileRepository{db: db}
}

func (r *FileRepository) Save(ctx context.Context, f *domain.FileObject) error {
	now := time.Now().UTC()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now

	// Check if old version exists to archive to file_versions
	var existing domain.FileObject
	err := r.db.WithContext(ctx).Where("bucket = ? AND path = ?", f.Bucket, f.Path).First(&existing).Error
	if err == nil && existing.ID != "" {
		if existing.SHA256Hash != f.SHA256Hash {
			// Archive previous version
			f.Version = existing.Version + 1
			ver := domain.FileVersion{
				ID:          uuid.New().String(),
				FileID:      existing.ID,
				Version:     existing.Version,
				Bucket:      existing.Bucket,
				Path:        existing.Path,
				Size:        existing.Size,
				ContentType: existing.ContentType,
				MD5Hash:     existing.MD5Hash,
				SHA256Hash:  existing.SHA256Hash,
				CreatedAt:   existing.UpdatedAt,
			}
			_ = r.db.WithContext(ctx).Create(&ver).Error
		}
		if f.ID == "" || f.ID != existing.ID {
			f.ID = existing.ID
		}
	} else if f.ID == "" {
		f.ID = uuid.New().String()
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "size", "content_type", "md5_hash", "sha256_hash",
			"download_token", "is_public", "version", "metadata", "expires_at", "deleted_at", "updated_at",
		}),
	}).Create(f).Error
}

func (r *FileRepository) GetByPath(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	var f domain.FileObject
	err := r.db.WithContext(ctx).Where("bucket = ? AND path = ? AND deleted_at IS NULL", bucket, path).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *FileRepository) GetByPathIncludingTrash(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	var f domain.FileObject
	err := r.db.WithContext(ctx).Where("bucket = ? AND path = ?", bucket, path).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *FileRepository) GetByToken(ctx context.Context, token string) (*domain.FileObject, error) {
	var f domain.FileObject
	err := r.db.WithContext(ctx).Where("download_token = ? AND deleted_at IS NULL", token).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *FileRepository) SoftDelete(ctx context.Context, bucket, path string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&domain.FileObject{}).
		Where("bucket = ? AND path = ? AND deleted_at IS NULL", bucket, path).
		Update("deleted_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("file not found")
	}
	return nil
}

func (r *FileRepository) Restore(ctx context.Context, bucket, path string) (*domain.FileObject, error) {
	res := r.db.WithContext(ctx).Model(&domain.FileObject{}).
		Where("bucket = ? AND path = ? AND deleted_at IS NOT NULL", bucket, path).
		Update("deleted_at", nil)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("file not found in trash")
	}
	return r.GetByPath(ctx, bucket, path)
}

func (r *FileRepository) HardDelete(ctx context.Context, bucket, path string) error {
	res := r.db.WithContext(ctx).Where("bucket = ? AND path = ?", bucket, path).Delete(&domain.FileObject{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("file not found")
	}
	// Also delete versions
	_ = r.db.WithContext(ctx).Where("bucket = ? AND path = ?", bucket, path).Delete(&domain.FileVersion{})
	return nil
}

func (r *FileRepository) UpdateToken(ctx context.Context, bucket, path, newToken string) error {
	res := r.db.WithContext(ctx).Model(&domain.FileObject{}).
		Where("bucket = ? AND path = ? AND deleted_at IS NULL", bucket, path).
		Updates(map[string]interface{}{
			"download_token": newToken,
			"updated_at":     time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("file not found")
	}
	return nil
}

func (r *FileRepository) ListVersions(ctx context.Context, bucket, path string) ([]domain.FileVersion, error) {
	var versions []domain.FileVersion
	err := r.db.WithContext(ctx).Where("bucket = ? AND path = ?", bucket, path).Order("version DESC").Find(&versions).Error
	return versions, err
}

func (r *FileRepository) List(ctx context.Context, filter domain.ListFilesFilter) (*domain.ListFilesResult, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	tx := r.db.WithContext(ctx).Model(&domain.FileObject{})

	if filter.TrashOnly {
		tx = tx.Where("deleted_at IS NOT NULL")
	} else if !filter.IncludeTrash {
		tx = tx.Where("deleted_at IS NULL")
	}

	if filter.Bucket != "" {
		tx = tx.Where("bucket = ?", filter.Bucket)
	}

	if filter.Prefix != "" {
		tx = tx.Where("path LIKE ?", filter.Prefix+"%")
	}

	if filter.Search != "" {
		tx = tx.Where("name LIKE ? OR path LIKE ?", "%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	// Delimiter mode
	if filter.Delimiter != "" {
		var paths []string
		if err := tx.Order("path ASC").Pluck("path", &paths).Error; err != nil {
			return nil, err
		}

		prefixMap := make(map[string]bool)
		var matchedPaths []string

		for _, fullPath := range paths {
			rel := strings.TrimPrefix(fullPath, filter.Prefix)
			if idx := strings.Index(rel, filter.Delimiter); idx != -1 {
				folder := filter.Prefix + rel[:idx+len(filter.Delimiter)]
				prefixMap[folder] = true
			} else {
				matchedPaths = append(matchedPaths, fullPath)
			}
		}

		var items []domain.FileObject
		totalFiles := len(matchedPaths)
		start := filter.Offset
		end := start + limit
		if start < totalFiles {
			if end > totalFiles {
				end = totalFiles
			}
			slicedPaths := matchedPaths[start:end]
			for _, p := range slicedPaths {
				fileObj, err := r.GetByPath(ctx, filter.Bucket, p)
				if err == nil && fileObj != nil {
					items = append(items, *fileObj)
				}
			}
		}

		var prefixes []string
		for p := range prefixMap {
			prefixes = append(prefixes, p)
		}
		var nextToken string
		if int64(end) < int64(totalFiles) {
			nextToken = strconv.Itoa(end)
		}

		return &domain.ListFilesResult{
			Items:         items,
			Prefixes:      prefixes,
			TotalCount:    int64(totalFiles + len(prefixes)),
			NextPageToken: nextToken,
		}, nil
	}

	var totalCount int64
	if err := tx.Count(&totalCount).Error; err != nil {
		return nil, err
	}

	var items []domain.FileObject
	if err := tx.Order("created_at DESC").Limit(limit).Offset(filter.Offset).Find(&items).Error; err != nil {
		return nil, err
	}

	var nextToken string
	if int64(filter.Offset+len(items)) < totalCount {
		nextToken = strconv.Itoa(filter.Offset + len(items))
	}

	return &domain.ListFilesResult{
		Items:         items,
		TotalCount:    totalCount,
		NextPageToken: nextToken,
	}, nil
}

func (r *FileRepository) GetBucketUsage(ctx context.Context, bucket string) (totalCount int64, totalBytes int64, err error) {
	if err := r.db.WithContext(ctx).Model(&domain.FileObject{}).
		Where("bucket = ? AND deleted_at IS NULL", bucket).
		Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	var sumBytes *int64
	row := r.db.WithContext(ctx).Model(&domain.FileObject{}).
		Where("bucket = ? AND deleted_at IS NULL", bucket).
		Select("coalesce(sum(size), 0)").Row()
	if err := row.Scan(&sumBytes); err != nil {
		return 0, 0, err
	}
	if sumBytes != nil {
		totalBytes = *sumBytes
	}
	return totalCount, totalBytes, nil
}

