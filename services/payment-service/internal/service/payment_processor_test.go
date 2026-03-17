package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/model"
	"go.uber.org/zap"
)

type fakePaymentPublisher struct {
	published []paymentPublishCall
}

type paymentPublishCall struct {
	topic   string
	key     string
	payload []byte
}

func (f *fakePaymentPublisher) Publish(_ context.Context, topic, key string, payload []byte) error {
	f.published = append(f.published, paymentPublishCall{topic: topic, key: key, payload: payload})
	return nil
}

func TestPaymentOutcomeForOrderIsDeterministic(t *testing.T) {
	orderID := "order-deterministic"

	status1, reason1 := paymentOutcomeForOrder(orderID)
	status2, reason2 := paymentOutcomeForOrder(orderID)

	if status1 != status2 || reason1 != reason2 {
		t.Fatalf("expected deterministic outcome, got (%s, %s) and (%s, %s)", status1, reason1, status2, reason2)
	}
}

func TestProcessOrderCreatedPublishesExpectedTopicAndPayload(t *testing.T) {
	completedID := findOrderIDForStatus("completed")
	failedID := findOrderIDForStatus("failed")

	testCases := []struct {
		name        string
		orderID     string
		expectTopic string
		expectState string
	}{
		{name: "completed", orderID: completedID, expectTopic: "payment.completed", expectState: "completed"},
		{name: "failed", orderID: failedID, expectTopic: "payment.failed", expectState: "failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			publisher := &fakePaymentPublisher{}
			processor := NewPaymentProcessor(publisher, "payment.completed", "payment.failed", zap.NewNop())

			status, err := processor.ProcessOrderCreated(context.Background(), model.OrderCreatedEvent{
				OrderID:    tc.orderID,
				CustomerID: "customer-1",
				Amount:     10,
			})
			if err != nil {
				t.Fatalf("ProcessOrderCreated returned error: %v", err)
			}
			if status != tc.expectState {
				t.Fatalf("unexpected status: %s", status)
			}
			if len(publisher.published) != 1 {
				t.Fatalf("expected one publish call, got %d", len(publisher.published))
			}
			if publisher.published[0].topic != tc.expectTopic {
				t.Fatalf("unexpected topic: %s", publisher.published[0].topic)
			}

			var payload model.PaymentEvent
			if err := json.Unmarshal(publisher.published[0].payload, &payload); err != nil {
				t.Fatalf("failed to decode payment payload: %v", err)
			}
			if payload.OrderID != tc.orderID || payload.Status != tc.expectState {
				t.Fatalf("unexpected payload: %+v", payload)
			}
		})
	}
}

func findOrderIDForStatus(target string) string {
	for i := 0; i < 10000; i++ {
		orderID := fmt.Sprintf("order-%d", i)
		status, _ := paymentOutcomeForOrder(orderID)
		if status == target {
			return orderID
		}
	}
	panic("failed to find order id for status " + target)
}
