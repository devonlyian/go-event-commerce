package service

import (
	"context"

	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/model"
	"go.uber.org/zap"
)

type NotificationService struct {
	logger *zap.Logger
}

func NewNotificationService(logger *zap.Logger) *NotificationService {
	return &NotificationService{logger: logger}
}

func (s *NotificationService) Notify(ctx context.Context, event model.PaymentEvent, sourceTopic string) {
	_ = ctx
	s.logger.Info("notification dispatched",
		zap.String("topic", sourceTopic),
		zap.String("order_id", event.OrderID),
		zap.String("customer_id", event.CustomerID),
		zap.String("status", event.Status),
		zap.Float64("amount", event.Amount),
		zap.String("reason", event.Reason),
	)
}
