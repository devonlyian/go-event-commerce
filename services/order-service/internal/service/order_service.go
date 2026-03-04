package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/contracts/events"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CreateOrderInput struct {
	CustomerID string
	Amount     float64
}

type EventPublishError struct {
	Cause error
}

func (e *EventPublishError) Error() string {
	if e.Cause == nil {
		return "order persisted but event publish failed"
	}
	return fmt.Sprintf("order persisted but event publish failed: %v", e.Cause)
}

func (e *EventPublishError) Unwrap() error {
	return e.Cause
}

type OrderService struct {
	orderRepo         *repository.OrderRepository
	outboxRepo        *repository.OutboxRepository
	publisher         Publisher
	orderCreatedTopic string
	logger            *zap.Logger
}

func NewOrderService(
	orderRepo *repository.OrderRepository,
	outboxRepo *repository.OutboxRepository,
	publisher Publisher,
	orderCreatedTopic string,
	logger *zap.Logger,
) *OrderService {
	return &OrderService{
		orderRepo:         orderRepo,
		outboxRepo:        outboxRepo,
		publisher:         publisher,
		orderCreatedTopic: orderCreatedTopic,
		logger:            logger,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, input CreateOrderInput) (*model.Order, error) {
	order := &model.Order{
		ID:         uuid.NewString(),
		CustomerID: input.CustomerID,
		Amount:     input.Amount,
		Status:     "created",
	}

	eventPayload := events.OrderCreatedEvent{
		EventID:    uuid.NewString(),
		OrderID:    order.ID,
		CustomerID: order.CustomerID,
		Amount:     order.Amount,
		OccurredAt: time.Now().UTC(),
	}

	payload, err := json.Marshal(eventPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal order event payload: %w", err)
	}

	outbox := &model.OutboxEvent{
		ID:            uuid.NewString(),
		AggregateID:   order.ID,
		AggregateType: "order",
		Topic:         s.orderCreatedTopic,
		Payload:       payload,
	}

	if err := s.orderRepo.CreateWithOutbox(ctx, order, outbox); err != nil {
		return nil, fmt.Errorf("create order with outbox: %w", err)
	}

	if err := s.publisher.Publish(ctx, outbox.Topic, outbox.AggregateID, outbox.Payload); err != nil {
		if updateErr := s.outboxRepo.SetPublishError(ctx, outbox.ID, err.Error()); updateErr != nil {
			s.logger.Error("failed to save outbox publish error", zap.Error(updateErr), zap.String("outbox_id", outbox.ID))
		}
		return order, &EventPublishError{Cause: err}
	}

	if err := s.outboxRepo.MarkPublished(ctx, outbox.ID); err != nil {
		s.logger.Warn("failed to mark outbox as published", zap.Error(err), zap.String("outbox_id", outbox.ID))
	}

	return order, nil
}

func (s *OrderService) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get order by id: %w", err)
	}
	return order, nil
}
