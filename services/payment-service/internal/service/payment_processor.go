package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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
	rng            *rand.Rand
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
		rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
		logger:         logger,
	}
}

func (p *PaymentProcessor) ProcessOrderCreated(ctx context.Context, event model.OrderCreatedEvent) (string, error) {
	status := "completed"
	topic := p.completedTopic
	reason := ""

	if p.rng.Float64() < 0.2 {
		status = "failed"
		topic = p.failedTopic
		reason = "card authorization failed (simulated)"
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
