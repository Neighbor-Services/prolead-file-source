package sqlite

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gostore/internal/core/domain"
)

type APIKeyRepository struct {
	db *DB
}

func NewAPIKeyRepository(db *DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(ctx context.Context, k *domain.APIKey) error {
	if k.ProjectID == "" {
		k.ProjectID = "p-default"
	}
	return r.db.WithContext(ctx).Create(k).Error
}

func (r *APIKeyRepository) GetByID(ctx context.Context, id string) (*domain.APIKey, error) {
	var k domain.APIKey
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepository) GetByKey(ctx context.Context, key string) (*domain.APIKey, error) {
	var k domain.APIKey
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepository) List(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	var keys []domain.APIKey
	query := r.db.WithContext(ctx)
	if projectID != "" {
		query = query.Where("project_id = ? OR project_id = '' OR project_id IS NULL", projectID)
	}
	err := query.Order("created_at DESC").Find(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *APIKeyRepository) Update(ctx context.Context, k *domain.APIKey) error {
	return r.db.WithContext(ctx).Save(k).Error
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&domain.APIKey{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"revoked":    true,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *APIKeyRepository) ToggleRevoke(ctx context.Context, id string, revoked bool) error {
	return r.db.WithContext(ctx).Model(&domain.APIKey{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"revoked":    revoked,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *APIKeyRepository) RecordUsage(ctx context.Context, id string) {
	now := time.Now().UTC()
	_ = r.db.WithContext(ctx).Model(&domain.APIKey{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"request_count": gorm.Expr("request_count + 1"),
			"last_used_at":  now,
		}).Error
}

func (r *APIKeyRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.APIKey{}).Error
}
