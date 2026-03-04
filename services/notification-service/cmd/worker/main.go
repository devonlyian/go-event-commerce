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
	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/config"
	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/handler"
	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/model"
	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/repository"
	"github.com/devonlyian/go-event-commerce/services/notification-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger, err := logging.New("notification-service")
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	consumer := repository.NewKafkaConsumer(cfg.KafkaBrokers, cfg.KafkaConsumerGroup, cfg.KafkaTopics)
	defer func() {
		if err := consumer.Close(); err != nil {
			logger.Warn("failed to close kafka consumer", zap.Error(err))
		}
	}()

	notifier := service.NewNotificationService(logger)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := handler.NewHealthHandler(func(checkCtx context.Context) error {
		return consumer.CheckConnection(checkCtx)
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
		logger.Info("notification-service health server started", zap.String("port", cfg.HTTPPort))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to start health server", zap.Error(err))
		}
	}()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		consumeLoop(ctx, consumer, notifier, logger)
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown health server", zap.Error(err))
	}

	<-workerDone
	logger.Info("notification-service stopped")
}

func consumeLoop(
	ctx context.Context,
	consumer *repository.KafkaConsumer,
	notifier *service.NotificationService,
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

		var event model.PaymentEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			logger.Warn("invalid payment payload",
				zap.Error(err),
				zap.ByteString("raw", msg.Value),
			)
			continue
		}

		notifier.Notify(ctx, event, msg.Topic)
	}
}
