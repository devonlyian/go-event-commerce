package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/model"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PaymentPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

type PaymentProcessor struct {
	publisher      PaymentPublisher
	completedTopic string
	failedTopic    string
	logger         *zap.Logger
}

func NewPaymentProcessor(
	publisher PaymentPublisher,
	completedTopic, failedTopic string,
	logger *zap.Logger,
) *PaymentProcessor {
	return &PaymentProcessor{
		publisher:      publisher,
		completedTopic: completedTopic,
		failedTopic:    failedTopic,
		logger:         logger,
	}
}

func (p *PaymentProcessor) ProcessOrderCreated(ctx context.Context, event model.OrderCreatedEvent) (string, error) {
	status, reason := paymentOutcomeForOrder(event.OrderID)
	topic := p.completedTopic
	if status == "failed" {
		topic = p.failedTopic
	}

	paymentEvent := model.PaymentEvent{
		EventID:    uuid.NewString(),
		OrderID:    event.OrderID,
		CustomerID: event.CustomerID,
		Amount:     event.Amount,
		Status:     status,
		Reason:     reason,
		OccurredAt: time.Now().UTC(),
	}

	payload, err := json.Marshal(paymentEvent)
	if err != nil {
		return "", fmt.Errorf("marshal payment event: %w", err)
	}

	if err := p.publisher.Publish(ctx, topic, event.OrderID, payload); err != nil {
		return "", fmt.Errorf("publish payment event: %w", err)
	}

	p.logger.Info("payment event published",
		zap.String("order_id", event.OrderID),
		zap.String("status", status),
		zap.String("topic", topic),
	)

	return status, nil
}

func paymentOutcomeForOrder(orderID string) (string, string) {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(orderID))
	if hasher.Sum32()%5 == 0 {
		return "failed", "card authorization failed (deterministic)"
	}
	return "completed", ""
}
