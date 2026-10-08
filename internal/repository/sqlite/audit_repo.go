package sqlite

import (
	"time"

	"gostore/internal/core/domain"
)

type AuditFilter struct {
	Action string
	Bucket string
	Actor  string
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}

type AuditRepo struct {
	db *DB
}

func NewAuditRepo(db *DB) *AuditRepo {
	return &AuditRepo{db: db}
}

func (r *AuditRepo) Create(log *domain.AuditLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	return r.db.Create(log).Error
}

func (r *AuditRepo) List(filter AuditFilter) ([]domain.AuditLog, int64, error) {
	query := r.db.Model(&domain.AuditLog{})

	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Bucket != "" {
		query = query.Where("bucket = ?", filter.Bucket)
	}
	if filter.Actor != "" {
		query = query.Where("actor = ?", filter.Actor)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", filter.From)
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}

	var logs []domain.AuditLog
	err := query.Order("created_at DESC").
		Limit(limit).
		Offset(filter.Offset).
		Find(&logs).Error

	return logs, total, err
}

func (r *AuditRepo) PurgeOlderThan(age time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-age)
	res := r.db.Where("created_at < ?", cutoff).Delete(&domain.AuditLog{})
	return res.RowsAffected, res.Error
}
