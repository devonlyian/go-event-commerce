package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/contracts/events"
	"github.com/devonlyian/go-event-commerce/libs/contracts/topics"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
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
	orderRepo         OrderRepository
	outboxRepo        OutboxRepository
	publisher         Publisher
	orderCreatedTopic string
	logger            *zap.Logger
}

type OrderRepository interface {
	CreateWithOutbox(ctx context.Context, order *model.Order, event *model.OutboxEvent) error
	GetByID(ctx context.Context, id string) (*model.Order, error)
	UpdateStatusIfCurrent(ctx context.Context, id, currentStatus, nextStatus string) (bool, error)
}

type OutboxRepository interface {
	MarkPublished(ctx context.Context, eventID string) error
	SetPublishError(ctx context.Context, eventID, message string) error
	ProcessPendingBatch(ctx context.Context, batchSize int, handler func(event *model.OutboxEvent) error) (int, error)
}

func NewOrderService(
	orderRepo OrderRepository,
	outboxRepo OutboxRepository,
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
		Status:     model.StatusCreated,
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

func (s *OrderService) RelayPendingOutbox(ctx context.Context, batchSize int) (int, error) {
	processed, err := s.outboxRepo.ProcessPendingBatch(ctx, batchSize, func(event *model.OutboxEvent) error {
		return s.publisher.Publish(ctx, event.Topic, event.AggregateID, event.Payload)
	})
	if err != nil {
		return 0, fmt.Errorf("process pending outbox: %w", err)
	}
	return processed, nil
}

func (s *OrderService) ApplyPaymentResult(ctx context.Context, event events.PaymentEvent, sourceTopic string) error {
	targetStatus, err := paymentStatusFromTopic(sourceTopic)
	if err != nil {
		return err
	}

	updated, err := s.orderRepo.UpdateStatusIfCurrent(ctx, event.OrderID, model.StatusCreated, targetStatus)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	if updated {
		s.logger.Info("order status updated from payment result",
			zap.String("order_id", event.OrderID),
			zap.String("status", targetStatus),
			zap.String("topic", sourceTopic),
		)
		return nil
	}

	order, err := s.orderRepo.GetByID(ctx, event.OrderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Warn("received payment result for missing order",
				zap.String("order_id", event.OrderID),
				zap.String("topic", sourceTopic),
			)
			return nil
		}
		return fmt.Errorf("load order after status update miss: %w", err)
	}

	switch {
	case order.Status == targetStatus:
		return nil
	case model.IsTerminalStatus(order.Status):
		s.logger.Warn("ignored conflicting payment result for terminal order",
			zap.String("order_id", event.OrderID),
			zap.String("current_status", order.Status),
			zap.String("incoming_status", targetStatus),
			zap.String("topic", sourceTopic),
		)
		return nil
	default:
		return fmt.Errorf("unexpected order status transition from %s to %s", order.Status, targetStatus)
	}
}

func paymentStatusFromTopic(topic string) (string, error) {
	switch topic {
	case topics.PaymentCompleted:
		return model.StatusPaid, nil
	case topics.PaymentFailed:
		return model.StatusPaymentFailed, nil
	default:
		return "", fmt.Errorf("unsupported payment topic: %s", topic)
	}
}
