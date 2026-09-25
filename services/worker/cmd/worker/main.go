package main

import (
	"context"
	"os/signal"
	"syscall"

	"video/services/worker/internal/config"
	"video/services/worker/internal/platform/logging"
	"video/services/worker/internal/platform/postgres"
	"video/services/worker/internal/platform/rabbitmq"
	processingapplication "video/services/worker/internal/processing/application"
	processingPostgres "video/services/worker/internal/processing/infrastructure/postgres"
)

func main() {
	logger := logging.New()
	cfg := config.Load()
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		return
	}
	defer db.Close()

	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("create RabbitMQ consumer", "error", err)
		return
	}
	defer consumer.Close()
	processingService := processingapplication.NewService(processingPostgres.NewRepository(db))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "env", cfg.AppEnv, "queue", rabbitmq.QueueName)
	if err := consumer.Run(ctx, processingService); err != nil {
		logger.Error("worker consumer stopped", "error", err)
	}
	logger.Info("shutdown signal received")
	logger.Info("worker stopped")
}
