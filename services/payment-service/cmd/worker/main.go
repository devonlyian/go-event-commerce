package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/platform/logging"
	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/config"
	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/handler"
	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/model"
	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/repository"
	"github.com/devonlyian/go-event-commerce/services/payment-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger, err := logging.New("payment-service")
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumer := repository.NewKafkaConsumer(cfg.KafkaBrokers, cfg.KafkaConsumerGroup, cfg.KafkaOrderCreatedTopic)
	defer func() {
		if err := consumer.Close(); err != nil {
			logger.Warn("failed to close kafka consumer", zap.Error(err))
		}
	}()

	publisher := repository.NewKafkaPublisher(cfg.KafkaBrokers)
	defer func() {
		if err := publisher.Close(); err != nil {
			logger.Warn("failed to close kafka publisher", zap.Error(err))
		}
	}()

	processor := service.NewPaymentProcessor(
		publisher,
		cfg.KafkaPaymentCompletedTopic,
		cfg.KafkaPaymentFailedTopic,
		logger,
	)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := handler.NewHealthHandler(func(checkCtx context.Context) error {
		if err := consumer.CheckConnection(checkCtx); err != nil {
			return err
		}
		if err := publisher.CheckConnection(checkCtx); err != nil {
			return err
		}
		return nil
	})
	healthHandler.Register(r)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info("payment-service health server started", zap.String("port", cfg.HTTPPort))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to start health server", zap.Error(err))
		}
	}()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		consumeLoop(ctx, consumer, processor, logger)
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown health server", zap.Error(err))
	}

	<-workerDone
	logger.Info("payment-service stopped")
}

func consumeLoop(
	ctx context.Context,
	consumer *repository.KafkaConsumer,
	processor *service.PaymentProcessor,
	logger *zap.Logger,
) {
	for {
		msg, err := consumer.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			logger.Warn("failed to read kafka message", zap.Error(err))
			continue
		}

		var event model.OrderCreatedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			logger.Warn("invalid order.created payload",
				zap.Error(err),
				zap.ByteString("raw", msg.Value),
			)
			continue
		}

		if _, err := processor.ProcessOrderCreated(ctx, event); err != nil {
			logger.Error("failed to process payment",
				zap.Error(err),
				zap.String("order_id", event.OrderID),
			)
			continue
		}
	}
}
