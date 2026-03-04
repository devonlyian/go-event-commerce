package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		return publisher.CheckConnection(checkCtx)
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

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown http server", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
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
