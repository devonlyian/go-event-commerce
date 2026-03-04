package config

import (
	"os"
	"strings"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/contracts/topics"
)

type Config struct {
	HTTPPort               string
	PostgresDSN            string
	KafkaBrokers           []string
	KafkaOrderCreatedTopic string
	ShutdownTimeout        time.Duration
}

func Load() Config {
	return Config{
		HTTPPort:               getEnv("HTTP_PORT", "8080"),
		PostgresDSN:            getEnv("POSTGRES_DSN", "host=localhost user=commerce password=commerce dbname=commerce port=5432 sslmode=disable TimeZone=UTC"),
		KafkaBrokers:           splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaOrderCreatedTopic: getEnv("KAFKA_TOPIC_ORDER_CREATED", topics.OrderCreated),
		ShutdownTimeout:        getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
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
