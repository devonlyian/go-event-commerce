package repository

import (
	"context"
	"time"

	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
			"attempt_count":   gorm.Expr("attempt_count + ?", 1),
			"last_attempt_at": now,
			"published_at":    now,
			"publish_error":   "",
		}).Error
}

func (r *OutboxRepository) SetPublishError(ctx context.Context, eventID, message string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"attempt_count":   gorm.Expr("attempt_count + ?", 1),
			"last_attempt_at": now,
			"publish_error":   message,
		}).Error
}

func (r *OutboxRepository) ProcessPendingBatch(
	ctx context.Context,
	batchSize int,
	handler func(event *model.OutboxEvent) error,
) (int, error) {
	processed := 0

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var events []model.OutboxEvent
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("published_at IS NULL").
			Order("created_at ASC").
			Limit(batchSize).
			Find(&events).Error; err != nil {
			return err
		}

		for i := range events {
			if err := handler(&events[i]); err != nil {
				if updateErr := setPublishErrorTx(tx, events[i].ID, err.Error()); updateErr != nil {
					return updateErr
				}
				processed++
				continue
			}

			if err := markPublishedTx(tx, events[i].ID); err != nil {
				return err
			}
			processed++
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return processed, nil
}

func markPublishedTx(tx *gorm.DB, eventID string) error {
	now := time.Now().UTC()
	return tx.Model(&model.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"attempt_count":   gorm.Expr("attempt_count + ?", 1),
			"last_attempt_at": now,
			"published_at":    now,
			"publish_error":   "",
		}).Error
}

func setPublishErrorTx(tx *gorm.DB, eventID, message string) error {
	now := time.Now().UTC()
	return tx.Model(&model.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]any{
			"attempt_count":   gorm.Expr("attempt_count + ?", 1),
			"last_attempt_at": now,
			"publish_error":   message,
		}).Error
}
