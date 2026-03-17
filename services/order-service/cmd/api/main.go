package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/devonlyian/go-event-commerce/libs/contracts/events"
	"github.com/devonlyian/go-event-commerce/libs/platform/logging"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/config"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/handler"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/model"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/repository"
	"github.com/devonlyian/go-event-commerce/services/order-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()

	logger, err := logging.New("order-service")
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := openPostgresWithRetry(ctx, cfg.PostgresDSN, logger)
	if err != nil {
		logger.Fatal("failed to connect postgres", zap.Error(err))
	}

	if err := db.AutoMigrate(&model.Order{}, &model.OutboxEvent{}); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	publisher := service.NewKafkaPublisher(cfg.KafkaBrokers, logger)
	defer func() {
		if err := publisher.Close(); err != nil {
			logger.Warn("failed to close kafka publisher", zap.Error(err))
		}
	}()
	paymentResultConsumer := service.NewKafkaConsumer(
		cfg.KafkaBrokers,
		cfg.KafkaPaymentResultGroup,
		[]string{cfg.KafkaPaymentCompletedTopic, cfg.KafkaPaymentFailedTopic},
	)
	defer func() {
		if err := paymentResultConsumer.Close(); err != nil {
			logger.Warn("failed to close payment result consumer", zap.Error(err))
		}
	}()

	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	orderSvc := service.NewOrderService(orderRepo, outboxRepo, publisher, cfg.KafkaOrderCreatedTopic, logger)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := handler.NewHealthHandler(func(checkCtx context.Context) error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		if err := sqlDB.PingContext(checkCtx); err != nil {
			return err
		}
		if err := publisher.CheckConnection(checkCtx); err != nil {
			return err
		}
		return paymentResultConsumer.CheckConnection(checkCtx)
	})
	healthHandler.Register(r)

	orderHandler := handler.NewOrderHandler(orderSvc, logger)
	orderHandler.Register(r)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		logger.Info("order-service started", zap.String("port", cfg.HTTPPort))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("failed to run http server", zap.Error(err))
		}
	}()

	workerErrCh := make(chan error, 2)
	var workerWG sync.WaitGroup
	workerWG.Add(2)

	go func() {
		defer workerWG.Done()
		if err := runOutboxRelay(ctx, orderSvc, cfg.OutboxRelayInterval, cfg.OutboxBatchSize, logger); err != nil && !errors.Is(err, context.Canceled) {
			workerErrCh <- fmt.Errorf("outbox relay worker failed: %w", err)
			stop()
		}
	}()

	go func() {
		defer workerWG.Done()
		if err := runPaymentResultConsumer(ctx, paymentResultConsumer, orderSvc, logger); err != nil && !errors.Is(err, context.Canceled) {
			workerErrCh <- fmt.Errorf("payment result consumer failed: %w", err)
			stop()
		}
	}()

	var workerErr error
	select {
	case <-ctx.Done():
	case workerErr = <-workerErrCh:
	}

	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown http server", zap.Error(err))
	}

	workerWG.Wait()

	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}

	if workerErr != nil {
		logger.Fatal("order-service stopped due to worker failure", zap.Error(workerErr))
	}

	logger.Info("order-service stopped")
}

func openPostgresWithRetry(ctx context.Context, dsn string, logger *zap.Logger) (*gorm.DB, error) {
	var lastErr error
	for attempt := 1; attempt <= 20; attempt++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		lastErr = err
		logger.Warn("retrying postgres connection", zap.Int("attempt", attempt), zap.Error(err))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("postgres connection failed: %w", lastErr)
}

type paymentResultApplier interface {
	ApplyPaymentResult(ctx context.Context, event events.PaymentEvent, sourceTopic string) error
}

type outboxRelayer interface {
	RelayPendingOutbox(ctx context.Context, batchSize int) (int, error)
}

func runOutboxRelay(
	ctx context.Context,
	relayer outboxRelayer,
	interval time.Duration,
	batchSize int,
	logger *zap.Logger,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		processed, err := relayer.RelayPendingOutbox(ctx, batchSize)
		if err != nil {
			return err
		}
		if processed > 0 {
			logger.Info("outbox relay batch processed", zap.Int("count", processed))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func runPaymentResultConsumer(
	ctx context.Context,
	consumer *service.KafkaConsumer,
	applier paymentResultApplier,
	logger *zap.Logger,
) error {
	for {
		msg, err := consumer.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			return fmt.Errorf("fetch payment result message: %w", err)
		}

		var event events.PaymentEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			logger.Warn("invalid payment result payload",
				zap.Error(err),
				zap.String("topic", msg.Topic),
				zap.ByteString("raw", msg.Value),
			)
			if err := consumer.CommitMessages(ctx, msg); err != nil {
				return fmt.Errorf("commit invalid payment result message: %w", err)
			}
			continue
		}

		if err := applier.ApplyPaymentResult(ctx, event, msg.Topic); err != nil {
			return fmt.Errorf("apply payment result for order %s: %w", event.OrderID, err)
		}

		if err := consumer.CommitMessages(ctx, msg); err != nil {
			return fmt.Errorf("commit payment result message: %w", err)
		}
	}
}
