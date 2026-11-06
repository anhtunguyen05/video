// processing-service accepts upload events and durably enqueues jobs for workers.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"video/services/worker/internal/config"
	"video/services/worker/internal/platform/logging"
	"video/services/worker/internal/platform/observability"
	"video/services/worker/internal/platform/postgres"
	"video/services/worker/internal/platform/rabbitmq"
	"video/services/worker/internal/processing/application"
	processingpostgres "video/services/worker/internal/processing/infrastructure/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("processing service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger := logging.New().With("service", "processing-service")
	slog.SetDefault(logger)
	shutdownTrace, err := observability.Initialize(context.Background(), "processing-service")
	if err != nil {
		return fmt.Errorf("initialize tracing: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTrace(ctx); err != nil {
			logger.Error("shutdown tracing", "error", err)
		}
	}()
	cfg := config.Load()
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}
	defer consumer.Close()
	repository := processingpostgres.NewRepository(db)
	service := application.NewReliableProcessingService(repository, nil, repository, nil, nil, nil, nil, nil, nil, "")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("processing service started", "queue", rabbitmq.QueueName)
	return consumer.Run(ctx, service)
}
