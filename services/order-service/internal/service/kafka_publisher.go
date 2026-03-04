package service

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type Publisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
	CheckConnection(ctx context.Context) error
	Close() error
}

type KafkaPublisher struct {
	writer  *kafka.Writer
	brokers []string
	logger  *zap.Logger
}

func NewKafkaPublisher(brokers []string, logger *zap.Logger) *KafkaPublisher {
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: false,
			RequiredAcks:           kafka.RequireOne,
			BatchTimeout:           50 * time.Millisecond,
		},
		brokers: brokers,
		logger:  logger,
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
		Time:  time.Now().UTC(),
	}
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("publish kafka message: %w", err)
	}
	return nil
}

func (p *KafkaPublisher) CheckConnection(ctx context.Context) error {
	if len(p.brokers) == 0 {
		return fmt.Errorf("no kafka brokers configured")
	}
	conn, err := kafka.DialContext(ctx, "tcp", p.brokers[0])
	if err != nil {
		return err
	}
	return conn.Close()
}

func (p *KafkaPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
