package repository

import (
	"context"
	"time"

	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
	"gorm.io/gorm"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"published_at":  now,
			"publish_error": "",
		}).Error
}

func (r *OutboxRepository) SetPublishError(ctx context.Context, eventID, message string) error {
	return r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", eventID).
		Update("publish_error", message).Error
}
