package main

import (
	"context"
	"errors"
	"testing"

	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/model"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type fakeOrderConsumer struct {
	messages    []kafka.Message
	commitCalls []kafka.Message
	commitErr   error
}

func (f *fakeOrderConsumer) FetchMessage(context.Context) (kafka.Message, error) {
	if len(f.messages) == 0 {
		return kafka.Message{}, context.Canceled
	}
	msg := f.messages[0]
	f.messages = f.messages[1:]
	return msg, nil
}

func (f *fakeOrderConsumer) CommitMessages(_ context.Context, msgs ...kafka.Message) error {
	f.commitCalls = append(f.commitCalls, msgs...)
	return f.commitErr
}

type fakeOrderProcessor struct {
	processErr error
	processed  []model.OrderCreatedEvent
}

func (f *fakeOrderProcessor) ProcessOrderCreated(_ context.Context, event model.OrderCreatedEvent) (string, error) {
	f.processed = append(f.processed, event)
	return "completed", f.processErr
}

func TestConsumeLoopCommitsOnSuccessfulProcessing(t *testing.T) {
	consumer := &fakeOrderConsumer{
		messages: []kafka.Message{{Value: []byte(`{"order_id":"order-1","customer_id":"customer-1","amount":12.3}`)}},
	}
	processor := &fakeOrderProcessor{}

	err := consumeLoop(context.Background(), consumer, processor, zap.NewNop())
	if err != nil {
		t.Fatalf("consumeLoop returned error: %v", err)
	}
	if len(consumer.commitCalls) != 1 {
		t.Fatalf("expected one commit call, got %d", len(consumer.commitCalls))
	}
	if len(processor.processed) != 1 {
		t.Fatalf("expected one processed event, got %d", len(processor.processed))
	}
}

func TestConsumeLoopReturnsWorkerErrorWithoutCommitOnProcessingFailure(t *testing.T) {
	consumer := &fakeOrderConsumer{
		messages: []kafka.Message{{Value: []byte(`{"order_id":"order-2","customer_id":"customer-2","amount":99.1}`)}},
	}
	processor := &fakeOrderProcessor{processErr: errors.New("publish failed")}

	err := consumeLoop(context.Background(), consumer, processor, zap.NewNop())
	if err == nil {
		t.Fatal("expected consumeLoop to return error")
	}
	if len(consumer.commitCalls) != 0 {
		t.Fatalf("commit should not happen on processing failure")
	}
}

func TestConsumeLoopCommitsMalformedPayload(t *testing.T) {
	consumer := &fakeOrderConsumer{
		messages: []kafka.Message{{Value: []byte(`{"order_id":`)}},
	}
	processor := &fakeOrderProcessor{}

	err := consumeLoop(context.Background(), consumer, processor, zap.NewNop())
	if err != nil {
		t.Fatalf("consumeLoop returned error: %v", err)
	}
	if len(consumer.commitCalls) != 1 {
		t.Fatalf("expected malformed payload to be committed once, got %d", len(consumer.commitCalls))
	}
	if len(processor.processed) != 0 {
		t.Fatalf("processor should not be called for malformed payload")
	}
}
