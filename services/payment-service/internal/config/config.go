package config

import (
	"os"
	"strings"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/contracts/topics"
)

type Config struct {
	HTTPPort                   string
	KafkaBrokers               []string
	KafkaOrderCreatedTopic     string
	KafkaPaymentCompletedTopic string
	KafkaPaymentFailedTopic    string
	KafkaConsumerGroup         string
	ShutdownTimeout            time.Duration
}

func Load() Config {
	return Config{
		HTTPPort:                   getEnv("HTTP_PORT", "8081"),
		KafkaBrokers:               splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaOrderCreatedTopic:     getEnv("KAFKA_TOPIC_ORDER_CREATED", topics.OrderCreated),
		KafkaPaymentCompletedTopic: getEnv("KAFKA_TOPIC_PAYMENT_COMPLETED", topics.PaymentCompleted),
		KafkaPaymentFailedTopic:    getEnv("KAFKA_TOPIC_PAYMENT_FAILED", topics.PaymentFailed),
		KafkaConsumerGroup:         getEnv("KAFKA_GROUP_ID", "payment-service"),
		ShutdownTimeout:            getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"localhost:9092"}
	}
	return out
}
