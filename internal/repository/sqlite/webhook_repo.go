package sqlite

import (
	"context"
	"time"

	"gostore/internal/core/domain"
)

type WebhookRepository struct {
	db *DB
}

func NewWebhookRepository(db *DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) Create(ctx context.Context, w *domain.Webhook) error {
	if w.ProjectID == "" {
		w.ProjectID = "p-default"
	}
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *WebhookRepository) GetByID(ctx context.Context, id string) (*domain.Webhook, error) {
	var wh domain.Webhook
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&wh).Error
	if err != nil {
		return nil, err
	}
	return &wh, nil
}

func (r *WebhookRepository) List(ctx context.Context, projectID string) ([]domain.Webhook, error) {
	var webhooks []domain.Webhook
	query := r.db.WithContext(ctx)
	if projectID != "" {
		query = query.Where("project_id = ? OR project_id = '' OR project_id IS NULL", projectID)
	}
	err := query.Order("created_at DESC").Find(&webhooks).Error
	return webhooks, err
}

func (r *WebhookRepository) Update(ctx context.Context, w *domain.Webhook) error {
	w.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *WebhookRepository) ToggleEnabled(ctx context.Context, id string, enabled bool) error {
	return r.db.WithContext(ctx).Model(&domain.Webhook{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"enabled":    enabled,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *WebhookRepository) RecordDeliveryStats(ctx context.Context, id string, statusCode int, success bool) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"last_delivery_at": &now,
		"last_status_code": statusCode,
		"updated_at":       now,
	}

	tx := r.db.WithContext(ctx).Model(&domain.Webhook{}).Where("id = ?", id)
	if success {
		return tx.Updates(updates).UpdateColumn("success_count", r.db.Raw("success_count + 1")).Error
	}
	return tx.Updates(updates).UpdateColumn("failure_count", r.db.Raw("failure_count + 1")).Error
}

func (r *WebhookRepository) Delete(ctx context.Context, id string) error {
	// Clean up deliveries associated with webhook
	_ = r.db.WithContext(ctx).Where("webhook_id = ?", id).Delete(&domain.WebhookDelivery{}).Error
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Webhook{}).Error
}

// Delivery logging
func (r *WebhookRepository) SaveDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *WebhookRepository) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]domain.WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var deliveries []domain.WebhookDelivery
	query := r.db.WithContext(ctx)
	if webhookID != "" {
		query = query.Where("webhook_id = ?", webhookID)
	}
	err := query.Order("created_at DESC").Limit(limit).Find(&deliveries).Error
	return deliveries, err
}

func (r *WebhookRepository) GetDeliveryByID(ctx context.Context, id string) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

