package sqlite

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gostore/internal/core/domain"
)

type ShareRepository struct {
	db *DB
}

func NewShareRepository(db *DB) *ShareRepository {
	return &ShareRepository{db: db}
}

func (r *ShareRepository) Create(ctx context.Context, share *domain.ShareLink) error {
	return r.db.WithContext(ctx).Create(share).Error
}

func (r *ShareRepository) GetByToken(ctx context.Context, token string) (*domain.ShareLink, error) {
	var s domain.ShareLink
	err := r.db.WithContext(ctx).Where("token = ?", token).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *ShareRepository) IncrementDownload(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Model(&domain.ShareLink{}).
		Where("token = ?", token).
		UpdateColumn("download_count", gorm.Expr("download_count + 1")).Error
}

func (r *ShareRepository) Delete(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Where("token = ?", token).Delete(&domain.ShareLink{}).Error
}

func (r *ShareRepository) ListByBucket(ctx context.Context, bucket string) ([]domain.ShareLink, error) {
	var list []domain.ShareLink
	err := r.db.WithContext(ctx).Where("bucket = ?", bucket).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (r *ShareRepository) CleanupExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at < ?", now).Delete(&domain.ShareLink{})
	return res.RowsAffected, res.Error
}
