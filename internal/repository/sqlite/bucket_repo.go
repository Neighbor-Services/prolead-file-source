package sqlite

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gostore/internal/core/domain"
)

type BucketRepository struct {
	db *DB
}

func NewBucketRepository(db *DB) *BucketRepository {
	return &BucketRepository{db: db}
}

func (r *BucketRepository) Create(ctx context.Context, b *domain.Bucket) error {
	if b.ProjectID == "" {
		b.ProjectID = "p-default"
	}
	return r.db.WithContext(ctx).Create(b).Error
}

func (r *BucketRepository) GetByName(ctx context.Context, name string) (*domain.Bucket, error) {
	var b domain.Bucket
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&b).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *BucketRepository) List(ctx context.Context, projectID string) ([]domain.Bucket, error) {
	var buckets []domain.Bucket
	query := r.db.WithContext(ctx)
	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	err := query.Order("created_at ASC, name ASC").Find(&buckets).Error
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

func (r *BucketRepository) Delete(ctx context.Context, name string) error {
	return r.db.WithContext(ctx).Where("name = ?", name).Delete(&domain.Bucket{}).Error
}

func (r *BucketRepository) Update(ctx context.Context, b *domain.Bucket) error {
	return r.db.WithContext(ctx).Model(&domain.Bucket{}).
		Where("name = ?", b.Name).
		Updates(map[string]interface{}{
			"description":   b.Description,
			"is_public":     b.IsPublic,
			"max_file_size": b.MaxFileSize,
			"allowed_mimes": b.AllowedMimes,
			"updated_at":    b.UpdatedAt,
		}).Error
}

func (r *BucketRepository) GetStats(ctx context.Context) (*domain.StorageStats, error) {
	var stats domain.StorageStats

	if err := r.db.WithContext(ctx).Model(&domain.Bucket{}).Count(&stats.TotalBuckets).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Model(&domain.FileObject{}).Count(&stats.TotalFiles).Error; err != nil {
		return nil, err
	}

	var totalBytes *int64
	row := r.db.WithContext(ctx).Model(&domain.FileObject{}).Select("coalesce(sum(size), 0)").Row()
	if err := row.Scan(&totalBytes); err != nil {
		return nil, err
	}
	if totalBytes != nil {
		stats.TotalBytes = *totalBytes
	}

	return &stats, nil
}
