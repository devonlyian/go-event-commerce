package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaConsumer struct {
	reader  *kafka.Reader
	brokers []string
}

func NewKafkaConsumer(brokers []string, groupID, topic string) *KafkaConsumer {
	return &KafkaConsumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  brokers,
			GroupID:  groupID,
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 10e6,
		}),
		brokers: brokers,
	}
}

func (c *KafkaConsumer) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return c.reader.ReadMessage(ctx)
}

func (c *KafkaConsumer) CheckConnection(ctx context.Context) error {
	if len(c.brokers) == 0 {
		return fmt.Errorf("no kafka brokers configured")
	}
	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn.Close()
}

func (c *KafkaConsumer) Close() error {
	if c.reader == nil {
		return nil
	}
	return c.reader.Close()
}
