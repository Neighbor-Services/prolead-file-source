package sqlite

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gostore/internal/core/domain"
)

type LifecycleRepository struct {
	db *DB
}

func NewLifecycleRepository(db *DB) *LifecycleRepository {
	return &LifecycleRepository{db: db}
}

func (r *LifecycleRepository) SaveRule(ctx context.Context, rule *domain.LifecycleRule) error {
	var existing domain.LifecycleRule
	err := r.db.WithContext(ctx).Where("bucket = ?", rule.Bucket).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.WithContext(ctx).Create(rule).Error
		}
		return err
	}
	rule.ID = existing.ID
	return r.db.WithContext(ctx).Save(rule).Error
}

func (r *LifecycleRepository) GetByBucket(ctx context.Context, bucket string) (*domain.LifecycleRule, error) {
	var rule domain.LifecycleRule
	err := r.db.WithContext(ctx).Where("bucket = ?", bucket).First(&rule).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

func (r *LifecycleRepository) ListAllEnabled(ctx context.Context) ([]domain.LifecycleRule, error) {
	var rules []domain.LifecycleRule
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&rules).Error
	return rules, err
}
