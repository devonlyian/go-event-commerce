package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devonlyian/go-event-commerce/libs/contracts/events"
	"github.com/devonlyian/go-event-commerce/libs/contracts/topics"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type fakeOrderRepository struct {
	createErr             error
	createdOrder          *model.Order
	createdOutbox         *model.OutboxEvent
	getByIDFn             func(context.Context, string) (*model.Order, error)
	updateStatusIfCurrent func(context.Context, string, string, string) (bool, error)
}

func (f *fakeOrderRepository) CreateWithOutbox(_ context.Context, order *model.Order, event *model.OutboxEvent) error {
	f.createdOrder = order
	f.createdOutbox = event
	return f.createErr
}

func (f *fakeOrderRepository) GetByID(ctx context.Context, id string) (*model.Order, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeOrderRepository) UpdateStatusIfCurrent(ctx context.Context, id, currentStatus, nextStatus string) (bool, error) {
	if f.updateStatusIfCurrent != nil {
		return f.updateStatusIfCurrent(ctx, id, currentStatus, nextStatus)
	}
	return false, nil
}

type fakeOutboxRepository struct {
	markPublishedIDs   []string
	publishErrorCalls  []publishErrorCall
	processPendingFunc func(context.Context, int, func(*model.OutboxEvent) error) (int, error)
}

type publishErrorCall struct {
	eventID string
	message string
}

func (f *fakeOutboxRepository) MarkPublished(_ context.Context, eventID string) error {
	f.markPublishedIDs = append(f.markPublishedIDs, eventID)
	return nil
}

func (f *fakeOutboxRepository) SetPublishError(_ context.Context, eventID, message string) error {
	f.publishErrorCalls = append(f.publishErrorCalls, publishErrorCall{eventID: eventID, message: message})
	return nil
}

func (f *fakeOutboxRepository) ProcessPendingBatch(
	ctx context.Context,
	batchSize int,
	handler func(event *model.OutboxEvent) error,
) (int, error) {
	if f.processPendingFunc != nil {
		return f.processPendingFunc(ctx, batchSize, handler)
	}
	return 0, nil
}

type fakePublisher struct {
	publishErr error
	published  []publishedMessage
}

type publishedMessage struct {
	topic   string
	key     string
	payload []byte
}

func (f *fakePublisher) Publish(_ context.Context, topic, key string, payload []byte) error {
	f.published = append(f.published, publishedMessage{topic: topic, key: key, payload: payload})
	return f.publishErr
}

func (f *fakePublisher) CheckConnection(context.Context) error { return nil }
func (f *fakePublisher) Close() error                          { return nil }

func TestCreateOrderSuccess(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	outboxRepo := &fakeOutboxRepository{}
	publisher := &fakePublisher{}
	svc := NewOrderService(orderRepo, outboxRepo, publisher, topics.OrderCreated, zap.NewNop())

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		CustomerID: "customer-1",
		Amount:     129.99,
	})
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}
	if order.Status != model.StatusCreated {
		t.Fatalf("unexpected order status: %s", order.Status)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}
	if len(outboxRepo.markPublishedIDs) != 1 {
		t.Fatalf("expected outbox mark published call")
	}
	if orderRepo.createdOrder == nil || orderRepo.createdOutbox == nil {
		t.Fatalf("order and outbox should be persisted")
	}

	var payload events.OrderCreatedEvent
	if err := json.Unmarshal(orderRepo.createdOutbox.Payload, &payload); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}
	if payload.OrderID != order.ID {
		t.Fatalf("unexpected payload order id: %s", payload.OrderID)
	}
}

func TestCreateOrderPublishFailureReturnsAcceptedError(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	outboxRepo := &fakeOutboxRepository{}
	publisher := &fakePublisher{publishErr: errors.New("kafka unavailable")}
	svc := NewOrderService(orderRepo, outboxRepo, publisher, topics.OrderCreated, zap.NewNop())

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		CustomerID: "customer-2",
		Amount:     33,
	})
	if order == nil {
		t.Fatal("expected persisted order on publish failure")
	}
	var publishErr *EventPublishError
	if !errors.As(err, &publishErr) {
		t.Fatalf("expected EventPublishError, got %v", err)
	}
	if len(outboxRepo.publishErrorCalls) != 1 {
		t.Fatalf("expected publish error to be stored")
	}
	if len(outboxRepo.markPublishedIDs) != 0 {
		t.Fatalf("mark published should not be called on publish failure")
	}
}

func TestRelayPendingOutboxPublishesPendingEvents(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	outboxRepo := &fakeOutboxRepository{}
	publisher := &fakePublisher{}
	var handlerErr error
	outboxRepo.processPendingFunc = func(_ context.Context, batchSize int, handler func(*model.OutboxEvent) error) (int, error) {
		if batchSize != 10 {
			t.Fatalf("unexpected batch size: %d", batchSize)
		}
		event := &model.OutboxEvent{
			ID:          "outbox-1",
			AggregateID: "order-1",
			Topic:       topics.OrderCreated,
			Payload:     []byte(`{"order_id":"order-1"}`),
		}
		handlerErr = handler(event)
		return 1, nil
	}
	svc := NewOrderService(orderRepo, outboxRepo, publisher, topics.OrderCreated, zap.NewNop())

	processed, err := svc.RelayPendingOutbox(context.Background(), 10)
	if err != nil {
		t.Fatalf("RelayPendingOutbox returned error: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
	if handlerErr != nil {
		t.Fatalf("handler should succeed, got %v", handlerErr)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(publisher.published))
	}
}

func TestRelayPendingOutboxKeepsProcessingWhenPublishFails(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	outboxRepo := &fakeOutboxRepository{}
	publisher := &fakePublisher{publishErr: errors.New("broker down")}
	var handlerErr error
	outboxRepo.processPendingFunc = func(_ context.Context, _ int, handler func(*model.OutboxEvent) error) (int, error) {
		event := &model.OutboxEvent{
			ID:          "outbox-2",
			AggregateID: "order-2",
			Topic:       topics.OrderCreated,
			Payload:     []byte(`{"order_id":"order-2"}`),
		}
		handlerErr = handler(event)
		return 1, nil
	}
	svc := NewOrderService(orderRepo, outboxRepo, publisher, topics.OrderCreated, zap.NewNop())

	processed, err := svc.RelayPendingOutbox(context.Background(), 5)
	if err != nil {
		t.Fatalf("RelayPendingOutbox should not fail on handler error, got %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 processed event, got %d", processed)
	}
	if !errors.Is(handlerErr, publisher.publishErr) {
		t.Fatalf("expected handler error to match publish error, got %v", handlerErr)
	}
}

func TestApplyPaymentResultTransitionsCreatedOrder(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	outboxRepo := &fakeOutboxRepository{}
	publisher := &fakePublisher{}
	var updateArgs []string
	orderRepo.updateStatusIfCurrent = func(_ context.Context, id, currentStatus, nextStatus string) (bool, error) {
		updateArgs = []string{id, currentStatus, nextStatus}
		return true, nil
	}
	svc := NewOrderService(orderRepo, outboxRepo, publisher, topics.OrderCreated, zap.NewNop())

	err := svc.ApplyPaymentResult(context.Background(), events.PaymentEvent{OrderID: "order-3"}, topics.PaymentCompleted)
	if err != nil {
		t.Fatalf("ApplyPaymentResult returned error: %v", err)
	}
	if len(updateArgs) != 3 {
		t.Fatalf("expected update arguments to be recorded")
	}
	if updateArgs[1] != model.StatusCreated || updateArgs[2] != model.StatusPaid {
		t.Fatalf("unexpected transition: %v", updateArgs)
	}
}

func TestApplyPaymentResultIgnoresAlreadyAppliedStatus(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	orderRepo.updateStatusIfCurrent = func(_ context.Context, _, _, _ string) (bool, error) {
		return false, nil
	}
	orderRepo.getByIDFn = func(_ context.Context, _ string) (*model.Order, error) {
		return &model.Order{ID: "order-4", Status: model.StatusPaid}, nil
	}
	svc := NewOrderService(orderRepo, &fakeOutboxRepository{}, &fakePublisher{}, topics.OrderCreated, zap.NewNop())

	if err := svc.ApplyPaymentResult(context.Background(), events.PaymentEvent{OrderID: "order-4"}, topics.PaymentCompleted); err != nil {
		t.Fatalf("ApplyPaymentResult should ignore duplicate terminal status: %v", err)
	}
}

func TestApplyPaymentResultIgnoresConflictingTerminalStatus(t *testing.T) {
	orderRepo := &fakeOrderRepository{}
	orderRepo.updateStatusIfCurrent = func(_ context.Context, _, _, _ string) (bool, error) {
		return false, nil
	}
	orderRepo.getByIDFn = func(_ context.Context, _ string) (*model.Order, error) {
		return &model.Order{ID: "order-5", Status: model.StatusPaid}, nil
	}
	svc := NewOrderService(orderRepo, &fakeOutboxRepository{}, &fakePublisher{}, topics.OrderCreated, zap.NewNop())

	if err := svc.ApplyPaymentResult(context.Background(), events.PaymentEvent{OrderID: "order-5"}, topics.PaymentFailed); err != nil {
		t.Fatalf("ApplyPaymentResult should ignore conflicting terminal status: %v", err)
	}
}
